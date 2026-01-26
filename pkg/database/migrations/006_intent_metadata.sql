-- Migration: 006_intent_metadata.sql
-- Description: Add intent metadata to batch_transactions for Transaction Center integration
-- Created: 2026-01-26
--
-- This migration adds:
-- - Intent metadata columns to batch_transactions (user_id, intent_id, chain info, amount)
-- - Indexes for Transaction Center queries
-- - View for intent-to-proof mapping
--
-- Per Transaction Center Data Migration Analysis

BEGIN;

-- ============================================================================
-- TABLE MODIFICATIONS: batch_transactions Intent Metadata
-- ============================================================================
-- Add columns for Firestore intent metadata to enable PostgreSQL-based queries

ALTER TABLE batch_transactions
ADD COLUMN IF NOT EXISTS user_id VARCHAR(128),               -- Firebase UID
ADD COLUMN IF NOT EXISTS intent_id VARCHAR(128),             -- Firestore intent document ID
ADD COLUMN IF NOT EXISTS from_chain VARCHAR(64),             -- Source chain (e.g., 'accumulate')
ADD COLUMN IF NOT EXISTS to_chain VARCHAR(64),               -- Destination chain (e.g., 'ethereum')
ADD COLUMN IF NOT EXISTS from_address VARCHAR(256),          -- Source address
ADD COLUMN IF NOT EXISTS to_address VARCHAR(256),            -- Destination address
ADD COLUMN IF NOT EXISTS amount VARCHAR(78),                 -- Amount as string (supports uint256)
ADD COLUMN IF NOT EXISTS token_symbol VARCHAR(32),           -- Token symbol (e.g., 'ACME', 'ETH')
ADD COLUMN IF NOT EXISTS adi_url VARCHAR(256),               -- ADI URL for the account
ADD COLUMN IF NOT EXISTS created_at_client TIMESTAMPTZ;      -- Client-side creation timestamp

-- ============================================================================
-- INDEXES: Transaction Center Query Optimization
-- ============================================================================

-- Primary lookup by user and intent (most common query pattern)
CREATE INDEX IF NOT EXISTS idx_batch_tx_user_intent
ON batch_transactions(user_id, intent_id)
WHERE user_id IS NOT NULL;

-- User's transactions ordered by time (Operations mode)
CREATE INDEX IF NOT EXISTS idx_batch_tx_user_created
ON batch_transactions(user_id, created_at DESC)
WHERE user_id IS NOT NULL;

-- Intent lookup (for proof enrichment)
CREATE INDEX IF NOT EXISTS idx_batch_tx_intent
ON batch_transactions(intent_id)
WHERE intent_id IS NOT NULL;

-- Chain-based filtering for audit
CREATE INDEX IF NOT EXISTS idx_batch_tx_chains
ON batch_transactions(from_chain, to_chain)
WHERE from_chain IS NOT NULL;

-- Token-based filtering
CREATE INDEX IF NOT EXISTS idx_batch_tx_token
ON batch_transactions(token_symbol)
WHERE token_symbol IS NOT NULL;

-- ============================================================================
-- VIEW: Intent Proof Mapping
-- ============================================================================
-- Provides a denormalized view joining batch_transactions with proof data

CREATE OR REPLACE VIEW intent_proof_mapping AS
SELECT
    bt.id AS batch_tx_id,
    bt.batch_id,
    bt.user_id,
    bt.intent_id,
    bt.accumulate_tx_hash,
    bt.account_url,
    bt.from_chain,
    bt.to_chain,
    bt.from_address,
    bt.to_address,
    bt.amount,
    bt.token_symbol,
    bt.adi_url,
    bt.governance_level,
    bt.governance_valid,
    bt.chained_proof_valid,
    bt.created_at,
    bt.created_at_client,

    -- Batch information
    ab.status AS batch_status,
    ab.merkle_root AS batch_merkle_root,
    ab.transaction_count AS batch_tx_count,
    ab.quorum_reached AS batch_quorum_reached,

    -- Anchor information (if anchored)
    ar.anchor_tx_hash,
    ar.anchor_block_number,
    ar.target_chain AS anchor_chain,
    ar.confirmations AS anchor_confirmations,
    ar.is_final AS anchor_is_final,

    -- Proof artifact information (if exists)
    pa.proof_id,
    pa.status AS proof_status,
    pa.gov_level AS proof_gov_level,
    pa.anchored_at AS proof_anchored_at,
    pa.verified_at AS proof_verified_at,

    -- Attestation count
    (SELECT COUNT(*) FROM validator_attestations va
     WHERE va.proof_id = pa.proof_id) AS attestation_count

