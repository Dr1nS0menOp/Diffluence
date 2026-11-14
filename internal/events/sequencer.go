// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// defaultSequenceBatch is how many events one sequencer transaction moves at most (D1).
const defaultSequenceBatch = 500

// Sequencer moves committed events from outbox_events into event_log, each with the next
// gapless position. It runs inside the relay's transaction (outbox.Sequencer), so the relay
// and the sequencer are one loop under one lock.
type Sequencer struct {
	BatchSize int // events per transaction; 0 = 500
}

// Takes takes the events lock for the rest of tx and reports whether the events run
// through PostgreSQL. Under the lock the next batch sees this one committed, and every row
// it takes became visible later, so positions follow visibility (D1).
func (s Sequencer) Takes(ctx context.Context, tx pgx.Tx) (bool, error) {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
		return false, fmt.Errorf("events: take the events lock: %w", err)
	}
	t, err := CurrentTransport(ctx, tx)
	if err != nil {
		return false, err
	}
	return t == TransportPostgres, nil
}

// laneSQL puts an event in a lane by the object it is about (D5): events of one page stay
// in one lane, so they keep their order. hashtext gives an int4, and abs of its lowest value
// does not fit one, hence the bigint.
const laneSQL = `(abs(hashtext(coalesce(o.payload->>'page_id', o.payload->>'space_id', o.payload->>'user_id',
	o.payload->>'group_id', o.id::text))::bigint) % 4)::smallint`

// Sequence moves the oldest waiting events into event_log, under the lock Takes took. It
// returns how many it moved.
func (s Sequencer) Sequence(ctx context.Context, tx pgx.Tx) (int, error) {
	batch := s.BatchSize
	if batch <= 0 {
		batch = defaultSequenceBatch
	}
	// No SKIP LOCKED: a row a relay of the previous release still holds is waited for, and
	// seen as published once that relay commits, so it goes one way only.
	rows, err := tx.Query(ctx, `SELECT id FROM outbox_events WHERE published_at IS NULL
		ORDER BY created_at, id LIMIT $1 FOR UPDATE`, batch)
	if err != nil {
		return 0, fmt.Errorf("events: read the outbox: %w", err)
	}
	waiting, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, fmt.Errorf("events: read the outbox: %w", err)
	}
	n := len(waiting)
	if n == 0 {
		return 0, nil
	}
	// One row, so positions are transactional and have no gaps; a sequence is neither.
	var head int64
	if err := tx.QueryRow(ctx, "UPDATE event_head SET pos = pos + $1 RETURNING pos", n).Scan(&head); err != nil {
		return 0, fmt.Errorf("events: move the head: %w", err)
	}
	tag, err := tx.Exec(ctx, `INSERT INTO event_log (pos, lane, id, type, payload, created_at, trace)
		SELECT $2::bigint + w.ord, `+laneSQL+`, o.id, o.type, o.payload, o.created_at, o.trace
		FROM unnest($1::uuid[]) WITH ORDINALITY AS w(id, ord) JOIN outbox_events o ON o.id = w.id`,
		waiting, head-int64(n))
	if err != nil {
		return 0, fmt.Errorf("events: write the log: %w", err)
	}
	// Fewer rows than positions would leave a hole; the transaction rolls back instead.
	if tag.RowsAffected() != int64(n) {
		return 0, fmt.Errorf("events: wrote %d events for %d positions", tag.RowsAffected(), n)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM outbox_events WHERE id = ANY($1)", waiting); err != nil {
		return 0, fmt.Errorf("events: empty the outbox: %w", err)
	}
	// A hint for the consumers; it reaches them only once this commits (D6, D7).
	if _, err := tx.Exec(ctx, "SELECT pg_notify($1, $2)", Channel, strconv.FormatInt(head, 10)); err != nil {
		return 0, fmt.Errorf("events: notify: %w", err)
	}
	return n, nil
}
