// SPDX-License-Identifier: Apache-2.0

// Package backups lets the console control backups that a worker makes (spec 058). The
// console writes settings and requests into the database; the worker (the backup service
// of Compose, a Kubernetes CronJob or a systemd timer) asks what to do, makes the backup
// with its own access to the database and files, and reports back. So the console looks
// the same on every platform, and the key that opens backups never reaches the server.
package backups

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/settings"
)

// Limits of the protocol between console and worker (spec 058, load budget).
const (
	// RequestsPerHour is how often "back up now" may be pressed: a backup reads the whole
	// database and every file, so pressing it in a loop must not wear the server down.
	RequestsPerHour = 4
	// pollAtMost is the longest a worker waits before asking again, so a request from the
	// console is picked up within a minute.
	pollAtMost = 60 * time.Second
	// staleClaim is how long a claimed request may stay unfinished before another try
	// takes it over: a worker that died must not block "back up now" for ever.
	staleClaim = 6 * time.Hour
	// WorkerAbsentAfter is how long the worker may be silent before the console says it
	// has not checked in.
	WorkerAbsentAfter = 15 * time.Minute
	// mailAtMostEvery keeps a backup that fails every few hours from filling inboxes.
	mailAtMostEvery = 12 * time.Hour
	// listed is how many backups the console lists.
	listed     = 20
	maxMessage = 300
)

// Reasons a backup is made.
const (
	ReasonSchedule = "schedule"
	ReasonRequest  = "request"
	ReasonManual   = "manual"
)

// Kinds of mail this package sends to administrators.
const (
	MailBackupFailed       = "backup_failed"
	MailRestoreCheckFailed = "restore_check_failed"
)

// backupName is the folder name the worker writes: diffluence-20261111T071618Z.
var backupName = regexp.MustCompile(`^diffluence-[0-9]{8}T[0-9]{6}Z$`)

// fingerprint is the start of a SHA-256 of the key, which shows the key changed without
// saying anything about it.
var fingerprint = regexp.MustCompile(`^[0-9a-f]{16}$`)

// Mailer queues a mail to one administrator; identity's account mail does it.
type Mailer interface {
	Send(ctx context.Context, q db.Querier, m identity.AccountMail) error
}

// Service is the backups module.
type Service struct {
	pool     *pgxpool.Pool
	settings *settings.Service
	mailer   Mailer
	console  string
}

// NewService returns the module. mailer may be nil: then nothing is mailed. consoleURL is
// the address of the backup page, which a mail links to.
func NewService(pool *pgxpool.Pool, s *settings.Service, mailer Mailer, consoleURL string) *Service {
	return &Service{pool: pool, settings: s, mailer: mailer, console: consoleURL}
}

// Defaults are what the worker was started with, and whether it holds a key.
type Defaults struct {
	Interval       time.Duration
	Keep           int
	KeyFingerprint string // empty: no key
}

// Decision is what the worker should do now.
type Decision struct {
	Now    bool
	Reason string // when Now
	Wait   time.Duration
	Keep   int
}

