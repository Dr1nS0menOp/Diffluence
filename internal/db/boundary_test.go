// SPDX-License-Identifier: Apache-2.0

package db

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The module boundary of CLAUDE.md and ARC-027, as a check (spec 072): every table the
// migrations create has one owning module, and a module names another module's table in
// SQL only where boundary_list_test.go allows it, as many times as it says.

var (
	createdView = regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+([\w."]+)`)
	// SQL in this code base is written with upper-case keywords and lower-case names, so
	// prose such as "copied from pages" in a message is not read as a query.
	namedTable = regexp.MustCompile(`\b(?:FROM|JOIN|INTO|UPDATE|USING|TABLE)\s+(?:ONLY\s+)?([a-z_][a-z0-9_]*(?:\.[a-z_][a-z0-9_]*)?)\b`)
)

// boundaryExempt holds the folders whose SQL is not a module's: their code only runs in tests.
var boundaryExempt = map[string]string{
	"internal/testutil": "helpers for integration tests, never part of the binary",
}

// migratedTables names every table and view the migrations create.
func migratedTables(migs []Migration) map[string]bool {
	out := map[string]bool{}
	for _, m := range migs {
		code := lineComment.ReplaceAllString(m.SQL, "")
		for _, match := range createdTable.FindAllStringSubmatch(code, -1) {
			out[tableKey(match[1])] = true
		}
		for _, match := range createdView.FindAllStringSubmatch(code, -1) {
			out[tableKey(match[1])] = true
		}
	}
	return out
}

// tablesNamed lists the known tables that a piece of SQL names, once per mention.
func tablesNamed(sql string, known map[string]bool) []string {
	var out []string
	for _, match := range namedTable.FindAllStringSubmatch(sql, -1) {
		if name := tableKey(match[1]); known[name] {
			out = append(out, name)
		}
	}
	return out
}

// stringLiterals returns the string literals of a Go file, so comments never count.
func stringLiterals(path string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err == nil {
			out = append(out, text)
		}
		return true
	})
	return out, nil
}

// crossing is a module that names a table of another module.
type crossing struct{ module, table string }

