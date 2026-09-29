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
)

// RB2-F6/F7: the lifecycle endpoints serve the whole lifecycle row the validator writes and the outcome of every chain
// member, so a client can see which chains settled, which failed and why, and which have no outcome yet. They served
// 18 of the table's columns and no member outcomes at all.
func TestIntentLifecycleServesEveryColumnAndEveryMember(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	intentID := "lifecycle-" + uuid.NewString()
	txHash := uuid.NewString()[:8] + "aa"

	if _, err := db.ExecContext(ctx, `
		INSERT INTO intent_lifecycle (intent_id, accum_tx_hash, status, target_chain, proof_class, error_message,
			target_chains, leg_count, execution_mode, legs_completed, legs_failed, member_chains, settling_at, failed_at)
		VALUES ($1, $2, 'failed', '11155111', 'on_demand', 'chain 84532 reverted', ARRAY['11155111','84532','421614'],
			3, 'sequential', 1, 1, ARRAY[11155111,84532,421614]::bigint[], now(), now())`, intentID, txHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO intent_member_outcomes (intent_id, chain_id, settlement, proof_cycle, legs, settlement_tx, write_back_tx, reason)
		VALUES ($1, 11155111, 'settled', 'written', 1, '0xs1', 'wb1', NULL),
		       ($1, 84532, 'reverted', 'written', 1, '0xs2', 'wb2', 'reverted: out of gas')`, intentID); err != nil {
		t.Fatal(err)
	}

	h := NewIntentLifecycleHandlers(repos, log.New(io.Discard, "", 0))
	for _, path := range []string{"/api/v1/intent/" + intentID, "/api/v1/intent/tx/" + txHash} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if path == "/api/v1/intent/"+intentID {
			h.HandleGetByIntentID(rr, req)
		} else {
			h.HandleGetByTxHash(rr, req)
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", path, rr.Code, rr.Body.String())
		}
		var got struct {
			Status        string   `json:"status"`
			ErrorMessage  *string  `json:"error_message"`
			LegCount      *int     `json:"leg_count"`
			ExecutionMode *string  `json:"execution_mode"`
			LegsCompleted *int     `json:"legs_completed"`
			LegsFailed    *int     `json:"legs_failed"`
			TargetChains  []string `json:"target_chains"`
			MemberChains  []int64  `json:"member_chains"`
			SettlingAt    *string  `json:"settling_at"`
			Members       []struct {
				ChainID      int64   `json:"chain_id"`
				Recorded     bool    `json:"recorded"`
				Settlement   *string `json:"settlement"`
				ProofCycle   *string `json:"proof_cycle"`
				SettlementTx *string `json:"settlement_tx"`
				Reason       *string `json:"reason"`
			} `json:"members"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.LegCount == nil || *got.LegCount != 3 || got.ExecutionMode == nil || *got.ExecutionMode != "sequential" ||
			got.LegsCompleted == nil || *got.LegsCompleted != 1 || got.LegsFailed == nil || *got.LegsFailed != 1 ||
			len(got.TargetChains) != 3 || len(got.MemberChains) != 3 || got.SettlingAt == nil {
			t.Fatalf("%s: lifecycle columns missing: %s", path, rr.Body.String())
		}
		if len(got.Members) != 3 {
			t.Fatalf("%s: want 3 members (every member chain, outcome or not), got %s", path, rr.Body.String())
		}
		byChain := map[int64]int{}
		for i, m := range got.Members {
			byChain[m.ChainID] = i
		}
		sep, base, arb := got.Members[byChain[11155111]], got.Members[byChain[84532]], got.Members[byChain[421614]]
		if !sep.Recorded || sep.Settlement == nil || *sep.Settlement != "settled" || sep.SettlementTx == nil || *sep.SettlementTx != "0xs1" {
			t.Fatalf("%s: sepolia member wrong: %s", path, rr.Body.String())
		}
		if !base.Recorded || base.Settlement == nil || *base.Settlement != "reverted" || base.Reason == nil || *base.Reason != "reverted: out of gas" {
			t.Fatalf("%s: base member wrong: %s", path, rr.Body.String())
		}
		if arb.Recorded || arb.Settlement != nil {
			t.Fatalf("%s: a member chain with no outcome must say it has none, got: %s", path, rr.Body.String())
		}
	}
}

