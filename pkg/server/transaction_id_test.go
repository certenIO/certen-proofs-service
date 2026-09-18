package server

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// Clients name a transaction by its Accumulate transaction ID (acc://<hash>@<principal>); proofs are
// stored under the bare hash. A proof request for a transaction that already has a proof is answered at
// once, and the proof is found by transaction ID, instead of queueing a request nothing would match.
func TestEndpointsAcceptTheAccumulateTransactionID(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	hash := strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	other := strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")
	txID := "acc://" + strings.ToUpper(hash) + "@txid-api.acme/data"

	artifact, err := repos.ProofArtifacts.CreateProofArtifact(ctx, &database.NewProofArtifact{
		ProofType: database.ProofTypeCertenAnchor, AccumTxHash: hash, AccountURL: "acc://txid-api.acme/data",
		ProofClass: database.ProofClassOnDemand, ValidatorID: "api-test", ArtifactJSON: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = db.ExecContext(bg, `DELETE FROM proof_requests WHERE accum_tx_hash LIKE '%' || $1 || '%' OR accum_tx_hash LIKE '%' || $2 || '%'`, hash, other)
		_, _ = db.ExecContext(bg, `DELETE FROM proof_artifacts WHERE proof_id = $1`, artifact.ProofID)
	})

	bundles := NewBundleHandlers(repos, nil, log.New(io.Discard, "", 0))
	post := func(accumTx string) (int, ProofRequestResponse) {
		t.Helper()
		body := `{"accum_tx_hash":"` + accumTx + `","proof_class":"on_demand"}`
		rec := httptest.NewRecorder()
		bundles.HandleRequestProof(rec, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/request", strings.NewReader(body)))
		var response ProofRequestResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &response)
		return rec.Code, response
	}

	code, response := post(txID)
	if code != http.StatusOK || response.Status != "completed" || response.ProofID == nil || *response.ProofID != artifact.ProofID {
		t.Fatalf("a request naming a proven transaction by its ID was not answered with its proof: %d %+v", code, response)
	}
	var queued int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM proof_requests WHERE accum_tx_hash LIKE '%' || $1 || '%'`, hash).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("a request for a proven transaction was queued anyway: %d, %v", queued, err)
	}

	// A transaction without a proof is queued, as the client named it.
	code, response = post("acc://" + other + "@txid-api.acme/data")
	if code != http.StatusAccepted || response.Status != "pending" {
		t.Fatalf("a request for an unproven transaction was not queued: %d %+v", code, response)
	}
	stored, err := repos.Requests.GetRequestByAccumTxHash(ctx, other)
	if err != nil || stored == nil || stored.RequestID != response.RequestID || stored.AccumTxHash.String != "acc://"+other+"@txid-api.acme/data" {
		t.Fatalf("the queued request is not found by its transaction: %+v, %v", stored, err)
	}

	proofs := PreserveRawPathMiddleware(http.HandlerFunc(NewProofHandlers(repos, "api-test", log.New(io.Discard, "", 0)).HandleGetProofByTxHash))
	for _, value := range []string{txID, hash} {
		rec := httptest.NewRecorder()
		proofs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x/api/v1/proofs/tx/"+value, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), artifact.ProofID.String()) {
			t.Errorf("GET /api/v1/proofs/tx/%s = %d %s", value, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	proofs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://x/api/v1/proofs/tx/acc://"+other+"@txid-api.acme/data", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("another transaction's ID answered %d", rec.Code)
	}
}
