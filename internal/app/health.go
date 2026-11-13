// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/events"
	"github.com/diffluence/diffluence/internal/health"
	"github.com/diffluence/diffluence/internal/storage"
	"github.com/diffluence/diffluence/internal/version"
)

// healthFreshFor is how long one reading of the facts is shown again (spec 051 load
// budget): the console, the band on every page and the metrics all read the same answer.
const healthFreshFor = 30 * time.Second

// How long finished work is kept before the daily cleaner removes it (spec 051, D7).
// Failed work is kept longer: a person may still need to look at it.
const (
	keepFinishedWork = 30 * 24 * time.Hour
	keepFailedWork   = 90 * 24 * time.Hour
	// keepGoneNode is how long a node that stopped without leaving stays in the registry.
	keepGoneNode = 24 * time.Hour
	// keepSizeSamples covers the 30 days the growth is measured over, with room to spare.
	keepSizeSamples = 40 * 24 * time.Hour
	// keepBackupHistory is how long the console lists what the backup worker reported.
	keepBackupHistory = 400 * 24 * time.Hour
)

// HealthSource is what reading the health facts needs, so "diffluence doctor" can read
// them without a running node.
type HealthSource struct {
	Pool           db.Querier
	Store          storage.Store
	Migrations     []db.Migration
	MailConfigured bool
	OIDCConfigured bool
	// ContractGrace is the configured wait before contract migrations run.
	ContractGrace time.Duration
}

// healthFactsSQL reads every count the checks need in one statement. Each part is cheap:
// an indexed lookup, or a table the daily cleaner keeps small.
const healthFactsSQL = `SELECT
	(SELECT count(*) FROM jobs WHERE status = 'failed' AND created_at > now() - interval '7 days'),
	(SELECT count(*) FROM outbox_events WHERE published_at IS NULL AND created_at < now() - interval '1 hour'),
	(SELECT count(*) FROM event_failures WHERE replayed_at IS NULL),
	(SELECT count(*) FROM mail_queue WHERE status = 'failed' AND created_at > now() - interval '1 day'),
	(SELECT count(*) FROM local_credentials WHERE password_locked_at IS NOT NULL),
	(SELECT occurred_at FROM audit_events WHERE action = $1 ORDER BY seq DESC LIMIT 1),
	(SELECT max(applied_at) FROM schema_migrations),
	(SELECT bytes FROM database_size_samples ORDER BY taken_at DESC LIMIT 1),
	(SELECT bytes FROM database_size_samples WHERE taken_at <= now() - make_interval(secs => $2)
		ORDER BY taken_at DESC LIMIT 1),
	(SELECT count(*) FROM audit_pending),
	(SELECT min(occurred_at) FROM audit_pending),
	COALESCE((SELECT NOT ok FROM backup_runs ORDER BY finished_at DESC LIMIT 1), false),
	COALESCE((SELECT NOT ok FROM backup_restore_checks ORDER BY checked_at DESC LIMIT 1), false),
	(SELECT CASE WHEN count(DISTINCT ip) = 1 AND count(DISTINCT user_id) > 1 THEN min(ip) ELSE '' END
		FROM sessions WHERE created_at > now() - interval '1 day' AND ip <> ''),
	(SELECT min(created_at) FROM outbox_events WHERE published_at IS NULL)`

// oneProxyAddress keeps the one address all logins came from only when it is private or
// loopback: a public one is a shared office line, which is no sign of an untrusted proxy.
func oneProxyAddress(address string) string {
	a, err := netip.ParseAddr(address)
	if err != nil || (!a.IsPrivate() && !a.IsLoopback()) {
		return ""
	}
	return address
}

// ReadHealthFacts reads what the checks judge: one statfs, the statement above and the
// migration state.
func ReadHealthFacts(ctx context.Context, src HealthSource, now time.Time) (health.Facts, error) {
	f := health.Facts{Now: now, MailConfigured: src.MailConfigured, OIDCConfigured: src.OIDCConfigured,
		ContractGrace: src.ContractGrace, Version: version.Version, BuildDate: version.BuildDate()}
	if spacer, ok := src.Store.(storage.Spacer); ok {
		// A store that cannot tell its room leaves the disk unknown, which never warns.
		if free, total, err := spacer.Space(ctx); err == nil {
			f.DiskFree, f.DiskTotal = free, total
		}
	}
	var lastBackup, lastMigration, oldestWaiting, oldestEvent *time.Time
	var size, sizeBefore *int64
	var loginAddress string
	err := src.Pool.QueryRow(ctx, healthFactsSQL, ActionBackupNoted, health.GrowthWindow.Seconds()).Scan(
		&f.FailedJobs7d, &f.OutboxStuck, &f.EventFailures, &f.MailFailed1d, &f.PasswordStops,
		&lastBackup, &lastMigration, &size, &sizeBefore, &f.AuditWaiting, &oldestWaiting, &f.BackupFailed, &f.RestoreCheckFailed,
		&loginAddress, &oldestEvent)
	if err != nil {
		return f, fmt.Errorf("app: read the health facts: %w", err)
	}
	f.OneLoginAddress = oneProxyAddress(loginAddress)
	if oldestWaiting != nil {
		f.OldestAuditWaiting = *oldestWaiting
	}
	if oldestEvent != nil {
		f.OldestEventWaiting = *oldestEvent
	}
	consumers, err := events.ReadConsumers(ctx, src.Pool)
	if err != nil {
		return f, err
	}
	for _, c := range consumers {
		if c.Lag > f.EventLag {
			f.EventLag, f.EventLagConsumer = c.Lag, c.Consumer
		}
	}
	if lastBackup != nil {
		f.LastBackup = *lastBackup
	}
	if lastMigration != nil {
		f.LastMigrationAt = *lastMigration
	}
	if size != nil {
		f.DatabaseBytes = *size
		if sizeBefore != nil {
			f.Growth30d, f.GrowthKnown = *size-*sizeBefore, true
		}
	}
	st, err := db.Status(ctx, src.Pool, src.Migrations)
	if err != nil {
		return f, fmt.Errorf("app: read the migration state: %w", err)
	}
	f.PendingContract = st.PendingContract
	return f, nil
}

