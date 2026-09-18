// Copyright 2025 Certen Protocol
//
// Read endpoints for the proof records the validators write beside each proof artifact: the four-component
// Certen anchor proof, the proof's level record (levels 1-4 and the cycle binding), the external chain
// results with their hash chain and BLS attestations, the validator set a quorum was counted against, and
// proof requests.

package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// ProofRecordHandlers serves the proof records.
type ProofRecordHandlers struct {
	repos  *database.Repositories
	logger *log.Logger
	api    *ProofHandlers // for its JSON and error writers
}

// NewProofRecordHandlers creates the proof record handlers.
func NewProofRecordHandlers(repos *database.Repositories, logger *log.Logger) *ProofRecordHandlers {
	if logger == nil {
		logger = log.New(log.Writer(), "[ProofRecords] ", log.LstdFlags)
	}
	return &ProofRecordHandlers{repos: repos, logger: logger, api: &ProofHandlers{repos: repos, logger: logger}}
}

// CertenProofView is a stored Certen proof and whether its hash still covers its content.
type CertenProofView struct {
	*database.CertenAnchorProof
	ProofHashVerified bool `json:"proof_hash_verified"`
}

// ResultView is one external chain result with the attestations over it.
type ResultView struct {
	database.ExternalChainResultRecord
	Attestations       []database.BLSAttestationRecord       `json:"attestations"`
	MessagesConsistent bool                                  `json:"messages_consistent"`
	Aggregate          *database.AggregatedAttestationRecord `json:"aggregate,omitempty"`
}

// ResultsView is a proof's external chain results and whether they form an intact hash chain.
type ResultsView struct {
	ProofID        uuid.UUID    `json:"proof_id"`
	HashChainValid bool         `json:"hash_chain_valid"`
	Results        []ResultView `json:"results"`
}

func (h *ProofRecordHandlers) begin(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, bool) {
	if r.Method != http.MethodGet {
		h.api.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return nil, nil, false
	}
	if h.repos == nil {
		h.api.writeError(w, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "Database not available")
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	return ctx, cancel, true
}

// pathParam returns the path segment after prefix, up to the next slash.
func pathParam(r *http.Request, prefix string) string {
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if rest == r.URL.Path {
		return ""
	}
	return strings.Split(rest, "/")[0]
}

func (h *ProofRecordHandlers) parseUUID(w http.ResponseWriter, value, what string) (uuid.UUID, bool) {
	id, err := uuid.Parse(value)
	if err != nil {
		h.api.writeError(w, http.StatusBadRequest, "INVALID_ID", fmt.Sprintf("Invalid %s", what))
		return uuid.Nil, false
	}
	return id, true
}

func (h *ProofRecordHandlers) fail(w http.ResponseWriter, what string, err error) {
	h.logger.Printf("Error getting %s: %v", what, err)
	h.api.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", fmt.Sprintf("Failed to retrieve %s", what))
}

func (h *ProofRecordHandlers) writeCertenProof(w http.ResponseWriter, proof *database.CertenAnchorProof, err error) {
	if errors.Is(err, database.ErrProofNotFound) {
		h.api.writeError(w, http.StatusNotFound, "CERTEN_PROOF_NOT_FOUND", "No Certen proof found")
		return
	}
	if err != nil {
		h.fail(w, "Certen proof", err)
		return
	}
	h.api.writeJSON(w, http.StatusOK, CertenProofView{CertenAnchorProof: proof, ProofHashVerified: proof.VerifyProofHash()})
}

// HandleGetProofCertenProof handles GET /api/v1/proofs/{proof_id}/certen
func (h *ProofRecordHandlers) HandleGetProofCertenProof(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	proofID, ok := h.parseUUID(w, pathParam(r, "/api/v1/proofs/"), "proof ID")
	if !ok {
		return
	}
	proof, err := h.repos.Proofs.GetProofByArtifactID(ctx, proofID)
	h.writeCertenProof(w, proof, err)
}

// HandleGetCertenProof handles GET /api/v1/certen-proofs/{id} and /api/v1/certen-proofs/tx/{accum_tx_hash}
func (h *ProofRecordHandlers) HandleGetCertenProof(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	if txHash := pathParam(r, "/api/v1/certen-proofs/tx/"); txHash != "" {
		proof, err := h.repos.Proofs.GetProofByAccumTxHash(ctx, txHash)
		h.writeCertenProof(w, proof, err)
		return
	}
	id, ok := h.parseUUID(w, pathParam(r, "/api/v1/certen-proofs/"), "Certen proof ID")
	if !ok {
		return
	}
	proof, err := h.repos.Proofs.GetProof(ctx, id)
	h.writeCertenProof(w, proof, err)
}

// HandleGetProofCycle handles GET /api/v1/proofs/{proof_id}/cycle
func (h *ProofRecordHandlers) HandleGetProofCycle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	proofID, ok := h.parseUUID(w, pathParam(r, "/api/v1/proofs/"), "proof ID")
	if !ok {
		return
	}
	completion, err := h.repos.ProofArtifacts.GetProofCycleCompletionByProof(ctx, proofID)
	if err != nil {
		h.fail(w, "proof cycle", err)
		return
	}
	if completion == nil {
		h.api.writeError(w, http.StatusNotFound, "PROOF_CYCLE_NOT_FOUND", fmt.Sprintf("No proof cycle record for proof %s", proofID))
		return
	}
	h.api.writeJSON(w, http.StatusOK, completion)
}

