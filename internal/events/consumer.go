// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/failures"
)

const (
	// readBatch is how many events one pass of a lane reads (D3).
	readBatch = 100
	// leaseFor is how long a lane stays with a worker that stopped renewing it (D3).
	leaseFor = time.Minute
	// renewEvery keeps a live worker's leases far from their end.
	renewEvery = 20 * time.Second
	// defaultPoll is the floor under the wake-ups: a lost notification costs at most this
	// much latency (D6).
	defaultPoll = time.Second
	// errorWait is the pause after a pass that failed, such as while the database restarts.
	errorWait = 5 * time.Second
	// releaseWait bounds giving the leases back when a worker stops.
	releaseWait = 5 * time.Second
)

// Runner runs the consumers of the event log on one worker: one loop per consumer and lane,
// and one retry loop per consumer. Any number of workers may run one; the leases share
// the lanes out.
type Runner struct {
	Pool      *pgxpool.Pool
	Node      uuid.UUID // holds the leases; unique per running worker
	Consumers []Consumer
	// GiveUp stores an event after its last try, inside the transaction that removes it
	// from the retries (failures.RecordIn).
	GiveUp func(ctx context.Context, tx pgx.Tx, consumer string, data []byte, cause error) error
	Log    *slog.Logger
	// Wake carries the notifications of the sequencer; nil = polling only.
	Wake *Wakeups
	// Poll is how long a loop waits without a wake-up; 0 = one second.
	Poll time.Duration
}

// lane is one consumer's lane as this worker knows it.
type lane struct {
	consumer Consumer
	number   int16
	// held says this worker holds the lease; pos is then the cursor, which only the holder
	// moves.
	held      bool
	pos       int64
	renewedAt time.Time
}

// lanes are the lanes of every consumer, none held yet.
func (r *Runner) lanes() []*lane {
	out := make([]*lane, 0, len(r.Consumers)*Lanes)
	for _, c := range r.Consumers {
		for n := range Lanes {
			out = append(out, &lane{consumer: c, number: int16(n)})
		}
	}
	return out
}

// EnsureCursors gives every consumer a cursor per lane, at the head: a consumer starts with
// the events that come after it, as the switch of spec 068 (2.11) needs. A cursor that
// exists stays where it is.
func (r *Runner) EnsureCursors(ctx context.Context) error {
	for _, c := range r.Consumers {
		_, err := r.Pool.Exec(ctx, `INSERT INTO event_cursors (consumer, lane, pos)
			SELECT $1, lane, (SELECT pos FROM event_head) FROM generate_series(0, $2 - 1) AS lane
			ON CONFLICT (consumer, lane) DO NOTHING`, c.Name, Lanes)
		if err != nil {
			return fmt.Errorf("events: create the cursors of %s: %w", c.Name, err)
		}
	}
	return nil
}

// takeLease takes or renews the lease of a lane, unless another worker holds a lease that
// has not ended. It reads the cursor, which is this worker's to move from now on.
func (r *Runner) takeLease(ctx context.Context, l *lane) error {
	err := r.Pool.QueryRow(ctx, `UPDATE event_cursors SET lease_owner = $3, lease_until = now() + make_interval(secs => $4)
		WHERE consumer = $1 AND lane = $2 AND (lease_until IS NULL OR lease_until < now() OR lease_owner = $3)
		RETURNING pos`, l.consumer.Name, l.number, r.Node, leaseFor.Seconds()).Scan(&l.pos)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		l.held = false
		return nil
	case err != nil:
		l.held = false
		return fmt.Errorf("events: take the lease of %s lane %d: %w", l.consumer.Name, l.number, err)
	}
	l.held, l.renewedAt = true, time.Now()
	return nil
}

// step runs one pass of a lane: take or renew its lease when due, hand the next batch to the
// consumer, and move the cursor past it. It returns how many events it handed over.
func (r *Runner) step(ctx context.Context, l *lane) (int, error) {
	if !l.held || time.Since(l.renewedAt) >= renewEvery {
		if err := r.takeLease(ctx, l); err != nil || !l.held {
			return 0, err
		}
	}
	rows, err := r.Pool.Query(ctx, `SELECT pos, id, type, payload, created_at FROM event_log
		WHERE lane = $1 AND pos > $2 ORDER BY pos LIMIT $3`, l.number, l.pos, readBatch)
	if err != nil {
		return 0, fmt.Errorf("events: read lane %d: %w", l.number, err)
	}
	batch, err := pgx.CollectRows(rows, scanEvent)
	if err != nil {
		return 0, fmt.Errorf("events: read lane %d: %w", l.number, err)
	}
	if len(batch) == 0 {
		return 0, nil
	}
	// No transaction is open while the handlers run, so a slow one holds no lock and no
	// connection (D3).
	failed := map[int64]error{}
	for _, e := range batch {
		// A batch of slow handlers can outlast a lease, so it is renewed as the batch goes.
		if time.Since(l.renewedAt) >= renewEvery {
			if err := r.takeLease(ctx, l); err != nil {
				return 0, err
			}
			if !l.held {
				return 0, errLeaseLost
			}
		}
		if cause := handle(ctx, l.consumer.Handle, e); cause != nil {
			if ctx.Err() != nil {
				// A stopping worker did not finish the batch; the next holder runs it.
				return 0, ctx.Err()
			}
			failed[e.pos] = cause
		}
	}
	last := batch[len(batch)-1].pos
	if err := r.commitBatch(ctx, l, batch, failed, last); err != nil {
		if errors.Is(err, errLeaseLost) {
			l.held = false
		}
		return 0, err
	}
	l.pos = last
	return len(batch), nil
}

