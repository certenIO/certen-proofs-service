package server

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// Boundary cases of the CFE1 header reader: each length at which the grammar changes from one named reason to the next.
func TestCFE1HeaderBoundaries(t *testing.T) {
	full := cfe1("k", 5, 3, 9, 0x07)
	body := full[:len(full)-1] // the header alone
	cases := []struct {
		name string
		in   []byte
		bad  string
	}{
		{"empty", nil, "magic"},
		{"three bytes", []byte("CFE"), "magic"},
		{"magic only", []byte("CFE1"), "kind_length"},
		{"magic and three length bytes", []byte("CFE1\x00\x00\x00"), "kind_length"},
		{"magic and an empty kind", []byte("CFE1\x00\x00\x00\x00"), "fixed_part_truncated"},
		{"kind fills the rest exactly", []byte("CFE1\x00\x00\x00\x03abc"), "fixed_part_truncated"},
		{"kind one byte longer than the rest", []byte("CFE1\x00\x00\x00\x04abc"), "kind_truncated"},
		{"fixed part one byte short", body[:len(body)-1], "fixed_part_truncated"},
		{"the header exactly", body, ""},
		{"the header and a body", full, ""},
	}
	for _, c := range cases {
		_, bad := parseCFE1Header(c.in)
		if bad != c.bad {
			t.Errorf("%s: %q, want %q", c.name, bad, c.bad)
		}
	}
	// every field is read from its own offset, including from a slice with no spare capacity
	tight := append([]byte(nil), body...)
	h, bad := parseCFE1Header(tight[:len(tight):len(tight)])
	if bad != "" || h.Kind != "k" || h.Chain != 5 || h.Level != 3 || h.BlockNumber != 9 || h.BlockHash != [32]byte(bytes.Repeat([]byte{0x07}, 32)) {
		t.Fatalf("%+v %q", h, bad)
	}
	// a hash that differs in its last byte is not the same leg
	other := append([]byte(nil), tight...)
	other[len(other)-1] = 0x08
	h2, _ := parseCFE1Header(other)
	if h2.BlockHash == h.BlockHash {
		t.Fatal("the last hash byte is not read")
	}
}

func TestChainsAtTheEdgesOfTheTable(t *testing.T) {
	proof := uuid.New()
	mk := func(chain int64, headerChain uint64) []database.FinalityAddendum {
		kind := "base-sepolia/finalized/v1"
		return []database.FinalityAddendum{row(proof, 1, 3, chain, kind, cfe1(kind, headerChain, 3, 1, 0x33))}
	}
	// chain 1 is the smallest the table allows
	if out, err := ComposeFinalitySection(proof, mk(1, 1)); err != nil || out == nil {
		t.Fatalf("chain 1: %q %v", out, err)
	}
	// chain 0 and a negative chain are not rows of the table, whatever the header says
	for _, c := range []struct {
		chain  int64
		header uint64
	}{{0, 0}, {-1, 0xFFFFFFFFFFFFFFFF}, {-1, 0}} {
		_, err := ComposeFinalitySection(proof, mk(c.chain, c.header))
		var named *FinalityComposeError
		if !errors.As(err, &named) || named.Code != FinalityRowInvalid || named.Fields["field"] != "chain" {
			t.Errorf("chain %d header %d: %v", c.chain, c.header, err)
		}
	}
}

func TestFinalityErrIgnoresADanglingKey(t *testing.T) {
	e := finalityErr("c", "a", "1", "dangling")
	if len(e.Fields) != 1 || e.Fields["a"] != "1" || e.Error() != "c a=1" {
		t.Fatalf("%+v", e)
	}
	if e := finalityErr("c", "only"); len(e.Fields) != 0 {
		t.Fatalf("%+v", e)
	}
}

func TestAStatusStateThatIsNotAnObjectIsRefusedWhateverItsLength(t *testing.T) {
	proof := uuid.New()
	for _, state := range []string{`1`, `"x"`, `true`, `[]`, `[1,2]`} {
		doc := `{"format":"certen-finality-status/1","class":"pending","levelReached":"none","nextLevel":"finalized","state":` + state + `}`
		_, err := SummarizeFinality(proof, nil, []database.FinalityStatus{statusRow(proof, 84532, 3, doc)})
		var named *FinalityComposeError
		if !errors.As(err, &named) || named.Code != FinalityStatusMalformed || named.Fields["reason"] != "state_not_an_object" {
			t.Errorf("state %s: %v", state, err)
		}
	}
}

func TestPortableErrorsNameTheirCause(t *testing.T) {
	p := stored()
	p.Spine[1] = `{not json`
	if _, err := ComposePortable(p); err == nil || !strings.Contains(err.Error(), "major block 2 is not JSON") {
		t.Fatalf("%v", err)
	}
	p = stored()
	p.Spine[0] = `[`
	if _, err := ComposePortable(p); err == nil || !strings.Contains(err.Error(), "major block 1 is not JSON") {
		t.Fatalf("%v", err)
	}
	p = stored()
	p.GovRootInputs = `{`
	if _, err := ComposePortable(p); err == nil || !strings.Contains(err.Error(), "govRoot v3 inputs are not JSON") {
		t.Fatalf("%v", err)
	}
	p = stored()
	p.Spine = p.Spine[:1]
	if _, err := ComposePortable(p); err == nil || !strings.Contains(err.Error(), "needs 2 major blocks, 1 given") {
		t.Fatalf("%v", err)
	}
}

func TestV2HandlerAnswersOnlyItsOwnPath(t *testing.T) {
	proof := uuid.NewString()
	h := NewPortableHandlers(&fakePortable{p: stored()}, nil)
	for _, path := range []string{
		"/api/v1/proofs/" + proof + "/finality",
		"/api/v1/proofs/" + proof + "/bundle",
		"/api/v1/proofs/" + proof,
		"/api/v1/proofs/" + proof + "/v2/extra",
	} {
		rr := httptest.NewRecorder()
		h.HandleGetProofV2(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "INVALID_PATH") {
			t.Errorf("%s: %d %s", path, rr.Code, rr.Body)
		}
	}
}

func TestAnUnusableDocumentIsAnUnnamedInternalErrorAndAFinalityRefusalIsNamed(t *testing.T) {
	short := stored()
	short.Spine = short.Spine[:1]
	h := NewPortableHandlers(&fakePortable{p: short}, nil).WithFinality(&fakeFinality{})
	rr := get(h, uuid.NewString(), nil)
	if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "INTERNAL_ERROR") || strings.Contains(rr.Body.String(), "FINALITY_SECTION_UNUSABLE") {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
}
