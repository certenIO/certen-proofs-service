// Copyright 2025 Certen Protocol
//
// Database Types for Certen Proof Artifact Storage
// These types map directly to the PostgreSQL schema defined in migrations/001_initial_schema.sql
//
// Per Technical Whitepaper Section 3.4:
// - Proof Architecture (3.4.1)
// - Anchor Creation Process (3.4.2)
// - Verification Process (3.4.3)

package database

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// BATCH TYPES
// ============================================================================

// BatchType represents the anchoring strategy
type BatchType string

const (
	// BatchTypeOnCadence is for regular ~15 min batches, ~$0.05/proof amortized
	BatchTypeOnCadence BatchType = "on_cadence"
	// BatchTypeOnDemand is for immediate anchoring, ~$0.25/proof
	BatchTypeOnDemand BatchType = "on_demand"
)

// BatchStatus represents the lifecycle of an anchor batch
type BatchStatus string

const (
	BatchStatusPending   BatchStatus = "pending"   // Batch is open, accepting transactions
	BatchStatusClosed    BatchStatus = "closed"    // Batch is closed, ready for anchoring
	BatchStatusAnchoring BatchStatus = "anchoring" // Anchor transaction submitted
	BatchStatusAnchored  BatchStatus = "anchored"  // Anchor transaction confirmed (1+ confirmations)
	BatchStatusConfirmed BatchStatus = "confirmed" // Anchor has sufficient confirmations
	BatchStatusFailed    BatchStatus = "failed"    // Anchoring failed
)

