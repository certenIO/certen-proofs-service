// Copyright 2026 Certen Protocol
//
// The `finality` member of a portable proof v2 document (V2_SPEC_S7 section 2.5), composed at request time from the validators'
// append-only addendum rows, exactly as `majors` and `govRootV3Inputs` are composed in ComposePortable. It is evidence, never a
// verdict: this file reads the leg an addendum names from the CFE1 container header and refuses to compose rows that contradict
// each other, but it does not verify a hash, a chain or any evidence. A verifier holding the member and the proof's settlement
// does that, offline.

package server

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// FinalitySectionFormat is the format string of the `finality` member.
const FinalitySectionFormat = "certen-finality-addenda/1"

// The stored levels of an addendum (the table's CHECK allows exactly these).
const (
	finalityLevelL1Posted  = 2
	finalityLevelFinalized = 3
)

// Named states of a composition that cannot be served. A stored document that must not be served as a shorter or different one
// is a 500 that names which check failed and which addendum it concerns.
const (
	FinalityRowInvalid       = "finality_addendum_row_invalid"
	FinalityHeaderMalformed  = "finality_addendum_header_malformed"
	FinalityHeaderMismatch   = "finality_addendum_header_mismatch"
	FinalityLegMismatch      = "finality_leg_mismatch"
	FinalityMemberInDocument = "finality_member_stored_in_document"
	FinalityEvidenceTooLarge = "finality_evidence_too_large"
	FinalityStatusMalformed  = "finality_status_malformed"
)

// FinalityComposeError is a named composition state with the fields it was judged on (never evidence bytes).
type FinalityComposeError struct {
	Code   string
	Fields map[string]string
}

func (e *FinalityComposeError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(e.Code)
	for _, k := range keys {
		fmt.Fprintf(&b, " %s=%s", k, e.Fields[k])
	}
	return b.String()
}

func finalityErr(code string, kv ...string) *FinalityComposeError {
	e := &FinalityComposeError{Code: code, Fields: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		e.Fields[kv[i]] = kv[i+1]
	}
	return e
}

func seqStr(seq int64) string { return fmt.Sprintf("%d", seq) }

// finalityLeg is the block a chain of addenda is bound to: the (chain, blockNumber, blockHash) of every CFE1 header.
type finalityLeg struct {
	Chain       uint64 `json:"chain"`
	BlockNumber uint64 `json:"blockNumber"`
	BlockHash   string `json:"blockHash"`
}

// cfe1Header is the container header of V2_SPEC_S7 section 3.1: "CFE1"(4) || STR kind || u64 chain || u8 level || u64
// blockNumber || B32 blockHash, with STR = u32be length || ASCII.
type cfe1Header struct {
	Kind        string
	Chain       uint64
	Level       uint8
	BlockNumber uint64
	BlockHash   [32]byte
}

// parseCFE1Header reads the header from the first bytes of an evidence container. Only a prefix is needed; anything that does
// not fit the grammar is named.
func parseCFE1Header(b []byte) (cfe1Header, string) {
	var h cfe1Header
	if len(b) < 4 || string(b[:4]) != "CFE1" {
		return h, "magic"
	}
	b = b[4:]
	if len(b) < 4 {
		return h, "kind_length"
	}
	n := binary.BigEndian.Uint32(b)
	b = b[4:]
	if uint64(n) > uint64(len(b)) {
		return h, "kind_truncated"
	}
	h.Kind = string(b[:n])
	b = b[n:]
	if len(b) < 8+1+8+32 {
		return h, "fixed_part_truncated"
	}
	h.Chain = binary.BigEndian.Uint64(b)
	h.Level = b[8]
	h.BlockNumber = binary.BigEndian.Uint64(b[9:])
	copy(h.BlockHash[:], b[17:49])
	return h, ""
}

func finalityLevelName(level int16) (string, bool) {
	switch level {
	case finalityLevelL1Posted:
		return "l1_posted", true
	case finalityLevelFinalized:
		return "finalized", true
	}
	return "", false
}

// finalityHeadFacts are the columns of an addendum row the leg is read against; both the full rows (the portable member) and
// the evidence-less rows (the summary) reduce to it.
type finalityHeadFacts struct {
	Seq    int64
	Level  int16
	Chain  int64
	Kind   string
	Header []byte
}

