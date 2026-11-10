// Copyright 2026 Certen Protocol
//
// The fail-before tests of POST /api/v1/proofs/verify/governance (DAX-F1). They use only the HTTP surface and a bare handler, so they
// compile against the handler as it was at the base commit, where each of them FAILS: that handler answered valid:true for any request
// whose proof_data merely had the right member names with non-nil values.
package server

import (
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