// Next records that the worker is alive and tells it whether to make a backup now: when
// the console asked for one, or when the interval since the last try has passed. It
// claims a request in the same transaction, so two workers never take the same one.
func (s *Service) Next(ctx context.Context, d Defaults) (Decision, error) {
	if d.Interval < time.Minute || d.Keep < 1 {
		return Decision{}, errors.New("backups: the worker needs an interval of a minute or more and a keep of 1 or more")
	}
	if d.KeyFingerprint != "" && !fingerprint.MatchString(d.KeyFingerprint) {
		return Decision{}, errors.New("backups: a key fingerprint is 16 hexadecimal characters")
	}
	var out Decision
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO backup_worker (id, seen_at, default_interval, default_keep, key_fingerprint)
			VALUES (1, now(), $1, $2, $3)
			ON CONFLICT (id) DO UPDATE SET seen_at = now(), default_interval = EXCLUDED.default_interval,
				default_keep = EXCLUDED.default_keep, key_fingerprint = EXCLUDED.key_fingerprint`,
			int(d.Interval.Seconds()), d.Keep, d.KeyFingerprint); err != nil {
			return err
		}
		interval, keep, err := s.effective(ctx, tx, d)
		if err != nil {
			return err
		}
		out.Keep = keep
		var id uuid.UUID
		var claimed *time.Time
		err = tx.QueryRow(ctx, `SELECT id, claimed_at FROM backup_requests WHERE finished_at IS NULL FOR UPDATE`).Scan(&id, &claimed)
		switch {
		case err == nil && (claimed == nil || time.Since(*claimed) > staleClaim):
			if _, err := tx.Exec(ctx, "UPDATE backup_requests SET claimed_at = now() WHERE id = $1", id); err != nil {
				return err
			}
			out.Now, out.Reason = true, ReasonRequest
			return nil
		case err == nil:
			// Another try is making the requested backup.
			out.Wait = pollAtMost
			return nil
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		var last *time.Time
		if err := tx.QueryRow(ctx, "SELECT max(started_at) FROM backup_runs").Scan(&last); err != nil {
			return err
		}
		if last == nil {
			out.Now, out.Reason = true, ReasonSchedule
			return nil
		}
		due := time.Until(last.Add(interval))
		if due <= 0 {
			out.Now, out.Reason = true, ReasonSchedule
			return nil
		}
		out.Wait = min(due, pollAtMost)
		return nil
	})
	return out, err
}

// effective is the interval and the number kept: the console's choice, or the worker's
// default where the console left it at zero.
func (s *Service) effective(ctx context.Context, q db.Querier, d Defaults) (time.Duration, int, error) {
	var b settings.Backups
	if err := s.settings.ReadSectionIn(ctx, q, settings.SectionBackups, &b); err != nil {
		return 0, 0, err
	}
	interval, keep := d.Interval, d.Keep
	if b.IntervalHours > 0 {
		interval = time.Duration(b.IntervalHours) * time.Hour
	}
	if b.Keep > 0 {
		keep = b.Keep
	}
	return interval, keep, nil
}

// Report is what the worker says about one backup it tried.
type Report struct {
	Name      string
	Reason    string
	StartedAt time.Time
	OK        bool
	SizeBytes int64
	Message   string // on failure, the step that failed
}

// cleanMessage keeps a message to one short line of printable text, so a worker can
// never put markup or a flood of text in front of an administrator.
func cleanMessage(m string) string {
	m = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, m)
	m = strings.Join(strings.Fields(m), " ")
	if len([]rune(m)) > maxMessage {
		m = string([]rune(m)[:maxMessage])
	}
	return m
}

func (r Report) validate() error {
	if !backupName.MatchString(r.Name) {
		return apperr.Invalid("A backup is named diffluence- with its time, such as diffluence-20261111T071618Z.")
	}
	if r.Reason != ReasonSchedule && r.Reason != ReasonRequest && r.Reason != ReasonManual {
		return apperr.Invalid("A backup is made on schedule, on request or by hand.")
	}
	if r.StartedAt.IsZero() || r.StartedAt.After(time.Now().Add(time.Minute)) {
		return apperr.Invalid("A backup needs the time it started.")
	}
	if r.SizeBytes < 0 {
		return apperr.Invalid("A size is not negative.")
	}
	return nil
}

// ReportRun records a backup the worker tried, ends the request it answered, writes it
// to the audit log, and mails the administrators when it failed.
func (s *Service) ReportRun(ctx context.Context, r Report) error {
	if err := r.validate(); err != nil {
		return err
	}
	r.Message = cleanMessage(r.Message)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		id := ids.New()
		var size *int64
		if r.OK {
			size = &r.SizeBytes
		}
		if _, err := tx.Exec(ctx, `INSERT INTO backup_runs (id, name, reason, started_at, ok, size_bytes, message)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, r.Name, r.Reason, r.StartedAt, r.OK, size, r.Message); err != nil {
			return err
		}
		if r.Reason == ReasonRequest {
			if _, err := tx.Exec(ctx, "UPDATE backup_requests SET finished_at = now() WHERE finished_at IS NULL AND claimed_at IS NOT NULL"); err != nil {
				return err
			}
		}
		action, result := "backup.made", audit.ResultSuccess
		details := map[string]any{"reason": r.Reason}
		if r.OK {
			details["bytes"] = r.SizeBytes
		} else {
			action, result = "backup.failed", audit.ResultFailure
			details["message"] = r.Message
		}
		e := identity.SystemCaller("backup").Event(action, "backup", r.Name, "", details)
		e.Result = result
		_, err := audit.Record(ctx, tx, e)
		return err
	})
}

// RestoreReport is the result of one restore check into a throw-away copy.
type RestoreReport struct {
	Name    string
	OK      bool
	Seconds int
	Message string
}

