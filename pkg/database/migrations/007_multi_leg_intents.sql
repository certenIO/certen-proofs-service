-- ============================================================================
-- CERTEN MULTI-LEG INTENTS SCHEMA
-- Migration: 007_multi_leg_intents
-- Version: 1.0.0
-- Description: Support for multi-leg intents - single user intent with multiple
--              independent legs targeting same or different chains
--
-- This schema implements:
-- - Master Intent Registry (certen_intents)
-- - Per-Leg Tracking (intent_legs)
-- - Leg Dependencies for execution coordination
-- - Foreign key additions to existing tables
-- ============================================================================

BEGIN;

-- ============================================================================
-- MASTER INTENT REGISTRY
-- Represents a single user-constructed intent that may have 1-N legs
-- All legs belong to one atomic workflow under a single Accumulate transaction
-- ============================================================================
CREATE TABLE IF NOT EXISTS certen_intents (
    -- Primary identification
    intent_id           VARCHAR(128) PRIMARY KEY,
    operation_id        VARCHAR(128) NOT NULL,       -- Hash of all 4 blobs (OperationID)

    -- User and organization
    user_id             VARCHAR(256),                -- Firebase UID or identifier
    organization_adi    VARCHAR(512),                -- acc://organization.acme

    -- Accumulate transaction reference
    accumulate_tx_hash  VARCHAR(128) NOT NULL,       -- The single Accumulate tx for this intent
    account_url         VARCHAR(512),                -- Data account URL
    partition           VARCHAR(50),                 -- BVN partition (e.g., 'bvn1')

    -- Multi-leg configuration
    leg_count           INTEGER NOT NULL DEFAULT 1,
    execution_mode      VARCHAR(20) NOT NULL DEFAULT 'sequential',
    proof_class         VARCHAR(20) NOT NULL DEFAULT 'on_demand',

    -- Overall intent status
    -- discovered: Intent found in Accumulate
    -- processing: Legs are being processed
    -- anchoring: Anchoring in progress across chains
    -- completed: All legs completed successfully
    -- partial_complete: Some legs completed, some failed
    -- failed: All legs failed or critical failure
    -- rolled_back: Atomic execution rolled back
    status              VARCHAR(30) NOT NULL DEFAULT 'discovered',

    -- Progress tracking
    current_leg_index   INTEGER NOT NULL DEFAULT 0,   -- For sequential execution
    legs_completed      INTEGER NOT NULL DEFAULT 0,
    legs_failed         INTEGER NOT NULL DEFAULT 0,
    legs_pending        INTEGER NOT NULL DEFAULT 0,

    -- 4 JSON Blobs (stored for reference/replay)
    intent_data         JSONB NOT NULL,               -- Blob 1: Metadata
    cross_chain_data    JSONB NOT NULL,               -- Blob 2: Legs array
    governance_data     JSONB NOT NULL,               -- Blob 3: Authorization
    replay_data         JSONB NOT NULL,               -- Blob 4: Nonce/expiry

    -- Timing
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ,                  -- From replay_data

    -- Error tracking
    error_message       TEXT,

    CONSTRAINT valid_execution_mode CHECK (
        execution_mode IN ('sequential', 'parallel', 'atomic')
    ),
    CONSTRAINT valid_proof_class CHECK (
        proof_class IN ('on_demand', 'on_cadence')
    ),
    CONSTRAINT valid_intent_status CHECK (
        status IN ('discovered', 'processing', 'anchoring', 'completed',
                   'partial_complete', 'failed', 'rolled_back', 'expired')
    ),
    CONSTRAINT valid_leg_count CHECK (leg_count >= 1)
);

