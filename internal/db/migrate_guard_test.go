// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

// OPS-074: a migration that holds its lock longer than planned is stopped and rolled back,
// instead of holding the site.
func TestAMigrationThatRunsTooLongIsStoppedAndRolledBack(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	m := []db.Migration{{Version: 1, Name: "slow", Phase: db.PhaseExpand, Checksum: "x",
		SQL: "CREATE TABLE slow_left (id int); SELECT pg_sleep(3);"}}
	_, err := db.Migrate(ctx, tdb.Pool, m, db.Options{StatementTimeout: 300 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "0001_slow") || !strings.Contains(err.Error(), "statement timeout") {
		t.Fatalf("want a stop naming the migration and the timeout, got %v", err)
	}
	var left, recorded bool
	if err := tdb.Pool.QueryRow(ctx, `SELECT to_regclass('slow_left') IS NOT NULL,
		EXISTS (SELECT 1 FROM schema_migrations WHERE version = 1)`).Scan(&left, &recorded); err != nil {
		t.Fatal(err)
	}
	if left || recorded {
		t.Fatalf("a stopped migration left its table (%v) or its record (%v)", left, recorded)
	}
}

// A migration that needs more time says so in its header, with the reason.
func TestTheHeaderGivesAMigrationMoreTime(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	migs, err := db.Parse(fstest.MapFS{"m/0001_slow.sql": {Data: []byte(
		"-- phase: expand\n-- statement_timeout: 10s\n-- It waits on purpose, for this test.\nSELECT pg_sleep(1);\n")}}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if migs[0].StatementTimeout != 10*time.Second {
		t.Fatalf("header read as %v", migs[0].StatementTimeout)
	}
	if _, err := db.Migrate(ctx, tdb.Pool, migs, db.Options{StatementTimeout: 300 * time.Millisecond}); err != nil {
		t.Fatalf("the header's time was not used: %v", err)
	}
}

func TestHeadersThatMakeNoSenseAreRefused(t *testing.T) {
	cases := map[string]string{
		"unknown transaction":   "-- phase: expand\n-- transaction: maybe\nSELECT 1;\n",
		"no duration":           "-- phase: expand\n-- statement_timeout: soon\nSELECT 1;\n",
		"negative duration":     "-- phase: expand\n-- statement_timeout: -1s\nSELECT 1;\n",
		"two statements alone":  "-- phase: expand\n-- transaction: none\nCREATE INDEX CONCURRENTLY a ON t (x);\nCREATE INDEX CONCURRENTLY b ON t (y);\n",
		"no statement at all":   "-- phase: expand\n-- transaction: none\n",
		"statement after a gap": "-- phase: expand\n-- transaction: none\nSELECT 1; SELECT 2\n",
	}
	for name, body := range cases {
		if _, err := db.Parse(fstest.MapFS{"m/0001_a.sql": {Data: []byte(body)}}, "m"); err == nil {
			t.Errorf("%s: expected a refusal", name)
		}
	}
}

// OPS-073: an index built concurrently cannot be built inside a transaction.
func TestAConcurrentIndexIsBuiltOutsideATransaction(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	migs, err := db.Parse(fstest.MapFS{
		"m/0001_table.sql": {Data: []byte("-- phase: expand\nCREATE TABLE t (x int); INSERT INTO t SELECT generate_series(1, 1000);\n")},
		"m/0002_index.sql": {Data: []byte("-- phase: expand\n-- transaction: none\nCREATE INDEX CONCURRENTLY IF NOT EXISTS t_x ON t (x);\n")},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if !migs[1].NoTransaction || migs[0].NoTransaction {
		t.Fatalf("headers read wrongly: %+v", migs)
	}
	rep, err := db.Migrate(ctx, tdb.Pool, migs, db.Options{})
	if err != nil || len(rep.Applied) != 2 {
		t.Fatalf("applied %d, err %v", len(rep.Applied), err)
	}
	var valid bool
	if err := tdb.Pool.QueryRow(ctx, "SELECT indisvalid FROM pg_index WHERE indexrelid = 't_x'::regclass").Scan(&valid); err != nil || !valid {
		t.Fatalf("index valid %v, err %v", valid, err)
	}
	// The session's own settings are back to what they were, for the next migration.
	var timeout string
	if err := tdb.Pool.QueryRow(ctx, "SHOW statement_timeout").Scan(&timeout); err != nil || timeout != "0" {
		t.Fatalf("statement_timeout left at %q, err %v", timeout, err)
	}
}

// A failed concurrent build leaves an invalid index behind; IF NOT EXISTS would then skip it
// quietly. The run must say which index and stop.
func TestAnIndexLeftInvalidIsNamed(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	migs, err := db.Parse(fstest.MapFS{
		"m/0001_table.sql": {Data: []byte("-- phase: expand\nCREATE TABLE t (x int); INSERT INTO t VALUES (1), (1);\n")},
		"m/0002_index.sql": {Data: []byte("-- phase: expand\n-- transaction: none\nCREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS t_x ON t (x);\n")},
	}, "m")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Migrate(ctx, tdb.Pool, migs, db.Options{}); err == nil {
		t.Fatal("a unique index over duplicates was built")
	}
	_, err = db.Migrate(ctx, tdb.Pool, migs, db.Options{})
	if err == nil || !strings.Contains(err.Error(), "t_x") || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("want a stop naming the invalid index, got %v", err)
	}
	var recorded bool
	if err := tdb.Pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 2)").Scan(&recorded); err != nil || recorded {
		t.Fatalf("recorded %v, err %v", recorded, err)
	}
}
