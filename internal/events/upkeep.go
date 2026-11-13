// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
)

const (
	// keepFor is how long event_log keeps an event, as the stream and the published outbox
	// rows did (D10).
	keepFor = 7 * 24 * time.Hour
	// cleanupBatch is how many events one delete removes at most (D10).
	cleanupBatch = 10_000
	// waitingTooLong is how long an event may wait in the outbox while a worker runs (L3).
	waitingTooLong = time.Minute
)

// cleanupSQL deletes the oldest events up to a bound: below every cursor, below every retry,
// before the first event younger than $1 seconds, and at most $2 events. Only a run from the
// start goes, so the log never gets a hole (L1). Without any cursor nothing goes: nobody has
// read anything yet.
const cleanupSQL = `DELETE FROM event_log WHERE pos <= (
	SELECT least(
		(SELECT min(pos) FROM event_cursors),
		(SELECT min(pos) - 1 FROM event_retries),
		(SELECT pos - 1 FROM event_log WHERE sequenced_at >= now() - make_interval(secs => $1) ORDER BY pos LIMIT 1),
		(SELECT min(pos) + $2 - 1 FROM event_log))
	WHERE EXISTS (SELECT 1 FROM event_cursors))`

// Cleanup deletes the events every consumer is done with after seven days, in batches. It
// returns how many it deleted.
func Cleanup(ctx context.Context, q db.Querier) (int64, error) {
	return cleanup(ctx, q, keepFor, cleanupBatch)
}

func cleanup(ctx context.Context, q db.Querier, keep time.Duration, batch int) (int64, error) {
	var total int64
	for {
		tag, err := q.Exec(ctx, cleanupSQL, keep.Seconds(), batch)
		if err != nil {
			return total, fmt.Errorf("events: clean up the log: %w", err)
		}
		total += tag.RowsAffected()
		if tag.RowsAffected() < int64(batch) {
			return total, nil
		}
	}
}

// Breach is the first rule of spec 068 (2.10) the event tables break.
type Breach struct {
	Rule   string // L1, L2 or L3
	Detail string
}

func (b *Breach) String() string { return b.Rule + ": " + b.Detail }

// Check runs the ledger rules L1 to L3 of spec 068 and returns the first breach, or nil.
// Each rule reads in one statement, so a sequencer running meanwhile cannot fake a breach.
func Check(ctx context.Context, q db.Querier) (*Breach, error) {
	for _, rule := range []func(context.Context, db.Querier) (*Breach, error){checkGapless, checkCursors, checkRetries, checkWaiting} {
		breach, err := rule(ctx, q)
		if err != nil || breach != nil {
			return breach, err
		}
	}
	return nil, nil
}

// checkGapless is L1: the positions from the oldest kept event to the head have no gap.
func checkGapless(ctx context.Context, q db.Querier) (*Breach, error) {
	var head, count, lowest, highest int64
	err := q.QueryRow(ctx, `SELECT (SELECT pos FROM event_head), count(*), coalesce(min(pos), 0), coalesce(max(pos), 0)
		FROM event_log`).Scan(&head, &count, &lowest, &highest)
	if err != nil {
		return nil, fmt.Errorf("events: check the positions: %w", err)
	}
	switch {
	case count == 0:
		return nil, nil
	case highest != head:
		return &Breach{"L1", fmt.Sprintf("the head is at %d but the log ends at %d", head, highest)}, nil
	case count != highest-lowest+1:
		return &Breach{"L1", fmt.Sprintf("%d positions are missing between %d and %d", highest-lowest+1-count, lowest, highest)}, nil
	}
	return nil, nil
}

// checkCursors is the first half of L2: no cursor is past the head.
func checkCursors(ctx context.Context, q db.Querier) (*Breach, error) {
	var consumer string
	var lane int16
	var pos, head int64
	err := q.QueryRow(ctx, `SELECT c.consumer, c.lane, c.pos, h.pos FROM event_cursors c, event_head h
		WHERE c.pos > h.pos ORDER BY c.consumer, c.lane LIMIT 1`).Scan(&consumer, &lane, &pos, &head)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("events: check the cursors: %w", err)
	}
	return &Breach{"L2", fmt.Sprintf("the cursor of %s lane %d is at %d, past the head at %d", consumer, lane, pos, head)}, nil
}

// checkRetries is the second half of L2: every retry names a kept event at or below its
// lane's cursor, so the cursor never passed an event that is neither done nor kept.
func checkRetries(ctx context.Context, q db.Querier) (*Breach, error) {
	var consumer string
	var pos int64
	err := q.QueryRow(ctx, `SELECT r.consumer, r.pos FROM event_retries r
		LEFT JOIN event_log l ON l.pos = r.pos
		LEFT JOIN event_cursors c ON c.consumer = r.consumer AND c.lane = l.lane
		WHERE c.pos IS NULL OR l.id <> r.event_id OR r.pos > c.pos
		ORDER BY r.consumer, r.pos LIMIT 1`).Scan(&consumer, &pos)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("events: check the retries: %w", err)
	}
	return &Breach{"L2", fmt.Sprintf("the retry of %s at %d has no event at or below its cursor", consumer, pos)}, nil
}

// checkWaiting is L3: no event waits in the outbox for more than a minute while a worker
// has run for longer than that.
func checkWaiting(ctx context.Context, q db.Querier) (*Breach, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM outbox_events
		WHERE published_at IS NULL AND created_at < now() - make_interval(secs => $1)
		AND EXISTS (SELECT 1 FROM nodes WHERE role IN ('all', 'worker')
			AND last_seen_at > now() - make_interval(secs => $1) AND started_at < now() - make_interval(secs => $1))`,
		waitingTooLong.Seconds()).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("events: check the outbox: %w", err)
	}
	if count > 0 {
		return &Breach{"L3", fmt.Sprintf("%d events have waited over a minute in the outbox while a worker runs", count)}, nil
	}
	return nil, nil
}

// ConsumerState is how far one consumer is behind.
type ConsumerState struct {
	Consumer string
	// Lag is the age of the oldest event past the consumer's cursors; zero when it is at
	// the head.
	Lag time.Duration
	// Retries counts its events waiting for a retry.
	Retries int64
}

// ReadConsumers reads the state of every consumer that has cursors (spec 068, 2.9).
func ReadConsumers(ctx context.Context, q db.Querier) ([]ConsumerState, error) {
	rows, err := q.Query(ctx, `SELECT c.consumer,
			coalesce(extract(epoch FROM now() - min(w.sequenced_at)), 0)::float8,
			(SELECT count(*) FROM event_retries r WHERE r.consumer = c.consumer)
		FROM event_cursors c
		LEFT JOIN LATERAL (SELECT sequenced_at FROM event_log l WHERE l.lane = c.lane AND l.pos > c.pos
			ORDER BY l.pos LIMIT 1) w ON true
		GROUP BY c.consumer ORDER BY c.consumer`)
	if err != nil {
		return nil, fmt.Errorf("events: read the consumers: %w", err)
	}
	states, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ConsumerState, error) {
		var s ConsumerState
		var seconds float64
		err := row.Scan(&s.Consumer, &seconds, &s.Retries)
		s.Lag = time.Duration(seconds * float64(time.Second))
		return s, err
	})
	if err != nil {
		return nil, fmt.Errorf("events: read the consumers: %w", err)
	}
	return states, nil
}
