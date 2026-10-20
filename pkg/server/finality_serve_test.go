package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

func twoRows(t *testing.T) (uuid.UUID, []database.FinalityAddendum) {
	t.Helper()
	proof := uuid.New()
	return proof, []database.FinalityAddendum{
		row(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", cfe1("base-sepolia/l1_posted/v1", 84532, 2, 47368146, 0x33)),
		row(proof, 2, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368146, 0x33)),
	}
}

func TestAMatchingIfNoneMatchIsAnsweredWithoutReadingAnyEvidence(t *testing.T) {
	proof, rows := twoRows(t)
	fin := &fakeFinality{rows: rows}
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin)
	code, body, hdr := getV2(h, proof.String(), nil)
	if code != http.StatusOK || !strings.Contains(body, `"finality":`) {
		t.Fatalf("first read: %d %s", code, body)
	}
	if fin.evidenceReads != 1 || fin.byteReads != 1 || fin.idReads != 1 {
		t.Fatalf("a full read took %d identities, %d sizes, %d evidence reads", fin.idReads, fin.byteReads, fin.evidenceReads)
	}
	etag := hdr.Get("ETag")
	code, body, hdr2 := getV2(h, proof.String(), map[string]string{"If-None-Match": etag})
	if code != http.StatusNotModified || body != "" {
		t.Fatalf("revalidation: %d %q", code, body)
	}
	if fin.evidenceReads != 1 || fin.byteReads != 1 {
		t.Fatalf("a 304 read the evidence: %d evidence reads, %d size reads", fin.evidenceReads, fin.byteReads)
	}
	if fin.idReads != 2 {
		t.Fatalf("a 304 needs the identities, once: %d", fin.idReads)
	}
	if hdr2.Get("ETag") != etag || !strings.Contains(hdr2.Get("Cache-Control"), "no-cache") {
		t.Fatalf("headers of the 304: %v", hdr2)
	}
	// a new addendum is a new tag, and the old tag no longer matches
	fin.rows = append(fin.rows, row(proof, 3, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368146, 0x33)))
	code, _, hdr3 := getV2(h, proof.String(), map[string]string{"If-None-Match": etag})
	if code != http.StatusOK || hdr3.Get("ETag") == etag {
		t.Fatalf("after a new addendum: %d %q", code, hdr3.Get("ETag"))
	}
}

func TestWithNoAddendaTheTagIsTheHashOfTheBodyAsItAlwaysWas(t *testing.T) {
	proof := uuid.New()
	plain := NewPortableHandlers(&fakePortable{p: stored2()}, nil)
	_, body, hdrPlain := getV2(plain, proof.String(), nil)
	sum := sha256.Sum256([]byte(body))
	want := `"` + hex.EncodeToString(sum[:]) + `"`
	if hdrPlain.Get("ETag") != want {
		t.Fatalf("unwired tag %q, hash of the body %q", hdrPlain.Get("ETag"), want)
	}
	fin := &fakeFinality{}
	wired := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin)
	code, body2, hdr := getV2(wired, proof.String(), nil)
	if code != http.StatusOK || body2 != body || hdr.Get("ETag") != want {
		t.Fatalf("wired with no addenda: %d tag %q", code, hdr.Get("ETag"))
	}
	if fin.byteReads != 0 || fin.evidenceReads != 0 {
		t.Fatalf("no addenda but the evidence was read: %d %d", fin.byteReads, fin.evidenceReads)
	}
	if c, _, _ := getV2(wired, proof.String(), map[string]string{"If-None-Match": want}); c != http.StatusNotModified {
		t.Fatalf("revalidation of a document without the member: %d", c)
	}
}

