-- ============================================================================
-- Migration 010: verification_history table (code/migration drift fix)
-- ============================================================================
-- The repository (CreateVerificationRecord / GetVerificationHistory) reads and
-- writes `verification_history`, but migration 002 only ever created
-- `proof_verifications`. Existing deployments were hand-reconciled and have the
-- table, so a fresh `migrate` (or a DR rebuild) would 500 every proof-detail
-- request. This creates the table the code actually uses, with the exact
-- columns selected by GetVerificationHistory. Idempotent: a no-op where the
-- table already exists.

CREATE TABLE IF NOT EXISTS verification_history (
    verification_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id) ON DELETE CASCADE,

    verification_type   VARCHAR(50) NOT NULL,

    passed              BOOLEAN NOT NULL,
    error_message       TEXT,
    error_code          VARCHAR(50),

    verifier_id         VARCHAR(128),
    verification_method VARCHAR(100),

    duration_ms         INTEGER,

    artifacts_json      JSONB,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_verhist_proof ON verification_history(proof_id);
CREATE INDEX IF NOT EXISTS idx_verhist_type ON verification_history(verification_type);
CREATE INDEX IF NOT EXISTS idx_verhist_time ON verification_history(created_at DESC);
