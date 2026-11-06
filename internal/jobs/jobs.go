// SPDX-License-Identifier: Apache-2.0

// Package jobs runs background work in the worker role: queued jobs (imports)
// and periodic tasks. Both are coordinated through PostgreSQL, so they behave
// the same with one worker or many, and need no extra moving parts.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
)

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job is one unit of queued work.
type Job struct {
	ID         uuid.UUID       `json:"id"`
	Kind       string          `json:"kind"`
	Status     string          `json:"status"`
	Payload    json.RawMessage `json:"-"`
	Result     json.RawMessage `json:"result"`
	CreatedBy  uuid.UUID       `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

// Handler does the work of one job kind. Its result is stored for the user.
type Handler func(ctx context.Context, j Job) (any, error)

// Task is periodic work.
type Task struct {
	Name  string
	Every time.Duration
	Run   func(ctx context.Context) error
}

// Service queues and runs jobs.
type Service struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	handlers map[string]Handler
	timeout  time.Duration
	held     func(context.Context) (bool, error)
	// longKinds are the kinds the slot for short jobs leaves alone.
	longKinds []string
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{pool: pool, log: log, handlers: map[string]Handler{}, timeout: time.Hour}
}

// Register sets the handler for a job kind.
func (s *Service) Register(kind string, h Handler) { s.handlers[kind] = h }

// RegisterLong adds a kind of job that can take minutes. It never gets the slot kept
// for short jobs, so a large export cannot delay a small copy (ARC-063).
func (s *Service) RegisterLong(kind string, h Handler) {
	s.handlers[kind] = h
	s.longKinds = append(s.longKinds, kind)
}

// Enqueue adds a job.
func (s *Service) Enqueue(ctx context.Context, tx pgx.Tx, kind string, payload any, createdBy uuid.UUID) (uuid.UUID, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, err
	}
	id := ids.New()
	_, err = tx.Exec(ctx, "INSERT INTO jobs (id, kind, status, payload, created_by) VALUES ($1, $2, 'queued', $3, $4)", id, kind, body, createdBy)
	return id, err
}

// Get returns a job to the user who started it.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (Job, error) {
	var j Job
	err := s.pool.QueryRow(ctx, `SELECT id, kind, status, result, created_by, created_at, finished_at FROM jobs
		WHERE id = $1 AND created_by = $2`, id, userID).
		Scan(&j.ID, &j.Kind, &j.Status, &j.Result, &j.CreatedBy, &j.CreatedAt, &j.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return j, apperr.NotFound()
	}
	return j, err
}

// Recent lists the latest jobs for the console. Payloads stay out: they can hold
// details of what someone was doing.
func (s *Service) Recent(ctx context.Context) ([]Job, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, kind, status, result, created_by, created_at, finished_at FROM jobs
		ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Job, error) {
		var j Job
		err := r.Scan(&j.ID, &j.Kind, &j.Status, &j.Result, &j.CreatedBy, &j.CreatedAt, &j.FinishedAt)
		return j, err
	})
	if out == nil {
		out = []Job{}
	}
	return out, err
}

func (s *Service) changeJob(ctx context.Context, c identity.Caller, id uuid.UUID, change jobChange) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE jobs SET status = $3, result = '{}', finished_at = NULL WHERE id = $1 AND status = $2", id, change.from, change.to)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("wrong_status", "This job is not "+change.from+".")
		}
		if _, err := audit.Record(ctx, tx, c.Event(change.action, "job", id.String(), "", nil)); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, change.action, map[string]any{"job_id": id})
		return err
	})
}

type jobChange struct {
	from, to, action string
}

// Retry puts a failed job back in the queue. The route requires administrator.
func (s *Service) Retry(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.changeJob(ctx, c, id, jobChange{from: StatusFailed, to: StatusQueued, action: "job.retried"})
}

// Cancel ends a job that has not started. A running job cannot be stopped yet.
func (s *Service) Cancel(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return s.changeJob(ctx, c, id, jobChange{from: StatusQueued, to: StatusFailed, action: "job.cancelled"})
}

// HoldWhile makes queued jobs wait for as long as the check says so. Maintenance mode
// uses it: a job may change what an operator is backing up (spec 013, F11).
func (s *Service) HoldWhile(check func(context.Context) (bool, error)) { s.held = check }

// RunOnce claims and runs one queued job. It reports whether there was one.
func (s *Service) RunOnce(ctx context.Context) (bool, error) { return s.runNext(ctx, nil) }

// runNext runs the oldest queued job that is not of a skipped kind.
func (s *Service) runNext(ctx context.Context, skippedKinds []string) (bool, error) {
	if s.held != nil {
		held, err := s.held(ctx)
		if err != nil {
			return false, err
		}
		if held {
			return false, nil
		}
	}
	var j Job
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `SELECT id, kind, status, payload, created_by, created_at FROM jobs
			WHERE status = 'queued' AND NOT kind = ANY($1) ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`, append([]string{}, skippedKinds...)).
			Scan(&j.ID, &j.Kind, &j.Status, &j.Payload, &j.CreatedBy, &j.CreatedAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE jobs SET status = 'running', started_at = now() WHERE id = $1", j.ID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	result, runErr := s.run(ctx, j)
	status := StatusDone
	if runErr != nil {
		status = StatusFailed
		s.log.Error("job failed", "job", j.ID, "kind", j.Kind, "error", runErr)
		message := "The job failed."
		if e, ok := apperr.As(runErr); ok {
			message = e.Message
		}
		result = map[string]any{"error": message, "partial": result}
	}
	body, err := json.Marshal(result)
	if err != nil {
		return true, err
	}
	// A fresh context: the outcome must be stored even during shutdown.
	store, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err = s.pool.Exec(store, "UPDATE jobs SET status = $2, result = $3, finished_at = now() WHERE id = $1", j.ID, status, body)
	return true, err
}

func (s *Service) run(ctx context.Context, j Job) (result any, err error) {
	h, ok := s.handlers[j.Kind]
	if !ok {
		return nil, fmt.Errorf("jobs: no handler for %q", j.Kind)
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("jobs: panic in %s: %v", j.Kind, v)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	return h(ctx, j)
}

// generalSlots is how many jobs of any kind one worker runs at once. One more slot
// takes short jobs only.
const generalSlots = 3

// Run works off queued jobs until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	var slots sync.WaitGroup
	for range generalSlots {
		slots.Go(func() { s.workSlot(ctx, nil) })
	}
	slots.Go(func() { s.workSlot(ctx, s.longKinds) })
	slots.Wait()
}

func (s *Service) workSlot(ctx context.Context, skippedKinds []string) {
	for ctx.Err() == nil {
		more, err := s.runNext(ctx, skippedKinds)
		if err != nil && ctx.Err() == nil {
			s.log.Error("job runner failed", "error", err)
		}
		if more {
			continue
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

// FailStale marks jobs as failed that a crashed worker left behind.
func (s *Service) FailStale(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE jobs SET status = 'failed', finished_at = now(),
			result = '{"error":"The worker stopped while this job was running."}'
		WHERE status = 'running' AND started_at < now() - make_interval(secs => $1)`, (s.timeout + 10*time.Minute).Seconds())
	return err
}