// AnchorBatch represents a batch of transactions anchored together
// Maps to: anchor_batches table
type AnchorBatch struct {
	BatchID      uuid.UUID   `db:"batch_id" json:"batch_id"`
	BatchType    BatchType   `db:"batch_type" json:"batch_type"`
	MerkleRoot   []byte      `db:"merkle_root" json:"merkle_root"`          // 32 bytes SHA256
	TxCount      int         `db:"transaction_count" json:"transaction_count"`
	StartTime    time.Time   `db:"batch_start_time" json:"batch_start_time"`
	EndTime      sql.NullTime `db:"batch_end_time" json:"batch_end_time,omitempty"`
	AccumHeight  sql.NullInt64 `db:"accumulate_block_height" json:"accumulate_block_height,omitempty"`
	AccumHash    sql.NullString `db:"accumulate_block_hash" json:"accumulate_block_hash,omitempty"`
	ValidatorID  string      `db:"validator_id" json:"validator_id"`
	Status       BatchStatus `db:"status" json:"status"`
	ErrorMessage sql.NullString `db:"error_message" json:"error_message,omitempty"`
	CreatedAt    time.Time   `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time   `db:"updated_at" json:"updated_at"`
}

// ============================================================================
// BATCH TRANSACTION TYPES
// ============================================================================

// MerklePathNode represents a single node in a Merkle proof path
type MerklePathNode struct {
	Hash     string `json:"hash"`     // Hex-encoded hash
	Position string `json:"position"` // "left" or "right"
}

// BatchTransaction represents an individual transaction in an anchor batch
// Maps to: batch_transactions table
type BatchTransaction struct {
	ID              int64          `db:"id" json:"id"`
	BatchID         uuid.UUID      `db:"batch_id" json:"batch_id"`
	AccumTxHash     string         `db:"accumulate_tx_hash" json:"accumulate_tx_hash"`
	AccountURL      string         `db:"account_url" json:"account_url"`
	TreeIndex       int            `db:"tree_index" json:"tree_index"`
	MerklePath      json.RawMessage `db:"merkle_path" json:"merkle_path"` // JSON array of MerklePathNode
	TxHash          []byte         `db:"transaction_hash" json:"transaction_hash"` // 32 bytes
	ChainedProof    json.RawMessage `db:"chained_proof" json:"chained_proof,omitempty"`
	ChainedValid    bool           `db:"chained_proof_valid" json:"chained_proof_valid"`
	GovProof        json.RawMessage `db:"governance_proof" json:"governance_proof,omitempty"`
	GovLevel        sql.NullString `db:"governance_level" json:"governance_level,omitempty"`
	GovValid        bool           `db:"governance_valid" json:"governance_valid"`
	IntentType      sql.NullString `db:"intent_type" json:"intent_type,omitempty"`
	IntentData      json.RawMessage `db:"intent_data" json:"intent_data,omitempty"`
	CreatedAt       time.Time      `db:"created_at" json:"created_at"`

	// Intent Metadata (for Transaction Center integration)
	UserID          sql.NullString `db:"user_id" json:"user_id,omitempty"`
	IntentID        sql.NullString `db:"intent_id" json:"intent_id,omitempty"`
	FromChain       sql.NullString `db:"from_chain" json:"from_chain,omitempty"`
	ToChain         sql.NullString `db:"to_chain" json:"to_chain,omitempty"`
	FromAddress     sql.NullString `db:"from_address" json:"from_address,omitempty"`
	ToAddress       sql.NullString `db:"to_address" json:"to_address,omitempty"`
	Amount          sql.NullString `db:"amount" json:"amount,omitempty"`
	TokenSymbol     sql.NullString `db:"token_symbol" json:"token_symbol,omitempty"`
	AdiURL          sql.NullString `db:"adi_url" json:"adi_url,omitempty"`
	CreatedAtClient sql.NullTime   `db:"created_at_client" json:"created_at_client,omitempty"`
}

// GetMerklePath deserializes the merkle path from JSON
func (bt *BatchTransaction) GetMerklePath() ([]MerklePathNode, error) {
	if bt.MerklePath == nil {
		return nil, nil
	}
	var path []MerklePathNode
	err := json.Unmarshal(bt.MerklePath, &path)
	return path, err
}

// ============================================================================
// ANCHOR RECORD TYPES
// ============================================================================

// TargetChain represents supported external blockchains
type TargetChain string

const (
	TargetChainEthereum TargetChain = "ethereum"
	TargetChainBitcoin  TargetChain = "bitcoin"
)

// AnchorRecord represents an anchor written to an external blockchain
// Maps to: anchor_records table
type AnchorRecord struct {
	AnchorID             uuid.UUID     `db:"anchor_id" json:"anchor_id"`
	BatchID              uuid.UUID     `db:"batch_id" json:"batch_id"`
	TargetChain          TargetChain   `db:"target_chain" json:"target_chain"`
	ChainID              sql.NullString `db:"chain_id" json:"chain_id,omitempty"`
	NetworkName          sql.NullString `db:"network_name" json:"network_name,omitempty"`
	ContractAddress      sql.NullString `db:"contract_address" json:"contract_address,omitempty"`
	AnchorTxHash         string        `db:"anchor_tx_hash" json:"anchor_tx_hash"`
	AnchorBlockNumber    int64         `db:"anchor_block_number" json:"anchor_block_number"`
	AnchorBlockHash      sql.NullString `db:"anchor_block_hash" json:"anchor_block_hash,omitempty"`
	AnchorTimestamp      sql.NullTime  `db:"anchor_timestamp" json:"anchor_timestamp,omitempty"`
	MerkleRoot           []byte        `db:"merkle_root" json:"merkle_root"` // 32 bytes
	AccumHeight          sql.NullInt64 `db:"accumulate_height" json:"accumulate_height,omitempty"`
	OperationCommitment  []byte        `db:"operation_commitment" json:"operation_commitment,omitempty"`
	CrossChainCommitment []byte        `db:"cross_chain_commitment" json:"cross_chain_commitment,omitempty"`
	GovernanceRoot       []byte        `db:"governance_root" json:"governance_root,omitempty"`
	Confirmations        int           `db:"confirmations" json:"confirmations"`
	RequiredConfirms     int           `db:"required_confirmations" json:"required_confirmations"`
	ConfirmedAt          sql.NullTime  `db:"confirmed_at" json:"confirmed_at,omitempty"`
	IsFinal              bool          `db:"is_final" json:"is_final"`
	GasUsed              sql.NullInt64 `db:"gas_used" json:"gas_used,omitempty"`
	GasPriceWei          sql.NullString `db:"gas_price_wei" json:"gas_price_wei,omitempty"` // NUMERIC as string
	TotalCostWei         sql.NullString `db:"total_cost_wei" json:"total_cost_wei,omitempty"`
	TotalCostUSD         sql.NullFloat64 `db:"total_cost_usd" json:"total_cost_usd,omitempty"`
	ValidatorID          string        `db:"validator_id" json:"validator_id"`
	CreatedAt            time.Time     `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time     `db:"updated_at" json:"updated_at"`
}

