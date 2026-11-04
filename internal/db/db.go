// SPDX-License-Identifier: Apache-2.0

// Package db owns the PostgreSQL connection pool and the migration runner.
package db

import (
	"context"
	"errors"
	"fmt"
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

// GrantApp gives the application role exactly what it needs, after migrations ran
// as the owner of the tables. The role can read and write content but cannot
// change or delete audit events: a blueprint 06 requirement. Because it owns
// nothing, it cannot disable the triggers either.
//
// A role name cannot be a query parameter, so it is quoted as an identifier.
func GrantApp(ctx context.Context, owner *pgxpool.Pool, role string) error {
	ident := pgx.Identifier{role}.Sanitize()
	for _, stmt := range []string{
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + ident,
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO " + ident,
		"REVOKE UPDATE, DELETE, TRUNCATE ON audit_events FROM " + ident,
		"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON schema_migrations FROM " + ident,
		"REVOKE UPDATE, DELETE, TRUNCATE ON audit_checkpoints FROM " + ident,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("db: grant to application role: %w", err)
		}
	}
	return nil
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