// claim takes the lease of a task when it is due. A lease row works everywhere,
// also through PgBouncer, where session-level advisory locks do not.
func (s *Service) claim(ctx context.Context, t Task, lease time.Duration) (bool, error) {
	if _, err := s.pool.Exec(ctx, "INSERT INTO job_leases (name) VALUES ($1) ON CONFLICT DO NOTHING", t.Name); err != nil {
		return false, err
	}
	tag, err := s.pool.Exec(ctx, `UPDATE job_leases SET lease_until = now() + make_interval(secs => $2), last_run = now()
		WHERE name = $1 AND lease_until < now() AND last_run + make_interval(secs => $3) <= now()`,
		t.Name, lease.Seconds(), t.Every.Seconds())
	return tag.RowsAffected() == 1, err
}

// Tick runs every task that is due. Tests call it directly.
func (s *Service) Tick(ctx context.Context, tasks []Task) {
	for _, t := range tasks {
		s.runIfDue(ctx, t)
	}
}

func (s *Service) runIfDue(ctx context.Context, t Task) {
	const lease = 30 * time.Minute
	ok, err := s.claim(ctx, t, lease)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("periodic task: claim failed", "task", t.Name, "error", err)
		}
		return
	}
	if !ok {
		return
	}
	taskCtx, cancel := context.WithTimeout(ctx, lease)
	defer cancel()
	if err := t.Run(taskCtx); err != nil && ctx.Err() == nil {
		s.log.Error("periodic task failed", "task", t.Name, "error", err)
	}
	release, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	if _, err := s.pool.Exec(release, "UPDATE job_leases SET lease_until = 'epoch' WHERE name = $1", t.Name); err != nil {
		s.log.Error("periodic task: release failed", "task", t.Name, "error", err)
	}
}

// RunPeriodic runs tasks until ctx is cancelled. Each task has a loop of its own, so
// one that takes minutes, such as the audit check, holds up nothing else (ARC-068).
func (s *Service) RunPeriodic(ctx context.Context, tasks []Task) {
	var loops sync.WaitGroup
	for _, t := range tasks {
		loops.Go(func() {
			for ctx.Err() == nil {
				s.runIfDue(ctx, t)
				select {
				case <-ctx.Done():
				case <-time.After(5 * time.Second):
				}
			}
		})
	}
	loops.Wait()
}
