// Copyright 2026 Certen Protocol

package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// R12-OUTCOME-TX: the optional leg.headerRlp of the `finality` member. The two headers are real settlement block headers
// (testdata/finality/legs_with_header.json, dumped from the validators' fixtures); the goldens are assembled by hand by
// testdata/finality/gen_header_goldens.mjs, never by this code.

type realLeg struct {
	Chain     uint64 `json:"chain"`
	Number    uint64 `json:"number"`
	Hash      string `json:"hash"`
	HeaderRlp string `json:"headerRlp"`
}

func realLegs(t *testing.T) map[string]realLeg {
	t.Helper()
	var f struct {
		Headers map[string]realLeg `json:"headers"`
	}
	if err := json.Unmarshal(readTestdata(t, "legs_with_header.json"), &f); err != nil {
		t.Fatal(err)
	}
	return f.Headers
}

func cfe1Real(kind string, chain uint64, level byte, number uint64, hash []byte) []byte {
	var b []byte
	b = append(b, "CFE1"...)
	b = binary.BigEndian.AppendUint32(b, uint32(len(kind)))
	b = append(b, kind...)
	b = binary.BigEndian.AppendUint64(b, chain)
	b = append(b, level)
	b = binary.BigEndian.AppendUint64(b, number)
	b = append(b, hash...)
	return append(b, 0x61)
}

func legRows(t *testing.T, l realLeg, kinds ...string) (uuid.UUID, []database.FinalityAddendum) {
	t.Helper()
	proof := uuid.MustParse("01234567-89ab-4def-8012-3456789abcde")
	hash, err := hex.DecodeString(l.Hash)
	if err != nil {
		t.Fatal(err)
	}
	var rows []database.FinalityAddendum
	for i, k := range kinds {
		level := int16(2)
		if strings.HasSuffix(k, "/finalized/v1") {
			level = 3
		}
		rows = append(rows, row(proof, int64(i+1), level, int64(l.Chain), k, cfe1Real(k, l.Chain, byte(level), l.Number, hash)))
	}
	return proof, rows
}

func TestKeccak256IsTheEthereumHashAndTheRealHeadersHashToTheirBlocks(t *testing.T) {
	for in, want := range map[string]string{
		"":    "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470",
		"abc": "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45",
		// longer than one 136-byte block
		strings.Repeat("a", 300): "",
	} {
		got := keccak256([]byte(in))
		if want != "" && hex.EncodeToString(got[:]) != want {
			t.Errorf("keccak256(%.10q) = %x, want %s", in, got, want)
		}
	}
	for name, l := range realLegs(t) {
		h, err := hex.DecodeString(l.HeaderRlp)
		if err != nil {
			t.Fatal(err)
		}
		if got := keccak256(h); hex.EncodeToString(got[:]) != l.Hash {
			t.Errorf("%s: the header hashes to %x, the block is %s", name, got, l.Hash)
		}
	}
}

func TestTheSectionCarriesTheSettlementHeaderGoldens(t *testing.T) {
	legs := realLegs(t)
	for _, c := range []struct {
		golden, leg string
		kinds       []string
	}{
		{"section_header_bundle_base_47368146.golden.json", "base_sepolia_47368146", []string{"base-sepolia/l1_posted/v1", "base-sepolia/finalized/v1"}},
		{"section_header_legacy_adiri_498759.golden.json", "telcoin_adiri_498759", []string{"adiri/finalized/v1"}},
	} {
		l := legs[c.leg]
		header, _ := hex.DecodeString(l.HeaderRlp)
		proof, rows := legRows(t, l, c.kinds...)
		want := readTestdata(t, c.golden)
		got, err := ComposeFinalitySectionWithHeader(proof, rows, header)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s: %v\n got  %.300s\n want %.300s", c.golden, err, got, want)
		}
		// the streamed body is the same bytes
		stream, err := newFinalitySectionStreamWithHeader(proof, rows, header)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := stream.writeTo(&buf); err != nil || !bytes.Equal(buf.Bytes(), want) || int64(buf.Len()) != stream.size() {
			t.Fatalf("%s: the stream differs from the composition (%v, %d bytes, size %d)", c.golden, err, buf.Len(), stream.size())
		}
		// without a header the same rows compose the member exactly as before: the golden without its headerRlp
		plain, err := ComposeFinalitySection(proof, rows)
		if err != nil || strings.Contains(string(plain), "headerRlp") ||
			string(plain) != strings.Replace(string(want), `,"headerRlp":"`+l.HeaderRlp+`"`, ``, 1) {
			t.Fatalf("%s: the header-less member changed: %v\n%.300s", c.golden, err, plain)
		}
	}
}

