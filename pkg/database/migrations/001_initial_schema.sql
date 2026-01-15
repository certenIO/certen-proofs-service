-- ============================================================================
-- CERTEN PROOF ARTIFACT STORAGE SCHEMA
-- Migration: 001_initial_schema
-- Version: 1.0.0
-- Description: Initial schema for Certen proof artifact storage
--
-- This schema implements the proof storage architecture defined in:
-- - Technical Whitepaper Section 3.4 (Proof Generation and Verification)
-- - Implementation Plan Phase 1 (PostgreSQL Infrastructure)
-- ============================================================================

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- ANCHOR BATCHES
-- Represents a batch of transactions anchored together (on-cadence or on-demand)
-- Per Whitepaper Section 3.4.2: Validators batch transactions into blocks
-- ============================================================================
CREATE TABLE IF NOT EXISTS anchor_batches (
    batch_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Batch type determines anchoring strategy
    -- on_cadence: Regular ~15 min batches, ~$0.05/proof amortized
    -- on_demand: Immediate anchoring, ~$0.25/proof
    batch_type          VARCHAR(20) NOT NULL CHECK (batch_type IN ('on_cadence', 'on_demand')),

    -- Merkle tree data - the root hash of all transactions in this batch
    -- This is what gets written to external chains (ETH/BTC)
    merkle_root         BYTEA NOT NULL,         -- 32 bytes SHA256
    transaction_count   INTEGER NOT NULL DEFAULT 0,

    -- Timing - when this batch was opened and closed
    batch_start_time    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    batch_end_time      TIMESTAMPTZ,

    -- Accumulate state at batch time
    accumulate_block_height BIGINT,
    accumulate_block_hash   VARCHAR(64),

    -- Validator that created this batch
    validator_id        VARCHAR(64) NOT NULL,

    -- Status tracking
    -- pending: Batch is open, accepting transactions
    -- closed: Batch is closed, ready for anchoring
    -- anchoring: Anchor transaction submitted to external chain
    -- anchored: Anchor transaction confirmed (1+ confirmations)
    -- confirmed: Anchor has sufficient confirmations (e.g., 12+ for ETH)
    -- failed: Anchoring failed
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message       TEXT,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_batch_status CHECK (
        status IN ('pending', 'closed', 'anchoring', 'anchored', 'confirmed', 'failed')
    ),
    CONSTRAINT valid_merkle_root_length CHECK (length(merkle_root) = 32)
);

-- ============================================================================
-- BATCH TRANSACTIONS
-- Individual transactions included in an anchor batch
-- Each transaction has its Merkle inclusion proof for independent verification
-- ============================================================================
CREATE TABLE IF NOT EXISTS batch_transactions (
    id                  BIGSERIAL PRIMARY KEY,
    batch_id            UUID NOT NULL REFERENCES anchor_batches(batch_id) ON DELETE CASCADE,

    -- Original Accumulate transaction identity
    accumulate_tx_hash  VARCHAR(64) NOT NULL,    -- 32-byte hex (64 chars)
    account_url         VARCHAR(512) NOT NULL,   -- acc://...

    -- Position in Merkle tree (for proof reconstruction)
    -- tree_index is the leaf position (0-indexed)
    tree_index          INTEGER NOT NULL,

    -- Merkle inclusion proof - serialized path from leaf to root
    -- Format: JSON array of {hash: hex, position: 'left'|'right'}
    merkle_path         JSONB NOT NULL,

    -- Hash of this transaction's data (the leaf in the Merkle tree)
    transaction_hash    BYTEA NOT NULL,          -- 32 bytes

    -- Proof components from Accumulate (Layer A proofs)
    -- ChainedProof: L1 (tx→BVN), L2 (BVN→DN), L3 (DN→CometBFT)
    chained_proof       JSONB,
    chained_proof_valid BOOLEAN DEFAULT FALSE,

    -- GovernanceProof: G0 (inclusion), G1 (authority), G2 (outcome)
    governance_proof    JSONB,
    governance_level    VARCHAR(10),             -- 'G0', 'G1', 'G2'
    governance_valid    BOOLEAN DEFAULT FALSE,

    -- Intent data (if this tx is a CERTEN_INTENT)
    intent_type         VARCHAR(50),             -- e.g., 'CERTEN_INTENT'
    intent_data         JSONB,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_batch_tree_index UNIQUE(batch_id, tree_index),
    CONSTRAINT unique_tx_in_batch UNIQUE(batch_id, accumulate_tx_hash),
    CONSTRAINT valid_tx_hash_length CHECK (length(transaction_hash) = 32)
);

