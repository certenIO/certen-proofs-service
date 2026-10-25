package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// The shared fixture: the member of golden G-S8-1 of the validator (a fully Stated container, 379 bytes, digest under the leg's D0). The
// validator's proofverify parser (parseCIMember) is run on THIS file by the cross-check recorded in checkins/RB12-EPS-S8-6.md, and the
// api-gateway's tests read a byte-identical copy. The producer must emit exactly these bytes.
const fixtureName = "testdata/contractidentity/member.G-S8-1.json"

const proofV = "01234567-89ab-4def-8012-3456789abcde"

func fixtureMember(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(fixtureName)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(b)
}

func mustHexBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.TrimPrefix(s, "0x"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureRow(t *testing.T) *database.ContractIdentityRow {
	t.Helper()
	var m struct {
		Leg      struct{ BlockHash string } `json:"leg"`
		Evidence string                     `json:"evidence"`
		Digest   string                     `json:"digest"`
	}
	if err := json.Unmarshal(fixtureMember(t), &m); err != nil {
		t.Fatal(err)
	}
	ev := mustHexBytes(t, m.Evidence)
	sum := sha256.Sum256(ev)
	return &database.ContractIdentityRow{ProofID: uuid.MustParse(proofV), Seq: 1, Chain: 11155111, BlockNumber: 11845679,
		BlockHash: mustHexBytes(t, m.Leg.BlockHash), Evidence: ev, EvidenceSHA256: sum[:], Digest: mustHexBytes(t, m.Digest)}
}

func TestComposeContractIdentityEmitsExactlyTheSharedFixtureBytes(t *testing.T) {
	row := fixtureRow(t)
	if got := hex.EncodeToString(row.EvidenceSHA256); got != "d009d115af904ed12afed394efb75040986863137151d6d1ef05494d36793937" || len(row.Evidence) != 379 {
		t.Fatalf("the fixture is not golden G-S8-1: sha256 %s, %d bytes", got, len(row.Evidence))
	}
	out, err := ComposeContractIdentity(row)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, fixtureMember(t)) {
		t.Fatalf("the producer's member differs from the shared fixture:\n got %.200s\nwant %.200s", out, fixtureMember(t))
	}
}

// The bound is pinned to the number the validator, the migration and the api-gateway pin in their own tests (drift = a failing test there).
func TestMaxContractIdentityBytesIsPinned(t *testing.T) {
	if MaxContractIdentityBytes != 4194304 || database.MaxContractIdentityBytes != 4194304 {
		t.Fatalf("MaxContractIdentityBytes %d / %d", MaxContractIdentityBytes, database.MaxContractIdentityBytes)
	}
}

func TestComposeContractIdentityRefusesARowThatContradictsItselfAndPinsTheSizeExactly(t *testing.T) {
	mutate := map[string]func(r *database.ContractIdentityRow){
		"chain 0":          func(r *database.ContractIdentityRow) { r.Chain = 0 },
		"negative block":   func(r *database.ContractIdentityRow) { r.BlockNumber = -1 },
		"short block hash": func(r *database.ContractIdentityRow) { r.BlockHash = r.BlockHash[:31] },
		"short digest":     func(r *database.ContractIdentityRow) { r.Digest = r.Digest[:31] },
		"short sha":        func(r *database.ContractIdentityRow) { r.EvidenceSHA256 = r.EvidenceSHA256[:31] },
		"empty evidence":   func(r *database.ContractIdentityRow) { r.Evidence = nil },
		"wrong sha":        func(r *database.ContractIdentityRow) { r.EvidenceSHA256 = bytes.Repeat([]byte{1}, 32) },
		"evidence flipped": func(r *database.ContractIdentityRow) { r.Evidence = append([]byte{0}, r.Evidence[1:]...) },
	}
	for name, f := range mutate {
		row := fixtureRow(t)
		f(row)
		if _, err := ComposeContractIdentity(row); err == nil || !strings.Contains(err.Error(), ContractIdentityRowInvalid) {
			t.Fatalf("%s must be %s, got %v", name, ContractIdentityRowInvalid, err)
		}
	}
	if _, err := ComposeContractIdentity(nil); err == nil {
		t.Fatal("a nil row is refused")
	}
	for n, wantOK := range map[int64]bool{MaxContractIdentityBytes: true, MaxContractIdentityBytes + 1: false} {
		row := fixtureRow(t)
		row.Evidence = bytes.Repeat([]byte{7}, int(n))
		sum := sha256.Sum256(row.Evidence)
		row.EvidenceSHA256 = sum[:]
		_, err := ComposeContractIdentity(row)
		if (err == nil) != wantOK {
			t.Fatalf("size %d: ok=%v err=%v", n, wantOK, err)
		}
	}
}

