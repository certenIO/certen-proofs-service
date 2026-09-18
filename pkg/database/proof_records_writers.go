// Copyright 2025 Certen Protocol
//
// The inserts the Level 4 proof records are stored through, shared with certen-validator
// (pkg/database/proof_artifact_repository.go). The validator writes these rows in production; this
// service carries the same statements so its repository API is complete and checked against the same
// shared schema.

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// ExternalChainResultInput matches the actual database schema for external_chain_results
type ExternalChainResultInput struct {
	ProofID               *uuid.UUID // Optional FK to proof_artifacts
	BundleID              []byte
	OperationID           []byte
	ChainType             string // ethereum, bitcoin, solana, polygon
	ChainID               int64
	NetworkName           string
	TxHash                []byte
	TxIndex               int
	TxGasUsed             int64
	TxFromAddress         []byte
	TxToAddress           []byte
	BlockNumber           int64
	BlockHash             []byte
	BlockTimestamp        time.Time
	StateRoot             []byte
	TransactionsRoot      []byte
	ReceiptsRoot          []byte
	ExecutionStatus       int // 0 or 1
	ExecutionSuccess      bool
	RevertReason          string
	ContractAddress       []byte
	LogsJSON              json.RawMessage
	ConfirmationBlocks    int
	RequiredConfirmations int
	IsFinalized           bool
	FinalizedAt           *time.Time // Set when IsFinalized is true
	ResultHash            []byte
	ObserverValidatorID   string
	ObservedAt            time.Time

	// Result hash chain binding. SequenceNumber is nil when the caller does not track the chain.
	SequenceNumber     *int64
	PreviousResultHash []byte
	AnchorProofHash    []byte

	// Execution evidence beyond the receipt
	ReturnData       []byte
	StorageProofJSON json.RawMessage
	StorageProofHash []byte
	ArtifactJSON     json.RawMessage

	// Validator set the result's attestations are counted against
	SnapshotID *uuid.UUID
}

// SaveExternalChainResultV2 creates a new external chain execution result matching the actual schema
func (r *ProofArtifactRepository) SaveExternalChainResultV2(ctx context.Context, input *ExternalChainResultInput) (uuid.UUID, error) {
	query := `
		INSERT INTO external_chain_results (
			proof_id, bundle_id, operation_id, chain_type, chain_id, network_name,
			tx_hash, tx_index, tx_gas_used, tx_from_address, tx_to_address,
			block_number, block_hash, block_timestamp,
			state_root, transactions_root, receipts_root,
			execution_status, execution_success, revert_reason, contract_address, logs_json,
			confirmation_blocks, required_confirmations, is_finalized, finalized_at,
			result_hash, observer_validator_id, observed_at,
			sequence_number, previous_result_hash, anchor_proof_hash,
			return_data, storage_proof_json, storage_proof_hash, artifact_json, snapshot_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29,
			$30, $31, $32, $33, $34, $35, $36, $37
		)
		RETURNING result_id`

	var resultID uuid.UUID
	err := r.db.QueryRowContext(ctx, query,
		input.ProofID, input.BundleID, input.OperationID, input.ChainType, input.ChainID, input.NetworkName,
		input.TxHash, input.TxIndex, input.TxGasUsed, input.TxFromAddress, input.TxToAddress,
		input.BlockNumber, input.BlockHash, input.BlockTimestamp,
		input.StateRoot, input.TransactionsRoot, input.ReceiptsRoot,
		input.ExecutionStatus, input.ExecutionSuccess, input.RevertReason, input.ContractAddress, nullableJSON(input.LogsJSON),
		input.ConfirmationBlocks, input.RequiredConfirmations, input.IsFinalized, input.FinalizedAt,
		input.ResultHash, input.ObserverValidatorID, input.ObservedAt,
		input.SequenceNumber, input.PreviousResultHash, input.AnchorProofHash,
		input.ReturnData, nullableJSON(input.StorageProofJSON), input.StorageProofHash, nullableJSON(input.ArtifactJSON), input.SnapshotID,
	).Scan(&resultID)

	if err != nil {
		return uuid.UUID{}, fmt.Errorf("failed to save external chain result: %w", err)
	}

	return resultID, nil
}

// NewBLSResultAttestation is the input for creating a BLS result attestation
type NewBLSResultAttestation struct {
	ResultID              uuid.UUID
	ResultHash            []byte
	BundleID              []byte
	MessageHash           []byte
	ValidatorID           string
	ValidatorAddress      []byte
	ValidatorIndex        int
	BLSSignature          []byte
	BLSPublicKey          []byte
	SignatureDomain       string
	AttestedBlockNumber   int64
	AttestedBlockHash     []byte
	ConfirmationsAtAttest int
	AttestationTime       time.Time

	// Validator set this attestation is counted against, the validator's weight in it, and whether the
	// signature point was checked to lie in the prime-order subgroup.
	SnapshotID    *uuid.UUID
	Weight        int64 // defaults to 1 when zero
	SubgroupValid bool
}

