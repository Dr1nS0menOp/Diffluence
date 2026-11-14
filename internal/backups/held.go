// SPDX-License-Identifier: Apache-2.0

package backups

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/settings"
)

// The worker's side of spec 076. The worker holds to how many backups it keeps and how long
// it waits at most between two, in a file in its own backup folder, which the server never
// reaches. What the console stores raises them at once; it lowers them only once the worker
// compared the code it wrote to its own log with the code a person typed into the console.
// So whatever the server's database role writes, the worker never removes more backups, nor
// waits longer, than it did before such a code.

// Codes makes and checks the codes the worker writes to its log. restore.Gate does it, with
// a key derived from the backup key, which the server never holds.
type Codes interface {
	HoldsKey() bool
	CodeOf(purpose string, message []byte) string
	Matches(purpose string, message []byte, entered string) bool
}

// scheduleCodePurpose keeps these codes apart from those of a restore.
const scheduleCodePurpose = "backup schedule confirmation code"

// nonceBytes is the size of the random part of a code's message: no two codes repeat, and a
// code the server saw once confirms nothing later.
const nonceBytes = 16

// Held is what the worker holds to between rounds.
type Held struct {
	// Keep is the fewest backups the worker keeps; IntervalSeconds the longest it waits.
	Keep            int `json:"keep"`
	IntervalSeconds int `json:"interval_seconds"`
	// Challenge is the code shown in the log that waits for a person, if any.
	Challenge *Challenge `json:"challenge,omitempty"`
	path      string
}

// Challenge is a code the worker wrote to its log for one ask and one pair of values.
type Challenge struct {
	Request         uuid.UUID `json:"request"`
	Nonce           []byte    `json:"nonce"`
	Keep            int       `json:"keep"`
	IntervalSeconds int       `json:"interval_seconds"`
	ShownAt         time.Time `json:"shown_at"`
	Wrong           int       `json:"wrong"`
}

// message is what the code of a challenge is made from.
func (c Challenge) message() []byte {
	out := append([]byte{}, c.Nonce...)
	out = append(out, c.Request[:]...)
	out = binary.BigEndian.AppendUint64(out, uint64(c.Keep))
	return binary.BigEndian.AppendUint64(out, uint64(c.IntervalSeconds))
}

// LoadHeld reads what the worker holds to from path. A missing file is a worker that holds
// nothing yet; a file that cannot be read is an error, never a fresh start.
func LoadHeld(path string) (Held, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Held{path: path}, nil
	}
	if err != nil {
		return Held{}, fmt.Errorf("backups: read what the worker holds to: %w", err)
	}
	var h Held
	if err := json.Unmarshal(raw, &h); err != nil {
		return Held{}, fmt.Errorf("backups: %s is damaged: %v", path, err)
	}
	if h.Keep < 0 || h.IntervalSeconds < 0 {
		return Held{}, fmt.Errorf("backups: %s holds a negative number", path)
	}
	h.path = path
	return h, nil
}

// Save writes what the worker holds to, whole or not at all.
func (h *Held) Save() error {
	if h.path == "" {
		return errors.New("backups: what the worker holds to has no file")
	}
	raw, err := json.Marshal(h)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(h.path), ".held-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), h.path)
}

// begin gives a worker that holds nothing yet the values that keep the most: the console's or
// its own defaults. An installation that kept fewer through the console before this release
// confirms that once with a code.
func (h *Held) begin(keep int, interval time.Duration, d Defaults) {
	if h.Keep < 1 {
		h.Keep = max(keep, d.Keep)
	}
	if h.IntervalSeconds < 1 {
		h.IntervalSeconds = int(min(interval, d.Interval).Seconds())
	}
}

