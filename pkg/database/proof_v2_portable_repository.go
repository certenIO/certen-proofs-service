// Copyright 2026 Certen Protocol
//
// The proof v2 portable document, as the validators store it (certen-validator migration 00026): the part that belongs to one
// proof, and the Directory's major-block records every proof of an incarnation shares. This service never builds or alters
// either; it composes them (RB7b-F30).

package database

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// PortableV2 is what a proof's portable document is composed from.
type PortableV2 struct {
	// Document is the portable document without its major blocks, exactly as the validator wrote it.
	Document string
	// Majors is how many major blocks from the first the document needs.
	Majors int
	// GovRootInputs is the document's govRootV3Inputs block, or "" when the intent's certificate has not recorded one.
	GovRootInputs string
	// Spine holds major blocks 1..Majors in order, each as its JSON.
	Spine []string
}

// GetPortableV2 returns what a proof's portable document is composed from, or (nil, nil) when the proof has none: it does not
// exist, or its v2 evidence has not been built (or failed to build). A stored spine that is shorter than the document needs,
// or that skips a major block, is an error and never a shorter document: a verifier replays the spine from genesis, and a
// document it cannot replay would be refused with a reason that hid the real one.
func (r *ProofArtifactRepository) GetPortableV2(ctx context.Context, proofID uuid.UUID) (*PortableV2, error) {
	var out PortableV2
	var inputs sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT p.document, p.majors, p.govroot_v3_inputs
		FROM proof_artifacts pa
		JOIN proof_v2_portable p ON p.intent_id = pa.intent_id::text
		WHERE pa.proof_id = $1 AND p.document <> ''`, proofID).Scan(&out.Document, &out.Majors, &inputs)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the portable proof v2 document: %w", err)
	}
	out.GovRootInputs = inputs.String

	rows, err := r.db.QueryContext(ctx, `SELECT major_index, record FROM proof_v2_spine_json WHERE major_index <= $1 ORDER BY major_index`, int64(out.Majors))
	if err != nil {
		return nil, fmt.Errorf("read the portable spine: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var idx int64
		var rec string
		if err := rows.Scan(&idx, &rec); err != nil {
			return nil, err
		}
		if idx != int64(len(out.Spine))+1 {
			return nil, fmt.Errorf("the stored portable spine skips from major block %d to %d", len(out.Spine), idx)
		}
		out.Spine = append(out.Spine, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out.Spine) != out.Majors {
		return nil, fmt.Errorf("the document needs %d major blocks and %d are stored", out.Majors, len(out.Spine))
	}
	return &out, nil
}
