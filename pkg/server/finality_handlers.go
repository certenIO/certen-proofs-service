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
	"reflect"
	"sort"
	"strconv"
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
// level yet. status_stored_without_addendum: the status row says the level was stored but no addendum row exists.
const (
	LevelStatePresent        = "present"
	LevelStatePending        = "pending"
	LevelStateUnavailable    = "unavailable"
	LevelStateRefused        = "refused"
	LevelStateNotApplicable  = "not_applicable"
	LevelStateDerived        = "derived"
	LevelStateNotJudged      = "not_judged"
	LevelStateStoredNoRow    = "status_stored_without_addendum"
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
	// PinsSHA256 is the identity (sha256, 64 lowercase hex) of the pins the builder judged this level under, when its status row
	// records it (the additive pinsSha256 member of certen-finality-status/1). A consumer that verifies under the pins IT trusts can
	// compare it with its own. Absent when the row records none; on a stored level only when the row is that addendum's own verified
	// status. Additive and last: a summary of rows without it is byte for byte what it was.
	PinsSHA256 string `json:"pinsSha256,omitempty"`
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

// statusDocument is the stored builder status (certen-finality-status/1), read strictly: a member that is not listed here is
// refused (DisallowUnknownFields). The members the validators add are added here, and validated, before they are written.
type statusDocument struct {
	Format       string          `json:"format"`
	Class        string          `json:"class"`
	LevelReached string          `json:"levelReached"`
	NextLevel    string          `json:"nextLevel"`
	ETA          string          `json:"eta"`
	Seq          uint64          `json:"seq"`
	State        json.RawMessage `json:"state"`
	// PinsSHA256 is the additive member pinsSha256 (validator pkg/finality/status.go, pv2/rb11-dbfix): the sha256 of the pins the
	// builder used, 64 lowercase hex, omitted by the writer when it holds none. It is kept raw so that a member that is present but
	// null, empty or not a string is told from an absent one and refused by its own reason instead of being read as absent.
	PinsSHA256 json.RawMessage `json:"pinsSha256"`
}

// The reasons a stored status document is refused besides the shape ones above. Each names the member it is about.
const (
	reasonBadPinsSHA256     = "bad_pins_sha256"
	reasonBadAttempt        = "bad_attempt"
	reasonBadNextAttemptAt  = "bad_next_attempt_at"
	reasonNotAStatusDoc     = "not_a_status_document"
	reasonWrongStatusFormat = "wrong_format"
	// reasonMemberCase: a member is spelled in another case than the contract spells it (PINSSHA256 for pinsSha256). encoding/json
	// would match it to the member; the validator's writer never produces it, so it is refused, naming the member it resembles.
	reasonMemberCase = "member_name_case"
)

// statusMembers are the names of the members of certen-finality-status/1, taken from the tags of statusDocument so that a member added
// to the struct is covered by the case check without a second list.
var statusMembers = jsonMembersOf(reflect.TypeOf(statusDocument{}))

func jsonMembersOf(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); name != "" && name != "-" {
			out = append(out, name)
		}
	}
	return out
}

// caseVariantOf returns the member of exact that a key of keys resembles without being spelled exactly like it (same letters, another
// case), or "" when every key that resembles one is spelled exactly. Keys are visited in sorted order so the answer is deterministic.
func caseVariantOf(keys []string, exact ...string) string {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for _, k := range sorted {
		for _, m := range exact {
			if k != m && strings.EqualFold(k, m) {
				return m
			}
		}
	}
	return ""
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// decodeStatus reads a stored status document strictly: a member that is not listed is refused, and so is a member spelled in another
// case than the contract spells it (encoding/json alone would read it as the member). The reason is empty when it decoded and carries
// the right format; for reasonMemberCase, member is the contract's spelling of the member the stored one resembles.
func decodeStatus(raw []byte) (d statusDocument, reason string, member string) {
	var names map[string]json.RawMessage
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&names); err != nil {
		return statusDocument{}, reasonNotAStatusDoc, ""
	}
	if m := caseVariantOf(keysOf(names), statusMembers...); m != "" {
		return statusDocument{}, reasonMemberCase, m
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return statusDocument{}, reasonNotAStatusDoc, ""
	}
	if d.Format != finalityStatusFormat {
		return statusDocument{}, reasonWrongStatusFormat, ""
	}
	return d, "", ""
}

// pinsIdentity is the document's pins identity: ("", true) when the member is absent, (hex, true) when it is a JSON string of exactly
// 64 lowercase hex characters, and ("", false) for anything else a writer could not have produced (null, "", a number, upper case,
// a prefix, a different length).
func pinsIdentity(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil || len(s) != 64 {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return "", false
		}
	}
	return s, true
}

