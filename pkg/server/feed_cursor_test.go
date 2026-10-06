package server

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// The feed cursor carries both positions; an intent id is opaque text and may hold the separator. A v1 cursor (issued
// before members were served) reads as its request position with the member position at the same instant. Anything
// else is refused.
func TestFeedCursorRoundTripsBothPositions(t *testing.T) {
	at := time.Date(2026, 10, 5, 22, 7, 10, 316660000, time.UTC)
	want := feedPosition{
		Request: database.RequestPosition{EndedAt: at.Add(-40 * time.Minute), RequestID: uuid.New()},
		Member:  database.MemberPosition{RecordedAt: at, IntentID: "f6f19875|odd=id", ChainID: 421614},
	}
	got, err := decodeFeedCursor(encodeFeedCursor(want))
	if err != nil || !got.Request.EndedAt.Equal(want.Request.EndedAt) || got.Request.RequestID != want.Request.RequestID ||
		!got.Member.RecordedAt.Equal(want.Member.RecordedAt) || got.Member.IntentID != want.Member.IntentID || got.Member.ChainID != want.Member.ChainID {
		t.Fatalf("round trip: got %+v %v, want %+v", got, err, want)
	}

	id := uuid.New()
	v1 := base64.RawURLEncoding.EncodeToString([]byte("v1|" + at.Format(time.RFC3339Nano) + "|" + id.String()))
	got, err = decodeFeedCursor(v1)
	if err != nil || !got.Request.EndedAt.Equal(at) || got.Request.RequestID != id || !got.Member.RecordedAt.Equal(at) ||
		got.Member.IntentID != "" || got.Member.ChainID != 0 {
		t.Fatalf("v1 cursor: got %+v %v", got, err)
	}

	for _, raw := range []string{
		"v2|" + at.Format(time.RFC3339Nano) + "|" + id.String() + "|" + at.Format(time.RFC3339Nano) + "|aWQ|notanumber",
		"v2|" + at.Format(time.RFC3339Nano) + "|" + id.String() + "|yesterday|aWQ|1",
		"v2|" + at.Format(time.RFC3339Nano) + "|" + id.String() + "|" + at.Format(time.RFC3339Nano) + "|%%%|1",
		"v2|" + at.Format(time.RFC3339Nano) + "|" + id.String(),
		"v3|" + at.Format(time.RFC3339Nano) + "|" + id.String() + "|" + at.Format(time.RFC3339Nano) + "|aWQ|1",
	} {
		if _, err := decodeFeedCursor(base64.RawURLEncoding.EncodeToString([]byte(raw))); err == nil {
			t.Fatalf("a malformed cursor was accepted: %q", raw)
		}
	}
}
