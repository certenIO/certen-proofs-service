package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

// An intent on several chains has ONE proof request, which ends with the first chain member's proof (RB7, intent
// f6f19875: request ended 21:27:12 with Base; Sepolia landed 21:48:12, Arbitrum 22:07:10). A caller that learns of
// ends from this feed must learn of every member as it lands, the last one above all, or it waits out its backoff.
// These tests decode the page themselves, so they run unchanged against a feed that predates `members`.

type memberFeedItem struct {
	IntentID    string    `json:"intent_id"`
	ChainID     int64     `json:"chain_id"`
	AccumTxHash *string   `json:"accum_tx_hash"`
	Settlement  string    `json:"settlement"`
	ProofCycle  string    `json:"proof_cycle"`
	ProofID     *string   `json:"proof_id"`
	RecordedAt  time.Time `json:"recorded_at"`
}

type memberFeedPage struct {
	Requests []struct {
		RequestID string `json:"request_id"`
	} `json:"requests"`
	Members *[]memberFeedItem `json:"members"`
	Next    string            `json:"next"`
	Cursor  string            `json:"cursor"`
}

func getMemberFeed(t *testing.T, h *BundleHandlers, query url.Values) memberFeedPage {
	t.Helper()
	rec := httptest.NewRecorder()
	h.HandleTerminalRequests(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proofs/requests/completed?"+query.Encode(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("feed: %d %s", rec.Code, rec.Body.String())
	}
	var page memberFeedPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode feed page: %v: %s", err, rec.Body.String())
	}
	if page.Members == nil {
		t.Fatalf("the feed serves no chain members, so a member that lands without its proof request ending is never served: %s", rec.Body.String())
	}
	return page
}

// A multi-chain intent fixture: its lifecycle row, one member per chain recorded at the given times, each with the
// proof its cycle produced.
type memberFixture struct {
	intentID, txHash string
	proofs           map[int64]string
}

