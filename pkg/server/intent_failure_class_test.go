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
	last := database.RequiredSchema[len(database.RequiredSchema)-1]
	if last.Version != "00014" {
		t.Fatalf("the service requires the shared catalog through %s; its SQL reads intent_lifecycle.failure_class (00014)", last.Version)
	}
}
