// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

// PgBouncer in transaction mode drops the settings a client sends at connect, so the
// database itself holds them, and every new session gets them however it connects (spec 063).
func TestANewSessionPlansWithItsValuesWithoutAskingForIt(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	// A plain connection sends no settings at connect, as one through PgBouncer arrives.
	conn, err := pgx.Connect(ctx, tdb.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for setting, want := range map[string]string{"plan_cache_mode": "force_custom_plan", "jit": "off"} {
		var got string
		if err := conn.QueryRow(ctx, "SHOW "+setting).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s = %q on a plain connection, want %q", setting, got, want)
		}
	}
}

// A role that does not own the database cannot set its defaults. The migration then only
// notes that, because the pool still sends both settings on a direct connection: an
// install on a managed database must not be stopped by it.
func TestTheSessionSettingsAskNothingTheOwnerMayNotDo(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	body, err := os.ReadFile("migrations/0050_session_settings.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Roles belong to the whole server, so the name is new each run: runs side by side
	// must not meet.
	suffix := make([]byte, 6)
	_, _ = rand.Read(suffix)
	ident := pgx.Identifier{"dfl_not_owner_" + hex.EncodeToString(suffix)}.Sanitize()
	if _, err := tdb.Pool.Exec(ctx, "CREATE ROLE "+ident); err != nil {
		t.Skipf("the test role cannot create roles: %v", err)
	}
	t.Cleanup(func() { _, _ = tdb.Pool.Exec(context.Background(), "DROP ROLE IF EXISTS "+ident) })
	err = pgx.BeginFunc(ctx, tdb.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+ident); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, string(body))
		return err
	})
	if err != nil {
		t.Errorf("the migration failed for a role that does not own the database: %v", err)
	}
}

// The pool keeps sending both settings, so a direct connection has them even where the
// database could not take them.
func TestThePoolStillAsksForBothSettings(t *testing.T) {
	tdb := testutil.Database(t)
	pool, err := db.Open(context.Background(), tdb.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	params := pool.Config().ConnConfig.RuntimeParams
	if params["plan_cache_mode"] != "force_custom_plan" || params["jit"] != "off" {
		t.Errorf("startup parameters = %v, want both settings", params)
	}
}