// moduleOf is the module of a path below the repository: its first two folders.
func moduleOf(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// countCrossings counts, below root, each mention of a table in SQL of a module that does
// not own it. Tests are left out.
func countCrossings(root string, owners map[string]string) (map[crossing]int, error) {
	known := map[string]bool{}
	for table := range owners {
		known[table] = true
	}
	out := map[crossing]int{}
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			module := moduleOf(rel)
			if _, exempt := boundaryExempt[module]; exempt || module == "" {
				return nil
			}
			literals, err := stringLiterals(path)
			if err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			for _, text := range literals {
				for _, table := range tablesNamed(text, known) {
					if owners[table] != module {
						out[crossing{module, table}]++
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func TestTablesNamedFindsEveryWayAQueryNamesATable(t *testing.T) {
	known := map[string]bool{"pages": true, "spaces": true, "users": true, "ops.restores": true, "page_tree": true}
	cases := map[string][]string{
		"SELECT title FROM pages WHERE id = $1":                          {"pages"},
		"SELECT 1 FROM pages p JOIN spaces s ON s.id = p.space_id":       {"pages", "spaces"},
		"INSERT INTO users (id) VALUES ($1)":                             {"users"},
		"UPDATE ONLY pages SET title = $1":                               {"pages"},
		"DELETE FROM page_tree t USING pages p WHERE p.id = t.page_id":   {"page_tree", "pages"},
		"LOCK TABLE spaces IN SHARE MODE":                                {"spaces"},
		"SELECT * FROM ops.restores":                                     {"ops.restores"},
		"SELECT * FROM public.users":                                     {"users"},
		"SELECT EXTRACT(EPOCH FROM p.created_at) FROM pages p":           {"pages"},
		"SELECT 1 FROM pages_seen":                                       nil,
		"SELECT 1 FROM unnest($1::uuid[])":                               nil,
		"the page was copied from pages of another space":                nil,
		"INSERT INTO notes (id) SELECT id FROM users WHERE kind = 'bot'": {"users"},
	}
	for sql, want := range cases {
		if got := tablesNamed(sql, known); !slices.Equal(got, want) {
			t.Errorf("%q: got %v, want %v", sql, got, want)
		}
	}
}

func TestOnlyStringLiteralsCountAsSQL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	src := "package x\n\n// Reads SELECT 1 FROM users in a comment.\nconst q = `SELECT 1 FROM pages`\n\nvar r = \"DELETE FROM spaces\"\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := stringLiterals(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"SELECT 1 FROM pages", "DELETE FROM spaces"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := stringLiterals(filepath.Join(dir, "missing.go")); err == nil {
		t.Fatal("a file that cannot be read is no error")
	}
}

func TestAModuleIsItsFirstTwoFolders(t *testing.T) {
	cases := map[string]string{
		"internal/pages/pages.go":            "internal/pages",
		"internal/identity/leaked/leaked.go": "internal/identity",
		"cmd/diffluence/restore.go":          "cmd/diffluence",
		"main.go":                            "",
	}
	for path, want := range cases {
		if got := moduleOf(path); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}

func TestCountCrossingsCountsOnlyOtherModulesOutsideTests(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("internal/pages/pages.go", "package pages\nconst q = `SELECT 1 FROM pages JOIN users u ON true`\n")
	write("internal/pages/pages_test.go", "package pages\nconst t = `SELECT 1 FROM users`\n")
	write("internal/webhooks/w.go", "package webhooks\nconst a, b = `SELECT title FROM pages`, `UPDATE pages SET x = 1`\n")
	write("internal/testutil/u.go", "package testutil\nconst q = `DELETE FROM pages`\n")
	write("cmd/diffluence/main.go", "package main\nconst q = `SELECT 1 FROM users`\n")
	owners := map[string]string{"pages": "internal/pages", "users": "internal/identity"}
	got, err := countCrossings(root, owners)
	if err != nil {
		t.Fatal(err)
	}
	want := map[crossing]int{
		{"internal/pages", "users"}:    1,
		{"internal/webhooks", "pages"}: 2,
		{"cmd/diffluence", "users"}:    1,
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	write("internal/broken/b.go", "package broken\nconst q = `unterminated\n")
	if _, err := countCrossings(root, owners); err == nil || !strings.Contains(err.Error(), "internal/broken") {
		t.Fatalf("a file that does not parse is not named: %v", err)
	}
}

// Every table the migrations create has exactly one owner, and the list names no table that
// does not exist, so a new table cannot slip past the check below.
func TestEveryTableHasAnOwningModule(t *testing.T) {
	migs, err := Migrations()
	if err != nil {
		t.Fatal(err)
	}
	tables := migratedTables(migs)
	for table := range tables {
		if _, ok := tableOwners[table]; !ok {
			t.Errorf("table %s has no owner: add it to tableOwners in boundary_list_test.go", table)
		}
	}
	for table, owner := range tableOwners {
		if !tables[table] {
			t.Errorf("tableOwners names %s, which no migration creates", table)
		}
		if info, err := os.Stat(filepath.Join("..", "..", owner)); err != nil || !info.IsDir() {
			t.Errorf("the owner of %s, %s, is not a folder of the repository", table, owner)
		}
	}
}

// A module names another module's table only as often as the list allows. A new mention
// fails: call the owner's Go API instead, or add it to the list with the reason.
func TestModulesNameOnlyTheirOwnTablesOrListedOnes(t *testing.T) {
	got, err := countCrossings(filepath.Join("..", ".."), tableOwners)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[crossing]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range crossings {
		keys[k] = true
	}
	ordered := make([]crossing, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].module != ordered[j].module {
			return ordered[i].module < ordered[j].module
		}
		return ordered[i].table < ordered[j].table
	})
	total := 0
	for _, k := range ordered {
		have, allowed := got[k], crossings[k]
		total += have
		switch {
		case have > allowed:
			t.Errorf("%s names %s, owned by %s, %d times where %d are listed: use the owner's Go API, or list it with a reason as {%q, %q}: %d,",
				k.module, k.table, tableOwners[k.table], have, allowed, k.module, k.table, have)
		case have < allowed:
			t.Errorf("%s names %s only %d times now, %d are listed: lower the number in boundary_list_test.go", k.module, k.table, have, allowed)
		}
	}
	t.Logf("%d mentions of another module's table, in %d pairs of module and table", total, len(got))
}
