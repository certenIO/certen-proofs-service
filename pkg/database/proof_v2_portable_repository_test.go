package database

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// Against a database the validators' migrations have been applied to (certen-validator 00026): the join from a proof to its
// intent's portable document, and the spine it needs. Set CERTEN_TEST_DB to run it.
func TestGetPortableV2AgainstTheValidatorsTables(t *testing.T) {
	dsn := os.Getenv("CERTEN_TEST_DB")
	if dsn == "" {
		t.Skip("CERTEN_TEST_DB is not set; the validator's tables are needed")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	r := NewProofArtifactRepository(db)

	intent := "it-" + uuid.NewString()
	proof := uuid.New()
	unbuilt := uuid.New()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	art := func(id uuid.UUID, intentID string) {
		mustExec(`INSERT INTO proof_artifacts (proof_id, proof_type, accum_tx_hash, account_url, proof_class, validator_id, artifact_json, artifact_hash, intent_id)
			VALUES ($1, 'chained', $2, 'acc://x.acme/data', 'on_demand', 'v1', '{}'::jsonb, '\x00', $3)`, id, "tx-"+id.String(), intentID)
	}
	art(proof, intent)
	art(unbuilt, "it-none-"+uuid.NewString())
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM proof_artifacts WHERE proof_id = ANY($1)`, "{"+proof.String()+","+unbuilt.String()+"}")
		_, _ = db.Exec(`DELETE FROM proof_v2_portable WHERE intent_id = $1`, intent)
		_, _ = db.Exec(`DELETE FROM proof_v2_spine_json`)
	})
	mustExec(`DELETE FROM proof_v2_spine_json`)
	mustExec(`INSERT INTO proof_v2_spine_json (major_index, record) VALUES (1, '{"index":1}'), (2, '{"index":2}'), (3, '{"index":3}')`)

	// the certificate's inputs alone are not a document
	mustExec(`INSERT INTO proof_v2_portable (intent_id, document, majors, govroot_v3_inputs) VALUES ($1, '', 1, '{"g0Hash":"aa"}')`, intent)
	if p, err := r.GetPortableV2(ctx, proof); err != nil || p != nil {
		t.Fatalf("inputs with no document: %v %v", p, err)
	}

	mustExec(`UPDATE proof_v2_portable SET document = '{"format":"f","majors":[]}', majors = 2 WHERE intent_id = $1`, intent)
	p, err := r.GetPortableV2(ctx, proof)
	if err != nil || p == nil {
		t.Fatalf("document: %v %v", p, err)
	}
	if p.Document != `{"format":"f","majors":[]}` || p.Majors != 2 || p.GovRootInputs != `{"g0Hash":"aa"}` || len(p.Spine) != 2 || p.Spine[1] != `{"index":2}` {
		t.Fatalf("%+v", p)
	}

	// a proof with no portable row, and an id with no proof, have no document
	if p, err := r.GetPortableV2(ctx, unbuilt); err != nil || p != nil {
		t.Fatalf("unbuilt: %v %v", p, err)
	}
	if p, err := r.GetPortableV2(ctx, uuid.New()); err != nil || p != nil {
		t.Fatalf("unknown: %v %v", p, err)
	}

	// a spine shorter than the document needs is an error, never a shorter document
	mustExec(`UPDATE proof_v2_portable SET majors = 5 WHERE intent_id = $1`, intent)
	if p, err := r.GetPortableV2(ctx, proof); err == nil {
		t.Fatalf("a spine of 3 for a document needing 5 gave %+v", p)
	}
	// and so is a gap
	mustExec(`UPDATE proof_v2_portable SET majors = 3 WHERE intent_id = $1`, intent)
	mustExec(`DELETE FROM proof_v2_spine_json WHERE major_index = 2`)
	if _, err := r.GetPortableV2(ctx, proof); err == nil {
		t.Fatal("a spine with a missing major block was served")
	}
}
