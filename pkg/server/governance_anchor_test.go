// Copyright 2026 Certen Protocol
//
// DAX-F16: a G0/G2 receipt is verified only against the anchor the caller pinned. VerifyMerkle checks a receipt against ITS OWN
// anchor and accepts an empty path when start equals anchor, so a receipt the caller invented passes it; without a pinned anchor
// the answer is anchor_unbound (valid=false), with another one anchor_mismatch (valid=false).
package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/certen/proofs-service/pkg/database"
)

// govForgedReceipts are receipts nobody produced: each is self-consistent, so the validator's VerifyMerkle accepts it.
func govForgedReceipts(t *testing.T) map[string]map[string]interface{} {
	t.Helper()
	leaf := strings.Repeat("ab", 32)
	sibling := strings.Repeat("cd", 32)
	lb, _ := hex.DecodeString(leaf)
	sb, _ := hex.DecodeString(sibling)
	root := sha256.Sum256(append(append([]byte{}, lb...), sb...)) // the sibling is on the right
	return map[string]map[string]interface{}{
		"start == anchor, empty path": {"start": leaf, "anchor": leaf, "localBlock": 1, "entries": []interface{}{}},
		"one-entry path": {"start": leaf, "anchor": hex.EncodeToString(root[:]), "localBlock": 1,
			"entries": []interface{}{map[string]interface{}{"hash": sibling, "right": true}}},
	}
}

func govReceiptBody(t *testing.T, level string, receipt interface{}, pin string) string {
	t.Helper()
	b := map[string]interface{}{"governance_level": level, "proof_data": map[string]interface{}{"receipt": receipt}}
	if pin != "" {
		b["pinned_anchor"] = pin
	}
	return govMustJSON(t, b)
}

func TestGovernanceForgedReceiptIsNeverValidWithoutAPinnedAnchor(t *testing.T) {
	for name, rc := range govForgedReceipts(t) {
		for _, level := range []string{"G0", "G2"} {
			// the validator's own receipt check accepts it: the answer must still not be valid
			got := govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, govReceiptBody(t, level, rc, ""))
			govWantRefused(t, got, GovStateAnchorUnbound)
			if got.Resp.Details["anchor"] != rc["anchor"] {
				t.Fatalf("%s %s: the refusal must hand the caller the receipt's anchor to pin: %v", name, level, got.Resp.Details)
			}
			if !strings.Contains(got.Resp.Reason, "pinned_anchor") {
				t.Fatalf("%s %s: the reason must say how to bind the anchor: %q", name, level, got.Resp.Reason)
			}
			// the real anchor of a real transaction is not the invented receipt's anchor
			real := govLiveReceipt(t)["anchor"].(string)
			got = govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, govReceiptBody(t, level, rc, real))
			govWantRefused(t, got, GovStateAnchorMismatch)
			// stored, the same: a stored row with an invented receipt is not valid unpinned
			st := &govFakeStore{levels: []database.GovernanceProofLevel{govReceiptRow(level, rc, true)}}
			got = govCall(t, govTestHandler(st, ""), http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": level}))
			govWantRefused(t, got, GovStateAnchorUnbound)
		}
	}
}

func TestGovernanceRealReceiptNeedsThePinnedAnchorInBothSources(t *testing.T) {
	rc := govLiveReceipt(t)
	anchor := rc["anchor"].(string)
	wrong := govFlipHex(anchor)
	for _, level := range []string{"G0", "G2"} {
		carried := func(pin string) govCallResult {
			return govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, govReceiptBody(t, level, rc, pin))
		}
		stored := func(pin string) govCallResult {
			st := &govFakeStore{levels: []database.GovernanceProofLevel{govReceiptRow(level, rc, false)}}
			b := map[string]string{"proof_id": liveProofID, "governance_level": level}
			if pin != "" {
				b["pinned_anchor"] = pin
			}
			return govCall(t, govTestHandler(st, ""), http.MethodPost, govMustJSON(t, b))
		}
		for src, call := range map[string]func(string) govCallResult{"carried": carried, "stored": stored} {
			// no pin: the receipt recomputes to its own anchor and that is all
			un := call("")
			govWantRefused(t, un, GovStateAnchorUnbound)
			if un.Resp.Details["anchor"] != anchor || un.Resp.Details["path_steps"].(float64) != 55 {
				t.Fatalf("%s %s: details must carry the receipt's anchor: %v", level, src, un.Resp.Details)
			}
			// the correct pin: valid, and the details say what is and is not established
			ok := call(anchor)
			if !ok.Resp.Valid || ok.Resp.State != GovStateVerified || ok.Resp.Details["pinned_anchor"] != anchor {
				t.Fatalf("%s %s: the correct pin must verify: %+v", level, src, ok.Resp)
			}
			if _, has := ok.Resp.Details["establishes"]; !has {
				t.Fatalf("%s %s: the verdict must say what is established", level, src)
			}
			if d, _ := ok.Resp.Details["does_not"].(string); !strings.Contains(d, "pinned anchor is the right one") {
				t.Fatalf("%s %s: the verdict must say that the pin is the caller's record: %q", level, src, d)
			}
			// the same bytes written in capitals and with a 0x prefix are the same anchor
			if r := call("0x" + strings.ToUpper(anchor)); !r.Resp.Valid {
				t.Fatalf("%s %s: anchors are compared as bytes, not strings: %+v", level, src, r.Resp)
			}
			// a wrong pin
			govWantRefused(t, call(wrong), GovStateAnchorMismatch)
		}
	}
}

func TestGovernanceMalformedPinnedAnchorIsA400(t *testing.T) {
	rc := govLiveReceipt(t)
	for name, pin := range map[string]string{
		"too short":   "abcd",
		"not hex":     strings.Repeat("zz", 32),
		"63 digits":   strings.Repeat("a", 63),
		"65 digits":   strings.Repeat("a", 65),
		"two anchors": strings.Repeat("ab", 32) + strings.Repeat("ab", 32),
	} {
		for _, level := range []string{"G0", "G2"} {
			carried := govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, govReceiptBody(t, level, rc, pin))
			st := &govFakeStore{levels: []database.GovernanceProofLevel{govReceiptRow(level, rc, false)}}
			stored := govCall(t, govTestHandler(st, ""), http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": level, "pinned_anchor": pin}))
			for src, got := range map[string]govCallResult{"carried": carried, "stored": stored} {
				if got.Status != http.StatusBadRequest || !strings.Contains(got.Raw, "INVALID_PINNED_ANCHOR") {
					t.Fatalf("%s %s %s: a set but unusable pinned_anchor is a 400 INVALID_PINNED_ANCHOR: %d %.200s", name, level, src, got.Status, got.Raw)
				}
			}
		}
	}
	// the member name is matched exactly, like every other member
	got := govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, `{"governance_level":"G0","Pinned_Anchor":"`+strings.Repeat("ab", 32)+`","proof_data":{"receipt":{}}}`)
	if got.Status != http.StatusBadRequest {
		t.Fatalf("a member name in another case is refused: %d", got.Status)
	}
}
