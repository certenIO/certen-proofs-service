// Certen Proof Explorer - Type Definitions
// Mirrors the OpenAPI schema for type safety

export type ProofStatus =
  | 'pending'
  | 'batched'
  | 'anchored'
  | 'attested'
  | 'verified'
  | 'failed';

export type GovernanceLevel = 'G0' | 'G1' | 'G2';

export type ProofClass = 'on_cadence' | 'on_demand';

export type ProofType = 'certen_anchor' | 'chained' | 'governance' | 'merkle';

export interface ProofArtifact {
  proof_id: string;
  proof_type: ProofType;
  proof_version: string;
  accum_tx_hash: string;
  account_url: string;
  batch_id?: string;
  anchor_tx_hash?: string;
  anchor_chain?: string;
  anchor_block_number?: number;
  gov_level?: GovernanceLevel;
  proof_class: ProofClass;
  status: ProofStatus;
  created_at: string;
  anchored_at?: string;
  verified_at?: string;
}

export interface GovernanceProofLevel {
  level_id: string;
  gov_level: GovernanceLevel;
  level_name: string;
  block_height?: number;
  finality_timestamp?: string;
  anchor_height?: number;
  authority_url?: string;
  threshold_m?: number;
  threshold_n?: number;
  signature_count?: number;
  outcome_type?: string;
  verified: boolean;
  verified_at?: string;
}

export interface ProofAttestation {
  attestation_id: string;
  validator_id: string;
  validator_pubkey?: string;
  signature: string;
  attested_at: string;
  is_valid: boolean;
}

export interface ProofWithDetails extends ProofArtifact {
  merkle_root?: string;
  leaf_hash?: string;
  leaf_index?: number;
  artifact_hash?: string;
  governance_levels?: GovernanceProofLevel[];
  attestations?: ProofAttestation[];
}

export interface ProofList {
  proofs: ProofArtifact[];
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
}

export interface MerklePathEntry {
  hash: string;
  right: boolean;
}

export interface MerkleInclusionProof {
  merkle_root: string;
  leaf_hash: string;
  leaf_index: number;
  merkle_path: MerklePathEntry[];
}

export interface AnchorReference {
  target_chain: string;
  anchor_tx_hash: string;
  anchor_block_number: number;
  confirmations: number;
}

export interface ProofLayer {
  layer_name: string;
  source_hash: string;
  target_hash: string;
  receipt?: {
    start: string;
    anchor: string;
    entries: Array<{hash: string; right: boolean}>;
  };
  verified: boolean;
}

export interface ChainedProof {
  layer1?: ProofLayer;
  layer2?: ProofLayer;
  layer3?: ProofLayer;
}

export interface GovernanceProof {
  level: GovernanceLevel;
  g0?: Record<string, unknown>;
  g1?: Record<string, unknown>;
  g2?: Record<string, unknown>;
}

export interface ValidatorAttestation {
  validator_id: string;
  signature: string;
  attested_at: string;
}

export interface BundleIntegrity {
  artifact_hash: string;
  custody_chain_hash: string;
  bundle_signature?: string;
}

export interface CertenProofBundle {
  $schema: string;
  bundle_version: string;
  bundle_id: string;
  generated_at: string;
  transaction_reference: {
    accum_tx_hash: string;
    account_url: string;
    transaction_type?: string;
  };
  proof_components: {
    '1_merkle_inclusion': MerkleInclusionProof;
    '2_anchor_reference': AnchorReference;
    '3_chained_proof': ChainedProof;
    '4_governance_proof': GovernanceProof;
  };
  validator_attestations: ValidatorAttestation[];
  bundle_integrity: BundleIntegrity;
}

export interface CustodyEvent {
  event_id: string;
  event_type: 'created' | 'anchored' | 'attested' | 'verified' | 'retrieved';
  event_timestamp: string;
  actor_type: string;
  actor_id?: string;
  previous_hash?: string;
  current_hash: string;
}

export interface CustodyChain {
  proof_id: string;
  chain_length: number;
  chain_valid: boolean;
  events: CustodyEvent[];
}

export interface BundleVerificationResult {
  bundle_id: string;
  proof_id: string;
  bundle_valid: boolean;
  components: {
    chained_proof: boolean;
    governance_proof: boolean;
    merkle_proof: boolean;
    anchor_reference: boolean;
  };
  integrity: {
    hash_valid: boolean;
    format_valid: boolean;
    attestations_valid: boolean;
  };
  verification_timestamp: string;
}

export interface ProofStatistics {
  total_proofs: number;
  proofs_by_status: Record<string, number>;
  proofs_by_type: Record<string, number>;
  proofs_by_gov_level: Record<string, number>;
  attestation_stats: {
    total_attestations: number;
    unique_validators: number;
    quorum_rate: number;
  };
  time_windows: {
    last_24h: {
      proofs_created: number;
      proofs_verified: number;
      bundles_downloaded: number;
    };
    last_7d: {
      proofs_created: number;
      proofs_verified: number;
      bundles_downloaded: number;
    };
  };
  generated_at: string;
}

export interface SystemHealth {
  status: 'healthy' | 'degraded' | 'unhealthy';
  version: string;
  uptime_seconds: number;
  components: {
    database: { status: string; latency_ms: number };
    accumulate_client: { status: string; connected: boolean };
    anchor_service: { status: string; last_anchor_at?: string };
    attestation_service: { status: string; active_validators: number };
  };
  timestamp: string;
}
