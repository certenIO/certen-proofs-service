//go:build pgtest

package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// These tests run the repository's queries against the validators' real DDL (migrations 00027 and 00028, copied under
// testdata/validator) in a PRIVATE schema of the scratch database named by CERTEN_PG_SCRATCH_DB, so they never touch rows other
// tests read. The scratch database is deliberately not CERTEN_TEST_DB: TestMain requires that one to be the migrated shared
// catalog, which does not carry the migrations of the unmerged validator branch these tests mirror. They are opt-in by build tag
// (go test -tags pgtest) and fail, not skip, when the database is not named; the untagged finality_ddl_test.go pins the same
// column sets without a database.

func finalityTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CERTEN_PG_SCRATCH_DB")
	if dsn == "" {
		t.Fatal("CERTEN_PG_SCRATCH_DB must name a scratch PostgreSQL database for the pgtest build tag")
	}
	schema := "fin_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	ctx := context.Background()
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := sql.Open("postgres", dsn)
		if err == nil {
			_, _ = c.Exec("DROP SCHEMA " + schema + " CASCADE")
			c.Close()
		}
	})
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := sql.Open("postgres", dsn+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	ddl := []string{
		`CREATE TABLE proof_artifacts (proof_id uuid PRIMARY KEY, intent_id character varying(256))`,
		// the validators' migrations, with their public. qualifier pointed at the private schema
		strings.ReplaceAll(normalizedLF(validatorMigration00027), "public.", schema+"."),
		strings.ReplaceAll(normalizedLF(validatorMigration00028), "public.", schema+"."),
	}
	for _, q := range ddl {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("%.80s: %v", q, err)
		}
	}
	return db
}

func TestFinalityTablesHaveExactlyThePinnedColumns(t *testing.T) {
	db := finalityTestDB(t)
	for table, want := range pinnedFinalityColumns {
		rows, err := db.Query(`SELECT column_name, data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 ORDER BY ordinal_position`, table)
		if err != nil {
			t.Fatal(err)
		}
		var got []pinnedColumn
		for rows.Next() {
			var c pinnedColumn
			if err := rows.Scan(&c.Name, &c.DataType); err != nil {
				t.Fatal(err)
			}
			got = append(got, c)
		}
		rows.Close()
		if len(got) != len(want) {
			t.Fatalf("%s has %d columns, pinned %d: %v", table, len(got), len(want), got)
		}
		for i := range want {
			if got[i].Name != want[i].Name || got[i].DataType != want[i].DataType {
				t.Errorf("%s column %d is %v, pinned %v", table, i, got[i], want[i])
			}
		}
	}
}

