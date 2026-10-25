// Copyright 2026 Certen Protocol

package database

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
)

// legacyLegHeaderItem is the capture item under which the validators store the reconstructed header of a legacy leg (a stored execution
// proof written before block_header_rlp was captured; certen-validator pkg/finality/legacyhdr ItemLegacyHeader).
const legacyLegHeaderItem = "legacyleg/header"

// maxLegBundleBytes bounds a stored bundle, compressed and decompressed, before it is read for its header (the validator's own bound).
const maxLegBundleBytes = 16 << 20

// LegHeaderRLP is the RLP of the settlement block header of a proof's leg, for the optional leg.headerRlp of the portable finality
// member: the newest stored bundle's execution proof block_header_rlp (proof_components.5_execution_proof) or, for a legacy leg whose
// bundle has none, the header row the validators reconstructed and verified (proof_finality_capture, item legacyleg/header, keyed by
// the leg's chain, block number and block hash). It returns nil, nil when there is no header. It does not hash anything: the caller
// checks the bytes against the leg's block hash, and a verifier checks them again.
func (r *ProofArtifactRepository) LegHeaderRLP(ctx context.Context, proofID uuid.UUID, chain, number uint64, hash [32]byte) ([]byte, error) {
	var data []byte
	err := r.db.QueryRowContext(ctx, `SELECT CASE WHEN octet_length(bundle_data) <= $2 THEN bundle_data END FROM proof_bundles
		WHERE proof_id = $1 ORDER BY created_at DESC, bundle_id DESC LIMIT 1`, proofID, maxLegBundleBytes).Scan(&data)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("reading the newest bundle of proof %s: %w", proofID, err)
	}
	if h := headerOfBundle(data); len(h) > 0 {
		return h, nil
	}
	var stored []byte
	err = r.db.QueryRowContext(ctx, `SELECT bytes FROM proof_finality_capture WHERE chain = $1 AND l1_block = $2 AND l1_hash = $3 AND item = $4`,
		int64(chain), int64(number), hash[:], legacyLegHeaderItem).Scan(&stored)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the reconstructed header of proof %s: %w", proofID, err)
	}
	return stored, nil
}

// headerOfBundle reads proof_components.5_execution_proof.block_header_rlp (a 0x-prefixed hex string) of a stored, gzip-compressed
// certen_v1 bundle; nil when the bundle is absent, unreadable, or has no usable header (a legacy bundle).
func headerOfBundle(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	defer zr.Close()
	doc, err := io.ReadAll(io.LimitReader(zr, maxLegBundleBytes+1))
	if err != nil || len(doc) > maxLegBundleBytes {
		return nil
	}
	var b struct {
		ProofComponents struct {
			Execution struct {
				Header string `json:"block_header_rlp"`
			} `json:"5_execution_proof"`
		} `json:"proof_components"`
	}
	if json.Unmarshal(doc, &b) != nil {
		return nil
	}
	h, err := hex.DecodeString(strings.TrimPrefix(b.ProofComponents.Execution.Header, "0x"))
	if err != nil {
		return nil
	}
	return h
}
