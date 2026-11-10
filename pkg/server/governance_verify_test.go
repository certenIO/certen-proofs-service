// Copyright 2026 Certen Protocol
//
// Tests of POST /api/v1/proofs/verify/governance (DAX-F1, DAX-F2). Every positive here is a verdict of the validator's own
// offline verifier over a REAL proof; every negative is a refusal the old handler answered `valid:true` (or echoed a stored flag).
//
// Fixtures (pkg/server/testdata/governance):
//
//	stored_proof_live21.json.gz   the stored form (document + spine) of a real proof of a real proof-gated contract call on Base
//	                              Sepolia, captured from the hosted gateway on 2026-11-06 (proof 6c28abe7-475e-4b1a-9374-db7954cc2381,
//	                              Accumulate Kermit tx 348e3d94...). Evidence 2.1, 556 major blocks.
//	vectors/*.json                the validator's own certify1 conformance vectors, byte for byte, with the validator's expected verdicts.
package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// liveIncarnation is the Kermit incarnation the real fixture proves (the SDK's proofv2 manifest records the same value for its
// 2.0 fixture of the same network): the value an operator would pin from their own record.
const liveIncarnation = "cac6698ed49a286ad8a3de94540a3354dfe964f366a439f4fdfb34533059fda0"

const liveProofID = "6c28abe7-475e-4b1a-9374-db7954cc2381"

type govStoredFixture struct {
	SourceFile string   `json:"source_file"`
	ProofID    string   `json:"proof_id"`
	Document   string   `json:"document"`
	Majors     int      `json:"majors"`
	GovRoot    string   `json:"govroot_v3_inputs"`
	Spine      []string `json:"spine"`
}

func loadGovStored(t *testing.T) govStoredFixture {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "governance", "stored_proof_live21.json.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var fx govStoredFixture
	if err := json.NewDecoder(zr).Decode(&fx); err != nil {
		t.Fatal(err)
	}
	if fx.Majors != len(fx.Spine) || fx.ProofID != liveProofID {
		t.Fatalf("fixture is not what the tests expect: majors %d spine %d proof %s", fx.Majors, len(fx.Spine), fx.ProofID)
	}
	return fx
}

func (fx govStoredFixture) portable() *database.PortableV2 {
	return &database.PortableV2{Document: fx.Document, Majors: fx.Majors, GovRootInputs: fx.GovRoot, Spine: append([]string(nil), fx.Spine...)}
}

// govFakeStore is the store behind the handler: what the database would return for a proof.
type govFakeStore struct {
	levels      []database.GovernanceProofLevel
	levelsErr   error
	portable    *database.PortableV2
	portableErr error
}

func (s *govFakeStore) GetGovernanceProofLevels(_ context.Context, _ uuid.UUID) ([]database.GovernanceProofLevel, error) {
	return s.levels, s.levelsErr
}

func (s *govFakeStore) GetPortableV2(_ context.Context, _ uuid.UUID) (*database.PortableV2, error) {
	return s.portable, s.portableErr
}

func govTestHandler(store governanceStore, servicePin string) *BundleHandlers {
	return &BundleHandlers{logger: log.New(io.Discard, "", 0), governance: store, pinnedIncarnation: servicePin}
}

type govCallResult struct {
	Status int
	Resp   GovernanceVerificationResponse
	Raw    string
}

// call posts body to the handler and enforces the one invariant of the endpoint on EVERY response: valid is true only with the
// state "verified", and only on a 200.
func govCall(t *testing.T, h *BundleHandlers, method string, body string) govCallResult {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/proofs/verify/governance", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleVerifyGovernance(rec, req)
	out := govCallResult{Status: rec.Code, Raw: rec.Body.String()}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out.Resp); err != nil {
			t.Fatalf("200 body is not a verdict: %v: %.200s", err, out.Raw)
		}
		if out.Resp.Valid && out.Resp.State != GovStateVerified {
			t.Fatalf("valid:true with state %q", out.Resp.State)
		}
		if !out.Resp.Valid && out.Resp.State == GovStateVerified {
			t.Fatalf("state verified with valid:false")
		}
		if out.Resp.State == "" {
			t.Fatalf("a verdict without a state: %s", out.Raw)
		}
	} else if strings.Contains(out.Raw, `"valid":true`) {
		t.Fatalf("a non-200 response says valid:true: %.200s", out.Raw)
	}
	return out
}

func govMustJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func govWantRefused(t *testing.T, got govCallResult, state string) {
	t.Helper()
	if got.Status != http.StatusOK || got.Resp.Valid || got.Resp.State != state {
		t.Fatalf("want 200 valid=false state=%s, got %d valid=%v state=%q reason=%q", state, got.Status, got.Resp.Valid, got.Resp.State, got.Resp.Reason)
	}
	if got.Resp.Reason == "" {
		t.Fatalf("a refusal must say why (state %s)", state)
	}
}

// ---------------------------------------------------------------------------------------------------------------------
// G1: the real stored proof (positive) and its refusals
// ---------------------------------------------------------------------------------------------------------------------

func TestGovernanceG1RealStoredProofVerifies(t *testing.T) {
	fx := loadGovStored(t)
	h := govTestHandler(&govFakeStore{portable: fx.portable()}, "")
	got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G1", "pinned_incarnation": liveIncarnation}))
	if got.Status != 200 || !got.Resp.Valid || got.Resp.State != GovStateVerified {
		t.Fatalf("a real stored proof must verify: %d %+v", got.Status, got.Resp)
	}
	d := got.Resp.Details
	if d["incarnation"] != liveIncarnation || d["evidence"] != "2.1" || d["set_verdict"] != "verified" || d["pinned_by"] != "request" {
		t.Fatalf("details do not describe the verified evidence: %v", d)
	}
	if d["majors"].(float64) != 556 || d["tx_hash"] != "348e3d94f7df467dbbfdd76b1a7bc799a9dc6f75a32ef07387c9ab2f9731e6f9" {
		t.Fatalf("the verified facts are not the real proof's: %v", d)
	}
}

func TestGovernanceG1ServicePinIsUsedWhenTheRequestHasNone(t *testing.T) {
	fx := loadGovStored(t)
	h := govTestHandler(&govFakeStore{portable: fx.portable()}, liveIncarnation)
	got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G1"}))
	if !got.Resp.Valid || got.Resp.Details["pinned_by"] != "service" {
		t.Fatalf("the service's pin must be the trust base when the request names none: %+v", got.Resp)
	}
}

func TestGovernanceG1WithoutAPinIsANamedStateNeverValid(t *testing.T) {
	fx := loadGovStored(t)
	h := govTestHandler(&govFakeStore{portable: fx.portable()}, "")
	got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G1"}))
	govWantRefused(t, got, GovStateIncarnationNotPinned)
	if got.Resp.Details["incarnation"] != liveIncarnation {
		t.Fatalf("the refusal should hand the caller the incarnation the document names: %v", got.Resp.Details)
	}
}

func TestGovernanceG1WrongIncarnationIsRefused(t *testing.T) {
	fx := loadGovStored(t)
	other := "cac6698ed49a286ad8a3de94540a3354dfe964f366a439f4fdfb34533059fda1" // the real one with its last bit changed
	for name, h := range map[string]*BundleHandlers{
		"pin in the request": govTestHandler(&govFakeStore{portable: fx.portable()}, ""),
		"pin in the service": govTestHandler(&govFakeStore{portable: fx.portable()}, other),
	} {
		body := map[string]string{"proof_id": liveProofID, "governance_level": "G1"}
		if name == "pin in the request" {
			body["pinned_incarnation"] = other
		}
		got := govCall(t, h, http.MethodPost, govMustJSON(t, body))
		if got.Status != 200 || got.Resp.Valid || got.Resp.State != GovStateIncarnationMismatch {
			t.Fatalf("%s: a pin that is not the proven incarnation must be refused: %d %+v", name, got.Status, got.Resp)
		}
	}
}

