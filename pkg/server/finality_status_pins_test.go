package server

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// The validators' builder status document (certen-finality-status/1) gains, additively, the top-level member pinsSha256 (the
// sha256 of the pins the builder judged under: validator pkg/finality/status.go, pv2/rb11-dbfix) and two fields of the named state,
// attempt and next_attempt_at (pv2/rb12-p0-live). The reader decodes the document strictly, so each of these has to be accepted
// BEFORE the validators write it, and a malformed one has to be refused by its own reason, not as "not_a_status_document".

func hexOf(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

var (
	pinsA = hexOf("certen finality pins A")
	pinsB = hexOf("certen finality pins B")
)

// withPins is a document with the pinsSha256 member added last, as the validator writes it.
func withPins(doc, pins string) string {
	return strings.TrimSuffix(doc, "}") + `,"pinsSha256":"` + pins + `"}`
}

const (
	// the state a back-off adds to an unavailable status: Canonical form, sorted keys, unsigned numbers as decimal strings
	attemptState = `{"code":"finality_build_timeout","fields":{"after":"60000000000","attempt":"3","cause":"appending the verified addendum: context deadline exceeded","chain":"84532","next_attempt_at":"2026-10-23T12:30:00Z"}}`
	attemptDoc   = `{"format":"certen-finality-status/1","class":"unavailable","levelReached":"l1_posted","nextLevel":"finalized","state":` + attemptState + `}`
	verifiedDoc  = `{"format":"certen-finality-status/1","class":"verified","levelReached":"l1_posted","nextLevel":"l1_posted","seq":1}`
	judgedAtJSON = `"judgedAt":"2026-10-20T10:15:00Z"`
	includedJSON = `"included":{"state":"derived","basis":"derived from the proof's own settlement evidence (S6); never stored as an addendum and not checked by this service"}`
)

func malformedReason(t *testing.T, err error) (level, reason string) {
	t.Helper()
	var named *FinalityComposeError
	if err == nil || !errors.As(err, &named) || named.Code != FinalityStatusMalformed {
		t.Fatalf("want %s, got %v", FinalityStatusMalformed, err)
	}
	return named.Fields["level"], named.Fields["reason"]
}

// A pending status row that carries pinsSha256 is summarised, and the pins identity is served beside the builder's judgement.
func TestStatusRowWithThePinsMemberIsSummarisedAndServesThePinsIdentity(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	heads := []database.FinalityAddendumHead{headOf(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", 47368146, 0x33)}
	statuses := []database.FinalityStatus{statusRow(proof, 84532, 3, withPins(pendingDoc, pinsA))}
	s, err := SummarizeFinality(proof, heads, statuses)
	want := `{"format":"certen-finality-summary/1","proofId":"01234567-89ab-4def-8012-3456789abcde","recordedLevel":"l1_posted",` +
		`"leg":{"chain":84532,"blockNumber":47368146,"blockHash":"3333333333333333333333333333333333333333333333333333333333333333"},` +
		`"levels":{` + includedJSON + `,` +
		`"l1_posted":{"state":"present","chain":84532,"kind":"base-sepolia/l1_posted/v1","seq":1,"createdAt":"2026-10-20T09:30:00Z"},` +
		`"finalized":{"state":"pending","chain":84532,"levelReached":"l1_posted","nextLevel":"finalized","eta":"2026-10-25T23:06:00Z",` +
		`"reason":{"code":"finality_pending","fields":{"chain":"84532","reason":"within_finality_delay"}},` + judgedAtJSON + `,"pinsSha256":"` + pinsA + `"}}}`
	if got := summaryJSON(t, s, err); got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
}

// A failed ask's status carries attempt and next_attempt_at in its named state: the state is served whole, with or without pins.
func TestStatusRowWithTheBackOffStateFieldsIsSummarised(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	for name, doc := range map[string]string{"without pins": attemptDoc, "with pins": withPins(attemptDoc, pinsB)} {
		s, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc)})
		wantPins := ""
		if name == "with pins" {
			wantPins = `,"pinsSha256":"` + pinsB + `"`
		}
		want := `{"format":"certen-finality-summary/1","proofId":"01234567-89ab-4def-8012-3456789abcde","recordedLevel":"included","leg":null,` +
			`"levels":{` + includedJSON + `,"l1_posted":{"state":"not_judged"},` +
			`"finalized":{"state":"unavailable","chain":84532,"levelReached":"l1_posted","nextLevel":"finalized","reason":` + attemptState + `,` + judgedAtJSON + wantPins + `}}}`
		if got := summaryJSON(t, s, err); got != want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
	}
}

// Old rows (no new member, no back-off fields) summarise byte for byte as before: the summary only grows a member where a row has one.
func TestRowsWithoutTheNewMembersAreSummarisedAsBefore(t *testing.T) {
	proof := uuid.New()
	for name, doc := range map[string]string{
		"pending": pendingDoc, "not applicable": naDoc,
		"unavailable, no back-off": `{"format":"certen-finality-status/1","class":"unavailable","levelReached":"included","nextLevel":"finalized","state":{"code":"finality_build_timeout","fields":{"after":"60000000000","cause":"x","chain":"84532"}}}`,
		"verified":                 verifiedDoc,
	} {
		s, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc)})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := summaryJSON(t, s, nil); strings.Contains(got, "pinsSha256") || strings.Contains(got, "attempt") {
			t.Errorf("%s: %s", name, got)
		}
	}
}

