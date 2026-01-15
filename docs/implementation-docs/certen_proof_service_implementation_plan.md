 Certen Proof Service - Comprehensive Implementation Plan

 Executive Summary

 Design and implement a unified Proof Artifact Service that enables external discovery, retrieval, and verification of proof artifacts and metadata already produced by the Certen Protocol. The proofs themselves are already implemented - this service exposes them to external consumers.

 Key Clarification: The four proof types (Cryptographic/ChainedProof, Governance G0/G1/G2, Certen Layer, External Chain) are already implemented. This service provides:
 - Storage and indexing of proof artifacts
 - External API for discovery and retrieval
 - Self-contained verification bundles
 - User interface for inspection

 Available Resources:
 - JavaScript Accumulate SDK: C:\Accumulate_Stuff\typescript-sdk-accumulate-mod\javascript
 - Accumulate Explorer (reusable UI): C:\Accumulate_Stuff\accumulate-explorer-clone\explorer

 ---
 1. Architecture Overview

 System Architecture

                               +------------------------+
                               |   External Clients     |
                               |  (Auditors, Services)  |
                               +----------+-------------+
                                          |
                                          v
 +----------------+           +------------------------+           +----------------+
 |  Accumulate    |           |     API Gateway       |           |  Ethereum/BTC  |
 |  Network       |<--------->|   (REST + WebSocket)  |<--------->|  Blockchains   |
 |  (Lite Client) |           +------------------------+           +----------------+
 +----------------+                       |
                                          v
                          +--------------------------------+
                          |     Proof Service Core         |
                          |  +----------+  +-----------+   |
                          |  | Proof    |  | Lifecycle |   |
                          |  | Generator|  | Manager   |   |
                          |  +----------+  +-----------+   |
                          |  +-----------+ +------------+  |
                          |  | Artifact  | | Attestation|  |
                          |  | Manager   | | Collector  |  |
                          |  +-----------+ +------------+  |
                          +--------------------------------+
                                          |
                                          v
                          +--------------------------------+
                          |   PostgreSQL + Artifact Store  |
                          +--------------------------------+

 Core Service Components

 | Component                   | Responsibility                                         |
 |-----------------------------|--------------------------------------------------------|
 | ProofGeneratorService       | Orchestrates generation of all 4 proof components      |
 | ProofLifecycleManager       | Manages proof states (pending -> anchored -> verified) |
 | ArtifactStorageService      | Handles JSON bundle storage with integrity hashing     |
 | AttestationCollectorService | Collects multi-validator attestations (2/3+1 quorum)   |
 | AnchorSchedulerService      | Manages on-cadence (~15 min) and on-demand anchoring   |
 | ExternalVerificationAPI     | Public API for proof discovery and verification        |

 ---
 2. Proof Types and Components

 Four-Component Proof Architecture (per Whitepaper Section 3.4.1)

 | Component                            | Description                                              | Source                         |
 |--------------------------------------|----------------------------------------------------------|--------------------------------|
 | 1. Transaction Inclusion Proof       | Merkle proof demonstrating tx exists in Accumulate block | Batch merkle tree              |
 | 2. Anchor Reference                  | Reference to anchor tx on external blockchain            | Ethereum/Bitcoin anchor        |
 | 3. State Proof (ChainedProof)        | L1/L2/L3 cryptographic proof of Accumulate state         | working-proof/                 |
 | 4. Authority Proof (GovernanceProof) | G0/G1/G2 proof of Key Book authorization                 | consolidated_governance-proof/ |

 Proof Pricing Tiers

 | Tier       | Cost   | Latency     | Use Case                  |
 |------------|--------|-------------|---------------------------|
 | on_cadence | ~$0.05 | ~15 minutes | Routine operations        |
 | on_demand  | ~$0.25 | Immediate   | High-value/time-sensitive |

 ---
 3. Database Schema Enhancements

 New Migration: 003_proof_service_enhancements.sql

 -- 1. Proof Bundle Table (self-contained verification bundles)
 CREATE TABLE IF NOT EXISTS proof_bundles (
     bundle_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
     proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id),
     bundle_format       VARCHAR(20) NOT NULL DEFAULT 'certen_v1',
     bundle_version      VARCHAR(20) NOT NULL DEFAULT '1.0',
     bundle_data         BYTEA NOT NULL,          -- Gzipped JSON
     bundle_hash         BYTEA NOT NULL,          -- SHA256 of uncompressed
     bundle_size_bytes   INTEGER NOT NULL,
     includes_chained    BOOLEAN NOT NULL DEFAULT TRUE,
     includes_governance BOOLEAN NOT NULL DEFAULT TRUE,
     includes_merkle     BOOLEAN NOT NULL DEFAULT TRUE,
     includes_anchor     BOOLEAN NOT NULL DEFAULT TRUE,
     expires_at          TIMESTAMPTZ,
     created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
 );

 -- 2. Proof Pricing Tiers
 CREATE TABLE IF NOT EXISTS proof_pricing_tiers (
     tier_id             VARCHAR(50) PRIMARY KEY,
     tier_name           VARCHAR(100) NOT NULL,
     base_cost_usd       NUMERIC(10, 4) NOT NULL,
     batch_delay_seconds INTEGER NOT NULL,
     priority            INTEGER NOT NULL,
     is_active           BOOLEAN NOT NULL DEFAULT TRUE,
     created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
 );

 -- 3. Custody Chain Events (audit trail)
 CREATE TABLE IF NOT EXISTS custody_chain_events (
     event_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
     proof_id            UUID NOT NULL REFERENCES proof_artifacts(proof_id),
     event_type          VARCHAR(50) NOT NULL,  -- 'created', 'anchored', 'attested', 'verified', 'retrieved'
     event_timestamp     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
     actor_type          VARCHAR(50) NOT NULL,  -- 'validator', 'api', 'system', 'external'
     actor_id            VARCHAR(256),
     previous_hash       BYTEA,
     current_hash        BYTEA NOT NULL,
     event_details       JSONB,
     signature           BYTEA
 );

 -- 4. External API Keys
 CREATE TABLE IF NOT EXISTS api_keys (
     key_id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
     key_hash            BYTEA NOT NULL,
     client_name         VARCHAR(256) NOT NULL,
     client_type         VARCHAR(50) NOT NULL,  -- 'auditor', 'service', 'institution'
     can_read_proofs     BOOLEAN NOT NULL DEFAULT TRUE,
     can_request_proofs  BOOLEAN NOT NULL DEFAULT FALSE,
     can_bulk_download   BOOLEAN NOT NULL DEFAULT FALSE,
     rate_limit_per_min  INTEGER NOT NULL DEFAULT 100,
     is_active           BOOLEAN NOT NULL DEFAULT TRUE,
     expires_at          TIMESTAMPTZ,
     created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
 );

 ---
 4. API Design

 Existing Endpoints (proof_handlers.go)

 | Method | Endpoint                             | Description                         |
 |--------|--------------------------------------|-------------------------------------|
 | GET    | /api/v1/proofs/tx/{accum_tx_hash}    | Get proof by Accumulate tx hash     |
 | GET    | /api/v1/proofs/{proof_id}            | Get proof by ID with full details   |
 | GET    | /api/v1/proofs/account/{account_url} | List proofs for account (paginated) |
 | GET    | /api/v1/proofs/batch/{batch_id}      | List proofs in batch                |
 | POST   | /api/v1/proofs/query                 | Query with filters                  |

 New Endpoints Required

 Proof Request

 POST /api/v1/proofs/request
 {
   "accum_tx_hash": "64-char-hex",
   "account_url": "acc://example.acme/tokens",
   "proof_class": "on_demand" | "on_cadence",
   "governance_level": "G0" | "G1" | "G2",
   "callback_url": "https://..."  // Optional webhook
 }

 Bundle Download

 GET /api/v1/proofs/{proof_id}/bundle
 Response: Binary (gzipped JSON bundle)
 Headers:
   X-Bundle-Hash: sha256:abc123...
   X-Bundle-Format: certen_v1

 Bundle Verification

 GET /api/v1/proofs/{proof_id}/bundle/verify
 {
   "bundle_valid": true,
   "components": {
     "chained_proof": true,
     "governance_proof": true,
     "merkle_proof": true,
     "anchor_reference": true
   }
 }

 Bulk Export (for auditors)

 POST /api/v1/proofs/bulk/export
 {
   "account_urls": ["acc://..."],
   "date_range": {"start": "2025-01-01", "end": "2025-01-31"},
   "format": "json_lines" | "csv" | "parquet"
 }

 Verification Endpoints

 POST /api/v1/proofs/verify/merkle    -- Verify merkle inclusion
 POST /api/v1/proofs/verify/governance -- Verify governance proof

 ---
 5. Proof Bundle Format (CertenProofBundle v1.0)

 {
   "$schema": "https://certen.io/schemas/proof-bundle/v1.0",
   "bundle_version": "1.0",
   "bundle_id": "uuid",
   "generated_at": "2025-01-15T12:00:00Z",

   "transaction_reference": {
     "accum_tx_hash": "64-char-hex",
     "account_url": "acc://example.acme/tokens",
     "transaction_type": "sendTokens"
   },

   "proof_components": {
     "1_merkle_inclusion": {
       "merkle_root": "32-byte-hex",
       "leaf_hash": "32-byte-hex",
       "leaf_index": 5,
       "merkle_path": [{"hash": "...", "right": true}]
     },

     "2_anchor_reference": {
       "target_chain": "ethereum",
       "anchor_tx_hash": "0x...",
       "anchor_block_number": 12345678,
       "confirmations": 12
     },

     "3_chained_proof": {
       "layer1": { /* tx_to_bvn */ },
       "layer2": { /* bvn_to_dn */ },
       "layer3": { /* dn_to_consensus */ }
     },

     "4_governance_proof": {
       "level": "G1",
       "g0": { /* execution_inclusion */ },
       "g1": { /* authority_snapshot + signatures */ }
     }
   },

   "validator_attestations": [
     {
       "validator_id": "...",
       "signature": "64-byte-hex",
       "attested_at": "..."
     }
   ],

   "bundle_integrity": {
     "artifact_hash": "sha256:...",
     "custody_chain_hash": "sha256:...",
     "bundle_signature": "64-byte-hex"
   }
 }

 ---
 6. User Interface Design

 Tech Stack (From Accumulate Explorer)

 - Framework: React 17 + React Router 5
 - UI Library: Ant Design (antd) v4.16.13
 - Build Tool: Vite v5
 - SDK: JavaScript Accumulate SDK (api_v3 JsonRpcClient)
 - Icons: React Icons (Remix Icons)

 Reusable Components from Explorer

 Copy directly from C:\Accumulate_Stuff\accumulate-explorer-clone\explorer\src\components\:

 | Component      | Source                | Purpose                  |
 |----------------|-----------------------|--------------------------|
 | Network.tsx    | common/Network.tsx    | Global API context       |
 | useAsync.tsx   | common/useAsync.tsx   | Async data fetching hook |
 | query.ts       | common/query.ts       | Query effect patterns    |
 | InfoTable.tsx  | common/InfoTable.tsx  | Structured data display  |
 | RawData.tsx    | common/RawData.tsx    | JSON syntax highlighting |
 | SearchForm.tsx | common/SearchForm.tsx | Account/tx search        |
 | EntryHash.tsx  | common/EntryHash.tsx  | Hash formatting          |
 | Amount.tsx     | common/Amount.tsx     | Token display            |
 | Link.tsx       | common/Link.tsx       | Navigation links         |
 | AccTitle.tsx   | common/AccTitle.tsx   | Header with account info |
 | Chain.tsx      | account/Chain.tsx     | Paginated chain display  |
 | Status.tsx     | message/Status.tsx    | Status badges            |
 | Signatures.tsx | common/Signatures.tsx | Signature visualization  |

 Utilities from explorer/src/utils/:
 - ManagedRange.ts - Pagination management
 - ChainFilter.ts - Chain filtering
 - types.ts - Type definitions

 Key Screens

 1. Proof Explorer Dashboard
   - Search bar (tx hash, account URL, batch ID) - use SearchForm.tsx
   - Recent proofs table with status indicators - use Chain.tsx pattern
   - Current batch status, system health
 2. Proof Detail View
   - Overview card (TX hash, account, status) - use AccTitle.tsx, InfoTable.tsx
   - Visual proof chain diagram (L1->L2->L3) - NEW: custom component
   - Governance panel (G0/G1/G2 levels) - use RawData.tsx
   - Anchor reference with block explorer links - use Link.tsx
   - Attestations list - use Signatures.tsx
   - Download bundle button
 3. Account Timeline
   - Chronological list - use Chain.tsx pattern
   - Filter by governance level, date range
   - Bulk export for auditing
 4. Batch Inspector
   - Merkle root, transaction count - use InfoTable.tsx
   - Anchor confirmation tracker (0/12 -> 12/12)
   - Multi-validator attestation status - use Status.tsx

 Components to Build NEW

 1. MerkleTreeVisualization - Receipt path as tree diagram (not in Explorer)
 2. ProofChainDiagram - L1->L2->L3 visual flow
 3. GovernanceLevelCard - G0/G1/G2 display with verification status
 4. AnchorProgressTracker - Confirmation progress (0/12 -> 12/12)
 5. BundleDownloader - Download verification bundle

 Proof Chain Visualization

 +-------------+     +-------------+     +-------------+
 | Transaction |---->|     BVN     |---->|     DN      |
 |   (Leaf)    |     | (Layer 1)   |     | (Layer 2-3) |
 +-------------+     +-------------+     +-------------+
       |                   |                   |
       v                   v                   v
 +-------------+     +-------------+     +-------------+
 | Merkle Path |     | BVN Receipt |     | DN Consensus|
 |  (Batch)    |     |   + BPT     |     | + Validators|
 +-------------+     +-------------+     +-------------+
                           |
                           v
                   +---------------+
                   | Anchor Chain  |
                   | (ETH/BTC)     |
                   +---------------+

 SDK Integration

 Use JsonRpcClient from JavaScript SDK for all queries:

 import { JsonRpcClient } from 'accumulate.js/api_v3';

 // Query with receipt
 const result = await api.query(url, {
   queryType: 'chain',
   name: 'main',
   includeReceipt: true
 });

 // Receipt structure available:
 // result.receipt.entries - Merkle path
 // result.receipt.anchor - Final anchor hash

 ---
 7. Implementation Phases

 Phase 1: Proof Artifact Collection & Bundle Format (2-3 weeks)

 Goal: Collect artifacts from existing proof generators, create bundle format

 Note: Proofs are already generated by existing code. This phase creates the service layer to collect and bundle them.

 Tasks:
 - Create ProofArtifactService that collects outputs from existing proof generators
 - Implement CertenProofBundle format serialization (JSON schema)
 - Add proof artifact storage via enhanced PostgreSQL schema
 - Unit tests for artifact collection and bundling

 Files:
 - NEW: services/validator/pkg/proof/artifact_service.go
 - NEW: services/validator/pkg/proof/bundle_format.go
 - MODIFY: services/validator/pkg/database/proof_artifact_repository.go

 Existing Proof Sources (read artifacts from):
 - accumulate-lite-client-2/liteclient/proof/working-proof/ - ChainedProof L1/L2/L3
 - accumulate-lite-client-2/liteclient/proof/consolidated_governance-proof/ - G0/G1/G2

 Phase 2: Lifecycle Management (1-2 weeks)

 Goal: State machine for proof lifecycle with batch scheduling

 Tasks:
 - Implement ProofLifecycleManager with state transitions
 - Create AnchorSchedulerService for on-cadence/on-demand
 - Add database migration 003_proof_service_enhancements.sql
 - Implement custody chain tracking

 Files:
 - NEW: services/validator/pkg/proof/lifecycle.go
 - NEW: services/validator/pkg/anchor/scheduler.go
 - NEW: services/validator/pkg/database/migrations/003_proof_service_enhancements.sql

 Phase 3: Attestation System (1-2 weeks)

 Goal: Multi-validator attestation collection (2/3+1 quorum)

 Tasks:
 - Implement AttestationCollectorService
 - P2P attestation broadcast
 - Quorum verification logic
 - Ed25519 signature verification

 Files:
 - NEW: services/validator/pkg/proof/attestation.go
 - MODIFY: services/validator/pkg/database/repository_attestation.go

 Phase 4: External API Enhancement (1-2 weeks)

 Goal: Complete public API for external services

 Tasks:
 - Add proof request endpoints
 - Implement bundle download endpoint
 - Add bulk export functionality
 - Rate limiting and API key validation
 - OpenAPI specification

 Files:
 - MODIFY: services/validator/pkg/server/proof_handlers.go
 - NEW: services/validator/pkg/server/bundle_handlers.go
 - NEW: services/validator/pkg/server/bulk_handlers.go
 - NEW: docs/api/openapi.yaml

 Phase 5: User Interface (2-3 weeks)

 Goal: Proof explorer web interface using Accumulate Explorer patterns

 Tasks:
 - Copy reusable components from Accumulate Explorer (see Section 6)
 - Adapt Network context for Proof Service API
 - Build Proof Explorer dashboard using SearchForm, Chain patterns
 - Build Proof detail view with new ProofChainDiagram component
 - Build Account timeline using Chain.tsx pattern
 - Create custom MerkleTreeVisualization component
 - Integrate JavaScript SDK for receipt queries

 Files:
 - NEW: web/proof-explorer/ (Vite + React + Ant Design)
 - COPY FROM: C:\Accumulate_Stuff\accumulate-explorer-clone\explorer\src\components\common\
 - COPY FROM: C:\Accumulate_Stuff\accumulate-explorer-clone\explorer\src\utils\
 - USE SDK: C:\Accumulate_Stuff\typescript-sdk-accumulate-mod\javascript

 Phase 6: Integration Testing & Hardening (1-2 weeks)

 Goal: End-to-end testing and documentation

 Tasks:
 - Integration tests with devnet
 - Performance testing for bulk operations
 - Security audit of API endpoints
 - Documentation finalization

 ---
 8. Critical Files

 Existing Proof Implementations (READ - artifacts come from here)

 | File                                                                                        | Purpose                              |
 |---------------------------------------------------------------------------------------------|--------------------------------------|
 | services/validator/accumulate-lite-client-2/liteclient/proof/working-proof/                 | ChainedProof L1/L2/L3 implementation |
 | services/validator/accumulate-lite-client-2/liteclient/proof/consolidated_governance-proof/ | Governance G0/G1/G2 implementation   |
 | services/validator/pkg/proof/liteclient_adapter.go                                          | LiteClient proof generation          |
 | services/validator/pkg/proof/governance_types.go                                            | G0/G1/G2 type definitions            |
 | contracts/ethereum/contracts/CertenAnchorV3.sol                                             | ZK-BLS anchor verification           |

 Existing Database/API (MODIFY)

 | File                                                         | Purpose                  |
 |--------------------------------------------------------------|--------------------------|
 | services/validator/pkg/database/proof_artifact_repository.go | Proof storage repository |
 | services/validator/pkg/server/proof_handlers.go              | API handlers             |
 | services/validator/pkg/database/migrations/                  | Database schemas         |

 External Resources (COPY/USE)

 | Path                                                                          | Purpose                   |
 |-------------------------------------------------------------------------------|---------------------------|
 | C:\Accumulate_Stuff\typescript-sdk-accumulate-mod\javascript                  | JavaScript SDK for UI     |
 | C:\Accumulate_Stuff\accumulate-explorer-clone\explorer\src\components\common\ | Reusable React components |
 | C:\Accumulate_Stuff\accumulate-explorer-clone\explorer\src\utils\             | Utility functions         |

 New Files to Create

 | File                                                                          | Purpose                                                  |
 |-------------------------------------------------------------------------------|----------------------------------------------------------|
 | services/validator/pkg/proof/artifact_service.go                              | ProofArtifactService (collects from existing generators) |
 | services/validator/pkg/proof/bundle_format.go                                 | CertenProofBundle serialization                          |
 | services/validator/pkg/proof/lifecycle.go                                     | ProofLifecycleManager                                    |
 | services/validator/pkg/proof/attestation.go                                   | AttestationCollectorService                              |
 | services/validator/pkg/anchor/scheduler.go                                    | AnchorSchedulerService                                   |
 | services/validator/pkg/server/bundle_handlers.go                              | Bundle download API                                      |
 | services/validator/pkg/server/bulk_handlers.go                                | Bulk export API                                          |
 | services/validator/pkg/database/migrations/003_proof_service_enhancements.sql | Schema enhancements                                      |
 | web/proof-explorer/                                                           | React UI application                                     |

 ---
 9. Success Criteria

 1. Proof Generation: All 4 proof components generated correctly for any Accumulate transaction
 2. External Discovery: External services can discover proofs by tx hash, account, or batch
 3. Bundle Retrieval: Self-contained verification bundles downloadable and verifiable offline
 4. Audit Compliance: Complete custody chain tracking for regulatory requirements
 5. Performance: Support for bulk export of 10,000+ proofs for auditing
 6. UI: Intuitive proof explorer with visual chain diagram