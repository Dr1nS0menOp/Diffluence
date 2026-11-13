// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// No query is put together with fmt.Sprintf: a value goes in as a parameter, and a name of
// a table or column is written out in the query, or comes from a list in code that holds
// whole queries. The exceptions say why they are safe.
func TestNoQueryIsPutTogetherWithSprintf(t *testing.T) {
	exceptions := map[string]string{
		"db/migrate.go": "SET takes no parameters, and the only value is a number of milliseconds",
	}
	built := regexp.MustCompile("Sprintf\\(\\s*[\"`]\\s*(?i:select|update|insert|delete|with|create|alter|drop|lock|set|grant|revoke|truncate|copy)\\b")
	root := filepath.Join("..")
	found := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !built.Match(src) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if _, ok := exceptions[rel]; ok {
			found++
			return nil
		}
		t.Errorf("%s puts a query together with fmt.Sprintf; write it out, or pass the value as a parameter", rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found != len(exceptions) {
		t.Fatalf("%d of %d exceptions still build a query; remove the ones that do not", found, len(exceptions))
	}
}
