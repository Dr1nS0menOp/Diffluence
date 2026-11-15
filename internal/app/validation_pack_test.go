// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// DOC-790: the validation pack names every requirement line of blueprint 09.8, and every test
// it names exists, so a customer's validation never points at a test that was renamed away.
func TestTheValidationPackNamesEveryLineAndItsTests(t *testing.T) {
	root := filepath.Join("..", "..")
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	chapter := read("docs/blueprint/09-enterprise/08-electronic-signatures.md")
	pack := read("docs/operations/validation-pack.md")
	lines := regexp.MustCompile(`\*\*(DOC-\d+)\*\*`).FindAllStringSubmatch(chapter, -1)
	if len(lines) < 20 {
		t.Fatalf("only %d lines found in 09.8", len(lines))
	}
	for _, l := range lines {
		if !regexp.MustCompile(`(?m)^\| ` + l[1] + ` \|`).MatchString(pack) {
			t.Errorf("the pack does not name %s", l[1])
		}
	}

	var goTests, e2eTests strings.Builder
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		switch {
		case strings.HasSuffix(path, "_test.go"):
			b, err := os.ReadFile(path)
			goTests.Write(b)
			return err
		case strings.HasSuffix(path, ".spec.ts"):
			b, err := os.ReadFile(path)
			e2eTests.Write(b)
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	named := 0
	for _, m := range regexp.MustCompile("`(Test[A-Za-z0-9]+)`").FindAllStringSubmatch(pack, -1) {
		named++
		if !strings.Contains(goTests.String(), "func "+m[1]+"(") {
			t.Errorf("the pack names %s, which does not exist", m[1])
		}
	}
	for _, m := range regexp.MustCompile(`e2e: "([^"]+)"`).FindAllStringSubmatch(pack, -1) {
		named++
		if !strings.Contains(e2eTests.String(), "'"+m[1]+"'") {
			t.Errorf("the pack names the browser test %q, which does not exist", m[1])
		}
	}
	if named < 25 {
		t.Fatalf("only %d tests named", named)
	}
}
