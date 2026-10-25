//go:build pgtest

package database

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
)

// LegHeaderRLP against the validators' real DDL for proof_finality_capture (migration 00028) in a private schema: the header comes from
// the newest bundle when it has one, else (a legacy leg) from the reconstructed header row keyed by the leg's chain, block and hash.
func TestLegHeaderRLPFromTheBundleAndFromTheLegacyHeaderRow(t *testing.T) {
	db := finalityTestDB(t)
	ctx := context.Background()
	r := NewProofArtifactRepository(db)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`CREATE TABLE proof_bundles (bundle_id uuid PRIMARY KEY DEFAULT gen_random_uuid(), proof_id uuid NOT NULL, bundle_data bytea NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now())`)
	withHeader := func(h string) []byte {
		return gz(t, `{"proof_components":{"5_execution_proof":{"chain_id":"84532","block_header_rlp":"`+h+`"}}}`)
	}
	legacy := gz(t, `{"proof_components":{"5_execution_proof":{"chain_id":"84532"}}}`)
	hash := bytes.Repeat([]byte{0x44}, 32)
	var hash32 [32]byte
	copy(hash32[:], hash)

	byBundle, byRow, none, newest := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO proof_bundles (proof_id, bundle_data, created_at) VALUES ($1, $2, now())`, byBundle, withHeader("0x01020304"))
	// the newest bundle decides: an older bundle with a header is not read once a newer one exists
	exec(`INSERT INTO proof_bundles (proof_id, bundle_data, created_at) VALUES ($1, $2, now() - interval '1 hour')`, newest, withHeader("0xaaaa"))
	exec(`INSERT INTO proof_bundles (proof_id, bundle_data, created_at) VALUES ($1, $2, now())`, newest, withHeader("0xbbbb"))
	exec(`INSERT INTO proof_bundles (proof_id, bundle_data) VALUES ($1, $2)`, byRow, legacy)
	exec(`INSERT INTO proof_finality_capture (chain, l1_block, l1_hash, item, bytes) VALUES (84532, 47368146, $1, 'legacyleg/header', $2)`, hash, []byte{9, 9, 9})

	get := func(proof uuid.UUID, chain, number uint64, h [32]byte) []byte {
		t.Helper()
		got, err := r.LegHeaderRLP(ctx, proof, chain, number, h)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := get(byBundle, 84532, 47368146, hash32); !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Errorf("bundle-sourced: %x", got)
	}
	if got := get(newest, 84532, 47368146, hash32); !bytes.Equal(got, []byte{0xbb, 0xbb}) {
		t.Errorf("the newest bundle: %x", got)
	}
	if got := get(byRow, 84532, 47368146, hash32); !bytes.Equal(got, []byte{9, 9, 9}) {
		t.Errorf("legacy-sourced: %x", got)
	}
	var other [32]byte
	other[0] = 1
	for name, got := range map[string][]byte{
		"a legacy leg whose row is for another hash":  get(byRow, 84532, 47368146, other),
		"a legacy leg whose row is for another block": get(byRow, 84532, 47368147, hash32),
		"a legacy leg whose row is for another chain": get(byRow, 1, 47368146, hash32),
		"a proof with no bundle and no row":           get(none, 84532, 47368146, other),
	} {
		if len(got) != 0 {
			t.Errorf("%s: %x", name, got)
		}
	}
}
