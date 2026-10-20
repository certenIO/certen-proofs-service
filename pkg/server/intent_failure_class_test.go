package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// RB4-F13: a failed intent is served with why it failed (failure_class, validator migration 00014), and the
// service requires the catalog its SQL reads - it required 00005 while reading tables up to 00013.
func TestAFailedIntentIsServedWithItsFailureClass(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	id := "f13-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO intent_lifecycle (intent_id, accum_tx_hash, status, error_message, failure_class, failed_at)
		VALUES ($1, $2, 'failed', 'G1 governance proof incomplete', 'governance_unsatisfied', now())`, id, uuid.NewString()[:16]); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	NewIntentLifecycleHandlers(repos, log.New(io.Discard, "", 0)).HandleGetByIntentID(rr, httptest.NewRequest(http.MethodGet, "/api/v1/intent/"+id, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Status       string  `json:"status"`
		FailureClass *string `json:"failure_class"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.FailureClass == nil || *got.FailureClass != "governance_unsatisfied" {
		t.Fatalf("served %s / %v: %s", got.Status, got.FailureClass, rr.Body.String())
	}
	// What this test protects: the service's SQL reads intent_lifecycle.failure_class (migration 00014), so 00014 must be among the
	// migrations the startup check requires, with the file's checksum. (It used to assert that 00014 was the NEWEST required
	// migration, which broke the day a newer one was required.)
	var found *database.SchemaVersion
	for i := range database.RequiredSchema {
		if database.RequiredSchema[i].Version == "00014" {
			found = &database.RequiredSchema[i]
		}
	}
	if found == nil {
		t.Fatalf("the service reads intent_lifecycle.failure_class (00014) but does not require migration 00014")
	}
	if found.SHA256 != "490c05b0486492537eb72308599cf390cca561c0a65da4924625fdf292058e2f" {
		t.Fatalf("migration 00014 is required with checksum %s", found.SHA256)
	}
}
