// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// Read and understood, and periodic review (spec 064 wave DC5, blueprint 09.9). Both follow a
// version after it is approved: the groups its state names are asked to confirm they read it,
// and a timed state gives it a next review date.

// ReadRequest is what the band tells someone asked to confirm reading (DOC-820, DOC-824).
type ReadRequest struct {
	Version   int  `json:"version"`
	Confirmed bool `json:"confirmed"`
	// CanConfirm says the person opened this version in this session, so the button shows.
	CanConfirm bool `json:"can_confirm"`
}

// afterApproval asks for reading and sets the next review date of a version just approved
// into a state.
func (s *Service) afterApproval(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, version int, to workflow.State) error {
	if err := scheduleReview(ctx, tx, p.ID, version, to); err != nil {
		return err
	}
	if to.ReadAndUnderstood == nil {
		return nil
	}
	tag, err := tx.Exec(ctx, "INSERT INTO read_requests (page_id, version_number, groups) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING",
		p.ID, version, to.ReadAndUnderstood.Groups)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	rows, err := tx.Query(ctx, `INSERT INTO read_confirmations (page_id, version_number, user_id)
		SELECT DISTINCT $1::uuid, $2::int, gm.user_id FROM group_members gm
		JOIN users u ON u.id = gm.user_id AND u.type = 'member' AND u.status = 'active'
		WHERE gm.group_id = ANY($3) AND $4::uuid IN (SELECT ps.space_id FROM perm_spaces(u.id, 'space.view') ps)
		ON CONFLICT DO NOTHING RETURNING user_id`, p.ID, version, to.ReadAndUnderstood.Groups, p.SpaceID)
	if err != nil {
		return err
	}
	people, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	return askToRead(ctx, tx, c, p, version, people, false)
}

func askToRead(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, version int, people []uuid.UUID, late bool) error {
	if len(people) == 0 {
		return nil
	}
	return recordEvent(ctx, tx, c, "page.read_requested", p, map[string]any{"version": version, "people": len(people), "late": late},
		map[string]any{"user_ids": people, "version": version})
}

// NoteOpened records that a person opened the version they are asked to confirm, in this
// session (DOC-824). The page's detail calls it when it shows that version.
func (s *Service) NoteOpened(ctx context.Context, userID, pageID uuid.UUID, version int, session uuid.UUID) error {
	if session == uuid.Nil {
		return nil
	}
	_, err := s.pool.Exec(ctx, `UPDATE read_confirmations SET opened_at = now(), opened_session = $4
		WHERE page_id = $1 AND version_number = $2 AND user_id = $3 AND confirmed_at IS NULL
		  AND opened_session IS DISTINCT FROM $4`, pageID, version, userID, session)
	return err
}

// ConfirmRead records that a person read and understood a version (DOC-821): only someone
// asked, only the version readers see now, only in the session that opened it, and never
// without a session, so never through a token. It is an acknowledgement, not a proof of
// understanding, as the guide says.
func (s *Service) ConfirmRead(ctx context.Context, c identity.Caller, id uuid.UUID, version int, session uuid.UUID) error {
	if session == uuid.Nil || c.UserID == uuid.Nil {
		return apperr.Forbidden()
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		var approved *int
		err = tx.QueryRow(ctx, "SELECT approved_version FROM page_control WHERE page_id = $1 AND state_kind <> 'obsolete'", id).Scan(&approved)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Forbidden()
		}
		if err != nil {
			return err
		}
		if approved == nil || *approved != version {
			return apperr.Conflict("not_current_version", "Only the version readers see now can be confirmed.")
		}
		var opened *uuid.UUID
		var confirmed bool
		err = tx.QueryRow(ctx, `SELECT opened_session, confirmed_at IS NOT NULL FROM read_confirmations
			WHERE page_id = $1 AND version_number = $2 AND user_id = $3 FOR UPDATE`, id, version, c.UserID).Scan(&opened, &confirmed)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Forbidden()
		}
		if err != nil {
			return err
		}
		switch {
		case confirmed:
			return apperr.Conflict("already_confirmed", "You confirmed this version already.")
		case opened == nil || *opened != session:
			return apperr.Conflict("not_opened", "Open this version first, then confirm.")
		}
		if _, err := tx.Exec(ctx, "UPDATE read_confirmations SET confirmed_at = now() WHERE page_id = $1 AND version_number = $2 AND user_id = $3",
			id, version, c.UserID); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "page.read_confirmed", "page", id, p.SpaceID, map[string]any{"version": version, "user_id": c.UserID})
	})
}

