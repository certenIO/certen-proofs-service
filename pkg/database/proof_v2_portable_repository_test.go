package database

import (
	"context"
	"database/sql"
	"os"
	"strings"
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
	// A private schema, so this never touches the rows other tests in a shared database read: the DDL below is the validator's
	// migration 00026, and proof_artifacts only as far as the query reads it.
	schema := "portable_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx := context.Background()
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := sql.Open("postgres", dsn+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, ddl := range []string{
		`CREATE TABLE proof_artifacts (proof_id uuid PRIMARY KEY, intent_id character varying(256))`,
		`CREATE TABLE proof_v2_portable (intent_id character varying(128) PRIMARY KEY, document text NOT NULL,
			majors integer NOT NULL CONSTRAINT proof_v2_portable_majors_is_positive CHECK (majors > 0), govroot_v3_inputs text,
			built_at timestamp with time zone DEFAULT now() NOT NULL, updated_at timestamp with time zone DEFAULT now() NOT NULL)`,
		`CREATE TABLE proof_v2_spine_json (major_index bigint PRIMARY KEY CONSTRAINT proof_v2_spine_json_major_is_positive CHECK (major_index > 0),
			record text NOT NULL, recorded_at timestamp with time zone DEFAULT now() NOT NULL)`,
	} {
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
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
		mustExec(`INSERT INTO proof_artifacts (proof_id, intent_id) VALUES ($1, $2)`, id, intentID)
	}
	art(proof, intent)
	art(unbuilt, "it-none-"+uuid.NewString())
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
