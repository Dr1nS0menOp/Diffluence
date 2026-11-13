// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
	// ActionUpgradeWithoutBackup records an upgrade that went on without a recent backup:
	// the first one under the backup rule, or one an operator asked for (ADR-042).
	ActionUpgradeWithoutBackup = "system.upgrade_without_backup"
)

// The reasons recorded with ActionUpgradeWithoutBackup.
const (
	waiverFirstUpgrade = "first_upgrade_under_the_backup_rule"
	waiverOperator     = "operator_override"
)

// ProblemKind names what stops an upgrade, without its details, for the waiting page.
type ProblemKind string

// The kinds of problem the checks before an upgrade find.
const (
	ProblemBackup    ProblemKind = "backup"
	ProblemDisk      ProblemKind = "disk"
	ProblemQueue     ProblemKind = "queue"
	ProblemExtension ProblemKind = "extension"
	// ProblemMigrations is a node that does not migrate, on a database another process has
	// not migrated yet: in Compose, the backup service (spec 070).
	ProblemMigrations ProblemKind = "migrations"
)

// retryWhenMigrated is how soon a node that waits only for another process to migrate looks
// again: that takes seconds, and a new installation should not stand still for a minute.
const retryWhenMigrated = 5 * time.Second

// UpgradeProblem is one reason an upgrade may not start.
type UpgradeProblem struct {
	Kind ProblemKind
	Text string
}

// UpgradeRefused is the error of a node that may not migrate yet. The node waits and
// tries again instead of exiting (spec 054).
type UpgradeRefused struct {
	Problems []UpgradeProblem
}

func (e *UpgradeRefused) Error() string {
	texts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		texts[i] = p.Text
	}
	return "upgrade refused before migrating: " + strings.Join(texts, "; ")
}

// Kinds lists the kinds of the problems, each once, in the order found.
func (e *UpgradeRefused) Kinds() []ProblemKind {
	var kinds []ProblemKind
	for _, p := range e.Problems {
		if !slices.Contains(kinds, p.Kind) {
			kinds = append(kinds, p.Kind)
		}
	}
	return kinds
}

// RetryAfter is how long the node waits before it looks again.
func (e *UpgradeRefused) RetryAfter() time.Duration {
	if kinds := e.Kinds(); len(kinds) == 1 && kinds[0] == ProblemMigrations {
		return retryWhenMigrated
	}
	return RetryUpgradeEvery
}

// BackupOverride says whether an operator chose to upgrade once without a recent backup.
type BackupOverride int

const (
	// BackupRequired applies the backup rule of ADR-040 and ADR-042.
	BackupRequired BackupOverride = iota
	// WithoutBackupOnce upgrades this once without a backup and records that it did.
	WithoutBackupOnce
)

// prepareSchema makes sure this release may use the database. It refuses a database a
// newer release already contracted, and migrates when this node is configured to; a node
// that does not migrate waits until another process did (spec 070).
func (a *App) prepareSchema(ctx context.Context) error {
	if !a.cfg.MigrateOnStart {
		if err := db.CheckCompatible(ctx, a.pool, a.migrations); err != nil {
			return err
		}
		return a.waitForMigrations(ctx)
	}
	return a.asOwner(ctx, func(owner *pgxpool.Pool) error {
		if err := a.checkBeforeUpgrade(ctx, owner); err != nil {
			return err
		}
		if _, err := db.Migrate(ctx, owner, a.migrations, db.Options{Log: a.log}); err != nil {
			return err
		}
		// A database restored into a fresh one lost what 0050 set, though it is recorded.
		return db.ApplySessionSettings(ctx, owner)
	})
}

