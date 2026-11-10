// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"runtime"
	"time"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/metrics"
)

// gaugeTimeout keeps a scrape short when the database is slow.
const gaugeTimeout = 2 * time.Second

func boolGauge(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

// Gauges are the numbers /metrics reads when scraped (spec 021 D4). When the
// database does not answer, that is itself the number to show, so the scrape still
// gives what this node knows.
func (a *App) Gauges(ctx context.Context) []metrics.Gauge {
	stat := a.pool.Stat()
	gauges := []metrics.Gauge{
		{Name: "diffluence_goroutines", Help: "Goroutines in this process.", Value: float64(runtime.NumGoroutine())},
		{Name: "diffluence_queue_healthy", Help: "Whether this node reaches the queue.", Value: boolGauge(a.queue.Healthy())},
		{Name: "diffluence_db_pool_acquired_connections", Help: "Database connections in use.", Value: float64(stat.AcquiredConns())},
		{Name: "diffluence_db_pool_idle_connections", Help: "Database connections waiting for work.", Value: float64(stat.IdleConns())},
		{Name: "diffluence_db_pool_max_connections", Help: "The largest the pool may get.", Value: float64(stat.MaxConns())},
		{Name: "diffluence_db_queries_total", Help: "Queries this node sent since it started.", Value: float64(db.Queries(a.pool))},
		// The share that could move to a standby, measured before one exists (ARC-228).
		{Name: "diffluence_db_queries_may_lag_total", Help: "Of those, the queries that may read from a copy that lags.", Value: float64(db.LagQueries(a.pool))},
	}
	ctx, cancel := context.WithTimeout(ctx, gaugeTimeout)
	defer cancel()
	var outbox, jobsQueued, jobsFailed, mailQueued, deletions int
	err := a.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM outbox_events WHERE published_at IS NULL),
		(SELECT count(*) FROM jobs WHERE status = 'queued'),
		(SELECT count(*) FROM jobs WHERE status = 'failed' AND created_at > now() - interval '7 days'),
		(SELECT count(*) FROM mail_queue WHERE status = 'queued'),
		(SELECT count(*) FROM storage_deletions)`).Scan(&outbox, &jobsQueued, &jobsFailed, &mailQueued, &deletions)
	if err != nil {
		a.log.Warn("metrics: database unavailable", "error", err)
		return append(gauges, metrics.Gauge{Name: "diffluence_database_up", Help: "Whether the database answered this scrape.", Value: 0})
	}
	gauges = append(gauges,
		metrics.Gauge{Name: "diffluence_database_up", Help: "Whether the database answered this scrape.", Value: 1},
		metrics.Gauge{Name: "diffluence_outbox_unpublished", Help: "Events waiting to reach the queue.", Value: float64(outbox)},
		metrics.Gauge{Name: "diffluence_jobs_queued", Help: "Jobs waiting for a worker.", Value: float64(jobsQueued)},
		metrics.Gauge{Name: "diffluence_jobs_failed_7d", Help: "Jobs that failed in the last seven days.", Value: float64(jobsFailed)},
		metrics.Gauge{Name: "diffluence_mail_queued", Help: "Mails waiting to be sent.", Value: float64(mailQueued)},
		metrics.Gauge{Name: "diffluence_storage_deletions_queued", Help: "Stored files waiting to be removed.", Value: float64(deletions)},
	)
	// How far an upgrade is (blueprint 07): a contract migration that stays pending long
	// after every node runs the new release asks for a look.
	schema, err := db.Status(ctx, a.pool, a.migrations)
	if err != nil {
		a.log.Warn("metrics: migration state unavailable", "error", err)
		return gauges
	}
	return append(gauges,
		metrics.Gauge{Name: "diffluence_schema_migration", Help: "The highest migration applied to the database.", Value: float64(schema.Newest)},
		metrics.Gauge{Name: "diffluence_migrations_pending_expand", Help: "Expand migrations of this node not applied yet.", Value: float64(schema.PendingExpand)},
		metrics.Gauge{Name: "diffluence_migrations_pending_contract", Help: "Contract migrations of this node waiting for every node to know them.", Value: float64(schema.PendingContract)},
	)
}
