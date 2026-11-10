// SPDX-License-Identifier: Apache-2.0

package db

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration rules (see docs/blueprint/07):
//   - files are named NNNN_snake_case.sql and are never edited once merged
//   - the first line is "-- phase: expand" or "-- phase: contract"
//   - expand migrations only ADD things, so the previous release keeps working
//   - contract migrations REMOVE things and only run when explicitly allowed,
//     in a later release, after every node runs code that no longer needs them

//go:embed migrations/*.sql
var embedded embed.FS

// Phase is the kind of migration.
type Phase string

const (
	PhaseExpand   Phase = "expand"
	PhaseContract Phase = "contract"
)

// Migration is one numbered SQL file.
type Migration struct {
	Version  int
	Name     string
	Phase    Phase
	SQL      string
	Checksum string
}

// ErrChecksumMismatch means an applied migration file was changed afterwards.
var ErrChecksumMismatch = errors.New("db: an applied migration was modified")

// migrationLockKey is the advisory lock that serialises migration runs across nodes.
const migrationLockKey int64 = 0x6466_6c75_6d69_6772 // "dflumigr"

var fileName = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Migrations returns the migrations embedded in this binary, in order.
func Migrations() ([]Migration, error) { return Parse(embedded, "migrations") }

// Parse reads and validates migration files from dir.
func Parse(fsys fs.FS, dir string) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("db: migration file %q does not match NNNN_name.sql", e.Name())
		}
		body, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		first, _, _ := strings.Cut(string(body), "\n")
		var phase Phase
		switch strings.TrimSpace(first) {
		case "-- phase: expand":
			phase = PhaseExpand
		case "-- phase: contract":
			phase = PhaseContract
		default:
			return nil, fmt.Errorf("db: migration %q must start with '-- phase: expand' or '-- phase: contract'", e.Name())
		}
		v, _ := strconv.Atoi(m[1])
		sum := sha256.Sum256(body)
		out = append(out, Migration{Version: v, Name: m[2], Phase: phase, SQL: string(body), Checksum: hex.EncodeToString(sum[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i, m := range out {
		if m.Version != i+1 {
			return nil, fmt.Errorf("db: migration versions must be contiguous from 0001, found %04d at position %d", m.Version, i+1)
		}
	}
	return out, nil
}

// Options control a migration run.
type Options struct {
	IncludeContract bool
	Log             *slog.Logger
}

// Report says what a run did.
type Report struct {
	Applied         []Migration
	PendingContract []Migration
	NewerInDatabase int // versions applied by a newer release than this binary
}

const createTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version    integer PRIMARY KEY,
	name       text NOT NULL,
	phase      text NOT NULL,
	checksum   text NOT NULL,
	applied_at timestamptz NOT NULL DEFAULT now()
)`

// Migrate applies pending migrations. It is safe to call from many nodes at
// once: an advisory lock makes them take turns, and finished work is skipped.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migs []Migration, opts Options) (Report, error) {
	log := opts.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	var rep Report

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return rep, err
	}
	defer conn.Release()

	if err := RequirePostgres(ctx, conn); err != nil {
		return rep, err
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return rep, fmt.Errorf("db: take migration lock: %w", err)
	}
	defer func() {
		// Use a fresh context so the lock is released even if ctx was cancelled.
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	if _, err := conn.Exec(ctx, createTable); err != nil {
		return rep, fmt.Errorf("db: create schema_migrations: %w", err)
	}
	if err := CheckCompatible(ctx, conn, migs); err != nil {
		return rep, err
	}
	applied, err := appliedChecksums(ctx, conn)
	if err != nil {
		return rep, err
	}

	known := make(map[int]bool, len(migs))
	for _, m := range migs {
		known[m.Version] = true
		if sum, ok := applied[m.Version]; ok && sum != m.Checksum {
			return rep, fmt.Errorf("%w: %04d_%s", ErrChecksumMismatch, m.Version, m.Name)
		}
	}
	for v := range applied {
		if !known[v] {
			rep.NewerInDatabase++
		}
	}
	if rep.NewerInDatabase > 0 {
		log.Info("database has migrations from a newer release; this node keeps serving (N/N+1 compatibility)", "newer", rep.NewerInDatabase)
	}

	for _, m := range migs {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		if m.Phase == PhaseContract && !opts.IncludeContract {
			rep.PendingContract = append(rep.PendingContract, m)
			continue
		}
		if err := applyWithRetry(ctx, conn, m, log); err != nil {
			return rep, err
		}
		rep.Applied = append(rep.Applied, m)
		log.Info("migration applied", "version", m.Version, "name", m.Name, "phase", m.Phase)
	}
	return rep, nil
}

func appliedChecksums(ctx context.Context, q Querier) (map[int]string, error) {
	rows, err := q.Query(ctx, "SELECT version, checksum FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]string{}
	for rows.Next() {
		var v int
		var sum string
		if err := rows.Scan(&v, &sum); err != nil {
			return nil, err
		}
		out[v] = sum
	}
	return out, rows.Err()
}

// applyWithRetry runs one migration in a transaction with a short lock_timeout,
// so a migration never blocks live traffic for long. On a lock timeout it waits
// and tries again instead of queueing behind (and in front of) user queries.
func applyWithRetry(ctx context.Context, conn *pgxpool.Conn, m Migration, log *slog.Logger) error {
	const attempts = 6
	for i := 1; ; i++ {
		err := applyOnce(ctx, conn, m)
		if err == nil {
			return nil
		}
		if !isLockTimeout(err) || i == attempts {
			return fmt.Errorf("db: migration %04d_%s failed: %w", m.Version, m.Name, err)
		}
		wait := time.Duration(i) * 2 * time.Second
		log.Warn("migration waiting for a lock, retrying", "version", m.Version, "attempt", i, "wait", wait)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

func applyOnce(ctx context.Context, conn *pgxpool.Conn, m Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '5s'"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version, name, phase, checksum) VALUES ($1, $2, $3, $4)",
		m.Version, m.Name, string(m.Phase), m.Checksum); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PendingContract lists contract migrations of this binary that are not applied.
func PendingContract(ctx context.Context, q Querier, migs []Migration) ([]Migration, error) {
	applied, err := appliedChecksums(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []Migration
	for _, m := range migs {
		if _, ok := applied[m.Version]; !ok && m.Phase == PhaseContract {
			out = append(out, m)
		}
	}
	return out, nil
}

// PendingExpand counts expand migrations of this binary that are not applied yet.
// A node is not ready to serve while this is above zero.
func PendingExpand(ctx context.Context, q Querier, migs []Migration) (int, error) {
	applied, err := appliedChecksums(ctx, q)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, m := range migs {
		if _, ok := applied[m.Version]; !ok && m.Phase == PhaseExpand {
			n++
		}
	}
	return n, nil
}
