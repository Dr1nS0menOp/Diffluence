// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// Delegation and absence (spec 064 wave DC5, DOC-240, DOC-241). Neither ever widens who may
// approve: a reviewer hands their place only to someone the same entry of the state names, and
// a stand-in takes the place of someone absent only when the state names the stand-in too.
// Someone who looks after the workflows may hand a place to another person who may review,
// as they may add one already (DOC-202). A delegate of a signed step must be able to sign,
// and signs themselves: a signature is never handed on.

const (
	viaDelegated = "delegated"
	viaStandIn   = "stand_in"
	// maxStandInSteps is how far a chain of stand-ins is followed (DOC-241).
	maxStandInSteps = 3
)

// DelegateInput hands a place in a round to someone else.
type DelegateInput struct {
	// From is whose place; nil is the caller's own.
	From   *uuid.UUID
	To     uuid.UUID
	Reason string
}

// reviewerRow is how one person was asked in the current stage.
type reviewerRow struct {
	sourceType string
	sourceID   *uuid.UUID
	answered   bool
}

func readReviewer(ctx context.Context, tx pgx.Tx, st stage, user uuid.UUID) (reviewerRow, bool, error) {
	var r reviewerRow
	err := tx.QueryRow(ctx, `SELECT wr.source_type, wr.source_id, wr.answer IS NOT NULL FROM workflow_reviewers wr
		JOIN users u ON u.id = wr.user_id AND u.type = 'member' AND u.status = 'active'
		WHERE wr.round_id = $1 AND wr.stage = $2 AND wr.user_id = $3 AND wr.removed_at IS NULL`, st.round, st.number, user).
		Scan(&r.sourceType, &r.sourceID, &r.answered)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

// Delegate hands a reviewer's place in the current stage to another person (DOC-240). The
// delegator answers no more and their delegate does; the record keeps who handed it to whom
// and why, link by link.
func (s *Service) Delegate(ctx context.Context, c identity.Caller, id uuid.UUID, in DelegateInput) error {
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxRemovalReason || strings.ContainsRune(reason, 0) {
		return apperr.Invalid("Give a reason of at most 500 characters.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionApprovalRead, uuid.Nil); err != nil {
			return err
		}
		rc, err := s.lockReview(ctx, tx, id)
		if err != nil {
			return err
		}
		from, managed := c.UserID, false
		if in.From != nil && *in.From != c.UserID {
			if err := s.requireReviewManager(ctx, tx, c, rc, false); err != nil {
				return err
			}
			from, managed = *in.From, true
		}
		row, asked, err := readReviewer(ctx, tx, rc.stage, from)
		if err != nil {
			return err
		}
		switch {
		case !asked && !managed:
			return apperr.Forbidden()
		case !asked:
			return apperr.Conflict("not_asked", "This person is not asked in this round.")
		case row.answered:
			return apperr.Conflict("already_answered", "An answer was given; it can be changed, not handed on.")
		}
		if err := s.checkDelegate(ctx, tx, rc, row, in.To, managed); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workflow_reviewers SET removed_at = now(), removed_by = $4, removed_reason = $5
			WHERE round_id = $1 AND stage = $2 AND user_id = $3`, rc.stage.round, rc.stage.number, from, c.UserID, "delegated: "+reason); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO workflow_reviewers (round_id, stage, state_id, user_id, source_type, source_id, added_by, via, via_from, via_reason)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, rc.stage.round, rc.stage.number, rc.state.ID, in.To, row.sourceType, row.sourceID,
			c.UserID, viaDelegated, from, reason); err != nil {
			return err
		}
		authors, err := changeAuthors(ctx, tx, id, rc.ctl)
		if err != nil {
			return err
		}
		if err := recordAudit(ctx, tx, c, "page.review_delegated", "page", id, rc.page.SpaceID,
			map[string]any{"from": from, "to": in.To, "reason": reason, "round": rc.stage.round, "stage": rc.stage.number}); err != nil {
			return err
		}
		// Both sides hear of it (DOC-608): the one taken off as a removal, the delegate as asked.
		if err := recordEvent(ctx, tx, c, "page.reviewer_removed", rc.page,
			map[string]any{"user_id": from, "reason": "delegated", "round": rc.stage.round, "stage": rc.stage.number},
			map[string]any{"user_id": from, "user_ids": authors}); err != nil {
			return err
		}
		return s.askReviewers(ctx, tx, c, rc.page, rc.state, []uuid.UUID{in.To})
	})
}

// checkDelegate refuses a delegate who could not review here, or not this way.
func (s *Service) checkDelegate(ctx context.Context, tx pgx.Tx, rc reviewContext, row reviewerRow, to uuid.UUID, managed bool) error {
	found, _, err := resolveEntries(ctx, tx, rc.page.SpaceID, []workflow.Reviewer{{Type: workflow.ReviewerUser, ID: to}})
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return apperr.Conflict("cannot_review", "This person cannot review here: only members who see the space can, never guests.")
	}
	if !managed {
		named, err := namedBySameEntry(ctx, tx, rc, row, to)
		if err != nil {
			return err
		}
		if !named {
			return apperr.Conflict("not_a_reviewer_here", "Hand your place only to someone the same entry of this state names. Whoever looks after the workflows can choose another person.")
		}
	}
	var present bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM workflow_reviewers WHERE round_id = $1 AND stage = $2 AND user_id = $3)",
		rc.stage.round, rc.stage.number, to).Scan(&present); err != nil {
		return err
	}
	if present {
		return apperr.Conflict("already_asked", "This person was asked in this round already.")
	}
	if !rc.state.Review.AuthorsMayReview {
		authors, err := changeAuthors(ctx, tx, rc.page.ID, rc.ctl)
		if err != nil {
			return err
		}
		if slices.Contains(authors, to) {
			return apperr.Conflict("not_independent", "This person wrote part of this change, and the review must be independent.")
		}
	}
	return s.requireMaySignFor(ctx, tx, rc, to)
}

// requireMaySignFor refuses someone who cannot sign the step that follows an approval that is
// signed.
func (s *Service) requireMaySignFor(ctx context.Context, tx pgx.Tx, rc reviewContext, user uuid.UUID) error {
	if _, signed := approvalSignature(rc); !signed {
		return nil
	}
	ok, err := s.maySign(ctx, tx, user)
	if err != nil || ok {
		return err
	}
	return apperr.Conflict("cannot_sign", "The approval here is signed, and this person may not sign.")
}

func (s *Service) maySign(ctx context.Context, q db.Querier, user uuid.UUID) (bool, error) {
	if s.confirmations == nil {
		return false, nil
	}
	return s.confirmations.MaySign(ctx, q, user)
}

// namedBySameEntry says whether the entry that asked a reviewer names another person too.
// A person added by hand came through no entry, so only a manager hands their place on.
func namedBySameEntry(ctx context.Context, tx pgx.Tx, rc reviewContext, row reviewerRow, to uuid.UUID) (bool, error) {
	if row.sourceID == nil || row.sourceType == sourceAdded {
		return false, nil
	}
	found, _, err := resolveEntries(ctx, tx, rc.page.SpaceID, []workflow.Reviewer{{Type: row.sourceType, ID: *row.sourceID}})
	if err != nil {
		return false, err
	}
	for _, c := range found {
		if c.user == to {
			return true, nil
		}
	}
	return false, nil
}

// Absence is a person's time away, with who stands in (DOC-241).
type Absence struct {
	StartsOn string     `json:"starts_on"`
	EndsOn   string     `json:"ends_on"`
	StandIn  *uuid.UUID `json:"stand_in"`
}

// SetAbsence records the caller's own absence; nobody sets it for someone else.
func (s *Service) SetAbsence(ctx context.Context, c identity.Caller, in Absence) error {
	starts, err := time.Parse(time.DateOnly, in.StartsOn)
	if err != nil {
		return apperr.Invalid("The first day is a date, as 2026-01-31.")
	}
	ends, err := time.Parse(time.DateOnly, in.EndsOn)
	if err != nil || ends.Before(starts) {
		return apperr.Invalid("The last day is a date on or after the first.")
	}
	if in.StandIn != nil && *in.StandIn == c.UserID {
		return apperr.Invalid("Someone else stands in for you.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if in.StandIn != nil {
			var member bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND type = 'member' AND status = 'active')", *in.StandIn).
				Scan(&member); err != nil {
				return err
			}
			if !member {
				return apperr.Invalid("A stand-in is an active member.")
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO absences (user_id, starts_on, ends_on, stand_in) VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id) DO UPDATE SET starts_on = EXCLUDED.starts_on, ends_on = EXCLUDED.ends_on, stand_in = EXCLUDED.stand_in, set_at = now()`,
			c.UserID, starts, ends, in.StandIn); err != nil {
			return err
		}
		return recordPersonal(ctx, tx, c, "user.absence_set", map[string]any{"starts_on": in.StartsOn, "ends_on": in.EndsOn, "stand_in": in.StandIn})
	})
}

