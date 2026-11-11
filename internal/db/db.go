// SPDX-License-Identifier: Apache-2.0

// Package db owns the PostgreSQL connection pool and the migration runner.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is satisfied by *pgxpool.Pool, *pgxpool.Conn and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Open creates a connection pool and checks that the database answers.
// Pool size can be tuned in the URL (pool_max_conns=...); in HA setups put
// PgBouncer in front of PostgreSQL.
// defaultMaxConns replaces the driver's default of the larger of four and the
// number of CPUs, which is too small for a node that serves people (ARC-175).
const defaultMaxConns = 20

// Open creates a connection pool and checks that the database answers.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// Never echo the URL: it contains the password.
		return nil, errors.New("db: invalid DATABASE_URL")
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "diffluence"
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	if !strings.Contains(url, "pool_max_conns") {
		cfg.MaxConns = defaultMaxConns
	}
	cfg.ConnConfig.Tracer = &queryCounter{}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: cannot reach database: %w", err)
	}
	return pool, nil
}

// queryCounter counts the queries of a pool, for the query budgets of the main screens
// (ARC-239) and for /metrics, and how many of them were allowed to lag (ARC-228).
type queryCounter struct {
	n   atomic.Int64
	lag atomic.Int64
}

func (q *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.n.Add(1)
	if FreshnessOf(ctx) == MayLag {
		q.lag.Add(1)
	}
	return ctx
}

func (*queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// The pool calls the two methods below around every wait for a connection, and uses
// the context TraceAcquireStart returns for that wait. That is where a request's limit
// on waiting applies, so no code path can hang on a full pool (spec 050).

// TraceAcquireStart bounds the wait when the context carries a limit.
func (*queryCounter) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	watch, ok := ctx.Value(acquireLimitKey{}).(*acquireWatch)
	if !ok {
		return ctx
	}
	bounded, cancel := context.WithTimeoutCause(ctx, watch.limit, errAcquireLimit)
	return context.WithValue(bounded, acquireCancelKey{}, cancel)
}

// TraceAcquireEnd notes a wait that ran out, and frees the timer of the wait.
func (*queryCounter) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	cancel, ok := ctx.Value(acquireCancelKey{}).(context.CancelFunc)
	if !ok {
		return
	}
	if data.Err != nil && errors.Is(context.Cause(ctx), errAcquireLimit) {
		if watch, ok := ctx.Value(acquireLimitKey{}).(*acquireWatch); ok {
			watch.ranOut.Store(true)
		}
	}
	cancel()
}

// errAcquireLimit is the cause of a wait for a connection that took too long.
var errAcquireLimit = errors.New("db: no connection free in time")

type acquireLimitKey struct{}

type acquireCancelKey struct{}

// acquireWatch is the limit of one request, and whether a wait ran into it.
type acquireWatch struct {
	limit  time.Duration
	ranOut atomic.Bool
}

// WithAcquireLimit makes every wait for a connection under ctx give up after limit.
// Requests get one, so a full pool answers "busy" instead of hanging; background work
// keeps waiting, because it has nobody to answer.
func WithAcquireLimit(ctx context.Context, limit time.Duration) context.Context {
	return context.WithValue(ctx, acquireLimitKey{}, &acquireWatch{limit: limit})
}

// AcquireRanOut reports whether a wait for a connection under ctx gave up at its limit.
func AcquireRanOut(ctx context.Context) bool {
	watch, ok := ctx.Value(acquireLimitKey{}).(*acquireWatch)
	return ok && watch.ranOut.Load()
}

// Queries returns how many queries a pool made by Open has sent since it started.
// A pool made another way counts nothing.
func Queries(pool *pgxpool.Pool) int64 {
	if counter, ok := pool.Config().ConnConfig.Tracer.(*queryCounter); ok {
		return counter.n.Load()
	}
	return 0
}

// LagQueries returns how many of those queries were marked as allowed to lag.
func LagQueries(pool *pgxpool.Pool) int64 {
	if counter, ok := pool.Config().ConnConfig.Tracer.(*queryCounter); ok {
		return counter.lag.Load()
	}
	return 0
}

// GrantApp gives the application role exactly what it needs, after migrations ran
// as the owner of the tables. The role can read and write content but cannot
// change or delete audit events: a blueprint 06 requirement. Because it owns
// nothing, it cannot disable the triggers either.
//
// A role name cannot be a query parameter, so it is quoted as an identifier.
func GrantApp(ctx context.Context, owner *pgxpool.Pool, role string) error {
	ident := pgx.Identifier{role}.Sanitize()
	return pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		// GRANT rewrites catalog rows, so two nodes doing it at once would fail with
		// "tuple concurrently updated". The migration lock makes them take turns, and
		// makes this wait for a migration that is still running.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockKey); err != nil {
			return fmt.Errorf("db: take migration lock: %w", err)
		}
		for _, stmt := range []string{
			"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + ident,
			"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + ident,
			"REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM " + ident,
			"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON schema_migrations FROM " + ident,
			"REVOKE UPDATE, DELETE, TRUNCATE ON audit_checkpoints FROM " + ident,
			// Verification starts at an anchor, so the application may only read it.
			"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON audit_anchors FROM " + ident,
			// A personal part can be erased, and never changed.
			"REVOKE UPDATE, TRUNCATE ON audit_personal FROM " + ident,
			// A waiting event is sealed and then removed, or its personal part is erased;
			// the trigger of audit_pending allows nothing else (ADR-041).
			"REVOKE TRUNCATE ON audit_pending FROM " + ident,
			// Grants are what roles add up to. Only the role triggers, which run as the
			// owner, write them, so no code path can grant past a role (spec 036).
			"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON space_grants FROM " + ident,
			"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON global_grants FROM " + ident,
		} {
			if _, err := tx.Exec(ctx, stmt); err != nil {
				return fmt.Errorf("db: grant to application role: %w", err)
			}
		}
		return nil
	})
}

// RoleOf returns the user name in a connection URL.
func RoleOf(url string) (string, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return "", errors.New("db: invalid DATABASE_URL")
	}
	return cfg.ConnConfig.User, nil
}

// IsUniqueViolation reports whether err is a unique-constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isLockTimeout reports whether err was caused by lock_timeout.
func isLockTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "55P03"
}