// govComposedDocument is the real stored proof as a verifier receives it.
func govComposedDocument(t *testing.T, fx govStoredFixture) map[string]json.RawMessage {
	t.Helper()
	raw, err := ComposePortable(fx.portable())
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func govG1Body(t *testing.T, doc interface{}, pin string) string {
	t.Helper()
	return govMustJSON(t, map[string]interface{}{"governance_level": "G1", "pinned_incarnation": pin, "proof_data": map[string]interface{}{"portable": doc}})
}

func TestGovernanceG1CarriedDocumentVerifiesAndEveryTruncationOfTheChainIsRefused(t *testing.T) {
	fx := loadGovStored(t)
	doc := govComposedDocument(t, fx)
	h := govTestHandler(&govFakeStore{}, "")

	if got := govCall(t, h, http.MethodPost, govG1Body(t, doc, liveIncarnation)); !got.Resp.Valid {
		t.Fatalf("the carried real document must verify like the stored one: %+v", got.Resp)
	}

	var majors []json.RawMessage
	if err := json.Unmarshal(doc["majors"], &majors); err != nil {
		t.Fatal(err)
	}
	// the chain of major blocks is cut at several places; none may verify, and none may be answered as a pass
	for _, keep := range []int{len(majors) - 1, len(majors) / 2, 1, 0} {
		cut := map[string]json.RawMessage{}
		for k, v := range doc {
			cut[k] = v
		}
		cut["majors"] = govMustRaw(t, majors[:keep])
		got := govCall(t, h, http.MethodPost, govG1Body(t, cut, liveIncarnation))
		if got.Status != 200 || got.Resp.Valid || (got.Resp.State != GovStateEvidenceInvalid && got.Resp.State != GovStateEvidenceMalformed) {
			t.Fatalf("a chain cut to %d of %d major blocks must be refused: %d %+v", keep, len(majors), got.Status, got.Resp)
		}
		if got.Resp.Reason == "" {
			t.Fatalf("a refusal must say why")
		}
	}
}

func TestGovernanceG1StoredSpineTruncatedIsRefusedByName(t *testing.T) {
	fx := loadGovStored(t)
	p := fx.portable()
	p.Spine = p.Spine[:len(p.Spine)-1] // the stored spine is shorter than the document needs
	h := govTestHandler(&govFakeStore{portable: p}, "")
	got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G1", "pinned_incarnation": liveIncarnation}))
	govWantRefused(t, got, GovStateEvidenceMalformed)
}

func govMustRaw(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGovernanceG1OneByteTamperedDocumentIsRefused(t *testing.T) {
	fx := loadGovStored(t)
	doc := govComposedDocument(t, fx)
	// flip the first hex digit of the first step of the transaction receipt (a leaf-to-anchor step)
	var ev map[string]json.RawMessage
	if err := json.Unmarshal(doc["evidence"], &ev); err != nil {
		t.Fatal(err)
	}
	var rc map[string]json.RawMessage
	if err := json.Unmarshal(ev["receipt"], &rc); err != nil {
		t.Fatal(err)
	}
	var steps []map[string]interface{}
	if err := json.Unmarshal(rc["entries"], &steps); err != nil {
		t.Fatal(err)
	}
	steps[0]["hash"] = govFlipHex(steps[0]["hash"].(string))
	rc["entries"] = govMustRaw(t, steps)
	ev["receipt"] = govMustRaw(t, rc)
	doc["evidence"] = govMustRaw(t, ev)
	got := govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, govG1Body(t, doc, liveIncarnation))
	govWantRefused(t, got, GovStateEvidenceInvalid)
}

func govFlipHex(s string) string {
	b := []byte(s)
	if b[0] == '0' {
		b[0] = '1'
	} else {
		b[0] = '0'
	}
	return string(b)
}

func TestGovernanceG1PresentGarbageIsRefused(t *testing.T) {
	h := govTestHandler(&govFakeStore{}, "")
	for name, doc := range map[string]string{
		"empty object":               `{}`,
		"wrong format":               `{"format":"certen-proof-v2-accumulate-portable/9","pin":"` + liveIncarnation + `","majors":[],"genesis":{},"evidence":{}}`,
		"right format, nothing else": `{"format":"certen-proof-v2-accumulate-portable/1","pin":"` + liveIncarnation + `","majors":[],"genesis":{},"evidence":{}}`,
		"a string":                   `"portable"`,
		"an array":                   `[1,2,3]`,
		"a number":                   `7`,
		"the old accepted shape":     `{"authority_snapshot":{"x":1},"signatures":[{"y":2}]}`,
	} {
		got := govCall(t, h, http.MethodPost, `{"governance_level":"G1","pinned_incarnation":"`+liveIncarnation+`","proof_data":{"portable":`+doc+`}}`)
		if got.Status != 200 || got.Resp.Valid || got.Resp.Reason == "" {
			t.Fatalf("%s: present garbage must be refused with a reason: %d %+v", name, got.Status, got.Resp)
		}
	}
	// the members the OLD handler accepted as "evidence" are not evidence: refused by name, not accepted as non-nil fields
	old := govCall(t, h, http.MethodPost, `{"governance_level":"G1","proof_data":{"authority_snapshot":{"a":1},"signatures":[1]}}`)
	govWantRefused(t, old, GovStateEvidenceAbsent)
	// null is absent, not present
	nul := govCall(t, h, http.MethodPost, `{"governance_level":"G1","proof_data":{"portable":null}}`)
	govWantRefused(t, nul, GovStateEvidenceAbsent)
	// an empty proof_data is present and empty: nothing to verify
	empty := govCall(t, h, http.MethodPost, `{"governance_level":"G0","proof_data":{}}`)
	govWantRefused(t, empty, GovStateEvidenceAbsent)
}

func TestGovernanceG1NoStoredDocumentIsANamedState(t *testing.T) {
	h := govTestHandler(&govFakeStore{portable: nil}, "")
	got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G1", "pinned_incarnation": liveIncarnation}))
	govWantRefused(t, got, GovStateEvidenceAbsent)
}

// ---------------------------------------------------------------------------------------------------------------------
// G0 and G2: receipts; and DAX-F1(a): a stored `verified` flag is never an answer
// ---------------------------------------------------------------------------------------------------------------------

// govLiveReceipt is the REAL transaction receipt of the stored proof (Kermit tx 348e3d94..., 55 steps to its partition anchor).
func govLiveReceipt(t *testing.T) map[string]interface{} {
	t.Helper()
	var ev map[string]json.RawMessage
	if err := json.Unmarshal(govComposedDocument(t, loadGovStored(t))["evidence"], &ev); err != nil {
		t.Fatal(err)
	}
	var rc map[string]interface{}
	if err := json.Unmarshal(ev["receipt"], &rc); err != nil {
		t.Fatal(err)
	}
	return rc
}

func govReceiptRow(level string, receipt interface{}, verifiedFlag bool) database.GovernanceProofLevel {
	lj := map[string]interface{}{"result": map[string]interface{}{"g0_proof_complete": true}, "threshold_m": 2}
	if receipt != nil {
		lj["receipt"] = receipt
	}
	b, _ := json.Marshal(lj)
	return database.GovernanceProofLevel{GovLevel: database.GovernanceLevel(level), LevelName: level, LevelJSON: b, Verified: verifiedFlag}
}

func TestGovernanceReceiptRealReceiptVerifiesInBothSources(t *testing.T) {
	rc := govLiveReceipt(t)
	for _, level := range []string{"G0", "G2"} {
		// carried in the request
		got := govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, govMustJSON(t, map[string]interface{}{"governance_level": level, "proof_data": map[string]interface{}{"receipt": rc}}))
		if !got.Resp.Valid || got.Resp.Details["path_steps"].(float64) != 55 {
			t.Fatalf("%s carried: the real receipt must verify: %+v", level, got.Resp)
		}
		if _, ok := got.Resp.Details["does_not"]; !ok {
			t.Fatalf("a receipt verdict must say what it does not establish")
		}
		// stored: the flag is FALSE and the evidence is real: the answer is still valid (the flag is not read)
		st := &govFakeStore{levels: []database.GovernanceProofLevel{govReceiptRow(level, rc, false)}}
		got = govCall(t, govTestHandler(st, ""), http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": level}))
		if !got.Resp.Valid {
			t.Fatalf("%s stored: real evidence must verify whatever the stored flag says: %+v", level, got.Resp)
		}
	}
}

func TestGovernanceStoredVerifiedFlagIsNeverTheAnswer(t *testing.T) {
	rc := govLiveReceipt(t)
	steps := rc["entries"].([]interface{})
	bad := map[string]interface{}{"start": rc["start"], "anchor": rc["anchor"], "entries": append([]interface{}{map[string]interface{}{"hash": govFlipHex(steps[0].(map[string]interface{})["hash"].(string))}}, steps[1:]...)}

	for name, tc := range map[string]struct {
		row   database.GovernanceProofLevel
		state string
	}{
		"flag true, conclusion without evidence": {govReceiptRow("G0", nil, true), GovStateSummaryOnly},
		"flag true, evidence tampered":           {govReceiptRow("G0", bad, true), GovStateEvidenceInvalid},
		"flag true, evidence is garbage":         {govReceiptRow("G0", map[string]interface{}{"start": "zz", "anchor": "yy"}, true), GovStateEvidenceInvalid},
		"flag true, row is for another level":    {govReceiptRow("G1", rc, true), GovStateLevelNotStored},
	} {
		h := govTestHandler(&govFakeStore{levels: []database.GovernanceProofLevel{tc.row}}, "")
		got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G0"}))
		if got.Status != 200 || got.Resp.Valid || got.Resp.State != tc.state {
			t.Fatalf("%s: the stored flag must not decide: want %s, got %d valid=%v state=%q reason=%q", name, tc.state, got.Status, got.Resp.Valid, got.Resp.State, got.Resp.Reason)
		}
		if got.Resp.Reason == "" {
			t.Fatalf("%s: a refusal must say why", name)
		}
	}
}

func TestGovernanceStoredLevelJSONThatDoesNotParseIsNamedNotValid(t *testing.T) {
	row := database.GovernanceProofLevel{GovLevel: "G0", LevelName: "G0", LevelJSON: json.RawMessage(`{"receipt": 7}`), Verified: true}
	got := govCall(t, govTestHandler(&govFakeStore{levels: []database.GovernanceProofLevel{row}}, ""), http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G0"}))
	govWantRefused(t, got, GovStateEvidenceMalformed)
	none := govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G2"}))
	govWantRefused(t, none, GovStateEvidenceAbsent)
}

func TestGovernanceReceiptPresentGarbageIsRefused(t *testing.T) {
	rc := govLiveReceipt(t)
	h := govTestHandler(&govFakeStore{}, "")
	steps := rc["entries"].([]interface{})
	short := map[string]interface{}{"start": rc["start"], "anchor": rc["anchor"], "entries": steps[:len(steps)-1]} // a truncated path
	for name, receipt := range map[string]interface{}{
		"non-hex":                  map[string]interface{}{"start": "zz", "anchor": "yy", "entries": []interface{}{}},
		"start != anchor, no path": map[string]interface{}{"start": rc["start"], "anchor": rc["anchor"], "entries": []interface{}{}},
		"truncated path":           short,
		"wrong level label":        map[string]interface{}{"level": "G1", "start": rc["start"], "anchor": rc["anchor"], "entries": steps},
		"unknown member":           map[string]interface{}{"start": rc["start"], "anchor": rc["anchor"], "entries": steps, "verified": true},
		"member in another case":   map[string]interface{}{"Start": rc["start"], "anchor": rc["anchor"], "entries": steps},
		"the old shape":            map[string]interface{}{"merkle_proof": map[string]interface{}{"leaf": "x"}},
	} {
		got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]interface{}{"governance_level": "G0", "proof_data": map[string]interface{}{"receipt": receipt}}))
		if got.Status != 200 || got.Resp.Valid || got.Resp.Reason == "" {
			t.Fatalf("%s: must be refused with a reason: %d %+v", name, got.Status, got.Resp)
		}
	}
	// members the old handler accepted are not evidence
	old := govCall(t, h, http.MethodPost, `{"governance_level":"G0","proof_data":{"merkle_proof":{"anything":1}}}`)
	govWantRefused(t, old, GovStateEvidenceAbsent)
	g2 := govCall(t, h, http.MethodPost, `{"governance_level":"G2","proof_data":{"post_state_hash":"00","inclusion_proof":{"x":1}}}`)
	govWantRefused(t, g2, GovStateEvidenceAbsent)
}

