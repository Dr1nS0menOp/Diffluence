// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

func TestParseRules(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"bad name":     {"m/1_init.sql": {Data: []byte("-- phase: expand\n")}},
		"no header":    {"m/0001_init.sql": {Data: []byte("CREATE TABLE x();")}},
		"gap":          {"m/0001_a.sql": {Data: []byte("-- phase: expand\n")}, "m/0003_b.sql": {Data: []byte("-- phase: expand\n")}},
		"wrong header": {"m/0001_a.sql": {Data: []byte("-- phase: shrink\n")}},
	}
	for name, fsys := range cases {
		if _, err := db.Parse(fsys, "m"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	good := fstest.MapFS{
		"m/0002_b.sql": {Data: []byte("-- phase: contract\nDROP TABLE x;")},
		"m/0001_a.sql": {Data: []byte("-- phase: expand\nCREATE TABLE x();")},
	}
	migs, err := db.Parse(good, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 2 || migs[0].Version != 1 || migs[1].Phase != db.PhaseContract {
		t.Fatalf("unexpected result %+v", migs)
	}
}

func TestEmbeddedMigrationsAreValid(t *testing.T) {
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) == 0 || migs[0].Phase != db.PhaseExpand {
		t.Fatal("first migration must exist and be an expand migration")
	}
}

func TestMigrateIsIdempotentAndConcurrentSafe(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	migs, _ := db.Migrations()

	// Five nodes starting at the same moment must not trip over each other.
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := db.Migrate(ctx, tdb.Pool, migs, db.Options{})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rep, err := db.Migrate(ctx, tdb.Pool, migs, db.Options{})
	if err != nil || len(rep.Applied) != 0 {
		t.Fatalf("second run applied %d, err %v", len(rep.Applied), err)
	}
	n, err := db.PendingExpand(ctx, tdb.Pool, migs)
	if err != nil || n != 0 {
		t.Fatalf("pending = %d, err %v", n, err)
	}
}

func TestContractMigrationsWaitUntilAllowed(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	migs := []db.Migration{
		{Version: 1, Name: "add", Phase: db.PhaseExpand, SQL: "CREATE TABLE t (old text, new text)", Checksum: "a"},
		{Version: 2, Name: "drop_old", Phase: db.PhaseContract, SQL: "ALTER TABLE t DROP COLUMN old", Checksum: "b"},
	}
	rep, err := db.Migrate(ctx, tdb.Pool, migs, db.Options{})
	if err != nil || len(rep.Applied) != 1 || len(rep.PendingContract) != 1 {
		t.Fatalf("applied=%d pending=%d err=%v", len(rep.Applied), len(rep.PendingContract), err)
	}
	rep, err = db.Migrate(ctx, tdb.Pool, migs, db.Options{IncludeContract: true})
	if err != nil || len(rep.Applied) != 1 {
		t.Fatalf("contract run: applied=%d err=%v", len(rep.Applied), err)
	}
}

func TestOlderNodeAcceptsNewerDatabase(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	v1 := []db.Migration{{Version: 1, Name: "a", Phase: db.PhaseExpand, SQL: "CREATE TABLE a (id int)", Checksum: "a"}}
	v2 := append(v1, db.Migration{Version: 2, Name: "b", Phase: db.PhaseExpand, SQL: "CREATE TABLE b (id int)", Checksum: "b"})
	if _, err := db.Migrate(ctx, tdb.Pool, v2, db.Options{}); err != nil {
		t.Fatal(err)
	}
	// A node still running release N starts while N+1 already migrated: fine.
	rep, err := db.Migrate(ctx, tdb.Pool, v1, db.Options{})
	if err != nil || rep.NewerInDatabase != 1 {
		t.Fatalf("newer=%d err=%v", rep.NewerInDatabase, err)
	}
}

func TestEditedMigrationIsRefused(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	m := []db.Migration{{Version: 1, Name: "a", Phase: db.PhaseExpand, SQL: "CREATE TABLE a (id int)", Checksum: "original"}}
	if _, err := db.Migrate(ctx, tdb.Pool, m, db.Options{}); err != nil {
		t.Fatal(err)
	}
	m[0].Checksum = "edited"
	_, err := db.Migrate(ctx, tdb.Pool, m, db.Options{})
	if !errors.Is(err, db.ErrChecksumMismatch) {
		t.Fatalf("want ErrChecksumMismatch, got %v", err)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	ctx := context.Background()
	m := []db.Migration{{Version: 1, Name: "bad", Phase: db.PhaseExpand,
		SQL: "CREATE TABLE ok_table (id int); SELECT * FROM does_not_exist;", Checksum: "x"}}
	_, err := db.Migrate(ctx, tdb.Pool, m, db.Options{})
	if err == nil || !strings.Contains(err.Error(), "0001_bad") {
		t.Fatalf("want failure naming the migration, got %v", err)
	}
	var exists bool
	_ = tdb.Pool.QueryRow(ctx, "SELECT to_regclass('ok_table') IS NOT NULL").Scan(&exists)
	if exists {
		t.Fatal("partial migration was not rolled back")
	}
}
