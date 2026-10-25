//go:build pgtest

package database

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// validatorMigration00031 is certen-validator db/migrations/00031_proof_contract_identity.sql (its sha256 is the RequiredSchema pin).
const validatorMigration00031 = `-- S8 contract identity: the capture rows and the final containers (V2_SPEC_S8 Q2 "WHERE stored", RB12 EPSILON WP-S8-5).
-- NUMBERED 00031 because 00030 is the latest on main; it must be RENUMBERED if another migration lands first.
--
-- proof_identity_capture holds the leg-independent bytes the S8 capture reads near the head (node sets, code, headers). Key
-- (chain, anchor_id, block_hash, item); insert-only; never read by a verifier. proof_contract_identity holds the served container of a
-- proof: PK (proof_id, seq), the greatest seq is served, a better capture is a NEW row (never an update). Both tables refuse UPDATE,
-- DELETE and TRUNCATE by trigger (SQLSTATE 23001), whoever the writer. The guard pins search_path (pg_catalog, pg_temp) so that a schema a
-- caller controls cannot shadow anything it calls. octet_length checks equal MaxContractIdentityBytes / MaxItemBytes (4194304).
-- Expand-only: two new tables, one function, four triggers. Nothing reads or alters an existing object.

CREATE TABLE public.proof_identity_capture (
    chain       bigint NOT NULL
        CONSTRAINT proof_identity_capture_chain_is_positive CHECK (chain > 0),
    anchor_id   bytea NOT NULL
        CONSTRAINT proof_identity_capture_anchor_is_32_bytes CHECK (octet_length(anchor_id) = 32),
    block_hash  bytea NOT NULL
        CONSTRAINT proof_identity_capture_hash_is_32_bytes CHECK (octet_length(block_hash) = 32),
    item        character varying(160) NOT NULL
        CONSTRAINT proof_identity_capture_item_is_name CHECK (item ~ '^[a-z0-9_-]+(/[a-z0-9_-]+)*$'),
    bytes       bytea NOT NULL
        CONSTRAINT proof_identity_capture_bytes_bounded CHECK (octet_length(bytes) <= 4194304),
    created_at  timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT proof_identity_capture_pkey PRIMARY KEY (chain, anchor_id, block_hash, item)
);

CREATE TABLE public.proof_contract_identity (
    proof_id         uuid NOT NULL,
    seq              bigint NOT NULL
        CONSTRAINT proof_contract_identity_seq_is_positive CHECK (seq >= 1),
    chain            bigint NOT NULL
        CONSTRAINT proof_contract_identity_chain_is_positive CHECK (chain > 0),
    block_number     bigint NOT NULL
        CONSTRAINT proof_contract_identity_block_is_not_negative CHECK (block_number >= 0),
    block_hash       bytea NOT NULL
        CONSTRAINT proof_contract_identity_block_hash_is_32_bytes CHECK (octet_length(block_hash) = 32),
    evidence         bytea NOT NULL
        CONSTRAINT proof_contract_identity_evidence_bounded CHECK (octet_length(evidence) BETWEEN 1 AND 4194304),
    evidence_sha256  bytea NOT NULL
        CONSTRAINT proof_contract_identity_evidence_sha_is_32_bytes CHECK (octet_length(evidence_sha256) = 32),
    digest           bytea NOT NULL
        CONSTRAINT proof_contract_identity_digest_is_32_bytes CHECK (octet_length(digest) = 32),
    created_at       timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT proof_contract_identity_pkey PRIMARY KEY (proof_id, seq)
);

CREATE FUNCTION public.proof_identity_guard() RETURNS trigger
    LANGUAGE plpgsql SET search_path = pg_catalog, pg_temp
    AS $$
BEGIN
    RAISE EXCEPTION '% is insert-only: % refused', TG_TABLE_NAME, TG_OP USING ERRCODE = 'restrict_violation';
END;
$$;

CREATE TRIGGER proof_identity_capture_row_guard BEFORE UPDATE OR DELETE ON public.proof_identity_capture
    FOR EACH ROW EXECUTE FUNCTION public.proof_identity_guard();
CREATE TRIGGER proof_identity_capture_statement_guard BEFORE TRUNCATE ON public.proof_identity_capture
    FOR EACH STATEMENT EXECUTE FUNCTION public.proof_identity_guard();
CREATE TRIGGER proof_contract_identity_row_guard BEFORE UPDATE OR DELETE ON public.proof_contract_identity
    FOR EACH ROW EXECUTE FUNCTION public.proof_identity_guard();
CREATE TRIGGER proof_contract_identity_statement_guard BEFORE TRUNCATE ON public.proof_contract_identity
    FOR EACH STATEMENT EXECUTE FUNCTION public.proof_identity_guard();

COMMENT ON TABLE public.proof_identity_capture IS 'Insert-only S8 capture bytes (V2_SPEC_S8 Q2); never read by a verifier';
COMMENT ON TABLE public.proof_contract_identity IS 'Insert-only S8 contract-identity containers, greatest seq served (V2_SPEC_S8 Q2)';
`

func TestGetContractIdentityReadsTheGreatestSeqAndTheTableRefusesAnOversizeContainer(t *testing.T) {
	db := finalityTestDB(t)
	var cur string
	if err := db.QueryRow("SELECT current_schema()").Scan(&cur); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(strings.ReplaceAll(validatorMigration00031, "public.", cur+".")); err != nil {
		t.Fatalf("migration 00031: %v", err)
	}
	repo := &ProofArtifactRepository{db: db}
	ctx := context.Background()
	pid := uuid.New()
	if row, err := repo.GetContractIdentity(ctx, pid); err != nil || row != nil {
		t.Fatalf("a proof without a container has no row: %v %v", row, err)
	}
	ins := func(seq int, ev []byte) {
		sum := sha256.Sum256(ev)
		if _, err := db.Exec("INSERT INTO proof_contract_identity (proof_id, seq, chain, block_number, block_hash, evidence, evidence_sha256, digest) VALUES ($1,$2,11155111,7,$3,$4,$5,$6)",
			pid, seq, bytes.Repeat([]byte{2}, 32), ev, sum[:], bytes.Repeat([]byte{byte(seq)}, 32)); err != nil {
			t.Fatal(err)
		}
	}
	ins(1, []byte("first"))
	ins(2, []byte("second"))
	row, err := repo.GetContractIdentity(ctx, pid)
	if err != nil || row == nil || row.Seq != 2 || string(row.Evidence) != "second" || row.Chain != 11155111 || row.BlockNumber != 7 || len(row.Digest) != 32 || row.Digest[0] != 2 {
		t.Fatalf("the greatest seq is served: %+v %v", row, err)
	}
	// the table itself refuses a container over the bound (the same number as MaxContractIdentityBytes)
	if _, err := db.Exec("INSERT INTO proof_contract_identity (proof_id, seq, chain, block_number, block_hash, evidence, evidence_sha256, digest) VALUES ($1,3,1,1,$2,$3,$4,$4)",
		pid, bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{1}, int(MaxContractIdentityBytes)+1), make([]byte, 32)); err == nil {
		t.Fatal("the table must refuse evidence over MaxContractIdentityBytes")
	}
}
