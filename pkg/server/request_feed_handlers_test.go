package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// The gateway's poller learns that a proof request ended from this feed, once per tick, instead of
// waiting out a backoff of up to 15 minutes. These tests hold the feed to its contract: every request
// that reached a terminal status after the cursor, in (completed_at, request_id) order, paged; nothing
// that has not ended; and a malformed position refused by name.

func getFeed(t *testing.T, h *BundleHandlers, query url.Values, principal *Principal) (int, TerminalRequestsResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proofs/requests/completed?"+query.Encode(), nil)
	if principal != nil {
		req = req.WithContext(WithPrincipal(req.Context(), *principal))
	}
	rec := httptest.NewRecorder()
	h.HandleTerminalRequests(rec, req)
	var response TerminalRequestsResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatalf("decode feed page: %v: %s", err, rec.Body.String())
		}
	}
	return rec.Code, response, rec.Body.String()
}

func insertFeedRequest(t *testing.T, db *sql.DB, tx, status string, completedAt *time.Time, proofID *uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		INSERT INTO proof_requests (accum_tx_hash, proof_class, status, completed_at, proof_id, processed_at)
		VALUES ($1, 'on_demand', $2, $3, $4, NOW()) RETURNING request_id`,
		tx, status, completedAt, uuid.NullUUID{UUID: derefUUID(proofID), Valid: proofID != nil}).Scan(&id); err != nil {
		t.Fatalf("insert %s request: %v", status, err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM proof_requests WHERE request_id = $1`, id)
	})
	return id
}

func derefUUID(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

func TestTerminalRequestFeedServesEveryEndInOrderAndPages(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	h := NewBundleHandlers(repos, nil, log.New(io.Discard, "", 0))
	tag := uuid.NewString()

	artifact, err := repos.ProofArtifacts.CreateProofArtifact(ctx, &database.NewProofArtifact{
		ProofType: database.ProofTypeCertenAnchor, AccumTxHash: "feed-" + tag, AccountURL: "acc://feed.acme/data",
		ProofClass: database.ProofClassOnDemand, ValidatorID: "feed-test", ArtifactJSON: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM proof_requests WHERE proof_id = $1`, artifact.ProofID)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM proof_artifacts WHERE proof_id = $1`, artifact.ProofID)
	})

	// A window of the past nobody else writes into, so rows from other tests cannot interleave with these.
	base := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(rand.Int63n(1<<17)) * time.Hour) // 2001 to 2015
	at := func(s int) *time.Time { v := base.Add(time.Duration(s) * time.Second); return &v }

	completed := insertFeedRequest(t, db, "acc://"+tag+"-1@feed.acme/data", "completed", at(1), &artifact.ProofID)
	failed := insertFeedRequest(t, db, "acc://"+tag+"-2@feed.acme/data", "failed", at(2), nil)
	cancelled := insertFeedRequest(t, db, "acc://"+tag+"-3@feed.acme/data", "cancelled", at(2), nil)
	// Not ended, even though a stale completed_at is on the row: never served.
	insertFeedRequest(t, db, "acc://"+tag+"-4@feed.acme/data", "pending", at(3), nil)
	insertFeedRequest(t, db, "acc://"+tag+"-5@feed.acme/data", "processing", at(3), nil)
	insertFeedRequest(t, db, "acc://"+tag+"-6@feed.acme/data", "batched", at(3), nil)

	// Same instant: the request id breaks the tie.
	second, third := failed, cancelled
	if strings.Compare(third.String(), second.String()) < 0 {
		second, third = third, second
	}
	want := []uuid.UUID{completed, second, third}

	var got []TerminalRequest
	query := url.Values{"since": {base.Format(time.RFC3339Nano)}, "limit": {"2"}}
	code, page, body := getFeed(t, h, query, nil)
	if code != http.StatusOK {
		t.Fatalf("first page: %d %s", code, body)
	}
	if len(page.Requests) != 2 || page.Next == "" || page.Next != page.Cursor {
		t.Fatalf("a full page must name the next one: %+v", page)
	}
	got = append(got, page.Requests...)

	code, page, body = getFeed(t, h, url.Values{"after": {page.Next}, "limit": {"1"}}, nil)
	if code != http.StatusOK || len(page.Requests) != 1 {
		t.Fatalf("second page: %d %s", code, body)
	}
	got = append(got, page.Requests...)

	for i, id := range want {
		if got[i].RequestID != id {
			t.Fatalf("position %d: got %s want %s (all: %+v)", i, got[i].RequestID, id, got)
		}
	}
	if got[0].Status != "completed" || got[0].ProofID == nil || *got[0].ProofID != artifact.ProofID.String() ||
		got[0].AccumTxHash == nil || *got[0].AccumTxHash != "acc://"+tag+"-1@feed.acme/data" || !got[0].CompletedAt.Equal(*at(1)) {
		t.Fatalf("the completed request is not served as stored: %+v", got[0])
	}
	statuses := map[string]bool{got[1].Status: true, got[2].Status: true}
	if !statuses["failed"] || !statuses["cancelled"] || got[1].ProofID != nil || got[2].ProofID != nil {
		t.Fatalf("failed and cancelled requests are not served: %+v", got[1:])
	}

	// A page past everything is empty, has no next page, and hands back the position asked for.
	future := time.Now().Add(time.Hour).UTC()
	code, page, body = getFeed(t, h, url.Values{"since": {future.Format(time.RFC3339Nano)}}, nil)
	if code != http.StatusOK || len(page.Requests) != 0 || page.Next != "" || page.Cursor == "" {
		t.Fatalf("an empty page: %d %s", code, body)
	}
	position, err := decodeRequestCursor(page.Cursor)
	if err != nil || !position.EndedAt.Equal(future) || position.RequestID != uuid.Nil {
		t.Fatalf("an empty page must hand back the position asked for: %+v %v", position, err)
	}
}