// waitForMigrations refuses a database whose expand migrations of this release have not run,
// for a node that does not run them itself. It names the backup too when the process that
// migrates would refuse for want of one, as a node that migrates would.
func (a *App) waitForMigrations(ctx context.Context) error {
	st, err := db.Status(ctx, a.pool, a.migrations)
	if err != nil {
		return fmt.Errorf("app: read the state of the database: %w", err)
	}
	if st.PendingExpand == 0 {
		return nil
	}
	var problems []UpgradeProblem
	if st.Newest > 0 {
		var facts upgradeFacts
		if err := readBackupFacts(ctx, a.pool, &facts); err != nil {
			return err
		}
		if p, refused := backupProblem(facts, time.Now(), a.cfg.UpgradeBackupMaxAge); refused {
			problems = append(problems, p)
		}
	}
	problems = append(problems, UpgradeProblem{ProblemMigrations, fmt.Sprintf(
		"%d migration(s) of this release have not run, and this node does not migrate (DIFFLUENCE_MIGRATE_ON_START=false): "+
			"in Compose the backup service runs them (docker compose up -d --build backup), elsewhere diffluence migrate",
		st.PendingExpand)})
	return &UpgradeRefused{Problems: problems}
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
	// firstWithoutBackup is true when no backup was ever noted and no earlier upgrade
	// went on without one: the one upgrade the backup rule waives (ADR-042).
	firstWithoutBackup bool
	trigrams           extensionState
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

// backupWayOut tells an operator who has no recent backup how to go on (ADR-040, ADR-042).
const backupWayOut = "run scripts/backup.sh or the backup service first (docker compose run --rm backup once), " +
	"or upgrade once without one with diffluence migrate --without-backup, " +
	"or set DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off to upgrade without one"

// upgradeProblems lists the reasons an upgrade may not start. None = it may.
// backupMaxAge zero means the operator switched the backup check off.
func upgradeProblems(f upgradeFacts, now time.Time, backupMaxAge time.Duration) []UpgradeProblem {
	var problems []UpgradeProblem
	if f.freeKnown && f.freeBytes < minFreeForUpgrade {
		problems = append(problems, UpgradeProblem{ProblemDisk,
			fmt.Sprintf("the data folder has %d MiB free and needs %d MiB", f.freeBytes>>20, minFreeForUpgrade>>20)})
	}
	if !f.queueHealthy {
		problems = append(problems, UpgradeProblem{ProblemQueue, "the queue (NATS) does not answer"})
	}
	switch f.trigrams {
	case extensionMissing:
		problems = append(problems, UpgradeProblem{ProblemExtension, "the PostgreSQL extension pg_trgm is not available: install the PostgreSQL contrib modules, or ask the provider of a managed database to allow pg_trgm"})
	case extensionNotAllowed:
		problems = append(problems, UpgradeProblem{ProblemExtension, "the role that migrates may not create the extension pg_trgm: give it CREATE on the database, or create the extension once as a superuser"})
	}
	if p, refused := backupProblem(f, now, backupMaxAge); refused {
		problems = append(problems, p)
	}
	return problems
}

// backupProblem is the backup part of the checks: whether the last noted backup is too
// old for an upgrade. The first upgrade of an installation that never noted one is let
// through once (ADR-042).
func backupProblem(f upgradeFacts, now time.Time, maxAge time.Duration) (UpgradeProblem, bool) {
	switch {
	case maxAge == 0:
		return UpgradeProblem{}, false
	case f.lastBackup.IsZero() && f.firstWithoutBackup:
		return UpgradeProblem{}, false
	case f.lastBackup.IsZero():
		return UpgradeProblem{ProblemBackup, "no backup was noted: " + backupWayOut}, true
	case now.Sub(f.lastBackup) > maxAge:
		return UpgradeProblem{ProblemBackup, fmt.Sprintf("the last backup is from %s, longer ago than %s: %s",
			f.lastBackup.UTC().Format(time.RFC3339), maxAge, backupWayOut)}, true
	}
	return UpgradeProblem{}, false
}

// waivesFirstUpgrade is whether these facts let the upgrade through only because it is the
// first under the backup rule, so the waiver must be recorded.
func waivesFirstUpgrade(f upgradeFacts, maxAge time.Duration) bool {
	return maxAge > 0 && f.lastBackup.IsZero() && f.firstWithoutBackup
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
	if a.cfg.MigrateDatabaseURL != "" {
		a.log.Info("this node migrates as the owner of the tables; the compose.yaml of spec 070 leaves that to the backup " +
			"service, so the node holds only the application role (docs/operations/upgrade.md)")
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
	} else if err := readBackupFacts(ctx, owner, &facts); err != nil {
		return err
	}
	if problems := upgradeProblems(facts, time.Now(), maxAge); len(problems) > 0 {
		return &UpgradeRefused{Problems: problems}
	}
	if isNewInstall {
		return nil
	}
	if maxAge == 0 {
		a.log.Warn("upgrading with the backup check switched off (DIFFLUENCE_UPGRADE_BACKUP_MAX_AGE=off)",
			"last_backup_noted", !facts.lastBackup.IsZero())
	}
	if waivesFirstUpgrade(facts, maxAge) {
		if err := recordWaiver(ctx, owner, a.log, "node", waiverFirstUpgrade); err != nil {
			return err
		}
	}
	return markUpgrade(ctx, owner, st, facts.lastBackup, a.maxMigration())
}

// CheckBackupBeforeMigrating is the backup part of the checks before an upgrade, for
// diffluence migrate. It used to check nothing, which made it a silent way around the
// backup rule (spec 054). The other checks need the node's queue and data folder, so they
// stay with the node. On a database that holds data it leaves the upgrade marker.
func CheckBackupBeforeMigrating(ctx context.Context, owner *pgxpool.Pool, migs []db.Migration, maxAge time.Duration,
	override BackupOverride, log *slog.Logger) error {
	st, err := db.Status(ctx, owner, migs)
	if err != nil {
		return fmt.Errorf("app: read the state of the database: %w", err)
	}
	if st.PendingExpand == 0 || st.Newest == 0 {
		return nil
	}
	var facts upgradeFacts
	if err := readBackupFacts(ctx, owner, &facts); err != nil {
		return err
	}
	switch {
	case override == WithoutBackupOnce:
		if err := recordWaiver(ctx, owner, log, "cli", waiverOperator); err != nil {
			return err
		}
	case waivesFirstUpgrade(facts, maxAge):
		if err := recordWaiver(ctx, owner, log, "cli", waiverFirstUpgrade); err != nil {
			return err
		}
	default:
		if p, refused := backupProblem(facts, time.Now(), maxAge); refused {
			return &UpgradeRefused{Problems: []UpgradeProblem{p}}
		}
	}
	return markUpgrade(ctx, owner, st, facts.lastBackup, migs[len(migs)-1].Version)
}

// readBackupFacts reads when the last backup was noted, and whether an upgrade already went
// on without one.
func readBackupFacts(ctx context.Context, q db.Querier, f *upgradeFacts) error {
	var err error
	if f.lastBackup, err = lastBackup(ctx, q); err != nil {
		return err
	}
	var waived bool
	err = q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM audit_events WHERE action = $1)", ActionUpgradeWithoutBackup).Scan(&waived)
	if err != nil {
		return fmt.Errorf("app: read earlier upgrades without a backup: %w", err)
	}
	f.firstWithoutBackup = f.lastBackup.IsZero() && !waived
	return nil
}