// AbsenceOf reads the caller's absence, or nil.
func (s *Service) AbsenceOf(ctx context.Context, c identity.Caller) (*Absence, error) {
	var a Absence
	err := s.pool.QueryRow(ctx, "SELECT to_char(starts_on, 'YYYY-MM-DD'), to_char(ends_on, 'YYYY-MM-DD'), stand_in FROM absences WHERE user_id = $1", c.UserID).
		Scan(&a.StartsOn, &a.EndsOn, &a.StandIn)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, err
}

// EndAbsence removes the caller's absence.
func (s *Service) EndAbsence(ctx context.Context, c identity.Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM absences WHERE user_id = $1", c.UserID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return recordPersonal(ctx, tx, c, "user.absence_ended", map[string]any{})
	})
}

// recordPersonal audits what a person did about themselves, outside any space.
func recordPersonal(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, details map[string]any) error {
	_, err := audit.Record(ctx, tx, c.Event(action, "user", c.UserID.String(), "", details))
	return err
}

// ForgetAbsences removes what an anonymised person said about their time away, and their
// name as someone else's stand-in (PRM-176). Rounds keep who was asked, under the neutral name.
func (s *Service) ForgetAbsences(ctx context.Context, user uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM absences WHERE user_id = $1", user); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE absences SET stand_in = NULL WHERE stand_in = $1", user)
		return err
	})
}

