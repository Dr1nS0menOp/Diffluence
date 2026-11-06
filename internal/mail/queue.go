// SPDX-License-Identifier: Apache-2.0

package mail

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
)

// The kinds of mail in the queue.
const (
	KindInstant = "instant"
	KindDigest  = "digest"
	KindTest    = "test"
	// KindAccount is a reset, invitation or sign-up: its row holds a sealed message,
	// and it can go to someone without an account.
	KindAccount = "account"
)

const (
	maxAttempts   = 5
	batchSize     = 20
	batchWait     = time.Minute // mentions in the same minute become one mail
	stuckAfter    = 10 * time.Minute
	idlePoll      = 5 * time.Second
	listedInAdmin = 100
)

// retryWaits are the pauses after a failed try.
var retryWaits = []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour}

// Job is one mail waiting to be written and sent. It holds ids, never text.
type Job struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	Kind            string
	NotificationIDs []uuid.UUID
	// Sealed is the encrypted message of an account mail.
	Sealed []byte
}

// Rendered is the outcome of writing a mail.
type Rendered struct {
	Message Message
	Drop    bool      // nothing left to say, or the person lost access
	Hold    time.Time // quiet hours: try again at this moment
}

// Renderer writes a mail at the moment it is sent, checking access again.
type Renderer func(ctx context.Context, j Job) (Rendered, error)

// Service works through the queue.
type Service struct {
	pool   *pgxpool.Pool
	sender Sender
	render Renderer
	log    *slog.Logger
	held   func(context.Context) (bool, error)
}

// NewService creates the service. Without a sender, mail waits in the queue.
func NewService(pool *pgxpool.Pool, sender Sender, log *slog.Logger) *Service {
	return &Service{pool: pool, sender: sender, log: log}
}

// SetRenderer connects the module that knows what a mail says.
func (s *Service) SetRenderer(r Renderer) { s.render = r }

// HoldWhile makes mail wait for as long as the check says so: maintenance mode.
func (s *Service) HoldWhile(check func(context.Context) (bool, error)) { s.held = check }

// Configured reports whether a mail server is set.
func (s *Service) Configured() bool { return s.sender != nil }

// EnqueueInstant adds a notification to the person's next mail.
func EnqueueInstant(ctx context.Context, q db.Querier, user, notification uuid.UUID) error {
	_, err := q.Exec(ctx, `INSERT INTO mail_queue (id, user_id, kind, notification_ids, next_attempt_at)
		VALUES ($1, $2, 'instant', ARRAY[$3::uuid], now() + make_interval(secs => $4))
		ON CONFLICT (user_id) WHERE status = 'queued' AND kind = 'instant'
		DO UPDATE SET notification_ids = mail_queue.notification_ids || $3::uuid`,
		ids.New(), user, notification, batchWait.Seconds())
	return err
}

// EnqueueDigest queues one digest.
func EnqueueDigest(ctx context.Context, q db.Querier, user uuid.UUID, notifications []uuid.UUID) error {
	_, err := q.Exec(ctx, "INSERT INTO mail_queue (id, user_id, kind, notification_ids) VALUES ($1, $2, 'digest', $3)",
		ids.New(), user, notifications)
	return err
}

// EnqueueAccount queues a sealed account mail.
func EnqueueAccount(ctx context.Context, q db.Querier, sealed []byte) error {
	_, err := q.Exec(ctx, "INSERT INTO mail_queue (id, kind, sealed) VALUES ($1, $2, $3)", ids.New(), KindAccount, sealed)
	return err
}

// EnqueueTest queues a test mail to the person who asked for it.
func (s *Service) EnqueueTest(ctx context.Context, user uuid.UUID) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO mail_queue (id, user_id, kind) VALUES ($1, $2, 'test')", ids.New(), user)
	return err
}

// claim marks due mails as being sent, so another worker leaves them alone. New
// notifications then start a new mail instead of joining one that left already.
func (s *Service) claim(ctx context.Context) ([]Job, error) {
	rows, err := s.pool.Query(ctx, `UPDATE mail_queue SET status = 'sending', attempts = attempts + 1, next_attempt_at = now()
		WHERE id IN (SELECT id FROM mail_queue WHERE status = 'queued' AND next_attempt_at <= now()
			ORDER BY next_attempt_at LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING id, user_id, kind, notification_ids, sealed`, batchSize)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Job, error) {
		var j Job
		var user uuid.NullUUID
		err := r.Scan(&j.ID, &user, &j.Kind, &j.NotificationIDs, &j.Sealed)
		j.UserID = user.UUID
		return j, err
	})
}

