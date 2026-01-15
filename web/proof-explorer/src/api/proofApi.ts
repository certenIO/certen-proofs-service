// Certen Proof Explorer - API Client
// Provides typed access to the Proof Service API

import {
  ProofArtifact,
  ProofWithDetails,
  ProofList,
  CertenProofBundle,
  BundleVerificationResult,
  CustodyChain,
  ProofStatistics,
  SystemHealth,
  ProofStatus,
  GovernanceLevel,
  ProofClass,
} from '../types/proof';

const API_BASE = '/api/v1';

interface ApiError {
  error: string;
  message: string;
  details?: Record<string, unknown>;
}

class ProofApiClient {
  private apiKey?: string;

  setApiKey(key: string) {
    this.apiKey = key;
  }

  private async fetch<T>(
    path: string,
    options: RequestInit = {}
  ): Promise<T> {
    const headers: HeadersInit = {
      'Content-Type': 'application/json',
      ...(this.apiKey ? { 'X-API-Key': this.apiKey } : {}),
      ...options.headers,
    };

    const response = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const error: ApiError = await response.json().catch(() => ({
        error: 'UNKNOWN_ERROR',
        message: response.statusText,
      }));
      throw new Error(`${error.error}: ${error.message}`);
    }

    return response.json();
  }

  // Proof retrieval
  async getProofByTxHash(txHash: string): Promise<ProofArtifact> {
    return this.fetch<ProofArtifact>(`/proofs/tx/${txHash}`);
  }

  async getProofById(proofId: string): Promise<ProofWithDetails> {
    return this.fetch<ProofWithDetails>(`/proofs/${proofId}`);
  }

  async getProofsByAccount(
    accountUrl: string,
    options: { limit?: number; offset?: number; status?: ProofStatus; gov_level?: GovernanceLevel } = {}
  ): Promise<ProofList> {
    const params = new URLSearchParams();
    if (options.limit) params.set('limit', String(options.limit));
    if (options.offset) params.set('offset', String(options.offset));
    if (options.status) params.set('status', options.status);
    if (options.gov_level) params.set('gov_level', options.gov_level);

    const queryString = params.toString();
    return this.fetch<ProofList>(
      `/proofs/account/${encodeURIComponent(accountUrl)}${queryString ? `?${queryString}` : ''}`
    );
  }

  async getProofsByBatch(batchId: string): Promise<ProofList> {
    return this.fetch<ProofList>(`/proofs/batch/${batchId}`);
  }

  async queryProofs(query: {
    accum_tx_hash?: string;
    account_url?: string;
    batch_id?: string;
    status?: ProofStatus;
    gov_level?: GovernanceLevel;
    proof_class?: ProofClass;
    created_after?: string;
    created_before?: string;
    limit?: number;
    offset?: number;
  }): Promise<ProofList> {
    return this.fetch<ProofList>('/proofs/query', {
      method: 'POST',
      body: JSON.stringify(query),
    });
  }

  // Proof request
  async requestProof(request: {
    accum_tx_hash?: string;
    account_url?: string;
    proof_class: ProofClass;
    governance_level?: GovernanceLevel;
    callback_url?: string;
  }): Promise<{ request_id: string; status: string; estimated_time_ms: number; message: string }> {
    return this.fetch('/proofs/request', {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  // Bundle operations
  async downloadBundle(proofId: string): Promise<CertenProofBundle> {
    const response = await fetch(`${API_BASE}/proofs/${proofId}/bundle`, {
      headers: {
        Accept: 'application/json',
        'Accept-Encoding': 'identity',
        ...(this.apiKey ? { 'X-API-Key': this.apiKey } : {}),
      },
    });

    if (!response.ok) {
      throw new Error(`Failed to download bundle: ${response.statusText}`);
    }

    return response.json();
  }

  async downloadBundleBlob(proofId: string): Promise<Blob> {
    const response = await fetch(`${API_BASE}/proofs/${proofId}/bundle`, {
      headers: {
        Accept: 'application/gzip',
        ...(this.apiKey ? { 'X-API-Key': this.apiKey } : {}),
      },
    });

    if (!response.ok) {
      throw new Error(`Failed to download bundle: ${response.statusText}`);
    }

    return response.blob();
  }

  async verifyBundle(proofId: string): Promise<BundleVerificationResult> {
    return this.fetch<BundleVerificationResult>(`/proofs/${proofId}/bundle/verify`);
  }

  async getCustodyChain(proofId: string): Promise<CustodyChain> {
    return this.fetch<CustodyChain>(`/proofs/${proofId}/custody`);
  }

  // Verification
  async verifyMerkle(request: {
    proof_id?: string;
    merkle_proof?: {
      merkle_root: string;
      leaf_hash: string;
      leaf_index: number;
      merkle_path: Array<{ hash: string; right: boolean }>;
    };
  }): Promise<{ valid: boolean; computed_root: string; expected_root: string; root_matches: boolean }> {
    return this.fetch('/proofs/verify/merkle', {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  async verifyGovernance(request: {
    proof_id?: string;
    governance_level: GovernanceLevel;
    proof_data?: Record<string, unknown>;
  }): Promise<{ valid: boolean; governance_level: GovernanceLevel; details: Record<string, unknown> }> {
    return this.fetch('/proofs/verify/governance', {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  // Bulk operations
  async requestBulkExport(request: {
    account_urls?: string[];
    date_range?: { start: string; end: string };
    statuses?: ProofStatus[];
    governance_levels?: GovernanceLevel[];
    format?: 'json_lines' | 'csv';
    include_attestations?: boolean;
    limit?: number;
  }): Promise<{ job_id: string; status: string; estimated_record_count: number; message: string }> {
    return this.fetch('/proofs/bulk/export', {
      method: 'POST',
      body: JSON.stringify(request),
    });
  }

  async getExportStatus(jobId: string): Promise<{
    job_id: string;
    status: string;
    format: string;
    total_count: number;
    processed_count: number;
    file_size_bytes: number;
    started_at: string;
    completed_at?: string;
    error?: string;
  }> {
    return this.fetch(`/proofs/bulk/export/${jobId}`);
  }

  async downloadExport(jobId: string): Promise<Blob> {
    const response = await fetch(`${API_BASE}/proofs/bulk/export/${jobId}/download`, {
      headers: this.apiKey ? { 'X-API-Key': this.apiKey } : {},
    });

    if (!response.ok) {
      throw new Error(`Failed to download export: ${response.statusText}`);
    }

    return response.blob();
  }

  async bulkVerify(proofIds: string[]): Promise<{
    results: Array<{ proof_id: string; valid: boolean; error?: string }>;
    total_verified: number;
    valid_count: number;
    invalid_count: number;
  }> {
    return this.fetch('/proofs/bulk/verify', {
      method: 'POST',
      body: JSON.stringify({ proof_ids: proofIds }),
    });
  }

  // System
  async getProofStats(): Promise<ProofStatistics> {
    return this.fetch<ProofStatistics>('/proofs/stats');
  }

  async getSystemHealth(): Promise<SystemHealth> {
    return this.fetch<SystemHealth>('/system/health');
  }
}

export const proofApi = new ProofApiClient();
export default proofApi;