-- ============================================================================
-- ANCHOR RECORDS
-- Records of anchors written to external blockchains (ETH, BTC)
-- Per Whitepaper Section 3.4.2: Write root hash to designated anchor blockchain
-- ============================================================================
CREATE TABLE IF NOT EXISTS anchor_records (
    anchor_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id            UUID NOT NULL REFERENCES anchor_batches(batch_id) ON DELETE CASCADE,

    -- Target chain info
    target_chain        VARCHAR(20) NOT NULL,     -- 'ethereum', 'bitcoin'
    chain_id            VARCHAR(50),              -- e.g., 'ethereum-1', 'ethereum-11155111'
    network_name        VARCHAR(50),              -- e.g., 'mainnet', 'sepolia'
    contract_address    VARCHAR(42),              -- For ETH (0x...)

    -- Anchor transaction on external chain
    anchor_tx_hash      VARCHAR(66) NOT NULL,     -- 0x... (66 chars for ETH)
    anchor_block_number BIGINT NOT NULL,
    anchor_block_hash   VARCHAR(66),
    anchor_timestamp    TIMESTAMPTZ,

    -- What was anchored (copied from batch for independent verification)
    merkle_root         BYTEA NOT NULL,           -- Same as batch.merkle_root
    accumulate_height   BIGINT,                   -- Accumulate block height at anchor time

    -- Commitments written to chain (per CertenAnchorV2 contract)
    -- These are the 3 canonical commitments from the whitepaper
    operation_commitment   BYTEA,                 -- 32 bytes - tx batch merkle root
    cross_chain_commitment BYTEA,                 -- 32 bytes - cross-chain state
    governance_root        BYTEA,                 -- 32 bytes - governance proof root

    -- Confirmation tracking
    -- Per Whitepaper Section 3.4.3: Ensure anchor has sufficient confirmations
    confirmations       INTEGER NOT NULL DEFAULT 0,
    required_confirmations INTEGER NOT NULL DEFAULT 12,
    confirmed_at        TIMESTAMPTZ,
    is_final            BOOLEAN NOT NULL DEFAULT FALSE,

    -- Cost tracking
    gas_used            BIGINT,
    gas_price_wei       NUMERIC,
    total_cost_wei      NUMERIC,
    total_cost_usd      NUMERIC,

    -- Validator that created this anchor
    validator_id        VARCHAR(64) NOT NULL,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_anchor_merkle_root CHECK (length(merkle_root) = 32),
    CONSTRAINT unique_anchor_per_chain UNIQUE(batch_id, target_chain)
);

