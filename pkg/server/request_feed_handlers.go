// Copyright 2025 Certen Protocol
//
// The terminal-request feed: GET /api/v1/proofs/requests/completed
//
// A proof request ends in the proof database, written by the validators; nothing tells the caller that
// asked for it. The gateway's status poller backs a waiting intent off to 15 minutes, so it learned of a
// finished proof up to 15 minutes late. This feed lets it ask once per tick what has ended since it last
// asked, at a cost that does not grow with the number of intents waiting.
//
// An intent on several chains has ONE proof request, which ends with the first chain member's proof; each later
// member lands without any request ending (RB7, intent f6f19875: the request ended at 21:27:12 with the Base leg, the
// Arbitrum leg landed at 22:07:10, and the gateway completed 3.5 minutes after it, on its backoff). So the feed also
// serves every chain member's recorded outcome, under `members`, from its own position in the same cursor.

package server

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

const (
	terminalFeedDefaultLimit = 100
	terminalFeedMaxLimit     = 500
	requestCursorVersion     = "v1"
	feedCursorVersion        = "v2"
)

// TerminalRequest is one request on the feed.
type TerminalRequest struct {
	RequestID   uuid.UUID `json:"request_id"`
	AccumTxHash *string   `json:"accum_tx_hash"` // as the requester sent it; null for an account-only request
	ProofID     *string   `json:"proof_id"`      // null unless completed
	Status      string    `json:"status"`
	CompletedAt time.Time `json:"completed_at"`
}

// RecordedMember is one chain member's outcome as the validators recorded it: one per chain an intent settles on,
// served again each time it is recorded anew.
type RecordedMember struct {
	IntentID    string    `json:"intent_id"`
	ChainID     int64     `json:"chain_id"`
	AccumTxHash *string   `json:"accum_tx_hash"` // the intent's; null when the member has no lifecycle row
	Settlement  string    `json:"settlement"`
	ProofCycle  string    `json:"proof_cycle"`
	ProofID     *string   `json:"proof_id"` // the proof the member's recording cycle produced; null when it has none
	RecordedAt  time.Time `json:"recorded_at"`
}

// TerminalRequestsResponse is a page of the feed.
type TerminalRequestsResponse struct {
	Requests []TerminalRequest `json:"requests"`
	// Members are the chain-member outcomes recorded after the cursor's member position, in (recorded_at, intent_id,
	// chain_id) order, at most `limit` of them.
	Members []RecordedMember `json:"members"`
	// Next is the cursor of the following page, empty when this page was the last one there is now: set when
	// either list filled the page.
	Next string `json:"next"`
	// Cursor is the position after this page (the position asked for, when the page is empty). A caller
	// keeps it to ask again later, whether or not Next was empty.
	Cursor string `json:"cursor"`
	// TerminalWithoutEndTime counts requests in a terminal status that carry no completed_at, which the
	// feed cannot place in time and so does not serve. The validators do not stamp a failure's time today.
	TerminalWithoutEndTime int64 `json:"terminal_without_end_time"`
}

var errMalformedCursor = errors.New("malformed cursor")

// feedPosition is a place in the feed: one position in the requests, one in the members.
type feedPosition struct {
	Request database.RequestPosition
	Member  database.MemberPosition
}

// positionAt is the place just before everything that ended at or after t, in both lists.
func positionAt(t time.Time) feedPosition {
	return feedPosition{Request: database.RequestPosition{EndedAt: t}, Member: database.MemberPosition{RecordedAt: t}}
}

// encodeFeedCursor makes the opaque cursor for a feed position. The intent id is encoded on its own, so no character
// it may hold can be read as a separator.
func encodeFeedCursor(p feedPosition) string {
	raw := strings.Join([]string{feedCursorVersion,
		p.Request.EndedAt.UTC().Format(time.RFC3339Nano), p.Request.RequestID.String(),
		p.Member.RecordedAt.UTC().Format(time.RFC3339Nano), base64.RawURLEncoding.EncodeToString([]byte(p.Member.IntentID)),
		strconv.FormatInt(p.Member.ChainID, 10)}, "|")
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeFeedCursor reads a cursor made by encodeFeedCursor, or a v1 cursor this feed issued before it served members:
// its request position, with the member position at the same instant (the members recorded from then on).
func decodeFeedCursor(cursor string) (feedPosition, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return feedPosition{}, errMalformedCursor
	}
	parts := strings.Split(string(raw), "|")
	switch {
	case len(parts) == 3 && parts[0] == requestCursorVersion:
		request, err := parseRequestPosition(parts[1], parts[2])
		if err != nil {
			return feedPosition{}, err
		}
		return feedPosition{Request: request, Member: database.MemberPosition{RecordedAt: request.EndedAt}}, nil
	case len(parts) == 6 && parts[0] == feedCursorVersion:
		request, err := parseRequestPosition(parts[1], parts[2])
		if err != nil {
			return feedPosition{}, err
		}
		recordedAt, err := time.Parse(time.RFC3339Nano, parts[3])
		if err != nil {
			return feedPosition{}, errMalformedCursor
		}
		intentID, err := base64.RawURLEncoding.DecodeString(parts[4])
		if err != nil {
			return feedPosition{}, errMalformedCursor
		}
		chainID, err := strconv.ParseInt(parts[5], 10, 64)
		if err != nil {
			return feedPosition{}, errMalformedCursor
		}
		return feedPosition{Request: request, Member: database.MemberPosition{RecordedAt: recordedAt, IntentID: string(intentID), ChainID: chainID}}, nil
	default:
		return feedPosition{}, errMalformedCursor
	}
}

