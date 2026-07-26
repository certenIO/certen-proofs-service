package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The bug this guards against: an Accumulate tx hash in the path decodes to a
// double slash, ServeMux answers 301 to the cleaned path, and the caller's
// one-time service token gets replayed into the redirect — a permanent 401 and
// a stranded intent. The middleware must make the redirect impossible while
// still handing the handler the true, uncleaned path.

func TestCleanPathLikeServeMux(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/api/v1/proofs/tx/acc://hash@adi.acme/data", "/api/v1/proofs/tx/acc:/hash@adi.acme/data"},
		{"/api/v1/proofs/", "/api/v1/proofs/"},       // trailing slash preserved for prefix routes
		{"/api/v1//proofs", "/api/v1/proofs"},        // interior double slash collapsed
		{"/api/v1/proofs/../x", "/api/v1/x"},         // dot segments resolved
		{"", "/"},                                    // empty becomes root
		{"api/v1/proofs", "/api/v1/proofs"},          // leading slash added
	}
	for _, c := range cases {
		if got := cleanPathLikeServeMux(c.in); got != c.want {
			t.Errorf("cleanPathLikeServeMux(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPreserveRawPathGivesHandlerTheUncleanedPath(t *testing.T) {
	const txPath = "/api/v1/proofs/tx/acc://abc123@carp-seller.acme/data"

	var seenRaw, seenURL string
	h := PreserveRawPathMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRaw = RawPath(r)
		seenURL = r.URL.Path
	}))

	req := httptest.NewRequest(http.MethodGet, "http://x"+txPath, nil)
	h.ServeHTTP(httptest.NewRecorder(), req)

	// The handler must be able to recover the real acc:// hash...
	if seenRaw != txPath {
		t.Errorf("RawPath = %q, want the original %q", seenRaw, txPath)
	}
	// ...while the URL handed to ServeMux is already clean, so ServeMux finds
	// nothing to redirect to.
	want := "/api/v1/proofs/tx/acc:/abc123@carp-seller.acme/data"
	if seenURL != want {
		t.Errorf("r.URL.Path = %q, want cleaned %q", seenURL, want)
	}
}

func TestPreserveRawPathLeavesOrdinaryPathsAlone(t *testing.T) {
	const p = "/api/v1/proofs/9f3c1e2a-0000-4000-8000-000000000000"

	var seenRaw, seenURL string
	h := PreserveRawPathMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRaw = RawPath(r)
		seenURL = r.URL.Path
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://x"+p, nil))

	if seenRaw != p || seenURL != p {
		t.Errorf("clean path was altered: raw=%q url=%q, want both %q", seenRaw, seenURL, p)
	}
}

// A ServeMux behind the middleware must serve the request directly rather than
// answering 301 — this is the actual regression that broke the proof pipeline.
func TestNoRedirectThroughServeMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/proofs/tx/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(RawPath(r)))
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://x/api/v1/proofs/tx/acc://abc@adi.acme/data", nil)
	PreserveRawPathMiddleware(mux).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a 301 here is the bug: it replays the caller's one-time token)", rec.Code)
	}
	if got := rec.Body.String(); got != "/api/v1/proofs/tx/acc://abc@adi.acme/data" {
		t.Errorf("handler saw %q, want the uncleaned path", got)
	}
}
