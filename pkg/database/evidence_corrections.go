// Copyright 2025 Certen Protocol
//
// Corrections made to stored evidence after it was published (migration 00005, written by the validator's
// `repair anchor-blocks`). Each keeps what was stored, what replaced it and the chain reading that proves
// the replacement; the service shows them beside the record they corrected.

package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Correction record types, as evidence_corrections.record_type.
const (
	CorrectionRecordAnchorBatch = "anchor_batch"
	CorrectionRecordLayer5      = "layer5"
	CorrectionRecordCertenProof = "certen_anchor_proof"
)

// EvidenceCorrection is one recorded correction.
type EvidenceCorrection struct {
	CorrectionID  uuid.UUID       `json:"correction_id"`
	RecordType    string          `json:"record_type"`
	RecordID      string          `json:"record_id"`
	Reason        string          `json:"reason"`
	Previous      json.RawMessage `json:"previous"`
	Corrected     json.RawMessage `json:"corrected"`
	ChainEvidence json.RawMessage `json:"chain_evidence"`
	CorrectedBy   string          `json:"corrected_by"`
	CorrectedAt   string          `json:"corrected_at"`
}

// GetCorrections returns the corrections recorded for one record, oldest first.
func (r *ProofRepository) GetCorrections(ctx context.Context, recordType, recordID string) ([]EvidenceCorrection, error) {
	rows, err := r.client.QueryContext(ctx, `
		SELECT correction_id, record_type, record_id, reason, previous, corrected, chain_evidence, corrected_by,
		       to_char(corrected_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		FROM evidence_corrections
		WHERE record_type = $1 AND record_id = $2
		ORDER BY corrected_at, correction_id`, recordType, strings.ToLower(recordID))
	if err != nil {
		return nil, fmt.Errorf("read corrections for %s %s: %w", recordType, recordID, err)
	}
	defer rows.Close()
	corrections := []EvidenceCorrection{}
	for rows.Next() {
		var c EvidenceCorrection
		var prev, next, evidence []byte
		if err := rows.Scan(&c.CorrectionID, &c.RecordType, &c.RecordID, &c.Reason, &prev, &next, &evidence,
			&c.CorrectedBy, &c.CorrectedAt); err != nil {
			return nil, fmt.Errorf("scan correction: %w", err)
		}
		c.Previous, c.Corrected, c.ChainEvidence = prev, next, evidence
		corrections = append(corrections, c)
	}
	return corrections, rows.Err()
}
