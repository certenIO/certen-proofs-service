package database

import (
	"strings"
	"testing"
)

func TestCheckSchemaHistory(t *testing.T) {
	required := []SchemaVersion{{"00000", "aa"}, {"00001", "bb"}}
	cases := []struct {
		name    string
		applied map[string]string
		wantErr string
	}{
		{"exact", map[string]string{"00000": "aa", "00001": "bb"}, ""},
		{"newer rows from a rolling deploy", map[string]string{"00000": "aa", "00001": "bb", "00002": "cc"}, ""},
		{"older schema", map[string]string{"00000": "aa"}, "migration 00001 is not applied"},
		{"no adoption", map[string]string{}, "migration 00000 is not applied"},
		{"edited file", map[string]string{"00000": "aa", "00001": "zz"}, "has checksum zz"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSchemaHistory(tc.applied, required)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestRequiredSchemaIsOrderedAndWellFormed(t *testing.T) {
	if len(RequiredSchema) == 0 || RequiredSchema[0].Version != "00000" {
		t.Fatal("RequiredSchema must begin with the 00000 baseline")
	}
	for i, v := range RequiredSchema {
		if len(v.Version) != 5 || len(v.SHA256) != 64 || strings.ToLower(v.SHA256) != v.SHA256 {
			t.Fatalf("malformed required schema entry %+v", v)
		}
		if i > 0 && RequiredSchema[i-1].Version >= v.Version {
			t.Fatalf("RequiredSchema out of order at %s", v.Version)
		}
	}
}
