// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// OPS-092: the log events the operations guide lists are an interface an operator scripts
// against, so each one must still be a message the code writes.
func TestEveryListedLogEventIsStillWritten(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "operations", "monitoring.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, found := strings.Cut(string(guide), "\n## Log events\n")
	if !found {
		t.Fatal("the guide has no section Log events")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var listed []string
	inRows := false
	for _, line := range strings.Split(section, "\n") {
		// The header row names the column, not an event: rows start after the separator.
		if strings.HasPrefix(line, "|---") || strings.HasPrefix(line, "| ---") {
			inRows = true
			continue
		}
		if !inRows {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "| `"); ok {
			if msg, _, ok := strings.Cut(rest, "`"); ok {
				listed = append(listed, msg)
			}
		}
	}
	if len(listed) < 10 {
		t.Fatalf("found only %d listed events: %v", len(listed), listed)
	}
	var code strings.Builder
	for _, root := range []string{filepath.Join("..", "..", "internal"), filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, err := os.ReadFile(path)
			code.Write(b)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, msg := range listed {
		if !strings.Contains(code.String(), `"`+msg+`"`) {
			t.Errorf("the guide lists %q, which no code writes", msg)
		}
	}
}
