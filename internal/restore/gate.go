// SPDX-License-Identifier: Apache-2.0

package restore

import (
	"context"
	"crypto/hmac"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
)

// The backup service carries out a restore only with what the server never holds (spec 070):
// a code it wrote to its own log, which a person types into the console, and a seal on the
// restore it confirmed and on the safety backup it made. Both come from a key derived from
// the backup key. The application role may write anything in ops.restores; none of it makes
// the backup service change the installation.

const (
	// CodeDigits is the length of a confirmation code.
	CodeDigits = 10
	// CodeLifetime is how long a code shown in the log may wait for a person.
	CodeLifetime = 30 * time.Minute
	// CodeTries is how many wrong codes end a restore. More than one forgives a typing
	// error, which a new request could not, as only one restore may be asked for a day.
	CodeTries = 3
	// maxEntered bounds what the console stores, with room for spaces and dashes.
	maxEntered = 40
)

// The audit actions of the gate.
const (
	ActionCodeEntered = "backup.restore_code_entered"
	ActionCodeWrong   = "backup.restore_code_wrong"
	ActionConfirmed   = "backup.restore_confirmed"
	ActionRefused     = "backup.restore_refused"
)

// ErrNotConfirmed means the backup service never confirmed this restore, or its backup
// changed since.
var ErrNotConfirmed = errors.New("restore: the backup service did not confirm this restore")

// Gate makes the code that confirms a restore and the seals on what the backup service
// confirmed. Its MAC is keyed with what only the backup service holds; the zero Gate holds
// no key and confirms nothing.
type Gate struct {
	mac func(purpose string, message []byte) []byte
}

// NewGate returns a gate that uses mac, such as keys.Worker.MAC.
func NewGate(mac func(purpose string, message []byte) []byte) Gate { return Gate{mac: mac} }

func (g Gate) holdsKey() bool { return g.mac != nil }

// HoldsKey says whether the gate can make and check codes at all.
func (g Gate) HoldsKey() bool { return g.holdsKey() }

func gateMessage(id uuid.UUID, name string) []byte {
	return append(id[:], []byte(name)...)
}

// Code is the confirmation code of a restore of backup, as the log shows it: 12345-67890.
func (g Gate) Code(id uuid.UUID, backup string) string {
	return g.CodeOf("restore confirmation code", gateMessage(id, backup))
}

// CodeOf is a confirmation code for message, kept apart from every other use by purpose, as
// the log shows it: 12345-67890. Another step of the backup service confirms its own changes
// with it (spec 076).
func (g Gate) CodeOf(purpose string, message []byte) string {
	sum := g.mac(purpose, message)
	n := binary.BigEndian.Uint64(sum[:8]) % 10_000_000_000
	digits := fmt.Sprintf("%0*d", CodeDigits, n)
	return digits[:CodeDigits/2] + "-" + digits[CodeDigits/2:]
}

// Matches compares, in constant time, a code a person typed with the code of message. A gate
// without a key matches nothing.
func (g Gate) Matches(purpose string, message []byte, entered string) bool {
	if !g.holdsKey() {
		return false
	}
	return hmac.Equal([]byte(normaliseCode(entered)), []byte(normaliseCode(g.CodeOf(purpose, message))))
}

// CleanCode drops the spaces and dashes a reader adds to a code, and says whether what is
// left is a code at all.
func CleanCode(s string) (string, bool) {
	code := normaliseCode(s)
	return code, isCode(code)
}

// seal marks what the backup service confirmed: a restore of a backup, or the safety backup
// it made for that restore.
func (g Gate) seal(what string, id uuid.UUID, name string) []byte {
	return g.mac("restore seal "+what, gateMessage(id, name))
}

func (g Gate) sealHolds(what string, id uuid.UUID, name string, seal []byte) bool {
	return g.holdsKey() && len(seal) > 0 && hmac.Equal(seal, g.seal(what, id, name))
}

// normaliseCode drops the spaces and dashes a reader adds.
func normaliseCode(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '\t' {
			return -1
		}
		return r
	}, s)
}

func isCode(s string) bool {
	if len(s) != CodeDigits {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// EnterCode stores the code a person typed in the console beside the restore that waits for
// one. The backup service compares it on its next round. The code itself is never recorded.
func EnterCode(ctx context.Context, pool *pgxpool.Pool, c identity.Caller, code string) error {
	code = normaliseCode(code)
	if !isCode(code) || len(code) > maxEntered {
		return apperr.Invalid(fmt.Sprintf("A confirmation code has %d digits, such as 12345-67890.", CodeDigits))
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		var name string
		err := tx.QueryRow(ctx, `UPDATE ops.restores SET confirmation = $1, updated_at = now()
			WHERE finished_at IS NULL AND state = $2 AND code_shown_at IS NOT NULL AND confirmation IS NULL
			RETURNING backup_name`, code, StateRequested).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("no_code_awaited",
				"No restore waits for a confirmation code, or the last one entered has not been checked yet.")
		}
		if err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event(ActionCodeEntered, "backup", name, "", nil))
		return err
	})
}

