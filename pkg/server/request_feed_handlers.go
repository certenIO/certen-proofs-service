// Copyright 2025 Certen Protocol
//
// The terminal-request feed: GET /api/v1/proofs/requests/completed
//
// A proof request ends in the proof database, written by the validators; nothing tells the caller that
// asked for it. The gateway's status poller backs a waiting intent off to 15 minutes, so it learned of a
// finished proof up to 15 minutes late. This feed lets it ask once per tick what has ended since it last
// asked, at a cost that does not grow with the number of intents waiting.

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
)

// TerminalRequest is one request on the feed.
type TerminalRequest struct {
	RequestID   uuid.UUID `json:"request_id"`
	AccumTxHash *string   `json:"accum_tx_hash"` // as the requester sent it; null for an account-only request
	ProofID     *string   `json:"proof_id"`      // null unless completed
	Status      string    `json:"status"`
	CompletedAt time.Time `json:"completed_at"`
}

// TerminalRequestsResponse is a page of the feed.
type TerminalRequestsResponse struct {
	Requests []TerminalRequest `json:"requests"`
	// Next is the cursor of the following page, empty when this page was the last one there is now.
	Next string `json:"next"`
	// Cursor is the position after this page (the position asked for, when the page is empty). A caller
	// keeps it to ask again later, whether or not Next was empty.
	Cursor string `json:"cursor"`
	// TerminalWithoutEndTime counts requests in a terminal status that carry no completed_at, which the
	// feed cannot place in time and so does not serve. The validators do not stamp a failure's time today.
	TerminalWithoutEndTime int64 `json:"terminal_without_end_time"`
}

var errMalformedCursor = errors.New("malformed cursor")

// encodeRequestCursor makes the opaque cursor for a feed position.
func encodeRequestCursor(p database.RequestPosition) string {
	raw := requestCursorVersion + "|" + p.EndedAt.UTC().Format(time.RFC3339Nano) + "|" + p.RequestID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeRequestCursor reads a cursor made by encodeRequestCursor.
func decodeRequestCursor(cursor string) (database.RequestPosition, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return database.RequestPosition{}, errMalformedCursor
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 3 || parts[0] != requestCursorVersion {
		return database.RequestPosition{}, errMalformedCursor
	}
	endedAt, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil {
		return database.RequestPosition{}, errMalformedCursor
	}
	requestID, err := uuid.Parse(parts[2])
	if err != nil {
		return database.RequestPosition{}, errMalformedCursor
	}
	return database.RequestPosition{EndedAt: endedAt, RequestID: requestID}, nil
}

// HandleTerminalRequests handles GET /api/v1/proofs/requests/completed?after=<cursor>&limit=<n>
//
// It serves the proof requests that reached a terminal status ('completed', 'failed', 'cancelled') after
// the position given, in (completed_at, request_id) order. The position is `after`, a cursor from an
// earlier page, or `since`, an RFC 3339 time to start from when the caller holds no cursor yet; with
// neither, the feed starts at the beginning. `limit` is 1-500, 100 when absent.
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
	var position database.RequestPosition
	switch {
	case after != "" && since != "":
		h.writeError(w, http.StatusBadRequest, "CONFLICTING_POSITION", "give either after (a cursor) or since (a time), not both")
		return
	case after != "":
		p, err := decodeRequestCursor(after)
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
		position = database.RequestPosition{EndedAt: t}
	default:
		position = database.RequestPosition{EndedAt: time.Unix(0, 0).UTC()}
	}

	if h.repos == nil || h.repos.Requests == nil {
		h.writeError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "the proof database is not connected")
		return
	}
	ctx := r.Context()
	rows, err := h.repos.Requests.GetTerminalRequestsAfter(ctx, position, limit)
	if err != nil {
		h.logger.Printf("Error reading terminal proof requests: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read terminal proof requests")
		return
	}
	unplaced, err := h.repos.Requests.CountTerminalWithoutEndTime(ctx)
	if err != nil {
		h.logger.Printf("Error counting terminal proof requests without an end time: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read terminal proof requests")
		return
	}

	response := TerminalRequestsResponse{Requests: make([]TerminalRequest, 0, len(rows)), TerminalWithoutEndTime: unplaced}
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
		position = database.RequestPosition{EndedAt: row.CompletedAt.Time, RequestID: row.RequestID}
	}
	response.Cursor = encodeRequestCursor(position)
	if len(rows) == limit {
		response.Next = response.Cursor
	}
	h.writeJSON(w, http.StatusOK, response)
}