// follow takes more backups or more rounds at once, and returns what the worker holds to.
func (h *Held) follow(keep int, interval time.Duration) (int, time.Duration) {
	if keep >= h.Keep {
		h.Keep = keep
	}
	if seconds := int(interval.Seconds()); seconds <= h.IntervalSeconds {
		h.IntervalSeconds = seconds
	}
	return h.Keep, time.Duration(h.IntervalSeconds) * time.Second
}

// asksFewer says whether the console asks for fewer backups or fewer rounds than held.
func (h Held) asksFewer(keep int, interval time.Duration) bool {
	return keep < h.Keep || int(interval.Seconds()) > h.IntervalSeconds
}

// hold is the part of Next that holds the worker to what it confirmed, inside Next's
// transaction. It reports what the worker holds to, for the console.
func (s *Service) hold(ctx context.Context, tx pgx.Tx, d Defaults, keep int, interval time.Duration) (int, time.Duration, error) {
	d.Held.begin(keep, interval, d)
	keep, interval = d.Held.follow(keep, interval)
	if _, err := tx.Exec(ctx, "UPDATE backup_worker SET held_keep = $1, held_interval = $2 WHERE id = 1",
		keep, int(interval.Seconds())); err != nil {
		return 0, 0, err
	}
	return keep, interval, d.Held.Save()
}

// lockedRequest is the ask that waits, with the code entered for it, locked until the
// transaction ends.
type lockedRequest struct {
	id            uuid.UUID
	intervalHours int
	keep          int
	confirmation  *string
}

func lockRequest(ctx context.Context, tx pgx.Tx) (*lockedRequest, error) {
	var r lockedRequest
	err := tx.QueryRow(ctx, `SELECT id, interval_hours, keep, confirmation FROM backup_schedule_requests
		WHERE finished_at IS NULL FOR UPDATE`).Scan(&r.id, &r.intervalHours, &r.keep, &r.confirmation)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// endRequest ends an ask with its outcome, and records it as done by the worker.
func endRequest(ctx context.Context, tx pgx.Tx, id uuid.UUID, outcome, action, reason string) error {
	if _, err := tx.Exec(ctx, `UPDATE backup_schedule_requests SET outcome = $2, finished_at = now(), confirmation = NULL
		WHERE id = $1`, id, outcome); err != nil {
		return err
	}
	e := audit.Event{ActorType: audit.ActorSystem, ActorID: "backup", Action: action, ObjectType: "backup",
		ObjectID: id.String(), Result: audit.ResultSuccess}
	if reason != "" {
		e.Result, e.Details = audit.ResultFailure, map[string]any{"reason": reason}
	}
	_, err := audit.Record(ctx, tx, e)
	return err
}

// ConfirmFewer is the worker's round for an ask of fewer backups or fewer rounds: it writes a
// code for the ask to its log, compares the code a person typed in constant time, and lowers
// what it holds to only on the right one. Three wrong codes, or none within half an hour, end
// the ask. It returns the line to write to the log, if any, and saves what the worker holds to
// before the transaction ends.
func (s *Service) ConfirmFewer(ctx context.Context, codes Codes, d Defaults) (string, error) {
	h := d.Held
	if h == nil {
		return "", errors.New("backups: confirming needs what the worker holds to")
	}
	var line string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var b settings.Backups
		if err := s.settings.ReadSectionIn(ctx, tx, settings.SectionBackups, &b); err != nil {
			return err
		}
		interval, keep := resolve(b, d.Interval, d.Keep)
		h.begin(keep, interval, d)
		req, err := lockRequest(ctx, tx)
		if err != nil {
			return err
		}
		if line, err = h.answer(ctx, tx, codes, b, req, keep, interval); err != nil {
			return err
		}
		return h.Save()
	})
	return line, err
}

