// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// TendOptions are what the backup service migrates with (spec 070).
type TendOptions struct {
	// AppRole is the application role, which gets its rights after each migration.
	AppRole string
	// BackupMaxAge is how old the last noted backup may be before an upgrade; zero switches
	// the check off, as DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off does.
	BackupMaxAge  time.Duration
	ContractGrace time.Duration
	Log           *slog.Logger
}

// TendReport says how many migrations a round applied.
type TendReport struct {
	Expand   int
	Contract int
}

// TendSchema is what the backup service does every round as the owner of the tables, in
// place of the server (spec 070). With expand migrations pending it runs the backup part of
// the checks before an upgrade, leaves the upgrade marker and migrates; it runs the contract
// migrations once every live node knows them and ran for the grace period, as a server's
// worker did. A refused upgrade is an *UpgradeRefused, and nothing changed.
func TendSchema(ctx context.Context, owner *pgxpool.Pool, migs []db.Migration, o TendOptions) (TendReport, error) {
	var rep TendReport
	st, err := db.Status(ctx, owner, migs)
	if err != nil {
		return rep, fmt.Errorf("app: read the state of the database: %w", err)
	}
	if st.PendingExpand > 0 {
		if err := CheckBackupBeforeMigrating(ctx, owner, migs, o.BackupMaxAge, BackupRequired, o.Log); err != nil {
			return rep, err
		}
		if rep.Expand, err = migrateAsOwner(ctx, owner, migs, db.Options{Log: o.Log}, o.AppRole); err != nil {
			return rep, err
		}
	}
	pending, due, err := contractDue(ctx, owner, migs, o.ContractGrace)
	if err != nil || !due {
		return rep, err
	}
	if rep.Contract, err = migrateAsOwner(ctx, owner, migs, db.Options{IncludeContract: true, Log: o.Log}, o.AppRole); err != nil {
		return rep, err
	}
	return rep, recordContracts(ctx, owner, "backup", pending)
}

// migrateAsOwner applies migrations, sets the session defaults again, and gives the
// application role its rights, as a node that migrates does. It returns how many it applied.
func migrateAsOwner(ctx context.Context, owner *pgxpool.Pool, migs []db.Migration, opts db.Options, appRole string) (int, error) {
	ran, err := db.Migrate(ctx, owner, migs, opts)
	if err != nil {
		return 0, err
	}
	// A database restored into a fresh one lost what 0050 set, though it is recorded.
	if err := db.ApplySessionSettings(ctx, owner); err != nil {
		return 0, err
	}
	if appRole == "" {
		return len(ran.Applied), nil
	}
	return len(ran.Applied), db.GrantApp(ctx, owner, appRole)
}