// BLSResultAttestationRecord represents a stored BLS result attestation
type BLSResultAttestationRecord struct {
	AttestationID         uuid.UUID
	ResultID              uuid.UUID
	ResultHash            []byte
	BundleID              []byte
	MessageHash           []byte
	ValidatorID           string
	ValidatorAddress      []byte
	ValidatorIndex        int
	BLSSignature          []byte
	BLSPublicKey          []byte
	SignatureDomain       string
	AttestedBlockNumber   int64
	AttestedBlockHash     []byte
	ConfirmationsAtAttest int
	SignatureValid        *bool
	VerifiedAt            *time.Time
	VerificationError     *string
	AttestationTime       time.Time
	CreatedAt             time.Time
}

// SaveBLSResultAttestation creates a new BLS result attestation in bls_result_attestations table
func (r *ProofArtifactRepository) SaveBLSResultAttestation(ctx context.Context, input *NewBLSResultAttestation) (*BLSResultAttestationRecord, error) {
	domain := input.SignatureDomain
	if domain == "" {
		domain = "CERTEN_RESULT_ATTESTATION_V1"
	}

	query := `
		INSERT INTO bls_result_attestations (
			result_id, result_hash, bundle_id, message_hash,
			validator_id, validator_address, validator_index,
			bls_signature, bls_public_key, signature_domain,
			attested_block_number, attested_block_hash, confirmations_at_attest,
			attestation_time, snapshot_id, weight, subgroup_valid
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		)
		ON CONFLICT (result_id, validator_id) DO UPDATE SET
			bls_signature = EXCLUDED.bls_signature,
			bls_public_key = EXCLUDED.bls_public_key,
			attestation_time = EXCLUDED.attestation_time,
			snapshot_id = COALESCE(EXCLUDED.snapshot_id, bls_result_attestations.snapshot_id),
			weight = EXCLUDED.weight,
			subgroup_valid = EXCLUDED.subgroup_valid
		RETURNING attestation_id, created_at`

	weight := input.Weight
	if weight == 0 {
		weight = 1
	}

	var att BLSResultAttestationRecord
	att.ResultID = input.ResultID
	att.ResultHash = input.ResultHash
	att.BundleID = input.BundleID
	att.MessageHash = input.MessageHash
	att.ValidatorID = input.ValidatorID
	att.ValidatorAddress = input.ValidatorAddress
	att.ValidatorIndex = input.ValidatorIndex
	att.BLSSignature = input.BLSSignature
	att.BLSPublicKey = input.BLSPublicKey
	att.SignatureDomain = domain
	att.AttestedBlockNumber = input.AttestedBlockNumber
	att.AttestedBlockHash = input.AttestedBlockHash
	att.ConfirmationsAtAttest = input.ConfirmationsAtAttest
	att.AttestationTime = input.AttestationTime

	err := r.db.QueryRowContext(ctx, query,
		input.ResultID, input.ResultHash, input.BundleID, input.MessageHash,
		input.ValidatorID, input.ValidatorAddress, input.ValidatorIndex,
		input.BLSSignature, input.BLSPublicKey, domain,
		input.AttestedBlockNumber, input.AttestedBlockHash, input.ConfirmationsAtAttest,
		input.AttestationTime, input.SnapshotID, weight, input.SubgroupValid,
	).Scan(&att.AttestationID, &att.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to save BLS result attestation: %w", err)
	}

	return &att, nil
}

// NewAggregatedBLSAttestation is the input for creating an aggregated BLS attestation
type NewAggregatedBLSAttestation struct {
	ResultID              uuid.UUID
	ResultHash            []byte
	BundleID              []byte
	MessageHash           []byte
	AttestedBlockNumber   int64
	AggregateSignature    []byte
	AggregatePublicKey    []byte
	ValidatorBitfield     []byte
	ValidatorCount        int
	ValidatorAddresses    [][]byte
	ValidatorIndices      []int32
	AttestationIDs        []uuid.UUID
	TotalVotingPower      string // Use string for NUMERIC(78,0)
	SignedVotingPower     string
	VotingPowerPercentage float64
	ThresholdNumerator    int
	ThresholdDenominator  int
	ThresholdMet          bool
	FirstAttestationAt    time.Time
	LastAttestationAt     time.Time
	AggregationHash       []byte

	// Validator set the weights were counted against, the participating validator ids, and whether
	// every aggregated attestation signed the same message.
	SnapshotID              *uuid.UUID
	ParticipantIDs          json.RawMessage
	MessageConsistencyValid bool
}