// healthCache keeps the last facts of this node for healthFreshFor.
type healthCache struct {
	mu     sync.Mutex
	readAt time.Time
	facts  health.Facts
}

// HealthFacts are the facts of this installation, read again at most every 30 seconds.
func (a *App) HealthFacts(ctx context.Context) (health.Facts, error) {
	a.healthCache.mu.Lock()
	defer a.healthCache.mu.Unlock()
	now := time.Now()
	if !a.healthCache.readAt.IsZero() && now.Sub(a.healthCache.readAt) < healthFreshFor {
		return a.healthCache.facts, nil
	}
	facts, err := ReadHealthFacts(ctx, HealthSource{Pool: a.pool, Store: a.store, Migrations: a.migrations,
		MailConfigured: a.mail.Configured(), OIDCConfigured: a.sso != nil, ContractGrace: a.cfg.ContractGrace}, now)
	if err != nil {
		return facts, err
	}
	a.healthCache.readAt, a.healthCache.facts = now, facts
	return facts, nil
}

// HealthChecks judges the facts of this installation, for the console.
func (a *App) HealthChecks(ctx context.Context) ([]health.Check, error) {
	facts, err := a.HealthFacts(ctx)
	if err != nil {
		return nil, err
	}
	return health.Evaluate(facts), nil
}

// ReadyNotes are what /readyz adds while this node is ready but not at its best. A node
// with a nearly full disk stays ready, because text still saves (spec 051, D5). It reads
// only the guard's cached room, so a probe never waits for the database.
func (a *App) ReadyNotes(ctx context.Context) []string {
	if a.guard != nil && a.guard.Refusing(ctx) {
		return []string{"disk_low"}
	}
	return nil
}

// sampleDatabaseSize measures the database once an hour (spec 051, D3).
func (a *App) sampleDatabaseSize(ctx context.Context) error {
	_, err := a.pool.Exec(ctx, `INSERT INTO database_size_samples (bytes)
		VALUES (pg_database_size(current_database())) ON CONFLICT (taken_at) DO NOTHING`)
	return err
}

// PurgeFinishedWork removes what would otherwise grow for ever: finished jobs, sent mail,
// nodes that stopped without leaving, and old size samples (OPS-105). It is exported for
// tests.
func PurgeFinishedWork(ctx context.Context, q db.Querier) error {
	for _, step := range []struct {
		sql string
		age time.Duration
	}{
		{"DELETE FROM jobs WHERE status = 'done' AND finished_at < now() - make_interval(secs => $1)", keepFinishedWork},
		{"DELETE FROM jobs WHERE status = 'failed' AND finished_at < now() - make_interval(secs => $1)", keepFailedWork},
		{"DELETE FROM mail_queue WHERE status IN ('sent', 'dropped') AND created_at < now() - make_interval(secs => $1)", keepFinishedWork},
		{"DELETE FROM mail_queue WHERE status = 'failed' AND created_at < now() - make_interval(secs => $1)", keepFailedWork},
		{"DELETE FROM nodes WHERE last_seen_at < now() - make_interval(secs => $1)", keepGoneNode},
		{"DELETE FROM database_size_samples WHERE taken_at < now() - make_interval(secs => $1)", keepSizeSamples},
		// A year of backup history is enough to see a pattern; the backups themselves are
		// pruned by the worker (spec 058).
		{"DELETE FROM backup_runs WHERE finished_at < now() - make_interval(secs => $1)", keepBackupHistory},
		{"DELETE FROM backup_restore_checks WHERE checked_at < now() - make_interval(secs => $1)", keepBackupHistory},
		{"DELETE FROM backup_requests WHERE finished_at < now() - make_interval(secs => $1)", keepFailedWork},
	} {
		if _, err := q.Exec(ctx, step.sql, step.age.Seconds()); err != nil {
			return fmt.Errorf("app: purge finished work: %w", err)
		}
	}
	return nil
}

func (a *App) purgeFinishedWork(ctx context.Context) error { return PurgeFinishedWork(ctx, a.pool) }
