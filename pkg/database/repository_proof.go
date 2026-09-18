// Copyright 2025 Certen Protocol
//
// Proof Repository - CRUD operations for Certen anchor proofs
// Per Whitepaper Section 3.4.1, a proof has 4 components:
// 1. Transaction Inclusion Proof (Merkle proof in batch)
// 2. Anchor Reference (ETH/BTC tx hash + block)
// 3. State Proof (ChainedProof from Accumulate L1-L3)
// 4. Authority Proof (GovernanceProof G0-G2)

package database

import (
	"context"
	"fmt"
)

// ProofRepository handles Certen anchor proof operations
type ProofRepository struct {
	client *Client
}

// NewProofRepository creates a new proof repository
func NewProofRepository(client *Client) *ProofRepository {
	return &ProofRepository{client: client}
}

// CurrentProofVersion is the current version of the proof format
const CurrentProofVersion = "1.0.0"

// ============================================================================
// PROOF QUERY OPERATIONS
// ============================================================================

// CountProofs returns the total number of proofs
func (r *ProofRepository) CountProofs(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM certen_anchor_proofs`

	var count int64
	err := r.client.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count proofs: %w", err)
	}

	return count, nil
}
