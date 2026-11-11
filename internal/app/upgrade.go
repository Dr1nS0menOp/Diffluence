// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/version"
)

// minFreeForUpgrade is the free space the data folder needs before a node migrates:
// room for the files of the queue and for the uploads that arrive during the upgrade.
const minFreeForUpgrade = 512 << 20

// The audit actions of an upgrade.
const (
	// ActionBackupNoted is what "diffluence backup note" writes, which scripts/backup.sh
	// runs once a backup is complete.
	ActionBackupNoted = "system.backup_noted"
	// ActionUpgradeStarted marks the point before a node migrates a database that holds
	// data: the backup it names is the one a full restore goes back to.
	ActionUpgradeStarted = "system.upgrade_started"
)

// prepareSchema makes sure this release may use the database. It refuses a database a
// newer release already contracted, and migrates when this node is configured to.
func (a *App) prepareSchema(ctx context.Context) error {
	if !a.cfg.MigrateOnStart {
		return db.CheckCompatible(ctx, a.pool, a.migrations)
	}
	return a.asOwner(ctx, func(owner *pgxpool.Pool) error {
		if err := a.checkBeforeUpgrade(ctx, owner); err != nil {
			return err
		}
		_, err := db.Migrate(ctx, owner, a.migrations, db.Options{Log: a.log})
		return err
	})
}

// extensionState is whether a migration can create a PostgreSQL extension it needs.
type extensionState int

const (
	extensionReady extensionState = iota
	extensionMissing
	extensionNotAllowed
)

// trigramStateSQL reads whether pg_trgm (migration 0043) can be had. Since PostgreSQL 13 it
// is trusted: whoever may create objects in the database may create it.
const trigramStateSQL = `SELECT
	EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_trgm'),
	EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')
		OR has_database_privilege(current_database(), 'CREATE')
		OR (SELECT rolsuper FROM pg_roles WHERE rolname = current_user)`

// upgradeFacts is what the checks before a migration look at.
type upgradeFacts struct {
	freeBytes    uint64
	freeKnown    bool // false where the system cannot tell
	queueHealthy bool
	lastBackup   time.Time // zero = no backup noted
	trigrams     extensionState
}

func trigramState(ctx context.Context, q db.Querier) (extensionState, error) {
	var available, allowed bool
	if err := q.QueryRow(ctx, trigramStateSQL).Scan(&available, &allowed); err != nil {
		return extensionReady, fmt.Errorf("app: read whether pg_trgm can be created: %w", err)
	}
	switch {
	case !available:
		return extensionMissing, nil
	case !allowed:
		return extensionNotAllowed, nil
	}
	return extensionReady, nil
}

// backupWayOut tells an operator who has no recent backup how to go on (ADR-040).
const backupWayOut = "run scripts/backup.sh first, or set DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off to upgrade without one"

// upgradeProblems lists the reasons an upgrade may not start. None = it may.
// backupMaxAge zero means the operator switched the backup check off.
func upgradeProblems(f upgradeFacts, now time.Time, backupMaxAge time.Duration) []string {
	var problems []string
	if f.freeKnown && f.freeBytes < minFreeForUpgrade {
		problems = append(problems, fmt.Sprintf("the data folder has %d MiB free and needs %d MiB", f.freeBytes>>20, minFreeForUpgrade>>20))
	}
	if !f.queueHealthy {
		problems = append(problems, "the queue (NATS) does not answer")
	}
	switch f.trigrams {
	case extensionMissing:
		problems = append(problems, "the PostgreSQL extension pg_trgm is not available: install the PostgreSQL contrib modules, or ask the provider of a managed database to allow pg_trgm")
	case extensionNotAllowed:
		problems = append(problems, "the role that migrates may not create the extension pg_trgm: give it CREATE on the database, or create the extension once as a superuser")
	}
	if backupMaxAge == 0 {
		return problems
	}
	switch {
	case f.lastBackup.IsZero():
		problems = append(problems, "no backup was noted: "+backupWayOut)
	case now.Sub(f.lastBackup) > backupMaxAge:
		problems = append(problems, fmt.Sprintf("the last backup is from %s, longer ago than %s: %s",
			f.lastBackup.UTC().Format(time.RFC3339), backupMaxAge, backupWayOut))
	}
	return problems
}

