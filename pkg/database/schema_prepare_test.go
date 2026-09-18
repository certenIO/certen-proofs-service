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

// serviceSQLStatements collects every complete SQL statement in the service's non-test Go files: a string
// literal, or a concatenation of string literals and package-level string constants (a shared column
// list) folded into the text the database receives. A concatenation with a runtime operand cannot be
// folded; its literal fragments are still checked on their own.
func serviceSQLStatements(root string) ([]serviceSQL, error) {
	seen := make(map[string]serviceSQL)
	fileSet := token.NewFileSet()
	packages := map[string][]*ast.File{}
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
		packages[filepath.Dir(path)] = append(packages[filepath.Dir(path)], file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	record := func(value string, pos token.Pos) {
		if _, exists := seen[value]; exists {
			return
		}
		position := fileSet.Position(pos)
		source, _ := filepath.Rel(root, position.Filename)
		seen[value] = serviceSQL{Source: fmt.Sprintf("%s:%d", filepath.ToSlash(source), position.Line), SQL: value}
	}
	for _, files := range packages {
		constants := stringConstants(files)
		for _, file := range files {
			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.BinaryExpr:
					if n.Op != token.ADD {
						return true
					}
					folded, ok := foldString(n, constants)
					if !ok {
						return true
					}
					if looksLikeSQL(folded) {
						record(folded, n.Pos())
					}
					return false
				case *ast.BasicLit:
					if n.Kind != token.STRING {
						return true
					}
					if value, err := strconv.Unquote(n.Value); err == nil && looksLikeSQL(value) {
						record(value, n.Pos())
					}
				}
				return true
			})
		}
	}
	result := make([]serviceSQL, 0, len(seen))
	for _, statement := range seen {
		result = append(result, statement)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Source < result[j].Source })
	return result, nil
}

// stringConstants returns the package-level string constants of one package, folding constants defined
// in terms of other constants.
func stringConstants(files []*ast.File) map[string]string {
	pending := map[string]ast.Expr{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				for i, name := range value.Names {
					if i < len(value.Values) {
						pending[name.Name] = value.Values[i]
					}
				}
			}
		}
	}
	constants := map[string]string{}
	for progress := true; progress; {
		progress = false
		for name, expr := range pending {
			if folded, ok := foldString(expr, constants); ok {
				constants[name] = folded
				delete(pending, name)
				progress = true
			}
		}
	}
	return constants
}

// foldString evaluates a concatenation of string literals and known string constants.
func foldString(expr ast.Expr, constants map[string]string) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(e.Value)
		return value, err == nil
	case *ast.Ident:
		value, ok := constants[e.Name]
		return value, ok
	case *ast.ParenExpr:
		return foldString(e.X, constants)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := foldString(e.X, constants)
		if !ok {
			return "", false
		}
		right, ok := foldString(e.Y, constants)
		return left + right, ok
	}
	return "", false
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