// ---------------------------------------------------------------------------------------------------------------------
// every error path ends in valid=false or a 4xx/5xx
// ---------------------------------------------------------------------------------------------------------------------

func TestGovernanceMalformedRequestsAreRefusedBeforeAnyVerification(t *testing.T) {
	h := govTestHandler(&govFakeStore{}, "")
	id := liveProofID
	for name, tc := range map[string]struct {
		method string
		body   string
		status int
	}{
		"GET":                         {http.MethodGet, ``, 405},
		"not json":                    {http.MethodPost, `{`, 400},
		"empty body":                  {http.MethodPost, ``, 400},
		"unknown member":              {http.MethodPost, `{"governance_level":"G0","proof_id":"` + id + `","proof":{}}`, 400},
		"member name in another case": {http.MethodPost, `{"Governance_Level":"G0","proof_id":"` + id + `"}`, 400},
		"proof_id in another case":    {http.MethodPost, `{"governance_level":"G0","PROOF_ID":"` + id + `"}`, 400},
		"data after the value":        {http.MethodPost, `{"governance_level":"G0","proof_id":"` + id + `"} {}`, 400},
		"no level":                    {http.MethodPost, `{"proof_id":"` + id + `"}`, 400},
		"level G3":                    {http.MethodPost, `{"governance_level":"G3","proof_id":"` + id + `"}`, 400},
		"level g1 lowercase":          {http.MethodPost, `{"governance_level":"g1","proof_id":"` + id + `"}`, 400},
		"neither source":              {http.MethodPost, `{"governance_level":"G0"}`, 400},
		"both sources":                {http.MethodPost, `{"governance_level":"G0","proof_id":"` + id + `","proof_data":{"receipt":{}}}`, 400},
		"bad proof id":                {http.MethodPost, `{"governance_level":"G0","proof_id":"not-a-uuid"}`, 400},
		"bad pin (short)":             {http.MethodPost, `{"governance_level":"G1","proof_id":"` + id + `","pinned_incarnation":"abcd"}`, 400},
		"bad pin (not hex)":           {http.MethodPost, `{"governance_level":"G1","proof_id":"` + id + `","pinned_incarnation":"` + strings.Repeat("zz", 32) + `"}`, 400},
		"member of wrong type":        {http.MethodPost, `{"governance_level":"G0","proof_id":7}`, 400},
		"proof_data not object":       {http.MethodPost, `{"governance_level":"G0","proof_data":[1]}`, 400},
	} {
		got := govCall(t, h, tc.method, tc.body)
		if got.Status != tc.status {
			t.Fatalf("%s: want %d, got %d: %.200s", name, tc.status, got.Status, got.Raw)
		}
	}
}