// Through the endpoint: before this change a row carrying the member was answered 500 FINALITY_SUMMARY_UNUSABLE for the whole proof.
func TestFinalityEndpointAnswersRowsCarryingTheNewMembers(t *testing.T) {
	proof := uuid.New()
	store := &fakeSummaryStore{exists: true,
		heads: []database.FinalityAddendumHead{headOf(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", 7, 0x33)},
		statuses: []database.FinalityStatus{
			statusRow(proof, 84532, 3, withPins(attemptDoc, pinsA)),
		}}
	rr := getFinality(NewFinalityHandlers(store, nil), proof.String())
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"pinsSha256":"`+pinsA+`"`) ||
		!strings.Contains(rr.Body.String(), `"attempt":"3"`) || !strings.Contains(rr.Body.String(), `"next_attempt_at":"2026-10-23T12:30:00Z"`) {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
	bad := &fakeSummaryStore{exists: true, statuses: []database.FinalityStatus{statusRow(proof, 84532, 3, withPins(pendingDoc, strings.ToUpper(pinsA)))}}
	rr = getFinality(NewFinalityHandlers(bad, nil), proof.String())
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "FINALITY_SUMMARY_UNUSABLE") ||
		!strings.Contains(rr.Body.String(), FinalityStatusMalformed) || !strings.Contains(rr.Body.String(), "bad_pins_sha256") {
		t.Fatalf("malformed pins: %d %s", rr.Code, rr.Body)
	}
}

// pinsSha256, when present, is 64 lowercase hex characters; anything else is malformed by its own reason, naming the level.
func TestMalformedPinsAreRefusedByTheirOwnReason(t *testing.T) {
	proof := uuid.New()
	for name, member := range map[string]string{
		"empty":         `""`,
		"null":          `null`,
		"number":        `7`,
		"true":          `true`,
		"array":         `["` + pinsA + `"]`,
		"object":        `{"v":"` + pinsA + `"}`,
		"upper case":    `"` + strings.ToUpper(pinsA) + `"`,
		"mixed case":    `"` + strings.ToUpper(pinsA[:10]) + pinsA[10:] + `"`,
		"63 characters": `"` + pinsA[:63] + `"`,
		"65 characters": `"` + pinsA + `0"`,
		"not hex":       `"` + pinsA[:63] + `g"`,
		"0x prefix":     `"0x` + pinsA[:62] + `"`,
		"padded":        `" ` + pinsA[:63] + `"`,
		"trailing nl":   `"` + pinsA[:63] + `\n"`,
		"unicode digit": `"` + pinsA[:63] + `٣"`,
	} {
		doc := strings.TrimSuffix(pendingDoc, "}") + `,"pinsSha256":` + member + `}`
		for _, level := range []int16{2, 3} {
			s, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, level, doc)})
			lv, reason := malformedReason(t, err)
			wantLevel := map[int16]string{2: "l1_posted", 3: "finalized"}[level]
			if reason != "bad_pins_sha256" || lv != wantLevel || s != nil {
				t.Errorf("%s at level %d: level=%q reason=%q summary=%+v", name, level, lv, reason, s)
			}
		}
	}
	// a verified-class row is judged the same way
	_, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 2, strings.TrimSuffix(verifiedDoc, "}")+`,"pinsSha256":"abc"}`)})
	if _, reason := malformedReason(t, err); reason != "bad_pins_sha256" {
		t.Fatalf("verified row: %v", err)
	}
}

// Nothing else became acceptable: the strict decode of the document stands, and the back-off fields are members of the STATE only.
func TestOtherUnknownMembersAreStillRefused(t *testing.T) {
	proof := uuid.New()
	add := func(doc, member string) string { return strings.TrimSuffix(doc, "}") + "," + member + "}" }
	for name, doc := range map[string]string{
		"extra beside valid pins":         add(withPins(pendingDoc, pinsA), `"extra":1`),
		"pins named otherwise":            add(pendingDoc, `"pins":"`+pinsA+`"`),
		"pins hash named otherwise":       add(pendingDoc, `"pinsHash":"`+pinsA+`"`),
		"pins with a suffix":              add(pendingDoc, `"pinsSha256Hex":"`+pinsA+`"`),
		"attempt as a document member":    add(pendingDoc, `"attempt":"2"`),
		"next attempt as a member":        add(pendingDoc, `"next_attempt_at":"2026-10-23T12:30:00Z"`),
		"attempt number as a member":      add(pendingDoc, `"attempt":2`),
		"a second class of member (soon)": add(pendingDoc, `"soon":"x"`),
	} {
		_, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc)})
		if _, reason := malformedReason(t, err); reason != "not_a_status_document" {
			t.Errorf("%s: reason %q", name, reason)
		}
	}
}

// attempt is the decimal ordinal (>= 1) of the failed ask, next_attempt_at an RFC 3339 time: both are checked where they appear in a state.
func TestMalformedBackOffFieldsAreRefusedByTheirOwnReason(t *testing.T) {
	proof := uuid.New()
	doc := func(fields string) string {
		return `{"format":"certen-finality-status/1","class":"refused","levelReached":"included","nextLevel":"finalized","state":{"code":"finality_build_timeout","fields":{` + fields + `}}}`
	}
	for _, c := range []struct{ name, fields, reason string }{
		{"attempt zero", `"attempt":"0"`, "bad_attempt"},
		{"attempt leading zero", `"attempt":"03"`, "bad_attempt"},
		{"attempt negative", `"attempt":"-1"`, "bad_attempt"},
		{"attempt plus", `"attempt":"+1"`, "bad_attempt"},
		{"attempt text", `"attempt":"three"`, "bad_attempt"},
		{"attempt empty", `"attempt":""`, "bad_attempt"},
		{"attempt a JSON number", `"attempt":3`, "bad_attempt"},
		{"attempt null", `"attempt":null`, "bad_attempt"},
		{"attempt overflows uint64", `"attempt":"18446744073709551616"`, "bad_attempt"},
		{"attempt padded", `"attempt":" 3"`, "bad_attempt"},
		{"next attempt text", `"attempt":"2","next_attempt_at":"tomorrow"`, "bad_next_attempt_at"},
		{"next attempt empty", `"next_attempt_at":""`, "bad_next_attempt_at"},
		{"next attempt a number", `"next_attempt_at":1792000000`, "bad_next_attempt_at"},
		{"next attempt null", `"next_attempt_at":null`, "bad_next_attempt_at"},
		{"next attempt date only", `"next_attempt_at":"2026-10-23"`, "bad_next_attempt_at"},
		{"next attempt without zone", `"next_attempt_at":"2026-10-23T12:30:00"`, "bad_next_attempt_at"},
	} {
		s, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc(c.fields))})
		lv, reason := malformedReason(t, err)
		if reason != c.reason || lv != "finalized" || s != nil {
			t.Errorf("%s: level=%q reason=%q", c.name, lv, reason)
		}
	}
	// the well-formed extremes are accepted
	for _, fields := range []string{`"attempt":"1"`, `"attempt":"18446744073709551615"`, `"next_attempt_at":"2026-10-23T12:30:00Z"`, `"next_attempt_at":"2026-10-23T14:30:00+02:00"`, `"attempt":"9","next_attempt_at":"2026-10-23T12:30:00Z"`} {
		if _, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc(fields))}); err != nil {
			t.Errorf("%s: %v", fields, err)
		}
	}
}

// A stored level is present whatever its status row says; the row's pins identity is shown on it only when the row is the verified
// status of THAT addendum (class verified, the addendum's seq), so a stale or foreign row cannot attribute pins to a stored level.
func TestAStoredLevelShowsThePinsOnlyOfItsOwnVerifiedStatus(t *testing.T) {
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	heads := func() []database.FinalityAddendumHead {
		return []database.FinalityAddendumHead{headOf(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", 47368146, 0x33)}
	}
	verifiedAt := func(seq string, pins string) string {
		d := `{"format":"certen-finality-status/1","class":"verified","levelReached":"l1_posted","nextLevel":"l1_posted","seq":` + seq + `}`
		if pins != "" {
			d = withPins(d, pins)
		}
		return d
	}
	const present = `"l1_posted":{"state":"present","chain":84532,"kind":"base-sepolia/l1_posted/v1","seq":1,"createdAt":"2026-10-20T09:30:00Z"`
	for _, c := range []struct {
		name, doc string
		pins      bool
	}{
		{"its verified status, with pins", verifiedAt("1", pinsA), true},
		{"its verified status, without pins (an older writer)", verifiedAt("1", ""), false},
		{"a verified status of another addendum (seq)", verifiedAt("2", pinsA), false},
		{"a stale pending status", withPins(pendingDoc, pinsA), false},
		{"a pending status that names this addendum seq", strings.TrimSuffix(withPins(pendingDoc, pinsA), "}") + `,"seq":1}`, false},
		{"a malformed document", `[`, false},
		{"an unknown member", strings.TrimSuffix(verifiedAt("1", pinsA), "}") + `,"x":1}`, false},
		{"a verified status with malformed pins", verifiedAt("1", "ABC"), false},
	} {
		s, err := SummarizeFinality(proof, heads(), []database.FinalityStatus{statusRow(proof, 84532, 2, c.doc)})
		if err != nil {
			t.Errorf("%s: a stored level is present whatever its status row says: %v", c.name, err)
			continue
		}
		got := summaryJSON(t, s, nil)
		want := present + `}`
		if c.pins {
			want = present + `,"pinsSha256":"` + pinsA + `"}`
		}
		if !strings.Contains(got, want) {
			t.Errorf("%s:\n got %s\nwant it to contain %s", c.name, got, want)
		}
	}
}