func (s *Service) finish(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE mail_queue SET status = $2, last_error = '', sealed = NULL,
		sent_at = CASE WHEN $2 = 'sent' THEN now() END WHERE id = $1`, id, status)
	return err
}

// hold waits for quiet hours to end. It is no failed try.
func (s *Service) hold(ctx context.Context, id uuid.UUID, until time.Time) error {
	_, err := s.pool.Exec(ctx, "UPDATE mail_queue SET status = 'queued', attempts = attempts - 1, next_attempt_at = $2 WHERE id = $1", id, until)
	return err
}

func retryWait(attempts int) time.Duration {
	return retryWaits[min(max(attempts-1, 0), len(retryWaits)-1)]
}

func (s *Service) retry(ctx context.Context, id uuid.UUID, sendErr error) error {
	var attempts int
	if err := s.pool.QueryRow(ctx, "SELECT attempts FROM mail_queue WHERE id = $1", id).Scan(&attempts); err != nil {
		return err
	}
	message := safeError(sendErr)
	s.log.Warn("sending a mail failed", "mail", id, "attempt", attempts, "reason", message)
	if attempts >= maxAttempts {
		_, err := s.pool.Exec(ctx, "UPDATE mail_queue SET status = 'failed', last_error = $2, sealed = NULL WHERE id = $1", id, message)
		return err
	}
	_, err := s.pool.Exec(ctx, `UPDATE mail_queue SET status = 'queued', last_error = $2,
		next_attempt_at = now() + make_interval(secs => $3) WHERE id = $1`, id, message, retryWait(attempts).Seconds())
	return err
}

func (s *Service) deliver(ctx context.Context, j Job) error {
	out, err := s.render(ctx, j)
	if err != nil {
		return s.retry(ctx, j.ID, err)
	}
	if out.Drop {
		return s.finish(ctx, j.ID, "dropped")
	}
	if !out.Hold.IsZero() {
		return s.hold(ctx, j.ID, out.Hold)
	}
	if err := s.sender.Send(ctx, out.Message); err != nil {
		return s.retry(ctx, j.ID, err)
	}
	return s.finish(ctx, j.ID, "sent")
}

// RunOnce sends what is due, and returns how many mails it handled. Without a mail
// server everything simply stays in the queue.
func (s *Service) RunOnce(ctx context.Context) (int, error) {
	if s.sender == nil || s.render == nil {
		return 0, nil
	}
	if s.held != nil {
		held, err := s.held(ctx)
		if err != nil || held {
			return 0, err
		}
	}
	if _, err := s.pool.Exec(ctx, `UPDATE mail_queue SET status = 'queued'
		WHERE status = 'sending' AND next_attempt_at < now() - make_interval(secs => $1)`, stuckAfter.Seconds()); err != nil {
		return 0, err
	}
	jobs, err := s.claim(ctx)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if err := s.deliver(ctx, j); err != nil {
			return 0, err
		}
	}
	return len(jobs), nil
}

// Run sends mail until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	for ctx.Err() == nil {
		n, err := s.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			s.log.Error("mail queue failed", "error", err)
		}
		if n > 0 {
			continue
		}
		select {
		case <-ctx.Done():
		case <-time.After(idlePoll):
		}
	}
}

// Entry is one row of the queue, for the admin console: no content, no addresses.
type Entry struct {
	ID        uuid.UUID  `json:"id"`
	Kind      string     `json:"kind"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	LastError string     `json:"last_error"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at"`
}

// List shows the newest rows of the queue. The route requires administrator.
func (s *Service) List(ctx context.Context) ([]Entry, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, kind, status, attempts, last_error, created_at, sent_at
		FROM mail_queue ORDER BY created_at DESC, id LIMIT $1`, listedInAdmin)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Entry, error) {
		var e Entry
		err := r.Scan(&e.ID, &e.Kind, &e.Status, &e.Attempts, &e.LastError, &e.CreatedAt, &e.SentAt)
		return e, err
	})
	if out == nil {
		out = []Entry{}
	}
	return out, err
}