// ============================================================================
// CERTEN ANCHOR PROOF TYPES
// Per Whitepaper Section 3.4.1, a proof has 4 components:
// 1. Transaction Inclusion Proof
// 2. Anchor Reference
// 3. State Proof (ChainedProof)
// 4. Authority Proof (GovernanceProof)
// ============================================================================

// GovernanceLevel represents the governance proof level
type GovernanceLevel string

const (
	GovLevelG0 GovernanceLevel = "G0" // Inclusion and finality only
	GovLevelG1 GovernanceLevel = "G1" // Governance correctness (authority validated)
	GovLevelG2 GovernanceLevel = "G2" // Governance + outcome binding
)

// CertenAnchorProof represents a complete Certen proof
// Maps to: certen_anchor_proofs table
type CertenAnchorProof struct {
	ProofID          uuid.UUID       `db:"proof_id" json:"proof_id"`
	BatchID          uuid.UUID       `db:"batch_id" json:"batch_id"`
	AnchorID         uuid.NullUUID   `db:"anchor_id" json:"anchor_id,omitempty"`
	TransactionID    int64           `db:"transaction_id" json:"transaction_id"`

	// Original Accumulate tx
	AccumTxHash      string          `db:"accumulate_tx_hash" json:"accumulate_tx_hash"`
	AccountURL       string          `db:"account_url" json:"account_url"`

	// Component 1: Transaction Inclusion Proof
	MerkleRoot       []byte          `db:"merkle_root" json:"merkle_root"` // 32 bytes
	MerkleInclusion  json.RawMessage `db:"merkle_inclusion_proof" json:"merkle_inclusion_proof"`

	// Component 2: Anchor Reference
	AnchorChain       TargetChain     `db:"anchor_chain" json:"anchor_chain"`
	AnchorTxHash      string          `db:"anchor_tx_hash" json:"anchor_tx_hash"`
	AnchorBlockNumber int64           `db:"anchor_block_number" json:"anchor_block_number"`
	AnchorBlockHash   sql.NullString  `db:"anchor_block_hash" json:"anchor_block_hash,omitempty"`
	AnchorConfirms    int             `db:"anchor_confirmations" json:"anchor_confirmations"`

	// Component 3: State Proof (ChainedProof L1-L3)
	AccumStateProof  json.RawMessage `db:"accumulate_state_proof" json:"accumulate_state_proof,omitempty"`
	AccumBlockHeight sql.NullInt64   `db:"accumulate_block_height" json:"accumulate_block_height,omitempty"`
	AccumBVN         sql.NullString  `db:"accumulate_bvn" json:"accumulate_bvn,omitempty"`

	// Component 4: Authority Proof (GovernanceProof G0-G2)
	GovProof         json.RawMessage `db:"governance_proof" json:"governance_proof,omitempty"`
	GovLevel         sql.NullString  `db:"governance_level" json:"governance_level,omitempty"`
	GovValid         bool            `db:"governance_valid" json:"governance_valid"`

	// Verification status
	Verified         bool            `db:"verified" json:"verified"`
	VerificationTime sql.NullTime    `db:"verification_time" json:"verification_time,omitempty"`
	VerifyDetails    json.RawMessage `db:"verification_details" json:"verification_details,omitempty"`

	// Validator info
	ValidatorID      string          `db:"validator_id" json:"validator_id"`
	ValidatorSig     []byte          `db:"validator_signature" json:"validator_signature,omitempty"`

	// Metadata
	ProofVersion     string          `db:"proof_version" json:"proof_version"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
}

// GetMerkleInclusionProof deserializes the merkle inclusion proof
func (p *CertenAnchorProof) GetMerkleInclusionProof() ([]MerklePathNode, error) {
	if p.MerkleInclusion == nil {
		return nil, nil
	}
	var path []MerklePathNode
	err := json.Unmarshal(p.MerkleInclusion, &path)
	return path, err
}

// ============================================================================
// VALIDATOR ATTESTATION TYPES
// ============================================================================

// ValidatorAttestation represents a validator's attestation over a proof
// Maps to: validator_attestations table
type ValidatorAttestation struct {
	AttestationID     uuid.UUID `db:"attestation_id" json:"attestation_id"`
	ProofID           uuid.UUID `db:"proof_id" json:"proof_id"`
	ValidatorID       string    `db:"validator_id" json:"validator_id"`
	ValidatorPubkey   []byte    `db:"validator_pubkey" json:"validator_pubkey"` // 32 bytes Ed25519
	Signature         []byte    `db:"signature" json:"signature"`               // 64 bytes Ed25519
	AttestedMerkleRoot []byte   `db:"attested_merkle_root" json:"attested_merkle_root"`
	AttestedAnchorTx  string    `db:"attested_anchor_tx_hash" json:"attested_anchor_tx_hash"`
	AttestedAt        time.Time `db:"attested_at" json:"attested_at"`
}

// ============================================================================
// PROOF REQUEST TYPES
// ============================================================================

// RequestType represents the type of proof request
type RequestType string

const (
	RequestTypeOnCadence RequestType = "on_cadence"
	RequestTypeOnDemand  RequestType = "on_demand"
)

// RequestPriority represents the priority of a proof request
type RequestPriority string

const (
	PriorityLow    RequestPriority = "low"
	PriorityNormal RequestPriority = "normal"
	PriorityHigh   RequestPriority = "high"
	PriorityUrgent RequestPriority = "urgent"
)

// RequestStatus represents the status of a proof request
type RequestStatus string

const (
	RequestStatusPending    RequestStatus = "pending"
	RequestStatusProcessing RequestStatus = "processing"
	RequestStatusBatched    RequestStatus = "batched"
	RequestStatusCompleted  RequestStatus = "completed"
	RequestStatusFailed     RequestStatus = "failed"
)

// ProofRequest represents an incoming proof request
// Maps to: proof_requests table
type ProofRequest struct {
	RequestID    uuid.UUID       `db:"request_id" json:"request_id"`
	AccumTxHash  sql.NullString  `db:"accumulate_tx_hash" json:"accumulate_tx_hash,omitempty"`
	AccountURL   sql.NullString  `db:"account_url" json:"account_url,omitempty"`
	RequestType  RequestType     `db:"request_type" json:"request_type"`
	Priority     RequestPriority `db:"priority" json:"priority"`
	Status       RequestStatus   `db:"status" json:"status"`
	BatchID      uuid.NullUUID   `db:"batch_id" json:"batch_id,omitempty"`
	ProofID      uuid.NullUUID   `db:"proof_id" json:"proof_id,omitempty"`
	RequestedAt  time.Time       `db:"requested_at" json:"requested_at"`
	ProcessedAt  sql.NullTime    `db:"processed_at" json:"processed_at,omitempty"`
	CompletedAt  sql.NullTime    `db:"completed_at" json:"completed_at,omitempty"`
	RequesterID  sql.NullString  `db:"requester_id" json:"requester_id,omitempty"`
	ErrorMessage sql.NullString  `db:"error_message" json:"error_message,omitempty"`
	RetryCount   int             `db:"retry_count" json:"retry_count"`
}

// ============================================================================
// HELPER TYPES FOR INSERT/UPDATE OPERATIONS
// ============================================================================

// NewAnchorBatch is used to create a new batch
type NewAnchorBatch struct {
	BatchType   BatchType
	ValidatorID string
}

// NewBatchTransaction is used to add a transaction to a batch
type NewBatchTransaction struct {
	BatchID      uuid.UUID
	AccumTxHash  string
	AccountURL   string
	TreeIndex    int
	MerklePath   []MerklePathNode
	TxHash       []byte
	ChainedProof json.RawMessage // Optional
	GovProof     json.RawMessage // Optional
	GovLevel     GovernanceLevel // Optional
	IntentType   string          // Optional
	IntentData   json.RawMessage // Optional

	// Intent Metadata (for Transaction Center integration)
	UserID          string    // Firebase UID
	IntentID        string    // Firestore intent document ID
	FromChain       string    // Source chain
	ToChain         string    // Destination chain
	FromAddress     string    // Source address
	ToAddress       string    // Destination address
	Amount          string    // Amount as string
	TokenSymbol     string    // Token symbol
	AdiURL          string    // ADI URL
	CreatedAtClient time.Time // Client-side creation timestamp
}

// NewAnchorRecord is used to create a new anchor record
type NewAnchorRecord struct {
	BatchID              uuid.UUID
	TargetChain          TargetChain
	ChainID              string
	NetworkName          string
	ContractAddress      string
	AnchorTxHash         string
	AnchorBlockNumber    int64
	AnchorBlockHash      string
	MerkleRoot           []byte
	AccumHeight          int64
	OperationCommitment  []byte
	CrossChainCommitment []byte
	GovernanceRoot       []byte
	ValidatorID          string
	GasUsed              int64
	GasPriceWei          string
	TotalCostWei         string
}

// NewCertenAnchorProof is used to create a new proof
type NewCertenAnchorProof struct {
	BatchID          uuid.UUID
	AnchorID         uuid.UUID
	TransactionID    int64
	AccumTxHash      string
	AccountURL       string
	MerkleRoot       []byte
	MerkleInclusion  []MerklePathNode
	AnchorChain      TargetChain
	AnchorTxHash     string
	AnchorBlockNumber int64
	AnchorBlockHash  string
	AccumStateProof  json.RawMessage
	AccumBlockHeight int64
	AccumBVN         string
	GovProof         json.RawMessage
	GovLevel         GovernanceLevel
	ValidatorID      string
}

// NewProofRequest is used to create a new proof request
type NewProofRequest struct {
	AccumTxHash string
	AccountURL  string
	RequestType RequestType
	Priority    RequestPriority
	RequesterID string
}

// ============================================================================
// UUID HELPERS
// ============================================================================

// uuid.NullUUID for nullable UUIDs in database
type NullUUID = uuid.NullUUID

// ParseUUID parses a string into a UUID
func ParseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// NewUUID generates a new random UUID
func NewUUID() uuid.UUID {
	return uuid.New()
}

// ============================================================================
// MULTI-LEG INTENT TYPES
// Per Multi-Leg Intents Implementation Plan
// ============================================================================

// ExecutionMode represents how legs are executed in a multi-leg intent
type ExecutionMode string

const (
	ExecutionModeSequential ExecutionMode = "sequential" // Legs execute in order
	ExecutionModeParallel   ExecutionMode = "parallel"   // All legs execute simultaneously
	ExecutionModeAtomic     ExecutionMode = "atomic"     // All or nothing
)

// IntentStatus represents the overall status of a multi-leg intent
type IntentStatus string

const (
	IntentStatusDiscovered      IntentStatus = "discovered"
	IntentStatusProcessing      IntentStatus = "processing"
	IntentStatusAnchoring       IntentStatus = "anchoring"
	IntentStatusCompleted       IntentStatus = "completed"
	IntentStatusPartialComplete IntentStatus = "partial_complete"
	IntentStatusFailed          IntentStatus = "failed"
	IntentStatusRolledBack      IntentStatus = "rolled_back"
	IntentStatusExpired         IntentStatus = "expired"
)

// LegStatus represents the status of a single leg
type LegStatus string

const (
	LegStatusPending    LegStatus = "pending"
	LegStatusReady      LegStatus = "ready"
	LegStatusProcessing LegStatus = "processing"
	LegStatusBatched    LegStatus = "batched"
	LegStatusAnchored   LegStatus = "anchored"
	LegStatusConfirmed  LegStatus = "confirmed"
	LegStatusExecuted   LegStatus = "executed"
	LegStatusCompleted  LegStatus = "completed"
	LegStatusFailed     LegStatus = "failed"
	LegStatusSkipped    LegStatus = "skipped"
)

// LegRole represents the role of a leg in a multi-leg intent
type LegRole string

const (
	LegRoleSource       LegRole = "source"
	LegRoleDestination  LegRole = "destination"
	LegRoleIntermediate LegRole = "intermediate"
)

// DependencyCondition represents the condition type for leg dependencies
type DependencyCondition string

const (
	DependencySuccess      DependencyCondition = "success"      // Depends on successful completion
	DependencyCompletion   DependencyCondition = "completion"   // Depends on any completion
	DependencyConfirmation DependencyCondition = "confirmation" // Depends on anchor confirmation
)

// CertenIntent represents a master intent record (1-N legs)
// Maps to: certen_intents table
type CertenIntent struct {
	IntentID         string          `db:"intent_id" json:"intent_id"`
	OperationID      string          `db:"operation_id" json:"operation_id"`
	UserID           sql.NullString  `db:"user_id" json:"user_id,omitempty"`
	OrganizationADI  sql.NullString  `db:"organization_adi" json:"organization_adi,omitempty"`
	AccumulateTxHash string          `db:"accumulate_tx_hash" json:"accumulate_tx_hash"`
	AccountURL       sql.NullString  `db:"account_url" json:"account_url,omitempty"`
	Partition        sql.NullString  `db:"partition" json:"partition,omitempty"`
	LegCount         int             `db:"leg_count" json:"leg_count"`
	ExecutionMode    ExecutionMode   `db:"execution_mode" json:"execution_mode"`
	ProofClass       string          `db:"proof_class" json:"proof_class"`
	Status           IntentStatus    `db:"status" json:"status"`
	CurrentLegIndex  int             `db:"current_leg_index" json:"current_leg_index"`
	LegsCompleted    int             `db:"legs_completed" json:"legs_completed"`
	LegsFailed       int             `db:"legs_failed" json:"legs_failed"`
	LegsPending      int             `db:"legs_pending" json:"legs_pending"`
	IntentData       json.RawMessage `db:"intent_data" json:"intent_data"`
	CrossChainData   json.RawMessage `db:"cross_chain_data" json:"cross_chain_data"`
	GovernanceData   json.RawMessage `db:"governance_data" json:"governance_data"`
	ReplayData       json.RawMessage `db:"replay_data" json:"replay_data"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
	CompletedAt      sql.NullTime    `db:"completed_at" json:"completed_at,omitempty"`
	ExpiresAt        sql.NullTime    `db:"expires_at" json:"expires_at,omitempty"`
	ErrorMessage     sql.NullString  `db:"error_message" json:"error_message,omitempty"`
}