FROM batch_transactions bt
LEFT JOIN anchor_batches ab ON bt.batch_id = ab.batch_id
LEFT JOIN anchor_records ar ON ab.batch_id = ar.batch_id
LEFT JOIN proof_artifacts pa ON bt.accumulate_tx_hash = pa.accum_tx_hash;

-- ============================================================================
-- VIEW: User Intent Summary
-- ============================================================================
-- Lightweight view for listing user's intents with status

CREATE OR REPLACE VIEW user_intent_summary AS
SELECT
    bt.user_id,
    bt.intent_id,
    bt.accumulate_tx_hash,
    bt.from_chain,
    bt.to_chain,
    bt.amount,
    bt.token_symbol,
    bt.adi_url,
    bt.created_at,
    bt.created_at_client,

    -- Derive overall status
    CASE
        WHEN pa.status = 'verified' THEN 'completed'
        WHEN pa.status = 'anchored' THEN 'anchored'
        WHEN ab.status = 'confirmed' THEN 'confirmed'
        WHEN ab.status = 'anchored' THEN 'anchored'
        WHEN ab.status = 'anchoring' THEN 'anchoring'
        WHEN ab.status IN ('pending', 'closed') THEN 'batched'
        ELSE 'pending'
    END AS status,

    -- Progress indicators
    bt.governance_level,
    bt.governance_valid,
    bt.chained_proof_valid,
    ab.quorum_reached,

    -- Anchor info
    ar.confirmations AS anchor_confirmations,
    ar.is_final AS anchor_is_final,
    ar.anchor_tx_hash,

    -- Proof ID for detail lookups
    pa.proof_id

FROM batch_transactions bt
LEFT JOIN anchor_batches ab ON bt.batch_id = ab.batch_id
LEFT JOIN anchor_records ar ON ab.batch_id = ar.batch_id
LEFT JOIN proof_artifacts pa ON bt.accumulate_tx_hash = pa.accum_tx_hash
WHERE bt.user_id IS NOT NULL AND bt.intent_id IS NOT NULL;

-- ============================================================================
-- VIEW: Audit Trail Entry
-- ============================================================================
-- Combines custody_chain_events with intent context for audit queries

CREATE OR REPLACE VIEW intent_audit_trail AS
SELECT
    cce.event_id,
    cce.proof_id,
    bt.user_id,
    bt.intent_id,
    bt.accumulate_tx_hash,
    bt.adi_url,
    cce.event_type,
    cce.actor_id,
    cce.actor_type,
    cce.action,
    cce.previous_hash,
    cce.current_hash,
    cce.details,
    cce.created_at AS event_timestamp
FROM custody_chain_events cce
JOIN proof_artifacts pa ON cce.proof_id = pa.proof_id
LEFT JOIN batch_transactions bt ON pa.accum_tx_hash = bt.accumulate_tx_hash;

-- ============================================================================
-- FUNCTION: Get Intent Progress Stage
-- ============================================================================
-- Maps proof status and governance level to UI stage number (1-9)

CREATE OR REPLACE FUNCTION get_intent_stage(
    p_batch_status VARCHAR,
    p_anchor_confirmations INTEGER,
    p_anchor_is_final BOOLEAN,
    p_governance_level VARCHAR,
    p_proof_status VARCHAR
) RETURNS INTEGER AS $$
BEGIN
    -- Stage 9: Fully verified with G2
    IF p_proof_status = 'verified' AND p_governance_level = 'G2' THEN
        RETURN 9;
    END IF;

    -- Stage 8: Verified with G1
    IF p_proof_status = 'verified' AND p_governance_level = 'G1' THEN
        RETURN 8;
    END IF;

    -- Stage 7: Verified with G0
    IF p_proof_status = 'verified' THEN
        RETURN 7;
    END IF;

    -- Stage 6: Anchor finalized
    IF p_anchor_is_final THEN
        RETURN 6;
    END IF;

    -- Stage 5: Anchor confirming
    IF p_anchor_confirmations > 0 THEN
        RETURN 5;
    END IF;

    -- Stage 4: Anchored
    IF p_proof_status = 'anchored' OR p_batch_status = 'anchored' THEN
        RETURN 4;
    END IF;

    -- Stage 3: Anchoring
    IF p_batch_status = 'anchoring' THEN
        RETURN 3;
    END IF;

    -- Stage 2: Batched
    IF p_batch_status IN ('pending', 'closed') THEN
        RETURN 2;
    END IF;

    -- Stage 1: Submitted
    RETURN 1;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- ============================================================================
-- MIGRATION RECORD
-- ============================================================================

INSERT INTO schema_migrations (version, description, applied_at)
VALUES ('006', 'Intent metadata for Transaction Center integration', NOW())
ON CONFLICT (version) DO NOTHING;

COMMIT;