// PresetCode stores the right code at once, for a request made inside the backup service,
// which holds the key and so needs no person to carry the code.
func PresetCode(ctx context.Context, pool *pgxpool.Pool, g Gate, r Restore) error {
	if !g.holdsKey() {
		return errors.New("restore: no backup key to make the code with")
	}
	_, err := pool.Exec(ctx, `UPDATE ops.restores SET confirmation = $2, code_shown_at = now(), updated_at = now()
		WHERE id = $1 AND finished_at IS NULL AND state = $3`, r.ID, normaliseCode(g.Code(r.ID, r.BackupName)), StateRequested)
	return err
}

// Claimed is what the backup service does about the restore that waits: carry out Restore,
// or write Code to its log for Show.
type Claimed struct {
	Restore *Restore
	Show    *Restore
	Code    string
}

// refuse ends a restore that changed nothing, and records why.
func refuse(ctx context.Context, tx pgx.Tx, r Restore, step, reason string) error {
	if _, err := tx.Exec(ctx, `UPDATE ops.restores SET state = $2, step = $3, updated_at = now(), finished_at = now()
		WHERE id = $1 AND finished_at IS NULL`, r.ID, StateRefused, cleanStep(step)); err != nil {
		return err
	}
	_, err := audit.Record(ctx, tx, refusal(r, reason))
	return err
}

// refusal is the audit event of a restore the backup service would not carry out.
func refusal(r Restore, reason string) audit.Event {
	return audit.Event{ActorType: audit.ActorSystem, ActorID: "backup", Action: ActionRefused,
		ObjectType: "backup", ObjectID: r.BackupName, Result: audit.ResultFailure, Details: map[string]any{"reason": reason}}
}

// gateClaim checks the code of the restore that waits, in the transaction of Claim.
func gateClaim(ctx context.Context, tx pgx.Tx, g Gate) (Claimed, error) {
	var out Claimed
	var entered *string
	var shown, expired bool
	var wrong int
	r, err := scanWith(tx.QueryRow(ctx, "SELECT "+columns+`, confirmation, code_shown_at IS NOT NULL,
			COALESCE(code_shown_at < now() - make_interval(secs => $2), false), wrong_codes
		FROM ops.restores WHERE finished_at IS NULL AND state = $1 FOR UPDATE`, StateRequested, CodeLifetime.Seconds()),
		&entered, &shown, &expired, &wrong)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	switch {
	case !g.holdsKey():
		return out, refuse(ctx, tx, r, "the backup service has no backup key, so it cannot confirm a restore; nothing was changed", "no_key")
	case entered != nil:
		return confirm(ctx, tx, g, r, *entered, wrong)
	case expired:
		return out, refuse(ctx, tx, r, fmt.Sprintf("no confirmation code was entered within %d minutes; nothing was changed",
			int(CodeLifetime.Minutes())), "no_code")
	case !shown:
		if _, err := tx.Exec(ctx, `UPDATE ops.restores SET code_shown_at = now(), updated_at = now(),
				step = 'waiting for the confirmation code from the log of the backup service' WHERE id = $1`, r.ID); err != nil {
			return out, err
		}
		r.CodeShown = true
		out.Show, out.Code = &r, g.Code(r.ID, r.BackupName)
	}
	return out, nil
}

// confirm compares the code a person entered, in constant time.
func confirm(ctx context.Context, tx pgx.Tx, g Gate, r Restore, entered string, wrong int) (Claimed, error) {
	var out Claimed
	want := normaliseCode(g.Code(r.ID, r.BackupName))
	if hmac.Equal([]byte(normaliseCode(entered)), []byte(want)) {
		confirmed, err := scan(tx.QueryRow(ctx, `UPDATE ops.restores SET state = $2, step = 'checking the backup',
				confirmation = NULL, worker_seal = $3, updated_at = now()
			WHERE id = $1 RETURNING `+columns, r.ID, StatePreparing, g.seal("restore", r.ID, r.BackupName)))
		if err != nil {
			return out, err
		}
		if _, err := audit.Record(ctx, tx, audit.Event{ActorType: audit.ActorSystem, ActorID: "backup", Action: ActionConfirmed,
			ObjectType: "backup", ObjectID: r.BackupName, Result: audit.ResultSuccess}); err != nil {
			return out, err
		}
		out.Restore = &confirmed
		return out, nil
	}
	wrong++
	if _, err := audit.Record(ctx, tx, audit.Event{ActorType: audit.ActorSystem, ActorID: "backup", Action: ActionCodeWrong,
		ObjectType: "backup", ObjectID: r.BackupName, Result: audit.ResultFailure, Details: map[string]any{"try": wrong}}); err != nil {
		return out, err
	}
	if wrong >= CodeTries {
		return out, refuse(ctx, tx, r, fmt.Sprintf("%d wrong confirmation codes; nothing was changed", wrong), "wrong_code")
	}
	_, err := tx.Exec(ctx, `UPDATE ops.restores SET confirmation = NULL, wrong_codes = $2, updated_at = now(), step = $3
		WHERE id = $1`, r.ID, wrong, fmt.Sprintf("the confirmation code was wrong; %d tries left", CodeTries-wrong))
	return out, err
}

// seals reads what the backup service sealed on a restore.
func seals(ctx context.Context, q db.Querier, id uuid.UUID) (restoreSeal, safetySeal []byte, err error) {
	err = q.QueryRow(ctx, "SELECT worker_seal, safety_seal FROM ops.restores WHERE id = $1", id).Scan(&restoreSeal, &safetySeal)
	return restoreSeal, safetySeal, err
}
