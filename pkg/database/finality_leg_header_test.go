// Copyright 2026 Certen Protocol

package database

import (
	"bytes"
	"compress/gzip"
	"testing"
)

func gz(t *testing.T, doc string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte(doc)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestHeaderOfBundleReadsTheExecutionProofHeaderAndNothingElse(t *testing.T) {
	bundle := func(exec string) []byte {
		return gz(t, `{"bundle_integrity":{"custody_chain_hash":"x"},"proof_components":{"4_other":{"block_header_rlp":"0xdead"},"5_execution_proof":`+exec+`}}`)
	}
	if got := headerOfBundle(bundle(`{"chain_id":"84532","block_header_rlp":"0xc0c1"}`)); !bytes.Equal(got, []byte{0xc0, 0xc1}) {
		t.Errorf("a bundle with a header: %x", got)
	}
	if got := headerOfBundle(bundle(`{"chain_id":"84532","block_header_rlp":"c0c1"}`)); !bytes.Equal(got, []byte{0xc0, 0xc1}) {
		t.Errorf("an unprefixed header: %x", got)
	}
	for name, data := range map[string][]byte{
		"a legacy bundle (no header member)": bundle(`{"chain_id":"84532"}`),
		"a null header":                      bundle(`{"block_header_rlp":null}`),
		"an empty header":                    bundle(`{"block_header_rlp":""}`),
		"a header that is not hex":           bundle(`{"block_header_rlp":"0xzz"}`),
		"a header that is not a string":      bundle(`{"block_header_rlp":7}`),
		"no execution proof":                 gz(t, `{"proof_components":{}}`),
		"not JSON":                           gz(t, `nope`),
		"not gzip":                           []byte("plain"),
		"nothing stored":                     nil,
	} {
		if got := headerOfBundle(data); len(got) != 0 {
			t.Errorf("%s: %x", name, got)
		}
	}
}
