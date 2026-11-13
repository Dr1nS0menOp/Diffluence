// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/failures"
)

// retryWaits are the pauses before each retry: the first after the event failed in its
// lane, the next after each failed retry. They are the waits NATS used (ARC-050), so an
// event runs six times before it goes to event_failures.
var retryWaits = []time.Duration{10 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}

const (
	// retryBatch is how many due retries one pass takes.
	retryBatch = 10
	// retryClaim keeps a retry from other workers while one runs it. It outlasts the
	// handler's minute, and a worker that dies meanwhile only delays the retry.
	retryClaim = 2 * time.Minute
)

// retry is a due retry with its event.
type retry struct {
	event
	tries int
}

// retryStep runs the due retries of one consumer and returns how many it ran. No
// transaction stays open while a handler runs: a retry is claimed by moving its time on.
func (r *Runner) retryStep(ctx context.Context, c Consumer) (int, error) {
	rows, err := r.Pool.Query(ctx, `WITH due AS (
			SELECT consumer, pos FROM event_retries WHERE consumer = $1 AND next_at <= now()
			ORDER BY next_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE event_retries t SET next_at = now() + make_interval(secs => $3)
		FROM due JOIN event_log l ON l.pos = due.pos
		WHERE t.consumer = due.consumer AND t.pos = due.pos
		RETURNING l.pos, l.id, l.type, l.payload, l.created_at, t.tries`, c.Name, retryBatch, retryClaim.Seconds())
	if err != nil {
		return 0, fmt.Errorf("events: claim the retries of %s: %w", c.Name, err)
	}
	due, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (retry, error) {
		var t retry
		var payload []byte
		err := row.Scan(&t.pos, &t.msg.ID, &t.msg.Type, &payload, &t.msg.CreatedAt, &t.tries)
		t.msg.Payload = payload
		return t, err
	})
	if err != nil {
		return 0, fmt.Errorf("events: claim the retries of %s: %w", c.Name, err)
	}
	for _, t := range due {
		if err := r.runRetry(ctx, c, t); err != nil {
			return 0, err
		}
	}
	return len(due), nil
}

// runRetry runs one retry and records what became of it.
func (r *Runner) runRetry(ctx context.Context, c Consumer, t retry) error {
	cause := handle(ctx, c.Handle, t.event)
	switch {
	case cause == nil:
		_, err := r.Pool.Exec(ctx, "DELETE FROM event_retries WHERE consumer = $1 AND pos = $2", c.Name, t.pos)
		return err
	case ctx.Err() != nil:
		// A stopping worker: the claim ends and the retry runs again with the same try.
		return ctx.Err()
	case t.tries < len(retryWaits):
		_, err := r.Pool.Exec(ctx, `UPDATE event_retries SET tries = tries + 1, next_at = now() + make_interval(secs => $3),
			last_error = $4 WHERE consumer = $1 AND pos = $2`, c.Name, t.pos, retryWaits[t.tries].Seconds(), failures.ShortError(cause))
		return err
	}
	data, err := t.body()
	if err != nil {
		return err
	}
	err = pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		if err := r.GiveUp(ctx, tx, c.Name, data, cause); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM event_retries WHERE consumer = $1 AND pos = $2", c.Name, t.pos)
		return err
	})
	if err != nil {
		// The retry stays and comes back when its claim ends.
		return fmt.Errorf("events: store a failed event of %s: %w", c.Name, err)
	}
	r.Log.Warn("events: gave up on an event", "consumer", c.Name, "event_id", t.msg.ID, "tries", t.tries+1, "error", cause)
	return nil
}
