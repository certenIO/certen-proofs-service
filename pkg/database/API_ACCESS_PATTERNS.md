# Certen Proof API Access Patterns

This document defines the API access patterns for external customers and auditing nodes to discover and retrieve proof artifacts from the Certen Protocol.

---

## Table of Contents

1. [Proof Discovery Endpoints](#proof-discovery-endpoints)
2. [Proof Retrieval Endpoints](#proof-retrieval-endpoints)
3. [Batch Operations](#batch-operations)
4. [Attestation Queries](#attestation-queries)
5. [Verification Status](#verification-status)
6. [Sync/Replication Endpoints](#syncreplication-endpoints)
7. [Index Strategy Summary](#index-strategy-summary)

---

## Proof Discovery Endpoints

### 1. Get Proof by Transaction Hash

**Use Case**: Customer wants to retrieve proof for a specific Accumulate transaction.

```
GET /api/v1/proofs/tx/{accum_tx_hash}
```

**Response**: Complete `ProofArtifactWithDetails` including:
- Proof metadata
- ChainedProof layers (L1-L3)
- GovernanceProof levels (G0-G2)
- Validator attestations
- Anchor reference
- Verification history

**PostgreSQL Query**:
```sql
SELECT * FROM proof_artifacts WHERE accum_tx_hash = $1;
```

**Index Used**: `idx_proof_artifacts_tx_hash`

---

### 2. List Proofs by Account

**Use Case**: Customer wants to see all proofs for their account.

```
GET /api/v1/proofs/account/{account_url}?limit=50&offset=0
```

**Query Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 50 | Max results (1-1000) |
| `offset` | int | 0 | Pagination offset |
| `status` | string | null | Filter by status |
| `gov_level` | string | null | Filter by G0/G1/G2 |

**Response**: Array of `ProofSummary` objects

**PostgreSQL Query**:
```sql
SELECT proof_id, proof_type, accum_tx_hash, account_url, gov_level,
       status, created_at, anchored_at,
       (SELECT COUNT(*) FROM validator_attestations WHERE proof_id = pa.proof_id)
FROM proof_artifacts pa
WHERE account_url = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;
```

**Index Used**: `idx_proof_artifacts_account_time`

---

### 3. List Proofs by Batch

**Use Case**: Auditing node wants to verify all proofs in a batch.

```
GET /api/v1/proofs/batch/{batch_id}
```

**Response**: Array of `ProofArtifact` with batch positions

**PostgreSQL Query**:
```sql
SELECT * FROM proof_artifacts
WHERE batch_id = $1
ORDER BY batch_position;
```

**Index Used**: `idx_proof_artifacts_batch_position`

---

### 4. List Proofs by Anchor Transaction

**Use Case**: Customer wants to find all proofs anchored in a specific Ethereum tx.

```
GET /api/v1/proofs/anchor/{anchor_tx_hash}
```

**Response**: Array of `ProofArtifact` objects

**PostgreSQL Query**:
```sql
SELECT pa.*, ar.anchor_block_number, ar.confirmations
FROM proof_artifacts pa
JOIN anchor_references ar ON pa.proof_id = ar.proof_id
WHERE ar.anchor_tx_hash = $1;
```

**Index Used**: `idx_anchor_refs_tx`

---

### 5. Query Proofs with Filters

**Use Case**: Complex discovery queries with multiple criteria.

```
POST /api/v1/proofs/query
```

**Request Body**:
```json
{
  "account_url": "acc://example.acme/tokens",
  "proof_type": "certen_anchor",
  "gov_level": "G1",
  "status": "anchored",
  "anchor_chain": "ethereum",
  "created_after": "2025-01-01T00:00:00Z",
  "created_before": "2025-12-31T23:59:59Z",
  "limit": 100,
  "offset": 0
}
```

**Response**: Array of `ProofSummary` objects

**Index Used**: Multiple composite indexes depending on filters

---

## Proof Retrieval Endpoints

### 6. Get Complete Proof by ID

**Use Case**: Retrieve full proof artifact with all details.

```
GET /api/v1/proofs/{proof_id}
```

**Response**: `ProofArtifactWithDetails`

```json
{
  "proof_id": "550e8400-e29b-41d4-a716-446655440000",
  "proof_type": "certen_anchor",
  "accum_tx_hash": "abc123...",
  "account_url": "acc://example.acme/tokens",
  "status": "anchored",
  "artifact_json": { /* Full ChainedProof + GovernanceProof */ },
  "chained_layers": [
    {"layer_number": 1, "layer_name": "tx_to_bvn", "layer_json": {...}},
    {"layer_number": 2, "layer_name": "bvn_to_dn", "layer_json": {...}},
    {"layer_number": 3, "layer_name": "dn_to_consensus", "layer_json": {...}}
  ],
  "governance_levels": [
    {"gov_level": "G0", "level_json": {...}},
    {"gov_level": "G1", "level_json": {...}}
  ],
  "attestations": [
    {"validator_id": "validator-1", "signature": "...", "signature_valid": true}
  ],
  "anchor_reference": {
    "target_chain": "ethereum",
    "anchor_tx_hash": "0xabc...",
    "anchor_block_number": 12345678,
    "confirmations": 12,
    "is_confirmed": true
  }
}
```

---

### 7. Get Raw Proof Artifact JSON

**Use Case**: Download the raw proof artifact for offline verification.

```
GET /api/v1/proofs/{proof_id}/artifact
```

**Response**: Raw JSON artifact

```json
{
  "merkle_inclusion": {
    "root": "abc123...",
    "leaf_hash": "def456...",
    "path": [{"hash": "...", "position": "left"}]
  },
  "chained_proof": {
    "layer1": {...},
    "layer2": {...},
    "layer3": {...}
  },
  "governance_proof": {
    "g0": {...},
    "g1": {...}
  }
}
```

---

### 8. Get Merkle Inclusion Proof

**Use Case**: Verify transaction inclusion in batch.

```
GET /api/v1/proofs/{proof_id}/merkle
```

**Response**: `MerkleInclusionRecord`

```json
{
  "merkle_root": "abc123...",
  "leaf_hash": "def456...",
  "leaf_index": 5,
  "tree_size": 100,
  "merkle_path": [
    {"hash": "aaa...", "position": "left"},
    {"hash": "bbb...", "position": "right"}
  ],
  "verified": true
}
```

---

## Batch Operations

### 9. Get Batch Statistics

**Use Case**: View batch status and proof counts.

```
GET /api/v1/batches/{batch_id}/stats
```

**Response**: `BatchProofStats`

```json
{
  "batch_id": "550e8400-e29b-41d4-a716-446655440000",
  "proof_count": 150,
  "attestation_count": 3,
  "verified_count": 148,
  "failed_count": 2
}
```

---

### 10. List Recent Batches

**Use Case**: Browse recent anchored batches.

```
GET /api/v1/batches?limit=20&status=anchored
```

**Response**: Array of batch summaries with anchor info

---

## Attestation Queries

### 11. Get Attestations for Proof

**Use Case**: Verify multi-validator signatures.

```
GET /api/v1/proofs/{proof_id}/attestations
```

**Response**: Array of `ProofAttestation`

```json
[
  {
    "attestation_id": "...",
    "validator_id": "validator-1",
    "validator_pubkey": "base64...",
    "signature": "base64...",
    "attested_hash": "hex...",
    "signature_valid": true,
    "attested_at": "2025-01-15T12:00:00Z"
  }
]
```

---

### 12. Get Attestations by Validator

**Use Case**: Audit validator participation.

```
GET /api/v1/attestations/validator/{validator_id}?limit=100
```

**Response**: Array of attestations with proof references

**Index Used**: `idx_attestations_validator`

---

### 13. Count Valid Attestations

**Use Case**: Check if attestation threshold is met.

```
GET /api/v1/batches/{batch_id}/attestation-count
```

**Response**:
```json
{
  "batch_id": "...",
  "total_attestations": 3,
  "valid_attestations": 3,
  "threshold": 2,
  "threshold_met": true
}
```

---

## Verification Status

### 14. Get Verification History

**Use Case**: Audit proof verification attempts.

```
GET /api/v1/proofs/{proof_id}/verifications
```

**Response**: Array of `ProofVerificationRecord`

```json
[
  {
    "verification_id": "...",
    "verification_type": "merkle",
    "passed": true,
    "duration_ms": 15,
    "created_at": "2025-01-15T12:00:00Z"
  },
  {
    "verification_id": "...",
    "verification_type": "signature",
    "passed": true,
    "duration_ms": 8,
    "created_at": "2025-01-15T12:00:01Z"
  }
]
```

---

### 15. Verify Proof Integrity

**Use Case**: Check if artifact hash matches stored hash.

```
GET /api/v1/proofs/{proof_id}/integrity
```

**Response**:
```json
{
  "proof_id": "...",
  "artifact_hash": "abc123...",
  "computed_hash": "abc123...",
  "integrity_valid": true
}
```

---

## Sync/Replication Endpoints

### 16. Get Proofs Modified Since

**Use Case**: Auditing node syncing proofs.

```
GET /api/v1/proofs/sync?since=2025-01-01T00:00:00Z&limit=1000
```

**Response**: Array of `ProofArtifact` modified since timestamp

**PostgreSQL Query**:
```sql
SELECT * FROM proof_artifacts
WHERE created_at > $1 OR anchored_at > $1
ORDER BY COALESCE(anchored_at, created_at)
LIMIT $2;
```

---

### 17. Get Proof Count by Status

**Use Case**: Dashboard metrics.

```
GET /api/v1/proofs/counts
```

**Response**:
```json
{
  "total": 150000,
  "pending": 500,
  "anchored": 145000,
  "verified": 140000,
  "failed": 100
}
```

---

## Index Strategy Summary

### Primary Keys (B-tree)

| Table | Primary Key |
|-------|-------------|
| `proof_artifacts` | `proof_id` (UUID) |
| `chained_proof_layers` | `layer_id` (UUID) |
| `governance_proof_levels` | `level_id` (UUID) |
| `validator_attestations` | `attestation_id` (UUID) |

### High-Traffic Lookup Indexes

| Index | Column(s) | Use Case |
|-------|-----------|----------|
| `idx_proof_artifacts_tx_hash` | `accum_tx_hash` | Proof by TX hash |
| `idx_proof_artifacts_account` | `account_url` | Proofs by account |
| `idx_proof_artifacts_batch` | `batch_id` | Proofs in batch |
| `idx_proof_artifacts_anchor_tx` | `anchor_tx_hash` | Proofs by anchor |

### Discovery Indexes

| Index | Column(s) | Use Case |
|-------|-----------|----------|
| `idx_proof_artifacts_account_time` | `account_url, created_at DESC` | Account timeline |
| `idx_proof_artifacts_gov_level` | `gov_level` | Filter by G0/G1/G2 |
| `idx_proof_artifacts_status` | `status` | Filter by status |
| `idx_proof_artifacts_validator` | `validator_id` | Validator's proofs |

### Time-Series Indexes

| Index | Column(s) | Use Case |
|-------|-----------|----------|
| `idx_proof_artifacts_created` | `created_at DESC` | Recent proofs |
| `idx_proof_artifacts_anchored` | `anchored_at DESC` | Recent anchors |
| `idx_attestations_time` | `attested_at DESC` | Recent attestations |

### Chain Exploration Indexes

| Index | Column(s) | Use Case |
|-------|-----------|----------|
| `idx_proof_artifacts_chain_block` | `anchor_chain, anchor_block_number` | Browse by chain/block |
| `idx_anchor_refs_chain_block` | `target_chain, anchor_block_number` | Anchor exploration |

### JSONB Index

| Index | Type | Use Case |
|-------|------|----------|
| `idx_proof_artifacts_artifact_gin` | GIN | Deep artifact queries |

---

## Rate Limits & Pagination

### Recommended Limits

| Endpoint Type | Max per Request | Rate Limit |
|---------------|-----------------|------------|
| List queries | 1000 | 100/min |
| Detail queries | 1 | 1000/min |
| Sync endpoints | 1000 | 10/min |

### Pagination

All list endpoints support:
- `limit`: Max results (default 50, max 1000)
- `offset`: Skip first N results

For large datasets, use cursor-based pagination with `created_at > last_seen_timestamp`.

---

## Error Responses

```json
{
  "error": {
    "code": "PROOF_NOT_FOUND",
    "message": "Proof with ID xyz not found",
    "details": {}
  }
}
```

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `PROOF_NOT_FOUND` | 404 | Proof ID does not exist |
| `INVALID_TX_HASH` | 400 | Invalid transaction hash format |
| `RATE_LIMITED` | 429 | Too many requests |
| `INTERNAL_ERROR` | 500 | Server error |

---

## Authentication

All API endpoints require authentication via:

```
Authorization: Bearer <api_token>
```

Tokens are provisioned per customer with specific scopes:
- `proofs:read` - Read proof artifacts
- `proofs:write` - Request new proofs
- `attestations:read` - Read attestations
- `admin:*` - Full access