func parseRequestPosition(endedAtText, requestIDText string) (database.RequestPosition, error) {
	endedAt, err := time.Parse(time.RFC3339Nano, endedAtText)
	if err != nil {
		return database.RequestPosition{}, errMalformedCursor
	}
	requestID, err := uuid.Parse(requestIDText)
	if err != nil {
		return database.RequestPosition{}, errMalformedCursor
	}
	return database.RequestPosition{EndedAt: endedAt, RequestID: requestID}, nil
}

// HandleTerminalRequests handles GET /api/v1/proofs/requests/completed?after=<cursor>&limit=<n>
//
// It serves the proof requests that reached a terminal status ('completed', 'failed', 'cancelled') after
// the position given, in (completed_at, request_id) order, and under `members` the chain-member outcomes the
// validators recorded after it, in (recorded_at, intent_id, chain_id) order. The position is `after`, a cursor
// from an earlier page, or `since`, an RFC 3339 time to start from when the caller holds no cursor yet; with
// neither, the feed starts at the beginning. `limit` is 1-500, 100 when absent, and bounds each list.
//
// The feed spans every requester, so it is served to service callers only: a user principal is refused.
func (h *BundleHandlers) HandleTerminalRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}
	if p, ok := PrincipalFrom(r.Context()); ok && p.Type != PrincipalService {
		h.writeError(w, http.StatusForbidden, "FORBIDDEN", "the terminal-request feed spans every requester and is served to service callers only")
		return
	}

	query := r.URL.Query()
	limit := terminalFeedDefaultLimit
	if raw := query.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > terminalFeedMaxLimit {
			h.writeError(w, http.StatusBadRequest, "INVALID_LIMIT", fmt.Sprintf("limit must be an integer from 1 to %d", terminalFeedMaxLimit))
			return
		}
		limit = n
	}

	after, since := query.Get("after"), query.Get("since")
	var position feedPosition
	switch {
	case after != "" && since != "":
		h.writeError(w, http.StatusBadRequest, "CONFLICTING_POSITION", "give either after (a cursor) or since (a time), not both")
		return
	case after != "":
		p, err := decodeFeedCursor(after)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "INVALID_CURSOR", "after is not a cursor this feed issued")
			return
		}
		position = p
	case since != "":
		t, err := time.Parse(time.RFC3339Nano, since)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "INVALID_SINCE", "since must be an RFC 3339 time")
			return
		}
		position = positionAt(t)
	default:
		position = positionAt(time.Unix(0, 0).UTC())
	}

	if h.repos == nil || h.repos.Requests == nil || h.repos.IntentLifecycle == nil {
		h.writeError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "the proof database is not connected")
		return
	}
	ctx := r.Context()
	rows, err := h.repos.Requests.GetTerminalRequestsAfter(ctx, position.Request, limit)
	if err != nil {
		h.logger.Printf("Error reading terminal proof requests: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read terminal proof requests")
		return
	}
	members, err := h.repos.IntentLifecycle.GetMembersRecordedAfter(ctx, position.Member, limit)
	if err != nil {
		h.logger.Printf("Error reading recorded chain members: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read recorded chain members")
		return
	}
	unplaced, err := h.repos.Requests.CountTerminalWithoutEndTime(ctx)
	if err != nil {
		h.logger.Printf("Error counting terminal proof requests without an end time: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read terminal proof requests")
		return
	}

	response := TerminalRequestsResponse{Requests: make([]TerminalRequest, 0, len(rows)), Members: make([]RecordedMember, 0, len(members)),
		TerminalWithoutEndTime: unplaced}
	for _, row := range rows {
		item := TerminalRequest{RequestID: row.RequestID, Status: string(row.Status), CompletedAt: row.CompletedAt.Time.UTC()}
		if row.AccumTxHash.Valid {
			hash := row.AccumTxHash.String
			item.AccumTxHash = &hash
		}
		if row.ProofID.Valid {
			id := row.ProofID.UUID.String()
			item.ProofID = &id
		}
		response.Requests = append(response.Requests, item)
		position.Request = database.RequestPosition{EndedAt: row.CompletedAt.Time, RequestID: row.RequestID}
	}
	for _, m := range members {
		item := RecordedMember{IntentID: m.IntentID, ChainID: m.ChainID, Settlement: m.Settlement, ProofCycle: m.ProofCycle,
			RecordedAt: m.RecordedAt.UTC()}
		if m.AccumTxHash.Valid {
			hash := m.AccumTxHash.String
			item.AccumTxHash = &hash
		}
		if m.ProofID.Valid {
			id := m.ProofID.String
			item.ProofID = &id
		}
		response.Members = append(response.Members, item)
		position.Member = database.MemberPosition{RecordedAt: m.RecordedAt, IntentID: m.IntentID, ChainID: m.ChainID}
	}
	response.Cursor = encodeFeedCursor(position)
	if len(rows) == limit || len(members) == limit {
		response.Next = response.Cursor
	}
	h.writeJSON(w, http.StatusOK, response)
}
