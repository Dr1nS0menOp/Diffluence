// SPDX-License-Identifier: Apache-2.0

// Package restore carries a full restore of the installation from the console (spec 059).
// The console asks; every node stops serving and shows a waiting page; the backup worker
// makes a safety backup, restores the chosen backup, brings the database up to this
// release, writes the restore into the restored audit log and checks it, and only then
// lets the nodes serve again. What steers it lives in the schema ops, which a restore never
// touches, so the state survives the rewind of every table in public.
package restore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/backups"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

// The states of a restore, in order. requested and preparing leave the nodes serving;
// stopping and backing_up keep them waiting but nothing has changed yet; from restoring on
// the installation is being changed. done and refused end a restore; failed ends it only
// when nothing had changed, and otherwise keeps every node waiting for the way back.
const (
	StateRequested = "requested"
	StatePreparing = "preparing"
	StateStopping  = "stopping"
	StateBackingUp = "backing_up"
	StateRestoring = "restoring"
	StateChecking  = "checking"
	StateDone      = "done"
	StateRefused   = "refused"
	StateFailed    = "failed"
)

// Modes a node says it is in.
const (
	ModeStarting = "starting"
	ModeServing  = "serving"
	ModeWaiting  = "waiting"
)

const (
	// PerDay is how many restores may be asked for in a day (spec 058, 3.2): a restore stops
	// the whole installation, so it must never become something to press again and again.
	PerDay = 1
	// NodeGoneAfter is how long a node may be silent before the worker counts it as gone.
	// A node says what it does every few seconds.
	NodeGoneAfter = 20 * time.Second
	// abandonedAfter is how long a restore may stand still before anything changed, before
	// the nodes stop waiting for it: a worker that died must not keep the site down.
	abandonedAfter = 6 * time.Hour
	maxStep        = 300
	maxName        = 200
)

// ErrNodesServing means a node kept serving: the worker never restores under it.
var ErrNodesServing = errors.New("restore: a node kept serving")

// Restore is one restore, as the console and the nodes see it.
type Restore struct {
	ID              uuid.UUID  `json:"id"`
	BackupName      string     `json:"backup_name"`
	RequestedByName string     `json:"requested_by_name"`
	RequestedAt     time.Time  `json:"requested_at"`
	State           string     `json:"state"`
	Step            string     `json:"step"`
	SafetyName      string     `json:"safety_name"`
	Changed         bool       `json:"changed"`
	UpdatedAt       time.Time  `json:"updated_at"`
	FinishedAt      *time.Time `json:"finished_at"`
	// CodeShown says the backup service wrote the confirmation code to its log; CodeEntered,
	// that a code waits for it to compare (spec 070).
	CodeShown   bool `json:"code_shown"`
	CodeEntered bool `json:"code_entered"`
	WrongCodes  int  `json:"wrong_codes"`
}

// Open says whether the restore is still on its way, or failed after it changed something.
func (r Restore) Open() bool { return r.FinishedAt == nil }

const columns = `id, backup_name, requested_by_name, requested_at, state, step, coalesce(safety_name, ''),
	changed, updated_at, finished_at, code_shown_at IS NOT NULL, confirmation IS NOT NULL, wrong_codes`

func scan(row pgx.Row) (Restore, error) { return scanWith(row) }

// scanWith reads the columns of a restore, then extra.
func scanWith(row pgx.Row, extra ...any) (Restore, error) {
	var r Restore
	dest := append([]any{&r.ID, &r.BackupName, &r.RequestedByName, &r.RequestedAt, &r.State, &r.Step, &r.SafetyName,
		&r.Changed, &r.UpdatedAt, &r.FinishedAt, &r.CodeShown, &r.CodeEntered, &r.WrongCodes}, extra...)
	err := row.Scan(dest...)
	return r, err
}

// missing says the schema ops is not there yet: a release from before spec 059 made the
// database, and nothing can be restoring.
func missing(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "42P01" || pgErr.Code == "3F000")
}

// cleanStep keeps a step to one short line of printable text.
func cleanStep(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > maxStep {
		s = string([]rune(s)[:maxStep])
	}
	return s
}

// Ask is a request for a restore.
type Ask struct {
	BackupName string
	// Confirm is what the person typed; it must be the installation's name.
	Confirm      string
	Installation string
	Caller       identity.Caller
	CallerName   string
}