// AggregatedBLSAttestationRecord represents a stored aggregated BLS attestation
type AggregatedBLSAttestationRecord struct {
	AggregationID         uuid.UUID
	ResultID              uuid.UUID
	ResultHash            []byte
	BundleID              []byte
	MessageHash           []byte
	AttestedBlockNumber   int64
	AggregateSignature    []byte
	AggregatePublicKey    []byte
	ValidatorBitfield     []byte
	ValidatorCount        int
	TotalVotingPower      string
	SignedVotingPower     string
	VotingPowerPercentage float64
	ThresholdNumerator    int
	ThresholdDenominator  int
	ThresholdMet          bool
	FirstAttestationAt    time.Time
	LastAttestationAt     time.Time
	FinalizedAt           *time.Time
	AggregateVerified     *bool
	VerifiedAt            *time.Time
	VerificationError     *string
	AggregationHash       []byte
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// SaveAggregatedBLSAttestation creates a new aggregated BLS attestation in aggregated_bls_attestations table
func (r *ProofArtifactRepository) SaveAggregatedBLSAttestation(ctx context.Context, input *NewAggregatedBLSAttestation) (*AggregatedBLSAttestationRecord, error) {
	query := `
		INSERT INTO aggregated_bls_attestations (
			result_id, result_hash, bundle_id, message_hash, attested_block_number,
			aggregate_signature, aggregate_public_key, validator_bitfield,
			validator_count, validator_addresses, validator_indices, attestation_ids,
			total_voting_power, signed_voting_power, voting_power_percentage,
			threshold_numerator, threshold_denominator, threshold_met,
			first_attestation_at, last_attestation_at, aggregation_hash,
			snapshot_id, participant_ids, message_consistency_valid
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24
		)
		ON CONFLICT (result_id) DO UPDATE SET
			aggregate_signature = EXCLUDED.aggregate_signature,
			aggregate_public_key = EXCLUDED.aggregate_public_key,
			validator_bitfield = EXCLUDED.validator_bitfield,
			validator_count = EXCLUDED.validator_count,
			validator_addresses = EXCLUDED.validator_addresses,
			validator_indices = EXCLUDED.validator_indices,
			attestation_ids = EXCLUDED.attestation_ids,
			total_voting_power = EXCLUDED.total_voting_power,
			signed_voting_power = EXCLUDED.signed_voting_power,
			voting_power_percentage = EXCLUDED.voting_power_percentage,
			threshold_met = EXCLUDED.threshold_met,
			last_attestation_at = EXCLUDED.last_attestation_at,
			aggregation_hash = EXCLUDED.aggregation_hash,
			snapshot_id = COALESCE(EXCLUDED.snapshot_id, aggregated_bls_attestations.snapshot_id),
			participant_ids = COALESCE(EXCLUDED.participant_ids, aggregated_bls_attestations.participant_ids),
			message_consistency_valid = EXCLUDED.message_consistency_valid,
			updated_at = NOW()
		RETURNING aggregation_id, created_at, updated_at`

	var agg AggregatedBLSAttestationRecord
	agg.ResultID = input.ResultID
	agg.ResultHash = input.ResultHash
	agg.BundleID = input.BundleID
	agg.MessageHash = input.MessageHash
	agg.AttestedBlockNumber = input.AttestedBlockNumber
	agg.AggregateSignature = input.AggregateSignature
	agg.AggregatePublicKey = input.AggregatePublicKey
	agg.ValidatorBitfield = input.ValidatorBitfield
	agg.ValidatorCount = input.ValidatorCount
	agg.TotalVotingPower = input.TotalVotingPower
	agg.SignedVotingPower = input.SignedVotingPower
	agg.VotingPowerPercentage = input.VotingPowerPercentage
	agg.ThresholdNumerator = input.ThresholdNumerator
	agg.ThresholdDenominator = input.ThresholdDenominator
	agg.ThresholdMet = input.ThresholdMet
	agg.FirstAttestationAt = input.FirstAttestationAt
	agg.LastAttestationAt = input.LastAttestationAt
	agg.AggregationHash = input.AggregationHash

	// Convert arrays to pq-compatible types for PostgreSQL
	// BYTEA[] - use pq.ByteaArray
	byteaArray := pq.ByteaArray(input.ValidatorAddresses)

	// INTEGER[] - use pq.Int32Array
	int32Array := pq.Int32Array(input.ValidatorIndices)

	// UUID[] - convert to string array for PostgreSQL
	uuidStrings := make([]string, len(input.AttestationIDs))
	for i, id := range input.AttestationIDs {
		uuidStrings[i] = id.String()
	}
	uuidArray := pq.StringArray(uuidStrings)

	err := r.db.QueryRowContext(ctx, query,
		input.ResultID, input.ResultHash, input.BundleID, input.MessageHash, input.AttestedBlockNumber,
		input.AggregateSignature, input.AggregatePublicKey, input.ValidatorBitfield,
		input.ValidatorCount, byteaArray, int32Array, uuidArray,
		input.TotalVotingPower, input.SignedVotingPower, input.VotingPowerPercentage,
		input.ThresholdNumerator, input.ThresholdDenominator, input.ThresholdMet,
		input.FirstAttestationAt, input.LastAttestationAt, input.AggregationHash,
		input.SnapshotID, nullableJSON(input.ParticipantIDs), input.MessageConsistencyValid,
	).Scan(&agg.AggregationID, &agg.CreatedAt, &agg.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to save aggregated BLS attestation: %w", err)
	}

	return &agg, nil
}
