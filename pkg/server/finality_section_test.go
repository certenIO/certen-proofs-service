package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// chains.json is the validator's own vector set finality_chain2 (certenIO/certen-validator origin/pv2/rb11-b2b2a-db, file
// pkg/finality/testdata/finality_chain2/chains.json), copied byte for byte. Its manifest hash is pinned here.
const chainsJSONSHA256 = "fe430d589f48013e6deb4ffba1be85247b4685433e138c614b9b94f438237b08"

type chainVector struct {
	Name      string `json:"name"`
	Addenda   []map[string]any
	Carried   map[string]any `json:"carried"`
	Leg       map[string]any `json:"leg"`
	rawExpect json.RawMessage
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "finality", name))
	if err != nil {
		t.Fatal(err)
	}
	// a checkout with CRLF conversion hashes as the committed LF bytes
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func loadVectors(t *testing.T) map[string]chainVector {
	t.Helper()
	raw := readTestdata(t, "chains.json")
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != chainsJSONSHA256 {
		t.Fatalf("chains.json is not the validator's finality_chain2 vector set: sha256 %x", sum)
	}
	var file struct {
		Vectors []struct {
			Name    string           `json:"name"`
			Addenda []map[string]any `json:"addenda"`
			Carried map[string]any   `json:"carried"`
			Leg     map[string]any   `json:"leg"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	out := map[string]chainVector{}
	for _, v := range file.Vectors {
		out[v.Name] = chainVector{Name: v.Name, Addenda: v.Addenda, Carried: v.Carried, Leg: v.Leg}
	}
	return out
}

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rowsOf turns a vector's addenda into the rows the service reads from proof_finality_addenda.
func rowsOf(t *testing.T, v chainVector) []database.FinalityAddendum {
	t.Helper()
	var rows []database.FinalityAddendum
	for _, a := range v.Addenda {
		level := int16(0)
		switch a["level"].(string) {
		case "l1_posted":
			level = 2
		case "finalized":
			level = 3
		default:
			t.Fatalf("vector %s: level %v", v.Name, a["level"])
		}
		rows = append(rows, database.FinalityAddendum{
			ProofID: uuid.MustParse(a["proofId"].(string)), Seq: int64(a["seq"].(float64)), Level: level, Chain: int64(a["chain"].(float64)),
			Kind: a["kind"].(string), Evidence: mustHex(t, a["evidence"].(string)),
			PrevHash: mustHex(t, a["prevHash"].(string)), OwnHash: mustHex(t, a["ownHash"].(string)),
		})
	}
	return rows
}

func vectorProof(t *testing.T) uuid.UUID {
	return uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
}

// cfe1 builds the CFE1 container header of V2_SPEC_S7 3.1 followed by a body byte.
func cfe1(kind string, chain uint64, level byte, number uint64, hashByte byte) []byte {
	var b []byte
	b = append(b, "CFE1"...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(kind)))
	b = append(b, kind...)
	b = binary.BigEndian.AppendUint64(b, chain)
	b = append(b, level)
	b = binary.BigEndian.AppendUint64(b, number)
	b = append(b, bytes.Repeat([]byte{hashByte}, 32)...)
	return append(b, 0x61)
}

func row(proof uuid.UUID, seq int64, level int16, chain int64, kind string, evidence []byte) database.FinalityAddendum {
	return database.FinalityAddendum{
		ProofID: proof, Seq: seq, Level: level, Chain: chain, Kind: kind, Evidence: evidence,
		PrevHash: bytes.Repeat([]byte{byte(seq)}, 32), OwnHash: bytes.Repeat([]byte{byte(seq) + 0x10}, 32),
	}
}

func TestComposeFinalitySectionEqualsTheValidatorsGoldens(t *testing.T) {
	vectors := loadVectors(t)
	for _, name := range []string{"valid_sepolia_finalized", "valid_base_l1_posted_then_finalized", "valid_base_finalized_then_l1_posted"} {
		v := vectors[name]
		got, err := ComposeFinalitySection(vectorProof(t), rowsOf(t, v))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := readTestdata(t, "section_"+name+".golden.json")
		if string(got) != string(want) {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
		// the leg is the vector's own leg member, and both level orders are composed as stored (seq order, not level order)
		var sec struct {
			Leg     map[string]any   `json:"leg"`
			Addenda []map[string]any `json:"addenda"`
		}
		if err := json.Unmarshal(got, &sec); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(sec.Leg, v.Leg) || !reflect.DeepEqual(sec.Addenda, v.Addenda) {
			t.Errorf("%s: the composed member differs from the vector's leg/addenda", name)
		}
	}
}

func TestNoAddendaNoMember(t *testing.T) {
	v := loadVectors(t)["valid_empty"]
	got, err := ComposeFinalitySection(vectorProof(t), rowsOf(t, v))
	if err != nil || got != nil {
		t.Fatalf("no addenda gave %q, %v", got, err)
	}
	if got, err := ComposeFinalitySection(vectorProof(t), []database.FinalityAddendum{}); err != nil || got != nil {
		t.Fatalf("empty slice gave %q, %v", got, err)
	}
	// the document is byte-for-byte what it was without the feature, and equals the golden
	out, err := ComposePortableWithFinality(stored2(), nil)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := ComposePortable(stored2())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) || strings.Contains(string(out), "finality") {
		t.Fatalf("a document with no addenda changed: %s", out)
	}
	if string(out) != string(readTestdata(t, "document_valid_empty.golden.json")) {
		t.Fatalf("document %s", out)
	}
}

func stored2() *database.PortableV2 {
	p := stored()
	p.GovRootInputs = `{"g0Hash":"00"}`
	return p
}

func TestComposedDocumentEqualsGoldenAndKeepsEveryOtherMemberAsStored(t *testing.T) {
	vectors := loadVectors(t)
	for _, name := range []string{"valid_sepolia_finalized", "valid_base_l1_posted_then_finalized", "valid_base_finalized_then_l1_posted"} {
		section := readTestdata(t, "section_"+name+".golden.json")
		got, err := ComposePortableWithFinality(stored2(), section)
		if err != nil {
			t.Fatal(err)
		}
		if want := readTestdata(t, "document_"+name+".golden.json"); string(got) != string(want) {
			t.Errorf("%s:\n got %s\nwant %s", name, got, want)
		}
		// every member other than finality is exactly what ComposePortable serves
		var with, without map[string]json.RawMessage
		plain, _ := ComposePortable(stored2())
		if err := json.Unmarshal(got, &with); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(plain, &without); err != nil {
			t.Fatal(err)
		}
		if string(with["finality"]) != string(section) {
			t.Errorf("%s: the member changed on the way into the document", name)
		}
		delete(with, "finality")
		if !reflect.DeepEqual(with, without) {
			t.Errorf("%s: other members changed", name)
		}
		_ = vectors
	}
}

func TestAStoredDocumentWithItsOwnFinalityMemberIsRefused(t *testing.T) {
	for _, member := range []string{`{}`, `null`, `[]`, `{"format":"certen-finality-addenda/1"}`} {
		p := stored2()
		p.Document = `{"format":"certen-proof-v2-accumulate-portable/1","majors":[],"finality":` + member + `}`
		for name, call := range map[string]func() ([]byte, error){
			"ComposePortable":              func() ([]byte, error) { return ComposePortable(p) },
			"ComposePortableWithFinality":  func() ([]byte, error) { return ComposePortableWithFinality(p, json.RawMessage(`{"format":"x"}`)) },
			"ComposePortableWithoutMember": func() ([]byte, error) { return ComposePortableWithFinality(p, nil) },
		} {
			out, err := call()
			var named *FinalityComposeError
			if err == nil || !errors.As(err, &named) || named.Code != FinalityMemberInDocument {
				t.Errorf("%s with stored finality %s: %q, %v", name, member, out, err)
			}
		}
	}
}

func TestLegMismatchBetweenAddendaIsRefusedByName(t *testing.T) {
	proof := uuid.New()
	base := cfe1("base-sepolia/l1_posted/v1", 84532, 2, 47368146, 0x33)
	cases := map[string]database.FinalityAddendum{
		"another block number": row(proof, 2, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368147, 0x33)),
		"another block hash":   row(proof, 2, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368146, 0x34)),
		"another chain":        row(proof, 2, 3, 11155111, "eth-sepolia/finalized/v1", cfe1("eth-sepolia/finalized/v1", 11155111, 3, 47368146, 0x33)),
	}
	for name, second := range cases {
		rows := []database.FinalityAddendum{row(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", base), second}
		out, err := ComposeFinalitySection(proof, rows)
		var named *FinalityComposeError
		if err == nil || !errors.As(err, &named) || named.Code != FinalityLegMismatch || named.Fields["seq"] != "2" {
			t.Errorf("%s: %q, %v", name, out, err)
		}
		if out != nil {
			t.Errorf("%s: a member was composed anyway", name)
		}
	}
	// the same leg in both is composed
	same := []database.FinalityAddendum{row(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", base),
		row(proof, 2, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368146, 0x33))}
	if out, err := ComposeFinalitySection(proof, same); err != nil || out == nil {
		t.Fatalf("%q %v", out, err)
	}
	// and the disagreement is found wherever it sits in the chain
	three := []database.FinalityAddendum{same[0], same[1], row(proof, 3, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 9, 0x33))}
	if _, err := ComposeFinalitySection(proof, three); err == nil {
		t.Fatal("a third addendum on another block was composed")
	}
}

func TestAHeaderThatDisagreesWithItsOwnRowIsRefusedByName(t *testing.T) {
	proof := uuid.New()
	good := func() database.FinalityAddendum {
		return row(proof, 1, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368146, 0x33))
	}
	cases := map[string]struct {
		mutate func(*database.FinalityAddendum)
		code   string
		field  string
		reason string
	}{
		"kind":                      {func(a *database.FinalityAddendum) { a.Kind = "base-sepolia/l1_posted/v1" }, FinalityHeaderMismatch, "kind", ""},
		"chain":                     {func(a *database.FinalityAddendum) { a.Chain = 11155111 }, FinalityHeaderMismatch, "chain", ""},
		"negative chain":            {func(a *database.FinalityAddendum) { a.Chain = -1 }, FinalityRowInvalid, "chain", ""},
		"chain zero":                {func(a *database.FinalityAddendum) { a.Chain = 0 }, FinalityRowInvalid, "chain", ""},
		"level":                     {func(a *database.FinalityAddendum) { a.Level = 2 }, FinalityHeaderMismatch, "level", ""},
		"magic":                     {func(a *database.FinalityAddendum) { a.Evidence[0] = 'X' }, FinalityHeaderMalformed, "", "magic"},
		"shorter than the magic":    {func(a *database.FinalityAddendum) { a.Evidence = a.Evidence[:3] }, FinalityHeaderMalformed, "", "magic"},
		"no kind length":            {func(a *database.FinalityAddendum) { a.Evidence = a.Evidence[:7] }, FinalityHeaderMalformed, "", "kind_length"},
		"kind longer than the rest": {func(a *database.FinalityAddendum) { a.Evidence = a.Evidence[:4+4+10] }, FinalityHeaderMalformed, "", "kind_truncated"},
		"fixed part cut short":      {func(a *database.FinalityAddendum) { a.Evidence = a.Evidence[:len(a.Evidence)-2] }, FinalityHeaderMalformed, "", "fixed_part_truncated"},
		"empty evidence":            {func(a *database.FinalityAddendum) { a.Evidence = nil }, FinalityHeaderMalformed, "", "magic"},
	}
	for name, c := range cases {
		a := good()
		a.Evidence = append([]byte(nil), a.Evidence...)
		c.mutate(&a)
		out, err := ComposeFinalitySection(proof, []database.FinalityAddendum{a})
		var named *FinalityComposeError
		if err == nil || !errors.As(err, &named) || named.Code != c.code || out != nil {
			t.Errorf("%s: %q, %v", name, out, err)
			continue
		}
		if c.field != "" && named.Fields["field"] != c.field {
			t.Errorf("%s: field %v", name, named.Fields)
		}
		if c.reason != "" && named.Fields["reason"] != c.reason {
			t.Errorf("%s: reason %v", name, named.Fields)
		}
		if named.Fields["seq"] != "1" {
			t.Errorf("%s: seq %v", name, named.Fields)
		}
	}
	// the exact header length is enough: the body after it is not parsed
	exact := good()
	exact.Evidence = exact.Evidence[:len(exact.Evidence)-1]
	if out, err := ComposeFinalitySection(proof, []database.FinalityAddendum{exact}); err != nil || out == nil {
		t.Fatalf("header-only evidence: %q %v", out, err)
	}
}

func TestRowsThatCannotBeTheDocumentedFormAreRefusedByName(t *testing.T) {
	proof := uuid.New()
	good := func() database.FinalityAddendum {
		return row(proof, 1, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 47368146, 0x33))
	}
	cases := map[string]struct {
		mutate func(*database.FinalityAddendum)
		field  string
	}{
		"another proof":     {func(a *database.FinalityAddendum) { a.ProofID = uuid.New() }, "proofId"},
		"level 1":           {func(a *database.FinalityAddendum) { a.Level = 1 }, "level"},
		"level 0":           {func(a *database.FinalityAddendum) { a.Level = 0 }, "level"},
		"level 4":           {func(a *database.FinalityAddendum) { a.Level = 4 }, "level"},
		"short prev hash":   {func(a *database.FinalityAddendum) { a.PrevHash = a.PrevHash[:31] }, "prevHash"},
		"long prev hash":    {func(a *database.FinalityAddendum) { a.PrevHash = append(a.PrevHash, 0) }, "prevHash"},
		"short own hash":    {func(a *database.FinalityAddendum) { a.OwnHash = a.OwnHash[:31] }, "ownHash"},
		"long own hash":     {func(a *database.FinalityAddendum) { a.OwnHash = append(a.OwnHash, 0) }, "ownHash"},
		"missing own hash":  {func(a *database.FinalityAddendum) { a.OwnHash = nil }, "ownHash"},
		"missing prev hash": {func(a *database.FinalityAddendum) { a.PrevHash = nil }, "prevHash"},
	}
	for name, c := range cases {
		a := good()
		c.mutate(&a)
		out, err := ComposeFinalitySection(proof, []database.FinalityAddendum{a})
		var named *FinalityComposeError
		if err == nil || !errors.As(err, &named) || named.Code != FinalityRowInvalid || named.Fields["field"] != c.field || out != nil {
			t.Errorf("%s: %q, %v", name, out, err)
		}
	}
}

// The service says nothing about validity: a chain a verifier refuses is composed exactly as stored, so the refusal happens where
// the verifier is. The vectors below are the validator's own tampered chains.
func TestTamperedChainsAreComposedAsStoredNeverJudged(t *testing.T) {
	vectors := loadVectors(t)
	for _, name := range []string{"tamper_seq_gap", "tamper_prev_hash", "tamper_first_prev_hash_not_zero", "tamper_own_hash", "tamper_evidence_byte", "tamper_level_duplicate", "wrong_d0_with_matching_leg"} {
		v := vectors[name]
		out, err := ComposeFinalitySection(vectorProof(t), rowsOf(t, v))
		if err != nil {
			t.Errorf("%s: a chain the verifier judges was refused by the service: %v", name, err)
			continue
		}
		var sec struct {
			Leg     map[string]any   `json:"leg"`
			Addenda []map[string]any `json:"addenda"`
		}
		if err := json.Unmarshal(out, &sec); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(sec.Addenda, v.Addenda) || !reflect.DeepEqual(sec.Leg, v.Leg) {
			t.Errorf("%s: not composed as stored", name)
		}
	}
	// an altered carried leg is not the service's concern either: the leg it serves is the one in the headers
	v := vectors["leg_binding_altered_leg_member"]
	out, err := ComposeFinalitySection(vectorProof(t), rowsOf(t, v))
	if err != nil {
		t.Fatal(err)
	}
	var sec struct {
		Leg map[string]any `json:"leg"`
	}
	_ = json.Unmarshal(out, &sec)
	if !reflect.DeepEqual(sec.Leg, v.Leg) || reflect.DeepEqual(sec.Leg, v.Carried) {
		t.Errorf("leg %v, want the header leg %v and not the altered %v", sec.Leg, v.Leg, v.Carried)
	}
}

type fakeFinality struct {
	rows []database.FinalityAddendum
	err  error
	got  uuid.UUID
}

func (f *fakeFinality) ListFinalityAddenda(_ context.Context, id uuid.UUID) ([]database.FinalityAddendum, error) {
	f.got = id
	return f.rows, f.err
}

func getV2(h *PortableHandlers, id string, hdr map[string]string) (int, string, http.Header) {
	rr := get(h, id, hdr)
	return rr.Code, rr.Body.String(), rr.Header()
}

func TestHandlerServesTheFinalityMemberComposedAtRequestTime(t *testing.T) {
	vectors := loadVectors(t)
	v := vectors["valid_base_finalized_then_l1_posted"]
	proof := vectorProof(t)
	fin := &fakeFinality{rows: rowsOf(t, v)}
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin)
	code, body, hdr := getV2(h, proof.String(), nil)
	if code != http.StatusOK || fin.got != proof {
		t.Fatalf("status %d, looked up %s", code, fin.got)
	}
	if want := string(readTestdata(t, "document_valid_base_finalized_then_l1_posted.golden.json")); body != want {
		t.Fatalf("body\n got %s\nwant %s", body, want)
	}
	if !strings.Contains(hdr.Get("Cache-Control"), "private") || !strings.Contains(hdr.Get("Cache-Control"), "no-cache") {
		t.Fatalf("a document whose finality member grows must be revalidated: %q", hdr.Get("Cache-Control"))
	}
	if strings.Contains(hdr.Get("Cache-Control"), "max-age=3600") {
		t.Fatalf("cached for an hour: %q", hdr.Get("Cache-Control"))
	}
	// a new addendum changes the ETag, and an unchanged document revalidates as 304
	etag := hdr.Get("ETag")
	if c, _, _ := getV2(h, proof.String(), map[string]string{"If-None-Match": etag}); c != http.StatusNotModified {
		t.Fatalf("unchanged: %d", c)
	}
	fin.rows = rowsOf(t, vectors["valid_sepolia_finalized"])
	_, _, hdr2 := getV2(h, proof.String(), nil)
	if hdr2.Get("ETag") == etag {
		t.Fatal("the ETag did not change with the finality member")
	}
	if c, _, _ := getV2(h, proof.String(), map[string]string{"If-None-Match": etag}); c != http.StatusOK {
		t.Fatalf("a stale ETag after a new level: %d", c)
	}
}

func TestHandlerWithNoAddendaServesTheDocumentItServedBefore(t *testing.T) {
	proof := uuid.NewString()
	_, without, _ := getV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil), proof, nil)
	_, with, _ := getV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{}), proof, nil)
	if with != without || strings.Contains(with, "finality") {
		t.Fatalf("with an empty chain the document changed:\n%s\n%s", with, without)
	}
}

func TestHandlerRefusesWhatItCannotComposeWithANamed500(t *testing.T) {
	proof := uuid.New()
	a := row(proof, 1, 2, 84532, "base-sepolia/l1_posted/v1", cfe1("base-sepolia/l1_posted/v1", 84532, 2, 5, 0x33))
	b := row(proof, 2, 3, 84532, "base-sepolia/finalized/v1", cfe1("base-sepolia/finalized/v1", 84532, 3, 6, 0x33))
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: []database.FinalityAddendum{a, b}})
	code, body, _ := getV2(h, proof.String(), nil)
	if code != http.StatusInternalServerError || !strings.Contains(body, "FINALITY_SECTION_UNUSABLE") || !strings.Contains(body, FinalityLegMismatch) {
		t.Fatalf("a mismatching leg: %d %s", code, body)
	}
	if strings.Contains(body, "finality\":{") || strings.Contains(body, "evidence") {
		t.Fatalf("the refusal leaked a document or evidence: %s", body)
	}

	bad := stored2()
	bad.Document = `{"format":"x","majors":[],"finality":{}}`
	code, body, _ = getV2(NewPortableHandlers(&fakePortable{p: bad}, nil).WithFinality(&fakeFinality{}), proof.String(), nil)
	if code != http.StatusInternalServerError || !strings.Contains(body, FinalityMemberInDocument) {
		t.Fatalf("a stored finality key: %d %s", code, body)
	}

	// a failing read is not "no addenda": the document is not served without the member
	code, body, _ = getV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{err: errors.New("db down")}), proof.String(), nil)
	if code != http.StatusInternalServerError || strings.Contains(body, "majors") || !strings.Contains(body, "INTERNAL_ERROR") {
		t.Fatalf("a failed addenda read: %d %s", code, body)
	}
	// and a proof with no document is still not available, whatever its addenda
	code, body, _ = getV2(NewPortableHandlers(&fakePortable{}, nil).WithFinality(&fakeFinality{rows: []database.FinalityAddendum{a}}), proof.String(), nil)
	if code != http.StatusNotFound || !strings.Contains(body, "PROOF_V2_NOT_AVAILABLE") {
		t.Fatalf("no document: %d %s", code, body)
	}
}

func TestTheNamedErrorsAreStableText(t *testing.T) {
	e := finalityErr(FinalityLegMismatch, "seq", "2", "a", "b")
	if e.Error() != "finality_leg_mismatch a=b seq=2" {
		t.Fatalf("%q", e.Error())
	}
	if finalityErr(FinalityMemberInDocument).Error() != "finality_member_stored_in_document" {
		t.Fatal("no-field text")
	}
}
