// SPDX-License-Identifier: Apache-2.0

package backups

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/settings"
)

// The console sets how often the worker backs up and how many backups it keeps (spec 058).
// Keeping more, or backing up more often, is an administrator's change. Keeping fewer, or
// backing up less often, needs a system administrator with a fresh login and a second factor,
// and takes effect only once the worker compared the code it wrote to its own log with the
// one typed into the console (spec 076). This file is the console's side; held.go is the
// worker's.

// The audit actions of a change that keeps fewer backups.
const (
	ActionScheduleAsked       = "backup.schedule_asked"
	ActionScheduleCodeEntered = "backup.schedule_code_entered"
	ActionScheduleCodeWrong   = "backup.schedule_code_wrong"
	ActionScheduleConfirmed   = "backup.schedule_confirmed"
	ActionScheduleRefused     = "backup.schedule_refused"
	ActionScheduleNotNeeded   = "backup.schedule_not_needed"
)

const (
	// ConfirmationTries is how many wrong codes end an ask, as for a restore.
	ConfirmationTries = 3
	// ConfirmationLifetime is how long a code in the log may wait for a person.
	ConfirmationLifetime = 30 * time.Minute
	// maxEnteredCode bounds what the console stores, with room for spaces and dashes.
	maxEnteredCode = 40
)

// The ends of an ask.
const (
	outcomeConfirmed = "confirmed"
	outcomeRefused   = "refused"
	outcomeReplaced  = "replaced"
	outcomeNotNeeded = "not_needed"
)

// ScheduleRequest is an ask for fewer backups or fewer rounds, as the console shows it.
type ScheduleRequest struct {
	ID            uuid.UUID `json:"id"`
	AskedAt       time.Time `json:"asked_at"`
	IntervalHours int       `json:"interval_hours"`
	Keep          int       `json:"keep"`
	CodeShown     bool      `json:"code_shown"`
	CodeEntered   bool      `json:"code_entered"`
	WrongCodes    int       `json:"wrong_codes"`
}

const scheduleColumns = `id, asked_at, interval_hours, keep, code_shown_at IS NOT NULL, confirmation IS NOT NULL, wrong_codes`