// ReportRestore records a restore check, writes it to the audit log, and mails the
// administrators when the backup did not come back.
func (s *Service) ReportRestore(ctx context.Context, r RestoreReport) error {
	if !backupName.MatchString(r.Name) {
		return apperr.Invalid("A backup is named diffluence- with its time, such as diffluence-20261111T071618Z.")
	}
	if r.Seconds < 0 {
		return apperr.Invalid("A restore does not take negative time.")
	}
	r.Message = cleanMessage(r.Message)
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		id := ids.New()
		if _, err := tx.Exec(ctx, `INSERT INTO backup_restore_checks (id, name, ok, seconds, message)
			VALUES ($1, $2, $3, $4, $5)`, id, r.Name, r.OK, r.Seconds, r.Message); err != nil {
			return err
		}
		e := identity.SystemCaller("backup").Event("backup.restore_checked", "backup", r.Name, "",
			map[string]any{"seconds": r.Seconds, "message": r.Message})
		if !r.OK {
			e.Result = audit.ResultFailure
		}
		_, err := audit.Record(ctx, tx, e)
		return err
	})
}

// failureMail is one kind of failure the administrators are mailed about. Its queries are
// written out in full, so no SQL is put together while the program runs.
type failureMail struct {
	kind string
	// waiting asks whether a failure waits to be mailed; markMailed marks every one mailed.
	waiting, markMailed string
}

var failureMails = []failureMail{
	{kind: MailBackupFailed,
		waiting:    `SELECT EXISTS (SELECT 1 FROM backup_runs WHERE NOT ok AND mailed_at IS NULL)`,
		markMailed: `UPDATE backup_runs SET mailed_at = now() WHERE NOT ok AND mailed_at IS NULL`},
	{kind: MailRestoreCheckFailed,
		waiting:    `SELECT EXISTS (SELECT 1 FROM backup_restore_checks WHERE NOT ok AND mailed_at IS NULL)`,
		markMailed: `UPDATE backup_restore_checks SET mailed_at = now() WHERE NOT ok AND mailed_at IS NULL`},
}

// MailFailures mails every active administrator about backups and restore checks that
// failed since the last look, at most once per kind in 12 hours; failures within those 12
// hours are folded into the mail already sent. A server node runs it every few minutes,
// because only the server holds the key that seals mail; the worker only writes rows.
func (s *Service) MailFailures(ctx context.Context) error {
	if s.mailer == nil {
		return nil
	}
	for _, m := range failureMails {
		if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return s.mailFailure(ctx, tx, m) }); err != nil {
			return fmt.Errorf("backups: mail %s: %w", m.kind, err)
		}
	}
	return nil
}

func (s *Service) mailFailure(ctx context.Context, tx pgx.Tx, m failureMail) error {
	var waiting bool
	if err := tx.QueryRow(ctx, m.waiting).Scan(&waiting); err != nil {
		return err
	}
	if !waiting {
		return nil
	}
	// The lock makes two nodes that look at once send one mail, not two, also before the
	// first mail of a kind gave backup_mail a row to lock.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "backup mail "+m.kind); err != nil {
		return err
	}
	var sentAt *time.Time
	if err := tx.QueryRow(ctx, "SELECT sent_at FROM backup_mail WHERE kind = $1", m.kind).Scan(&sentAt); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if sentAt == nil || time.Since(*sentAt) >= mailAtMostEvery {
		rows, err := tx.Query(ctx, `SELECT email FROM users u WHERE u.status = 'active' AND u.type <> 'service'
			AND u.anonymised_at IS NULL AND perm_has_global(u.id, 'administrator') ORDER BY email`)
		if err != nil {
			return err
		}
		to, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, address := range to {
			if err := s.mailer.Send(ctx, tx, identity.AccountMail{To: address, Kind: m.kind, Link: s.console}); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO backup_mail (kind, sent_at) VALUES ($1, now())
			ON CONFLICT (kind) DO UPDATE SET sent_at = now()`, m.kind); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, m.markMailed)
	return err
}

// RequestNow asks the worker for a backup now. Only one can wait at a time, and at most
// RequestsPerHour an hour. The route requires administrator.
func (s *Service) RequestNow(ctx context.Context, c identity.Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var lastHour int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM backup_requests WHERE requested_at > now() - interval '1 hour'`).Scan(&lastHour); err != nil {
			return err
		}
		if lastHour >= RequestsPerHour {
			return apperr.Busy(fmt.Sprintf("At most %d backups an hour can be asked for. Try again later.", RequestsPerHour))
		}
		id := ids.New()
		_, err := tx.Exec(ctx, "INSERT INTO backup_requests (id, requested_by) VALUES ($1, $2)", id, c.UserID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// The index backup_requests_one_open lets only one request wait, also when two
			// administrators press it at the same moment.
			return apperr.Conflict("backup_requested", "A backup was asked for and is not finished yet.")
		}
		if err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("backup.requested", "backup", id.String(), "", nil))
		return err
	})
}