// IntentLeg represents a single leg within a multi-leg intent
// Maps to: intent_legs table
type IntentLeg struct {
	LegID           uuid.UUID      `db:"leg_id" json:"leg_id"`
	IntentID        string         `db:"intent_id" json:"intent_id"`
	LegIndex        int            `db:"leg_index" json:"leg_index"`
	LegExternalID   sql.NullString `db:"leg_external_id" json:"leg_external_id,omitempty"`
	TargetChain     string         `db:"target_chain" json:"target_chain"`
	ChainID         sql.NullInt64  `db:"chain_id" json:"chain_id,omitempty"`
	NetworkName     sql.NullString `db:"network_name" json:"network_name,omitempty"`
	Role            LegRole        `db:"role" json:"role"`
	SequenceOrder   int            `db:"sequence_order" json:"sequence_order"`
	DependsOnLegs   []string       `db:"depends_on_legs" json:"depends_on_legs,omitempty"` // TEXT[]
	FromAddress     sql.NullString `db:"from_address" json:"from_address,omitempty"`
	ToAddress       sql.NullString `db:"to_address" json:"to_address,omitempty"`
	Amount          sql.NullString `db:"amount" json:"amount,omitempty"`
	TokenSymbol     sql.NullString `db:"token_symbol" json:"token_symbol,omitempty"`
	TokenAddress    sql.NullString `db:"token_address" json:"token_address,omitempty"`
	AssetNative     bool           `db:"asset_native" json:"asset_native"`
	AssetDecimals   int            `db:"asset_decimals" json:"asset_decimals"`
	GasLimit        sql.NullInt64  `db:"gas_limit" json:"gas_limit,omitempty"`
	MaxFeePerGas    sql.NullString `db:"max_fee_per_gas" json:"max_fee_per_gas,omitempty"`
	MaxPriorityFee  sql.NullString `db:"max_priority_fee" json:"max_priority_fee,omitempty"`
	GasPayer        sql.NullString `db:"gas_payer" json:"gas_payer,omitempty"`
	AnchorContract  sql.NullString `db:"anchor_contract" json:"anchor_contract,omitempty"`
	FunctionSelector sql.NullString `db:"function_selector" json:"function_selector,omitempty"`
	Status          LegStatus      `db:"status" json:"status"`
	ExecutionTxHash sql.NullString `db:"execution_tx_hash" json:"execution_tx_hash,omitempty"`
	ExecutionBlock  sql.NullInt64  `db:"execution_block" json:"execution_block,omitempty"`
	ExecutionGasUsed sql.NullInt64 `db:"execution_gas_used" json:"execution_gas_used,omitempty"`
	ExecutionError  sql.NullString `db:"execution_error" json:"execution_error,omitempty"`
	BatchID         uuid.NullUUID  `db:"batch_id" json:"batch_id,omitempty"`
	AnchorID        uuid.NullUUID  `db:"anchor_id" json:"anchor_id,omitempty"`
	ProofID         uuid.NullUUID  `db:"proof_id" json:"proof_id,omitempty"`
	RetryCount      int            `db:"retry_count" json:"retry_count"`
	MaxRetries      int            `db:"max_retries" json:"max_retries"`
	LastRetryAt     sql.NullTime   `db:"last_retry_at" json:"last_retry_at,omitempty"`
	CreatedAt       time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at" json:"updated_at"`
	StartedAt       sql.NullTime   `db:"started_at" json:"started_at,omitempty"`
	CompletedAt     sql.NullTime   `db:"completed_at" json:"completed_at,omitempty"`
}