func TestFinalityRepositoryAgainstTheValidatorsTables(t *testing.T) {
	db := finalityTestDB(t)
	ctx := context.Background()
	r := NewProofArtifactRepository(db)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	proof, empty, unknown := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO proof_artifacts (proof_id, intent_id) VALUES ($1, 'i1'), ($2, 'i2')`, proof, empty)

	zero := make([]byte, 32)
	h1, h2 := bytes.Repeat([]byte{0x11}, 32), bytes.Repeat([]byte{0x22}, 32)
	long := append(append([]byte("CFE1"), 0, 0, 0, 24), bytes.Repeat([]byte{0x61}, 300000)...)
	short := []byte("CFE1-short")
	// finalized first (seq 1), l1_posted second: any order
	exec(`INSERT INTO proof_finality_addenda (proof_id, seq, level, chain, kind, evidence, prev_hash, own_hash) VALUES ($1, 1, 3, 84532, 'base-sepolia/finalized/v1', $2, $3, $4)`, proof, long, zero, h1)
	exec(`INSERT INTO proof_finality_addenda (proof_id, seq, level, chain, kind, evidence, prev_hash, own_hash) VALUES ($1, 2, 2, 84532, 'base-sepolia/l1_posted/v1', $2, $3, $4)`, proof, short, h1, h2)

	rows, err := r.ListFinalityAddenda(ctx, proof)
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %v", rows, err)
	}
	if rows[0].Seq != 1 || rows[0].Level != 3 || rows[0].Chain != 84532 || rows[0].Kind != "base-sepolia/finalized/v1" || rows[0].ProofID != proof ||
		!bytes.Equal(rows[0].Evidence, long) || !bytes.Equal(rows[0].PrevHash, zero) || !bytes.Equal(rows[0].OwnHash, h1) || rows[0].CreatedAt.IsZero() {
		t.Fatalf("row 1: %+v", rows[0])
	}
	if rows[1].Seq != 2 || rows[1].Level != 2 || !bytes.Equal(rows[1].Evidence, short) || !bytes.Equal(rows[1].PrevHash, h1) || !bytes.Equal(rows[1].OwnHash, h2) {
		t.Fatalf("row 2: %+v", rows[1])
	}

	heads, err := r.ListFinalityAddendumHeads(ctx, proof)
	if err != nil || len(heads) != 2 {
		t.Fatalf("%v %v", heads, err)
	}
	if heads[0].Seq != 1 || len(heads[0].Header) != FinalityHeaderBytes || !bytes.Equal(heads[0].Header, long[:FinalityHeaderBytes]) || heads[0].EvidenceLen != int64(len(long)) {
		t.Fatalf("head 1: header %d bytes, len %d", len(heads[0].Header), heads[0].EvidenceLen)
	}
	if heads[1].Seq != 2 || !bytes.Equal(heads[1].Header, short) || heads[1].EvidenceLen != int64(len(short)) || heads[1].Kind != "base-sepolia/l1_posted/v1" || heads[1].Level != 2 {
		t.Fatalf("head 2: %+v", heads[1])
	}

	// a proof with none has none, and an unknown proof has none: an empty answer, not an error
	for _, id := range []uuid.UUID{empty, unknown} {
		if a, err := r.ListFinalityAddenda(ctx, id); err != nil || len(a) != 0 {
			t.Fatalf("%v %v", a, err)
		}
		if a, err := r.ListFinalityAddendumHeads(ctx, id); err != nil || len(a) != 0 {
			t.Fatalf("%v %v", a, err)
		}
	}

	// status rows come back ordered by target level, the jsonb as stored
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 3, '{"format":"certen-finality-status/1","class":"pending"}'::jsonb)`, proof)
	exec(`INSERT INTO proof_finality_status (proof_id, chain, target_level, state) VALUES ($1, 84532, 2, '{"format":"certen-finality-status/1","class":"verified"}'::jsonb)`, proof)
	st, err := r.ListFinalityStatus(ctx, proof)
	if err != nil || len(st) != 2 || st[0].TargetLevel != 2 || st[1].TargetLevel != 3 || st[0].Chain != 84532 || st[0].JudgedAt.IsZero() {
		t.Fatalf("%+v %v", st, err)
	}
	var doc map[string]string
	if err := json.Unmarshal(st[1].State, &doc); err != nil || doc["class"] != "pending" {
		t.Fatalf("%s %v", st[1].State, err)
	}
	if st, err := r.ListFinalityStatus(ctx, empty); err != nil || len(st) != 0 {
		t.Fatalf("%v %v", st, err)
	}

	for id, want := range map[uuid.UUID]bool{proof: true, empty: true, unknown: false} {
		if got, err := r.ProofExists(ctx, id); err != nil || got != want {
			t.Fatalf("%s: %v %v", id, got, err)
		}
	}
}

// The copied DDL is the validators' append-only table: a service bug must not be able to rewrite evidence, and the test proves the
// guard is in the DDL the queries were run against.
func TestTheValidatorsTriggerRefusesWhatThisServiceNeverDoes(t *testing.T) {
	db := finalityTestDB(t)
	proof := uuid.New()
	zero := make([]byte, 32)
	if _, err := db.Exec(`INSERT INTO proof_finality_addenda (proof_id, seq, level, chain, kind, evidence, prev_hash, own_hash) VALUES ($1, 1, 3, 84532, 'base-sepolia/finalized/v1', 'x', $2, $2)`, proof, zero); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE proof_finality_addenda SET kind = 'k'`,
		`DELETE FROM proof_finality_addenda`,
		`TRUNCATE proof_finality_addenda`,
	} {
		_, err := db.Exec(q)
		pe, ok := err.(*pq.Error)
		if err == nil || !ok || pe.Code != "23001" {
			t.Errorf("%s: %v", q, err)
		}
	}
}