type fakeIdentity struct {
	row *database.ContractIdentityRow
	err error
}

func (f *fakeIdentity) GetContractIdentity(context.Context, uuid.UUID) (*database.ContractIdentityRow, error) {
	return f.row, f.err
}

func TestPortableDocumentCarriesTheContractIdentityMemberOnlyWhenARowExistsAndTheTagFollowsTheRow(t *testing.T) {
	members := func(rr interface{ Bytes() []byte }) map[string]json.RawMessage {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(rr.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	// absent: the document and its tag are exactly what they were before this member existed (WithContractIdentity not even set)
	plain := get(NewPortableHandlers(&fakePortable{p: stored()}, nil), proofV, nil)
	none := get(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{}), proofV, nil)
	if none.Code != http.StatusOK || plain.Header().Get("ETag") != none.Header().Get("ETag") || !bytes.Equal(plain.Body.Bytes(), none.Body.Bytes()) {
		t.Fatalf("no row: the document and its tag must be unchanged: %d %q %q", none.Code, plain.Header().Get("ETag"), none.Header().Get("ETag"))
	}
	if _, ok := members(none.Body)["contractIdentity"]; ok {
		t.Fatal("the member is absent, not empty, when the proof has none")
	}
	// present
	row := fixtureRow(t)
	with := get(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{row: row}), proofV, nil)
	if with.Code != http.StatusOK || !bytes.Equal(members(with.Body)["contractIdentity"], fixtureMember(t)) {
		t.Fatalf("the served member must be the shared fixture bytes: %d", with.Code)
	}
	tag := with.Header().Get("ETag")
	if tag == "" || tag == plain.Header().Get("ETag") {
		t.Fatalf("the tag must include the row: %q vs %q", tag, plain.Header().Get("ETag"))
	}
	// a better capture is a new row (new digest, new container): a new tag; an unchanged row keeps its tag
	again := get(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{row: fixtureRow(t)}), proofV, nil)
	if again.Header().Get("ETag") != tag {
		t.Fatal("an unchanged row must keep its tag")
	}
	better := fixtureRow(t)
	better.Seq, better.Digest = 2, bytes.Repeat([]byte{9}, 32)
	if newer := get(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{row: better}), proofV, nil); newer.Header().Get("ETag") == tag {
		t.Fatal("a new row must change the tag")
	}
	// If-None-Match
	if nm := get(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{row: row}), proofV, map[string]string{"If-None-Match": tag}); nm.Code != http.StatusNotModified {
		t.Fatalf("a matching If-None-Match is 304, got %d", nm.Code)
	}
}

func TestPortableDocumentRefusesAStoredContractIdentityMemberAndAStoreFailureIsNamed(t *testing.T) {
	for _, key := range []string{"contractIdentity", "contractidentity", "CONTRACTIDENTITY"} {
		p := stored()
		p.Document = strings.Replace(p.Document, `"majors":[]`, `"majors":[],"`+key+`":{"format":"x"}`, 1)
		rr := get(NewPortableHandlers(&fakePortable{p: p}, nil).WithContractIdentity(&fakeIdentity{}), proofV, nil)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), ContractIdentityMemberInDocument) {
			t.Fatalf("a stored %q member must be refused by name: %d %s", key, rr.Code, rr.Body.String())
		}
	}
	for _, err := range []error{database.ErrContractIdentityTooLarge, context.DeadlineExceeded} {
		rr := get(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{err: err}), proofV, nil)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "CONTRACT_IDENTITY_UNUSABLE") || strings.Contains(rr.Body.String(), "majors") {
			t.Fatalf("a failing store is a named 500 and never a document without the member: %v %d %s", err, rr.Code, rr.Body.String())
		}
	}
	bad := fixtureRow(t)
	bad.EvidenceSHA256 = bytes.Repeat([]byte{1}, 32)
	if rr := get(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{row: bad}), proofV, nil); rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), ContractIdentityRowInvalid) {
		t.Fatalf("a row that contradicts itself is %s: %d %s", ContractIdentityRowInvalid, rr.Code, rr.Body.String())
	}
}

