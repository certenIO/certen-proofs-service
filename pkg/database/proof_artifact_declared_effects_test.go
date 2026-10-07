// Copyright 2025 Certen Protocol
//
// Declared effects on the proof lookups: the three states (unknown, declared nothing, declared events) must
// survive the join from batch_transactions to both GetProofByID and GetProofByTxHash.

package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestProofLookupsReturnDeclaredEffectsInTheirThreeStates(t *testing.T) {
	if testDB == nil {
		t.Skip("Test database not configured")
	}
	ctx := context.Background()
	repo := NewProofArtifactRepository(testDB)

	cases := []struct {
		name    string
		column  any // what batch_transactions.declared_effects holds; nil is SQL NULL
		wantKey bool
		want    string
	}{
		{"unknown (SQL NULL)", nil, false, ""},
		{"declared nothing ([])", `[]`, true, `[]`},
		{"declared events", `[{"contract":"0x00000000000000000000000000000000000000aa","topic0":"0xddf252ad"}]`, true,
			`[{"contract":"0x00000000000000000000000000000000000000aa","topic0":"0xddf252ad"}]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			batchID := uuid.New()
			intentID := "declared-effects-" + uuid.New().String()[:8]
			txHash := "de" + uuid.New().String()[:12]
			if _, err := testDB.ExecContext(ctx, `INSERT INTO anchor_batches (id) VALUES ($1)`, batchID); err != nil {
				t.Fatalf("anchor_batches: %v", err)
			}
			t.Cleanup(func() {
				_, _ = testDB.ExecContext(ctx, `DELETE FROM proof_artifacts WHERE intent_id = $1`, intentID)
				_, _ = testDB.ExecContext(ctx, `DELETE FROM batch_transactions WHERE batch_id = $1`, batchID)
				_, _ = testDB.ExecContext(ctx, `DELETE FROM anchor_batches WHERE id = $1`, batchID)
			})
			if _, err := testDB.ExecContext(ctx,
				`INSERT INTO batch_transactions (batch_id, accumulate_tx_hash, account_url, tree_index, intent_id, declared_effects)
				 VALUES ($1, $2, 'acc://declared.acme/tokens', 0, $3, $4)`, batchID, txHash, intentID, c.column); err != nil {
				t.Fatalf("batch_transactions: %v", err)
			}
			art, err := repo.CreateProofArtifact(ctx, &NewProofArtifact{
				ProofType: ProofTypeCertenAnchor, AccumTxHash: txHash, AccountURL: "acc://declared.acme/tokens",
				ProofClass: ProofClassOnDemand, ValidatorID: "test-validator-1", ArtifactJSON: json.RawMessage(`{}`),
			})
			if err != nil {
				t.Fatalf("CreateProofArtifact: %v", err)
			}
			if _, err := testDB.ExecContext(ctx, `UPDATE proof_artifacts SET intent_id = $1 WHERE proof_id = $2`, intentID, art.ProofID); err != nil {
				t.Fatalf("link intent: %v", err)
			}

			byID, err := repo.GetProofByID(ctx, art.ProofID)
			if err != nil || byID == nil {
				t.Fatalf("GetProofByID: %v %v", byID, err)
			}
			byTx, err := repo.GetProofByTxHash(ctx, txHash)
			if err != nil || byTx == nil {
				t.Fatalf("GetProofByTxHash: %v %v", byTx, err)
			}
			for lookup, got := range map[string]json.RawMessage{"GetProofByID": byID.DeclaredEffects, "GetProofByTxHash": byTx.DeclaredEffects} {
				if !c.wantKey {
					if got != nil {
						t.Errorf("%s: unknown must stay nil, got %s", lookup, got)
					}
					continue
				}
				if got == nil {
					t.Errorf("%s: declared effects missing, want %s", lookup, c.want)
					continue
				}
				var a, b any
				_ = json.Unmarshal(got, &a)
				_ = json.Unmarshal([]byte(c.want), &b)
				ja, _ := json.Marshal(a)
				jb, _ := json.Marshal(b)
				if string(ja) != string(jb) {
					t.Errorf("%s: got %s want %s", lookup, got, c.want)
				}
			}
			// the key itself: nil marshals without it (state 1), an empty array with it (state 2)
			out, _ := json.Marshal(byTx)
			var m map[string]json.RawMessage
			_ = json.Unmarshal(out, &m)
			if _, has := m["declared_effects"]; has != c.wantKey {
				t.Errorf("JSON key declared_effects present=%v, want %v", has, c.wantKey)
			}
		})
	}
}