-- ============================================================================
-- CERTEN ANCHOR PROOFS
-- Complete proofs combining all 4 components from Whitepaper Section 3.4.1:
-- 1. Transaction Inclusion Proof (Merkle proof)
-- 2. Anchor Reference (external chain tx)
-- 3. State Proof (ChainedProof from Accumulate)
-- 4. Authority Proof (GovernanceProof)
-- ============================================================================
CREATE TABLE IF NOT EXISTS certen_anchor_proofs (
    proof_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Links to other records
    batch_id            UUID NOT NULL REFERENCES anchor_batches(batch_id),
    anchor_id           UUID REFERENCES anchor_records(anchor_id),
    transaction_id      BIGINT NOT NULL REFERENCES batch_transactions(id),

    -- Original Accumulate tx being proven
    accumulate_tx_hash  VARCHAR(64) NOT NULL,
    account_url         VARCHAR(512) NOT NULL,

    -- =========================================================================
    -- PROOF COMPONENT 1: Transaction Inclusion Proof
    -- Merkle proof that tx exists in the anchored batch
    -- =========================================================================
    merkle_root         BYTEA NOT NULL,           -- Root that was anchored
    merkle_inclusion_proof JSONB NOT NULL,        -- Path from tx to root

    -- =========================================================================
    -- PROOF COMPONENT 2: Anchor Reference
    -- Reference to where the root was anchored on external chain
    -- =========================================================================
    anchor_chain        VARCHAR(20) NOT NULL,
    anchor_tx_hash      VARCHAR(66) NOT NULL,
    anchor_block_number BIGINT NOT NULL,
    anchor_block_hash   VARCHAR(66),
    anchor_confirmations INTEGER NOT NULL DEFAULT 0,

    -- =========================================================================
    -- PROOF COMPONENT 3: State Proof (from ChainedProof)
    -- Cryptographic proof that tx exists on Accumulate
    -- =========================================================================
    accumulate_state_proof JSONB,                 -- Full ChainedProof (L1-L3)
    accumulate_block_height BIGINT,
    accumulate_bvn      VARCHAR(20),              -- e.g., 'bvn0', 'bvn1'

    -- =========================================================================
    -- PROOF COMPONENT 4: Authority Proof (from GovernanceProof)
    -- Proof that Key Book governance requirements were satisfied
    -- =========================================================================
    governance_proof    JSONB,                    -- Full G0/G1/G2 result
    governance_level    VARCHAR(10),              -- 'G0', 'G1', 'G2'
    governance_valid    BOOLEAN NOT NULL DEFAULT FALSE,

    -- =========================================================================
    -- Overall Verification Status
    -- =========================================================================
    -- Per Whitepaper Section 3.4.3: Verification is deterministic
    verified            BOOLEAN NOT NULL DEFAULT FALSE,
    verification_time   TIMESTAMPTZ,
    verification_details JSONB,

    -- Validator that generated this proof
    validator_id        VARCHAR(64) NOT NULL,
    validator_signature BYTEA,                    -- Validator signs the proof

    -- Proof version for forward compatibility
    proof_version       VARCHAR(20) NOT NULL DEFAULT '1.0.0',

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_tx_anchor_proof UNIQUE(accumulate_tx_hash, anchor_tx_hash),
    CONSTRAINT valid_proof_merkle_root CHECK (length(merkle_root) = 32)
);

-- ============================================================================
-- VALIDATOR ATTESTATIONS
-- Multi-validator consensus over proofs
-- Per Whitepaper Section 3.3: Quorum of validators must verify and co-sign
-- ============================================================================
CREATE TABLE IF NOT EXISTS validator_attestations (
    attestation_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proof_id            UUID NOT NULL REFERENCES certen_anchor_proofs(proof_id) ON DELETE CASCADE,

    -- Validator identity
    validator_id        VARCHAR(64) NOT NULL,
    validator_pubkey    BYTEA NOT NULL,           -- Ed25519 public key (32 bytes)

    -- Attestation
    -- The signature covers: proof_id || merkle_root || anchor_tx_hash
    signature           BYTEA NOT NULL,           -- Ed25519 signature (64 bytes)

    -- What was attested
    attested_merkle_root    BYTEA NOT NULL,
    attested_anchor_tx_hash VARCHAR(66) NOT NULL,

    attested_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_validator_attestation UNIQUE(proof_id, validator_id),
    CONSTRAINT valid_pubkey_length CHECK (length(validator_pubkey) = 32),
    CONSTRAINT valid_signature_length CHECK (length(signature) = 64)
);

