// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

const (
	// switchMigration is the migration of release A, part 1 (0053). A node whose newest
	// migration is at least this one relays under the events lock and starts the consumers
	// of the log once the transport is postgres, so it may run across the switch; an older
	// node publishes to NATS whatever the transport says.
	switchMigration = 53
	// nodeSeenWithin is how recently a node must have been seen to count as running (spec
	// 068, 2.11). A node that stops cleanly leaves the registry at once.
	nodeSeenWithin = 5 * time.Minute
)

// olderNodesSQL lists the running nodes of a release from before the switch.
const olderNodesSQL = `SELECT role, version, max_migration FROM nodes
	WHERE max_migration < $1 AND last_seen_at > now() - make_interval(secs => $2)
	ORDER BY started_at`

// anOlderNodeSQL is olderNodesSQL as a condition, with the same parameters.
const anOlderNodeSQL = `EXISTS (SELECT 1 FROM nodes
	WHERE max_migration < $1 AND last_seen_at > now() - make_interval(secs => $2))`

// OlderNode is a running node of a release from before the switch.
type OlderNode struct {
	Role         string
	Version      string
	MaxMigration int
}

// NodesBehindError says the switch to postgres waits until the nodes named have stopped
// or been upgraded.
type NodesBehindError struct {
	Nodes []OlderNode
}

func (e *NodesBehindError) Error() string {
	names := make([]string, len(e.Nodes))
	for i, n := range e.Nodes {
		names[i] = fmt.Sprintf("%s (role %s, migration %d)", n.Version, n.Role, n.MaxMigration)
	}
	return fmt.Sprintf("events: nodes of a release from before the switch still run: %s; upgrade or stop them first",
		strings.Join(names, ", "))
}

func olderNodes(ctx context.Context, q db.Querier) ([]OlderNode, error) {
	rows, err := q.Query(ctx, olderNodesSQL, switchMigration, nodeSeenWithin.Seconds())
	if err != nil {
		return nil, fmt.Errorf("events: read the nodes: %w", err)
	}
	nodes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (OlderNode, error) {
		var n OlderNode
		err := row.Scan(&n.Role, &n.Version, &n.MaxMigration)
		return n, err
	})
	if err != nil {
		return nil, fmt.Errorf("events: read the nodes: %w", err)
	}
	return nodes, nil
}

// Switch sets the path events take, on an operator's command (spec 068, 2.11). It takes the
// events lock, so every relay batch before it went the old way and every one after it goes
// the new way. To postgres it refuses while a node of a release from before the switch runs,
// and it creates the cursors of the consumers named at the head as it stands, so the events
// sequenced before a worker first looks are handed over too. Either way NATS has to drain
// anew: switching to postgres again is how an operator starts a new drain. It reports
// whether the transport changed.
// A transport that does not exist breaks the table's CHECK, and nothing changes.
func Switch(ctx context.Context, pool *pgxpool.Pool, to Transport, consumers []string) (bool, error) {
	changed := false
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
			return fmt.Errorf("events: take the events lock: %w", err)
		}
		if to == TransportPostgres {
			behind, err := olderNodes(ctx, tx)
			if err != nil {
				return err
			}
			if len(behind) > 0 {
				return &NodesBehindError{Nodes: behind}
			}
			if err := createCursors(ctx, tx, consumers); err != nil {
				return err
			}
		}
		// changed_at moves on every switch, also to the same transport, so a drain noted on
		// an answer from before it does not count (NoteDrained).
		err := tx.QueryRow(ctx, `UPDATE event_transport n SET transport = $1, changed_at = clock_timestamp(),
			nats_drained_at = NULL FROM (SELECT transport FROM event_transport) o RETURNING o.transport <> n.transport`,
			string(to)).Scan(&changed)
		if err != nil {
			return fmt.Errorf("events: switch to %q: %w", to, err)
		}
		return nil
	})
	return changed, err
}

// State is where the switch stands.
type State struct {
	Transport Transport
	// ChangedAt is when an operator last switched, or when the table was made.
	ChangedAt time.Time
	// DrainedAt is when a worker saw NATS empty after the switch; zero while it is not.
	DrainedAt time.Time
	// OlderNodes run a release from before the switch.
	OlderNodes []OlderNode
}

// ReadState reads the transport, the drain and the nodes that hold the switch up.
func ReadState(ctx context.Context, q db.Querier) (State, error) {
	var s State
	var drained *time.Time
	err := q.QueryRow(ctx, "SELECT transport, changed_at, nats_drained_at FROM event_transport").
		Scan(&s.Transport, &s.ChangedAt, &drained)
	if err != nil {
		return s, fmt.Errorf("events: read the transport: %w", err)
	}
	if drained != nil {
		s.DrainedAt = *drained
	}
	s.OlderNodes, err = olderNodes(ctx, q)
	return s, err
}