func insertMultiChainIntent(t *testing.T, db *sql.DB, recorded map[int64]time.Time) memberFixture {
	t.Helper()
	ctx := context.Background()
	f := memberFixture{intentID: "feed-" + uuid.NewString(), txHash: uuid.NewString()[:8] + "feed", proofs: map[int64]string{}}
	if _, err := db.ExecContext(ctx, `INSERT INTO intent_lifecycle (intent_id, accum_tx_hash, status, member_chains)
		VALUES ($1, $2, 'settling', ARRAY[84532,11155111,421614]::bigint[])`, f.intentID, f.txHash); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM proof_artifacts WHERE accum_tx_hash = $1`, f.txHash)
		db.Exec(`DELETE FROM intent_member_outcomes WHERE intent_id = $1`, f.intentID)
		db.Exec(`DELETE FROM intent_lifecycle WHERE intent_id = $1`, f.intentID)
	})
	for chain, at := range recorded {
		cycle := f.intentID + "-cycle-" + time.Duration(chain).String()
		if _, err := db.ExecContext(ctx, `INSERT INTO intent_member_outcomes
			(intent_id, chain_id, settlement, proof_cycle, legs, settlement_tx, write_back_tx, cycle_id, recorded_at)
			VALUES ($1, $2, 'settled', 'written', 1, '0xs', 'wb', $3, $4)`, f.intentID, chain, cycle, at); err != nil {
			t.Fatal(err)
		}
		var proof string
		if err := db.QueryRowContext(ctx, `
			INSERT INTO proof_artifacts (proof_type, accum_tx_hash, account_url, proof_class, validator_id, status, anchor_chain, artifact_json, artifact_hash)
			VALUES ('certen_anchor', $1, 'acc://feed.acme/data', 'on_demand', 'feed-test', 'anchored', $2, jsonb_build_object('cycle_id', $3::text), '\x00')
			RETURNING proof_id::text`, f.txHash, chain, cycle).Scan(&proof); err != nil {
			t.Fatal(err)
		}
		f.proofs[chain] = proof
	}
	return f
}

// The f6f19875 shape: the request ends with the first member; the other two members land 21 and 40 minutes later. A
// caller positioned after the request's end is served both later members, each with its own proof, in order, paged.
func TestFeedServesEachChainMemberAsItLands(t *testing.T) {
	db, repos := recordsDB(t)
	h := NewBundleHandlers(repos, nil, log.New(io.Discard, "", 0))

	// A window of the past nobody else writes into (see TestTerminalRequestFeedServesEveryEndInOrderAndPages).
	base := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(rand.Int63n(1<<17)) * time.Hour)
	requestEnded := base.Add(time.Second)
	f := insertMultiChainIntent(t, db, map[int64]time.Time{
		84532:    requestEnded.Add(3 * time.Second),
		11155111: requestEnded.Add(21 * time.Minute),
		421614:   requestEnded.Add(40 * time.Minute),
	})
	insertFeedRequest(t, db, "acc://"+f.txHash+"@feed.acme/data", "completed", &requestEnded, nil)

	// The caller read the request's end: its cursor is just after it, and after the first member.
	first := getMemberFeed(t, h, url.Values{"since": {base.Format(time.RFC3339Nano)}, "limit": {"1"}})
	if len(first.Requests) != 1 || len(*first.Members) != 1 || (*first.Members)[0].ChainID != 84532 || first.Next == "" {
		t.Fatalf("first page: the request and the first member: %+v", first)
	}

	// Paged one at a time, and to the end: rows of other tests (recorded now, after this window) follow these.
	var later []memberFeedItem
	query := url.Values{"after": {first.Next}, "limit": {"1"}}
	for pages := 0; ; pages++ {
		if pages > 10000 {
			t.Fatal("the feed did not end")
		}
		page := getMemberFeed(t, h, query)
		for _, r := range page.Requests {
			if r.RequestID == first.Requests[0].RequestID {
				t.Fatalf("the request was served twice: %+v", page)
			}
		}
		for _, m := range *page.Members {
			if m.IntentID == f.intentID {
				later = append(later, m)
			}
		}
		if page.Next == "" {
			break
		}
		query = url.Values{"after": {page.Next}, "limit": {"1"}}
	}
	if len(later) != 2 || later[0].ChainID != 11155111 || later[1].ChainID != 421614 {
		t.Fatalf("THE regression: the members that land after the request ended must each be served, in order: %+v", later)
	}
	for _, m := range later {
		if m.IntentID != f.intentID || m.AccumTxHash == nil || *m.AccumTxHash != f.txHash || m.Settlement != "settled" ||
			m.ProofCycle != "written" || m.ProofID == nil || *m.ProofID != f.proofs[m.ChainID] {
			t.Fatalf("a member is not served as recorded, with its own proof: %+v (proofs %v)", m, f.proofs)
		}
	}
	if !later[1].RecordedAt.Equal(requestEnded.Add(40 * time.Minute)) {
		t.Fatalf("recorded_at is not the member's: %v", later[1].RecordedAt)
	}
}

// A member is served once its record has settled, never before; recorded again (its outcome changed) it is served
// again; a member with no lifecycle row is served with a null hash, never dropped; and a v1 cursor - the gateway may
// hold one across this deploy - serves the members recorded from its instant.
func TestFeedFollowsMemberRecords(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	h := NewBundleHandlers(repos, nil, log.New(io.Discard, "", 0))

	start := time.Now().Add(-time.Minute).UTC()
	f := insertMultiChainIntent(t, db, map[int64]time.Time{84532: time.Now()})
	orphan := "feed-orphan-" + uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO intent_member_outcomes (intent_id, chain_id, settlement, proof_cycle, legs, recorded_at)
		VALUES ($1, 421614, 'none', 'refused', 1, now() - interval '10 seconds')`, orphan); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec(`DELETE FROM intent_member_outcomes WHERE intent_id = $1`, orphan) })

	v1 := base64.RawURLEncoding.EncodeToString([]byte("v1|" + start.Format(time.RFC3339Nano) + "|" + uuid.Nil.String()))
	served := func() map[string]memberFeedItem {
		t.Helper()
		out := map[string]memberFeedItem{}
		query := url.Values{"after": {v1}, "limit": {"500"}}
		for pages := 0; pages < 100; pages++ {
			page := getMemberFeed(t, h, query)
			for _, m := range *page.Members {
				out[m.IntentID+"/"+time.Duration(m.ChainID).String()] = m
			}
			if page.Next == "" {
				return out
			}
			query = url.Values{"after": {page.Next}, "limit": {"500"}}
		}
		t.Fatal("the feed did not end")
		return nil
	}
	key := f.intentID + "/" + time.Duration(84532).String()

	if _, ok := served()[key]; ok {
		t.Fatal("a member was served before its record had settled; a cursor could pass an earlier stamp still committing")
	}
	if _, err := db.ExecContext(ctx, `UPDATE intent_member_outcomes SET recorded_at = recorded_at - interval '6 seconds' WHERE intent_id = $1`, f.intentID); err != nil {
		t.Fatal(err)
	}
	got := served()
	if m, ok := got[key]; !ok || m.ProofCycle != "written" {
		t.Fatalf("a settled member record was not served from a v1 cursor: %+v", got)
	}
	if m, ok := got[orphan+"/"+time.Duration(421614).String()]; !ok || m.AccumTxHash != nil || m.ProofID != nil || m.ProofCycle != "refused" {
		t.Fatalf("a member without a lifecycle row must be served with a null hash: %+v", got)
	}

	// Recorded anew: served again at its new place, from a cursor that had passed the old one.
	page := getMemberFeed(t, h, url.Values{"after": {v1}, "limit": {"500"}})
	for page.Next != "" {
		page = getMemberFeed(t, h, url.Values{"after": {page.Next}, "limit": {"500"}})
	}
	if _, err := db.ExecContext(ctx, `UPDATE intent_member_outcomes SET proof_cycle = 'proof_pending', recorded_at = now() - interval '6 seconds'
		WHERE intent_id = $1`, f.intentID); err != nil {
		t.Fatal(err)
	}
	again := getMemberFeed(t, h, url.Values{"after": {page.Cursor}, "limit": {"500"}})
	found := false
	for _, m := range *again.Members {
		if m.IntentID == f.intentID && m.ProofCycle == "proof_pending" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a member recorded anew was not served again: %+v", *again.Members)
	}
}