// commitBatch keeps the failures for a retry and moves the cursor, in one transaction. If
// this worker lost the lease meanwhile, nothing is kept: the new holder runs the batch again.
func (r *Runner) commitBatch(ctx context.Context, l *lane, batch []event, failed map[int64]error, last int64) error {
	return pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		// A cursor commit lost in a crash only moves the cursor back, so events repeat and
		// none is skipped; it need not wait for the disk (D4).
		if _, err := tx.Exec(ctx, "SET LOCAL synchronous_commit = off"); err != nil {
			return err
		}
		for _, e := range batch {
			cause, ok := failed[e.pos]
			if !ok {
				continue
			}
			if _, err := tx.Exec(ctx, `INSERT INTO event_retries (consumer, pos, event_id, tries, next_at, last_error)
				VALUES ($1, $2, $3, 1, now() + make_interval(secs => $4), $5) ON CONFLICT (consumer, pos) DO NOTHING`,
				l.consumer.Name, e.pos, e.msg.ID, retryWaits[0].Seconds(), failures.ShortError(cause)); err != nil {
				return fmt.Errorf("events: keep a failed event: %w", err)
			}
		}
		tag, err := tx.Exec(ctx, "UPDATE event_cursors SET pos = $3 WHERE consumer = $1 AND lane = $2 AND lease_owner = $4",
			l.consumer.Name, l.number, last, r.Node)
		if err != nil {
			return fmt.Errorf("events: move the cursor: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return errLeaseLost
		}
		return nil
	})
}

// release gives a lane's lease back, so another worker takes it at once.
func (r *Runner) release(ctx context.Context, l *lane) error {
	if !l.held {
		return nil
	}
	l.held = false
	_, err := r.Pool.Exec(ctx, `UPDATE event_cursors SET lease_owner = NULL, lease_until = NULL
		WHERE consumer = $1 AND lane = $2 AND lease_owner = $3`, l.consumer.Name, l.number, r.Node)
	return err
}

// releaseAll gives every lease back; a lease it cannot give back ends by itself in a minute.
func (r *Runner) releaseAll(lanes []*lane) {
	ctx, cancel := context.WithTimeout(context.Background(), releaseWait)
	defer cancel()
	for _, l := range lanes {
		if err := r.release(ctx, l); err != nil {
			r.Log.Warn("events: a lease was not given back; it ends by itself", "consumer", l.consumer.Name, "lane", l.number, "error", err)
		}
	}
}

// Drain hands over everything that waits in every lane, then gives the leases back. Tests
// and the measurement use it to stay in step; a worker uses Run.
func (r *Runner) Drain(ctx context.Context) (int, error) {
	if err := r.EnsureCursors(ctx); err != nil {
		return 0, err
	}
	lanes := r.lanes()
	defer r.releaseAll(lanes)
	total := 0
	for {
		round := 0
		for _, l := range lanes {
			n, err := r.step(ctx, l)
			if err != nil {
				return total + round, err
			}
			round += n
		}
		if round == 0 {
			return total, nil
		}
		total += round
	}
}

// Run runs every lane and retry loop until ctx ends, then gives the leases back.
func (r *Runner) Run(ctx context.Context) {
	for {
		err := r.EnsureCursors(ctx)
		if err == nil {
			break
		}
		r.Log.Error("events: the consumers cannot start; trying again", "error", err)
		if !sleep(ctx, errorWait) {
			return
		}
	}
	lanes := r.lanes()
	var wg sync.WaitGroup
	for _, l := range lanes {
		wg.Go(func() {
			r.loop(ctx, "lane", func(ctx context.Context) (int, error) { return r.step(ctx, l) }, readBatch)
		})
	}
	for _, c := range r.Consumers {
		wg.Go(func() {
			r.loop(ctx, "retries", func(ctx context.Context) (int, error) { return r.retryStep(ctx, c) }, retryBatch)
		})
	}
	wg.Wait()
	r.releaseAll(lanes)
}

// loop runs pass after pass: at once after a full one, else on a wake-up or after the poll.
func (r *Runner) loop(ctx context.Context, what string, pass func(context.Context) (int, error), full int) {
	var wake <-chan struct{}
	if r.Wake != nil {
		wake = r.Wake.Subscribe()
		defer r.Wake.Unsubscribe(wake)
	}
	poll := r.Poll
	if poll <= 0 {
		poll = defaultPoll
	}
	for ctx.Err() == nil {
		n, err := pass(ctx)
		wait := poll
		switch {
		case ctx.Err() != nil:
			return
		case errors.Is(err, errLeaseLost):
			r.Log.Warn("events: another worker took a lane; it runs the batch again", "loop", what)
			continue
		case err != nil:
			r.Log.Error("events: a pass failed; trying again", "loop", what, "error", err)
			wait = errorWait
		case n >= full:
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-time.After(wait):
		}
	}
}

// sleep waits d, or less when ctx ends; it reports whether ctx is still live.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
