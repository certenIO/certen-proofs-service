package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

type fakeSummaryStore struct {
	exists   bool
	existsEr error
	heads    []database.FinalityAddendumHead
	headsEr  error
	statuses []database.FinalityStatus
	statusEr error
	asked    []uuid.UUID
}

func (f *fakeSummaryStore) ProofExists(_ context.Context, id uuid.UUID) (bool, error) {
	f.asked = append(f.asked, id)
	return f.exists, f.existsEr
}
func (f *fakeSummaryStore) ListFinalityAddendumHeads(context.Context, uuid.UUID) ([]database.FinalityAddendumHead, error) {
	return f.heads, f.headsEr
}
func (f *fakeSummaryStore) ListFinalityStatus(context.Context, uuid.UUID) ([]database.FinalityStatus, error) {
	return f.statuses, f.statusEr
}

var (
	created = time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC)
	judged  = time.Date(2026, 10, 20, 10, 15, 0, 0, time.UTC)
)

func headOf(proof uuid.UUID, seq int64, level int16, chain int64, kind string, number uint64, hashByte byte) database.FinalityAddendumHead {
	ev := cfe1(kind, uint64(chain), byte(level), number, hashByte)
	prefix := ev
	if len(prefix) > database.FinalityHeaderBytes {
		prefix = prefix[:database.FinalityHeaderBytes]
	}
	return database.FinalityAddendumHead{ProofID: proof, Seq: seq, Level: level, Chain: chain, Kind: kind, Header: prefix, EvidenceLen: int64(len(ev)) + 1000, CreatedAt: created}
}

func statusRow(proof uuid.UUID, chain int64, level int16, doc string) database.FinalityStatus {
	return database.FinalityStatus{ProofID: proof, Chain: chain, TargetLevel: level, State: []byte(doc), JudgedAt: judged}
}

const (
	pendingDoc = `{"format":"certen-finality-status/1","class":"pending","levelReached":"l1_posted","nextLevel":"finalized","eta":"2026-10-25T23:06:00Z","state":{"code":"finality_pending","fields":{"chain":"84532","reason":"within_finality_delay"}}}`
	naDoc      = `{"format":"certen-finality-status/1","class":"not_applicable","levelReached":"included","nextLevel":"l1_posted","state":{"code":"level_not_applicable","fields":{"chain":"11155111","level":"l1_posted","reason":"this chain is the L1"}}}`
)