// Request asks the worker to restore a backup over the live installation. The caller has
// proved who they are with a second factor (the route checks it). It refuses a name typed
// wrong, a backup the worker never reported complete, a restore while a backup or another
// restore is open, and more than PerDay a day.
func Request(ctx context.Context, pool *pgxpool.Pool, a Ask) (Restore, error) {
	if strings.TrimSpace(a.Confirm) != a.Installation {
		return Restore{}, apperr.Invalid(fmt.Sprintf("Type the name of this installation, %s, to confirm.", a.Installation))
	}
	name := a.CallerName
	if len([]rune(name)) > maxName {
		name = string([]rune(name)[:maxName])
	}
	var out Restore
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		ready, err := backups.ReadinessToRestore(ctx, tx, a.BackupName)
		if err != nil {
			return err
		}
		if !ready.Complete {
			return apperr.Invalid("Only a backup the worker reported complete can be restored.")
		}
		if ready.BackupOpen {
			return apperr.Conflict("backup_requested", "A backup was asked for and is not finished yet. Restore after it.")
		}
		var today int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM ops.restores WHERE requested_at > now() - interval '1 day'`).Scan(&today); err != nil {
			return err
		}
		if today >= PerDay {
			return apperr.Conflict("restore_limit", fmt.Sprintf("At most %d restore a day can be asked for.", PerDay))
		}
		var by *uuid.UUID
		if a.Caller.UserID != uuid.Nil {
			by = &a.Caller.UserID
		}
		row := tx.QueryRow(ctx, `INSERT INTO ops.restores (id, backup_name, requested_by, requested_by_name, state)
			VALUES ($1, $2, $3, $4, $5) RETURNING `+columns, ids.New(), a.BackupName, by, name, StateRequested)
		r, err := scan(row)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// restores_one_open lets one restore be open, also when two administrators ask
			// at the same moment.
			return apperr.Conflict("restore_open", "A restore is already on its way.")
		}
		if err != nil {
			return err
		}
		out = r
		_, err = audit.Record(ctx, tx, a.Caller.Event("backup.restore_requested", "backup", a.BackupName, "", nil))
		return err
	})
	return out, err
}

// Cancel takes back a restore the worker has not picked up yet.
func Cancel(ctx context.Context, pool *pgxpool.Pool, c identity.Caller) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `UPDATE ops.restores SET state = $1, step = 'taken back before the worker began',
				updated_at = now(), finished_at = now()
			WHERE finished_at IS NULL AND state = $2 RETURNING backup_name`, StateRefused, StateRequested).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("restore_begun", "No restore waits for the worker; one that began cannot be taken back.")
		}
		if err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("backup.restore_cancelled", "backup", name, "", nil))
		return err
	})
}

// Latest is the newest restore, or nil when there was none.
func Latest(ctx context.Context, q db.Querier) (*Restore, error) {
	r, err := scan(q.QueryRow(ctx, "SELECT "+columns+" FROM ops.restores ORDER BY requested_at DESC LIMIT 1"))
	if errors.Is(err, pgx.ErrNoRows) || missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// MustWait is the restore every node waits for, or nil when nodes may serve. Nodes wait
// from the moment the worker begins to stop them until the restore is done; and after a
// restore that failed once it had changed something, until the way back ran. A restore
// that stood still for hours before it changed anything is not waited for.
func MustWait(ctx context.Context, q db.Querier) (*Restore, error) {
	r, err := scan(q.QueryRow(ctx, "SELECT "+columns+` FROM ops.restores
		WHERE finished_at IS NULL AND (
			(state IN ($1, $2) AND updated_at > now() - make_interval(secs => $3))
			OR state IN ($4, $5, $6))`,
		StateStopping, StateBackingUp, abandonedAfter.Seconds(), StateRestoring, StateChecking, StateFailed))
	if errors.Is(err, pgx.ErrNoRows) || missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Seen notes what a node is doing.
func Seen(ctx context.Context, q db.Querier, node uuid.UUID, mode string) error {
	_, err := q.Exec(ctx, `INSERT INTO ops.nodes (node_id, mode, seen_at) VALUES ($1, $2, now())
		ON CONFLICT (node_id) DO UPDATE SET mode = EXCLUDED.mode, seen_at = now()`, node, mode)
	if missing(err) {
		return nil
	}
	return err
}

// Leave forgets a node at once, for a node that stops saying it is starting.
func Leave(ctx context.Context, q db.Querier, node uuid.UUID) error {
	_, err := q.Exec(ctx, "DELETE FROM ops.nodes WHERE node_id = $1", node)
	if missing(err) {
		return nil
	}
	return err
}

// Busy counts the nodes that said they are starting or serving within NodeGoneAfter, and
// forgets nodes silent for a day.
func Busy(ctx context.Context, q db.Querier) (int, error) {
	if _, err := q.Exec(ctx, "DELETE FROM ops.nodes WHERE seen_at < now() - interval '1 day'"); err != nil {
		return 0, err
	}
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM ops.nodes WHERE mode IN ($1, $2) AND seen_at > now() - make_interval(secs => $3)`,
		ModeStarting, ModeServing, NodeGoneAfter.Seconds()).Scan(&n)
	return n, err
}