func TestTheTagCoversTheBaseTheProofAndEveryIdentityField(t *testing.T) {
	var base [32]byte
	copy(base[:], bytes.Repeat([]byte{0x01}, 32))
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	ids := []database.FinalityIdentity{{Seq: 1, Level: 2, OwnHash: bytes.Repeat([]byte{0xa1}, 32)}, {Seq: 2, Level: 3, OwnHash: bytes.Repeat([]byte{0xb2}, 32)}}
	ref := finalityETag(base, proof, ids)
	if again := finalityETag(base, proof, ids); again != ref {
		t.Fatal("the tag is not deterministic")
	}
	if !strings.HasPrefix(ref, `"`) || !strings.HasSuffix(ref, `"`) || len(ref) != 66 {
		t.Fatalf("tag %q", ref)
	}
	other := base
	other[31] ^= 1
	changes := map[string]string{
		"base":           finalityETag(other, proof, ids),
		"proof":          finalityETag(base, uuid.MustParse("01234567-89ab-4def-8012-3456789abcdf"), ids),
		"fewer rows":     finalityETag(base, proof, ids[:1]),
		"swapped order":  finalityETag(base, proof, []database.FinalityIdentity{ids[1], ids[0]}),
		"seq":            finalityETag(base, proof, []database.FinalityIdentity{{Seq: 9, Level: 2, OwnHash: ids[0].OwnHash}, ids[1]}),
		"level":          finalityETag(base, proof, []database.FinalityIdentity{{Seq: 1, Level: 3, OwnHash: ids[0].OwnHash}, ids[1]}),
		"own hash":       finalityETag(base, proof, []database.FinalityIdentity{{Seq: 1, Level: 2, OwnHash: bytes.Repeat([]byte{0xa2}, 32)}, ids[1]}),
		"last hash byte": finalityETag(base, proof, []database.FinalityIdentity{ids[0], {Seq: 2, Level: 3, OwnHash: append(bytes.Repeat([]byte{0xb2}, 31), 0xb3)}}),
	}
	for name, tag := range changes {
		if tag == ref {
			t.Errorf("the tag does not change with the %s", name)
		}
	}
	// no addenda: the base's own hash
	if got := finalityETag(base, proof, nil); got != `"`+hex.EncodeToString(base[:])+`"` {
		t.Fatalf("no addenda: %q", got)
	}
	if got := finalityETag(base, proof, []database.FinalityIdentity{}); got != `"`+hex.EncodeToString(base[:])+`"` {
		t.Fatalf("empty addenda: %q", got)
	}
}

func TestTheTagNamesWhatWasServedWhenAnAddendumIsCommittedBetweenTheReads(t *testing.T) {
	proof, rows := twoRows(t)
	late := row(proof, 3, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368146, 0x33))
	fin := &fakeFinality{rows: rows, late: []database.FinalityAddendum{late}}
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin)
	_, body, hdr := getV2(h, proof.String(), nil)
	if strings.Count(body, `"ownHash"`) != 3 {
		t.Fatalf("the committed addendum is not in the body: %s", body)
	}
	_, _, before := getV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: rows}), proof.String(), nil)
	_, _, after := getV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: append(append([]database.FinalityAddendum(nil), rows...), late)}), proof.String(), nil)
	if hdr.Get("ETag") != after.Get("ETag") || hdr.Get("ETag") == before.Get("ETag") {
		t.Fatalf("tag %q; for the two rows %q; for the three %q", hdr.Get("ETag"), before.Get("ETag"), after.Get("ETag"))
	}
}

