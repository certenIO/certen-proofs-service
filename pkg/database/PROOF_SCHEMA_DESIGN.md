# Comprehensive Proof Artifact Schema Design

## Executive Summary

This document defines the PostgreSQL schema for storing all Certen Protocol proof artifacts, designed for:
- **High-volume storage**: Full JSON artifacts, not just metadata
- **API access patterns**: External customers and auditing nodes
- **Discovery & retrieval**: Multiple access keys for different query patterns
- **Auditability**: Complete proof chain from transaction to genesis

---

## Proof Type Taxonomy

### 1. ChainedProof (Accumulate State Proof) - L1/L2/L3

```
Layer1 (TX → BVN):
├── TxHash, Principal, BVNPartition
├── Receipt (entries + anchor)
└── ReceiptSteps (hash, right flag)

Layer2 (BVN → DN):
├── BVNRoot, DNRoot, AnchorSequence
├── AnchorReceipt
└── BVNPartitionID

Layer3 (DN → Consensus):
├── DNBlockHash, DNBlockHeight
├── ValidatorSignatures[]
└── ConsensusTimestamp
```

### 2. GovernanceProof - G0/G1/G2

```
G0 (Inclusion & Finality):
├── TxHash, BlockHeight, Timestamp
├── Anchored, AnchorHeight
└── FinalizationProof

G1 (Governance Correctness):
├── AuthoritySnapshot (KeyPages[], Thresholds)
├── ValidatedSignatures[]
└── GovernanceRules

G2 (Outcome Binding):
├── Outcome, ConditionalLogic
├── BindingConstraints
└── OutcomeHash
```

### 3. CertenAnchorProof - 4 Components (Whitepaper 3.4.1)

```
Component 1 - Transaction Inclusion:
├── MerkleRoot, LeafHash, LeafIndex
└── MerklePath[] (hash, position)

Component 2 - Anchor Reference:
├── TargetChain, ChainID
├── AnchorTxHash, BlockNumber, BlockHash
└── ContractAddress

Component 3 - State Proof:
└── ChainedProof (L1/L2/L3) - see above

Component 4 - Authority Proof:
└── GovernanceProof (G0/G1/G2) - see above

ValidatorAttestations[]:
├── ValidatorID, PublicKey
├── Signature (Ed25519)
└── Timestamp
```

---

## Schema Design

### Core Tables

#### 1. `proof_artifacts` - Master Proof Registry

