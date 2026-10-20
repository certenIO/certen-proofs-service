package database

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// The two SQL files below are byte-for-byte copies of certen-validator migrations 00027_proof_finality_addenda.sql and
// 00028_proof_finality_status_capture.sql at origin/pv2/rb11-b2b2a-db (dccea75). This service never creates these tables: the
// validators' migrations do, and this service only reads them. The copies exist so the repository's queries are tested against
// the real DDL, and the tests in this file fail if a copy drifts from its pinned hash or if the table's column set differs from
// the list the queries were written against. When the validator changes a column, change the copy, the hash and the list
// together, and the queries.

//go:embed testdata/validator/00027_proof_finality_addenda.sql
var validatorMigration00027 string

//go:embed testdata/validator/00028_proof_finality_status_capture.sql
var validatorMigration00028 string

const (
	migration00027SHA256 = "33111ed2735c8af9581bf4502f422935c1131589f47a9e31b8b874301cfe7d63"
	migration00028SHA256 = "759cfbda9571a7a1063a4034fcf7502ce94210800ed49eb53e4e5992ab8b81c5"
)

// pinnedColumn is a column of a table this service reads: its name, its type as the validator's DDL spells it, and the
// information_schema data_type PostgreSQL reports for it.
type pinnedColumn struct {
	Name, DDLType, DataType string
}

// The column sets the repository's queries are written against, in the DDL's order.
var pinnedFinalityColumns = map[string][]pinnedColumn{
	"proof_finality_addenda": {
		{"proof_id", "uuid", "uuid"},
		{"seq", "bigint", "bigint"},
		{"level", "smallint", "smallint"},
		{"chain", "bigint", "bigint"},
		{"kind", "character varying(64)", "character varying"},
		{"evidence", "bytea", "bytea"},
		{"prev_hash", "bytea", "bytea"},
		{"own_hash", "bytea", "bytea"},
		{"created_at", "timestamp with time zone", "timestamp with time zone"},
	},
	"proof_finality_status": {
		{"proof_id", "uuid", "uuid"},
		{"chain", "bigint", "bigint"},
		{"target_level", "smallint", "smallint"},
		{"state", "jsonb", "jsonb"},
		{"judged_at", "timestamp with time zone", "timestamp with time zone"},
	},
}

func normalizedLF(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

func TestValidatorMigrationCopiesAreUnchanged(t *testing.T) {
	for name, c := range map[string]struct{ sql, want string }{
		"00027": {validatorMigration00027, migration00027SHA256},
		"00028": {validatorMigration00028, migration00028SHA256},
	} {
		sum := sha256.Sum256([]byte(normalizedLF(c.sql)))
		if got := hex.EncodeToString(sum[:]); got != c.want {
			t.Errorf("migration %s copy has sha256 %s, pinned %s: it no longer mirrors the validator's migration", name, got, c.want)
		}
	}
}

var createTable = regexp.MustCompile(`(?s)CREATE TABLE public\.(\w+) \(\n(.*?)\n\);`)

// ddlColumns reads the column list of a CREATE TABLE out of the validator's DDL: the lines indented by exactly four spaces that
// are not table constraints.
func ddlColumns(t *testing.T, ddl, table string) []pinnedColumn {
	t.Helper()
	for _, m := range createTable.FindAllStringSubmatch(normalizedLF(ddl), -1) {
		if m[1] != table {
			continue
		}
		var cols []pinnedColumn
		for _, line := range strings.Split(m[2], "\n") {
			if !strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "     ") || strings.HasPrefix(strings.TrimSpace(line), "CONSTRAINT") {
				continue
			}
			f := strings.Fields(line)
			rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), f[0]))
			for _, stop := range []string{" NOT NULL", " DEFAULT", ","} {
				if i := strings.Index(rest, stop); i >= 0 {
					rest = rest[:i]
				}
			}
			cols = append(cols, pinnedColumn{Name: f[0], DDLType: strings.TrimSpace(rest)})
		}
		return cols
	}
	t.Fatalf("no CREATE TABLE public.%s in the migration copies", table)
	return nil
}

func TestFinalityColumnSetsMatchThePinnedLists(t *testing.T) {
	for table, source := range map[string]string{"proof_finality_addenda": validatorMigration00027, "proof_finality_status": validatorMigration00028} {
		got := ddlColumns(t, source, table)
		var want []pinnedColumn
		for _, c := range pinnedFinalityColumns[table] {
			want = append(want, pinnedColumn{Name: c.Name, DDLType: c.DDLType})
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s columns in the validator DDL differ from the list this service's queries were written against:\n got %v\nwant %v", table, got, want)
		}
	}
}