func TestEvidenceAboveTheBoundIsANamedRefusalNeverATruncation(t *testing.T) {
	if MaxFinalityEvidenceBytes != 6<<20 {
		t.Fatalf("the bound is %d", MaxFinalityEvidenceBytes)
	}
	// hex doubles the evidence: the bound plus the rest of the document must fit the gateway's 16 MiB cap on a downstream body
	if 2*MaxFinalityEvidenceBytes >= 16<<20 {
		t.Fatal("the bound leaves no room under the gateway's cap")
	}
	proof, rows := twoRows(t)
	for _, c := range []struct {
		name  string
		bytes int64
		ok    bool
	}{
		{"one byte over", MaxFinalityEvidenceBytes + 1, false},
		{"far over", 64 << 20, false},
		{"exactly the bound", MaxFinalityEvidenceBytes, true},
		{"one under", MaxFinalityEvidenceBytes - 1, true},
		{"zero", 0, true},
	} {
		n := c.bytes
		fin := &fakeFinality{rows: rows, bytes: &n}
		code, body, _ := getV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin), proof.String(), nil)
		if c.ok {
			if code != http.StatusOK || !strings.Contains(body, `"finality":`) {
				t.Errorf("%s: %d %s", c.name, code, body)
			}
			continue
		}
		if code != http.StatusInternalServerError || !strings.Contains(body, "FINALITY_SECTION_UNUSABLE") || !strings.Contains(body, "finality_evidence_too_large") ||
			!strings.Contains(body, "bytes="+itoa(c.bytes)) || !strings.Contains(body, "limit="+itoa(MaxFinalityEvidenceBytes)) {
			t.Errorf("%s: %d %s", c.name, code, body)
		}
		if strings.Contains(body, "majors") || strings.Contains(body, `"finality":`) {
			t.Errorf("%s: a document was served: %s", c.name, body)
		}
		if fin.evidenceReads != 0 {
			t.Errorf("%s: the evidence was read before the refusal", c.name)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func newTestLogger(w io.Writer) *log.Logger { return log.New(w, "", 0) }

func TestTheSizeAndIdentityReadsFailLoudly(t *testing.T) {
	proof, rows := twoRows(t)
	for name, fin := range map[string]*fakeFinality{
		"identities": {rows: rows, idsErr: errors.New("db down")},
		"size":       {rows: rows, bytesErr: errors.New("db down")},
		"evidence":   {rows: rows, err: errors.New("db down")},
	} {
		code, body, _ := getV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin), proof.String(), nil)
		if code != http.StatusInternalServerError || !strings.Contains(body, "INTERNAL_ERROR") || strings.Contains(body, "majors") || strings.Contains(body, "db down") {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
}

func TestTheFailureLogLinesCarryTheTokensToAlertOn(t *testing.T) {
	proof, rows := twoRows(t)
	rows[1].Evidence = cfe1("base-sepolia/finalized/v1", 84532, 3, 9, 0x33) // another block: a leg mismatch
	var logged bytes.Buffer
	h := NewPortableHandlers(&fakePortable{p: stored2()}, newTestLogger(&logged)).WithFinality(&fakeFinality{rows: rows})
	if code, _, _ := getV2(h, proof.String(), nil); code != http.StatusInternalServerError {
		t.Fatalf("status %d", code)
	}
	if !strings.Contains(logged.String(), "FINALITY_SECTION_UNUSABLE proof="+proof.String()+" finality_leg_mismatch") {
		t.Fatalf("log: %q", logged.String())
	}
	logged.Reset()
	bad := &fakeSummaryStore{exists: true, statuses: []database.FinalityStatus{statusRow(proof, 84532, 3, "[")}}
	rec := getFinality(NewFinalityHandlers(bad, newTestLogger(&logged)), proof.String())
	if rec.Code != http.StatusInternalServerError || !strings.Contains(logged.String(), "FINALITY_SUMMARY_UNUSABLE proof="+proof.String()+" finality_status_malformed") {
		t.Fatalf("%d log: %q", rec.Code, logged.String())
	}
}

func TestTheSummaryReadsStatusBeforeHeadsSoACommitBetweenThemShowsNoDisagreement(t *testing.T) {
	proof := uuid.New()
	stored := `{"format":"certen-finality-status/1","class":"verified","levelReached":"finalized","nextLevel":"finalized","seq":1}`
	store := &fakeSummaryStore{
		exists:    true,
		statuses:  []database.FinalityStatus{statusRow(proof, 84532, 3, stored)},
		lateHeads: []database.FinalityAddendumHead{headOf(proof, 1, 3, 84532, "base-sepolia/finalized/v1", 7, 0x33)},
	}
	rr := getFinality(NewFinalityHandlers(store, nil), proof.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
	body := rr.Body.String()
	if strings.Contains(body, "status_stored_without_addendum") || !strings.Contains(body, `"finalized":{"state":"present"`) || !strings.Contains(body, `"recordedLevel":"finalized"`) {
		t.Fatalf("the commit between the reads showed a disagreement: %s", body)
	}
	if len(store.order) != 2 || store.order[0] != "status" || store.order[1] != "heads" {
		t.Fatalf("read order %v", store.order)
	}
	// a stored-status row with no addendum at all is still named (the validators' records do disagree)
	still := &fakeSummaryStore{exists: true, statuses: []database.FinalityStatus{statusRow(proof, 84532, 3, stored)}}
	rr = getFinality(NewFinalityHandlers(still, nil), proof.String())
	if !strings.Contains(rr.Body.String(), `"state":"status_stored_without_addendum"`) {
		t.Fatalf("%s", rr.Body)
	}
}
