// Copyright 2026 Certen Protocol
//
// GET /api/v1/proofs/{proof_id}/finality serves what the validators have recorded about a proof's staged finality (PROOF_V2
// section 7, V2_SPEC_S7): which levels have an addendum, and for the others the builder's last stated reason (pending with an
// eta, unavailable, refused, not applicable). It is a status summary of stored rows, not a verdict: nothing here says verified,
// and the level it names is "as recorded by the validators". The levels are independent (any order, one addendum per level);
// `included` is derived from the proof's own settlement evidence and is never stored.

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// FinalitySummaryFormat is the format string of the summary document.
const FinalitySummaryFormat = "certen-finality-summary/1"

// finalityStatusFormat is the format of the validators' builder status document (the jsonb of proof_finality_status).
const finalityStatusFormat = "certen-finality-status/1"

// The states a level can be reported in. present: an addendum is stored. pending, unavailable, refused, not_applicable: the
// builder's stored judgement (class). derived: `included`, never stored. not_judged: the builder has recorded nothing for this
// level yet. status_verified_without_addendum: the status row says the level was stored but no addendum row exists.
const (
	LevelStatePresent        = "present"
	LevelStatePending        = "pending"
	LevelStateUnavailable    = "unavailable"
	LevelStateRefused        = "refused"
	LevelStateNotApplicable  = "not_applicable"
	LevelStateDerived        = "derived"
	LevelStateNotJudged      = "not_judged"
	LevelStateVerifiedNoRow  = "status_verified_without_addendum"
	includedBasisDescription = "derived from the proof's own settlement evidence (S6); never stored as an addendum and not checked by this service"
)

// FinalityLevelState is one level of the summary.
type FinalityLevelState struct {
	State        string          `json:"state"`
	Basis        string          `json:"basis,omitempty"`
	Chain        int64           `json:"chain,omitempty"`
	Kind         string          `json:"kind,omitempty"`
	Seq          int64           `json:"seq,omitempty"`
	CreatedAt    string          `json:"createdAt,omitempty"`
	LevelReached string          `json:"levelReached,omitempty"`
	NextLevel    string          `json:"nextLevel,omitempty"`
	ETA          string          `json:"eta,omitempty"`
	Reason       json.RawMessage `json:"reason,omitempty"`
	JudgedAt     string          `json:"judgedAt,omitempty"`
}

// FinalityLevels are the three levels in ladder order.
type FinalityLevels struct {
	Included  FinalityLevelState `json:"included"`
	L1Posted  FinalityLevelState `json:"l1_posted"`
	Finalized FinalityLevelState `json:"finalized"`
}

// FinalitySummary is the response body. RecordedLevel is the highest level with a stored addendum, or "included" when there is
// none: the level as recorded, not as verified. Leg is the block the addenda are bound to (null when there are no addenda).
type FinalitySummary struct {
	Format        string         `json:"format"`
	ProofID       string         `json:"proofId"`
	RecordedLevel string         `json:"recordedLevel"`
	Leg           *finalityLeg   `json:"leg"`
	Levels        FinalityLevels `json:"levels"`
}

// statusDocument is the stored builder status (certen-finality-status/1), read strictly.
type statusDocument struct {
	Format       string          `json:"format"`
	Class        string          `json:"class"`
	LevelReached string          `json:"levelReached"`
	NextLevel    string          `json:"nextLevel"`
	ETA          string          `json:"eta"`
	Seq          uint64          `json:"seq"`
	State        json.RawMessage `json:"state"`
}

func validLevelOrNone(s string) bool {
	switch s {
	case "none", "included", "l1_posted", "finalized":
		return true
	}
	return false
}

func statusMalformed(level string, reason string) error {
	return finalityErr(FinalityStatusMalformed, "level", level, "reason", reason)
}