// GET /api/v1/proofs/{id}/contract-identity: the bounded member-only endpoint the api-gateway reads (the v2 document is streamed and never parsed
// there). The body is the member exactly as the document carries it; 404 names the absence; the tag is the sha256 of the member, so it includes the row.
func getIdentity(h *PortableHandlers, id string, hdr map[string]string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proofs/"+id+"/contract-identity", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	h.HandleGetContractIdentity(rr, req)
	return rr
}

func TestContractIdentityEndpointServesTheMemberWithATagThatFollowsTheRow(t *testing.T) {
	h := NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{row: fixtureRow(t)})
	rr := getIdentity(h, proofV, nil)
	if rr.Code != http.StatusOK || !bytes.Equal(rr.Body.Bytes(), fixtureMember(t)) {
		t.Fatalf("the body must be the shared fixture member: %d", rr.Code)
	}
	sum := sha256.Sum256(fixtureMember(t))
	tag := rr.Header().Get("ETag")
	if tag != `"`+hex.EncodeToString(sum[:])+`"` || rr.Header().Get("Content-Type") != "application/json" || rr.Header().Get("Content-Length") == "" {
		t.Fatalf("tag %q type %q length %q", tag, rr.Header().Get("Content-Type"), rr.Header().Get("Content-Length"))
	}
	if nm := getIdentity(h, proofV, map[string]string{"If-None-Match": tag}); nm.Code != http.StatusNotModified || nm.Body.Len() != 0 {
		t.Fatalf("a matching If-None-Match is an empty 304, got %d", nm.Code)
	}
	better := fixtureRow(t)
	better.Seq, better.Digest = 2, bytes.Repeat([]byte{9}, 32)
	if newer := getIdentity(NewPortableHandlers(&fakePortable{p: stored()}, nil).WithContractIdentity(&fakeIdentity{row: better}), proofV, map[string]string{"If-None-Match": tag}); newer.Code != http.StatusOK || newer.Header().Get("ETag") == tag {
		t.Fatal("a new row has a new tag and is served in full")
	}
}

func TestContractIdentityEndpointNamesAbsenceRefusalsAndFailures(t *testing.T) {
	cases := []struct {
		name string
		h    *PortableHandlers
		id   string
		want int
		code string
	}{
		{"no row", NewPortableHandlers(&fakePortable{}, nil).WithContractIdentity(&fakeIdentity{}), proofV, http.StatusNotFound, "CONTRACT_IDENTITY_NOT_AVAILABLE"},
		{"not wired", NewPortableHandlers(&fakePortable{}, nil), proofV, http.StatusNotFound, "CONTRACT_IDENTITY_NOT_AVAILABLE"},
		{"bad id", NewPortableHandlers(&fakePortable{}, nil).WithContractIdentity(&fakeIdentity{}), "not-a-uuid", http.StatusBadRequest, "INVALID_PROOF_ID"},
		{"store error", NewPortableHandlers(&fakePortable{}, nil).WithContractIdentity(&fakeIdentity{err: context.DeadlineExceeded}), proofV, http.StatusInternalServerError, "CONTRACT_IDENTITY_UNUSABLE"},
		{"too large", NewPortableHandlers(&fakePortable{}, nil).WithContractIdentity(&fakeIdentity{err: database.ErrContractIdentityTooLarge}), proofV, http.StatusInternalServerError, "CONTRACT_IDENTITY_UNUSABLE"},
	}
	for _, c := range cases {
		rr := getIdentity(c.h, c.id, nil)
		if rr.Code != c.want || !strings.Contains(rr.Body.String(), c.code) {
			t.Fatalf("%s: %d %s", c.name, rr.Code, rr.Body.String())
		}
	}
	bad := fixtureRow(t)
	bad.EvidenceSHA256 = bytes.Repeat([]byte{1}, 32)
	if rr := getIdentity(NewPortableHandlers(&fakePortable{}, nil).WithContractIdentity(&fakeIdentity{row: bad}), proofV, nil); rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), ContractIdentityRowInvalid) {
		t.Fatalf("a self-contradicting row: %d %s", rr.Code, rr.Body.String())
	}
	post := httptest.NewRecorder()
	NewPortableHandlers(&fakePortable{}, nil).HandleGetContractIdentity(post, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/"+proofV+"/contract-identity", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", post.Code)
	}
}