func TestGovernanceStorageFailureIsAServerErrorNotAVerdict(t *testing.T) {
	boom := errors.New("connection reset")
	for _, level := range []string{"G0", "G1", "G2"} {
		st := &govFakeStore{levelsErr: boom, portableErr: boom}
		got := govCall(t, govTestHandler(st, ""), http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": level, "pinned_incarnation": liveIncarnation}))
		if got.Status != http.StatusInternalServerError {
			t.Fatalf("%s: a storage failure is a 500, not a verdict: %d %.200s", level, got.Status, got.Raw)
		}
		if strings.Contains(got.Raw, "connection reset") {
			t.Fatalf("the storage error text must not reach the client")
		}
	}
	// a handler with no store at all fails closed too
	none := govCall(t, &BundleHandlers{logger: log.New(io.Discard, "", 0)}, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G0"}))
	if none.Status != http.StatusInternalServerError {
		t.Fatalf("no store must be a 500: %d", none.Status)
	}
}

func TestGovernanceAnUnusableServicePinIsAServerErrorNeverNoPin(t *testing.T) {
	fx := loadGovStored(t)
	h := govTestHandler(&govFakeStore{portable: fx.portable()}, "not-an-incarnation")
	got := govCall(t, h, http.MethodPost, govMustJSON(t, map[string]string{"proof_id": liveProofID, "governance_level": "G1"}))
	if got.Status != http.StatusInternalServerError {
		t.Fatalf("an unusable configured pin must not be read as 'no pin': %d", got.Status)
	}
	if ValidatePinnedIncarnation("not-an-incarnation") == nil || ValidatePinnedIncarnation("") != nil || ValidatePinnedIncarnation(liveIncarnation) != nil || ValidatePinnedIncarnation("0x"+liveIncarnation) != nil {
		t.Fatalf("ValidatePinnedIncarnation does not match parsePinnedIncarnation")
	}
}

func TestGovernanceOversizedRequestIsRefused(t *testing.T) {
	was := governanceVerifyBodyLimit
	governanceVerifyBodyLimit = 512
	defer func() { governanceVerifyBodyLimit = was }()
	body := `{"governance_level":"G0","proof_data":{"receipt":{"start":"` + strings.Repeat("a", 2048) + `"}}}`
	got := govCall(t, govTestHandler(&govFakeStore{}, ""), http.MethodPost, body)
	if got.Status != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", got.Status)
	}
}

// ---------------------------------------------------------------------------------------------------------------------
// the validator's own vectors give the same verdicts (DAX-F2)
// ---------------------------------------------------------------------------------------------------------------------

type govVectorManifest struct {
	ManifestSHA256 string `json:"manifest_sha256"`
	Cases          []struct {
		Name   string `json:"name"`
		Expect string `json:"expect"`
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
		State  string `json:"state"`
		Prefix string `json:"prefix"`
	} `json:"cases"`
}

func loadGovVectors(t *testing.T) govVectorManifest {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "governance", "vectors", "manifest_excerpt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m govVectorManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Cases) < 3 {
		t.Fatalf("the vector excerpt lost cases")
	}
	return m
}

