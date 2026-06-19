-- ============================================================================
-- Migration 011: reconcile proof_requests columns (migration drift fix)
-- ============================================================================
-- `proof_requests` is defined in BOTH 001 and 003 via CREATE TABLE IF NOT
-- EXISTS with incompatible shapes. If 001 ran first, 003's body is skipped and
-- the columns the repository inserts (proof_class, governance_level,
-- api_key_id, callback_url, created_at, ...) are missing, so CreateProofRequest
-- 500s on a fresh DB. This adds any missing columns idempotently (nullable, no
-- constraints — the app supplies values). A no-op where 003's shape is present.

ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS accum_tx_hash    VARCHAR(128);
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS account_url      VARCHAR(512);
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS proof_class      VARCHAR(20);
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS governance_level VARCHAR(10);
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS api_key_id       UUID;
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS callback_url     VARCHAR(1024);
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS status           VARCHAR(30) DEFAULT 'pending';
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS proof_id         UUID;
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS error_message    TEXT;
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS retry_count      INTEGER DEFAULT 0;
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS created_at       TIMESTAMPTZ DEFAULT NOW();
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS processed_at     TIMESTAMPTZ;
ALTER TABLE proof_requests ADD COLUMN IF NOT EXISTS completed_at     TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_requests_status_reconciled ON proof_requests(status);
CREATE INDEX IF NOT EXISTS idx_requests_created_reconciled ON proof_requests(created_at DESC);