```sql
CREATE TABLE proof_artifacts (
    -- Primary Key
    proof_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Classification
    proof_type          VARCHAR(50) NOT NULL,  -- 'certen_anchor', 'chained', 'governance'
    proof_version       VARCHAR(20) NOT NULL DEFAULT '1.0',

    -- Transaction Reference (PRIMARY LOOKUP KEY)
    accum_tx_hash       VARCHAR(128) NOT NULL,
    account_url         VARCHAR(512) NOT NULL,

    -- Batch Reference
    batch_id            UUID REFERENCES anchor_batches(batch_id),
    batch_position      INTEGER,  -- Position within batch merkle tree

    -- Anchor Reference (for anchored proofs)
    anchor_id           UUID REFERENCES anchor_records(anchor_id),
    anchor_tx_hash      VARCHAR(128),
    anchor_block_number BIGINT,
    anchor_chain        VARCHAR(50),  -- 'ethereum', 'bitcoin'

    -- Merkle Inclusion
    merkle_root         BYTEA,
    leaf_hash           BYTEA,
    leaf_index          INTEGER,

    -- Governance Level
    gov_level           VARCHAR(10),  -- 'G0', 'G1', 'G2', NULL

    -- Proof Class (pricing tier)
    proof_class         VARCHAR(20) NOT NULL,  -- 'on_cadence', 'on_demand'

    -- Validator Attribution
    validator_id        VARCHAR(128) NOT NULL,

    -- Status & Lifecycle
    status              VARCHAR(30) NOT NULL DEFAULT 'pending',
    verification_status VARCHAR(30),  -- 'verified', 'failed', 'pending'

    -- Timestamps
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    anchored_at         TIMESTAMPTZ,
    verified_at         TIMESTAMPTZ,

    -- Full JSON Artifacts (THE ACTUAL PROOF DATA)
    artifact_json       JSONB NOT NULL,  -- Complete proof structure

    -- Computed Fields for Indexing
    artifact_hash       BYTEA NOT NULL,  -- SHA256(artifact_json) for integrity

    CONSTRAINT valid_proof_type CHECK (proof_type IN ('certen_anchor', 'chained', 'governance')),
    CONSTRAINT valid_gov_level CHECK (gov_level IS NULL OR gov_level IN ('G0', 'G1', 'G2')),
    CONSTRAINT valid_proof_class CHECK (proof_class IN ('on_cadence', 'on_demand'))
);

-- PRIMARY ACCESS INDEXES
CREATE INDEX idx_proof_artifacts_tx_hash ON proof_artifacts(accum_tx_hash);
CREATE INDEX idx_proof_artifacts_account ON proof_artifacts(account_url);
CREATE INDEX idx_proof_artifacts_batch ON proof_artifacts(batch_id);
CREATE INDEX idx_proof_artifacts_anchor_tx ON proof_artifacts(anchor_tx_hash);
CREATE INDEX idx_proof_artifacts_merkle_root ON proof_artifacts(merkle_root);
CREATE INDEX idx_proof_artifacts_validator ON proof_artifacts(validator_id);
CREATE INDEX idx_proof_artifacts_gov_level ON proof_artifacts(gov_level) WHERE gov_level IS NOT NULL;
CREATE INDEX idx_proof_artifacts_status ON proof_artifacts(status);
CREATE INDEX idx_proof_artifacts_created ON proof_artifacts(created_at DESC);
CREATE INDEX idx_proof_artifacts_anchored ON proof_artifacts(anchored_at DESC) WHERE anchored_at IS NOT NULL;

-- COMPOSITE INDEXES FOR COMMON QUERIES
CREATE INDEX idx_proof_artifacts_account_time ON proof_artifacts(account_url, created_at DESC);
CREATE INDEX idx_proof_artifacts_batch_position ON proof_artifacts(batch_id, batch_position);
CREATE INDEX idx_proof_artifacts_type_status ON proof_artifacts(proof_type, status);
CREATE INDEX idx_proof_artifacts_chain_block ON proof_artifacts(anchor_chain, anchor_block_number);

-- JSONB INDEXES FOR ARTIFACT QUERIES
CREATE INDEX idx_proof_artifacts_artifact_gin ON proof_artifacts USING GIN (artifact_json);
```

#### 2. `chained_proof_layers` - L1/L2/L3 Breakdown

```sql
CREATE TABLE chained_proof_layers (
    -- Primary Key
    layer_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent Reference
    proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id) ON DELETE CASCADE,

    -- Layer Classification
    layer_number        INTEGER NOT NULL,  -- 1, 2, or 3
    layer_name          VARCHAR(50) NOT NULL,  -- 'tx_to_bvn', 'bvn_to_dn', 'dn_to_consensus'

    -- Layer 1 Fields (TX → BVN)
    bvn_partition       VARCHAR(50),
    receipt_anchor      BYTEA,

    -- Layer 2 Fields (BVN → DN)
    bvn_root            BYTEA,
    dn_root             BYTEA,
    anchor_sequence     BIGINT,
    bvn_partition_id    VARCHAR(50),

    -- Layer 3 Fields (DN → Consensus)
    dn_block_hash       BYTEA,
    dn_block_height     BIGINT,
    consensus_timestamp TIMESTAMPTZ,

    -- Full Layer Artifact
    layer_json          JSONB NOT NULL,

    -- Verification
    verified            BOOLEAN DEFAULT FALSE,
    verified_at         TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_layer_number CHECK (layer_number IN (1, 2, 3))
);

CREATE INDEX idx_chained_layers_proof ON chained_proof_layers(proof_id);
CREATE INDEX idx_chained_layers_number ON chained_proof_layers(proof_id, layer_number);
CREATE INDEX idx_chained_layers_dn_block ON chained_proof_layers(dn_block_height) WHERE layer_number = 3;
CREATE INDEX idx_chained_layers_bvn ON chained_proof_layers(bvn_partition) WHERE layer_number = 1;
```

#### 3. `governance_proof_levels` - G0/G1/G2 Breakdown

