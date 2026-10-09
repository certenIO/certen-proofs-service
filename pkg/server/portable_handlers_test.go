package server

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

type fakePortable struct {
	p   *database.PortableV2
	err error
	got uuid.UUID
}

func (f *fakePortable) GetPortableV2(_ context.Context, id uuid.UUID) (*database.PortableV2, error) {
	f.got = id
	return f.p, f.err
}

func stored() *database.PortableV2 {
	return &database.PortableV2{
		Document: `{"format":"certen-proof-v2-accumulate-portable/1","pin":"aa","genesis":{"x":1},"majors":[],"evidence":{"version":"2.0"}}`,
		Majors:   2,
		Spine:    []string{`{"index":1}`, `{"index":2}`},
	}
}

func get(h *PortableHandlers, id string, hdr map[string]string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proofs/"+id+"/v2", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	h.HandleGetProofV2(rr, req)
	return rr
}

func TestComposePortableFillsTheSpineAndKeepsEverythingElse(t *testing.T) {
	p := stored()
	p.GovRootInputs = `{"g0Hash":"00"}`
	out, err := ComposePortable(p)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["majors"]) != `[{"index":1},{"index":2}]` {
		t.Fatalf("majors %s", doc["majors"])
	}
	if string(doc["govRootV3Inputs"]) != `{"g0Hash":"00"}` {
		t.Fatalf("inputs %s", doc["govRootV3Inputs"])
	}
	for k, want := range map[string]string{"format": `"certen-proof-v2-accumulate-portable/1"`, "pin": `"aa"`, "genesis": `{"x":1}`, "evidence": `{"version":"2.0"}`} {
		if string(doc[k]) != want {
			t.Fatalf("%s changed: %s", k, doc[k])
		}
	}
	// no inputs recorded: the member is absent, not null
	p.GovRootInputs = ""
	out, _ = ComposePortable(p)
	if strings.Contains(string(out), "govRootV3Inputs") {
		t.Fatalf("a document with no recorded inputs carries the member: %s", out)
	}
}

func TestComposePortableRefusesWhatCannotBeAVerifiableDocument(t *testing.T) {
	for name, mutate := range map[string]func(*database.PortableV2){
		"a short spine":            func(p *database.PortableV2) { p.Spine = p.Spine[:1] },
		"a spine record":           func(p *database.PortableV2) { p.Spine[1] = `{not json` },
		"a document":               func(p *database.PortableV2) { p.Document = `[` },
		"inputs that are not JSON": func(p *database.PortableV2) { p.GovRootInputs = `{` },
	} {
		p := stored()
		mutate(p)
		if _, err := ComposePortable(p); err == nil {
			t.Errorf("%s was composed", name)
		}
	}
}

func TestHandlerServesTheDocumentWithAnETag(t *testing.T) {
	f := &fakePortable{p: stored()}
	h := NewPortableHandlers(f, nil)
	id := uuid.NewString()
	rr := get(h, id, nil)
	if rr.Code != http.StatusOK || f.got.String() != id {
		t.Fatalf("status %d, looked up %s", rr.Code, f.got)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type %q", ct)
	}
	etag := rr.Header().Get("ETag")
	if etag == "" || !strings.Contains(rr.Header().Get("Cache-Control"), "private") {
		t.Fatalf("etag %q cache %q", etag, rr.Header().Get("Cache-Control"))
	}
	if !strings.Contains(rr.Body.String(), `"majors":[{"index":1},{"index":2}]`) {
		t.Fatalf("body %s", rr.Body.String())
	}
	if again := get(h, id, map[string]string{"If-None-Match": etag}); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
		t.Fatalf("a repeat with the ETag: %d with %d bytes", again.Code, again.Body.Len())
	}
}

func TestHandlerGzipsWhenAsked(t *testing.T) {
	h := NewPortableHandlers(&fakePortable{p: stored()}, nil)
	rr := get(h, uuid.NewString(), map[string]string{"Accept-Encoding": "gzip"})
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("not compressed: %v", rr.Header())
	}
	zr, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(zr)
	if !json.Valid(b) || !strings.Contains(string(b), `"index":2`) {
		t.Fatalf("decompressed %s", b)
	}
}

func TestNoDocumentIsNotAvailableAndAFailureIsNot(t *testing.T) {
	missing := get(NewPortableHandlers(&fakePortable{}, nil), uuid.NewString(), nil)
	if missing.Code != http.StatusNotFound || !strings.Contains(missing.Body.String(), "PROOF_V2_NOT_AVAILABLE") {
		t.Fatalf("no document: %d %s", missing.Code, missing.Body)
	}
	broken := get(NewPortableHandlers(&fakePortable{err: errors.New("db down")}, nil), uuid.NewString(), nil)
	if broken.Code != http.StatusInternalServerError || strings.Contains(broken.Body.String(), "NOT_AVAILABLE") {
		t.Fatalf("a failed lookup was reported as %d %s", broken.Code, broken.Body)
	}
	bad := stored()
	bad.Spine = bad.Spine[:1]
	if rr := get(NewPortableHandlers(&fakePortable{p: bad}, nil), uuid.NewString(), nil); rr.Code != http.StatusInternalServerError {
		t.Fatalf("a spine that does not compose: %d", rr.Code)
	}
}

func TestHandlerRefusesWhatIsNotAProofID(t *testing.T) {
	h := NewPortableHandlers(&fakePortable{p: stored()}, nil)
	if rr := get(h, "not-a-uuid", nil); rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d", rr.Code)
	}
	rr := httptest.NewRecorder()
	h.HandleGetProofV2(rr, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/"+uuid.NewString()+"/v2", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rr.Code)
	}
}