func TestGovernanceVectorFilesAreTheValidatorsBytes(t *testing.T) {
	m := loadGovVectors(t)
	for _, c := range m.Cases {
		b, err := os.ReadFile(filepath.Join("testdata", "governance", "vectors", c.File))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != c.SHA256 {
			t.Fatalf("%s: sha256 %x is not the validator's recorded %s (a copy that drifted, or a checkout that converted line endings)", c.File, sum, c.SHA256)
		}
	}
}

func TestGovernanceValidatorVectorsGiveTheValidatorsVerdicts(t *testing.T) {
	m := loadGovVectors(t)
	h := govTestHandler(&govFakeStore{}, "")
	unprovenNamed := 0
	for _, c := range m.Cases {
		b, err := os.ReadFile(filepath.Join("testdata", "governance", "vectors", c.File))
		if err != nil {
			t.Fatal(err)
		}
		// the vector worlds are synthetic Accumulate networks: the pin is the world's own incarnation, named by the document
		var head struct {
			Pin string `json:"pin"`
		}
		if err := json.Unmarshal(b, &head); err != nil || head.Pin == "" {
			t.Fatalf("%s has no pin", c.File)
		}
		got := govCall(t, h, http.MethodPost, `{"governance_level":"G1","pinned_incarnation":"`+head.Pin+`","proof_data":{"portable":`+string(b)+`}}`)
		switch c.Expect {
		case "verified":
			// the validator's verifier returned no error. The endpoint is stricter than "no error" in one way the validator documents (a
			// report with Unproven entries is not a verified S3): a synthetic world that leaves facts unproven is evidence_unproven, named,
			// never valid. Anything the verifier REFUSES is evidence_invalid, so "verified" here must never be that.
			if got.Status != 200 || got.Resp.State == GovStateEvidenceInvalid || got.Resp.State == GovStateEvidenceMalformed {
				t.Fatalf("%s: the validator verifies this vector, so the endpoint must not refuse it: %d %+v", c.Name, got.Status, got.Resp)
			}
			if got.Resp.Valid != (got.Resp.State == GovStateVerified) || got.Resp.Details["incarnation"] != head.Pin {
				t.Fatalf("%s: verdict does not describe the verified evidence: %+v", c.Name, got.Resp)
			}
			if got.Resp.State == GovStateEvidenceUnproven {
				if u, _ := got.Resp.Details["unproven"].([]interface{}); len(u) == 0 {
					t.Fatalf("%s: evidence_unproven must name what is unproven: %+v", c.Name, got.Resp)
				}
				unprovenNamed++
			}
		case "refused":
			if got.Status != 200 || got.Resp.Valid || got.Resp.State != GovStateEvidenceInvalid {
				t.Fatalf("%s: the validator refuses this vector, so must the endpoint: %d %+v", c.Name, got.Status, got.Resp)
			}
			if c.Prefix == "" || !strings.Contains(got.Resp.Reason, c.Prefix) {
				t.Fatalf("%s: the refusal must carry the validator's own verdict text %q: %q", c.Name, c.Prefix, got.Resp.Reason)
			}
		default:
			t.Fatalf("%s: unknown expectation %q", c.Name, c.Expect)
		}
	}
	if unprovenNamed == 0 {
		t.Fatalf("the synthetic verified vector is expected to carry a named unproven fact; the strictness check was not exercised")
	}
}

