package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/certen/proofs-service/pkg/database"
)

// A bundle lookup that failed is a server error, never "no bundle found": a 404 tells a verifier the
// bundle does not exist when the database simply did not answer (RB3-F122).
func TestAFailedBundleLookupIsNotReportedAsNotFound(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://nobody@127.0.0.1:1/none?sslmode=disable&connect_timeout=2")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	h := NewBundleHandlers(database.NewRepositories(database.NewClientFromDB(db)), nil, nil)
	for _, path := range []string{
		"/api/v1/proofs/" + uuid.NewString() + "/bundle/verify",
		"/api/v1/proofs/" + uuid.NewString() + "/bundle",
	} {
		rr := httptest.NewRecorder()
		if path[len(path)-6:] == "verify" {
			h.HandleVerifyBundle(rr, httptest.NewRequest(http.MethodGet, path, nil))
		} else {
			h.HandleDownloadBundle(rr, httptest.NewRequest(http.MethodGet, path, nil))
		}
		if rr.Code != http.StatusInternalServerError {
			t.Errorf("%s: status %d on a failed lookup, want 500", path, rr.Code)
		}
	}
}