func summaryJSON(t *testing.T, s *FinalitySummary, err error) string {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	b, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

func TestSummaryOfAProofWithNothingRecorded(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	s, err := SummarizeFinality(proof, nil, nil)
	want := `{"format":"certen-finality-summary/1","proofId":"01234567-89ab-4def-8012-3456789abcde","recordedLevel":"included","leg":null,` +
		`"levels":{"included":{"state":"derived","basis":"derived from the proof's own settlement evidence (S6); never stored as an addendum and not checked by this service"},` +
		`"l1_posted":{"state":"not_judged"},"finalized":{"state":"not_judged"}}}`
	if got := summaryJSON(t, s, err); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestSummaryOfBaseWithL1PostedStoredAndFinalizedPending(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	heads := []database.FinalityAddendumHead{headOf(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", 47368146, 0x33)}
	statuses := []database.FinalityStatus{statusRow(proof, 84532, 3, pendingDoc)}
	s, err := SummarizeFinality(proof, heads, statuses)
	want := `{"format":"certen-finality-summary/1","proofId":"01234567-89ab-4def-8012-3456789abcde","recordedLevel":"l1_posted",` +
		`"leg":{"chain":84532,"blockNumber":47368146,"blockHash":"3333333333333333333333333333333333333333333333333333333333333333"},` +
		`"levels":{"included":{"state":"derived","basis":"derived from the proof's own settlement evidence (S6); never stored as an addendum and not checked by this service"},` +
		`"l1_posted":{"state":"present","chain":84532,"kind":"base-sepolia/l1_posted/v1","seq":1,"createdAt":"2026-10-20T09:30:00Z"},` +
		`"finalized":{"state":"pending","chain":84532,"levelReached":"l1_posted","nextLevel":"finalized","eta":"2026-10-25T23:06:00Z",` +
		`"reason":{"code":"finality_pending","fields":{"chain":"84532","reason":"within_finality_delay"}},"judgedAt":"2026-10-20T10:15:00Z"}}}`
	if got := summaryJSON(t, s, err); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestSummaryOfSepoliaWithL1PostedNotApplicableAndFinalizedStored(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	heads := []database.FinalityAddendumHead{headOf(proof, 1, 3, 11155111, "eth-sepolia/finalized/v1", 11845679, 0x22)}
	statuses := []database.FinalityStatus{statusRow(proof, 11155111, 2, naDoc)}
	s, err := SummarizeFinality(proof, heads, statuses)
	want := `{"format":"certen-finality-summary/1","proofId":"01234567-89ab-4def-8012-3456789abcde","recordedLevel":"finalized",` +
		`"leg":{"chain":11155111,"blockNumber":11845679,"blockHash":"2222222222222222222222222222222222222222222222222222222222222222"},` +
		`"levels":{"included":{"state":"derived","basis":"derived from the proof's own settlement evidence (S6); never stored as an addendum and not checked by this service"},` +
		`"l1_posted":{"state":"not_applicable","chain":11155111,"levelReached":"included","nextLevel":"l1_posted",` +
		`"reason":{"code":"level_not_applicable","fields":{"chain":"11155111","level":"l1_posted","reason":"this chain is the L1"}},"judgedAt":"2026-10-20T10:15:00Z"},` +
		`"finalized":{"state":"present","chain":11155111,"kind":"eth-sepolia/finalized/v1","seq":1,"createdAt":"2026-10-20T09:30:00Z"}}}`
	if got := summaryJSON(t, s, err); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

func TestLevelsAreIndependentAndInAnyOrder(t *testing.T) {
	proof := uuid.New()
	finalizedFirst := []database.FinalityAddendumHead{
		headOf(proof, 1, 3, 84532, "base-sepolia/finalized/v1", 7, 0x33),
		headOf(proof, 2, 2, 84532, "base-sepolia/l1_posted/v1", 7, 0x33),
	}
	s, err := SummarizeFinality(proof, finalizedFirst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.RecordedLevel != "finalized" || s.Levels.L1Posted.State != LevelStatePresent || s.Levels.Finalized.State != LevelStatePresent ||
		s.Levels.L1Posted.Seq != 2 || s.Levels.Finalized.Seq != 1 {
		t.Fatalf("%+v", s)
	}
	// finalized without l1_posted (blobs expired): finalized is recorded, l1_posted is not claimed
	s, err = SummarizeFinality(proof, finalizedFirst[:1], nil)
	if err != nil || s.RecordedLevel != "finalized" || s.Levels.L1Posted.State != LevelStateNotJudged {
		t.Fatalf("%+v %v", s, err)
	}
	// l1_posted alone records l1_posted
	s, err = SummarizeFinality(proof, finalizedFirst[1:], nil)
	if err != nil || s.RecordedLevel != "l1_posted" || s.Levels.Finalized.State != LevelStateNotJudged {
		t.Fatalf("%+v %v", s, err)
	}
	// l1_posted first then finalized
	s, err = SummarizeFinality(proof, []database.FinalityAddendumHead{finalizedFirst[1], finalizedFirst[0]}, nil)
	if err != nil || s.RecordedLevel != "finalized" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestAStoredLevelIsPresentWhateverTheStatusRowStillSays(t *testing.T) {
	proof := uuid.New()
	heads := []database.FinalityAddendumHead{headOf(proof, 1, 3, 84532, "base-sepolia/finalized/v1", 7, 0x33)}
	stale := statusRow(proof, 84532, 3, pendingDoc)
	s, err := SummarizeFinality(proof, heads, []database.FinalityStatus{stale})
	if err != nil {
		t.Fatal(err)
	}
	f := s.Levels.Finalized
	if f.State != LevelStatePresent || f.ETA != "" || f.Reason != nil || f.NextLevel != "" || f.JudgedAt != "" || f.Kind != "base-sepolia/finalized/v1" {
		t.Fatalf("%+v", f)
	}
}

func TestEveryBuilderClassIsReportedByItsOwnName(t *testing.T) {
	proof := uuid.New()
	doc := func(class string) string {
		return `{"format":"certen-finality-status/1","class":"` + class + `","levelReached":"included","nextLevel":"finalized","state":{"code":"c","fields":{}}}`
	}
	for class, want := range map[string]string{"pending": LevelStatePending, "unavailable": LevelStateUnavailable, "refused": LevelStateRefused, "not_applicable": LevelStateNotApplicable} {
		for _, level := range []int16{2, 3} {
			s, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, level, doc(class))})
			if err != nil {
				t.Fatal(err)
			}
			got, other := s.Levels.L1Posted, s.Levels.Finalized
			if level == 3 {
				got, other = other, got
			}
			if got.State != want || got.Chain != 84532 || string(got.Reason) != `{"code":"c","fields":{}}` || got.LevelReached != "included" || got.NextLevel != "finalized" || other.State != LevelStateNotJudged {
				t.Errorf("%s at level %d: %+v / %+v", class, level, got, other)
			}
		}
	}
	// the builder says it stored a level but there is no addendum row: named, never shown as present
	verified := `{"format":"certen-finality-status/1","class":"verified","levelReached":"finalized","nextLevel":"finalized","seq":2}`
	s, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, verified)})
	if err != nil || s.Levels.Finalized.State != LevelStateVerifiedNoRow || s.Levels.Finalized.Seq != 2 || s.Levels.Finalized.Reason != nil || s.RecordedLevel != "included" {
		t.Fatalf("%+v %v", s, err)
	}
	// no eta stated stays unstated
	if s, _ := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc("pending"))}); s.Levels.Finalized.ETA != "" {
		t.Fatalf("eta %q", s.Levels.Finalized.ETA)
	}
}

