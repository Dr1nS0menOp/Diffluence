// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/testutil"
)

// A database restored into a fresh one has every migration recorded but has lost the
// defaults migration 0050 set, because a dump without --create does not carry them. The node
// that starts on it sets them again, so every session plans with its values (spec 065).
func TestANodeStartingOnARestoredDatabaseSetsTheSessionSettingsAgain(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	var name string
	if err := tdb.Pool.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil {
		t.Fatal(err)
	}
	ident := pgx.Identifier{name}.Sanitize()
	for _, setting := range []string{"plan_cache_mode", "jit"} {
		if _, err := tdb.Pool.Exec(ctx, "ALTER DATABASE "+ident+" RESET "+setting); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := startNode(t, upgradeConfig(t, tdb.URL)); err != nil {
		t.Fatal(err)
	}
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
			t.Errorf("%s = %q after a node started, want %q", setting, got, want)
		}
	}
}