// Waiting reports how many events NATS still holds for its consumers: not yet delivered, or
// delivered and not yet acknowledged, which includes an event waiting for its retry.
type Waiting func(ctx context.Context) (int, error)

// NoteDrained notes when NATS has nothing left after the switch, which the release without
// NATS needs before it upgrades (spec 068, 2.11), and reports whether NATS counts as drained.
// A node of a release from before the switch may publish to NATS, so while one runs NATS is
// not drained, and a drain noted before it came back is undone. NATS is asked without the
// events lock; a switch meanwhile moves changed_at, and the answer then counts for nothing.
func NoteDrained(ctx context.Context, pool *pgxpool.Pool, waiting Waiting) (bool, error) {
	if _, err := pool.Exec(ctx, `UPDATE event_transport SET nats_drained_at = NULL
		WHERE nats_drained_at IS NOT NULL AND `+anOlderNodeSQL, switchMigration, nodeSeenWithin.Seconds()); err != nil {
		return false, fmt.Errorf("events: undo the drain: %w", err)
	}
	s, err := ReadState(ctx, pool)
	switch {
	case err != nil:
		return false, err
	case s.Transport != TransportPostgres || len(s.OlderNodes) > 0:
		return false, nil
	case !s.DrainedAt.IsZero():
		return true, nil
	}
	n, err := waiting(ctx)
	if err != nil {
		return false, fmt.Errorf("events: ask NATS what it holds: %w", err)
	}
	if n > 0 {
		return false, nil
	}
	// Every switch moves changed_at, so an unchanged one means the transport read above.
	tag, err := pool.Exec(ctx, `UPDATE event_transport SET nats_drained_at = now()
		WHERE changed_at = $3 AND NOT `+anOlderNodeSQL,
		switchMigration, nodeSeenWithin.Seconds(), s.ChangedAt)
	if err != nil {
		return false, fmt.Errorf("events: note the drain: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// ConsumersNeeded reports whether this worker must run the consumers of the log: while the
// events run through PostgreSQL, and after a switch back to NATS for as long as the log holds
// an event a cursor has not passed or a retry waits.
func ConsumersNeeded(ctx context.Context, q db.Querier) (bool, error) {
	var needed bool
	err := q.QueryRow(ctx, `SELECT (SELECT transport = 'postgres' FROM event_transport)
		OR EXISTS (SELECT 1 FROM event_retries)
		OR EXISTS (SELECT 1 FROM event_cursors c
			WHERE EXISTS (SELECT 1 FROM event_log l WHERE l.lane = c.lane AND l.pos > c.pos))`).Scan(&needed)
	if err != nil {
		return false, fmt.Errorf("events: read whether the consumers are needed: %w", err)
	}
	return needed, nil
}

// NATSRemovalProblem is the check of the release without NATS (release B) before it
// migrates: it names what still needs NATS and what to do about it, or returns "" when nothing
// does (spec 068, 2.11). It reads the database as any earlier release leaves it.
func NATSRemovalProblem(ctx context.Context, q db.Querier) (string, error) {
	const upgradeFirst = "this database comes from a release that still needs NATS: upgrade to the release that switches the events to PostgreSQL first, run diffluence events switch postgres, and wait until diffluence events status says NATS is drained"
	canSwitch, err := hasTheSwitch(ctx, q)
	if err != nil || !canSwitch {
		return upgradeFirst, err
	}
	s, err := ReadState(ctx, q)
	if err != nil {
		return "", err
	}
	switch {
	case s.Transport != TransportPostgres:
		return "the events still go through NATS: run diffluence events switch postgres, then wait until diffluence events status says NATS is drained", nil
	case len(s.OlderNodes) > 0:
		return (&NodesBehindError{Nodes: s.OlderNodes}).Error() + ": a node of an older release may still publish to NATS", nil
	case s.DrainedAt.IsZero():
		return "NATS may still hold events from before the switch: wait until diffluence events status says NATS is drained, up to about 1.5 hours while a retry waits", nil
	}
	// Once, before an upgrade: no index serves it, and none is worth its cost on every save.
	var late bool
	if err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM outbox_events WHERE published_at >= $1)", s.DrainedAt).Scan(&late); err != nil {
		return "", fmt.Errorf("events: look for events published after the drain: %w", err)
	}
	if late {
		return "a node of an older release published events to NATS after it was drained: stop that node, run diffluence events switch postgres again, and wait until diffluence events status says NATS is drained", nil
	}
	return "", nil
}

// hasTheSwitch reports whether the database has the switch and the drain of this release.
func hasTheSwitch(ctx context.Context, q db.Querier) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'event_transport' AND column_name = 'nats_drained_at')`).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("events: read whether the database can switch: %w", err)
	}
	return ok, nil
}
