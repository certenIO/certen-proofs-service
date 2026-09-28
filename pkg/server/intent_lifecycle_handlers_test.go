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
				ChainID     int64   `json:"chain_id"`
				Recorded    bool    `json:"recorded"`
				Settlement  *string `json:"settlement"`
				ProofCycle  *string `json:"proof_cycle"`
				SettlementTx *string `json:"settlement_tx"`
				Reason      *string `json:"reason"`
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