// LegDependency represents an explicit dependency between legs
// Maps to: leg_dependencies table
type LegDependency struct {
	DependencyID   uuid.UUID           `db:"dependency_id" json:"dependency_id"`
	IntentID       string              `db:"intent_id" json:"intent_id"`
	LegID          uuid.UUID           `db:"leg_id" json:"leg_id"`
	DependsOnLegID uuid.UUID           `db:"depends_on_leg_id" json:"depends_on_leg_id"`
	ConditionType  DependencyCondition `db:"condition_type" json:"condition_type"`
	IsSatisfied    bool                `db:"is_satisfied" json:"is_satisfied"`
	SatisfiedAt    sql.NullTime        `db:"satisfied_at" json:"satisfied_at,omitempty"`
	CreatedAt      time.Time           `db:"created_at" json:"created_at"`
}

// IntentChainGroup represents legs grouped by target chain
// Maps to: intent_chain_groups table
type IntentChainGroup struct {
	GroupID      uuid.UUID     `db:"group_id" json:"group_id"`
	IntentID     string        `db:"intent_id" json:"intent_id"`
	TargetChain  string        `db:"target_chain" json:"target_chain"`
	ChainID      sql.NullInt64 `db:"chain_id" json:"chain_id,omitempty"`
	ChainKey     string        `db:"chain_key" json:"chain_key"` // e.g., "ethereum:1"
	LegCount     int           `db:"leg_count" json:"leg_count"`
	LegIDs       []uuid.UUID   `db:"leg_ids" json:"leg_ids"` // UUID[]
	Status       LegStatus     `db:"status" json:"status"`
	BatchID      uuid.NullUUID `db:"batch_id" json:"batch_id,omitempty"`
	AnchorID     uuid.NullUUID `db:"anchor_id" json:"anchor_id,omitempty"`
	AnchorTxHash sql.NullString `db:"anchor_tx_hash" json:"anchor_tx_hash,omitempty"`
	AnchorBlock  sql.NullInt64 `db:"anchor_block" json:"anchor_block,omitempty"`
	CreatedAt    time.Time     `db:"created_at" json:"created_at"`
	AnchoredAt   sql.NullTime  `db:"anchored_at" json:"anchored_at,omitempty"`
	CompletedAt  sql.NullTime  `db:"completed_at" json:"completed_at,omitempty"`
}

