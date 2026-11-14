// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

// The removal half of expand and contract, end to end on a copy of an installed database
// that holds data: every real migration, then a release that adds a column and stops using
// it, then the contract migration that drops it. No contract migration has shipped yet, so
// this is the only place the whole path runs before one does (spec 072).
func TestAContractMigrationRemovesAColumnOnACopyOfAnInstallation(t *testing.T) {
	ctx := context.Background()
	source := testutil.InstalledDatabase(t)
	shipped, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	next := len(shipped) + 1
	expand := db.Migration{Version: next, Name: "probe_note", Phase: db.PhaseExpand, Checksum: "probe-expand",
		SQL: "-- phase: expand\nALTER TABLE settings ADD COLUMN probe_note text;"}
	contract := db.Migration{Version: next + 1, Name: "drop_probe_note", Phase: db.PhaseContract, Checksum: "probe-contract",
		SQL: "-- phase: contract\nALTER TABLE settings DROP COLUMN probe_note;"}
	releaseN := append(append([]db.Migration(nil), shipped...), expand)
	releaseN1 := append(append([]db.Migration(nil), releaseN...), contract)

	if _, err := db.Migrate(ctx, source.Pool, releaseN, db.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Pool.Exec(ctx, `INSERT INTO settings (key, value, probe_note) VALUES ('kept', '"yes"', 'old')`); err != nil {
		t.Fatal(err)
	}
	copied := copyDatabase(t, source)

	rep, err := db.Migrate(ctx, copied, releaseN1, db.Options{})
	if err != nil || len(rep.Applied) != 0 || len(rep.PendingContract) != 1 {
		t.Fatalf("without permission: applied %d, pending %d, err %v", len(rep.Applied), len(rep.PendingContract), err)
	}
	rep, err = db.Migrate(ctx, copied, releaseN1, db.Options{IncludeContract: true})
	if err != nil || len(rep.Applied) != 1 || rep.Applied[0].Version != next+1 {
		t.Fatalf("the contract migration: applied %v, err %v", rep.Applied, err)
	}
	var phase string
	if err := copied.QueryRow(ctx, "SELECT phase FROM schema_migrations WHERE version = $1", next+1).Scan(&phase); err != nil || phase != "contract" {
		t.Fatalf("recorded as %q, err %v", phase, err)
	}
	var value string
	if err := copied.QueryRow(ctx, "SELECT value #>> '{}' FROM settings WHERE key = 'kept'").Scan(&value); err != nil || value != "yes" {
		t.Fatalf("the data around the column: %q, err %v", value, err)
	}
	if _, err := copied.Exec(ctx, "SELECT probe_note FROM settings"); err == nil {
		t.Fatal("the column is still there")
	}
	// Release N refuses the copy now; the source it was copied from is untouched.
	if err := db.CheckCompatible(ctx, copied, releaseN); !errors.Is(err, db.ErrNewerContract) {
		t.Fatalf("release N on the copy: want ErrNewerContract, got %v", err)
	}
	reopened, err := db.Open(ctx, source.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := db.CheckCompatible(ctx, reopened, releaseN); err != nil {
		t.Fatalf("the source changed: %v", err)
	}
	if _, err := reopened.Exec(ctx, "SELECT probe_note FROM settings"); err != nil {
		t.Fatalf("the source lost the column: %v", err)
	}
}

// copyDatabase closes the source's pool, makes a copy of it as PostgreSQL's template, and
// opens the copy, which is dropped after the test.
func copyDatabase(t *testing.T, source testutil.TestDB) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	source.Pool.Close()
	admin, err := pgx.Connect(ctx, os.Getenv(testutil.EnvDatabaseURL))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	u, err := url.Parse(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := "dfl_copy_" + hex.EncodeToString(b)
	from := pgx.Identifier{u.Path[1:]}.Sanitize()
	to := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+to+" TEMPLATE "+from); err != nil {
		t.Fatalf("copy the database: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+to+" WITH (FORCE)") })
	u.Path = "/" + name
	pool, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