// answer moves the ask along by one step. It changes h; the caller saves it.
func (h *Held) answer(ctx context.Context, tx pgx.Tx, codes Codes, b settings.Backups, req *lockedRequest,
	keep int, interval time.Duration) (string, error) {
	seconds := int(interval.Seconds())
	if !h.asksFewer(keep, interval) {
		h.Challenge = nil
		if req == nil {
			return "", nil
		}
		return "", endRequest(ctx, tx, req.id, outcomeNotNeeded, ActionScheduleNotNeeded, "")
	}
	// Only the console's own ask, for exactly what the section holds now, is answered: a
	// section changed after the ask is not what the person confirmed.
	if req == nil || req.keep != b.Keep || req.intervalHours != b.IntervalHours {
		h.Challenge = nil
		return "", nil
	}
	ch := h.Challenge
	if ch != nil && (ch.Request != req.id || ch.Keep != keep || ch.IntervalSeconds != seconds) {
		h.Challenge, ch = nil, nil
	}
	if ch == nil {
		return h.show(ctx, tx, codes, req.id, keep, seconds)
	}
	switch {
	case req.confirmation != nil && codes.Matches(scheduleCodePurpose, ch.message(), *req.confirmation):
		h.Keep, h.IntervalSeconds = ch.Keep, ch.IntervalSeconds
		h.Challenge = nil
		return "", endRequest(ctx, tx, req.id, outcomeConfirmed, ActionScheduleConfirmed, "")
	case req.confirmation != nil:
		ch.Wrong++
		if _, err := audit.Record(ctx, tx, audit.Event{ActorType: audit.ActorSystem, ActorID: "backup",
			Action: ActionScheduleCodeWrong, ObjectType: "backup", ObjectID: req.id.String(), Result: audit.ResultFailure,
			Details: map[string]any{"try": ch.Wrong}}); err != nil {
			return "", err
		}
		if ch.Wrong >= ConfirmationTries {
			h.Challenge = nil
			return "", endRequest(ctx, tx, req.id, outcomeRefused, ActionScheduleRefused, "wrong_code")
		}
		_, err := tx.Exec(ctx, "UPDATE backup_schedule_requests SET confirmation = NULL, wrong_codes = $2 WHERE id = $1",
			req.id, ch.Wrong)
		return "", err
	case time.Since(ch.ShownAt) > ConfirmationLifetime:
		h.Challenge = nil
		return "", endRequest(ctx, tx, req.id, outcomeRefused, ActionScheduleRefused, "no_code")
	}
	return "", nil
}

// show writes a new code for the ask that waits. Every code has its own random part, so a
// code the server saw once confirms nothing later.
func (h *Held) show(ctx context.Context, tx pgx.Tx, codes Codes, id uuid.UUID, keep, seconds int) (string, error) {
	if !codes.HoldsKey() {
		return "", endRequest(ctx, tx, id, outcomeRefused, ActionScheduleRefused, "no_key")
	}
	nonce := make([]byte, nonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ch := &Challenge{Request: id, Nonce: nonce, Keep: keep, IntervalSeconds: seconds, ShownAt: time.Now()}
	if _, err := tx.Exec(ctx, `UPDATE backup_schedule_requests SET code_shown_at = now(), confirmation = NULL, wrong_codes = 0
		WHERE id = $1`, id); err != nil {
		return "", err
	}
	h.Challenge = ch
	return fmt.Sprintf("To keep the newest %d backups and back up every %s, enter this confirmation code in the console: %s (valid %d minutes)",
		keep, every(seconds), codes.CodeOf(scheduleCodePurpose, ch.message()), int(ConfirmationLifetime.Minutes())), nil
}

// every says an interval in words: whole hours, or minutes.
func every(seconds int) string {
	switch {
	case seconds == 3600:
		return "hour"
	case seconds%3600 == 0:
		return fmt.Sprintf("%d hours", seconds/3600)
	default:
		return fmt.Sprintf("%d minutes", seconds/60)
	}
}

// HeldKeep reads only how many backups the worker holds to, for the shell that removes old
// backups: zero when it holds nothing yet.
func HeldKeep(path string) (int, error) {
	h, err := LoadHeld(path)
	return h.Keep, err
}