-- ============================================================================
-- PER-LEG TRACKING
-- Each leg within a multi-leg intent
-- A leg targets a specific chain and may have dependencies on other legs
-- ============================================================================
CREATE TABLE IF NOT EXISTS intent_legs (
    leg_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    intent_id           VARCHAR(128) NOT NULL REFERENCES certen_intents(intent_id) ON DELETE CASCADE,

    -- Leg identification within intent
    leg_index           INTEGER NOT NULL,             -- 0-indexed position
    leg_external_id     VARCHAR(128),                 -- LegID from cross_chain_data

    -- Target chain information
    target_chain        VARCHAR(50) NOT NULL,         -- 'ethereum', 'polygon', 'solana'
    chain_id            BIGINT,                       -- Chain ID (1, 137, etc.)
    network_name        VARCHAR(50),                  -- 'mainnet', 'sepolia', etc.

    -- Leg role and ordering
    role                VARCHAR(30) NOT NULL DEFAULT 'destination',
    sequence_order      INTEGER NOT NULL DEFAULT 0,   -- For sequential execution
    depends_on_legs     TEXT[],                       -- Array of leg_external_ids this depends on

    -- Transaction details
    from_address        VARCHAR(256),
    to_address          VARCHAR(256),
    amount              VARCHAR(100),                 -- String to support uint256
    token_symbol        VARCHAR(32),
    token_address       VARCHAR(256),

    -- Asset details from CCLeg
    asset_native        BOOLEAN DEFAULT FALSE,
    asset_decimals      INTEGER DEFAULT 18,

    -- Gas policy
    gas_limit           BIGINT,
    max_fee_per_gas     VARCHAR(50),
    max_priority_fee    VARCHAR(50),
    gas_payer           VARCHAR(256),

    -- Anchor contract for this leg's chain
    anchor_contract     VARCHAR(256),
    function_selector   VARCHAR(20),

    -- Leg status
    -- pending: Waiting to be processed or for dependencies
    -- ready: Dependencies satisfied, ready for execution
    -- processing: Being included in batch
    -- batched: Added to anchor batch
    -- anchored: Anchor written to target chain
    -- confirmed: Anchor confirmed on target chain
    -- executed: Transaction executed on target chain
    -- completed: Full cycle complete
    -- failed: Execution failed
    -- skipped: Skipped due to atomic rollback
    status              VARCHAR(30) NOT NULL DEFAULT 'pending',

    -- Execution results
    execution_tx_hash   VARCHAR(256),                 -- TX hash on target chain
    execution_block     BIGINT,
    execution_gas_used  BIGINT,
    execution_error     TEXT,

    -- Links to batch system
    batch_id            UUID,                         -- FK added below after table exists
    anchor_id           UUID,                         -- FK added below
    proof_id            UUID,                         -- FK added below

    -- Retry tracking
    retry_count         INTEGER NOT NULL DEFAULT 0,
    max_retries         INTEGER DEFAULT 3,
    last_retry_at       TIMESTAMPTZ,

    -- Timing
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,

    CONSTRAINT unique_intent_leg_index UNIQUE (intent_id, leg_index),
    CONSTRAINT unique_intent_leg_external_id UNIQUE (intent_id, leg_external_id),
    CONSTRAINT valid_leg_role CHECK (
        role IN ('source', 'destination', 'intermediate')
    ),
    CONSTRAINT valid_leg_status CHECK (
        status IN ('pending', 'ready', 'processing', 'batched', 'anchored',
                   'confirmed', 'executed', 'completed', 'failed', 'skipped')
    )
);

-- Add foreign keys for batch system references
-- These reference tables from 001_initial_schema
ALTER TABLE intent_legs
ADD CONSTRAINT fk_intent_legs_batch
FOREIGN KEY (batch_id) REFERENCES anchor_batches(batch_id) ON DELETE SET NULL;

ALTER TABLE intent_legs
ADD CONSTRAINT fk_intent_legs_anchor
FOREIGN KEY (anchor_id) REFERENCES anchor_records(anchor_id) ON DELETE SET NULL;

-- proof_id references proof_artifacts from 002_comprehensive_proof_schema
-- Adding as optional since proof_artifacts may not exist for all legs

