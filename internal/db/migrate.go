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
//   - a contract migration only removes what the PREVIOUS release already stopped
//     reading and writing, so rolling back one release stays safe after it ran (spec 048 D2)
//   - every migration runs with a statement_timeout of a minute; a header line
//     "-- statement_timeout: <duration>" gives one more, with the reason below it
//   - a header line "-- transaction: none" runs a file of exactly one statement outside a
//     transaction, for CREATE INDEX CONCURRENTLY (OPS-073)
//   - from 0048 on, a statement that rewrites or long locks a table is refused by
//     TestNoMigrationRewritesATable unless a "-- rewrite-ok: <reason>" comment allows it

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
	// NoTransaction runs the one statement of the file outside a transaction (OPS-073).
	NoTransaction bool
	// StatementTimeout is the header's own limit; zero means the run's default (OPS-074).
	StatementTimeout time.Duration
}

// DefaultStatementTimeout stops a migration that holds its lock longer than planned. Long
// work belongs in a background job (OPS-035), not in a migration that holds a table.
const DefaultStatementTimeout = time.Minute

// The header lines a migration may carry after its phase.
const (
	headerTransaction      = "-- transaction:"
	headerStatementTimeout = "-- statement_timeout:"
)

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
		mig := Migration{Version: v, Name: m[2], Phase: phase, SQL: string(body), Checksum: hex.EncodeToString(sum[:])}
		if err := readHeaders(&mig); err != nil {
			return nil, fmt.Errorf("db: migration %q: %w", e.Name(), err)
		}
		out = append(out, mig)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	for i, m := range out {
		if m.Version != i+1 {
			return nil, fmt.Errorf("db: migration versions must be contiguous from 0001, found %04d at position %d", m.Version, i+1)
		}
	}
	return out, nil
}

// readHeaders reads the comment lines at the top of a migration, after its phase.
func readHeaders(m *Migration) error {
	for _, line := range strings.Split(m.SQL, "\n")[1:] {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "--") {
			break
		}
		switch {
		case strings.HasPrefix(line, headerTransaction):
			if value := strings.TrimSpace(strings.TrimPrefix(line, headerTransaction)); value != "none" {
				return fmt.Errorf("%s may only say none, not %q", headerTransaction, value)
			}
			m.NoTransaction = true
		case strings.HasPrefix(line, headerStatementTimeout):
			value := strings.TrimSpace(strings.TrimPrefix(line, headerStatementTimeout))
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 {
				return fmt.Errorf("%s needs a positive duration such as 5m, not %q", headerStatementTimeout, value)
			}
			m.StatementTimeout = d
		}
	}
	if m.NoTransaction && statementCount(m.SQL) != 1 {
		return fmt.Errorf("a migration with %s none must hold exactly one statement, ending in a semicolon", headerTransaction)
	}
	return nil
}

// statementCount counts the statements of a file without a transaction: whole-line
// comments do not count, and the text must end with the last semicolon. It is meant for the
// one-line statements such files hold, not for SQL with semicolons inside strings.
func statementCount(sql string) int {
	var code strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "--") {
			code.WriteString(t)
			code.WriteString("\n")
		}
	}
	text := strings.TrimSpace(code.String())
	if text == "" || !strings.HasSuffix(text, ";") {
		return 0
	}
	return strings.Count(text, ";")
}

// Options control a migration run.
type Options struct {
	IncludeContract bool
	Log             *slog.Logger
	// StatementTimeout replaces DefaultStatementTimeout for migrations without a header of
	// their own. Zero means the default.
	StatementTimeout time.Duration
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
		if err := applyWithRetry(ctx, conn, m, statementTimeout(m, opts), log); err != nil {
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
func applyWithRetry(ctx context.Context, conn *pgxpool.Conn, m Migration, timeout time.Duration, log *slog.Logger) error {
	const attempts = 6
	for i := 1; ; i++ {
		var err error
		if m.NoTransaction {
			err = applyAlone(ctx, conn, m, timeout)
		} else {
			err = applyOnce(ctx, conn, m, timeout)
		}
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

// statementTimeout is the limit one migration runs under: its own header, else the run's.
func statementTimeout(m Migration, opts Options) time.Duration {
	if m.StatementTimeout > 0 {
		return m.StatementTimeout
	}
	if opts.StatementTimeout > 0 {
		return opts.StatementTimeout
	}
	return DefaultStatementTimeout
}

const recordMigration = "INSERT INTO schema_migrations (version, name, phase, checksum) VALUES ($1, $2, $3, $4)"

func applyOnce(ctx context.Context, conn *pgxpool.Conn, m Migration, timeout time.Duration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The values are numbers made here, not input, so they can be part of the text: SET
	// takes no parameters.
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL lock_timeout = '5s'; SET LOCAL statement_timeout = %d", timeout.Milliseconds())); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, recordMigration, m.Version, m.Name, string(m.Phase), m.Checksum); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// applyAlone runs a migration of one statement outside a transaction, with the same limits,
// then checks that it left no index invalid: a concurrent build that fails leaves one
// behind, and IF NOT EXISTS would skip it quietly the next time (OPS-073).
func applyAlone(ctx context.Context, conn *pgxpool.Conn, m Migration, timeout time.Duration) error {
	defer func() {
		// A fresh context, so the session's limits are reset even when ctx was cancelled.
		_, _ = conn.Exec(context.Background(), "RESET lock_timeout; RESET statement_timeout")
	}()
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET lock_timeout = '5s'; SET statement_timeout = %d", timeout.Milliseconds())); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, m.SQL); err != nil {
		return err
	}
	invalid, err := invalidIndexes(ctx, conn)
	if err != nil {
		return err
	}
	if len(invalid) > 0 {
		return fmt.Errorf("index %s was left invalid by a failed concurrent build; drop it with DROP INDEX CONCURRENTLY and start again",
			strings.Join(invalid, ", "))
	}
	_, err = conn.Exec(ctx, recordMigration, m.Version, m.Name, string(m.Phase), m.Checksum)
	return err
}

// invalidIndexes names the indexes of this schema PostgreSQL marks as not valid.
func invalidIndexes(ctx context.Context, q Querier) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT c.relname FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT i.indisvalid AND n.nspname = current_schema() ORDER BY c.relname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
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
