// Copyright 2026 Certen Protocol
//
// Staged finality evidence as the validators store it (certen-validator migrations 00027 and 00028, V2_SPEC_S7 sections 2.4 and
// 2.7): the append-only addendum chain of a settled leg and the builder's mutable status per target level. This service never
// writes either and never judges either: it reads them to compose the `finality` member of the portable proof v2 document and
// to summarise what the validators recorded. Evidence, never a verdict.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FinalityHeaderBytes is how many leading bytes of an addendum's evidence cover the CFE1 container header
// ("CFE1" 4 + STR kind 4+64 at most + u64 chain 8 + u8 level 1 + u64 blockNumber 8 + B32 blockHash 32 = 121): enough to read the
// leg an addendum is bound to without loading evidence that can be megabytes.
const FinalityHeaderBytes = 128

// FinalityAddendum is one row of proof_finality_addenda. Level is 2 (l1_posted) or 3 (finalized); the table never stores 1.
type FinalityAddendum struct {
	ProofID   uuid.UUID
	Seq       int64
	Level     int16
	Chain     int64
	Kind      string
	Evidence  []byte
	PrevHash  []byte
	OwnHash   []byte
	CreatedAt time.Time
}

// FinalityAddendumHead is a row of proof_finality_addenda without its evidence: the CFE1 header prefix (the first
// FinalityHeaderBytes bytes, or fewer when the evidence is shorter) and the evidence length.
type FinalityAddendumHead struct {
	ProofID     uuid.UUID
	Seq         int64
	Level       int16
	Chain       int64
	Kind        string
	Header      []byte
	EvidenceLen int64
	CreatedAt   time.Time
}

// FinalityIdentity names a stored addendum without its evidence: its place in the chain, its level and its chain hash. own_hash
// commits to the evidence (sha256 of it is one of its preimage fields), so these three identify what the row holds.
type FinalityIdentity struct {
	Seq     int64
	Level   int16
	OwnHash []byte
}

// FinalityStatus is one row of proof_finality_status. State is the stored jsonb document (certen-finality-status/1) exactly as
// the validator wrote it.
type FinalityStatus struct {
	ProofID     uuid.UUID
	Chain       int64
	TargetLevel int16
	State       []byte
	JudgedAt    time.Time
}

// ListFinalityAddenda returns a proof's addendum chain in seq order, evidence included. A proof with none has none (an empty
// slice, not an error).
func (r *ProofArtifactRepository) ListFinalityAddenda(ctx context.Context, proofID uuid.UUID) ([]FinalityAddendum, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT proof_id, seq, level, chain, kind, evidence, prev_hash, own_hash, created_at
		FROM proof_finality_addenda WHERE proof_id = $1 ORDER BY seq`, proofID)
	if err != nil {
		return nil, fmt.Errorf("read the finality addenda: %w", err)
	}
	defer rows.Close()
	var out []FinalityAddendum
	for rows.Next() {
		var a FinalityAddendum
		if err := rows.Scan(&a.ProofID, &a.Seq, &a.Level, &a.Chain, &a.Kind, &a.Evidence, &a.PrevHash, &a.OwnHash, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("read a finality addendum: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFinalityIdentities returns the identity of each of a proof's addenda in seq order, without reading any evidence.
func (r *ProofArtifactRepository) ListFinalityIdentities(ctx context.Context, proofID uuid.UUID) ([]FinalityIdentity, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT seq, level, own_hash FROM proof_finality_addenda WHERE proof_id = $1 ORDER BY seq`, proofID)
	if err != nil {
		return nil, fmt.Errorf("read the finality addendum identities: %w", err)
	}
	defer rows.Close()
	var out []FinalityIdentity
	for rows.Next() {
		var id FinalityIdentity
		if err := rows.Scan(&id.Seq, &id.Level, &id.OwnHash); err != nil {
			return nil, fmt.Errorf("read a finality addendum identity: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FinalityEvidenceBytes is the total size of the evidence of a proof's addenda, measured in the database without reading it.
func (r *ProofArtifactRepository) FinalityEvidenceBytes(ctx context.Context, proofID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(octet_length(evidence)), 0)::bigint FROM proof_finality_addenda WHERE proof_id = $1`, proofID).Scan(&n); err != nil {
		return 0, fmt.Errorf("measure the finality evidence: %w", err)
	}
	return n, nil
}

// ListFinalityAddendumHeads is ListFinalityAddenda without the evidence bodies.
func (r *ProofArtifactRepository) ListFinalityAddendumHeads(ctx context.Context, proofID uuid.UUID) ([]FinalityAddendumHead, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT proof_id, seq, level, chain, kind, substring(evidence from 1 for $2), octet_length(evidence), created_at
		FROM proof_finality_addenda WHERE proof_id = $1 ORDER BY seq`, proofID, FinalityHeaderBytes)
	if err != nil {
		return nil, fmt.Errorf("read the finality addendum headers: %w", err)
	}
	defer rows.Close()
	var out []FinalityAddendumHead
	for rows.Next() {
		var a FinalityAddendumHead
		if err := rows.Scan(&a.ProofID, &a.Seq, &a.Level, &a.Chain, &a.Kind, &a.Header, &a.EvidenceLen, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("read a finality addendum header: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFinalityStatus returns the builder's status rows of a proof, ordered by target level.
func (r *ProofArtifactRepository) ListFinalityStatus(ctx context.Context, proofID uuid.UUID) ([]FinalityStatus, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT proof_id, chain, target_level, state, judged_at
		FROM proof_finality_status WHERE proof_id = $1 ORDER BY target_level`, proofID)
	if err != nil {
		return nil, fmt.Errorf("read the finality status: %w", err)
	}
	defer rows.Close()
	var out []FinalityStatus
	for rows.Next() {
		var s FinalityStatus
		if err := rows.Scan(&s.ProofID, &s.Chain, &s.TargetLevel, &s.State, &s.JudgedAt); err != nil {
			return nil, fmt.Errorf("read a finality status row: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ProofExists reports whether a proof_artifacts row has this id.
func (r *ProofArtifactRepository) ProofExists(ctx context.Context, proofID uuid.UUID) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM proof_artifacts WHERE proof_id = $1`, proofID).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("look up the proof: %w", err)
	}
	return true, nil
}
