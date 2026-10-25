// Copyright 2026 Certen Protocol

package server

import (
	"context"
	"encoding/hex"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// legHeader is the settlement block header the `finality` member of a proof's document may carry (R12-OUTCOME-TX), or nil. The
// member is optional and additive: a store that cannot serve it, a read that fails, a leg that has no stored header and a header that
// does not hash to the leg's block hash all give nil and a log line, and the document is composed exactly as it was before. It never
// fails a request.
func (h *PortableHandlers) legHeader(ctx context.Context, proofID uuid.UUID, rows []database.FinalityAddendum) []byte {
	lh, ok := h.finality.(legHeaderStore)
	if !ok {
		return nil
	}
	leg, err := finalityLegOf(proofID, rows)
	if err != nil {
		return nil // the composition names the refusal
	}
	raw, err := hex.DecodeString(leg.BlockHash)
	if err != nil || len(raw) != 32 {
		return nil
	}
	var hash [32]byte
	copy(hash[:], raw)
	header, err := lh.LegHeaderRLP(ctx, proofID, leg.Chain, leg.BlockNumber, hash)
	if err != nil {
		h.logger.Printf("FINALITY_LEG_HEADER_UNREAD proof=%s %v", proofID, err)
		return nil
	}
	if len(header) == 0 {
		return nil
	}
	if _, err := withLegHeader(leg, header); err != nil {
		h.logger.Printf("FINALITY_LEG_HEADER_IGNORED proof=%s %v", proofID, err)
		return nil
	}
	return header
}
