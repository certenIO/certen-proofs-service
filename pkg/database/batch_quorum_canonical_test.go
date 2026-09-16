// Copyright 2025 Certen Protocol
//
// The Transaction Center's "quorum met" claim, against a database holding both kinds of anchor row.

package database

import (
	"context"
	"database/sql"
	"os"
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

func quorumTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn := os.Getenv("CERTEN_TEST_DB")
	if conn == "" {
		t.Skip("CERTEN_TEST_DB not set")
	}
	db, err := sql.Open("postgres", conn)
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// createQuorumFixtureSchema builds only the columns GetProofByIntentID reads. A narrow fixture keeps the
// test about the SQL under review rather than about the fleet's full schema.
func createQuorumFixtureSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	stmts := []string{
		`DROP TABLE IF EXISTS anchor_records, batch_transactions, proof_artifacts, anchor_batches, certen_intents CASCADE`,
		`CREATE TABLE anchor_batches (
			id UUID PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'confirmed',
			merkle_root BYTEA,
			transaction_count INT NOT NULL DEFAULT 0,
			quorum_reached BOOLEAN NOT NULL DEFAULT FALSE,
			chain_id BIGINT,
			bundle_id TEXT
		)`,
		`CREATE TABLE batch_transactions (
			batch_id UUID NOT NULL,
			accumulate_tx_hash TEXT,
			account_url TEXT NOT NULL DEFAULT '',
			user_id TEXT,
			intent_id TEXT,
			from_chain TEXT NOT NULL DEFAULT 'accumulate',
			to_chain TEXT NOT NULL DEFAULT 'base-sepolia',
			from_address TEXT NOT NULL DEFAULT '',
			to_address TEXT NOT NULL DEFAULT '',
			amount TEXT NOT NULL DEFAULT '0',
			token_symbol TEXT NOT NULL DEFAULT 'ACME',
			adi_url TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at_client TIMESTAMPTZ
		)`,
		// GetProofByIntentID falls through to a lookup by accum_tx_hash when no proof row matches, so the
		// fixture carries the columns that lookup selects. It stays empty in these tests; what matters is
		// that the fall-through can run and return nothing.
		`CREATE TABLE proof_artifacts (
			proof_id UUID,
			intent_id TEXT,
			proof_type TEXT,
			proof_version TEXT,
			accum_tx_hash TEXT,
			account_url TEXT,
			batch_id UUID,
			batch_position INT,
			anchor_id UUID,
			anchor_tx_hash TEXT,
			anchor_block_number BIGINT,
			anchor_chain TEXT,
			merkle_root BYTEA,
			leaf_hash BYTEA,
			leaf_index INT,
			gov_level TEXT,
			proof_class TEXT,
			validator_id TEXT,
			status TEXT,
			verification_status TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			anchored_at TIMESTAMPTZ,
			verified_at TIMESTAMPTZ,
			artifact_json JSONB,
			artifact_hash BYTEA
		)`,
		// Multi-leg detection reads this table; an intent absent from it is single-leg.
		`CREATE TABLE certen_intents (
			intent_id TEXT PRIMARY KEY,
			leg_count INT NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE anchor_records (
			batch_id UUID,
			anchor_tx_hash TEXT,
			confirmations INT,
			is_final BOOLEAN
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("fixture schema: %v\n%s", err, s)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(),
			`DROP TABLE IF EXISTS anchor_records, batch_transactions, proof_artifacts, anchor_batches, certen_intents CASCADE`)
	})
}

// insertAnchoredIntent adds one anchor row and the member row pointing at it.
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
	if _, err := db.ExecContext(ctx,
		`INSERT INTO batch_transactions (batch_id, accumulate_tx_hash, account_url, intent_id)
		 VALUES ($1, '0xabc', 'acc://example.acme/tokens', $2)`,
		batchID, intentID); err != nil {
		t.Fatalf("inserting member row: %v", err)
	}
}

// A shadow row's quorum_reached must never reach the Transaction Center.
func TestBatchQuorumMetIsFalseForAShadowRow(t *testing.T) {
	db := quorumTestDB(t)
	createQuorumFixtureSchema(t, db)

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
	createQuorumFixtureSchema(t, db)

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
	createQuorumFixtureSchema(t, db)

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
