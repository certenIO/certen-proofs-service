package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/lib/pq"

	"github.com/certen/proofs-service/pkg/database"
)

func requestProofWith(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	db, err := sql.Open("postgres", "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewBundleHandlers(database.NewRepositories(database.NewClientFromDB(db)), nil, nil)
	rr := httptest.NewRecorder()
	h.HandleRequestProof(rr, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/request", strings.NewReader(body)))
	return rr
}

// A callback_url is refused by name: no validator delivers it any more, and accepting it would store a destination
// nothing calls (RB7 T5-4).
func TestACallbackURLIsRefusedByName(t *testing.T) {
	rr := requestProofWith(t, `{"account_url":"acc://a.acme","proof_class":"on_demand","callback_url":"http://169.254.169.254/x"}`)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "CALLBACK_NOT_SUPPORTED") {
		t.Fatalf("status %d body %s, want 400 CALLBACK_NOT_SUPPORTED", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "/api/v1/proofs/requests/completed") {
		t.Fatalf("the refusal does not point to the completion feed: %s", rr.Body.String())
	}
}

// A request without the field is handled exactly as before: it reaches the store, which here is unreachable, so the
// answer is the 500 of a failed insert and never the refusal.
func TestARequestWithoutACallbackStillReachesTheStore(t *testing.T) {
	for _, body := range []string{
		`{"account_url":"acc://a.acme","proof_class":"on_demand"}`,
		`{"account_url":"acc://a.acme","proof_class":"on_demand","callback_url":""}`,
	} {
		rr := requestProofWith(t, body)
		if rr.Code != http.StatusInternalServerError || strings.Contains(rr.Body.String(), "CALLBACK_NOT_SUPPORTED") {
			t.Fatalf("%s: status %d body %s, want the store's 500", body, rr.Code, rr.Body.String())
		}
	}
}