// backOffFieldsReason checks the two fields a failed ask adds to a status' named state (validator pv2/rb12-p0-live): attempt is the
// decimal ordinal (1 or more, no sign, no leading zero, within uint64) of the failed ask, next_attempt_at an RFC 3339 time. They are
// fields of the state, not members of the document, so they are validated where they appear and nowhere else; a state without
// them, or whose fields are not an object, is read as it always was.
func backOffFieldsReason(state map[string]json.RawMessage) (reason, member string) {
	// the members of a named state are code and fields; one spelled in another case is not read as either
	if m := caseVariantOf(keysOf(state), "code", "fields"); m != "" {
		return reasonMemberCase, m
	}
	rawFields, ok := state["fields"]
	if !ok {
		return "", ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(rawFields, &fields) != nil {
		return "", ""
	}
	// the other fields are free-form names; these two are validated, so a spelling of either in another case is refused, not skipped
	if m := caseVariantOf(keysOf(fields), "attempt", "next_attempt_at"); m != "" {
		return reasonMemberCase, m
	}
	if raw, ok := fields["attempt"]; ok {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return reasonBadAttempt, ""
		}
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil || n == 0 || strconv.FormatUint(n, 10) != s {
			return reasonBadAttempt, ""
		}
	}
	if raw, ok := fields["next_attempt_at"]; ok {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return reasonBadNextAttemptAt, ""
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return reasonBadNextAttemptAt, ""
		}
	}
	return "", ""
}

// pinsOfStoredLevel is the pins identity to show on a level that has an addendum. The level is present whatever its status row
// says, so the row is only believed for this: it must be the VERIFIED status of this very addendum (class verified, the
// addendum's seq) and carry a well-formed identity. Anything else (a stale pending row, another addendum's row, a malformed
// document) shows no identity; it never changes the level or fails the summary.
func pinsOfStoredLevel(row database.FinalityStatus, seq int64) string {
	d, reason, _ := decodeStatus(row.State)
	if reason != "" || d.Class != "verified" || seq <= 0 || d.Seq != uint64(seq) {
		return ""
	}
	pins, ok := pinsIdentity(d.PinsSHA256)
	if !ok {
		return ""
	}
	return pins
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

// statusMemberMalformed is statusMalformed for reasonMemberCase: it names the member of the contract the stored one resembles (never the
// stored spelling, which is not the writer's text).
func statusMemberMalformed(level, reason, member string) error {
	if member == "" {
		return statusMalformed(level, reason)
	}
	return finalityErr(FinalityStatusMalformed, "level", level, "reason", reason, "member", member)
}

// levelFromStatus turns a stored status row into the level's reported state.
func levelFromStatus(level string, row database.FinalityStatus) (FinalityLevelState, error) {
	d, reason, member := decodeStatus(row.State)
	if reason != "" {
		return FinalityLevelState{}, statusMemberMalformed(level, reason, member)
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
		state = LevelStateStoredNoRow
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
	pins, ok := pinsIdentity(d.PinsSHA256)
	if !ok {
		return FinalityLevelState{}, statusMalformed(level, reasonBadPinsSHA256)
	}
	out := FinalityLevelState{
		State: state, Chain: row.Chain, LevelReached: d.LevelReached, NextLevel: d.NextLevel, ETA: d.ETA,
		JudgedAt: row.JudgedAt.UTC().Format(time.RFC3339), Seq: int64(d.Seq), PinsSHA256: pins,
	}
	hasState := len(d.State) > 0 && !bytes.Equal(bytes.TrimSpace(d.State), []byte("null"))
	switch {
	case d.Class == "verified" && hasState:
		return FinalityLevelState{}, statusMalformed(level, "stored_carries_state")
	case d.Class != "verified" && !hasState:
		return FinalityLevelState{}, statusMalformed(level, "state_missing")
	case hasState:
		var obj map[string]json.RawMessage
		if json.Unmarshal(d.State, &obj) != nil {
			return FinalityLevelState{}, statusMalformed(level, "state_not_an_object")
		}
		if reason, member := backOffFieldsReason(obj); reason != "" {
			return FinalityLevelState{}, statusMemberMalformed(level, reason, member)
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
			target.PinsSHA256 = pinsOfStoredLevel(row, target.Seq)
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
	// The status rows are read BEFORE the addendum heads. The validators commit an addendum and then, separately, the status row
	// that says it is stored; read in this order a status row that says "stored" implies a head that is already visible, so a
	// commit between the two reads cannot show the builder's own bookkeeping as a disagreement between their records. (The other
	// race, a head appearing after an older status was read, only shows a stored level beside a stale status, and a stored level
	// wins.)
	statuses, err := h.store.ListFinalityStatus(r.Context(), proofID)
	if err != nil {
		h.logger.Printf("finality status of %s: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality status")
		return
	}
	heads, err := h.store.ListFinalityAddendumHeads(r.Context(), proofID)
	if err != nil {
		h.logger.Printf("finality addenda of %s: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality addenda")
		return
	}
	summary, err := SummarizeFinality(proofID, heads, statuses)
	if err != nil {
		// SummarizeFinality returns only named states (FinalityComposeError); the text carries the code and the seq or level.
		h.logger.Printf("FINALITY_SUMMARY_UNUSABLE proof=%s %v", proofID, err)
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
