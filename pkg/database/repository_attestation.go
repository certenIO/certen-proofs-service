// Copyright 2025 Certen Protocol
//
// Attestation Repository - CRUD operations for validator attestations over proofs
// Validators sign attestations to cryptographically endorse proof validity

package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// AttestationRepository handles validator attestation operations
type AttestationRepository struct {
	client *Client
}

// NewAttestationRepository creates a new attestation repository
func NewAttestationRepository(client *Client) *AttestationRepository {
	return &AttestationRepository{client: client}
}

// ============================================================================
// VALIDATOR ATTESTATION OPERATIONS
// ============================================================================

// NewValidatorAttestation is used to create a new attestation
type NewValidatorAttestation struct {
	ProofID            uuid.UUID
	ValidatorID        string
	ValidatorPubkey    []byte // 32 bytes Ed25519 public key
	Signature          []byte // 64 bytes Ed25519 signature
	AttestedMerkleRoot []byte // The merkle root being attested
	AttestedAnchorTx   string // The anchor tx hash being attested
}

// CountAttestationsForProof returns the number of attestations for a proof
func (r *AttestationRepository) CountAttestationsForProof(ctx context.Context, proofID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM validator_attestations WHERE proof_id = $1`

	var count int
	err := r.client.QueryRowContext(ctx, query, proofID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count attestations: %w", err)
	}

	return count, nil
}

// CountAttestationsByValidator returns the total number of attestations by a validator
func (r *AttestationRepository) CountAttestationsByValidator(ctx context.Context, validatorID string) (int64, error) {
	query := `SELECT COUNT(*) FROM validator_attestations WHERE validator_id = $1`

	var count int64
	err := r.client.QueryRowContext(ctx, query, validatorID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count validator attestations: %w", err)
	}

	return count, nil
}

// CountTotalAttestations returns the total number of attestations in the system
func (r *AttestationRepository) CountTotalAttestations(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM validator_attestations`

	var count int64
	err := r.client.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count total attestations: %w", err)
	}

	return count, nil
}

// GetDistinctValidators returns a list of distinct validator IDs that have submitted attestations
func (r *AttestationRepository) GetDistinctValidators(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT validator_id FROM validator_attestations ORDER BY validator_id`

	rows, err := r.client.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query distinct validators: %w", err)
	}
	defer rows.Close()

	var validators []string
	for rows.Next() {
		var validatorID string
		if err := rows.Scan(&validatorID); err != nil {
			return nil, fmt.Errorf("failed to scan validator ID: %w", err)
		}
		validators = append(validators, validatorID)
	}

	return validators, rows.Err()
}