func TestSummaryRefusesRowsItCannotSummariseByName(t *testing.T) {
	proof := uuid.New()
	ok := func() database.FinalityAddendumHead {
		return headOf(proof, 1, 3, 84532, "base-sepolia/finalized/v1", 7, 0x33)
	}
	type tc struct {
		name   string
		heads  []database.FinalityAddendumHead
		status []database.FinalityStatus
		code   string
		k, v   string
	}
	otherLeg := headOf(proof, 2, 2, 84532, "base-sepolia/l1_posted/v1", 8, 0x33)
	badHeader := ok()
	badHeader.Header = []byte("CFE2....")
	otherProof := ok()
	otherProof.ProofID = uuid.New()
	level1 := ok()
	level1.Level = 1
	repeated := headOf(proof, 2, 3, 84532, "base-sepolia/finalized/v1", 7, 0x33)
	kindMismatch := ok()
	kindMismatch.Kind = "base-sepolia/l1_posted/v1"
	st := func(level int16, doc string) []database.FinalityStatus {
		return []database.FinalityStatus{statusRow(proof, 84532, level, doc)}
	}
	cases := []tc{
		{"two legs", []database.FinalityAddendumHead{ok(), otherLeg}, nil, FinalityLegMismatch, "seq", "2"},
		{"header not CFE1", []database.FinalityAddendumHead{badHeader}, nil, FinalityHeaderMalformed, "reason", "magic"},
		{"header disagrees with its row", []database.FinalityAddendumHead{kindMismatch}, nil, FinalityHeaderMismatch, "field", "kind"},
		{"another proof's row", []database.FinalityAddendumHead{otherProof}, nil, FinalityRowInvalid, "field", "proofId"},
		{"level 1 row", []database.FinalityAddendumHead{level1}, nil, FinalityRowInvalid, "field", "level"},
		{"a level twice", []database.FinalityAddendumHead{ok(), repeated}, nil, FinalityRowInvalid, "field", "level_repeated"},
		{"status not json", nil, st(3, `[`), FinalityStatusMalformed, "reason", "not_a_status_document"},
		{"status an array", nil, st(3, `[]`), FinalityStatusMalformed, "reason", "not_a_status_document"},
		{"status unknown field", nil, st(3, `{"format":"certen-finality-status/1","class":"pending","levelReached":"none","nextLevel":"finalized","state":{},"extra":1}`), FinalityStatusMalformed, "reason", "not_a_status_document"},
		{"status wrong format", nil, st(3, `{"format":"certen-finality-status/2","class":"pending","levelReached":"none","nextLevel":"finalized","state":{}}`), FinalityStatusMalformed, "reason", "wrong_format"},
		{"status unknown class", nil, st(3, `{"format":"certen-finality-status/1","class":"verifiedish","levelReached":"none","nextLevel":"finalized","state":{}}`), FinalityStatusMalformed, "reason", "unknown_class"},
		{"status unknown level reached", nil, st(3, `{"format":"certen-finality-status/1","class":"pending","levelReached":"final","nextLevel":"finalized","state":{}}`), FinalityStatusMalformed, "reason", "unknown_level"},
		{"status unknown next level", nil, st(3, `{"format":"certen-finality-status/1","class":"pending","levelReached":"none","nextLevel":"later","state":{}}`), FinalityStatusMalformed, "reason", "unknown_level"},
		{"status bad eta", nil, st(3, `{"format":"certen-finality-status/1","class":"pending","levelReached":"none","nextLevel":"finalized","eta":"tomorrow","state":{}}`), FinalityStatusMalformed, "reason", "bad_eta"},
		{"verified with a state", nil, st(3, `{"format":"certen-finality-status/1","class":"verified","levelReached":"finalized","nextLevel":"finalized","state":{}}`), FinalityStatusMalformed, "reason", "verified_carries_state"},
		{"pending without a state", nil, st(3, `{"format":"certen-finality-status/1","class":"pending","levelReached":"none","nextLevel":"finalized"}`), FinalityStatusMalformed, "reason", "state_missing"},
		{"pending with a null state", nil, st(3, `{"format":"certen-finality-status/1","class":"pending","levelReached":"none","nextLevel":"finalized","state":null}`), FinalityStatusMalformed, "reason", "state_missing"},
		{"state not an object", nil, st(2, `{"format":"certen-finality-status/1","class":"pending","levelReached":"none","nextLevel":"l1_posted","state":[1]}`), FinalityStatusMalformed, "reason", "state_not_an_object"},
		{"status of level 1", nil, st(1, pendingDoc), FinalityStatusMalformed, "reason", "target_level"},
		{"status of another proof", nil, []database.FinalityStatus{statusRow(uuid.New(), 84532, 3, pendingDoc)}, FinalityStatusMalformed, "reason", "proof_id"},
	}
	for _, c := range cases {
		s, err := SummarizeFinality(proof, c.heads, c.status)
		var named *FinalityComposeError
		if err == nil || !errors.As(err, &named) || named.Code != c.code || named.Fields[c.k] != c.v || s != nil {
			t.Errorf("%s: %+v, %v", c.name, s, err)
		}
	}
	// the level a malformed status concerns is named
	_, err := SummarizeFinality(proof, nil, st(2, `[`))
	var named *FinalityComposeError
	if !errors.As(err, &named) || named.Fields["level"] != "l1_posted" {
		t.Fatalf("%v", err)
	}
	_, err = SummarizeFinality(proof, nil, st(3, `[`))
	if !errors.As(err, &named) || named.Fields["level"] != "finalized" {
		t.Fatalf("%v", err)
	}
}

