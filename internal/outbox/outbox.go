// SPDX-License-Identifier: Apache-2.0

// Package outbox implements the transactional outbox (docs/blueprint/04):
// events are written in the same transaction as the change, and a relay in the
// worker publishes them to NATS. No event is lost, and none is sent for a
// change that was rolled back.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/ids"
)

// SubjectPrefix is prepended to the event type to form the NATS subject.
const SubjectPrefix = "diffluence.events."

// MaxEventBytes stays below the 1 MiB that NATS takes in one message by default. An event
// holds ids, so only a fault makes one this large.
const MaxEventBytes = 900 << 10

// errTooLarge says why an event was set aside.
var errTooLarge = errors.New("the event is too large for the queue")

var typePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// Message is the JSON body published to NATS.
type Message struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// Enqueue stores an event inside tx.
func Enqueue(ctx context.Context, tx pgx.Tx, eventType string, payload any) (uuid.UUID, error) {
	if !typePattern.MatchString(eventType) {
		return uuid.Nil, fmt.Errorf("outbox: invalid event type %q", eventType)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, fmt.Errorf("outbox: encode payload: %w", err)
	}
	id := ids.New()
	if _, err := tx.Exec(ctx, "INSERT INTO outbox_events (id, type, payload) VALUES ($1, $2, $3)", id, eventType, body); err != nil {
		return uuid.Nil, fmt.Errorf("outbox: insert: %w", err)
	}
	return id, nil
}

// PurgePublished removes events that were published longer ago than keep.
func PurgePublished(ctx context.Context, pool *pgxpool.Pool, keep time.Duration) error {
	_, err := pool.Exec(ctx, "DELETE FROM outbox_events WHERE published_at < now() - make_interval(secs => $1)", keep.Seconds())
	return err
}

// Publisher sends one message. msgID is used for de-duplication, so a message
// that is published twice (after a crash between publish and commit) is stored once.
type Publisher interface {
	Publish(ctx context.Context, subject string, data []byte, msgID string) error
}

// Sequencer moves events into the event log of PostgreSQL in place of the publisher, once
// the events run through PostgreSQL (spec 068).
type Sequencer interface {
	// Takes reports inside tx whether the events go to the event log now. It holds the
	// events lock until tx ends, so one batch goes one way only.
	Takes(ctx context.Context, tx pgx.Tx) (bool, error)
	// Sequence moves one batch inside tx and returns how many events it moved.
	Sequence(ctx context.Context, tx pgx.Tx) (int, error)
}

// Relay moves events from the outbox table to the publisher.
type Relay struct {
	Pool      *pgxpool.Pool
	Publisher Publisher
	Log       *slog.Logger
	Interval  time.Duration // poll interval when idle (default 500ms)
	BatchSize int           // events per transaction (default 100)
	// Refused keeps an event the queue can never take, so that it is not lost and does
	// not stop the events behind it. Without it, such an event is only logged.
	Refused func(ctx context.Context, data []byte, cause error) error
	// Sequencer takes the batch when the events run through PostgreSQL. Nil = always the
	// publisher.
	Sequencer Sequencer
}

// setAside takes a message the queue would refuse out of the way.
func (r *Relay) setAside(ctx context.Context, m Message, body []byte) error {
	if r.Log != nil {
		r.Log.Error("outbox: an event is too large for the queue", "event_id", m.ID, "type", m.Type, "bytes", len(body))
	}
	if r.Refused == nil {
		return nil
	}
	return r.Refused(ctx, body, errTooLarge)
}

// RunOnce publishes one batch and returns how many events it published.
// Several relays may run at once: rows are claimed with SKIP LOCKED.
func (r *Relay) RunOnce(ctx context.Context) (int, error) {
	batch := r.BatchSize
	if batch <= 0 {
		batch = 100
	}
	n := 0
	err := pgx.BeginFunc(ctx, r.Pool, func(tx pgx.Tx) error {
		if r.Sequencer != nil {
			takes, err := r.Sequencer.Takes(ctx, tx)
			if err != nil {
				return err
			}
			if takes {
				n, err = r.Sequencer.Sequence(ctx, tx)
				return err
			}
		}
		rows, err := tx.Query(ctx, `SELECT id, type, payload, created_at FROM outbox_events
			WHERE published_at IS NULL ORDER BY created_at, id LIMIT $1 FOR UPDATE SKIP LOCKED`, batch)
		if err != nil {
			return err
		}
		msgs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
			var m Message
			var payload []byte
			err := row.Scan(&m.ID, &m.Type, &payload, &m.CreatedAt)
			m.Payload = payload
			return m, err
		})
		if err != nil {
			return err
		}
		for _, m := range msgs {
			body, err := json.Marshal(m)
			if err != nil {
				return err
			}
			if len(body) > MaxEventBytes {
				if err := r.setAside(ctx, m, body); err != nil {
					return fmt.Errorf("outbox: set aside %s: %w", m.ID, err)
				}
			} else if err := r.Publisher.Publish(ctx, SubjectPrefix+m.Type, body, m.ID.String()); err != nil {
				return fmt.Errorf("outbox: publish %s: %w", m.ID, err)
			}
			if _, err := tx.Exec(ctx, "UPDATE outbox_events SET published_at = now() WHERE id = $1", m.ID); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Run publishes until ctx is cancelled. Errors are logged and retried with backoff.
func (r *Relay) Run(ctx context.Context) {
	interval := r.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	log := r.Log
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	backoff := interval
	for {
		n, err := r.RunOnce(ctx)
		wait := interval
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			log.Error("outbox relay failed, retrying", "error", err, "wait", backoff)
			wait = backoff
			backoff = min(backoff*2, 30*time.Second)
		case n > 0:
			backoff = interval
			wait = 0 // there may be more
		default:
			backoff = interval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
