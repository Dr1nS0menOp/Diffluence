// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/testutil"
)

// upgradeConfig starts a node on url the way an operator does.
func upgradeConfig(t *testing.T, databaseURL string) config.Config {
	u, _ := url.Parse("http://localhost")
	return config.Config{
		Role: config.RoleAll, HTTPAddr: "127.0.0.1:0", PublicURL: u, InstanceName: "Test",
		DatabaseURL: databaseURL, DataDir: t.TempDir(), MigrateOnStart: true,
		SessionIdleTimeout: time.Hour, SessionMaxAge: 24 * time.Hour,
	}
}

// previousRelease leaves a database as the release before this one left it: every
// migration but the last applied.
func previousRelease(t *testing.T) (testutil.TestDB, []db.Migration) {
	t.Helper()
	tdb := testutil.EmptyDatabase(t)
	migs, err := db.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Migrate(context.Background(), tdb.Pool, migs[:len(migs)-1], db.Options{}); err != nil {
		t.Fatal(err)
	}
	return tdb, migs
}

func startNode(t *testing.T, cfg config.Config) (*app.App, error) {
	t.Helper()
	a, err := app.New(context.Background(), cfg, slog.New(slog.DiscardHandler))
	if err == nil {
		t.Cleanup(a.Close)
	}
	return a, err
}

func auditCount(t *testing.T, tdb testutil.TestDB, action string) int {
	t.Helper()
	var n int
	if err := tdb.Pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_events WHERE action = $1", action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAnUpgradeWithoutTheBackupTheOperatorAskedForIsRefusedBeforeMigrating(t *testing.T) {
	tdb, migs := previousRelease(t)
	cfg := upgradeConfig(t, tdb.URL)
	cfg.UpgradeBackupMaxAge = 24 * time.Hour
	_, err := startNode(t, cfg)
	if err == nil || !strings.Contains(err.Error(), "upgrade refused before migrating: no backup was noted") {
		t.Fatalf("want a refusal that names the backup, got %v", err)
	}
	st, err := db.Status(context.Background(), tdb.Pool, migs)
	if err != nil || st.PendingExpand != 1 {
		t.Fatalf("the refused node migrated anyway: %+v %v", st, err)
	}
	if n := auditCount(t, tdb, app.ActionUpgradeStarted); n != 0 {
		t.Fatalf("a refused upgrade left %d markers", n)
	}
}

func TestAnUpgradeLeavesAMarkerNamingTheLastBackup(t *testing.T) {
	tdb, migs := previousRelease(t)
	if err := app.NoteBackup(context.Background(), tdb.Pool); err != nil {
		t.Fatal(err)
	}
	cfg := upgradeConfig(t, tdb.URL)
	cfg.UpgradeBackupMaxAge = 24 * time.Hour
	if _, err := startNode(t, cfg); err != nil {
		t.Fatal(err)
	}
	var objectID, payload string
	err := tdb.Pool.QueryRow(context.Background(), "SELECT object_id, payload FROM audit_events WHERE action = $1",
		app.ActionUpgradeStarted).Scan(&objectID, &payload)
	if err != nil {
		t.Fatal(err)
	}
	last := migs[len(migs)-1].Version
	for _, want := range []string{`"backup_noted_at":"`, `"to_migration":` + strconv.Itoa(last), `"from_migration":` + strconv.Itoa(last-1)} {
		if !strings.Contains(payload, want) {
			t.Errorf("the marker lacks %s: %s", want, payload)
		}
	}
	if objectID != fmt.Sprintf("%04d", last) {
		t.Errorf("object %q", objectID)
	}
}

// A restart that has nothing to migrate is not an upgrade: it neither waits for a backup
// nor leaves a marker.
func TestARestartIsNoUpgrade(t *testing.T) {
	tdb := testutil.Database(t)
	cfg := upgradeConfig(t, tdb.URL)
	cfg.UpgradeBackupMaxAge = time.Hour
	if _, err := startNode(t, cfg); err != nil {
		t.Fatal(err)
	}
	if n := auditCount(t, tdb, app.ActionUpgradeStarted); n != 0 {
		t.Fatalf("a restart left %d markers", n)
	}
}

func TestANewInstallNeedsNoBackup(t *testing.T) {
	tdb := testutil.EmptyDatabase(t)
	cfg := upgradeConfig(t, tdb.URL)
	cfg.UpgradeBackupMaxAge = time.Hour
	if _, err := startNode(t, cfg); err != nil {
		t.Fatal(err)
	}
	if n := auditCount(t, tdb, app.ActionUpgradeStarted); n != 0 {
		t.Fatalf("a new install left %d markers", n)
	}
}

// A node that leaves migrating to others still refuses a database a newer release has
// contracted: the columns it reads may be gone.
func TestANodeRefusesADatabaseANewerReleaseContracted(t *testing.T) {
	tdb := testutil.Database(t)
	_, err := tdb.Pool.Exec(context.Background(),
		"INSERT INTO schema_migrations (version, name, phase, checksum) VALUES (9999, 'drop_something', 'contract', 'x')")
	if err != nil {
		t.Fatal(err)
	}
	for _, migrates := range []bool{false, true} {
		cfg := upgradeConfig(t, tdb.URL)
		cfg.MigrateOnStart = migrates
		if _, err := startNode(t, cfg); !errors.Is(err, db.ErrNewerContract) || !strings.Contains(err.Error(), "9999_drop_something") {
			t.Errorf("migrates=%v: want ErrNewerContract naming the migration, got %v", migrates, err)
		}
	}
}