// Run is one backup the worker tried, as the console lists it.
type Run struct {
	Name          string     `json:"name"`
	Reason        string     `json:"reason"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    time.Time  `json:"finished_at"`
	OK            bool       `json:"ok"`
	SizeBytes     *int64     `json:"size_bytes"`
	Message       string     `json:"message"`
	RestoreOK     *bool      `json:"restore_ok"`
	RestoreAt     *time.Time `json:"restore_at"`
	RestoreSecond *int       `json:"restore_seconds"`
}

// Worker is what the console knows of the worker.
type Worker struct {
	SeenAt               time.Time `json:"seen_at"`
	Absent               bool      `json:"absent"`
	DefaultIntervalHours float64   `json:"default_interval_hours"`
	DefaultKeep          int       `json:"default_keep"`
	KeyPresent           bool      `json:"key_present"`
	KeyFingerprint       string    `json:"key_fingerprint"`
}

// Request is the "back up now" that waits, if any.
type Request struct {
	RequestedAt time.Time  `json:"requested_at"`
	ClaimedAt   *time.Time `json:"claimed_at"`
}

// RestoreCheck is the latest restore check.
type RestoreCheck struct {
	Name      string    `json:"name"`
	CheckedAt time.Time `json:"checked_at"`
	OK        bool      `json:"ok"`
	Seconds   *int      `json:"seconds"`
	Message   string    `json:"message"`
}

// Overview is the backup page of the console.
type Overview struct {
	Settings settings.Backups `json:"settings"`
	// Worker is nil until a worker has ever asked what to do.
	Worker       *Worker       `json:"worker"`
	Request      *Request      `json:"request"`
	Runs         []Run         `json:"runs"`
	RestoreCheck *RestoreCheck `json:"restore_check"`
	// RequestsPerHour is the limit on "back up now", for the page to say.
	RequestsPerHour int `json:"requests_per_hour"`
}

// Overview reads the backup page: four small queries on tables that stay small. The
// route requires administrator.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	out := Overview{Runs: []Run{}, RequestsPerHour: RequestsPerHour}
	if err := s.settings.ReadSection(ctx, settings.SectionBackups, &out.Settings); err != nil {
		return out, err
	}
	var w Worker
	var interval int
	err := s.pool.QueryRow(ctx, `SELECT seen_at, default_interval, default_keep, key_fingerprint FROM backup_worker WHERE id = 1`).
		Scan(&w.SeenAt, &interval, &w.DefaultKeep, &w.KeyFingerprint)
	switch {
	case err == nil:
		w.DefaultIntervalHours = float64(interval) / 3600
		w.KeyPresent = w.KeyFingerprint != ""
		w.Absent = time.Since(w.SeenAt) > WorkerAbsentAfter
		out.Worker = &w
	case !errors.Is(err, pgx.ErrNoRows):
		return out, err
	}
	var req Request
	err = s.pool.QueryRow(ctx, "SELECT requested_at, claimed_at FROM backup_requests WHERE finished_at IS NULL").Scan(&req.RequestedAt, &req.ClaimedAt)
	switch {
	case err == nil:
		out.Request = &req
	case !errors.Is(err, pgx.ErrNoRows):
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT r.name, r.reason, r.started_at, r.finished_at, r.ok, r.size_bytes, r.message,
			c.ok, c.checked_at, c.seconds
		FROM backup_runs r
		LEFT JOIN LATERAL (SELECT ok, checked_at, seconds FROM backup_restore_checks c
			WHERE c.name = r.name ORDER BY checked_at DESC LIMIT 1) c ON true
		ORDER BY r.finished_at DESC LIMIT $1`, listed)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.Name, &r.Reason, &r.StartedAt, &r.FinishedAt, &r.OK, &r.SizeBytes, &r.Message,
			&r.RestoreOK, &r.RestoreAt, &r.RestoreSecond); err != nil {
			rows.Close()
			return out, err
		}
		out.Runs = append(out.Runs, r)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	var rc RestoreCheck
	err = s.pool.QueryRow(ctx, `SELECT name, checked_at, ok, seconds, message FROM backup_restore_checks
		ORDER BY checked_at DESC LIMIT 1`).Scan(&rc.Name, &rc.CheckedAt, &rc.OK, &rc.Seconds, &rc.Message)
	switch {
	case err == nil:
		out.RestoreCheck = &rc
	case !errors.Is(err, pgx.ErrNoRows):
		return out, err
	}
	return out, nil
}
