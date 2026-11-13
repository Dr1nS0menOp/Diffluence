// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"
)

// The queries of a sealed column are written out by hand, so each must name its own table,
// column and id, exactly as the rotation reads and writes them.
func TestEachSealedColumnHasItsOwnQueries(t *testing.T) {
	flat := func(sql string) string { return strings.Join(strings.Fields(sql), " ") }
	for _, c := range sealedColumns {
		read := "SELECT " + c.id + "::text, " + c.column + " FROM " + c.table + " WHERE " + c.column +
			" IS NOT NULL AND " + c.id + "::text > $1 ORDER BY " + c.id + "::text LIMIT $2"
		if got := flat(c.readPage); got != read {
			t.Errorf("%s.%s reads with %q, want %q", c.table, c.column, got, read)
		}
		reseal := "UPDATE " + c.table + " SET " + c.column + " = $2 WHERE " + c.id + "::text = $1 AND " + c.column + " = $3"
		if got := flat(c.reseal); got != reseal {
			t.Errorf("%s.%s seals again with %q, want %q", c.table, c.column, got, reseal)
		}
	}
	for column, query := range cursorUpdates {
		if query != "UPDATE storage_state SET "+column+" = $1" {
			t.Errorf("the cursor %s is saved with %q", column, query)
		}
	}
}
