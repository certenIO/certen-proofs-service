package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// RB4-F28 (owner: keep the batch repository's methods, fixed to read the full record). Every reader returns the
// complete anchor_batches record and reads a row the validators' quorum path writes (validator_id NULL); the
// writers write no invented values (an all-zero merkle root, an "ethereum" chain), keep both transaction counts in
// step, and name an update to a batch that does not exist instead of succeeding silently.
func TestBatchRepositoryReadsAndWritesTheFullRecord(t *testing.T) {
	if testDB == nil {
		t.Skip("Test database not configured")
	}
	ctx := context.Background()
	repo := NewBatchRepository(&Client{db: testDB})
	validator := "validator-" + uuid.NewString()[:8]

	// A row as the quorum path writes it: no validator, a chain id and quorum evidence.
	quorumID := uuid.New()
	quorumRoot := sha("quorum-root-" + quorumID.String())
	if _, err := testDB.ExecContext(ctx, `
		INSERT INTO anchor_batches (id, batch_type, status, merkle_root, target_chain, validator_id, transaction_count, tx_count,
			chain_id, bundle_id, quorum_reached, attestation_count, closed_at)
		VALUES ($1, 'on_demand', 'closed', $2, 'ethereum', NULL, 2, 2, 421614, $3, TRUE, 5, now())`,
		quorumID, quorumRoot, "bundle-"+quorumID.String()[:8]); err != nil {
		t.Fatal(err)
	}
	byRoot, err := repo.GetBatchByMerkleRoot(ctx, quorumRoot)
	if err != nil {
		t.Fatalf("GetBatchByMerkleRoot on a quorum row: %v", err)
	}
	if byRoot.BatchID != quorumID || byRoot.ValidatorID != nil || byRoot.ChainID == nil || *byRoot.ChainID != 421614 || !byRoot.QuorumReached {
		t.Fatalf("GetBatchByMerkleRoot: not the full record: %+v", byRoot)
	}
	ready, err := repo.GetBatchesReadyForAnchoring(ctx)
	if err != nil {
		t.Fatalf("GetBatchesReadyForAnchoring with a quorum row present: %v", err)
	}
	found := false
	for _, b := range ready {
		if b.BatchID == quorumID {
			found = b.AttestationCount == 5 && b.BundleID != nil
		}
	}
	if !found {
		t.Fatal("GetBatchesReadyForAnchoring: the closed quorum row is missing or not the full record")
	}

	// The writer's lifecycle.
	if _, err := repo.CreateBatch(ctx, &NewAnchorBatch{BatchType: BatchTypeOnCadence, ValidatorID: validator}); err == nil {
		t.Fatal("CreateBatch without a target chain must be refused, not anchored to an invented chain")
	}
	if _, err := repo.CreateBatch(ctx, &NewAnchorBatch{BatchType: BatchTypeOnCadence, TargetChain: "base-sepolia"}); err == nil {
		t.Fatal("CreateBatch without a validator must be refused")
	}
	created, err := repo.CreateBatch(ctx, &NewAnchorBatch{BatchType: BatchTypeOnCadence, ValidatorID: validator, TargetChain: "base-sepolia"})
	if err != nil {
		t.Fatal(err)
	}
	if created.MerkleRoot != nil || created.Status != "pending" || created.TargetChain != "base-sepolia" ||
		created.ValidatorID == nil || *created.ValidatorID != validator || created.StartedAt == nil {
		t.Fatalf("CreateBatch: %+v", created)
	}
	pending, err := repo.GetPendingBatch(ctx, validator, BatchTypeOnCadence)
	if err != nil || pending.BatchID != created.BatchID {
		t.Fatalf("GetPendingBatch: %v %+v", err, pending)
	}
	if err := repo.IncrementTxCount(ctx, created.BatchID); err != nil {
		t.Fatal(err)
	}
	var txCount, transactionCount int
	if err := testDB.QueryRowContext(ctx, `SELECT tx_count, transaction_count FROM anchor_batches WHERE id = $1`, created.BatchID).
		Scan(&txCount, &transactionCount); err != nil {
		t.Fatal(err)
	}
	if txCount != 1 || transactionCount != 1 {
		t.Fatalf("IncrementTxCount: tx_count %d, transaction_count %d — both count the batch's transactions", txCount, transactionCount)
	}
	root := sha("closed-root-" + created.BatchID.String())
	if err := repo.CloseBatch(ctx, created.BatchID, root, 77, "0xaccum"); err != nil {
		t.Fatal(err)
	}
	closed, err := repo.GetBatch(ctx, created.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != "closed" || closed.MerkleRoot == nil || *closed.MerkleRoot != hex.EncodeToString(root) ||
		closed.ClosedAt == nil || closed.EndedAt == nil || closed.AccumulateBlockHeight == nil || *closed.AccumulateBlockHeight != 77 {
		t.Fatalf("CloseBatch: %+v", closed)
	}
	if err := repo.CloseBatch(ctx, created.BatchID, root, 77, "0xaccum"); err == nil || errors.Is(err, ErrBatchNotFound) {
		t.Fatalf("closing a closed batch: want a not-pending error, got %v", err)
	}
	if err := repo.UpdateBatchStatus(ctx, created.BatchID, BatchStatusAnchored, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateBatchStatus(ctx, created.BatchID, BatchStatusConfirmed, ""); err != nil {
		t.Fatal(err)
	}
	confirmed, err := repo.GetBatch(ctx, created.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "confirmed" || confirmed.AnchoredAt == nil || confirmed.ConfirmedAt == nil {
		t.Fatalf("UpdateBatchStatus: the transition times are not recorded: %+v", confirmed)
	}

	// A batch that does not exist is named, by every writer.
	missing := uuid.New()
	for name, err := range map[string]error{
		"CloseBatch":        repo.CloseBatch(ctx, missing, root, 1, "0x"),
		"UpdateBatchStatus": repo.UpdateBatchStatus(ctx, missing, BatchStatusFailed, "x"),
		"IncrementTxCount":  repo.IncrementTxCount(ctx, missing),
		"UpdateBatchPhase5": repo.UpdateBatchPhase5(ctx, missing, &BatchPhase5Update{}),
	} {
		if !errors.Is(err, ErrBatchNotFound) {
			t.Errorf("%s on a missing batch: want ErrBatchNotFound, got %v", name, err)
		}
	}
	if _, err := repo.GetPendingBatch(ctx, "validator-none-"+uuid.NewString()[:8], BatchTypeOnDemand); !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("GetPendingBatch with none: want ErrBatchNotFound, got %v", err)
	}
}

func sha(label string) []byte {
	sum := sha256.Sum256([]byte(label))
	return sum[:]
}