-- ============================================================================
-- LEG DEPENDENCIES
-- Explicit dependency tracking between legs for complex execution patterns
-- ============================================================================
CREATE TABLE IF NOT EXISTS leg_dependencies (
    dependency_id       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    intent_id           VARCHAR(128) NOT NULL REFERENCES certen_intents(intent_id) ON DELETE CASCADE,
    leg_id              UUID NOT NULL REFERENCES intent_legs(leg_id) ON DELETE CASCADE,
    depends_on_leg_id   UUID NOT NULL REFERENCES intent_legs(leg_id) ON DELETE CASCADE,

    -- Dependency type
    -- success: Depends on successful completion
    -- completion: Depends on any completion (success or failure)
    -- confirmation: Depends on anchor confirmation
    condition_type      VARCHAR(20) NOT NULL DEFAULT 'success',

    -- Tracking
    is_satisfied        BOOLEAN NOT NULL DEFAULT FALSE,
    satisfied_at        TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_dependency UNIQUE (leg_id, depends_on_leg_id),
    CONSTRAINT no_self_dependency CHECK (leg_id != depends_on_leg_id),
    CONSTRAINT valid_condition_type CHECK (
        condition_type IN ('success', 'completion', 'confirmation')
    )
);

-- ============================================================================
-- CHAIN GROUPS
-- Track legs grouped by target chain for efficient anchoring
-- One anchor call per chain group
-- ============================================================================
CREATE TABLE IF NOT EXISTS intent_chain_groups (
    group_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    intent_id           VARCHAR(128) NOT NULL REFERENCES certen_intents(intent_id) ON DELETE CASCADE,

    -- Chain identification
    target_chain        VARCHAR(50) NOT NULL,
    chain_id            BIGINT,
    chain_key           VARCHAR(100) NOT NULL,        -- e.g., 'ethereum:1', 'polygon:137'

    -- Legs in this group
    leg_count           INTEGER NOT NULL DEFAULT 0,
    leg_ids             UUID[],                       -- Array of leg_ids in this group

    -- Group status
    status              VARCHAR(30) NOT NULL DEFAULT 'pending',

    -- Anchoring for this group
    batch_id            UUID REFERENCES anchor_batches(batch_id),
    anchor_id           UUID REFERENCES anchor_records(anchor_id),
    anchor_tx_hash      VARCHAR(256),
    anchor_block        BIGINT,

    -- Timing
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    anchored_at         TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ,

    CONSTRAINT unique_intent_chain_group UNIQUE (intent_id, chain_key),
    CONSTRAINT valid_chain_group_status CHECK (
        status IN ('pending', 'batched', 'anchoring', 'anchored', 'confirmed',
                   'executed', 'completed', 'failed')
    )
);

-- ============================================================================
-- ALTER EXISTING TABLES
-- Add leg references to existing batch system tables
-- ============================================================================

-- batch_transactions: Link individual transactions to legs
ALTER TABLE batch_transactions
ADD COLUMN IF NOT EXISTS leg_id UUID;

ALTER TABLE batch_transactions
ADD COLUMN IF NOT EXISTS multi_leg_intent_id VARCHAR(128);

-- anchor_records: Track which intent/leg this anchor serves
ALTER TABLE anchor_records
ADD COLUMN IF NOT EXISTS intent_id VARCHAR(128);

ALTER TABLE anchor_records
ADD COLUMN IF NOT EXISTS leg_ids UUID[];

ALTER TABLE anchor_records
ADD COLUMN IF NOT EXISTS chain_group_id UUID;

-- proof_artifacts: Link to specific leg
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'proof_artifacts') THEN
        ALTER TABLE proof_artifacts
        ADD COLUMN IF NOT EXISTS leg_id UUID;

        ALTER TABLE proof_artifacts
        ADD COLUMN IF NOT EXISTS multi_leg_intent_id VARCHAR(128);
    END IF;
