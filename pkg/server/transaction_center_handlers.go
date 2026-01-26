// Copyright 2025 Certen Protocol
//
// Transaction Center API Handlers
// Provides endpoints for web app Transaction Center integration
//
// Endpoints:
// - GET /api/v1/intents/{intentId}/proof        - Full proof with layers
// - GET /api/v1/intents/{intentId}/timeline     - Custody chain events
// - GET /api/v1/intents/{intentId}/attestations - Validator signatures
// - GET /api/v1/user/{userId}/intents           - User's intent list
// - GET /api/v1/audit/intents                   - Audit search

package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/certen/proofs-service/pkg/database"
)

// TransactionCenterHandlers provides HTTP handlers for Transaction Center operations
type TransactionCenterHandlers struct {
	repos       *database.Repositories
	validatorID string
	logger      *log.Logger
}

// NewTransactionCenterHandlers creates new Transaction Center handlers
func NewTransactionCenterHandlers(repos *database.Repositories, validatorID string, logger *log.Logger) *TransactionCenterHandlers {
	if logger == nil {
		logger = log.New(log.Writer(), "[TransactionCenterAPI] ", log.LstdFlags)
	}
	return &TransactionCenterHandlers{
		repos:       repos,
		validatorID: validatorID,
		logger:      logger,
	}
}

// ============================================================================
// INTENT DETAIL ENDPOINTS
// ============================================================================

// HandleGetIntentProof handles GET /api/v1/intents/{intentId}/proof
func (h *TransactionCenterHandlers) HandleGetIntentProof(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}

	// Extract intent ID from path: /api/v1/intents/{intentId}/proof
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/intents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "proof" {
		h.writeError(w, http.StatusBadRequest, "INVALID_PATH", "Invalid path format")
		return
	}
	intentID := parts[0]
	if intentID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_INTENT_ID", "Intent ID is required")
		return
	}

	ctx := r.Context()
	details, err := h.repos.ProofArtifacts.GetProofByIntentID(ctx, intentID)
	if err != nil {
		h.logger.Printf("Error getting proof by intent: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve proof")
		return
	}

	if details == nil {
		h.writeError(w, http.StatusNotFound, "INTENT_NOT_FOUND", fmt.Sprintf("No proof found for intent: %s", intentID))
		return
	}

	h.writeJSON(w, http.StatusOK, details)
}

// HandleGetIntentTimeline handles GET /api/v1/intents/{intentId}/timeline
func (h *TransactionCenterHandlers) HandleGetIntentTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}

	// Extract intent ID from path: /api/v1/intents/{intentId}/timeline
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/intents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "timeline" {
		h.writeError(w, http.StatusBadRequest, "INVALID_PATH", "Invalid path format")
		return
	}
	intentID := parts[0]
	if intentID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_INTENT_ID", "Intent ID is required")
		return
	}

	ctx := r.Context()
	events, err := h.repos.ProofArtifacts.GetTimelineByIntentID(ctx, intentID)
	if err != nil {
		h.logger.Printf("Error getting timeline: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve timeline")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"intent_id": intentID,
		"events":    events,
		"count":     len(events),
	})
}

// HandleGetIntentAttestations handles GET /api/v1/intents/{intentId}/attestations
func (h *TransactionCenterHandlers) HandleGetIntentAttestations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}

	// Extract intent ID from path: /api/v1/intents/{intentId}/attestations
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/intents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "attestations" {
		h.writeError(w, http.StatusBadRequest, "INVALID_PATH", "Invalid path format")
		return
	}
	intentID := parts[0]
	if intentID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_INTENT_ID", "Intent ID is required")
		return
	}

	ctx := r.Context()
	summary, err := h.repos.ProofArtifacts.GetAttestationsByIntentID(ctx, intentID)
	if err != nil {
		h.logger.Printf("Error getting attestations: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve attestations")
		return
	}

	h.writeJSON(w, http.StatusOK, summary)
}

// ============================================================================
// USER INTENTS ENDPOINT
// ============================================================================