// ---------------------------------------------------------------------------------------------------------------------
// the compiled dependency IS the recorded validator code (DAX-F2)
// ---------------------------------------------------------------------------------------------------------------------

type provenanceModule struct {
	ModulePath string `json:"module_path"`
	ReplacedBy string `json:"replaced_by"`
	Version    string `json:"version"`
	Files      []struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

type verifierProvenance struct {
	SourceRepo string             `json:"source_repo"`
	Commit     string             `json:"commit"`
	Modules    []provenanceModule `json:"modules"`
}

// TestGovernanceVerifierIsTheRecordedValidatorCode fails when go.mod is moved to another validator commit (or a source file the
// endpoint depends on changes) without docs/verify-governance-provenance.json being updated by a person who has read the change.
func TestGovernanceVerifierIsTheRecordedValidatorCode(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "verify-governance-provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rec verifierProvenance
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatal(err)
	}
	if len(rec.Commit) != 40 || len(rec.Modules) != 2 {
		t.Fatalf("the provenance record is incomplete")
	}
	files := 0
	for _, m := range rec.Modules {
		cmd := exec.Command("go", "list", "-m", "-json", m.ModulePath)
		cmd.Dir = filepath.Join("..", "..")
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list -m %s: %v", m.ModulePath, err)
		}
		var mod struct {
			Path    string
			Version string
			Replace *struct {
				Path    string
				Version string
				Dir     string
			}
		}
		if err := json.Unmarshal(out, &mod); err != nil {
			t.Fatal(err)
		}
		if mod.Replace == nil || mod.Replace.Path != m.ReplacedBy || mod.Replace.Version != m.Version {
			t.Fatalf("go.mod resolves %s to %+v, the record says %s %s", m.ModulePath, mod.Replace, m.ReplacedBy, m.Version)
		}
		if !strings.HasSuffix(m.Version, "-"+rec.Commit[:12]) {
			t.Fatalf("the recorded version %s is not the recorded commit %s", m.Version, rec.Commit)
		}
		if mod.Replace.Dir == "" {
			t.Fatalf("%s is not in the module cache (run go mod download)", m.ModulePath)
		}
		for _, f := range m.Files {
			body, err := os.ReadFile(filepath.Join(mod.Replace.Dir, filepath.FromSlash(f.Path)))
			if err != nil {
				t.Fatalf("%s: %v", f.Path, err)
			}
			sum := sha256.Sum256(body)
			if hex.EncodeToString(sum[:]) != f.SHA256 {
				t.Fatalf("%s is not the recorded validator file (record %s, compiled %x): update docs/verify-governance-provenance.json deliberately", f.Path, f.SHA256, sum)
			}
			files++
		}
	}
	if files < 7 {
		t.Fatalf("the provenance record names %d files, fewer than the verifier code the endpoint runs", files)
	}
	// the handler imports exactly the symbols the record says it runs
	src, err := os.ReadFile("governance_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range []string{"VerifyMerkle()", "proofv2.VerifyPortable(", "proof.GovernanceLevelsFromStorage("} {
		if !bytes.Contains(src, []byte(sym)) {
			t.Fatalf("governance_verify.go no longer calls %s", sym)
		}
	}
}