// HandleGetIncompleteProofCycles handles GET /api/v1/proof-cycles/incomplete?limit=N
func (h *ProofRecordHandlers) HandleGetIncompleteProofCycles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	limit := h.api.parseIntParam(r, "limit", 100)
	if limit > 1000 {
		limit = 1000
	}
	cycles, err := h.repos.ProofArtifacts.GetIncompleteProofCycles(ctx, limit)
	if err != nil {
		h.fail(w, "incomplete proof cycles", err)
		return
	}
	h.api.writeJSON(w, http.StatusOK, map[string]interface{}{"cycles": cycles, "count": len(cycles)})
}

// HandleGetProofResults handles GET /api/v1/proofs/{proof_id}/results
func (h *ProofRecordHandlers) HandleGetProofResults(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	proofID, ok := h.parseUUID(w, pathParam(r, "/api/v1/proofs/"), "proof ID")
	if !ok {
		return
	}
	repo := h.repos.ProofArtifacts
	results, err := repo.GetExternalChainResultsByProof(ctx, proofID)
	if err != nil {
		h.fail(w, "external chain results", err)
		return
	}
	chainValid, err := repo.VerifyExternalChainResultHashChain(ctx, proofID)
	if err != nil {
		h.fail(w, "result hash chain", err)
		return
	}
	view := ResultsView{ProofID: proofID, HashChainValid: chainValid, Results: make([]ResultView, 0, len(results))}
	for _, result := range results {
		attestations, err := repo.GetBLSAttestationsByResult(ctx, result.ResultID)
		if err != nil {
			h.fail(w, "BLS attestations", err)
			return
		}
		consistent, err := repo.VerifyBLSAttestationMessageConsistency(ctx, result.ResultID)
		if err != nil {
			h.fail(w, "attestation consistency", err)
			return
		}
		aggregate, err := repo.GetAggregatedAttestationByResult(ctx, result.ResultID)
		if err != nil {
			h.fail(w, "aggregated attestation", err)
			return
		}
		if attestations == nil {
			attestations = []database.BLSAttestationRecord{}
		}
		view.Results = append(view.Results, ResultView{ExternalChainResultRecord: result, Attestations: attestations, MessagesConsistent: consistent, Aggregate: aggregate})
	}
	h.api.writeJSON(w, http.StatusOK, view)
}

// HandleGetValidatorSet handles GET /api/v1/validator-sets/{snapshot_id} and
// /api/v1/validator-sets/latest/{chain_id}
func (h *ProofRecordHandlers) HandleGetValidatorSet(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	var snapshot *database.ValidatorSetSnapshotRecord
	var err error
	if chainID := pathParam(r, "/api/v1/validator-sets/latest/"); chainID != "" {
		snapshot, err = h.repos.ProofArtifacts.GetLatestValidatorSetSnapshot(ctx, chainID)
	} else {
		id, ok := h.parseUUID(w, pathParam(r, "/api/v1/validator-sets/"), "validator set ID")
		if !ok {
			return
		}
		snapshot, err = h.repos.ProofArtifacts.GetValidatorSetSnapshotByID(ctx, id)
	}
	if err != nil {
		h.fail(w, "validator set", err)
		return
	}
	if snapshot == nil {
		h.api.writeError(w, http.StatusNotFound, "VALIDATOR_SET_NOT_FOUND", "No validator set snapshot found")
		return
	}
	h.api.writeJSON(w, http.StatusOK, snapshot)
}

// HandleGetProofRequest handles GET /api/v1/proof-requests/{request_id} and
// /api/v1/proof-requests/requester/{requester_id}?limit=N
func (h *ProofRecordHandlers) HandleGetProofRequest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, ok := h.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	if requester := pathParam(r, "/api/v1/proof-requests/requester/"); requester != "" {
		limit := h.api.parseIntParam(r, "limit", 50)
		if limit > 500 {
			limit = 500
		}
		requests, err := h.repos.Requests.GetRequestsByRequester(ctx, requester, limit)
		if err != nil {
			h.fail(w, "proof requests", err)
			return
		}
		h.api.writeJSON(w, http.StatusOK, map[string]interface{}{"requester_id": requester, "requests": requests, "count": len(requests)})
		return
	}
	id, ok := h.parseUUID(w, pathParam(r, "/api/v1/proof-requests/"), "request ID")
	if !ok {
		return
	}
	request, err := h.repos.Requests.GetRequest(ctx, id)
	if errors.Is(err, database.ErrRequestNotFound) {
		h.api.writeError(w, http.StatusNotFound, "REQUEST_NOT_FOUND", fmt.Sprintf("No request found with ID: %s", id))
		return
	}
	if err != nil {
		h.fail(w, "proof request", err)
		return
	}
	h.api.writeJSON(w, http.StatusOK, request)
}