// HandleGetUserIntents handles GET /api/v1/user/{userId}/intents
func (h *TransactionCenterHandlers) HandleGetUserIntents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}

	// Extract user ID from path: /api/v1/user/{userId}/intents
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/user/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "intents" {
		h.writeError(w, http.StatusBadRequest, "INVALID_PATH", "Invalid path format")
		return
	}
	userID := parts[0]
	if userID == "" {
		h.writeError(w, http.StatusBadRequest, "INVALID_USER_ID", "User ID is required")
		return
	}

	// Parse pagination params
	limit := h.parseIntParam(r, "limit", 50)
	offset := h.parseIntParam(r, "offset", 0)
	if limit > 1000 {
		limit = 1000
	}

	ctx := r.Context()
	intents, err := h.repos.ProofArtifacts.GetIntentsByUserID(ctx, userID, limit, offset)
	if err != nil {
		h.logger.Printf("Error getting user intents: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve intents")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": userID,
		"intents": intents,
		"count":   len(intents),
		"limit":   limit,
		"offset":  offset,
	})
}

// ============================================================================
// AUDIT SEARCH ENDPOINT
// ============================================================================

// HandleSearchAuditTrail handles GET /api/v1/audit/intents
func (h *TransactionCenterHandlers) HandleSearchAuditTrail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}

	// Build filter from query params
	filter := &database.IntentFilter{
		Limit:  h.parseIntParam(r, "limit", 50),
		Offset: h.parseIntParam(r, "offset", 0),
	}

	// Parse optional filters
	if v := r.URL.Query().Get("user_id"); v != "" {
		filter.UserID = &v
	}
	if v := r.URL.Query().Get("intent_id"); v != "" {
		filter.IntentID = &v
	}
	if v := r.URL.Query().Get("from_chain"); v != "" {
		filter.FromChain = &v
	}
	if v := r.URL.Query().Get("to_chain"); v != "" {
		filter.ToChain = &v
	}
	if v := r.URL.Query().Get("token_symbol"); v != "" {
		filter.TokenSymbol = &v
	}
	if v := r.URL.Query().Get("adi_url"); v != "" {
		filter.AdiURL = &v
	}
	if v := r.URL.Query().Get("status"); v != "" {
		filter.Status = &v
	}
	if v := r.URL.Query().Get("governance_level"); v != "" {
		filter.GovernanceLevel = &v
	}

	// Parse date filters
	if v := r.URL.Query().Get("created_after"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.CreatedAfter = &t
		}
	}
	if v := r.URL.Query().Get("created_before"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.CreatedBefore = &t
		}
	}

	// Parse sort options
	filter.SortBy = r.URL.Query().Get("sort_by")
	filter.SortOrder = r.URL.Query().Get("sort_order")

	ctx := r.Context()
	result, err := h.repos.ProofArtifacts.SearchAuditTrail(ctx, filter)
	if err != nil {
		h.logger.Printf("Error searching audit trail: %v", err)
		h.writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to search audit trail")
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// ============================================================================
// INTENT ROUTING HANDLER
// ============================================================================

// HandleIntentRouting routes requests to the appropriate handler based on the sub-path
func (h *TransactionCenterHandlers) HandleIntentRouting(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case strings.HasSuffix(path, "/proof"):
		h.HandleGetIntentProof(w, r)
	case strings.HasSuffix(path, "/timeline"):
		h.HandleGetIntentTimeline(w, r)
	case strings.HasSuffix(path, "/attestations"):
		h.HandleGetIntentAttestations(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "NOT_FOUND", "Endpoint not found")
	}
}

// ============================================================================
// HELPER METHODS
// ============================================================================

func (h *TransactionCenterHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Printf("Error encoding response: %v", err)
	}
}

func (h *TransactionCenterHandlers) writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func (h *TransactionCenterHandlers) parseIntParam(r *http.Request, name string, defaultValue int) int {
	if v := r.URL.Query().Get(name); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}