END $$;

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Intent queries
CREATE INDEX IF NOT EXISTS idx_intents_operation_id ON certen_intents(operation_id);
CREATE INDEX IF NOT EXISTS idx_intents_user_id ON certen_intents(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_intents_status ON certen_intents(status);
CREATE INDEX IF NOT EXISTS idx_intents_accumulate_tx ON certen_intents(accumulate_tx_hash);
CREATE INDEX IF NOT EXISTS idx_intents_created ON certen_intents(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_intents_organization ON certen_intents(organization_adi) WHERE organization_adi IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_intents_execution_mode ON certen_intents(execution_mode);
CREATE INDEX IF NOT EXISTS idx_intents_pending ON certen_intents(created_at)
    WHERE status IN ('discovered', 'processing', 'anchoring');

-- Leg queries
CREATE INDEX IF NOT EXISTS idx_legs_intent ON intent_legs(intent_id);
CREATE INDEX IF NOT EXISTS idx_legs_chain ON intent_legs(target_chain, chain_id);
CREATE INDEX IF NOT EXISTS idx_legs_status ON intent_legs(status);
CREATE INDEX IF NOT EXISTS idx_legs_batch ON intent_legs(batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_legs_anchor ON intent_legs(anchor_id) WHERE anchor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_legs_pending ON intent_legs(intent_id, sequence_order)
    WHERE status IN ('pending', 'ready');
CREATE INDEX IF NOT EXISTS idx_legs_execution_tx ON intent_legs(execution_tx_hash)
    WHERE execution_tx_hash IS NOT NULL;

-- Dependency queries
CREATE INDEX IF NOT EXISTS idx_deps_leg ON leg_dependencies(leg_id);
CREATE INDEX IF NOT EXISTS idx_deps_depends_on ON leg_dependencies(depends_on_leg_id);
CREATE INDEX IF NOT EXISTS idx_deps_unsatisfied ON leg_dependencies(leg_id) WHERE NOT is_satisfied;
CREATE INDEX IF NOT EXISTS idx_deps_intent ON leg_dependencies(intent_id);

-- Chain group queries
CREATE INDEX IF NOT EXISTS idx_chain_groups_intent ON intent_chain_groups(intent_id);
CREATE INDEX IF NOT EXISTS idx_chain_groups_chain ON intent_chain_groups(target_chain, chain_id);
CREATE INDEX IF NOT EXISTS idx_chain_groups_status ON intent_chain_groups(status);
CREATE INDEX IF NOT EXISTS idx_chain_groups_pending ON intent_chain_groups(intent_id)
    WHERE status NOT IN ('completed', 'failed');

-- Existing table indexes for new columns
CREATE INDEX IF NOT EXISTS idx_batch_tx_leg ON batch_transactions(leg_id) WHERE leg_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_batch_tx_multi_intent ON batch_transactions(multi_leg_intent_id)
    WHERE multi_leg_intent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_anchor_intent ON anchor_records(intent_id) WHERE intent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_anchor_chain_group ON anchor_records(chain_group_id) WHERE chain_group_id IS NOT NULL;

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Update updated_at on certen_intents
DROP TRIGGER IF EXISTS update_certen_intents_updated_at ON certen_intents;
CREATE TRIGGER update_certen_intents_updated_at
    BEFORE UPDATE ON certen_intents
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Update updated_at on intent_legs
DROP TRIGGER IF EXISTS update_intent_legs_updated_at ON intent_legs;
CREATE TRIGGER update_intent_legs_updated_at
    BEFORE UPDATE ON intent_legs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- VIEWS
-- ============================================================================

-- View: Intent with leg summary
CREATE OR REPLACE VIEW v_intent_leg_summary AS
SELECT
    i.intent_id,
    i.operation_id,
    i.user_id,
    i.accumulate_tx_hash,
    i.leg_count,
    i.execution_mode,
    i.proof_class,
    i.status AS intent_status,
    i.legs_completed,
    i.legs_failed,
    i.legs_pending,
    i.created_at,
    i.completed_at,

    -- Aggregate leg information
    ARRAY_AGG(DISTINCT l.target_chain) AS target_chains,
    COUNT(DISTINCT l.target_chain) AS chain_count,
    SUM(CASE WHEN l.status = 'completed' THEN 1 ELSE 0 END) AS actual_completed,
    SUM(CASE WHEN l.status = 'failed' THEN 1 ELSE 0 END) AS actual_failed,
    SUM(CASE WHEN l.status IN ('pending', 'ready') THEN 1 ELSE 0 END) AS actual_pending

FROM certen_intents i
LEFT JOIN intent_legs l ON i.intent_id = l.intent_id
GROUP BY i.intent_id;

-- View: Legs ready for execution (no unsatisfied dependencies)
CREATE OR REPLACE VIEW v_legs_ready_for_execution AS
SELECT
    l.leg_id,
    l.intent_id,
    l.leg_index,
    l.leg_external_id,
    l.target_chain,
    l.chain_id,
    l.status,
    l.sequence_order,
    i.execution_mode,
    i.proof_class
FROM intent_legs l
JOIN certen_intents i ON l.intent_id = i.intent_id
WHERE l.status = 'pending'
  AND i.status IN ('discovered', 'processing')
  AND NOT EXISTS (
      SELECT 1 FROM leg_dependencies d
      WHERE d.leg_id = l.leg_id
        AND d.is_satisfied = FALSE
  );

-- View: Chain groups with leg details
CREATE OR REPLACE VIEW v_chain_group_details AS
SELECT
    cg.group_id,
    cg.intent_id,
    cg.target_chain,
    cg.chain_id,
    cg.chain_key,
    cg.leg_count,
    cg.status AS group_status,
    cg.anchor_tx_hash,
    i.execution_mode,
    i.proof_class,
    i.status AS intent_status,
    ARRAY_AGG(l.leg_id ORDER BY l.sequence_order) AS ordered_leg_ids,
    ARRAY_AGG(l.status ORDER BY l.sequence_order) AS leg_statuses
FROM intent_chain_groups cg
JOIN certen_intents i ON cg.intent_id = i.intent_id
LEFT JOIN intent_legs l ON l.intent_id = cg.intent_id
    AND l.target_chain = cg.target_chain
GROUP BY cg.group_id, i.execution_mode, i.proof_class, i.status;

-- View: Multi-leg intent progress
CREATE OR REPLACE VIEW v_multi_leg_progress AS
SELECT
    i.intent_id,
    i.operation_id,
    i.user_id,
    i.leg_count,
    i.execution_mode,
    i.status AS intent_status,
    i.created_at,

    -- Progress percentage
    CASE
        WHEN i.leg_count = 0 THEN 0
        ELSE ROUND((i.legs_completed::NUMERIC / i.leg_count) * 100, 1)
    END AS progress_percent,

    -- Chain summary
    (SELECT COUNT(DISTINCT target_chain) FROM intent_legs WHERE intent_id = i.intent_id) AS unique_chains,

    -- Status breakdown
    (SELECT jsonb_object_agg(status, cnt) FROM (
        SELECT status, COUNT(*) as cnt
        FROM intent_legs
        WHERE intent_id = i.intent_id
        GROUP BY status
    ) s) AS leg_status_breakdown,

    -- Estimated completion (for sequential mode)
    CASE
        WHEN i.execution_mode = 'sequential' AND i.status = 'processing'
        THEN (SELECT l.leg_external_id FROM intent_legs l
              WHERE l.intent_id = i.intent_id AND l.status IN ('pending', 'ready', 'processing')
              ORDER BY l.sequence_order LIMIT 1)
        ELSE NULL
    END AS current_leg

FROM certen_intents i
WHERE i.leg_count > 1;

-- ============================================================================
-- FUNCTIONS
-- ============================================================================

-- Function: Check if all dependencies for a leg are satisfied
CREATE OR REPLACE FUNCTION check_leg_dependencies_satisfied(p_leg_id UUID)
RETURNS BOOLEAN AS $$
BEGIN
    RETURN NOT EXISTS (
        SELECT 1 FROM leg_dependencies
        WHERE leg_id = p_leg_id AND is_satisfied = FALSE
    );
END;
$$ LANGUAGE plpgsql STABLE;

-- Function: Mark dependency as satisfied and check if dependent leg is ready
CREATE OR REPLACE FUNCTION satisfy_leg_dependency(
    p_completed_leg_id UUID,
    p_condition VARCHAR DEFAULT 'success'
)
RETURNS TABLE(ready_leg_id UUID, ready_leg_intent_id VARCHAR) AS $$
BEGIN
    -- Mark dependencies as satisfied
    UPDATE leg_dependencies
    SET is_satisfied = TRUE, satisfied_at = NOW()
    WHERE depends_on_leg_id = p_completed_leg_id
      AND (condition_type = p_condition OR condition_type = 'completion');

    -- Return legs that are now ready (all dependencies satisfied)
    RETURN QUERY
    SELECT DISTINCT d.leg_id, i.intent_id
    FROM leg_dependencies d
    JOIN intent_legs l ON d.leg_id = l.leg_id
    JOIN certen_intents i ON l.intent_id = i.intent_id
    WHERE d.depends_on_leg_id = p_completed_leg_id
      AND l.status = 'pending'
      AND check_leg_dependencies_satisfied(d.leg_id);
END;
$$ LANGUAGE plpgsql;

-- Function: Update intent status based on leg completion
CREATE OR REPLACE FUNCTION update_intent_on_leg_change()
RETURNS TRIGGER AS $$
DECLARE
    v_total INTEGER;
    v_completed INTEGER;
    v_failed INTEGER;
    v_pending INTEGER;
    v_new_status VARCHAR(30);
BEGIN
    -- Count leg statuses
    SELECT
        COUNT(*),
        SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END),
        SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END),
        SUM(CASE WHEN status IN ('pending', 'ready', 'processing', 'batched') THEN 1 ELSE 0 END)
    INTO v_total, v_completed, v_failed, v_pending
    FROM intent_legs
    WHERE intent_id = NEW.intent_id;

    -- Determine new intent status
    IF v_completed = v_total THEN
        v_new_status := 'completed';
    ELSIF v_failed > 0 AND v_pending = 0 THEN
        v_new_status := 'partial_complete';
    ELSIF v_failed = v_total THEN
        v_new_status := 'failed';
    ELSIF v_completed > 0 OR v_failed > 0 THEN
        v_new_status := 'processing';
    ELSE
        v_new_status := 'discovered';
    END IF;

    -- Update intent
    UPDATE certen_intents
    SET legs_completed = v_completed,
        legs_failed = v_failed,
        legs_pending = v_pending,
        status = v_new_status,
        completed_at = CASE WHEN v_new_status IN ('completed', 'partial_complete', 'failed')
                           THEN NOW() ELSE NULL END
    WHERE intent_id = NEW.intent_id;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger: Update intent when leg status changes
DROP TRIGGER IF EXISTS trg_update_intent_on_leg_change ON intent_legs;
CREATE TRIGGER trg_update_intent_on_leg_change
    AFTER UPDATE OF status ON intent_legs
    FOR EACH ROW
    WHEN (OLD.status IS DISTINCT FROM NEW.status)
    EXECUTE FUNCTION update_intent_on_leg_change();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON TABLE certen_intents IS 'Master registry for multi-leg intents (1-N legs per intent)';
COMMENT ON TABLE intent_legs IS 'Individual legs within a multi-leg intent';
COMMENT ON TABLE leg_dependencies IS 'Explicit dependencies between legs for execution ordering';
COMMENT ON TABLE intent_chain_groups IS 'Legs grouped by target chain for efficient anchoring';

COMMENT ON COLUMN certen_intents.execution_mode IS 'sequential: legs execute in order; parallel: all at once; atomic: all or nothing';
COMMENT ON COLUMN certen_intents.leg_count IS 'Total number of legs in this intent';
COMMENT ON COLUMN intent_legs.depends_on_legs IS 'Array of leg_external_ids this leg depends on';
COMMENT ON COLUMN intent_legs.sequence_order IS 'Order for sequential execution (0-indexed)';
COMMENT ON COLUMN intent_chain_groups.chain_key IS 'Unique key for chain (e.g., ethereum:1, polygon:137)';

-- ============================================================================
-- MIGRATION RECORD
-- ============================================================================

INSERT INTO schema_migrations (version, description, applied_at)
VALUES ('007', 'Multi-leg intents support', NOW())
ON CONFLICT (version) DO NOTHING;

COMMIT;