// levelFromStatus turns a stored status row into the level's reported state.
func levelFromStatus(level string, row database.FinalityStatus) (FinalityLevelState, error) {
	var d statusDocument
	dec := json.NewDecoder(bytes.NewReader(row.State))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return FinalityLevelState{}, statusMalformed(level, "not_a_status_document")
	}
	if d.Format != finalityStatusFormat {
		return FinalityLevelState{}, statusMalformed(level, "wrong_format")
	}
	var state string
	switch d.Class {
	case "pending":
		state = LevelStatePending
	case "unavailable":
		state = LevelStateUnavailable
	case "refused":
		state = LevelStateRefused
	case "not_applicable":
		state = LevelStateNotApplicable
	case "verified":
		state = LevelStateVerifiedNoRow
	default:
		return FinalityLevelState{}, statusMalformed(level, "unknown_class")
	}
	if !validLevelOrNone(d.LevelReached) || !validLevelOrNone(d.NextLevel) {
		return FinalityLevelState{}, statusMalformed(level, "unknown_level")
	}
	if d.ETA != "" {
		if _, err := time.Parse(time.RFC3339, d.ETA); err != nil {
			return FinalityLevelState{}, statusMalformed(level, "bad_eta")
		}
	}
	out := FinalityLevelState{
		State: state, Chain: row.Chain, LevelReached: d.LevelReached, NextLevel: d.NextLevel, ETA: d.ETA,
		JudgedAt: row.JudgedAt.UTC().Format(time.RFC3339), Seq: int64(d.Seq),
	}
	hasState := len(d.State) > 0 && !bytes.Equal(bytes.TrimSpace(d.State), []byte("null"))
	switch {
	case d.Class == "verified" && hasState:
		return FinalityLevelState{}, statusMalformed(level, "verified_carries_state")
	case d.Class != "verified" && !hasState:
		return FinalityLevelState{}, statusMalformed(level, "state_missing")
	case hasState:
		var obj map[string]json.RawMessage
		if json.Unmarshal(d.State, &obj) != nil {
			return FinalityLevelState{}, statusMalformed(level, "state_not_an_object")
		}
		out.Reason = d.State
	}
	return out, nil
}

// SummarizeFinality builds the summary from a proof's stored addendum heads (seq order) and builder status rows. A level with an
// addendum is present whatever its status row says; a level without one is reported as the builder last judged it, or as
// not_judged. Rows that contradict each other (two legs, a header that disagrees with its row) are refused by name, as in the
// portable member.
func SummarizeFinality(proofID uuid.UUID, heads []database.FinalityAddendumHead, statuses []database.FinalityStatus) (*FinalitySummary, error) {
	s := &FinalitySummary{Format: FinalitySummaryFormat, ProofID: proofID.String(), RecordedLevel: "included"}
	s.Levels.Included = FinalityLevelState{State: LevelStateDerived, Basis: includedBasisDescription}
	byLevel := map[int16]*FinalityLevelState{finalityLevelL1Posted: &s.Levels.L1Posted, finalityLevelFinalized: &s.Levels.Finalized}
	names := map[int16]string{finalityLevelL1Posted: "l1_posted", finalityLevelFinalized: "finalized"}

	facts := make([]finalityHeadFacts, len(heads))
	present := map[int16]bool{}
	for i, a := range heads {
		if a.ProofID != proofID {
			return nil, finalityErr(FinalityRowInvalid, "seq", seqStr(a.Seq), "field", "proofId")
		}
		target, ok := byLevel[a.Level]
		if !ok {
			return nil, finalityErr(FinalityRowInvalid, "seq", seqStr(a.Seq), "field", "level")
		}
		if present[a.Level] {
			return nil, finalityErr(FinalityRowInvalid, "seq", seqStr(a.Seq), "field", "level_repeated")
		}
		present[a.Level] = true
		*target = FinalityLevelState{State: LevelStatePresent, Chain: a.Chain, Kind: a.Kind, Seq: a.Seq, CreatedAt: a.CreatedAt.UTC().Format(time.RFC3339)}
		facts[i] = finalityHeadFacts{Seq: a.Seq, Level: a.Level, Chain: a.Chain, Kind: a.Kind, Header: a.Header}
	}
	if len(heads) > 0 {
		leg, err := legOfRows(facts)
		if err != nil {
			return nil, err
		}
		s.Leg = &leg
		if present[finalityLevelFinalized] {
			s.RecordedLevel = names[finalityLevelFinalized]
		} else {
			s.RecordedLevel = names[finalityLevelL1Posted]
		}
	}
	for _, row := range statuses {
		if row.ProofID != proofID {
			return nil, statusMalformed(fmt.Sprint(row.TargetLevel), "proof_id")
		}
		target, ok := byLevel[row.TargetLevel]
		if !ok {
			return nil, statusMalformed(fmt.Sprint(row.TargetLevel), "target_level")
		}
		if present[row.TargetLevel] {
			continue
		}
		st, err := levelFromStatus(names[row.TargetLevel], row)
		if err != nil {
			return nil, err
		}
		*target = st
	}
	for _, target := range byLevel {
		if target.State == "" {
			*target = FinalityLevelState{State: LevelStateNotJudged}
		}
	}
	return s, nil
}

