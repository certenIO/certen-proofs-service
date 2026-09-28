// Copyright 2025 Certen Protocol
//
// Anchor batch endpoints: GET /api/v1/batches/{batch_id} and /api/v1/batches/{batch_id}/stats.

package server

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/certen/proofs-service/pkg/database"
)

// BatchHandlers serves anchor batches.
type BatchHandlers struct {
	records *ProofRecordHandlers
}

// NewBatchHandlers creates the anchor batch handlers.
func NewBatchHandlers(repos *database.Repositories, logger *log.Logger) *BatchHandlers {
	if logger == nil {
		logger = log.New(log.Writer(), "[Batches] ", log.LstdFlags)
	}
	return &BatchHandlers{records: NewProofRecordHandlers(repos, logger)}
}

// BatchView is an anchor batch as the validators wrote it, with the proofs stored against it.
type BatchView struct {
	*database.AnchorBatchRecord
	Proofs *database.BatchProofStats `json:"proofs"`
}

// HandleBatches handles GET /api/v1/batches/{batch_id} and GET /api/v1/batches/{batch_id}/stats.
func (h *BatchHandlers) HandleBatches(w http.ResponseWriter, r *http.Request) {
	// The request is checked before the database, so a malformed one is answered as such whatever the database's state.
	if r.Method != http.MethodGet {
		h.records.api.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/batches/"), "/")
	if len(parts) > 2 || (len(parts) == 2 && parts[1] != "stats") {
		h.records.api.writeError(w, http.StatusNotFound, "NOT_FOUND", "No such batch endpoint")
		return
	}
	id, ok := h.records.parseUUID(w, parts[0], "batch ID")
	if !ok {
		return
	}
	ctx, cancel, ok := h.records.begin(w, r)
	if !ok {
		return
	}
	defer cancel()
	batch, err := h.records.repos.Batches.GetBatch(ctx, id)
	if errors.Is(err, database.ErrBatchNotFound) {
		h.records.api.writeError(w, http.StatusNotFound, "BATCH_NOT_FOUND", "No anchor batch with ID "+id.String())
		return
	}
	if err != nil {
		h.records.fail(w, "anchor batch", err)
		return
	}
	stats, err := h.records.repos.ProofArtifacts.GetBatchProofStats(ctx, id)
	if err != nil {
		h.records.fail(w, "anchor batch proof counts", err)
		return
	}
	if len(parts) == 2 {
		h.records.api.writeJSON(w, http.StatusOK, stats)
		return
	}
	h.records.api.writeJSON(w, http.StatusOK, BatchView{AnchorBatchRecord: batch, Proofs: stats})
}
