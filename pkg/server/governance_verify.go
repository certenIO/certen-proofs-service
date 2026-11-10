// Copyright 2026 Certen Protocol
//
// POST /api/v1/proofs/verify/governance: verify governance evidence for real.
//
// WHAT THIS REPLACES (DAX-F1). The handler used to answer `valid = level.Verified` for a stored proof (it echoed a flag a writer
// had set) and `valid = true` for any request whose proof_data had the right member names with non-nil values. Neither read a byte
// of evidence. Both are gone: every `valid:true` this file produces is the verdict of the validator's own offline verifier,
// run over the evidence bytes, in this process.
//
// THE SAME CODE, NOT A LOOKALIKE (DAX-F2). The verifier is imported, not rewritten:
//
//	G0, G2  proof.GovReceiptEvidence.VerifyMerkle              (github.com/certen/independant-validator/pkg/proof)
//	G1      proofv2.VerifyPortable                             (github.com/certen/independant-validator/pkg/proof/v2)
//	stored  proof.GovernanceLevelsFromStorage                  (the reader cmd/proofverify --governance uses)
//
// go.mod resolves that module path to certenIO/certen-validator at one pinned commit (docs/verify-governance.md records the
// commit and the sha256 of every source file this service depends on, and TestGovernanceVerifierIsTheRecordedValidatorCode fails
// when the compiled dependency stops matching the record).
//
// FAIL CLOSED. Anything not established is a NAMED state with valid=false. There is no default true: the only path to
// `valid:true` is governanceVerified, reached after the verifier returned no error, the verdict was complete, and (G1) the
// incarnation the evidence proves equals the incarnation the caller or the operator pinned.
package server

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/certen/independant-validator/pkg/proof"
	proofv2 "github.com/certen/independant-validator/pkg/proof/v2"

	"github.com/certen/proofs-service/pkg/database"
)

// The named states of the endpoint. `verified` is the only one that carries valid=true.
const (
	GovStateVerified             = "verified"
	GovStateEvidenceAbsent       = "evidence_absent"          // nothing was supplied or stored to verify
	GovStateSummaryOnly          = "summary_only"             // a stored level records a conclusion but no evidence: nothing to re-check, never verified
	GovStateEvidenceMalformed    = "evidence_malformed"       // the evidence does not parse or does not compose
	GovStateEvidenceInvalid      = "evidence_invalid"         // the verifier refused the evidence
	GovStateEvidenceUnproven     = "evidence_unproven"        // the verifier accepted it but reports facts it could not establish
	GovStateSetUnverified        = "validator_set_unverified" // the validator set was asserted, not derived and bound
	GovStateIncarnationNotPinned = "incarnation_not_pinned"   // no incarnation was pinned, so which chain the evidence is about is unknown
	GovStateIncarnationMismatch  = "incarnation_mismatch"     // the evidence is about another Accumulate incarnation than the pinned one
	GovStateLevelNotStored       = "level_not_stored"         // the proof has no stored row for the requested level
)

// MaxGovernanceVerifyBodyBytes bounds one verification request. A live evidence 2.1 document without its finality member is
// about 1.1 MB; the cap is far above that and far below what would make a request a memory problem.
const MaxGovernanceVerifyBodyBytes = 32 << 20

// governanceVerifyBodyLimit is the limit in force; a variable only so a test can lower it.
var governanceVerifyBodyLimit int64 = MaxGovernanceVerifyBodyBytes

// governanceStore is what the endpoint reads of the database for a stored proof.
type governanceStore interface {
	GetGovernanceProofLevels(ctx context.Context, proofID uuid.UUID) ([]database.GovernanceProofLevel, error)
	GetPortableV2(ctx context.Context, proofID uuid.UUID) (*database.PortableV2, error)
}