// ReadPerson is one person asked to confirm a version, for the list of DOC-823.
type ReadPerson struct {
	UserID      uuid.UUID  `json:"user_id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Version     int        `json:"version"`
	AskedAt     time.Time  `json:"asked_at"`
	Late        bool       `json:"late"`
	ConfirmedAt *time.Time `json:"confirmed_at"`
}

// maxReadPeople bounds the list of one page.
const maxReadPeople = 5000

// ReadList lists who confirmed which version of a page and who did not, for space.audit
// (DOC-012, DOC-823).
func (s *Service) ReadList(ctx context.Context, c identity.Caller, id uuid.UUID) ([]ReadPerson, error) {
	var spaceID uuid.UUID
	if err := s.pool.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", id).Scan(&spaceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound()
		}
		return nil, err
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAudit); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT rc.user_id, COALESCE(u.display_name, ''), COALESCE(u.email, ''), rc.version_number, rc.asked_at, rc.late, rc.confirmed_at
		FROM read_confirmations rc LEFT JOIN users u ON u.id = rc.user_id
		WHERE rc.page_id = $1 ORDER BY rc.version_number DESC, rc.confirmed_at IS NOT NULL, lower(u.display_name) LIMIT $2`, id, maxReadPeople)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ReadPerson])
	if out == nil {
		out = []ReadPerson{}
	}
	return out, err
}

// askLateReaders asks people who joined a group after a version was approved, as the job sees
// them (DOC-1418). Guests and people who do not view the space are never asked.
func (s *Service) askLateReaders(ctx context.Context) (int, error) {
	n := 0
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `INSERT INTO read_confirmations (page_id, version_number, user_id, late)
			SELECT x.page_id, x.version_number, x.user_id, true FROM (
				SELECT DISTINCT rq.page_id, rq.version_number, gm.user_id
				FROM read_requests rq
				JOIN page_control pc ON pc.page_id = rq.page_id AND pc.approved_version = rq.version_number AND pc.state_kind <> 'obsolete'
				JOIN pages p ON p.id = rq.page_id AND p.status = 'published'
				JOIN group_members gm ON gm.group_id = ANY (rq.groups)
				JOIN users u ON u.id = gm.user_id AND u.type = 'member' AND u.status = 'active'
				WHERE NOT EXISTS (SELECT 1 FROM read_confirmations rc
				                  WHERE rc.page_id = rq.page_id AND rc.version_number = rq.version_number AND rc.user_id = gm.user_id)
				  AND p.space_id IN (SELECT ps.space_id FROM perm_spaces(u.id, 'space.view') ps)
				LIMIT $1) x
			ON CONFLICT DO NOTHING RETURNING page_id, version_number, user_id`, maxDuePass)
		if err != nil {
			return err
		}
		type late struct {
			page    uuid.UUID
			version int
			user    uuid.UUID
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (late, error) {
			var l late
			return l, r.Scan(&l.page, &l.version, &l.user)
		})
		if err != nil {
			return err
		}
		n = len(list)
		byPage := map[uuid.UUID][]uuid.UUID{}
		versions := map[uuid.UUID]int{}
		for _, l := range list {
			byPage[l.page] = append(byPage[l.page], l.user)
			versions[l.page] = l.version
		}
		for id, people := range byPage {
			p, err := load(ctx, tx, id)
			if err != nil {
				return err
			}
			if err := askToRead(ctx, tx, systemWorkflow, p, versions[id], people, true); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}

// Periodic review.

const (
	// reviewReminderDays is how long before its review date the owner is reminded (DOC-802).
	reviewReminderDays = 30
	// reviewAgainDays is how often the owner is reminded once the date passed, while the
	// page stays approved.
	reviewAgainDays = 7
)

