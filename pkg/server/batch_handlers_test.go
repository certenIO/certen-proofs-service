package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// RB4-F3: the bridge's /api/v1/anchor/batch/:batchId proxied /api/v1/batches/{id}, which no route served, so every
// batch was "not found". The endpoint serves the anchor batch as the validators write it (their quorum path leaves
// validator_id NULL, which the repository's older reader could not scan), with the counts of the proofs stored against it; an unknown batch is
// named as not found, and so are its stats (they were zero counts for any ID).
func TestBatchEndpointServesTheValidatorsRecord(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	id := uuid.New()
	root := h32("batch-root-" + id.String())
	bundle := "bundle-" + id.String()[:8]
	createTx := "0x" + hex.EncodeToString(h32("create-"+id.String()))
	verifyTx := "0x" + hex.EncodeToString(h32("verify-"+id.String()))
	if _, err := db.ExecContext(ctx, `
		INSERT INTO anchor_batches (id, batch_type, status, merkle_root, target_chain, validator_id, transaction_count, tx_count,
			chain_id, bundle_id, anchor_create_tx, verify_tx, verify_block, message_hash, signers, signed_voting_power,
			total_voting_power, proof_data_included, attestation_count, quorum_reached, consensus_completed_at,
			evidence_source, lane, anchor_tx_hash, anchored_at, confirmed_at, closed_at, anchor_block_num)
		VALUES ($1, 'on_demand', 'confirmed', $2, 'ethereum', NULL, 3, 3, 84532, $3, $4, $5, 101,
			'0xmsg', '[{"validator":"v1"}]'::jsonb, 5, 7, TRUE, 5, TRUE, now(), 'chain_event', 'on_demand',
			$4, now(), now(), now(), 100)`, id, root, bundle, createTx, verifyTx); err != nil {
		t.Fatal(err)
	}
	for i, status := range []string{"verified", "failed"} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO proof_artifacts (proof_type, accum_tx_hash, account_url, proof_class, validator_id, artifact_json,
				artifact_hash, batch_id, verification_status)
			VALUES ('certen_anchor', $1, 'acc://batch.acme/data', 'on_demand', 'validator-1', '{}'::jsonb, $2, $3, $4)`,
			hex.EncodeToString(h32(fmt.Sprintf("tx-%d-%s", i, id))), h32(fmt.Sprintf("artifact-%d-%s", i, id)), id, status); err != nil {
			t.Fatal(err)
		}
	}

	h := NewBatchHandlers(repos, log.New(io.Discard, "", 0))
	serve := func(path string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		h.HandleBatches(rr, httptest.NewRequest(http.MethodGet, path, nil))
		return rr
	}

	rr := serve("/api/v1/batches/" + id.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		BatchID           string              `json:"batch_id"`
		Status            string              `json:"status"`
		MerkleRoot        string              `json:"merkle_root"`
		TransactionCount  int                 `json:"transaction_count"`
		ChainID           int64               `json:"chain_id"`
		BundleID          string              `json:"bundle_id"`
		ValidatorID       *string             `json:"validator_id"`
		AnchorCreateTx    string              `json:"anchor_create_tx"`
		AnchorBlockNumber int64               `json:"anchor_block_number"`
		VerifyTx          string              `json:"verify_tx"`
		VerifyBlock       int64               `json:"verify_block"`
		QuorumReached     bool                `json:"quorum_reached"`
		AttestationCount  int                 `json:"attestation_count"`
		SignedVotingPower string              `json:"signed_voting_power"`
		TotalVotingPower  string              `json:"total_voting_power"`
		Signers           []map[string]string `json:"signers"`
		Proofs            *struct {
			ProofCount    int `json:"proof_count"`
			VerifiedCount int `json:"verified_count"`
			FailedCount   int `json:"failed_count"`
		} `json:"proofs"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.BatchID != id.String() || got.Status != "confirmed" || got.MerkleRoot != hex.EncodeToString(root) ||
		got.TransactionCount != 3 || got.ChainID != 84532 || got.BundleID != bundle || got.ValidatorID != nil ||
		got.AnchorCreateTx != createTx || got.AnchorBlockNumber != 100 || got.VerifyTx != verifyTx || got.VerifyBlock != 101 ||
		!got.QuorumReached || got.AttestationCount != 5 || got.SignedVotingPower != "5" || got.TotalVotingPower != "7" ||
		len(got.Signers) != 1 || got.Proofs == nil || got.Proofs.ProofCount != 2 || got.Proofs.VerifiedCount != 1 || got.Proofs.FailedCount != 1 {
		t.Fatalf("batch record not served as written: %s", rr.Body.String())
	}

	if rr := serve("/api/v1/batches/" + id.String() + "/stats"); rr.Code != http.StatusOK {
		t.Fatalf("stats: status %d: %s", rr.Code, rr.Body.String())
	}
	for _, path := range []string{"/api/v1/batches/" + uuid.NewString(), "/api/v1/batches/" + uuid.NewString() + "/stats"} {
		rr := serve(path)
		var e struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(rr.Body.Bytes(), &e)
		if rr.Code != http.StatusNotFound || e.Error.Code != "BATCH_NOT_FOUND" {
			t.Fatalf("%s: want 404 BATCH_NOT_FOUND, got %d %s", path, rr.Code, rr.Body.String())
		}
	}
	if rr := serve("/api/v1/batches/not-a-uuid"); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid id: want 400, got %d", rr.Code)
	}
	if rr := serve("/api/v1/batches/" + id.String() + "/elsewhere"); rr.Code != http.StatusNotFound {
		t.Fatalf("unknown sub-path: want 404, got %d", rr.Code)
	}
}
