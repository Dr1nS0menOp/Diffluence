// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"testing"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/testutil"
)

// exportRows exports everything in pages of two events.
func exportRows(t *testing.T, tdb testutil.TestDB, maxRows int) [][]string {
	t.Helper()
	var buf bytes.Buffer
	if _, err := audit.ExportCSV(context.Background(), tdb.Pool, audit.WithExportPageForTest(audit.Filter{}, 2), maxRows, &buf); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// An export ends with a line that counts its events, so a file that stopped on the way is
// told apart from a whole one. It reads in pages, each a short query, and misses nothing
// between them.
func TestAnExportEndsWithTheCountOfItsEvents(t *testing.T) {
	tdb := testutil.Database(t)
	for _, a := range []string{"page.created", "page.updated", "page.moved", "page.deleted", "page.restored"} {
		record(t, tdb, sample(a))
	}
	if err := audit.RecordStandalone(context.Background(), tdb.Pool, sample("page.viewed")); err != nil {
		t.Fatal(err)
	}

	rows := exportRows(t, tdb, 1000)
	if len(rows) != 1+6+1 {
		t.Fatalf("%d rows: %v", len(rows), rows)
	}
	previous := int64(0)
	for i, row := range rows[1:6] {
		seq, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil || seq <= previous || row[5] == "page.viewed" {
			t.Fatalf("row %d: %v", i, row)
		}
		previous = seq
	}
	if rows[6][0] != "" || rows[6][5] != "page.viewed" {
		t.Fatalf("the waiting event comes last, without a seq: %v", rows[6])
	}
	end := rows[len(rows)-1]
	if end[0] != audit.CSVComplete || end[1] != "6" || len(end) != len(audit.CSVHeader) {
		t.Fatalf("the last line: %v", end)
	}
}

// An export cut short by its row limit says so in its last line.
func TestAnExportCutAtItsLimitSaysSo(t *testing.T) {
	tdb := testutil.Database(t)
	for _, a := range []string{"page.created", "page.updated", "page.moved"} {
		record(t, tdb, sample(a))
	}
	rows := exportRows(t, tdb, 2)
	end := rows[len(rows)-1]
	if len(rows) != 1+2+1 || end[0] != audit.CSVLimited || end[1] != strconv.Itoa(2) {
		t.Fatalf("rows: %v", rows)
	}
}