// ============================================================================
// MULTI-LEG HELPER TYPES FOR INSERT/UPDATE OPERATIONS
// ============================================================================

// NewCertenIntent is used to create a new multi-leg intent
type NewCertenIntent struct {
	IntentID         string
	OperationID      string
	UserID           string
	OrganizationADI  string
	AccumulateTxHash string
	AccountURL       string
	Partition        string
	LegCount         int
	ExecutionMode    ExecutionMode
	ProofClass       string
	IntentData       json.RawMessage
	CrossChainData   json.RawMessage
	GovernanceData   json.RawMessage
	ReplayData       json.RawMessage
	ExpiresAt        time.Time
}

// NewIntentLeg is used to create a new leg within an intent
type NewIntentLeg struct {
	IntentID        string
	LegIndex        int
	LegExternalID   string
	TargetChain     string
	ChainID         int64
	NetworkName     string
	Role            LegRole
	SequenceOrder   int
	DependsOnLegs   []string
	FromAddress     string
	ToAddress       string
	Amount          string
	TokenSymbol     string
	TokenAddress    string
	AssetNative     bool
	AssetDecimals   int
	GasLimit        int64
	MaxFeePerGas    string
	MaxPriorityFee  string
	GasPayer        string
	AnchorContract  string
	FunctionSelector string
	MaxRetries      int
}

// NewLegDependency is used to create a new leg dependency
type NewLegDependency struct {
	IntentID       string
	LegID          uuid.UUID
	DependsOnLegID uuid.UUID
	ConditionType  DependencyCondition
}

// NewIntentChainGroup is used to create a new chain group
type NewIntentChainGroup struct {
	IntentID    string
	TargetChain string
	ChainID     int64
	ChainKey    string
	LegIDs      []uuid.UUID
}

// IntentLegProgress represents the progress of legs in an intent
type IntentLegProgress struct {
	IntentID      string   `json:"intent_id"`
	TotalLegs     int      `json:"total_legs"`
	CompletedLegs int      `json:"completed_legs"`
	FailedLegs    int      `json:"failed_legs"`
	PendingLegs   int      `json:"pending_legs"`
	TargetChains  []string `json:"target_chains"`
	Progress      float64  `json:"progress_percent"`
}

// IntentLegSummary provides a summary view of an intent with its legs
type IntentLegSummary struct {
	Intent       CertenIntent       `json:"intent"`
	Legs         []IntentLeg        `json:"legs"`
	ChainGroups  []IntentChainGroup `json:"chain_groups"`
	Dependencies []LegDependency    `json:"dependencies"`
}