// GovernanceVerificationRequest represents a governance proof verification request.
//
// Exactly one of ProofID and ProofData is given. ProofData members are kept as the caller's bytes (json.RawMessage): evidence is
// hashed as it stands, so a decode into interface{} (float64 numbers, sorted keys) would verify a different document.
type GovernanceVerificationRequest struct {
	ProofID         string                     `json:"proof_id"`
	GovernanceLevel string                     `json:"governance_level"` // G0, G1, G2
	ProofData       map[string]json.RawMessage `json:"proof_data"`
	// PinnedIncarnation is the Accumulate incarnation (hex32) the caller trusts, from its own record. Optional when the service
	// has one configured; G1 is never `verified` without one of them.
	PinnedIncarnation string `json:"pinned_incarnation,omitempty"`
}

// GovernanceVerificationResponse is the verdict. Valid is true only when State is "verified".
type GovernanceVerificationResponse struct {
	Valid      bool                   `json:"valid"`
	Level      string                 `json:"level"`
	State      string                 `json:"state"`
	Reason     string                 `json:"reason,omitempty"`
	Details    map[string]interface{} `json:"details"`
	VerifiedAt time.Time              `json:"verified_at"`
}

type governanceVerdict struct {
	state   string
	reason  string
	details map[string]interface{}
}

func (v governanceVerdict) valid() bool { return v.state == GovStateVerified }

func govFail(state, format string, args ...interface{}) governanceVerdict {
	return governanceVerdict{state: state, reason: boundedReason(fmt.Sprintf(format, args...)), details: map[string]interface{}{}}
}

// boundedReason keeps a verifier's error text from growing a response without limit.
func boundedReason(s string) string {
	const max = 600
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

// parsePinnedIncarnation reads a hex32 incarnation; "" is "none".
func parsePinnedIncarnation(s string) (pin [32]byte, set bool, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return pin, false, nil
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	b, derr := hex.DecodeString(s)
	if derr != nil || len(b) != 32 {
		return pin, false, errors.New("an incarnation is 64 hex characters (32 bytes)")
	}
	copy(pin[:], b)
	return pin, true, nil
}

// HandleVerifyGovernance handles POST /api/v1/proofs/verify/governance.
//
//	400  the request itself is malformed (not JSON, unknown member, bad level, bad proof_id, bad pin, neither or both of
//	     proof_id and proof_data)
//	405  not POST
//	413  the body is larger than MaxGovernanceVerifyBodyBytes
//	500  the stored evidence could not be read
//	200  a verdict: valid=true only with state "verified"; every other state is valid=false and names why
func (h *BundleHandlers) HandleVerifyGovernance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only POST is allowed")
		return
	}

	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, governanceVerifyBodyLimit))
	var top json.RawMessage
	if err := dec.Decode(&top); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			h.writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", fmt.Sprintf("The request is larger than %d bytes", governanceVerifyBodyLimit))
			return
		}
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request format")
		return
	}
	if _, err := dec.Token(); err != io.EOF {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request format: data after the JSON value")
		return
	}
	// member names are matched EXACTLY: encoding/json would otherwise read "Governance_Level" as governance_level
	var req GovernanceVerificationRequest
	if err := decodeExactMembers(top, &req, "proof_id", "governance_level", "proof_data", "pinned_incarnation"); err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request format: "+err.Error())
		return
	}

	if req.GovernanceLevel != "G0" && req.GovernanceLevel != "G1" && req.GovernanceLevel != "G2" {
		h.writeError(w, http.StatusBadRequest, "INVALID_LEVEL", "governance_level must be 'G0', 'G1', or 'G2'")
		return
	}
	hasID := req.ProofID != ""
	hasData := req.ProofData != nil
	if hasID == hasData {
		h.writeError(w, http.StatusBadRequest, "INVALID_EVIDENCE_SOURCE", "give exactly one of proof_id (a stored proof) and proof_data (evidence in the request)")
		return
	}

	pin, pinSet, err := parsePinnedIncarnation(req.PinnedIncarnation)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "INVALID_PINNED_INCARNATION", err.Error())
		return
	}
	pinSource := "request"
	if !pinSet {
		pin, pinSet, err = parsePinnedIncarnation(h.pinnedIncarnation)
		if err != nil {
			// the operator's own setting is unusable: not a client error, and never read as "no pin needed"
			h.logger.Printf("verify/governance: the configured pinned incarnation is invalid: %v", err)
			h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The service's pinned incarnation is not usable")
			return
		}
		pinSource = "service"
	}
	if !pinSet {
		pinSource = ""
	}

	var verdict governanceVerdict
	if hasID {
		proofID, perr := uuid.Parse(req.ProofID)
		if perr != nil {
			h.writeError(w, http.StatusBadRequest, "INVALID_PROOF_ID", "Invalid proof ID format")
			return
		}
		if h.governance == nil {
			h.logger.Printf("verify/governance: no store is configured")
			h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve governance proof")
			return
		}
		verdict, err = h.verifyStoredGovernance(r.Context(), h.governance, proofID, req.GovernanceLevel, pin, pinSet, pinSource)
		if err != nil {
			h.logger.Printf("verify/governance: proof %s: %v", proofID, err)
			h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve governance proof")
			return
		}
	} else {
		verdict = verifyGovernanceProofData(req.GovernanceLevel, req.ProofData, pin, pinSet, pinSource)
	}

	details := verdict.details
	if details == nil {
		details = map[string]interface{}{}
	}
	h.writeJSON(w, http.StatusOK, GovernanceVerificationResponse{
		Valid:      verdict.valid(),
		Level:      req.GovernanceLevel,
		State:      verdict.state,
		Reason:     verdict.reason,
		Details:    details,
		VerifiedAt: time.Now().UTC(),
	})
}

