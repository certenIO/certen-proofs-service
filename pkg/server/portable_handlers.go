// Copyright 2026 Certen Protocol
//
// GET /api/v1/proofs/{proof_id}/v2 serves a proof's portable proof v2 document (certen-proof-v2-accumulate-portable/1): everything
// a verifier needs to check the Accumulate side of the proof offline, from the incarnation's genesis. The validators build and
// store it; this composes it from the two stored parts, plus the staged finality addenda when there are any, and serves it. It is
// evidence, never a verdict: nothing here says verified.

package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// portableStore is what the handler needs of the database.
type portableStore interface {
	GetPortableV2(ctx context.Context, proofID uuid.UUID) (*database.PortableV2, error)
}

// finalityStore is what composing the `finality` member needs of the database: the proof's addendum chain, in seq order.
type finalityStore interface {
	ListFinalityAddenda(ctx context.Context, proofID uuid.UUID) ([]database.FinalityAddendum, error)
}

// PortableHandlers serves portable proof v2 documents.
type PortableHandlers struct {
	store    portableStore
	finality finalityStore
	logger   *log.Logger
}

// NewPortableHandlers returns the handlers over a store.
func NewPortableHandlers(store portableStore, logger *log.Logger) *PortableHandlers {
	if logger == nil {
		logger = log.New(log.Writer(), "[PortableAPI] ", log.LstdFlags)
	}
	return &PortableHandlers{store: store, logger: logger}
}

// WithFinality makes the handler compose the document's `finality` member (V2_SPEC_S7 section 2.5) from the validators' addendum
// rows at request time. Without it the member is never composed.
func (h *PortableHandlers) WithFinality(store finalityStore) *PortableHandlers {
	h.finality = store
	return h
}

// ComposePortable puts the shared major blocks, and the govRoot v3 inputs when there are some, into a stored document. The
// document keeps every other member exactly as the validator wrote it.
func ComposePortable(p *database.PortableV2) ([]byte, error) {
	return ComposePortableWithFinality(p, nil)
}

// ComposePortableWithFinality is ComposePortable plus the `finality` member, given as the JSON ComposeFinalitySection returned
// (nil: the member is absent). The member is composed at request time and is never part of the stored document: a stored
// document that carries its own `finality` key is refused, because serving it would present a claim this service did not compose
// from the addendum rows.
func ComposePortableWithFinality(p *database.PortableV2, finality json.RawMessage) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(p.Document), &doc); err != nil {
		return nil, fmt.Errorf("the stored document is not JSON: %w", err)
	}
	if _, stored := doc["finality"]; stored {
		return nil, finalityErr(FinalityMemberInDocument)
	}
	if len(p.Spine) != p.Majors {
		return nil, fmt.Errorf("the document needs %d major blocks, %d given", p.Majors, len(p.Spine))
	}
	majors := make([]json.RawMessage, len(p.Spine))
	for i, m := range p.Spine {
		if !json.Valid([]byte(m)) {
			return nil, fmt.Errorf("major block %d is not JSON", i+1)
		}
		majors[i] = json.RawMessage(m)
	}
	var err error
	if doc["majors"], err = json.Marshal(majors); err != nil {
		return nil, err
	}
	if p.GovRootInputs != "" {
		if !json.Valid([]byte(p.GovRootInputs)) {
			return nil, fmt.Errorf("the stored govRoot v3 inputs are not JSON")
		}
		doc["govRootV3Inputs"] = json.RawMessage(p.GovRootInputs)
	}
	if finality != nil {
		doc["finality"] = finality
	}
	return json.Marshal(doc)
}

// HandleGetProofV2 serves GET /api/v1/proofs/{proof_id}/v2.
//
//	200  the document
//	404  PROOF_V2_NOT_AVAILABLE: no such proof, or its v2 evidence was never built (an older proof, or a build that failed)
//	500  the stored parts cannot be read or do not compose: never reported as "not available"
func (h *PortableHandlers) HandleGetProofV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Only GET is allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/proofs/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "v2" {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PATH", "Invalid endpoint path")
		return
	}
	proofID, err := uuid.Parse(parts[0])
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_PROOF_ID", "Invalid proof ID format")
		return
	}

	p, err := h.store.GetPortableV2(r.Context(), proofID)
	if err != nil {
		h.logger.Printf("portable proof v2 %s: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the proof v2 document")
		return
	}
	if p == nil {
		writeJSONError(w, http.StatusNotFound, "PROOF_V2_NOT_AVAILABLE", fmt.Sprintf("No proof v2 document for proof %s", proofID))
		return
	}
	var section json.RawMessage
	if h.finality != nil {
		rows, err := h.finality.ListFinalityAddenda(r.Context(), proofID)
		if err != nil {
			h.logger.Printf("finality addenda of %s: %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality addenda")
			return
		}
		if section, err = ComposeFinalitySection(proofID, rows); err != nil {
			h.logger.Printf("finality addenda of %s do not compose: %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "FINALITY_SECTION_UNUSABLE", err.Error())
			return
		}
	}
	body, err := ComposePortableWithFinality(p, section)
	if err != nil {
		var named *FinalityComposeError
		if errors.As(err, &named) {
			h.logger.Printf("portable proof v2 %s does not compose: %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "FINALITY_SECTION_UNUSABLE", err.Error())
			return
		}
		h.logger.Printf("portable proof v2 %s does not compose: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The stored proof v2 document is not usable")
		return
	}

	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	// The document changes when it is rebuilt and when an addendum is appended to its `finality` member (a level is reached later
	// than the proof); either changes its ETag, so a client revalidates with it instead of holding a document for an hour that may
	// lack a level the validators have since recorded. Private because the route is authenticated.
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(body); err == nil && zw.Close() == nil {
			w.Header().Set("Content-Encoding", "gzip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(buf.Bytes())
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": map[string]string{"code": code, "message": message}})
}