-- ============================================================================
-- PROOF REQUESTS
-- Track incoming proof requests (both on-cadence and on-demand)
-- ============================================================================
CREATE TABLE IF NOT EXISTS proof_requests (
    request_id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Request details
    accumulate_tx_hash  VARCHAR(64),
    account_url         VARCHAR(512),

    -- Request type
    request_type        VARCHAR(20) NOT NULL DEFAULT 'on_cadence',
    priority            VARCHAR(10) NOT NULL DEFAULT 'normal',

    -- Status tracking
    status              VARCHAR(20) NOT NULL DEFAULT 'pending',

    -- Results
    batch_id            UUID REFERENCES anchor_batches(batch_id),
    proof_id            UUID REFERENCES certen_anchor_proofs(proof_id),

    -- Timing
    requested_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at        TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,

    -- Requester info
    requester_id        VARCHAR(256),

    -- Error tracking
    error_message       TEXT,
    retry_count         INTEGER NOT NULL DEFAULT 0,

    CONSTRAINT valid_request_type CHECK (request_type IN ('on_cadence', 'on_demand')),
    CONSTRAINT valid_priority CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    CONSTRAINT valid_request_status CHECK (
        status IN ('pending', 'processing', 'batched', 'completed', 'failed')
    )
);

-- ============================================================================
-- SCHEMA METADATA
-- Track schema version and migrations
-- ============================================================================
CREATE TABLE IF NOT EXISTS schema_migrations (
    version             VARCHAR(50) PRIMARY KEY,
    description         TEXT,
    applied_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Record this migration
INSERT INTO schema_migrations (version, description)
VALUES ('001_initial_schema', 'Initial schema for Certen proof artifact storage')
ON CONFLICT (version) DO NOTHING;

-- ============================================================================
-- INDEXES
-- Optimized for common query patterns
-- ============================================================================

-- Batch queries
CREATE INDEX IF NOT EXISTS idx_batch_status ON anchor_batches(status);
CREATE INDEX IF NOT EXISTS idx_batch_type ON anchor_batches(batch_type);
CREATE INDEX IF NOT EXISTS idx_batch_validator ON anchor_batches(validator_id);
CREATE INDEX IF NOT EXISTS idx_batch_created ON anchor_batches(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_batch_pending ON anchor_batches(status) WHERE status = 'pending';

-- Transaction queries
CREATE INDEX IF NOT EXISTS idx_tx_accumulate_hash ON batch_transactions(accumulate_tx_hash);
CREATE INDEX IF NOT EXISTS idx_tx_account_url ON batch_transactions(account_url);
CREATE INDEX IF NOT EXISTS idx_tx_batch ON batch_transactions(batch_id);
CREATE INDEX IF NOT EXISTS idx_tx_created ON batch_transactions(created_at DESC);

-- Anchor queries
CREATE INDEX IF NOT EXISTS idx_anchor_chain ON anchor_records(target_chain);
CREATE INDEX IF NOT EXISTS idx_anchor_tx_hash ON anchor_records(anchor_tx_hash);
CREATE INDEX IF NOT EXISTS idx_anchor_batch ON anchor_records(batch_id);
CREATE INDEX IF NOT EXISTS idx_anchor_confirmations ON anchor_records(confirmations) WHERE NOT is_final;
CREATE INDEX IF NOT EXISTS idx_anchor_unconfirmed ON anchor_records(created_at) WHERE NOT is_final;

-- Proof queries
CREATE INDEX IF NOT EXISTS idx_proof_tx_hash ON certen_anchor_proofs(accumulate_tx_hash);
CREATE INDEX IF NOT EXISTS idx_proof_anchor ON certen_anchor_proofs(anchor_tx_hash);
CREATE INDEX IF NOT EXISTS idx_proof_verified ON certen_anchor_proofs(verified);
CREATE INDEX IF NOT EXISTS idx_proof_validator ON certen_anchor_proofs(validator_id);
CREATE INDEX IF NOT EXISTS idx_proof_unverified ON certen_anchor_proofs(created_at) WHERE NOT verified;

-- Attestation queries
CREATE INDEX IF NOT EXISTS idx_attestation_proof ON validator_attestations(proof_id);
CREATE INDEX IF NOT EXISTS idx_attestation_validator ON validator_attestations(validator_id);

-- Request queries
CREATE INDEX IF NOT EXISTS idx_request_status ON proof_requests(status);
CREATE INDEX IF NOT EXISTS idx_request_pending ON proof_requests(requested_at) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_request_tx ON proof_requests(accumulate_tx_hash);

-- ============================================================================
-- FUNCTIONS
-- Utility functions for common operations
-- ============================================================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Triggers for updated_at
DROP TRIGGER IF EXISTS update_anchor_batches_updated_at ON anchor_batches;
CREATE TRIGGER update_anchor_batches_updated_at
    BEFORE UPDATE ON anchor_batches
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_anchor_records_updated_at ON anchor_records;
CREATE TRIGGER update_anchor_records_updated_at
    BEFORE UPDATE ON anchor_records
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_certen_anchor_proofs_updated_at ON certen_anchor_proofs;
CREATE TRIGGER update_certen_anchor_proofs_updated_at
    BEFORE UPDATE ON certen_anchor_proofs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- VIEWS
-- Convenient views for common queries
-- ============================================================================

-- View: Pending batches ready for anchoring
CREATE OR REPLACE VIEW v_batches_ready_for_anchoring AS
SELECT
    b.batch_id,
    b.batch_type,
    b.merkle_root,
    b.transaction_count,
    b.batch_start_time,
    b.validator_id,
    COUNT(bt.id) as actual_tx_count
FROM anchor_batches b
LEFT JOIN batch_transactions bt ON b.batch_id = bt.batch_id
WHERE b.status = 'closed'
GROUP BY b.batch_id;

-- View: Anchors awaiting confirmation
CREATE OR REPLACE VIEW v_anchors_awaiting_confirmation AS
SELECT
    ar.anchor_id,
    ar.batch_id,
    ar.target_chain,
    ar.anchor_tx_hash,
    ar.anchor_block_number,
    ar.confirmations,
    ar.required_confirmations,
    ar.created_at,
    b.merkle_root,
    b.transaction_count
FROM anchor_records ar
JOIN anchor_batches b ON ar.batch_id = b.batch_id
WHERE ar.is_final = FALSE;

-- View: Complete proof status
CREATE OR REPLACE VIEW v_proof_status AS
SELECT
    p.proof_id,
    p.accumulate_tx_hash,
    p.account_url,
    p.verified,
    p.governance_level,
    p.governance_valid,
    p.anchor_chain,
    p.anchor_tx_hash,
    p.anchor_confirmations,
    ar.is_final as anchor_final,
    COUNT(va.attestation_id) as attestation_count,
    p.created_at,
    p.verification_time
FROM certen_anchor_proofs p
LEFT JOIN anchor_records ar ON p.anchor_id = ar.anchor_id
LEFT JOIN validator_attestations va ON p.proof_id = va.proof_id
GROUP BY p.proof_id, ar.is_final;

-- ============================================================================
-- COMMENTS
-- Documentation for database objects
-- ============================================================================

COMMENT ON TABLE anchor_batches IS 'Batches of transactions anchored together to external chains';
COMMENT ON TABLE batch_transactions IS 'Individual transactions within an anchor batch with Merkle proofs';
COMMENT ON TABLE anchor_records IS 'Records of anchors written to external blockchains (ETH, BTC)';
COMMENT ON TABLE certen_anchor_proofs IS 'Complete Certen proofs combining all 4 whitepaper components';
COMMENT ON TABLE validator_attestations IS 'Multi-validator attestations over proofs';
COMMENT ON TABLE proof_requests IS 'Incoming proof requests (on-cadence and on-demand)';
COMMENT ON TABLE schema_migrations IS 'Schema version tracking';

COMMENT ON COLUMN anchor_batches.merkle_root IS 'SHA256 Merkle root of all transactions in batch - written to external chains';
COMMENT ON COLUMN anchor_batches.batch_type IS 'on_cadence (~15min, $0.05/proof) or on_demand (immediate, $0.25/proof)';
COMMENT ON COLUMN anchor_records.operation_commitment IS '32-byte commitment written to CertenAnchorV2 contract';
COMMENT ON COLUMN certen_anchor_proofs.merkle_inclusion_proof IS 'Merkle path from transaction to anchored root';