// verifyStoredGovernance verifies the evidence a stored proof carries. It never reads a stored verdict flag (level.Verified):
// the stored bytes are re-verified. An error is a storage failure (HTTP 500), not a verdict.
func (h *BundleHandlers) verifyStoredGovernance(ctx context.Context, store governanceStore, proofID uuid.UUID, level string, pin [32]byte, pinSet bool, pinSource string) (governanceVerdict, error) {
	if level == "G1" {
		p, err := store.GetPortableV2(ctx, proofID)
		if err != nil {
			return governanceVerdict{}, fmt.Errorf("read the portable proof v2 document: %w", err)
		}
		if p == nil {
			return govFail(GovStateEvidenceAbsent, "no portable proof v2 document is stored for this proof, so its governance correctness (G1) cannot be re-checked"), nil
		}
		doc, err := ComposePortable(p)
		if err != nil {
			return govFail(GovStateEvidenceMalformed, "the stored portable document does not compose: %v", err), nil
		}
		return verifyGovernancePortable(doc, pin, pinSet, pinSource), nil
	}

	rows, err := store.GetGovernanceProofLevels(ctx, proofID)
	if err != nil {
		return governanceVerdict{}, fmt.Errorf("read the governance levels: %w", err)
	}
	stored := make([]proof.StoredGovLevelRow, 0, len(rows))
	for _, l := range rows {
		stored = append(stored, proof.StoredGovLevelRow{GovLevel: string(l.GovLevel), LevelName: l.LevelName, LevelJSON: l.LevelJSON})
	}
	// the validator's own reader (the one cmd/proofverify --governance uses): level_json is read as stored, never repaired
	levels, err := proof.GovernanceLevelsFromStorage(ctx, sliceGovernanceReader(stored), proofID)
	if err != nil {
		if errors.Is(err, proof.ErrNoStoredGovernanceLevels) {
			return govFail(GovStateEvidenceAbsent, "no governance levels are stored for this proof"), nil
		}
		return govFail(GovStateEvidenceMalformed, "the stored governance levels do not read: %v", err), nil
	}
	for i := range levels {
		l := &levels[i]
		if l.Level != level {
			continue
		}
		if !l.HasEvidence() {
			// the stored row carries the conclusion and not the evidence: nothing was checked and nothing is claimed
			return govFail(GovStateSummaryOnly, "the stored %s level records a conclusion without a receipt path, so it cannot be re-checked", level), nil
		}
		return verifyGovernanceReceipt(level, l.Receipt), nil
	}
	return govFail(GovStateLevelNotStored, "no %s level is stored for this proof", level), nil
}

