// Copyright 2026 Certen Protocol
//
// The fail-before tests of POST /api/v1/proofs/verify/governance (DAX-F1). They use only the HTTP surface and a bare handler, so they
// compile against the handler as it was at the base commit, where each of them FAILS: that handler answered valid:true for any request
// whose proof_data merely had the right member names with non-nil values.
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postGovernance(t *testing.T, body string) (int, map[string]interface{}) {
	t.Helper()
	h := &BundleHandlers{logger: log.New(io.Discard, "", 0)}
	rec := httptest.NewRecorder()
	h.HandleVerifyGovernance(rec, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/verify/governance", strings.NewReader(body)))
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

// the members the old handler read as "evidence" are not evidence: present, non-nil, and nothing was verified
func TestGovernanceFailBeforePresentGarbageIsNotValid(t *testing.T) {
	for name, body := range map[string]string{
		"G0 merkle_proof present":                `{"governance_level":"G0","proof_data":{"merkle_proof":{"anything":1}}}`,
		"G1 authority_snapshot and signatures":   `{"governance_level":"G1","proof_data":{"authority_snapshot":{"a":1},"signatures":[1]}}`,
		"G2 post_state_hash and inclusion_proof": `{"governance_level":"G2","proof_data":{"post_state_hash":"00","inclusion_proof":{"x":1}}}`,
		"G0 a receipt that proves nothing":       `{"governance_level":"G0","proof_data":{"receipt":{"start":"zz","anchor":"yy"}}}`,
		"G1 a portable document that is not one": `{"governance_level":"G1","pinned_incarnation":"` + strings.Repeat("ab", 32) + `","proof_data":{"portable":{"format":"certen-proof-v2-accumulate-portable/1"}}}`,
	} {
		status, out := postGovernance(t, body)
		if status == http.StatusOK && out["valid"] == true {
			t.Fatalf("%s: answered valid:true for evidence that verifies nothing: %v", name, out)
		}
	}
}

// an unknown, unverified or absent verifier input is a named state and never a default true
func TestGovernanceFailBeforeNoEvidenceIsNeverValid(t *testing.T) {
	for name, body := range map[string]string{
		"neither source":   `{"governance_level":"G1"}`,
		"empty proof_data": `{"governance_level":"G0","proof_data":{}}`,
	} {
		status, out := postGovernance(t, body)
		if status == http.StatusOK && out["valid"] == true {
			t.Fatalf("%s: valid:true: %v", name, out)
		}
		if status == http.StatusOK {
			if s, _ := out["state"].(string); s == "" {
				t.Fatalf("%s: a 200 verdict must carry a named state: %v", name, out)
			}
		}
	}
}

// DAX-F16: a receipt the caller invented (self-consistent: start equals anchor with no path, or a one-entry path to a root computed
// by the caller) is not valid. At e6b4ba0 the answer was valid:true state:verified; no pinned anchor means a named state now.
func TestGovernanceFailBeforeForgedReceiptIsNotValid(t *testing.T) {
	leaf := strings.Repeat("ab", 32)
	sibling := strings.Repeat("cd", 32)
	lb, _ := hex.DecodeString(leaf)
	sb, _ := hex.DecodeString(sibling)
	root := sha256.Sum256(append(append([]byte{}, lb...), sb...))
	forged := map[string]string{
		"start == anchor, empty path": `{"start":"` + leaf + `","anchor":"` + leaf + `","localBlock":1,"entries":[]}`,
		"one-entry path":              `{"start":"` + leaf + `","anchor":"` + hex.EncodeToString(root[:]) + `","localBlock":1,"entries":[{"hash":"` + sibling + `","right":true}]}`,
	}
	for name, receipt := range forged {
		for _, level := range []string{"G0", "G2"} {
			status, out := postGovernance(t, `{"governance_level":"`+level+`","proof_data":{"receipt":`+receipt+`}}`)
			if status == http.StatusOK && out["valid"] == true {
				t.Fatalf("%s %s: a receipt the caller invented answered valid:true state=%v", name, level, out["state"])
			}
		}
	}
}