// A request ends through the validators' writer: it is served once its stamp has settled, never before,
// and a failure - which the writer does not stamp - is counted as unplaceable instead of dropped silently.
func TestTerminalRequestFeedFollowsTheWriter(t *testing.T) {
	db, repos := recordsDB(t)
	ctx := context.Background()
	h := NewBundleHandlers(repos, nil, log.New(io.Discard, "", 0))
	tag := uuid.NewString()

	artifact, err := repos.ProofArtifacts.CreateProofArtifact(ctx, &database.NewProofArtifact{
		ProofType: database.ProofTypeCertenAnchor, AccumTxHash: "feed-writer-" + tag, AccountURL: "acc://feed.acme/data",
		ProofClass: database.ProofClassOnDemand, ValidatorID: "feed-test", ArtifactJSON: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	newRequest := func(suffix string) *database.ProofRequest {
		request, err := repos.Requests.CreateRequest(ctx, &database.NewProofRequest{
			AccumTxHash: "acc://" + tag + suffix + "@feed.acme/data", RequestType: database.RequestTypeOnDemand,
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM proof_requests WHERE request_id = $1`, request.RequestID)
		})
		if err := repos.Requests.MarkProcessing(ctx, request.RequestID); err != nil {
			t.Fatal(err)
		}
		return request
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM proof_artifacts WHERE proof_id = $1`, artifact.ProofID)
	})

	since := url.Values{"since": {time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)}, "limit": {"500"}}
	served := func() map[uuid.UUID]TerminalRequest {
		t.Helper()
		out := map[uuid.UUID]TerminalRequest{}
		query := since
		for pages := 0; pages < 100; pages++ {
			code, page, body := getFeed(t, h, query, nil)
			if code != http.StatusOK {
				t.Fatalf("feed: %d %s", code, body)
			}
			for _, r := range page.Requests {
				out[r.RequestID] = r
			}
			if page.Next == "" {
				return out
			}
			query = url.Values{"after": {page.Next}, "limit": {"500"}}
		}
		t.Fatal("the feed did not end")
		return nil
	}
	_, before, _ := getFeed(t, h, since, nil)

	done := newRequest("-done")
	if err := repos.Requests.MarkCompleted(ctx, done.RequestID, artifact.ProofID); err != nil {
		t.Fatal(err)
	}
	broken := newRequest("-broken")
	if err := repos.Requests.MarkFailed(ctx, broken.RequestID, "no proof within 30m"); err != nil {
		t.Fatal(err)
	}

	if _, ok := served()[done.RequestID]; ok {
		t.Fatal("a request was served before its completion had settled; a cursor could pass an earlier stamp still committing")
	}
	// Let the stamp age past the settle window, as time would.
	if _, err := db.ExecContext(ctx, `UPDATE proof_requests SET completed_at = completed_at - interval '6 seconds' WHERE request_id = $1`, done.RequestID); err != nil {
		t.Fatal(err)
	}
	got, ok := served()[done.RequestID]
	if !ok || got.Status != "completed" || got.ProofID == nil || *got.ProofID != artifact.ProofID.String() {
		t.Fatalf("a completed request was not served once settled: %+v %v", got, ok)
	}
	if _, ok := served()[broken.RequestID]; ok {
		t.Fatal("a failure without an end time cannot be placed on the feed, yet it was served")
	}
	_, after, _ := getFeed(t, h, since, nil)
	if after.TerminalWithoutEndTime != before.TerminalWithoutEndTime+1 {
		t.Fatalf("the failure the writer did not stamp is not counted: before %d, after %d", before.TerminalWithoutEndTime, after.TerminalWithoutEndTime)
	}
}

func TestTerminalRequestFeedRefusesByName(t *testing.T) {
	_, repos := recordsDB(t)
	h := NewBundleHandlers(repos, nil, log.New(io.Discard, "", 0))
	valid := encodeRequestCursor(database.RequestPosition{EndedAt: time.Now(), RequestID: uuid.New()})

	cases := []struct {
		name      string
		query     url.Values
		principal *Principal
		status    int
		code      string
	}{
		{"not base64", url.Values{"after": {"%%%"}}, nil, http.StatusBadRequest, "INVALID_CURSOR"},
		{"not a cursor", url.Values{"after": {"aGVsbG8"}}, nil, http.StatusBadRequest, "INVALID_CURSOR"},
		{"bad time in cursor", url.Values{"after": {"djF8bm90LWEtdGltZXwwMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDA"}}, nil, http.StatusBadRequest, "INVALID_CURSOR"},
		{"both positions", url.Values{"after": {valid}, "since": {"2026-01-01T00:00:00Z"}}, nil, http.StatusBadRequest, "CONFLICTING_POSITION"},
		{"bad since", url.Values{"since": {"yesterday"}}, nil, http.StatusBadRequest, "INVALID_SINCE"},
		{"zero limit", url.Values{"limit": {"0"}}, nil, http.StatusBadRequest, "INVALID_LIMIT"},
		{"limit over max", url.Values{"limit": {"501"}}, nil, http.StatusBadRequest, "INVALID_LIMIT"},
		{"limit not a number", url.Values{"limit": {"ten"}}, nil, http.StatusBadRequest, "INVALID_LIMIT"},
		{"user principal", url.Values{}, &Principal{Type: PrincipalUser, UID: "u1"}, http.StatusForbidden, "FORBIDDEN"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _, body := getFeed(t, h, c.query, c.principal)
			if code != c.status || !strings.Contains(body, `"`+c.code+`"`) {
				t.Fatalf("got %d %s, want %d %s", code, body, c.status, c.code)
			}
		})
	}

	rec := httptest.NewRecorder()
	h.HandleTerminalRequests(rec, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/requests/completed", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rec.Code)
	}
	code, _, body := getFeed(t, h, url.Values{"after": {valid}, "limit": {"500"}}, &Principal{Type: PrincipalService})
	if code != http.StatusOK {
		t.Fatalf("a service caller with a valid cursor: %d %s", code, body)
	}
}