// RB4-F61: a multi-member intent has one write-back and one proof per member, and the intent-level fields state
// only one of each (intent 000ac79a: arbitrum's proof 904ef697 and base's a6f55447). Each member names the proof its
// own recording cycle produced - the artifact whose cycle_id is the member's - and whether its committed effects
// were proven; a member with no such proof says it has none, never another member's.
func TestEachMemberNamesItsOwnProof(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	intentID := "f61-" + uuid.NewString()
	txHash := uuid.NewString()[:8] + "bb"
	if _, err := db.ExecContext(ctx, `
		INSERT INTO intent_lifecycle (intent_id, accum_tx_hash, status, member_chains)
		VALUES ($1, $2, 'complete', ARRAY[84532,421614,11155111]::bigint[])`, intentID, txHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO intent_member_outcomes (intent_id, chain_id, settlement, proof_cycle, legs, settlement_tx, write_back_tx, cycle_id, effects_proven)
		VALUES ($1, 84532, 'settled', 'written', 1, '0xbase', 'wb-base', 'cycle-base', true),
		       ($1, 421614, 'settled', 'written', 1, '0xarb', 'wb-arb', 'cycle-arb', NULL)`, intentID); err != nil {
		t.Fatal(err)
	}
	var baseProof, arbProof string
	for _, p := range []struct {
		cycle, chain string
		into         *string
	}{{"cycle-base", "84532", &baseProof}, {"cycle-arb", "421614", &arbProof}, {"cycle-unrelated", "84532", new(string)}} {
		if err := db.QueryRowContext(ctx, `
			INSERT INTO proof_artifacts (proof_type, accum_tx_hash, account_url, proof_class, validator_id, status, anchor_chain, artifact_json, artifact_hash)
			VALUES ('certen_anchor', $1, 'acc://f61.acme/data', 'on_demand', 'validator-6', 'anchored', $2, jsonb_build_object('cycle_id', $3::text), '\x00')
			RETURNING proof_id::text`, txHash, p.chain, p.cycle).Scan(p.into); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM proof_artifacts WHERE accum_tx_hash = $1`, txHash)
		db.Exec(`DELETE FROM intent_member_outcomes WHERE intent_id = $1`, intentID)
		db.Exec(`DELETE FROM intent_lifecycle WHERE intent_id = $1`, intentID)
	})

	h := NewIntentLifecycleHandlers(repos, log.New(io.Discard, "", 0))
	rr := httptest.NewRecorder()
	h.HandleGetByIntentID(rr, httptest.NewRequest(http.MethodGet, "/api/v1/intent/"+intentID, nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var got struct {
		Members []struct {
			ChainID       int64   `json:"chain_id"`
			Recorded      bool    `json:"recorded"`
			ProofID       *string `json:"proof_id"`
			EffectsProven *bool   `json:"effects_proven"`
		} `json:"members"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	by := map[int64]int{}
	for i, m := range got.Members {
		by[m.ChainID] = i
	}
	base, arb, sep := got.Members[by[84532]], got.Members[by[421614]], got.Members[by[11155111]]
	if base.ProofID == nil || *base.ProofID != baseProof || arb.ProofID == nil || *arb.ProofID != arbProof {
		t.Fatalf("THE regression: each member must name the proof its own cycle produced: %s", rr.Body.String())
	}
	if base.EffectsProven == nil || !*base.EffectsProven || arb.EffectsProven != nil {
		t.Fatalf("effects_proven must be carried as recorded (true / not assessed): %s", rr.Body.String())
	}
	if sep.Recorded || sep.ProofID != nil {
		t.Fatalf("a member with no outcome has no proof: %s", rr.Body.String())
	}
}
