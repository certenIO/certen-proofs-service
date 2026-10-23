//go:build pgtest

package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/certen/proofs-service/pkg/database"
)

// The status documents of the new shapes (pinsSha256, attempt and next_attempt_at) stored in the validators' real jsonb column and
// read back by the real repository: jsonb re-renders the document (keys re-ordered, a space after every colon and comma), so this is
// the byte shape the handler really decodes. Opt-in by build tag; fails, not skips, when the database is not named.
func TestStatusRowsOfTheNewShapesOverTheValidatorsTables(t *testing.T) {
	dsn := os.Getenv("CERTEN_PG_SCRATCH_DB")
	if dsn == "" {
		t.Fatal("CERTEN_PG_SCRATCH_DB must name a scratch PostgreSQL database for the pgtest build tag")
	}
	schema := "pins_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
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
	ddl := []string{`CREATE TABLE proof_artifacts (proof_id uuid PRIMARY KEY, intent_id character varying(256))`}
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
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	repo := database.NewProofArtifactRepository(db)
	fin := NewFinalityHandlers(repo, nil)
	// serve answers the summary of a proof and fails, naming the answer, unless it is a 200 with levels
	serve := func(id uuid.UUID) (int, map[string]any) {
		t.Helper()
		rr := getFinality(fin, id.String())
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || rr.Code != http.StatusOK || body["levels"] == nil {
			t.Fatalf("%s: %d %s", id, rr.Code, rr.Body)
		}
		return rr.Code, body
	}

	// a proof with no addenda: a failed ask with the back-off fields and pins (level 3), a pending row with pins (level 2)
	failed, pending := uuid.New(), uuid.New()
	exec(`INSERT INTO proof_artifacts (proof_id, intent_id) VALUES ($1, 'it-a'), ($2, 'it-b')`, failed, pending)
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 3, $2::jsonb)`, failed, withPins(attemptDoc, pinsA))
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 2, $2::jsonb)`, pending, withPins(pendingDoc, pinsB))
	code, body := serve(failed)
	lv := body["levels"].(map[string]any)["finalized"].(map[string]any)
	fields := lv["reason"].(map[string]any)["fields"].(map[string]any)
	if code != http.StatusOK || lv["state"] != "unavailable" || lv["pinsSha256"] != pinsA || fields["attempt"] != "3" || fields["next_attempt_at"] != "2026-10-23T12:30:00Z" {
		t.Fatalf("failed ask: %d %+v", code, body)
	}
	code, body = serve(pending)
	lv = body["levels"].(map[string]any)["l1_posted"].(map[string]any)
	if code != http.StatusOK || lv["state"] != "pending" || lv["pinsSha256"] != pinsB {
		t.Fatalf("pending: %d %+v", code, body)
	}

	// a row of the old shape stays exactly what it was: no pinsSha256 member anywhere in the answer
	old := uuid.New()
	exec(`INSERT INTO proof_artifacts (proof_id, intent_id) VALUES ($1, 'it-c')`, old)
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 3, $2::jsonb)`, old, pendingDoc)
	if rr := getFinality(fin, old.String()); rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "pinsSha256") || !strings.Contains(rr.Body.String(), `"state":"pending"`) {
		t.Fatalf("old row: %d %s", rr.Code, rr.Body)
	}

	// malformed pins stored in the jsonb column are the named state, for the proof
	bad := uuid.New()
	exec(`INSERT INTO proof_artifacts (proof_id, intent_id) VALUES ($1, 'it-d')`, bad)
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 3, $2::jsonb)`, bad, withPins(pendingDoc, strings.ToUpper(pinsA)))
	if rr := getFinality(fin, bad.String()); rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "bad_pins_sha256") || !strings.Contains(rr.Body.String(), "finalized") {
		t.Fatalf("malformed pins: %d %s", rr.Code, rr.Body)
	}

	// stored levels: each level's own verified status (the addendum's seq) shows its pins, whatever the other rows say
	proof := vectorProof(t)
	exec(`INSERT INTO proof_artifacts (proof_id, intent_id) VALUES ($1, 'it-e')`, proof)
	for _, a := range rowsOf(t, loadVectors(t)["valid_base_l1_posted_then_finalized"]) {
		exec(`INSERT INTO proof_finality_addenda (proof_id, seq, level, chain, kind, evidence, prev_hash, own_hash) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			a.ProofID, a.Seq, a.Level, a.Chain, a.Kind, a.Evidence, a.PrevHash, a.OwnHash)
	}
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 2, $2::jsonb)`, proof,
		`{"format":"certen-finality-status/1","class":"verified","levelReached":"l1_posted","nextLevel":"l1_posted","seq":1,"pinsSha256":"`+pinsA+`"}`)
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 3, $2::jsonb)`, proof,
		`{"format":"certen-finality-status/1","class":"verified","levelReached":"finalized","nextLevel":"finalized","seq":1,"pinsSha256":"`+pinsB+`"}`) // seq 1 is the l1_posted addendum, not this level's
	code, body = serve(proof)
	levels := body["levels"].(map[string]any)
	l1, fz := levels["l1_posted"].(map[string]any), levels["finalized"].(map[string]any)
	if code != http.StatusOK || l1["state"] != "present" || l1["pinsSha256"] != pinsA || fz["state"] != "present" || fz["pinsSha256"] != nil {
		t.Fatalf("stored levels: %d %+v", code, body)
	}
}