```sql
CREATE TABLE governance_proof_levels (
    -- Primary Key
    level_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent Reference
    proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id) ON DELETE CASCADE,

    -- Level Classification
    gov_level           VARCHAR(10) NOT NULL,  -- 'G0', 'G1', 'G2'
    level_name          VARCHAR(50) NOT NULL,  -- 'inclusion_finality', 'governance_correctness', 'outcome_binding'

    -- G0 Fields (Inclusion & Finality)
    block_height        BIGINT,
    finality_timestamp  TIMESTAMPTZ,
    anchor_height       BIGINT,
    is_anchored         BOOLEAN,

    -- G1 Fields (Governance Correctness)
    authority_url       VARCHAR(512),
    key_page_count      INTEGER,
    threshold_m         INTEGER,
    threshold_n         INTEGER,
    signature_count     INTEGER,

    -- G2 Fields (Outcome Binding)
    outcome_type        VARCHAR(100),
    outcome_hash        BYTEA,
    binding_enforced    BOOLEAN,

    -- Full Level Artifact
    level_json          JSONB NOT NULL,

    -- Verification
    verified            BOOLEAN DEFAULT FALSE,
    verified_at         TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_gov_level CHECK (gov_level IN ('G0', 'G1', 'G2'))
);

CREATE INDEX idx_gov_levels_proof ON governance_proof_levels(proof_id);
CREATE INDEX idx_gov_levels_level ON governance_proof_levels(proof_id, gov_level);
CREATE INDEX idx_gov_levels_authority ON governance_proof_levels(authority_url) WHERE gov_level = 'G1';
CREATE INDEX idx_gov_levels_outcome ON governance_proof_levels(outcome_type) WHERE gov_level = 'G2';
```

#### 4. `merkle_inclusion_proofs` - Merkle Path Storage

```sql
CREATE TABLE merkle_inclusion_proofs (
    -- Primary Key
    inclusion_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent Reference
    proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id) ON DELETE CASCADE,

    -- Merkle Tree Reference
    merkle_root         BYTEA NOT NULL,
    leaf_hash           BYTEA NOT NULL,
    leaf_index          INTEGER NOT NULL,
    tree_size           INTEGER NOT NULL,

    -- Merkle Path (ordered array of nodes)
    merkle_path         JSONB NOT NULL,  -- [{hash, position: 'left'|'right'}, ...]

    -- Verification
    verified            BOOLEAN DEFAULT FALSE,
    verified_at         TIMESTAMPTZ,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_merkle_inclusion_proof ON merkle_inclusion_proofs(proof_id);
CREATE INDEX idx_merkle_inclusion_root ON merkle_inclusion_proofs(merkle_root);
CREATE INDEX idx_merkle_inclusion_leaf ON merkle_inclusion_proofs(leaf_hash);
```

#### 5. `validator_attestations` - Multi-Validator Signatures

```sql
CREATE TABLE validator_attestations (
    -- Primary Key
    attestation_id      UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent Reference (can be proof or batch)
    proof_id            UUID REFERENCES proof_artifacts(proof_id) ON DELETE CASCADE,
    batch_id            UUID REFERENCES anchor_batches(batch_id) ON DELETE CASCADE,

    -- Validator Identity
    validator_id        VARCHAR(128) NOT NULL,
    validator_pubkey    BYTEA NOT NULL,  -- Ed25519 public key (32 bytes)

    -- Attestation Data
    attested_hash       BYTEA NOT NULL,  -- What was signed (merkle_root || anchor_tx_hash)
    signature           BYTEA NOT NULL,  -- Ed25519 signature (64 bytes)

    -- Context
    anchor_tx_hash      VARCHAR(128),
    merkle_root         BYTEA,
    block_number        BIGINT,

    -- Verification
    signature_valid     BOOLEAN DEFAULT FALSE,
    verified_at         TIMESTAMPTZ,

    -- Timestamps
    attested_at         TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT attestation_has_parent CHECK (proof_id IS NOT NULL OR batch_id IS NOT NULL)
);

CREATE INDEX idx_attestations_proof ON validator_attestations(proof_id);
CREATE INDEX idx_attestations_batch ON validator_attestations(batch_id);
CREATE INDEX idx_attestations_validator ON validator_attestations(validator_id);
CREATE INDEX idx_attestations_anchor ON validator_attestations(anchor_tx_hash);
CREATE INDEX idx_attestations_merkle ON validator_attestations(merkle_root);
CREATE INDEX idx_attestations_time ON validator_attestations(attested_at DESC);

-- Ensure unique attestation per validator per batch/proof
CREATE UNIQUE INDEX idx_attestations_unique_proof ON validator_attestations(proof_id, validator_id) WHERE proof_id IS NOT NULL;
CREATE UNIQUE INDEX idx_attestations_unique_batch ON validator_attestations(batch_id, validator_id) WHERE batch_id IS NOT NULL;
```

