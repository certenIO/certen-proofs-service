// Copyright 2025 Certen Protocol
//
// The Transaction Center's "quorum met" claim, against a database holding both kinds of anchor row.

package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// anchor_batches carries two kinds of row and only one of them is evidence.
//
//	CANONICAL — written by the validator that proved the anchor, keyed (chain_id, bundle_id), root equal
//	            to the root the anchor holds, quorum_reached set from a signature the chain verified.
//	SHADOW    — a per-validator artefact of the retired batch pipeline. bundle_id NULL, merkle_root a
//	            local hash over pending blobs that was never published, quorum_reached set by a
//	            coordinator whose attestations never reached a chain.
//
// Reading the flag without distinguishing them is how the Transaction Center came to show "quorum met"
// for a root no anchor ever held. This test holds both rows in one database and requires the shadow one
// to report nothing.

// quorumTestDB is the shared schema (see TestMain). The rows go into the real tables, with every real
// constraint, because a hand-built fixture table tests the fixture rather than the database.
func quorumTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if testDB == nil {
		t.Skip("Test database not configured")
	}
	return testDB
}

// insertAnchoredIntent adds one anchor row and the member row pointing at it, and removes both when the
// test ends (the member row cascades from the anchor row).
func insertAnchoredIntent(t *testing.T, db *sql.DB, intentID string, bundleID *string, quorumReached bool) {
	t.Helper()
	ctx := context.Background()
	batchID := uuid.New()
	var chainID interface{}
	if bundleID != nil {
		chainID = int64(84532)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO anchor_batches (id, status, merkle_root, transaction_count, quorum_reached, chain_id, bundle_id)
		 VALUES ($1, 'confirmed', $2, 1, $3, $4, $5)`,
		batchID, []byte{0xaa}, quorumReached, chainID, bundleID); err != nil {
		t.Fatalf("inserting anchor row: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM anchor_batches WHERE id = $1`, batchID)
	})
	if _, err := db.ExecContext(ctx,
		`INSERT INTO batch_transactions (batch_id, accumulate_tx_hash, account_url, tree_index, intent_id)
		 VALUES ($1, '0xabc', 'acc://example.acme/tokens', 0, $2)`,
		batchID, intentID); err != nil {
		t.Fatalf("inserting member row: %v", err)
	}
}

// A shadow row's quorum_reached must never reach the Transaction Center.
func TestBatchQuorumMetIsFalseForAShadowRow(t *testing.T) {
	db := quorumTestDB(t)

	intentID := "intent-shadow-" + uuid.NewString()
	insertAnchoredIntent(t, db, intentID, nil, true) // no bundle_id: a shadow row claiming a quorum

	details, err := NewProofArtifactRepository(db).GetProofByIntentID(context.Background(), intentID)
	if err != nil {
		t.Fatalf("GetProofByIntentID: %v", err)
	}
	if details == nil {
		t.Fatal("no details for an intent that has a batch row")
	}
	if details.BatchQuorumMet {
		t.Fatal("REGRESSION: a shadow row reported a quorum the chain never verified")
	}
}

// A canonical row still reports what it genuinely holds.
func TestBatchQuorumMetIsTrueForACanonicalRow(t *testing.T) {
	db := quorumTestDB(t)

	bundleID := "0x" + uuid.NewString()
	intentID := "intent-canonical-" + uuid.NewString()
	insertAnchoredIntent(t, db, intentID, &bundleID, true)

	details, err := NewProofArtifactRepository(db).GetProofByIntentID(context.Background(), intentID)
	if err != nil {
		t.Fatalf("GetProofByIntentID: %v", err)
	}
	if details == nil {
		t.Fatal("no details for an intent that has a canonical batch row")
	}
	if !details.BatchQuorumMet {
		t.Fatal("a canonical anchor row did not report its quorum")
	}
}

// A canonical row that has NOT reached quorum still reports false — the canonical test adds a condition,
// it does not replace the flag.
func TestBatchQuorumMetIsFalseForACanonicalRowWithoutQuorum(t *testing.T) {
	db := quorumTestDB(t)

	bundleID := "0x" + uuid.NewString()
	intentID := "intent-pending-" + uuid.NewString()
	insertAnchoredIntent(t, db, intentID, &bundleID, false)

	details, err := NewProofArtifactRepository(db).GetProofByIntentID(context.Background(), intentID)
	if err != nil {
		t.Fatalf("GetProofByIntentID: %v", err)
	}
	if details == nil || details.BatchQuorumMet {
		t.Fatalf("a canonical row without quorum reported %+v", details)
	}
}
