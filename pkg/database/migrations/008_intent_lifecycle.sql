-- Migration 008: Intent Lifecycle Status Tracking
-- Unified tracking of intent lifecycle: submitted → pending_signatures → authorized → in_process → complete | failed
-- Note: The table may already exist if created by the validator's migration 007.
-- Using IF NOT EXISTS throughout to be idempotent.

INSERT INTO schema_migrations (version, applied_at) VALUES ('008_intent_lifecycle', NOW())
ON CONFLICT (version) DO NOTHING;

CREATE TABLE IF NOT EXISTS intent_lifecycle (
    id              BIGSERIAL PRIMARY KEY,
    intent_id       VARCHAR(256) NOT NULL,
    accum_tx_hash   VARCHAR(128) NOT NULL,
    user_id         VARCHAR(256),
    status          VARCHAR(32) NOT NULL DEFAULT 'submitted',
    target_chain    VARCHAR(64),
    proof_class     VARCHAR(20),
    error_message   TEXT,
    block_height    BIGINT,
    cycle_id        VARCHAR(256),
    write_back_tx   VARCHAR(128),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    submitted_at    TIMESTAMPTZ,
    authorized_at   TIMESTAMPTZ,
    in_process_at   TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    failed_at       TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_intent_lifecycle_intent_id ON intent_lifecycle(intent_id);
CREATE INDEX IF NOT EXISTS idx_intent_lifecycle_accum_tx_hash ON intent_lifecycle(accum_tx_hash);
CREATE INDEX IF NOT EXISTS idx_intent_lifecycle_status ON intent_lifecycle(status);
CREATE INDEX IF NOT EXISTS idx_intent_lifecycle_user_id ON intent_lifecycle(user_id);
CREATE INDEX IF NOT EXISTS idx_intent_lifecycle_created_at ON intent_lifecycle(created_at DESC);