#### 6. `anchor_references` - External Chain Anchors

```sql
CREATE TABLE anchor_references (
    -- Primary Key
    reference_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent Reference
    proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id) ON DELETE CASCADE,

    -- Target Chain
    target_chain        VARCHAR(50) NOT NULL,  -- 'ethereum', 'bitcoin', 'accumulate'
    chain_id            VARCHAR(50) NOT NULL,  -- '11155111' for Sepolia
    network_name        VARCHAR(50) NOT NULL,  -- 'sepolia', 'mainnet'

    -- Anchor Transaction
    anchor_tx_hash      VARCHAR(128) NOT NULL,
    anchor_block_number BIGINT NOT NULL,
    anchor_block_hash   VARCHAR(128),
    anchor_timestamp    TIMESTAMPTZ,

    -- Contract Reference (for Ethereum)
    contract_address    VARCHAR(128),

    -- Confirmation Status
    confirmations       INTEGER DEFAULT 0,
    is_confirmed        BOOLEAN DEFAULT FALSE,
    confirmed_at        TIMESTAMPTZ,

    -- Gas Costs (for Ethereum)
    gas_used            BIGINT,
    gas_price_wei       VARCHAR(50),
    total_cost_wei      VARCHAR(50),

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_anchor_refs_proof ON anchor_references(proof_id);
CREATE INDEX idx_anchor_refs_tx ON anchor_references(anchor_tx_hash);
CREATE INDEX idx_anchor_refs_chain_block ON anchor_references(target_chain, anchor_block_number);
CREATE INDEX idx_anchor_refs_contract ON anchor_references(contract_address);
CREATE INDEX idx_anchor_refs_confirmed ON anchor_references(is_confirmed, confirmed_at);
```

#### 7. `receipt_steps` - Merkle Receipt Path Steps

```sql
CREATE TABLE receipt_steps (
    -- Primary Key
    step_id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent Reference
    layer_id            UUID NOT NULL REFERENCES chained_proof_layers(layer_id) ON DELETE CASCADE,

    -- Step Data
    step_index          INTEGER NOT NULL,
    hash                BYTEA NOT NULL,
    is_right            BOOLEAN NOT NULL,  -- Position flag

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_receipt_steps_layer ON receipt_steps(layer_id);
CREATE INDEX idx_receipt_steps_order ON receipt_steps(layer_id, step_index);
```

#### 8. `validated_signatures` - Governance Signature Details

```sql
CREATE TABLE validated_signatures (
    -- Primary Key
    sig_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent Reference
    level_id            UUID NOT NULL REFERENCES governance_proof_levels(level_id) ON DELETE CASCADE,

    -- Signer Identity
    signer_url          VARCHAR(512) NOT NULL,  -- Key page URL
    key_hash            BYTEA NOT NULL,
    public_key          BYTEA NOT NULL,
    key_type            VARCHAR(50) NOT NULL,  -- 'ed25519', 'rcd1', etc.

    -- Signature Data
    signature           BYTEA NOT NULL,
    signed_hash         BYTEA NOT NULL,  -- What was signed

    -- Validation
    is_valid            BOOLEAN NOT NULL,
    validated_at        TIMESTAMPTZ,

    -- Key Page Reference
    key_page_index      INTEGER,
    key_index           INTEGER,

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_validated_sigs_level ON validated_signatures(level_id);
CREATE INDEX idx_validated_sigs_signer ON validated_signatures(signer_url);
CREATE INDEX idx_validated_sigs_key ON validated_signatures(key_hash);
```

#### 9. `proof_verifications` - Verification Audit Log

```sql
CREATE TABLE proof_verifications (
    -- Primary Key
    verification_id     UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Proof Reference
    proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id) ON DELETE CASCADE,

    -- Verification Details
    verification_type   VARCHAR(50) NOT NULL,  -- 'merkle', 'signature', 'state', 'governance', 'full'

    -- Result
    passed              BOOLEAN NOT NULL,
    error_message       TEXT,
    error_code          VARCHAR(50),

    -- Context
    verifier_id         VARCHAR(128),  -- Which validator/node performed verification
    verification_method VARCHAR(100),  -- Algorithm/approach used

    -- Performance
    duration_ms         INTEGER,

    -- Artifacts Checked
    artifacts_json      JSONB,  -- Details of what was verified

    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_verifications_proof ON proof_verifications(proof_id);
CREATE INDEX idx_verifications_type ON proof_verifications(verification_type);
CREATE INDEX idx_verifications_passed ON proof_verifications(passed);
CREATE INDEX idx_verifications_time ON proof_verifications(created_at DESC);
CREATE INDEX idx_verifications_verifier ON proof_verifications(verifier_id);
```

