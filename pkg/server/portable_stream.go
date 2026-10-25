// Copyright 2026 Certen Protocol
//
// The body of GET /api/v1/proofs/{proof_id}/v2 is written, not built: the evidence of a proof's finality addenda is up to 128 MiB
// (MaxFinalityEvidenceBytes) and is served as hex, so a body that is first marshalled whole (the section, then the document around it,
// then a compressed copy) holds the evidence several times over. The pieces below write the same bytes json.Marshal would
// (ComposePortableWithFinality is the oracle the tests compare them with), taking the hex of the evidence in small chunks straight
// from the rows into the response, so serving holds the rows once and nothing else of that size.

package server

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// hexChunkRaw is how much evidence is hex-encoded per write; the output buffer is twice that.
const hexChunkRaw = 16 << 10

// The time a response of the portable document is given to be written: a base window, and for each MiB of body at least this long, so
// that the largest legal document (about 272 MiB) is given minutes, not the server's 30 s WriteTimeout, to reach a slow reader.
const (
	portableWriteBase   = 30 * time.Second
	portableWritePerMiB = time.Second
)

// bodyPiece is a part of a response body whose size is known before any of it is written.
type bodyPiece interface {
	size() int64
	writeTo(w io.Writer) error
}

type fixedPiece []byte

func (p fixedPiece) size() int64 { return int64(len(p)) }

func (p fixedPiece) writeTo(w io.Writer) error {
	_, err := w.Write(p)
	return err
}

// portableBody is a document body as a sequence of pieces.
type portableBody struct{ pieces []bodyPiece }

// Len is the exact length of the body in bytes.
func (b portableBody) Len() int64 {
	var n int64
	for _, p := range b.pieces {
		n += p.size()
	}
	return n
}

func (b portableBody) writeTo(w io.Writer) error {
	for _, p := range b.pieces {
		if err := p.writeTo(w); err != nil {
			return err
		}
	}
	return nil
}

// sectionItem is one addendum of the finality member: the JSON up to the opening quote of its evidence, the evidence itself (written
// as lowercase hex), and the JSON from the closing quote on.
type sectionItem struct {
	prefix, suffix []byte
	evidence       []byte
}

// finalitySectionStream is the `finality` member as pieces: the same bytes ComposeFinalitySection returns, without the hex copy of the
// evidence. It holds the rows' evidence by reference and never copies it.
type finalitySectionStream struct {
	head  []byte
	items []sectionItem
}

const evidenceKey = `"evidence":"`

// newFinalitySectionStream validates the rows exactly as ComposeFinalitySection does (same refusals, same names) and returns the member
// as a stream, or nil when there are no rows (the member is then absent).
func newFinalitySectionStream(proofID uuid.UUID, rows []database.FinalityAddendum) (*finalitySectionStream, error) {
	return newFinalitySectionStreamWithHeader(proofID, rows, nil)
}

// newFinalitySectionStreamWithHeader is newFinalitySectionStream with the leg carrying header (see ComposeFinalitySectionWithHeader).
func newFinalitySectionStreamWithHeader(proofID uuid.UUID, rows []database.FinalityAddendum, header []byte) (*finalitySectionStream, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	names, leg, err := validateFinalityRows(proofID, rows)
	if err != nil {
		return nil, err
	}
	if leg, err = withLegHeader(leg, header); err != nil {
		return nil, err
	}
	empty, err := json.Marshal(finalitySectionJSON{Format: FinalitySectionFormat, Leg: leg, Addenda: []finalityAddendumJSON{}})
	if err != nil {
		return nil, err
	}
	s := &finalitySectionStream{head: empty[:len(empty)-2]} // everything before the closing "]}"
	for i, r := range rows {
		whole, err := json.Marshal(finalityAddendumJSON{
			ProofID: r.ProofID.String(), Seq: r.Seq, Level: names[i], Chain: r.Chain, Kind: r.Kind,
			PrevHash: hex.EncodeToString(r.PrevHash), OwnHash: hex.EncodeToString(r.OwnHash),
		})
		if err != nil {
			return nil, err
		}
		at := bytes.Index(whole, []byte(evidenceKey))
		if at < 0 {
			return nil, fmt.Errorf("the addendum JSON has no evidence member")
		}
		cut := at + len(evidenceKey)
		s.items = append(s.items, sectionItem{prefix: whole[:cut], suffix: whole[cut:], evidence: r.Evidence})
	}
	return s, nil
}

func (s *finalitySectionStream) size() int64 {
	n := int64(len(s.head)) + 2
	for i, it := range s.items {
		n += int64(len(it.prefix)) + 2*int64(len(it.evidence)) + int64(len(it.suffix))
		if i > 0 {
			n++ // the comma between addenda
		}
	}
	return n
}

func (s *finalitySectionStream) writeTo(w io.Writer) error {
	if _, err := w.Write(s.head); err != nil {
		return err
	}
	buf := make([]byte, 2*hexChunkRaw)
	for i, it := range s.items {
		if i > 0 {
			if _, err := w.Write([]byte{','}); err != nil {
				return err
			}
		}
		if _, err := w.Write(it.prefix); err != nil {
			return err
		}
		for rest := it.evidence; len(rest) > 0; {
			n := len(rest)
			if n > hexChunkRaw {
				n = hexChunkRaw
			}
			hex.Encode(buf, rest[:n])
			if _, err := w.Write(buf[:2*n]); err != nil {
				return err
			}
			rest = rest[n:]
		}
		if _, err := w.Write(it.suffix); err != nil {
			return err
		}
	}
	_, err := w.Write([]byte("]}"))
	return err
}

// planPortable is the document of ComposePortableWithFinality as pieces: every member in key order as json.Marshal writes a map, the
// finality member (when sec is not nil) written from the stream.
func planPortable(p *database.PortableV2, sec *finalitySectionStream) (portableBody, error) {
	doc, err := portableMembers(p)
	if err != nil {
		return portableBody{}, err
	}
	keys := make([]string, 0, len(doc)+1)
	for k := range doc {
		keys = append(keys, k)
	}
	if sec != nil {
		keys = append(keys, "finality")
	}
	sort.Strings(keys)
	var out portableBody
	fixed := []byte{'{'}
	for i, k := range keys {
		if i > 0 {
			fixed = append(fixed, ',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return portableBody{}, err
		}
		fixed = append(append(fixed, kb...), ':')
		if k == "finality" && sec != nil {
			out.pieces = append(out.pieces, fixedPiece(fixed), sec)
			fixed = nil
			continue
		}
		vb, err := json.Marshal(doc[k])
		if err != nil {
			return portableBody{}, err
		}
		fixed = append(fixed, vb...)
	}
	fixed = append(fixed, '}')
	out.pieces = append(out.pieces, fixedPiece(fixed))
	return out, nil
}

// extendWriteDeadline gives the response the time its size needs (a base window plus portableWritePerMiB per MiB). A writer that
// cannot take a deadline (a test recorder) keeps whatever it has; any other failure is returned.
func extendWriteDeadline(w http.ResponseWriter, bodyLen int64) error {
	window := portableWriteBase + time.Duration(bodyLen>>20)*portableWritePerMiB
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(window)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	return nil
}
