package database

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestRepositorySQLPreparesAgainstSharedSchema is the schema/code compatibility gate. Every complete SQL
// literal in the service's non-test code is prepared against the shared schema (see TestMain), so a table
// or column the shared schema lacks fails here rather than on a production request path.
func TestRepositorySQLPreparesAgainstSharedSchema(t *testing.T) {
	if testDB == nil {
		t.Skip("Test database not configured")
	}
	ctx := context.Background()
	statements, err := serviceSQLStatements(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) == 0 {
		t.Fatal("no SQL statements discovered")
	}
	t.Logf("preparing %d distinct statements", len(statements))
	for _, statement := range statements {
		prepared, err := testDB.PrepareContext(ctx, statement.SQL)
		if err != nil {
			t.Errorf("prepare %s: %v", statement.Source, err)
			continue
		}
		prepared.Close()
	}
}

type serviceSQL struct {
	Source string
	SQL    string
}

func serviceSQLStatements(root string) ([]serviceSQL, error) {
	seen := make(map[string]serviceSQL)
	fileSet := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path == root {
				return nil
			}
			// A nested go.mod is another module (CI checks certen-validator out inside this workspace).
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "testdata" || name == "web" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || !looksLikeSQL(value) {
				return true
			}
			if _, exists := seen[value]; !exists {
				position := fileSet.Position(literal.Pos())
				source, _ := filepath.Rel(root, position.Filename)
				seen[value] = serviceSQL{Source: fmt.Sprintf("%s:%d", filepath.ToSlash(source), position.Line), SQL: value}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]serviceSQL, 0, len(seen))
	for _, statement := range seen {
		result = append(result, statement)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Source < result[j].Source })
	return result, nil
}

// looksLikeSQL accepts complete statements only. A literal containing a format verb is a template whose
// final text exists only at run time, so it cannot be prepared in isolation.
func looksLikeSQL(value string) bool {
	value = strings.TrimSpace(value)
	if strings.Contains(value, "%") {
		return false
	}
	for _, prefix := range []string{"SELECT ", "INSERT ", "UPDATE ", "DELETE ", "WITH "} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