// sliceGovernanceReader serves the rows the repository already read, to the validator's reader.
type sliceGovernanceReader []proof.StoredGovLevelRow

func (s sliceGovernanceReader) GovernanceLevelRows(_ context.Context, _ uuid.UUID) ([]proof.StoredGovLevelRow, error) {
	return s, nil
}

// verifyGovernanceProofData verifies evidence carried in the request.
//
//	G0, G2  proof_data.receipt   a receipt evidence object {level?, start, anchor, localBlock, entries}
//	G1      proof_data.portable  a portable proof v2 document (certen-proof-v2-accumulate-portable/1)
func verifyGovernanceProofData(level string, data map[string]json.RawMessage, pin [32]byte, pinSet bool, pinSource string) governanceVerdict {
	if level == "G1" {
		raw, ok := data["portable"]
		if !ok || isJSONNull(raw) {
			return govFail(GovStateEvidenceAbsent, "G1 evidence is the portable proof v2 document in proof_data.portable")
		}
		for k := range data {
			if k != "portable" {
				return govFail(GovStateEvidenceMalformed, "proof_data for G1 carries only \"portable\"; unknown member %q", k)
			}
		}
		return verifyGovernancePortable(raw, pin, pinSet, pinSource)
	}

	raw, ok := data["receipt"]
	if !ok || isJSONNull(raw) {
		return govFail(GovStateEvidenceAbsent, "%s evidence is the receipt in proof_data.receipt", level)
	}
	for k := range data {
		if k != "receipt" {
			return govFail(GovStateEvidenceMalformed, "proof_data for %s carries only \"receipt\"; unknown member %q", level, k)
		}
	}
	var ev proof.GovReceiptEvidence
	if err := decodeExactMembers(raw, &ev, "level", "start", "anchor", "localBlock", "entries"); err != nil {
		return govFail(GovStateEvidenceMalformed, "the receipt does not parse: %v", err)
	}
	if ev.Level != "" && ev.Level != level {
		return govFail(GovStateEvidenceInvalid, "the receipt is labelled %s and the request asks about %s", ev.Level, level)
	}
	ev.Level = level
	if !ev.HasPath() {
		return govFail(GovStateEvidenceAbsent, "the receipt has no start or anchor")
	}
	return verifyGovernanceReceipt(level, &ev)
}

// verifyGovernanceReceipt recomputes one governance receipt from its own bytes with the validator's receipt verifier. It
// establishes that the leaf is under the receipt's own anchor, and nothing about whether that anchor is a real Accumulate anchor
// (that is G1's portable evidence): the response says so.
func verifyGovernanceReceipt(level string, ev *proof.GovReceiptEvidence) (v governanceVerdict) {
	defer func() {
		if rec := recover(); rec != nil {
			v = govFail(GovStateEvidenceInvalid, "the receipt verifier refused the evidence (%v)", rec)
		}
	}()
	if ev == nil || !ev.HasPath() {
		return govFail(GovStateSummaryOnly, "the %s level carries no receipt path", level)
	}
	if err := ev.VerifyMerkle(); err != nil {
		return govFail(GovStateEvidenceInvalid, "%v", err)
	}
	return governanceVerdict{state: GovStateVerified, details: map[string]interface{}{
		"start":       ev.Start,
		"anchor":      ev.Anchor,
		"local_block": ev.LocalBlock,
		"path_steps":  len(ev.Entries),
		"establishes": "the leaf (start) recomputes to the receipt's stated anchor from the path in the evidence",
		"does_not":    "show that the anchor is a real Accumulate anchor: that is the portable G1 evidence",
		"verifier":    "proof.GovReceiptEvidence.VerifyMerkle",
	}}
}

