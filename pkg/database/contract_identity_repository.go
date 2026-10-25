// Copyright 2026 Certen Protocol

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// MaxContractIdentityBytes bounds the stored and served container. It equals the validator's MaxContractIdentityBytes
// (pkg/contractidentity/limits.go), the octet_length check of migration 00031 and the api-gateway's bound; each repository pins the number
// in its own test, so a change in one is a failing test in the others' CI, not a silent drift.
const MaxContractIdentityBytes int64 = 4194304

// ErrContractIdentityTooLarge: the stored container is longer than MaxContractIdentityBytes. It is never truncated and never omitted.
var ErrContractIdentityTooLarge = errors.New("the stored contract-identity container exceeds MaxContractIdentityBytes")

// ContractIdentityRow is the greatest-seq row of proof_contract_identity for a proof.
type ContractIdentityRow struct {
	ProofID        uuid.UUID
	Seq            int64
	Chain          int64
	BlockNumber    int64
	BlockHash      []byte
	Evidence       []byte
	EvidenceSHA256 []byte
	Digest         []byte
}

// GetContractIdentity returns the served row of a proof (the greatest seq) or (nil, nil) when it has none. The size is measured in the
// database first; a container above the bound is ErrContractIdentityTooLarge and its bytes are never read.
func (r *ProofArtifactRepository) GetContractIdentity(ctx context.Context, proofID uuid.UUID) (*ContractIdentityRow, error) {
	row := &ContractIdentityRow{ProofID: proofID}
	var size int64
	err := r.db.QueryRowContext(ctx, `SELECT seq, chain, block_number, block_hash, octet_length(evidence), evidence_sha256, digest
		FROM proof_contract_identity WHERE proof_id = $1 ORDER BY seq DESC LIMIT 1`, proofID).
		Scan(&row.Seq, &row.Chain, &row.BlockNumber, &row.BlockHash, &size, &row.EvidenceSHA256, &row.Digest)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the contract identity of %s: %w", proofID, err)
	}
	if size > MaxContractIdentityBytes {
		return nil, fmt.Errorf("%w: %d bytes", ErrContractIdentityTooLarge, size)
	}
	// the same seq: a row appended between the two statements is a later seq and is not mixed in
	if err := r.db.QueryRowContext(ctx, `SELECT evidence FROM proof_contract_identity WHERE proof_id = $1 AND seq = $2`, proofID, row.Seq).Scan(&row.Evidence); err != nil {
		return nil, fmt.Errorf("read the contract-identity evidence of %s seq %d: %w", proofID, row.Seq, err)
	}
	return row, nil
}