// recordWaiver writes that an upgrade goes on without a recent backup, and says so loudly.
func recordWaiver(ctx context.Context, owner *pgxpool.Pool, log *slog.Logger, actor, reason string) error {
	log.Warn("UPGRADING WITHOUT A RECENT BACKUP: make backups before the next upgrade, which will refuse without one",
		"reason", reason)
	// Straight into the chain, as markUpgrade: this runs before migrating, and the next
	// upgrade must see the waiver at once (ADR-041).
	return audit.RecordSealed(ctx, owner, audit.Event{ActorType: audit.ActorSystem, ActorID: actor,
		Action: ActionUpgradeWithoutBackup, ObjectType: "backup", Result: audit.ResultSuccess,
		Details: map[string]any{"reason": reason}})
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

// markUpgrade records where the database stood before it is migrated to toMigration, and
// which backup holds that state.
func markUpgrade(ctx context.Context, owner *pgxpool.Pool, st db.SchemaStatus, backupAt time.Time, toMigration int) error {
	details := map[string]any{"from_migration": st.Newest, "to_migration": toMigration, "version": version.Version}
	if !backupAt.IsZero() {
		details["backup_noted_at"] = backupAt.UTC().Format(time.RFC3339)
	}
	// Straight into the chain: this runs before migrating, when the database may be of a
	// release without audit_pending (ADR-041).
	return audit.RecordSealed(ctx, owner, audit.Event{ActorType: audit.ActorSystem, ActorID: "node",
		Action: ActionUpgradeStarted, ObjectType: "migration", ObjectID: fmt.Sprintf("%04d", toMigration),
		Result: audit.ResultSuccess, Details: details})
}

// NoteBackup records that a complete backup was just made, for the checks before the
// next upgrade. The note goes straight into the chain, so the check before migrating reads
// it at once, and a newer binary can note a backup of a database it has not migrated yet.
func NoteBackup(ctx context.Context, pool *pgxpool.Pool) error {
	return audit.RecordSealed(ctx, pool, audit.Event{ActorType: audit.ActorSystem, ActorID: "cli",
		Action: ActionBackupNoted, ObjectType: "backup", Result: audit.ResultSuccess})
}
