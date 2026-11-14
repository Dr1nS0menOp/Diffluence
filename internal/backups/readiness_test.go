// SPDX-License-Identifier: Apache-2.0

package backups_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/backups"
	"github.com/diffluence/diffluence/internal/testutil"
)

// A restore asks backups whether a backup is whole and whether one is still being made.
func TestReadinessSaysWhetherABackupIsWholeAndWhetherOneIsOpen(t *testing.T) {
	tdb := testutil.Database(t)
	ctx := context.Background()
	check := func(name string, want backups.Readiness) {
		t.Helper()
		got, err := backups.ReadinessToRestore(ctx, tdb.Pool, name)
		if err != nil || got != want {
			t.Fatalf("%s: got %+v, want %+v, err %v", name, got, want, err)
		}
	}
	check("never-made", backups.Readiness{})
	add := `INSERT INTO backup_runs (id, name, reason, started_at, ok) VALUES ($1, $2, 'manual', now(), $3)`
	if _, err := tdb.Pool.Exec(ctx, add, uuid.New(), "failed", false); err != nil {
		t.Fatal(err)
	}
	if _, err := tdb.Pool.Exec(ctx, add, uuid.New(), "whole", true); err != nil {
		t.Fatal(err)
	}
	check("failed", backups.Readiness{})
	check("whole", backups.Readiness{Complete: true})
	if _, err := tdb.Pool.Exec(ctx, "INSERT INTO backup_requests (id) VALUES ($1)", uuid.New()); err != nil {
		t.Fatal(err)
	}
	check("whole", backups.Readiness{Complete: true, BackupOpen: true})
}