// verifyGovernancePortable verifies a portable proof v2 document with the validator's offline verifier and judges the
// incarnation it proves against the pinned one.
func verifyGovernancePortable(doc []byte, pin [32]byte, pinSet bool, pinSource string) (v governanceVerdict) {
	defer func() {
		if rec := recover(); rec != nil {
			v = govFail(GovStateEvidenceInvalid, "the proof v2 verifier refused the document (%v)", rec)
		}
	}()
	var p proofv2.Portable
	dec := json.NewDecoder(bytes.NewReader(doc))
	if err := dec.Decode(&p); err != nil {
		return govFail(GovStateEvidenceMalformed, "the portable document does not parse: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return govFail(GovStateEvidenceMalformed, "the portable document has data after its JSON value")
	}
	rep, err := proofv2.VerifyPortable(&p)
	if err != nil {
		return govFail(GovStateEvidenceInvalid, "%v", err)
	}
	if rep == nil {
		return govFail(GovStateEvidenceInvalid, "the verifier returned no report")
	}

	details := map[string]interface{}{
		"incarnation":     hex.EncodeToString(rep.Incarnation[:]),
		"evidence":        evidenceLabel(rep.Version),
		"majors":          rep.Majors,
		"certified_block": rep.CertifiedBlock,
		"certified_root":  hex.EncodeToString(rep.CertifiedRoot[:]),
		"check_block":     rep.CheckBlock,
		"partition":       rep.Partition,
		"anchor_block":    rep.AnchorBlock,
		"tx_hash":         hex.EncodeToString(rep.TxHash[:]),
		"validators":      rep.Validators,
		"threshold":       rep.Threshold,
		"set_verdict":     string(rep.SetVerdict),
		"verifier":        "proofv2.VerifyPortable",
	}
	if pinSet {
		details["pinned_incarnation"] = hex.EncodeToString(pin[:])
		details["pinned_by"] = pinSource
	}

	if !pinSet {
		return governanceVerdict{state: GovStateIncarnationNotPinned, details: details,
			reason: "the evidence verifies against the incarnation it names, but no incarnation was pinned (request pinned_incarnation or the service's CERTEN_PINNED_INCARNATION), so which Accumulate chain it is about is not established"}
	}
	if rep.Incarnation != pin {
		return governanceVerdict{state: GovStateIncarnationMismatch, details: details,
			reason: "the evidence is about another Accumulate incarnation than the pinned one"}
	}
	if rep.SetVerdict != proof.VerdictVerified {
		return governanceVerdict{state: GovStateSetUnverified, details: details,
			reason: fmt.Sprintf("the validator set verdict is %q, not %q", rep.SetVerdict, proof.VerdictVerified)}
	}
	if len(rep.Unproven) > 0 {
		codes := make([]string, 0, len(rep.Unproven))
		for i := range rep.Unproven {
			codes = append(codes, rep.Unproven[i].Code)
		}
		details["unproven"] = codes
		return governanceVerdict{state: GovStateEvidenceUnproven, details: details,
			reason: "the verifier could not establish every fact: " + strings.Join(codes, ", ")}
	}
	return governanceVerdict{state: GovStateVerified, details: details}
}

func evidenceLabel(version string) string {
	if version == "" {
		return "2.0"
	}
	return version
}

func isJSONNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// ValidatePinnedIncarnation reports whether s is usable as the service's pinned incarnation ("" is: no pin configured). The
// service refuses to start on a value that is set and unusable, so a typo cannot read as "no pin".
func ValidatePinnedIncarnation(s string) error {
	_, _, err := parsePinnedIncarnation(s)
	return err
}

// decodeExactMembers decodes one JSON object into v after requiring that every member name is one of allowed, spelled exactly
// (encoding/json matches names case-insensitively, so a variant such as "Start" would otherwise be read as "start"), and that
// nothing follows the object.
func decodeExactMembers(raw []byte, v interface{}, allowed ...string) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var members map[string]json.RawMessage
	if err := dec.Decode(&members); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("data after the JSON value")
	}
	if members == nil {
		return errors.New("not a JSON object")
	}
	ok := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		ok[a] = true
	}
	var unknown []string
	for k := range members {
		if !ok[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unknown member %q (names are matched exactly)", unknown[0])
	}
	return json.Unmarshal(raw, v)
}