// WaitForNodes waits until no node is starting or serving, and gives up after timeout.
func WaitForNodes(ctx context.Context, q db.Querier, timeout, every time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		n, err := Busy(ctx, q)
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: %d node(s) still serving after %s", ErrNodesServing, n, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(every):
		}
	}
}

// Claim looks at the requested restore for the worker (spec 070). It hands over a restore
// whose confirmation code was entered right; it asks the worker to write the code of a new
// request to its log; and it ends a request without a key, with wrong codes, or with none
// in time. First it ends a restore whose worker stopped long ago before anything changed.
func Claim(ctx context.Context, pool *pgxpool.Pool, g Gate) (Claimed, error) {
	var out Claimed
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE ops.restores SET state = $1, finished_at = now(), updated_at = now(),
				step = 'the worker stopped before it changed anything'
			WHERE finished_at IS NULL AND NOT changed AND state IN ($2, $3, $4)
				AND updated_at < now() - make_interval(secs => $5)`,
			StateRefused, StatePreparing, StateStopping, StateBackingUp, abandonedAfter.Seconds()); err != nil {
			return err
		}
		var err error
		out, err = gateClaim(ctx, tx, g)
		return err
	})
	if missing(err) {
		return Claimed{}, nil
	}
	return out, err
}

// Get reads one restore.
func Get(ctx context.Context, q db.Querier, id uuid.UUID) (Restore, error) {
	r, err := scan(q.QueryRow(ctx, "SELECT "+columns+" FROM ops.restores WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Restore{}, apperr.NotFound()
	}
	return r, err
}

// Move puts an open restore in a state. Entering restoring marks it changed, before the
// first step that alters the installation.
func Move(ctx context.Context, q db.Querier, id uuid.UUID, state, step string) error {
	tag, err := q.Exec(ctx, `UPDATE ops.restores SET state = $2, step = $3, updated_at = now(),
			changed = changed OR $2 = $4
		WHERE id = $1 AND finished_at IS NULL`, id, state, cleanStep(step), StateRestoring)
	if err == nil && tag.RowsAffected() != 1 {
		err = fmt.Errorf("restore: %s is not open", id)
	}
	return err
}

// setSafety names the safety backup made before the restore changed anything, sealed by the
// worker that made it, so the way back restores only a backup it made (spec 070).
func setSafety(ctx context.Context, q db.Querier, g Gate, id uuid.UUID, name string) error {
	_, err := q.Exec(ctx, "UPDATE ops.restores SET safety_name = $2, safety_seal = $3, updated_at = now() WHERE id = $1",
		id, name, g.seal("safety", id, name))
	return err
}

// Fail ends a restore that changed nothing, so the nodes serve again; a restore that
// changed something stays open and failed, so they keep waiting for the way back.
func Fail(ctx context.Context, q db.Querier, id uuid.UUID, step string) error {
	_, err := q.Exec(ctx, `UPDATE ops.restores SET step = $2, updated_at = now(),
			state = CASE WHEN changed THEN $3 ELSE $4 END,
			finished_at = CASE WHEN changed THEN NULL ELSE now() END
		WHERE id = $1 AND finished_at IS NULL`, id, cleanStep(step), StateFailed, StateRefused)
	return err
}

// Finish ends a restore that passed its checks.
func Finish(ctx context.Context, q db.Querier, id uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE ops.restores SET state = $2, step = '', updated_at = now(), finished_at = now()
		WHERE id = $1 AND finished_at IS NULL`, id, StateDone)
	return err
}

// FailedOpen is the restore that failed after it changed something, or nil.
func FailedOpen(ctx context.Context, q db.Querier) (*Restore, error) {
	r, err := scan(q.QueryRow(ctx, "SELECT "+columns+" FROM ops.restores WHERE finished_at IS NULL AND state = $1", StateFailed))
	if errors.Is(err, pgx.ErrNoRows) || missing(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// takeOver turns a failed restore into the restore of its safety backup: the way back. The
// restore is sealed again for the backup it now restores.
func takeOver(ctx context.Context, q db.Querier, g Gate, id uuid.UUID, safety string) error {
	tag, err := q.Exec(ctx, `UPDATE ops.restores SET backup_name = safety_name, state = $2,
			step = 'restoring the safety backup', updated_at = now(), worker_seal = $4
		WHERE id = $1 AND finished_at IS NULL AND state = $3 AND safety_name = $5`,
		id, StateRestoring, StateFailed, g.seal("restore", id, safety), safety)
	if err == nil && tag.RowsAffected() != 1 {
		err = errors.New("restore: no failed restore with a safety backup to go back to")
	}
	return err
}