func getFinality(h *FinalityHandlers, id string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.HandleGetProofFinality(rr, httptest.NewRequest(http.MethodGet, "/api/v1/proofs/"+id+"/finality", nil))
	return rr
}

func TestFinalityEndpointStatusCodes(t *testing.T) {
	proof := uuid.New()
	store := &fakeSummaryStore{exists: true,
		heads:    []database.FinalityAddendumHead{headOf(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", 7, 0x33)},
		statuses: []database.FinalityStatus{statusRow(proof, 84532, 3, pendingDoc)}}
	h := NewFinalityHandlers(store, nil)
	rr := getFinality(h, proof.String())
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" || !strings.Contains(rr.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("%d %v", rr.Code, rr.Header())
	}
	if len(store.asked) != 1 || store.asked[0] != proof {
		t.Fatalf("looked up %v", store.asked)
	}
	if !strings.Contains(rr.Body.String(), `"recordedLevel":"l1_posted"`) || strings.Contains(strings.ToLower(rr.Body.String()), "verified") {
		t.Fatalf("body %s", rr.Body)
	}

	// 404: no such proof; never an empty summary
	rr = getFinality(NewFinalityHandlers(&fakeSummaryStore{}, nil), proof.String())
	if rr.Code != http.StatusNotFound || !strings.Contains(rr.Body.String(), "PROOF_NOT_FOUND") {
		t.Fatalf("unknown proof: %d %s", rr.Code, rr.Body)
	}
	// a proof that exists with nothing recorded is a 200 with named not_judged levels
	rr = getFinality(NewFinalityHandlers(&fakeSummaryStore{exists: true}, nil), proof.String())
	if rr.Code != http.StatusOK || strings.Count(rr.Body.String(), `"not_judged"`) != 2 {
		t.Fatalf("nothing recorded: %d %s", rr.Code, rr.Body)
	}
	// 500: every read that fails, and rows that contradict each other
	for name, s := range map[string]*fakeSummaryStore{
		"proof lookup":  {existsEr: errors.New("db down")},
		"addendum read": {exists: true, headsEr: errors.New("db down")},
		"status read":   {exists: true, statusEr: errors.New("db down")},
	} {
		rr := getFinality(NewFinalityHandlers(s, nil), proof.String())
		if rr.Code != http.StatusInternalServerError || strings.Contains(rr.Body.String(), "NOT_FOUND") || !strings.Contains(rr.Body.String(), "INTERNAL_ERROR") {
			t.Errorf("%s: %d %s", name, rr.Code, rr.Body)
		}
	}
	bad := &fakeSummaryStore{exists: true, statuses: []database.FinalityStatus{statusRow(proof, 84532, 3, `[`)}}
	rr = getFinality(NewFinalityHandlers(bad, nil), proof.String())
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "FINALITY_SUMMARY_UNUSABLE") || !strings.Contains(rr.Body.String(), FinalityStatusMalformed) {
		t.Fatalf("a malformed status row: %d %s", rr.Code, rr.Body)
	}
	two := &fakeSummaryStore{exists: true, heads: []database.FinalityAddendumHead{
		headOf(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", 7, 0x33), headOf(proof, 2, 3, 84532, "base-sepolia/finalized/v1", 8, 0x33)}}
	rr = getFinality(NewFinalityHandlers(two, nil), proof.String())
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), FinalityLegMismatch) {
		t.Fatalf("two legs: %d %s", rr.Code, rr.Body)
	}
}

func TestFinalityEndpointRefusesWhatIsNotARequestForIt(t *testing.T) {
	h := NewFinalityHandlers(&fakeSummaryStore{exists: true}, nil)
	if rr := getFinality(h, "not-a-uuid"); rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "INVALID_PROOF_ID") {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
	rr := httptest.NewRecorder()
	h.HandleGetProofFinality(rr, httptest.NewRequest(http.MethodPost, "/api/v1/proofs/"+uuid.NewString()+"/finality", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", rr.Code)
	}
	for _, path := range []string{"/api/v1/proofs/" + uuid.NewString() + "/v2", "/api/v1/proofs/" + uuid.NewString() + "/finality/extra", "/api/v1/proofs/finality"} {
		rr := httptest.NewRecorder()
		h.HandleGetProofFinality(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "INVALID_PATH") {
			t.Errorf("%s: %d %s", path, rr.Code, rr.Body)
		}
	}
}