// openScheduleRequest is the ask that waits, or nil.
func openScheduleRequest(ctx context.Context, q db.Querier) (*ScheduleRequest, error) {
	var r ScheduleRequest
	err := q.QueryRow(ctx, "SELECT "+scheduleColumns+" FROM backup_schedule_requests WHERE finished_at IS NULL").
		Scan(&r.ID, &r.AskedAt, &r.IntervalHours, &r.Keep, &r.CodeShown, &r.CodeEntered, &r.WrongCodes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// workerFacts is what the console knows of the worker's choices.
type workerFacts struct {
	interval     time.Duration
	keep         int
	heldKeep     int
	heldInterval time.Duration
}

// readWorker reads the worker's defaults and what it holds to; nil before any worker asked.
func readWorker(ctx context.Context, q db.Querier) (*workerFacts, error) {
	var interval, keep, heldKeep, heldInterval int
	err := q.QueryRow(ctx, `SELECT default_interval, default_keep, COALESCE(held_keep, 0), COALESCE(held_interval, 0)
		FROM backup_worker WHERE id = 1`).Scan(&interval, &keep, &heldKeep, &heldInterval)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &workerFacts{interval: time.Duration(interval) * time.Second, keep: keep, heldKeep: heldKeep,
		heldInterval: time.Duration(heldInterval) * time.Second}, nil
}

// keepsFewer says whether next keeps fewer backups or backs up less often than stored. With
// the worker's defaults known, zero is its default; without them a change to or from zero
// cannot be judged, so it counts as fewer.
func keepsFewer(next, stored settings.Backups, w *workerFacts) bool {
	if w == nil {
		fewerKept := next.Keep != stored.Keep && (next.Keep == 0 || stored.Keep == 0 || next.Keep < stored.Keep)
		lessOften := next.IntervalHours != stored.IntervalHours &&
			(next.IntervalHours == 0 || stored.IntervalHours == 0 || next.IntervalHours > stored.IntervalHours)
		return fewerKept || lessOften
	}
	nextInterval, nextKeep := resolve(next, w.interval, w.keep)
	storedInterval, storedKeep := resolve(stored, w.interval, w.keep)
	return nextKeep < storedKeep || nextInterval > storedInterval
}

// belowHeld says whether the worker must confirm next before it holds to it: it keeps fewer
// or backs up less often than the worker holds to, or the worker never said what it holds.
func belowHeld(next settings.Backups, w *workerFacts) bool {
	if w == nil || w.heldKeep == 0 || w.heldInterval == 0 {
		return true
	}
	interval, keep := resolve(next, w.interval, w.keep)
	return keep < w.heldKeep || interval > w.heldInterval
}

// SetSchedule stores how often to back up and how many backups to keep. Keeping fewer, or
// backing up less often, than now is refused here: it is SetScheduleConfirmed. The route
// requires an administrator.
func (s *Service) SetSchedule(ctx context.Context, c identity.Caller, body io.Reader) (settings.Backups, error) {
	return s.setSchedule(ctx, c, body, settings.Change{At: time.Now()})
}

// SetScheduleConfirmed stores the schedule for a system administrator with a fresh login and
// a second factor, which the route and its handler check. When it keeps fewer backups or backs
// up less often than the worker holds to, it asks the worker for the code from its log; the
// worker goes on holding to more until that code was entered (spec 076).
func (s *Service) SetScheduleConfirmed(ctx context.Context, c identity.Caller, body io.Reader) (settings.Backups, error) {
	return s.setSchedule(ctx, c, body, settings.Change{Confirmed: true, At: time.Now()})
}

func (s *Service) setSchedule(ctx context.Context, c identity.Caller, body io.Reader, change settings.Change) (settings.Backups, error) {
	decoded, err := settings.DecodeSection(settings.SectionBackups, body)
	if err != nil {
		return settings.Backups{}, err
	}
	next := *decoded.(*settings.Backups)
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// One change at a time, so each is judged against the one before.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('backup schedule'))"); err != nil {
			return err
		}
		var stored settings.Backups
		if err := s.settings.ReadSectionIn(ctx, tx, settings.SectionBackups, &stored); err != nil {
			return err
		}
		w, err := readWorker(ctx, tx)
		if err != nil {
			return err
		}
		if !change.Confirmed && keepsFewer(next, stored, w) {
			return settings.ConfirmationRequired("Keeping fewer backups or backing up less often needs a system administrator, " +
				"a fresh login, a second factor and the code from the log of the backup service.")
		}
		if err := settings.StoreSection(ctx, tx, settings.SectionBackups, next); err != nil {
			return err
		}
		details := map[string]any{"section": settings.SectionBackups, "interval_hours": next.IntervalHours, "keep": next.Keep}
		if change.Confirmed {
			details["confirmed"] = true
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.updated", "settings", settings.SectionBackups, "", details)); err != nil {
			return err
		}
		if _, err := outbox.Enqueue(ctx, tx, "settings.updated", map[string]any{"section": settings.SectionBackups}); err != nil {
			return err
		}
		if !change.Confirmed || !belowHeld(next, w) {
			return nil
		}
		return askWorker(ctx, tx, c, next)
	})
	return next, err
}

// askWorker opens an ask for the worker's code, in place of any that waits.
func askWorker(ctx context.Context, tx pgx.Tx, c identity.Caller, next settings.Backups) error {
	if _, err := tx.Exec(ctx, `UPDATE backup_schedule_requests SET outcome = $1, finished_at = now()
		WHERE finished_at IS NULL`, outcomeReplaced); err != nil {
		return err
	}
	var by *uuid.UUID
	if c.UserID != uuid.Nil {
		by = &c.UserID
	}
	id := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO backup_schedule_requests (id, asked_by, interval_hours, keep) VALUES ($1, $2, $3, $4)`,
		id, by, next.IntervalHours, next.Keep); err != nil {
		return err
	}
	_, err := audit.Record(ctx, tx, c.Event(ActionScheduleAsked, "backup", id.String(), "",
		map[string]any{"interval_hours": next.IntervalHours, "keep": next.Keep}))
	return err
}

// EnterScheduleCode stores the code a person typed in the console beside the ask that waits
// for one, already cleaned by the handler. The worker compares it on its next round. The code
// itself is never recorded. The route requires a system administrator.
func (s *Service) EnterScheduleCode(ctx context.Context, c identity.Caller, code string) error {
	if code == "" || len(code) > maxEnteredCode {
		return apperr.Invalid("A confirmation code has 10 digits, such as 12345-67890.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		err := tx.QueryRow(ctx, `UPDATE backup_schedule_requests SET confirmation = $1
			WHERE finished_at IS NULL AND code_shown_at IS NOT NULL AND confirmation IS NULL
			RETURNING id`, code).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("no_code_awaited",
				"Nothing waits for a confirmation code, or the last one entered has not been checked yet.")
		}
		if err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event(ActionScheduleCodeEntered, "backup", id.String(), "", nil))
		return err
	})
}