// checkBeforeUpgrade runs before this node migrates (blueprint 07, rule 4). It only looks
// when there is something to migrate, so a restart without a new release never waits for
// it. On a database that holds data it leaves a marker in the audit log.
func (a *App) checkBeforeUpgrade(ctx context.Context, owner *pgxpool.Pool) error {
	st, err := db.Status(ctx, owner, a.migrations)
	if err != nil {
		return fmt.Errorf("app: read the state of the database: %w", err)
	}
	if st.PendingExpand == 0 {
		return nil
	}
	facts := upgradeFacts{queueHealthy: a.queue.Healthy()}
	if facts.freeBytes, facts.freeKnown, err = freeSpace(a.cfg.DataDir); err != nil {
		return fmt.Errorf("app: read the free space of the data folder: %w", err)
	}
	if facts.trigrams, err = trigramState(ctx, owner); err != nil {
		return err
	}
	// A new install has nothing to back up, and no audit log yet.
	isNewInstall := st.Newest == 0
	maxAge := a.cfg.UpgradeBackupMaxAge
	if isNewInstall {
		maxAge = 0
	} else if facts.lastBackup, err = lastBackup(ctx, owner); err != nil {
		return err
	}
	if problems := upgradeProblems(facts, time.Now(), maxAge); len(problems) > 0 {
		return fmt.Errorf("upgrade refused before migrating: %s", strings.Join(problems, "; "))
	}
	if isNewInstall {
		return nil
	}
	if maxAge == 0 {
		a.log.Warn("upgrading with the backup check switched off (DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off)",
			"last_backup_noted", !facts.lastBackup.IsZero())
	}
	return a.markUpgrade(ctx, owner, st, facts.lastBackup)
}

// lastBackup is when the last backup was noted, or zero when none was.
func lastBackup(ctx context.Context, q db.Querier) (time.Time, error) {
	var at time.Time
	err := q.QueryRow(ctx, "SELECT occurred_at FROM audit_events WHERE action = $1 ORDER BY seq DESC LIMIT 1", ActionBackupNoted).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("app: read the last backup: %w", err)
	}
	return at, nil
}

// markUpgrade records where the database stood before this node migrates it, and which
// backup holds that state.
func (a *App) markUpgrade(ctx context.Context, owner *pgxpool.Pool, st db.SchemaStatus, backupAt time.Time) error {
	details := map[string]any{"from_migration": st.Newest, "to_migration": a.maxMigration(), "version": version.Version}
	if !backupAt.IsZero() {
		details["backup_noted_at"] = backupAt.UTC().Format(time.RFC3339)
	}
	// Straight into the chain: this runs before migrating, when the database may be of a
	// release without audit_pending (ADR-041).
	return audit.RecordSealed(ctx, owner, audit.Event{ActorType: audit.ActorSystem, ActorID: "node",
		Action: ActionUpgradeStarted, ObjectType: "migration", ObjectID: fmt.Sprintf("%04d", a.maxMigration()),
		Result: audit.ResultSuccess, Details: details})
}

// NoteBackup records that a complete backup was just made, for the checks before the
// next upgrade. The note goes straight into the chain, so the check before migrating reads
// it at once, and a newer binary can note a backup of a database it has not migrated yet.
func NoteBackup(ctx context.Context, pool *pgxpool.Pool) error {
	return audit.RecordSealed(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "cli",
		Action: ActionBackupNoted, ObjectType: "backup", Result: audit.ResultSuccess})
}