// legOfRows is the single leg every addendum of a proof is bound to, read from each CFE1 header and checked against its own
// row: kind, chain and level must equal the row's, and all rows must name the same (chain, blockNumber, blockHash).
func legOfRows(rows []finalityHeadFacts) (finalityLeg, error) {
	var leg finalityLeg
	for i, r := range rows {
		if r.Chain <= 0 {
			return leg, finalityErr(FinalityRowInvalid, "seq", seqStr(r.Seq), "field", "chain")
		}
		h, bad := parseCFE1Header(r.Header)
		if bad != "" {
			return leg, finalityErr(FinalityHeaderMalformed, "seq", seqStr(r.Seq), "reason", bad)
		}
		if h.Kind != r.Kind {
			return leg, finalityErr(FinalityHeaderMismatch, "seq", seqStr(r.Seq), "field", "kind")
		}
		if h.Chain != uint64(r.Chain) {
			return leg, finalityErr(FinalityHeaderMismatch, "seq", seqStr(r.Seq), "field", "chain")
		}
		if int16(h.Level) != r.Level {
			return leg, finalityErr(FinalityHeaderMismatch, "seq", seqStr(r.Seq), "field", "level")
		}
		this := finalityLeg{Chain: h.Chain, BlockNumber: h.BlockNumber, BlockHash: hex.EncodeToString(h.BlockHash[:])}
		if i == 0 {
			leg = this
			continue
		}
		if this != leg {
			return finalityLeg{}, finalityErr(FinalityLegMismatch, "seq", seqStr(r.Seq))
		}
	}
	return leg, nil
}

// finalityAddendumJSON is the addendum of the portable form, in the validator's member order.
type finalityAddendumJSON struct {
	ProofID  string `json:"proofId"`
	Seq      int64  `json:"seq"`
	Level    string `json:"level"`
	Chain    int64  `json:"chain"`
	Kind     string `json:"kind"`
	Evidence string `json:"evidence"`
	PrevHash string `json:"prevHash"`
	OwnHash  string `json:"ownHash"`
}

type finalitySectionJSON struct {
	Format  string                 `json:"format"`
	Leg     finalityLeg            `json:"leg"`
	Addenda []finalityAddendumJSON `json:"addenda"`
}

// ComposeFinalitySection renders the spec-2.5 member for a proof's addendum rows (in seq order), or nil when there are none (the
// member is then absent from the document, not empty). Rows that name two legs, a header that disagrees with its own row, a
// level the table does not store or a hash that is not 32 bytes are refused by name; nothing is verified.
func ComposeFinalitySection(proofID uuid.UUID, rows []database.FinalityAddendum) (json.RawMessage, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	names, leg, err := validateFinalityRows(proofID, rows)
	if err != nil {
		return nil, err
	}
	section := finalitySectionJSON{Format: FinalitySectionFormat, Leg: leg, Addenda: make([]finalityAddendumJSON, 0, len(rows))}
	for i, r := range rows {
		section.Addenda = append(section.Addenda, finalityAddendumJSON{
			ProofID: r.ProofID.String(), Seq: r.Seq, Level: names[i], Chain: r.Chain, Kind: r.Kind,
			Evidence: hex.EncodeToString(r.Evidence), PrevHash: hex.EncodeToString(r.PrevHash), OwnHash: hex.EncodeToString(r.OwnHash),
		})
	}
	return json.Marshal(section)
}

// validateFinalityRows is every refusal ComposeFinalitySection and newFinalitySectionStream share: rows of another proof, a level the
// table does not store, a hash that is not 32 bytes, headers that disagree with their rows or name two legs. It returns the level name
// of each row and the leg. Nothing is verified.
func validateFinalityRows(proofID uuid.UUID, rows []database.FinalityAddendum) ([]string, finalityLeg, error) {
	facts := make([]finalityHeadFacts, len(rows))
	names := make([]string, len(rows))
	for i, r := range rows {
		if r.ProofID != proofID {
			return nil, finalityLeg{}, finalityErr(FinalityRowInvalid, "seq", seqStr(r.Seq), "field", "proofId")
		}
		name, ok := finalityLevelName(r.Level)
		if !ok {
			return nil, finalityLeg{}, finalityErr(FinalityRowInvalid, "seq", seqStr(r.Seq), "field", "level")
		}
		if len(r.PrevHash) != 32 {
			return nil, finalityLeg{}, finalityErr(FinalityRowInvalid, "seq", seqStr(r.Seq), "field", "prevHash")
		}
		if len(r.OwnHash) != 32 {
			return nil, finalityLeg{}, finalityErr(FinalityRowInvalid, "seq", seqStr(r.Seq), "field", "ownHash")
		}
		names[i] = name
		facts[i] = finalityHeadFacts{Seq: r.Seq, Level: r.Level, Chain: r.Chain, Kind: r.Kind, Header: r.Evidence}
	}
	leg, err := legOfRows(facts)
	if err != nil {
		return nil, finalityLeg{}, err
	}
	return names, leg, nil
}