---

## API Access Patterns

### Pattern 1: Proof Discovery by Transaction

```sql
-- Get complete proof by Accumulate TX hash
SELECT * FROM proof_artifacts WHERE accum_tx_hash = $1;

-- Get proof with all layers
SELECT pa.*, cpl.layer_json, gpl.level_json
FROM proof_artifacts pa
LEFT JOIN chained_proof_layers cpl ON pa.proof_id = cpl.proof_id
LEFT JOIN governance_proof_levels gpl ON pa.proof_id = gpl.proof_id
WHERE pa.accum_tx_hash = $1;
```

### Pattern 2: Proofs by Account

```sql
-- List all proofs for an account (paginated, newest first)
SELECT proof_id, accum_tx_hash, proof_type, gov_level, status, created_at, anchored_at
FROM proof_artifacts
WHERE account_url = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- Count proofs by status for account
SELECT status, COUNT(*) as count
FROM proof_artifacts
WHERE account_url = $1
GROUP BY status;
```

### Pattern 3: Proofs by Batch

```sql
-- Get all proofs in a batch with merkle positions
SELECT proof_id, accum_tx_hash, account_url, batch_position, merkle_root, leaf_hash
FROM proof_artifacts
WHERE batch_id = $1
ORDER BY batch_position;

-- Get batch summary with attestation count
SELECT b.batch_id, b.merkle_root, COUNT(DISTINCT pa.proof_id) as proof_count,
       COUNT(DISTINCT va.validator_id) as attestation_count
FROM anchor_batches b
LEFT JOIN proof_artifacts pa ON b.batch_id = pa.batch_id
LEFT JOIN validator_attestations va ON b.batch_id = va.batch_id
WHERE b.batch_id = $1
GROUP BY b.batch_id;
```

### Pattern 4: Proofs by Anchor Transaction

```sql
-- Find all proofs anchored in a specific Ethereum transaction
SELECT pa.*, ar.anchor_block_number, ar.confirmations
FROM proof_artifacts pa
JOIN anchor_references ar ON pa.proof_id = ar.proof_id
WHERE ar.anchor_tx_hash = $1;

-- List recent anchors with proof counts
SELECT ar.anchor_tx_hash, ar.target_chain, ar.anchor_block_number,
       ar.is_confirmed, COUNT(pa.proof_id) as proof_count
FROM anchor_references ar
JOIN proof_artifacts pa ON ar.proof_id = pa.proof_id
GROUP BY ar.anchor_tx_hash, ar.target_chain, ar.anchor_block_number, ar.is_confirmed
ORDER BY ar.anchor_block_number DESC
LIMIT 100;
```

### Pattern 5: Governance Proof Queries

```sql
-- Get all G2 (Outcome Binding) proofs for an account
SELECT pa.*, gpl.outcome_type, gpl.outcome_hash
FROM proof_artifacts pa
JOIN governance_proof_levels gpl ON pa.proof_id = gpl.proof_id
WHERE pa.account_url = $1 AND gpl.gov_level = 'G2';

-- Find proofs by authority/key page
SELECT pa.proof_id, pa.accum_tx_hash, gpl.authority_url
FROM proof_artifacts pa
JOIN governance_proof_levels gpl ON pa.proof_id = gpl.proof_id
WHERE gpl.authority_url LIKE $1 || '%' AND gpl.gov_level = 'G1';
```

### Pattern 6: Validator Attestation Queries

```sql
-- Get all attestations for a validator
SELECT va.*, pa.accum_tx_hash, pa.account_url
FROM validator_attestations va
LEFT JOIN proof_artifacts pa ON va.proof_id = pa.proof_id
WHERE va.validator_id = $1
ORDER BY va.attested_at DESC;

-- Find proofs with attestation threshold met
SELECT pa.proof_id, pa.accum_tx_hash, COUNT(va.attestation_id) as attestation_count
FROM proof_artifacts pa
JOIN validator_attestations va ON pa.proof_id = va.proof_id
WHERE va.signature_valid = TRUE
GROUP BY pa.proof_id
HAVING COUNT(va.attestation_id) >= $1;  -- threshold parameter
```