func TestAHeaderOfAnotherBlockIsRefusedByName(t *testing.T) {
	legs := realLegs(t)
	base, other := legs["base_sepolia_47368146"], legs["telcoin_adiri_498759"]
	proof, rows := legRows(t, base, "base-sepolia/l1_posted/v1")
	wrong, _ := hex.DecodeString(other.HeaderRlp)
	var named *FinalityComposeError
	if _, err := ComposeFinalitySectionWithHeader(proof, rows, wrong); !errors.As(err, &named) || named.Code != FinalityLegHeaderMismatch {
		t.Fatalf("composition: %v", err)
	}
	if _, err := newFinalitySectionStreamWithHeader(proof, rows, wrong); !errors.As(err, &named) || named.Code != FinalityLegHeaderMismatch {
		t.Fatalf("stream: %v", err)
	}
}

// headerFinality is a finality store that can also serve the leg's header.
type headerFinality struct {
	*fakeFinality
	header []byte
	err    error
	asked  int
}

func (f *headerFinality) LegHeaderRLP(_ context.Context, _ uuid.UUID, chain, number uint64, hash [32]byte) ([]byte, error) {
	f.asked++
	if chain != 84532 || number != 47368146 || hex.EncodeToString(hash[:]) == "" {
		return nil, errors.New("asked for another leg")
	}
	return f.header, f.err
}

func TestTheServedDocumentCarriesTheHeaderOnlyWhenTheStoreHasOneThatBinds(t *testing.T) {
	legs := realLegs(t)
	base, other := legs["base_sepolia_47368146"], legs["telcoin_adiri_498759"]
	header, _ := hex.DecodeString(base.HeaderRlp)
	wrong, _ := hex.DecodeString(other.HeaderRlp)
	proof, rows := legRows(t, base, "base-sepolia/l1_posted/v1", "base-sepolia/finalized/v1")
	serve := func(fin finalityStore) (int, string, string) {
		var logs bytes.Buffer
		h := NewPortableHandlers(&fakePortable{p: stored2()}, newTestLogger(&logs)).WithFinality(fin)
		code, body, _ := getV2(h, proof.String(), nil)
		return code, body, logs.String()
	}
	if code, body, _ := serve(&headerFinality{fakeFinality: &fakeFinality{rows: rows}, header: header}); code != http.StatusOK || !strings.Contains(body, `"headerRlp":"`+base.HeaderRlp+`"`) {
		t.Fatalf("a bound header was not carried: %d %.200s", code, body)
	}
	for name, fin := range map[string]finalityStore{
		"a store without the header face": &fakeFinality{rows: rows},
		"no stored header":                &headerFinality{fakeFinality: &fakeFinality{rows: rows}},
		"a failing read":                  &headerFinality{fakeFinality: &fakeFinality{rows: rows}, err: errors.New("boom")},
		"a header of another block":       &headerFinality{fakeFinality: &fakeFinality{rows: rows}, header: wrong},
	} {
		code, body, logs := serve(fin)
		if code != http.StatusOK || !strings.Contains(body, `"finality":`) || strings.Contains(body, "headerRlp") {
			t.Fatalf("%s: %d %.200s", name, code, body)
		}
		switch name {
		case "a failing read":
			if !strings.Contains(logs, "FINALITY_LEG_HEADER_UNREAD") {
				t.Errorf("%s: not logged: %q", name, logs)
			}
		case "a header of another block":
			if !strings.Contains(logs, "FINALITY_LEG_HEADER_IGNORED") {
				t.Errorf("%s: not logged: %q", name, logs)
			}
		}
	}
}