// finalitySummaryStore is what the summary endpoint needs of the database.
type finalitySummaryStore interface {
	ProofExists(ctx context.Context, proofID uuid.UUID) (bool, error)
	ListFinalityAddendumHeads(ctx context.Context, proofID uuid.UUID) ([]database.FinalityAddendumHead, error)
	ListFinalityStatus(ctx context.Context, proofID uuid.UUID) ([]database.FinalityStatus, error)
}

// FinalityHandlers serves the finality summary.
type FinalityHandlers struct {
	store  finalitySummaryStore
	logger *log.Logger
}

// NewFinalityHandlers returns the handlers over a store.
func NewFinalityHandlers(store finalitySummaryStore, logger *log.Logger) *FinalityHandlers {
	if logger == nil {
		logger = log.New(log.Writer(), "[FinalityAPI] ", log.LstdFlags)
	}
	return &FinalityHandlers{store: store, logger: logger}
}

// HandleGetProofFinality serves GET /api/v1/proofs/{proof_id}/finality.
//
//	200  the summary
//	404  PROOF_NOT_FOUND: no such proof
//	500  the rows cannot be read or contradict each other: never reported as "not found" or as a shorter summary
func (h *FinalityHandlers) HandleGetProofFinality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/proofs/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "finality" {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PATH", "Invalid endpoint path")
		return
	}
	proofID, err := uuid.Parse(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PROOF_ID", "Invalid proof ID format")
		return
	}
	exists, err := h.store.ProofExists(r.Context(), proofID)
	if err != nil {
		h.logger.Printf("finality of %s: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the proof")
		return
	}
	if !exists {
		writeJSONError(w, http.StatusNotFound, "PROOF_NOT_FOUND", fmt.Sprintf("No proof found with ID: %s", proofID))
		return
	}
	heads, err := h.store.ListFinalityAddendumHeads(r.Context(), proofID)
	if err != nil {
		h.logger.Printf("finality addenda of %s: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality addenda")
		return
	}
	statuses, err := h.store.ListFinalityStatus(r.Context(), proofID)
	if err != nil {
		h.logger.Printf("finality status of %s: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality status")
		return
	}
	summary, err := SummarizeFinality(proofID, heads, statuses)
	if err != nil {
		// SummarizeFinality returns only named states (FinalityComposeError); the text carries the code and the seq or level.
		h.logger.Printf("finality of %s does not summarise: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "FINALITY_SUMMARY_UNUSABLE", err.Error())
		return
	}
	body, err := json.Marshal(summary)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to encode the finality summary")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	// A level appears when the validators record it; a cached summary would hide it.
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
