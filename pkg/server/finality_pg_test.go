//go:build pgtest

package server

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/certen/proofs-service/pkg/database"
)

// End to end over the real repository and the validators' real DDL (the migration copies under pkg/database/testdata/validator,
// pointed at a private schema of CERTEN_PG_SCRATCH_DB): GET /v2 serves the finality member the stored rows compose to, and GET
// /finality summarises the same rows. Opt-in by build tag; fails, not skips, when the database is not named.
func TestFinalityEndToEndOverTheValidatorsTables(t *testing.T) {
	dsn := os.Getenv("CERTEN_PG_SCRATCH_DB")
	if dsn == "" {
		t.Fatal("CERTEN_PG_SCRATCH_DB must name a scratch PostgreSQL database for the pgtest build tag")
	}
	schema := "e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
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
	db.SetMaxOpenConns(1)
	ddl := []string{
		`CREATE TABLE proof_artifacts (proof_id uuid PRIMARY KEY, intent_id character varying(256))`,
		`CREATE TABLE proof_v2_portable (intent_id character varying(128) PRIMARY KEY, document text NOT NULL,
			majors integer NOT NULL CONSTRAINT proof_v2_portable_majors_is_positive CHECK (majors > 0), govroot_v3_inputs text,
			built_at timestamp with time zone DEFAULT now() NOT NULL, updated_at timestamp with time zone DEFAULT now() NOT NULL)`,
		`CREATE TABLE proof_v2_spine_json (major_index bigint PRIMARY KEY CONSTRAINT proof_v2_spine_json_major_is_positive CHECK (major_index > 0),
			record text NOT NULL, recorded_at timestamp with time zone DEFAULT now() NOT NULL)`,
	}
	for _, name := range []string{"00027_proof_finality_addenda.sql", "00028_proof_finality_status_capture.sql"} {
		b, err := os.ReadFile(filepath.Join("..", "database", "testdata", "validator", name))
		if err != nil {
			t.Fatal(err)
		}
		ddl = append(ddl, strings.ReplaceAll(string(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))), "public.", schema+"."))
	}
	for _, q := range ddl {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%.80s: %v", q, err)
		}
	}
	repo := database.NewProofArtifactRepository(db)

	// the validator's vector chain, stored as rows: l1_posted then finalized on the Base leg
	vectors := loadVectors(t)
	v := vectors["valid_base_l1_posted_then_finalized"]
	proof := vectorProof(t)
	intent := "it-" + uuid.NewString()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO proof_artifacts (proof_id, intent_id) VALUES ($1, $2)`, proof, intent)
	exec(`INSERT INTO proof_v2_spine_json (major_index, record) VALUES (1, '{"index":1}'), (2, '{"index":2}')`)
	exec(`INSERT INTO proof_v2_portable (intent_id, document, majors, govroot_v3_inputs) VALUES ($1, $2, 2, '{"g0Hash":"00"}')`, intent, stored().Document)

	handler := NewPortableHandlers(repo, nil).WithFinality(repo)
	// no addenda yet: the document is what it was, with no finality member
	rr := get(handler, proof.String(), nil)
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "finality") {
		t.Fatalf("no addenda: %d %s", rr.Code, rr.Body)
	}
	for _, a := range rowsOf(t, v) {
		exec(`INSERT INTO proof_finality_addenda (proof_id, seq, level, chain, kind, evidence, prev_hash, own_hash) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			a.ProofID, a.Seq, a.Level, a.Chain, a.Kind, a.Evidence, a.PrevHash, a.OwnHash)
	}
	rr = get(handler, proof.String(), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("%d %s", rr.Code, rr.Body)
	}
	if want := string(readTestdata(t, "section_valid_base_l1_posted_then_finalized.golden.json")); !strings.Contains(rr.Body.String(), `"finality":`+want) {
		t.Fatalf("the served document lacks the golden member:\n%s", rr.Body)
	}

	// the summary over the same rows, with the builder's status for nothing else to report
	fin := NewFinalityHandlers(repo, nil)
	rec := httptest.NewRecorder()
	fin.HandleGetProofFinality(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proofs/"+proof.String()+"/finality", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"recordedLevel":"finalized"`) ||
		!strings.Contains(rec.Body.String(), `"l1_posted":{"state":"present"`) || !strings.Contains(rec.Body.String(), `"leg":{"chain":84532,"blockNumber":47368146,"blockHash":"3333`) {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	fin.HandleGetProofFinality(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proofs/"+uuid.NewString()+"/finality", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown proof: %d %s", rec.Code, rec.Body)
	}
}
