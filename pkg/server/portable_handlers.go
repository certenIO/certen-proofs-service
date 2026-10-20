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
	"encoding/binary"
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

// finalityStore is what composing the `finality` member needs of the database: the identity of the proof's addenda (no evidence),
// the size of their evidence (no evidence), and the addendum chain with its evidence, in seq order.
type finalityStore interface {
	ListFinalityIdentities(ctx context.Context, proofID uuid.UUID) ([]database.FinalityIdentity, error)
	FinalityEvidenceBytes(ctx context.Context, proofID uuid.UUID) (int64, error)
	ListFinalityAddenda(ctx context.Context, proofID uuid.UUID) ([]database.FinalityAddendum, error)
}

// MaxFinalityEvidenceBytes bounds the evidence the portable document may carry, summed over a proof's addenda. The evidence is
// served as hex, so it doubles: 6 MiB of evidence is 12 MiB of the body, which leaves 4 MiB of the api-gateway's 16 MiB cap on
// a downstream body (DOWNSTREAM_MAX_CONTENT_LENGTH) for the rest of the document and the JSON around it. A proof above the bound
// is a named 500, never a truncated member: nothing is dropped to fit. The measured evidence is smaller (Base l1_posted about
// 1 MB, Sepolia finalized 2.7 to 3.2 MB, Arbitrum finalized 0.1 to 4 MB); raising the bound is a change of this constant together
// with the gateway's cap.
const MaxFinalityEvidenceBytes int64 = 6 << 20

// finalityETag is the entity tag of a document with the given base (the document without its finality member): the base's own
// hash when the proof has no addenda, so a document without the member keeps the tag it always had, and otherwise a hash of that
// base, the proof and the identity of every addendum, so it can be computed without reading any evidence.
func finalityETag(base [32]byte, proofID uuid.UUID, ids []database.FinalityIdentity) string {
	if len(ids) == 0 {
		return `"` + hex.EncodeToString(base[:]) + `"`
	}
	h := sha256.New()
	h.Write([]byte("certen:proof-v2-etag:finality/1\n"))
	h.Write(base[:])
	h.Write(proofID[:])
	for _, id := range ids {
		var b [9]byte
		binary.BigEndian.PutUint64(b[:8], uint64(id.Seq))
		b[8] = byte(id.Level)
		h.Write(b[:])
		h.Write(id.OwnHash)
	}
	return `"` + hex.EncodeToString(h.Sum(nil)) + `"`
}

func identitiesOf(rows []database.FinalityAddendum) []database.FinalityIdentity {
	out := make([]database.FinalityIdentity, len(rows))
	for i, r := range rows {
		out[i] = database.FinalityIdentity{Seq: r.Seq, Level: r.Level, OwnHash: r.OwnHash}
	}
	return out
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
	// The document without its finality member first: it is what the tag is built on, and it is refused here when the stored document
	// carries a finality member of its own.
	base, err := ComposePortable(p)
	if err != nil {
		var named *FinalityComposeError
		if errors.As(err, &named) {
			h.logger.Printf("FINALITY_SECTION_UNUSABLE proof=%s %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "FINALITY_SECTION_UNUSABLE", err.Error())
			return
		}
		h.logger.Printf("portable proof v2 %s does not compose: %v", proofID, err)
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The stored proof v2 document is not usable")
		return
	}
	baseSum := sha256.Sum256(base)
	etag := finalityETag(baseSum, proofID, nil)
	var ids []database.FinalityIdentity
	if h.finality != nil {
		// The tag comes from what is stored about the addenda (their identities), not from the composed body, so a matching
		// If-None-Match is answered without reading any evidence.
		if ids, err = h.finality.ListFinalityIdentities(r.Context(), proofID); err != nil {
			h.logger.Printf("finality addenda of %s: %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality addenda")
			return
		}
		etag = finalityETag(baseSum, proofID, ids)
	}
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
	body := base
	if h.finality != nil && len(ids) > 0 {
		n, err := h.finality.FinalityEvidenceBytes(r.Context(), proofID)
		if err != nil {
			h.logger.Printf("finality evidence size of %s: %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality addenda")
			return
		}
		if n > MaxFinalityEvidenceBytes {
			tooLarge := finalityErr(FinalityEvidenceTooLarge, "bytes", fmt.Sprintf("%d", n), "limit", fmt.Sprintf("%d", MaxFinalityEvidenceBytes))
			h.logger.Printf("FINALITY_SECTION_UNUSABLE proof=%s %v", proofID, tooLarge)
			writeJSONError(w, http.StatusInternalServerError, "FINALITY_SECTION_UNUSABLE", tooLarge.Error())
			return
		}
		rows, err := h.finality.ListFinalityAddenda(r.Context(), proofID)
		if err != nil {
			h.logger.Printf("finality addenda of %s: %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve the finality addenda")
			return
		}
		section, err := ComposeFinalitySection(proofID, rows)
		if err != nil {
			h.logger.Printf("FINALITY_SECTION_UNUSABLE proof=%s %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "FINALITY_SECTION_UNUSABLE", err.Error())
			return
		}
		if body, err = ComposePortableWithFinality(p, section); err != nil {
			h.logger.Printf("FINALITY_SECTION_UNUSABLE proof=%s %v", proofID, err)
			writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "The stored proof v2 document is not usable")
			return
		}
		// An addendum appended between the two reads is in the body: the tag names what was served.
		w.Header().Set("ETag", finalityETag(baseSum, proofID, identitiesOf(rows)))
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
