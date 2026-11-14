// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
)

// Some changes let more through than the section in force allows: a weaker rule for logging
// in, or a shorter audit retention. An administrator's session does not make them; a system
// administrator does, with a fresh login and a second factor (spec 076). Tightening stays an
// ordinary change.

// RetentionLoweringDelay is how long a shorter audit retention waits before it holds. Until
// then the longer time holds, and any administrator can take the shorter one back.
const RetentionLoweringDelay = 7 * 24 * time.Hour

// maxReason bounds the reason given for a shorter retention.
const maxReason = 500

// CodeConfirmationRequired is the error code of a change that needs a confirmed change.
const CodeConfirmationRequired = "confirmation_required"

// ConfirmationRequired is the refusal of a change an administrator's session cannot make.
func ConfirmationRequired(message string) *apperr.Error {
	return &apperr.Error{Kind: apperr.KindForbidden, Code: CodeConfirmationRequired, Message: message}
}

// Change is what stands behind a change of a section.
type Change struct {
	// Confirmed is a system administrator with a fresh login and a second factor, which the
	// route of a confirmed change and its handler check.
	Confirmed bool
	// Reason says why, for a change that needs one.
	Reason string
	// At is the moment of the change.
	At time.Time
}

// guarded is a section where some changes let more through than before.
type guarded interface {
	// guard checks the change against the section in force, and returns what to store.
	guard(c Change, stored section) (section, error)
}

// lockedSection reads the section in force and locks its row until the transaction ends, so
// two changes are judged one after the other.
func lockedSection(ctx context.Context, tx pgx.Tx, name string, at time.Time) (section, error) {
	v, err := sectionValue(name)
	if err != nil {
		return nil, err
	}
	var raw []byte
	err = tx.QueryRow(ctx, "SELECT value FROM settings WHERE key = $1 FOR UPDATE", sectionKeyPrefix+name).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return nil, fmt.Errorf("settings: read %s: %v", name, err)
	}
	return settle(v, at), nil
}

// settle gives a section as it holds at a moment: a shorter retention whose time came takes
// over. Other sections hold as stored.
func settle(v section, at time.Time) section {
	if r, ok := v.(*Retention); ok {
		settled := r.settled(at)
		return &settled
	}
	return v
}

// secondFactorStrength orders the rules for a second factor, from none to everyone. The rule
// of a new instance asks nothing during its grace, so it sits below administrators.
var secondFactorStrength = []string{FactorNone, FactorAfterGrace, FactorAdministrators, FactorMembers}

// looserThan says whether a lets more in than stored: a weaker rule for a second factor, a
// longer idle time or none, a new domain for signing up, or guest invitations turned on.
func (a Access) looserThan(stored Access) bool {
	if slices.Index(secondFactorStrength, a.SecondFactor) < slices.Index(secondFactorStrength, stored.SecondFactor) {
		return true
	}
	if stored.IdleMinutes != 0 && (a.IdleMinutes == 0 || a.IdleMinutes > stored.IdleMinutes) {
		return true
	}
	if a.GuestInvites && !stored.GuestInvites {
		return true
	}
	for _, d := range a.SignupDomains {
		if !slices.ContainsFunc(stored.SignupDomains, func(s string) bool { return strings.EqualFold(s, d) }) {
			return true
		}
	}
	return false
}

func (a Access) guard(c Change, stored section) (section, error) {
	if a.looserThan(*stored.(*Access)) && !c.Confirmed {
		return nil, ConfirmationRequired("A weaker rule for logging in needs a system administrator, a fresh login and a second factor.")
	}
	return &a, nil
}

// settled is the retention in force at a moment.
func (r Retention) settled(at time.Time) Retention {
	if r.Lowering == nil || at.Before(r.Lowering.From) {
		return r
	}
	return Retention{AuditDays: r.Lowering.AuditDays, LongerBecause: r.Lowering.LongerBecause}
}

// shorterThan says whether r keeps audit rows for less time than stored. Zero is for ever.
func (r Retention) shorterThan(stored Retention) bool {
	return r.AuditDays != 0 && (stored.AuditDays == 0 || r.AuditDays < stored.AuditDays)
}

// guard keeps a shorter retention waiting for RetentionLoweringDelay, with its reason, and
// keeps any waiting one through an ordinary change: only the server sets it.
func (r Retention) guard(c Change, stored section) (section, error) {
	inForce := *stored.(*Retention)
	if !r.shorterThan(inForce) {
		r.Lowering = inForce.Lowering
		return &r, nil
	}
	if !c.Confirmed {
		return nil, ConfirmationRequired("Keeping audit rows for a shorter time needs a system administrator, a fresh login and a second factor.")
	}
	reason := strings.TrimSpace(c.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxReason || strings.ContainsAny(reason, "\r\n\x00") {
		return nil, apperr.Invalid("Keeping audit rows for a shorter time needs a reason, one line of at most 500 characters.")
	}
	inForce.Lowering = &RetentionLowering{AuditDays: r.AuditDays, LongerBecause: r.LongerBecause, Because: reason,
		From: c.At.Add(RetentionLoweringDelay)}
	return &inForce, nil
}

// CancelRetentionLowering takes back a shorter audit retention before its time, so the time
// in force stays. Any administrator may: it keeps more, never less (route rule).
func (s *Service) CancelRetentionLowering(ctx context.Context, c identity.Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		stored, err := lockedSection(ctx, tx, SectionRetention, time.Now())
		if err != nil {
			return err
		}
		r := stored.(*Retention)
		if r.Lowering == nil {
			return apperr.Conflict("no_lowering", "No shorter time for the audit log waits.")
		}
		days := r.Lowering.AuditDays
		r.Lowering = nil
		if err := StoreSection(ctx, tx, SectionRetention, r); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("settings.retention_lowering_cancelled", "settings", SectionRetention, "",
			map[string]any{"section": SectionRetention, "audit_days": days})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "settings.updated", map[string]any{"section": SectionRetention})
		return err
	})
}