// scheduleReview gives a version approved into a timed state its next review date: the
// approval plus the period, or the page's own shorter one (DOC-800). Any other state has none.
func scheduleReview(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, version int, to workflow.State) error {
	if !to.Timed || to.Kind != workflow.KindApproved {
		_, err := tx.Exec(ctx, "DELETE FROM periodic_reviews WHERE page_id = $1", pageID)
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO periodic_reviews (page_id, version_number, due_at, next_at)
		SELECT $1, $2, d, GREATEST(LEAST(d - make_interval(days => $4), d), now()) FROM (
			SELECT now() + make_interval(months => LEAST($3, COALESCE((SELECT review_months FROM page_control WHERE page_id = $1), $3))) AS d) x
		ON CONFLICT (page_id) DO UPDATE SET version_number = EXCLUDED.version_number, due_at = EXCLUDED.due_at,
			next_at = EXCLUDED.next_at, reminders = 0, started_at = NULL`, pageID, version, to.Months(), reviewReminderDays)
	return err
}

// SetReviewPeriod sets a page's own review period, never longer than its state's; nil returns
// to the state's (DOC-800). It needs page.workflow or space.workflows.
func (s *Service) SetReviewPeriod(ctx context.Context, c identity.Caller, id uuid.UUID, months *int) error {
	if months != nil && (*months < 1 || *months > workflow.MaxReviewMonths) {
		return apperr.Invalid(fmt.Sprintf("A review period is 1 to %d months.", workflow.MaxReviewMonths))
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		p, ctl, def, err := readControlled(ctx, tx, id)
		if err != nil {
			return err
		}
		allowed, err := s.perms.CheckPage(ctx, tx, c.UserID, id, permissions.PageWorkflow, uuid.Nil)
		if err != nil {
			return err
		}
		if !allowed.Allowed {
			d, err := s.perms.CheckSpace(ctx, tx, c.UserID, p.SpaceID, permissions.SpaceWorkflows)
			if err != nil {
				return err
			}
			if !d.Allowed {
				return apperr.Forbidden()
			}
		}
		state, ok := def.State(ctl.approvedStateID)
		if !ok || !state.Timed {
			return apperr.Conflict("not_timed", "This page is not in a state with periodic review.")
		}
		if months != nil && *months > state.Months() {
			return apperr.Conflict("period_too_long", fmt.Sprintf("A page's own period is at most the %d months of its state.", state.Months()))
		}
		if _, err := tx.Exec(ctx, "UPDATE page_control SET review_months = $2 WHERE page_id = $1", id, months); err != nil {
			return err
		}
		// The date counts from the approval of the version readers see.
		if _, err := tx.Exec(ctx, `UPDATE periodic_reviews pr SET due_at = d, next_at = GREATEST(d - make_interval(days => $3), now()), reminders = 0
			FROM (SELECT pc.approved_at + make_interval(months => LEAST($2, COALESCE(pc.review_months, $2))) AS d
			      FROM page_control pc WHERE pc.page_id = $1) x
			WHERE pr.page_id = $1 AND pr.started_at IS NULL`, id, state.Months(), reviewReminderDays); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "page.review_period_set", "page", id, p.SpaceID, map[string]any{"months": months})
	})
}

// periodicPass acts on review dates whose moment came: a reminder before the date, and on the
// date the state's periodic review step, or another reminder while the page stays approved
// (DOC-802). The page owner is told, or the space owners when the owner left (DOC-805).
func (s *Service) periodicPass(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, "SELECT page_id FROM periodic_reviews WHERE next_at <= now() ORDER BY next_at LIMIT $1", maxDuePass)
	if err != nil {
		return 0, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	var failed []error
	for _, id := range list {
		if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return s.actOnReviewDate(ctx, tx, id) }); err != nil {
			failed = append(failed, fmt.Errorf("pages: review date of page %s: %w", id, err))
		}
	}
	return len(list), errors.Join(failed...)
}

func (s *Service) actOnReviewDate(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	claimed, err := claimPage(ctx, tx, id)
	if err != nil || !claimed {
		return err
	}
	var due bool
	err = tx.QueryRow(ctx, "SELECT due_at <= now() FROM periodic_reviews WHERE page_id = $1 AND next_at <= now() FOR UPDATE SKIP LOCKED", id).Scan(&due)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	p, ctl, def, err := readControlled(ctx, tx, id)
	var known *apperr.Error
	if errors.As(err, &known) {
		_, err = tx.Exec(ctx, "DELETE FROM periodic_reviews WHERE page_id = $1", id)
		return err
	}
	if err != nil {
		return err
	}
	// Only a page that sits in its approved state comes up; one already in a round waits.
	if !ctl.approved() || ctl.stateID != ctl.approvedStateID || ctl.headVersion != *ctl.approvedVersion {
		_, err := tx.Exec(ctx, "UPDATE periodic_reviews SET next_at = now() + make_interval(days => $2) WHERE page_id = $1", id, reviewAgainDays)
		return err
	}
	if due {
		if tr, ok := def.Outcome(ctl.stateID, workflow.OutcomeReviewDue); ok {
			// A review that cannot start, such as one where nobody may review, leaves the page
			// approved: its partial writes go back with the savepoint, and the owner hears.
			err := pgx.BeginFunc(ctx, tx, func(sp pgx.Tx) error { return s.startPeriodicReview(ctx, sp, p, ctl, def, tr) })
			var refused *apperr.Error
			if !errors.As(err, &refused) {
				return err
			}
			if err := recordAudit(ctx, tx, systemWorkflow, "page.periodic_review_refused", "page", p.ID, p.SpaceID,
				map[string]any{"transition": tr.ID, "code": refused.Code}); err != nil {
				return err
			}
		}
	}
	next := "due_at"
	if due {
		next = "now() + make_interval(days => " + fmt.Sprint(reviewAgainDays) + ")"
	}
	if _, err := tx.Exec(ctx, "UPDATE periodic_reviews SET reminders = reminders + 1, next_at = "+next+" WHERE page_id = $1", id); err != nil {
		return err
	}
	return s.remindOwner(ctx, tx, p, due)
}

// remindOwner tells the page owner of its review date, or the space owners when the owner
// was disabled or is gone (DOC-805, DOC-1223).
func (s *Service) remindOwner(ctx context.Context, tx pgx.Tx, p Page, due bool) error {
	rows, err := tx.Query(ctx, `SELECT u.id FROM users u WHERE u.id = $1 AND u.status = 'active' AND u.type = 'member'
		UNION
		SELECT x.id FROM (
			SELECT g.subject_id AS id FROM space_grants g WHERE g.space_id = $2 AND g.permission = 'space.administer' AND g.subject_type = 'user'
			UNION SELECT gm.user_id FROM space_grants g JOIN group_members gm ON gm.group_id = g.subject_id
			      WHERE g.space_id = $2 AND g.permission = 'space.administer' AND g.subject_type = 'group') x
		JOIN users su ON su.id = x.id AND su.status = 'active' AND su.type = 'member'
		WHERE NOT EXISTS (SELECT 1 FROM users o WHERE o.id = $1 AND o.status = 'active' AND o.type = 'member')`, p.OwnerID, p.SpaceID)
	if err != nil {
		return err
	}
	people, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	return recordEvent(ctx, tx, systemWorkflow, "page.periodic_review", p, map[string]any{"due": due, "people": len(people)},
		map[string]any{"user_ids": people})
}

// startPeriodicReview takes the state's step into review on the current text, as the system;
// readers keep the approved version while it runs (DOC-802).
func (s *Service) startPeriodicReview(ctx context.Context, tx pgx.Tx, p Page, ctl control, def workflow.Definition, tr workflow.Transition) error {
	to, ok := def.State(tr.To)
	if !ok || to.Review == nil {
		return fmt.Errorf("pages: workflow %s version %d has no review state %s", ctl.workflowID, ctl.workflowVersion, tr.To)
	}
	if _, err := tx.Exec(ctx, "UPDATE periodic_reviews SET started_at = now(), next_at = NULL WHERE page_id = $1", p.ID); err != nil {
		return err
	}
	round, err := openRound(ctx, tx, p.ID, ctl)
	if err != nil {
		return err
	}
	if err := systemAnswer(ctx, tx, p.ID, &round, ctl.stateID, to.ID, causePeriodicReview, ctl.headVersion); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE page_control SET state_id = $2, state_kind = $3, updated_at = now() WHERE page_id = $1", p.ID, to.ID, to.Kind); err != nil {
		return err
	}
	if err := recordAudit(ctx, tx, systemWorkflow, "page.periodic_review_started", "page", p.ID, p.SpaceID,
		map[string]any{"transition": tr.ID, "from": ctl.stateID, "to": to.ID, "version": ctl.headVersion}); err != nil {
		return err
	}
	ctl.stateID, ctl.stateKind = to.ID, to.Kind
	asked, err := s.enterReview(ctx, tx, p, ctl, round, to)
	if err != nil {
		return err
	}
	if err := s.remindOwner(ctx, tx, p, true); err != nil {
		return err
	}
	return s.askReviewers(ctx, tx, systemWorkflow, p, to, asked)
}

// causePeriodicReview marks the system's step into a periodic review (DOC-105).
const causePeriodicReview = "periodic_review"
