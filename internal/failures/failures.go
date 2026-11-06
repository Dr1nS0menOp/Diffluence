// SPDX-License-Identifier: Apache-2.0

// Package failures keeps the events a listener gave up on, so that one bad event
// cannot come back for ever, and is not lost either (ARC-050).
package failures

import (
	"context"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
)

const (
	// keptFor is how long a failure stays. The stream forgets an event after 7 days,
	// so a month leaves an administrator time to look.
	keptFor = 30 * 24 * time.Hour
	// maxErrorBytes keeps a long error from filling the table.
	maxErrorBytes = 500
)

// Failure is one event a listener gave up on. The payload stays out of the list.
type Failure struct {
	ID              uuid.UUID  `json:"id"`
	Consumer        string     `json:"consumer"`
	Error           string     `json:"error"`
	FailedAt        time.Time  `json:"failed_at"`
	ReplayRequested *time.Time `json:"replay_requested_at"`
}

// Handler takes one event, as the queue hands it to a listener.
type Handler func(ctx context.Context, data []byte) error

// Service stores and replays failures.
type Service struct {
	pool *pgxpool.Pool
	log  *slog.Logger
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, log: log}
}

func shortened(message string) string {
	if len(message) <= maxErrorBytes {
		return message
	}
	cut := maxErrorBytes
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	return message[:cut]
}

// Record stores an event its listener gave up on.
func (s *Service) Record(ctx context.Context, consumer string, data []byte, cause error) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO event_failures (id, consumer, payload, error) VALUES ($1, $2, $3, $4)",
		ids.New(), consumer, data, shortened(cause.Error()))
	return err
}

// List returns what still waits, newest first.
func (s *Service) List(ctx context.Context) ([]Failure, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, consumer, error, failed_at, replay_requested_at FROM event_failures
		WHERE replayed_at IS NULL ORDER BY failed_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Failure, error) {
		var f Failure
		err := r.Scan(&f.ID, &f.Consumer, &f.Error, &f.FailedAt, &f.ReplayRequested)
		return f, err
	})
	if out == nil {
		out = []Failure{}
	}
	return out, err
}

// RequestReplay asks the worker to try an event again. The route requires an
// administrator.
func (s *Service) RequestReplay(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE event_failures SET replay_requested_at = now() WHERE id = $1 AND replayed_at IS NULL", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		if _, err := audit.Record(ctx, tx, c.Event("event.replay_requested", "event_failure", id.String(), "", nil)); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "event.replay_requested", map[string]any{"failure_id": id})
		return err
	})
}

type waitingReplay struct {
	id       uuid.UUID
	consumer string
	payload  []byte
}

// RunReplays hands requested events to their listeners. It is a task, because only a
// worker has the listeners. An event that fails again stays, with its new error, and
// waits for the next request.
func (s *Service) RunReplays(ctx context.Context, handlers map[string]Handler) error {
	rows, err := s.pool.Query(ctx, `SELECT id, consumer, payload FROM event_failures
		WHERE replay_requested_at IS NOT NULL AND replayed_at IS NULL ORDER BY failed_at LIMIT 100`)
	if err != nil {
		return err
	}
	waiting, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (waitingReplay, error) {
		var w waitingReplay
		err := r.Scan(&w.id, &w.consumer, &w.payload)
		return w, err
	})
	if err != nil {
		return err
	}
	for _, w := range waiting {
		if err := s.replay(ctx, w, handlers[w.consumer]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) replay(ctx context.Context, w waitingReplay, handle Handler) error {
	if handle == nil {
		_, err := s.pool.Exec(ctx, "UPDATE event_failures SET replay_requested_at = NULL, error = 'This listener no longer exists.' WHERE id = $1", w.id)
		return err
	}
	if cause := handle(ctx, w.payload); cause != nil {
		s.log.Warn("event replay failed", "failure", w.id, "consumer", w.consumer, "error", cause)
		_, err := s.pool.Exec(ctx, "UPDATE event_failures SET replay_requested_at = NULL, error = $2 WHERE id = $1", w.id, shortened(cause.Error()))
		return err
	}
	_, err := s.pool.Exec(ctx, "UPDATE event_failures SET replayed_at = now() WHERE id = $1", w.id)
	return err
}

// Purge removes old failures.
func (s *Service) Purge(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM event_failures WHERE failed_at < now() - make_interval(secs => $1)", keptFor.Seconds())
	return err
}