### Pattern 7: Time-Range Queries

```sql
-- Get proofs created in time range
SELECT * FROM proof_artifacts
WHERE created_at BETWEEN $1 AND $2
ORDER BY created_at DESC;

-- Get recently anchored proofs
SELECT pa.*, ar.anchor_tx_hash, ar.anchor_block_number
FROM proof_artifacts pa
JOIN anchor_references ar ON pa.proof_id = ar.proof_id
WHERE ar.anchor_timestamp > NOW() - INTERVAL '1 hour'
ORDER BY ar.anchor_timestamp DESC;
```

### Pattern 8: Merkle Root Discovery

```sql
-- Find all proofs with same merkle root (same batch)
SELECT proof_id, accum_tx_hash, account_url, leaf_index
FROM proof_artifacts
WHERE merkle_root = $1
ORDER BY leaf_index;

-- Verify merkle inclusion exists
SELECT EXISTS(
    SELECT 1 FROM merkle_inclusion_proofs
    WHERE merkle_root = $1 AND leaf_hash = $2
);
```

### Pattern 9: Verification Status Queries

```sql
-- Get unverified proofs for processing
SELECT proof_id, accum_tx_hash, proof_type
FROM proof_artifacts
WHERE verification_status = 'pending'
ORDER BY created_at
LIMIT 100;

-- Get verification history for a proof
SELECT verification_type, passed, error_message, created_at
FROM proof_verifications
WHERE proof_id = $1
ORDER BY created_at DESC;
```

### Pattern 10: Auditing Node Sync

```sql
-- Get proofs modified since timestamp (for sync)
SELECT proof_id, accum_tx_hash, artifact_hash, created_at, anchored_at
FROM proof_artifacts
WHERE created_at > $1 OR anchored_at > $1
ORDER BY COALESCE(anchored_at, created_at)
LIMIT 1000;

-- Get full artifact for replication
SELECT artifact_json FROM proof_artifacts WHERE proof_id = $1;
```

---

## Indexing Strategy Summary

### Primary Lookup Keys
| Key | Use Case | Index |
|-----|----------|-------|
| `accum_tx_hash` | Proof by transaction | B-tree |
| `account_url` | Proofs by account | B-tree |
| `batch_id` | Proofs in batch | B-tree |
| `anchor_tx_hash` | Proofs by anchor | B-tree |
| `proof_id` | Direct lookup | Primary Key |

### Secondary Lookup Keys
| Key | Use Case | Index |
|-----|----------|-------|
| `merkle_root` | Same-batch proofs | B-tree |
| `validator_id` | Validator's proofs | B-tree |
| `gov_level` | Governance filtering | Partial B-tree |
| `anchor_chain + anchor_block_number` | Chain exploration | Composite B-tree |
| `created_at DESC` | Time-range queries | B-tree |
| `status` | Processing status | B-tree |

### JSONB Indexes
| Index | Use Case |
|-------|----------|
| `artifact_json GIN` | Deep artifact queries |

---

## Migration Path

1. **Phase 1**: Create new tables alongside existing `certen_anchor_proofs`
2. **Phase 2**: Migrate existing data to new schema
3. **Phase 3**: Update repository layer to use new schema
4. **Phase 4**: Deprecate old schema
5. **Phase 5**: Drop old tables

---

## Storage Estimates

| Table | Row Size (avg) | 1M Proofs | 10M Proofs |
|-------|---------------|-----------|------------|
| proof_artifacts | ~5KB | 5GB | 50GB |
| chained_proof_layers | ~2KB | 6GB | 60GB |
| governance_proof_levels | ~1.5KB | 4.5GB | 45GB |
| merkle_inclusion_proofs | ~500B | 500MB | 5GB |
| validator_attestations | ~200B | 200MB | 2GB |
| **Total** | | **~16GB** | **~162GB** |

All within reasonable PostgreSQL capacity with proper indexing and maintenance.

---

## Next Steps

1. [ ] Create migration SQL files
2. [ ] Update database repository interfaces
3. [ ] Implement new repository methods
4. [ ] Add API endpoints for access patterns
5. [ ] Create batch data migration script
