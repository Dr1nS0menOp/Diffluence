// SPDX-License-Identifier: Apache-2.0

package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

// A dump without --create does not carry the defaults of the database, so a restore into a
// fresh database loses what migration 0050 set while the migrations table says it ran.
// Applying the settings again brings both back for every new session (spec 065).
func TestTheSessionSettingsComeBackOnADatabaseThatLostThem(t *testing.T) {
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
	if err := db.ApplySessionSettings(ctx, tdb.Pool); err != nil {
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
			t.Errorf("%s = %q on a plain connection after applying the settings again, want %q", setting, got, want)
		}
	}
}

// The settings applied again are those of migration 0050, so the two cannot drift apart.
func TestTheSettingsAppliedAgainAreThoseOfTheMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/0050_session_settings.sql")
	if err != nil {
		t.Fatal(err)
	}
	var statements []string
	for _, line := range strings.Split(string(body), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			statements = append(statements, line)
		}
	}
	if got, want := strings.TrimSpace(db.SessionSettingsSQL), strings.TrimSpace(strings.Join(statements, "\n")); got != want {
		t.Errorf("SessionSettingsSQL differs from migration 0050:\n%s\nwant:\n%s", got, want)
	}
}