// standIns reads who is away today in the instance's zone, and who stands in for each.
func (s *Service) standIns(ctx context.Context, tx pgx.Tx, people []uuid.UUID) (map[uuid.UUID]*uuid.UUID, error) {
	zone, _, err := s.workingTime(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT user_id, stand_in FROM absences
		WHERE user_id = ANY($1) AND (now() AT TIME ZONE $2)::date BETWEEN starts_on AND ends_on`, people, zone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]*uuid.UUID{}
	for rows.Next() {
		var user uuid.UUID
		var standIn *uuid.UUID
		if err := rows.Scan(&user, &standIn); err != nil {
			return nil, err
		}
		out[user] = standIn
	}
	return out, rows.Err()
}

// askedPerson is one person a new stage asks, and why when it is not the entry itself.
type askedPerson struct {
	candidate
	via     string
	viaFrom *uuid.UUID
	reason  string
}

// replaceAbsent asks, for each absent person, their stand-in when the state names them,
// following a chain of stand-ins for at most three steps; past that, or when a stand-in may
// not take the place, the escalation targets are asked; with none, the absent person stays
// asked (DOC-241). Nobody the state does not name is ever asked this way.
func (s *Service) replaceAbsent(ctx context.Context, tx pgx.Tx, p Page, st workflow.State, found []candidate, eligible func(uuid.UUID) bool) ([]askedPerson, error) {
	people := make([]uuid.UUID, 0, len(found))
	named := map[uuid.UUID]candidate{}
	for _, c := range found {
		people = append(people, c.user)
		if _, ok := named[c.user]; !ok {
			named[c.user] = c
		}
	}
	away, err := s.standIns(ctx, tx, people)
	if err != nil {
		return nil, err
	}
	var out []askedPerson
	var fallback []candidate
	escalated := false
	for _, c := range found {
		if _, absent := away[c.user]; !absent {
			out = append(out, askedPerson{candidate: c})
			continue
		}
		replaced, ok, err := s.followStandIns(ctx, tx, c.user, away, named, eligible)
		if err != nil {
			return nil, err
		}
		if ok {
			from := c.user
			out = append(out, askedPerson{candidate: replaced, via: viaStandIn, viaFrom: &from, reason: "absent"})
			if err := recordAudit(ctx, tx, systemWorkflow, "page.review_stand_in", "page", p.ID, p.SpaceID,
				map[string]any{"absent": c.user, "stand_in": replaced.user, "state": st.ID}); err != nil {
				return nil, err
			}
			continue
		}
		if targets := st.Escalation(); len(targets) > 0 {
			if !escalated {
				if fallback, _, err = resolveEntries(ctx, tx, p.SpaceID, targets); err != nil {
					return nil, err
				}
				escalated = true
			}
			from := c.user
			for _, f := range fallback {
				if eligible(f.user) {
					out = append(out, askedPerson{candidate: f, via: viaEscalated, viaFrom: &from, reason: "absent"})
				}
			}
			continue
		}
		out = append(out, askedPerson{candidate: c, reason: "absent, nobody to stand in"})
	}
	return out, nil
}

// followStandIns walks the chain from someone absent to the first stand-in who is here and may
// take the place.
func (s *Service) followStandIns(ctx context.Context, tx pgx.Tx, user uuid.UUID, away map[uuid.UUID]*uuid.UUID,
	named map[uuid.UUID]candidate, eligible func(uuid.UUID) bool) (candidate, bool, error) {
	at := user
	for range maxStandInSteps {
		next := away[at]
		if next == nil {
			return candidate{}, false, nil
		}
		c, isNamed := named[*next]
		if !isNamed || !eligible(*next) {
			return candidate{}, false, nil
		}
		more, err := s.standIns(ctx, tx, []uuid.UUID{*next})
		if err != nil {
			return candidate{}, false, err
		}
		if _, absent := more[*next]; !absent {
			return c, true, nil
		}
		away[*next] = more[*next]
		at = *next
	}
	return candidate{}, false, nil
}
