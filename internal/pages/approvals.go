// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Round states and answers.
const (
	roundOpen = "open"

	AnswerApprove = "approve"
	AnswerChanges = "changes"

	maxApprovers       = 10
	maxApprovalMessage = 2000
)

// Approval is the mark on a page: an open round, or the approved version.
type Approval struct {
	RoundID         uuid.UUID `json:"round_id"`
	Status          string    `json:"status"`
	ApprovedVersion int       `json:"approved_version"`
	IsCurrent       bool      `json:"is_current"`
	Asked           int       `json:"asked"`
	Answered        int       `json:"answered"`
	MustAnswer      bool      `json:"must_answer"`
}

// ApprovalAnswer is what one person said in a round.
type ApprovalAnswer struct {
	UserID     uuid.UUID  `json:"user_id"`
	Name       string     `json:"name"`
	Answer     string     `json:"answer"`
	Comment    string     `json:"comment"`
	AnsweredAt *time.Time `json:"answered_at"`
}

// ApprovalRound is one question to a group of people.
type ApprovalRound struct {
	ID              uuid.UUID        `json:"id"`
	PageID          uuid.UUID        `json:"page_id"`
	AskedBy         uuid.UUID        `json:"asked_by"`
	AskedByName     string           `json:"asked_by_name"`
	Message         string           `json:"message"`
	DueOn           string           `json:"due_on"`
	Title           string           `json:"title"`
	BaseVersion     int              `json:"base_version"`
	ApprovedVersion int              `json:"approved_version"`
	Status          string           `json:"status"`
	CreatedAt       time.Time        `json:"created_at"`
	Answers         []ApprovalAnswer `json:"answers"`
	Document        json.RawMessage  `json:"document,omitempty"`
}

func approvalOf(ctx context.Context, q db.Querier, userID uuid.UUID, p Page) (*Approval, error) {
	var a Approval
	err := q.QueryRow(ctx, `SELECT r.id, r.status, COALESCE(r.approved_version, 0),
			(SELECT count(*) FROM approval_answers x WHERE x.round_id = r.id),
			(SELECT count(*) FROM approval_answers x WHERE x.round_id = r.id AND x.answer IS NOT NULL),
			EXISTS (SELECT 1 FROM approval_answers x WHERE x.round_id = r.id AND x.user_id = $2 AND x.answer IS NULL)
		FROM approval_rounds r
		WHERE r.page_id = $1 AND r.status <> 'closed' ORDER BY r.created_at DESC LIMIT 1`, p.ID, userID).
		Scan(&a.RoundID, &a.Status, &a.ApprovedVersion, &a.Asked, &a.Answered, &a.MustAnswer)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.IsCurrent = a.ApprovedVersion > 0 && a.ApprovedVersion == p.CurrentVersion
	return &a, nil
}

// frozenText is what a round asks about: the person's draft, or else the page.
func frozenText(ctx context.Context, tx pgx.Tx, p Page, userID uuid.UUID) (string, []byte, error) {
	var title string
	var doc []byte
	err := tx.QueryRow(ctx, "SELECT title, document FROM drafts WHERE page_id = $1 AND user_id = $2", p.ID, userID).Scan(&title, &doc)
	if err == nil {
		if title == "" {
			title = p.Title
		}
		return title, doc, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, err
	}
	if p.CurrentVersion == 0 {
		return "", nil, apperr.Conflict("nothing_to_approve", "Write something first: there is no text to approve yet.")
	}
	err = tx.QueryRow(ctx, "SELECT title, document FROM page_versions WHERE page_id = $1 AND number = $2", p.ID, p.CurrentVersion).Scan(&title, &doc)
	return title, doc, err
}

// RoundInput asks people to approve a page.
type RoundInput struct {
	UserIDs []uuid.UUID
	Message string
	DueOn   string
}

func (in RoundInput) validate() error {
	if len(in.UserIDs) == 0 || len(in.UserIDs) > maxApprovers {
		return apperr.Invalid("Ask between one and ten people.")
	}
	if utf8.RuneCountInString(in.Message) > maxApprovalMessage {
		return apperr.Invalid("A message can have at most 2000 characters.")
	}
	if in.DueOn != "" {
		if _, err := parseDate(in.DueOn); err != nil {
			return err
		}
	}
	return nil
}

// insertRound writes the round and closes the one before it. A new round replaces
// an open one, so a page has at most one question open.
func (s *Service) insertRound(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, in RoundInput) (uuid.UUID, error) {
	title, doc, err := frozenText(ctx, tx, p, c.UserID)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, "UPDATE approval_rounds SET status = 'closed', closed_at = now() WHERE page_id = $1 AND status = 'open'", p.ID); err != nil {
		return uuid.Nil, err
	}
	var due *string
	if in.DueOn != "" {
		due = &in.DueOn
	}
	id := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO approval_rounds (id, page_id, asked_by, message, due_on, title, document, base_version, status)
		VALUES ($1, $2, $3, $4, $5::date, $6, $7, $8, 'open')`, id, p.ID, c.UserID, strings.TrimSpace(in.Message), due, title, doc, p.CurrentVersion); err != nil {
		return uuid.Nil, err
	}
	for _, user := range in.UserIDs {
		if _, err := tx.Exec(ctx, "INSERT INTO approval_answers (round_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", id, user); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

// AskApproval freezes the text and asks people to approve it. It answers with
// those who cannot see the page, so the person can do something about it.
func (s *Service) AskApproval(ctx context.Context, c identity.Caller, id uuid.UUID, in RoundInput) (uuid.UUID, []uuid.UUID, error) {
	if err := in.validate(); err != nil {
		return uuid.Nil, nil, err
	}
	var round uuid.UUID
	cannotSee := []uuid.UUID{}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		if round, err = s.insertRound(ctx, tx, c, p, in); err != nil {
			return err
		}
		for _, user := range in.UserIDs {
			allowed, err := s.mayReadRound(ctx, tx, user, id)
			if err != nil {
				return err
			}
			if !allowed {
				cannotSee = append(cannotSee, user)
			}
		}
		if err := record(ctx, tx, c, "page.approval_requested", p, map[string]any{"people": len(in.UserIDs)}); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "page.approval_asked", map[string]any{"page_id": id, "round_id": round, "user_ids": in.UserIDs})
		return err
	})
	return round, cannotSee, err
}

// mayReadRound: someone named in a round may read its frozen copy when they hold
// view on the space and pass every restriction (spec 011, D9).
func (s *Service) mayReadRound(ctx context.Context, q db.Querier, userID, pageID uuid.UUID) (bool, error) {
	d, err := s.perms.CheckPage(ctx, q, userID, pageID, permissions.ActionApprovalRead, uuid.Nil)
	if err != nil {
		return false, err
	}
	return d.Allowed, nil
}

// ApprovalRule is what a space asks before publishing.
type ApprovalRule struct {
	Required  bool     `json:"required"`
	Approvers []Person `json:"approvers"`
}

// Person is someone named on a screen.
type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// ApprovalRuleOf says what SetApprovalRule set, for the settings of a space.
func (s *Service) ApprovalRuleOf(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (ApprovalRule, error) {
	out := ApprovalRule{Approvers: []Person{}}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceApprovals); err != nil {
		return out, err
	}
	if err := s.pool.QueryRow(ctx, "SELECT approval_required FROM spaces WHERE id = $1", spaceID).Scan(&out.Required); err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.display_name FROM space_approvers a
		JOIN users u ON u.id = a.user_id WHERE a.space_id = $1 ORDER BY lower(u.display_name)`, spaceID)
	if err != nil {
		return out, err
	}
	people, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Person, error) {
		var p Person
		return p, r.Scan(&p.ID, &p.Name)
	})
	if err != nil {
		return out, err
	}
	if len(people) > 0 {
		out.Approvers = people
	}
	return out, nil
}

const roundColumns = `r.id, r.page_id, r.asked_by, COALESCE(u.display_name, ''), r.message, COALESCE(to_char(r.due_on, 'YYYY-MM-DD'), ''),
	r.title, r.base_version, COALESCE(r.approved_version, 0), r.status, r.created_at`

func scanRound(row pgx.Row) (ApprovalRound, error) {
	var r ApprovalRound
	err := row.Scan(&r.ID, &r.PageID, &r.AskedBy, &r.AskedByName, &r.Message, &r.DueOn, &r.Title, &r.BaseVersion, &r.ApprovedVersion, &r.Status, &r.CreatedAt)
	return r, err
}

func answersOf(ctx context.Context, q db.Querier, roundID uuid.UUID) ([]ApprovalAnswer, error) {
	rows, err := q.Query(ctx, `SELECT a.user_id, COALESCE(u.display_name, ''), COALESCE(a.answer, ''), a.comment, a.answered_at
		FROM approval_answers a LEFT JOIN users u ON u.id = a.user_id WHERE a.round_id = $1 ORDER BY lower(u.display_name)`, roundID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ApprovalAnswer])
	if out == nil {
		out = []ApprovalAnswer{}
	}
	return out, err
}

// ApprovalRounds lists the rounds of a page, newest first.
func (s *Service) ApprovalRounds(ctx context.Context, c identity.Caller, id uuid.UUID) ([]ApprovalRound, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+roundColumns+` FROM approval_rounds r LEFT JOIN users u ON u.id = r.asked_by
		WHERE r.page_id = $1 ORDER BY r.created_at DESC LIMIT 50`, id)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ApprovalRound, error) { return scanRound(r) })
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].Answers, err = answersOf(ctx, s.pool, list[i].ID); err != nil {
			return nil, err
		}
	}
	if list == nil {
		list = []ApprovalRound{}
	}
	return list, nil
}

// roundFor loads a round for someone named in it, or who can see the page.
func (s *Service) roundFor(ctx context.Context, q db.Querier, c identity.Caller, id uuid.UUID) (ApprovalRound, error) {
	r, err := scanRound(q.QueryRow(ctx, `SELECT `+roundColumns+` FROM approval_rounds r LEFT JOIN users u ON u.id = r.asked_by WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperr.NotFound()
	}
	if err != nil {
		return r, err
	}
	var named bool
	if err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM approval_answers WHERE round_id = $1 AND user_id = $2)", id, c.UserID).Scan(&named); err != nil {
		return r, err
	}
	if named {
		allowed, err := s.mayReadRound(ctx, q, c.UserID, r.PageID)
		if err != nil {
			return r, err
		}
		if allowed {
			return r, nil
		}
	}
	return r, s.perms.RequirePage(ctx, q, c.UserID, r.PageID, permissions.ActionPageView, uuid.Nil)
}

// ApprovalRoundText shows what was asked, with the frozen text.
func (s *Service) ApprovalRoundText(ctx context.Context, c identity.Caller, id uuid.UUID) (ApprovalRound, error) {
	r, err := s.roundFor(ctx, s.pool, c, id)
	if err != nil {
		return r, err
	}
	if r.Answers, err = answersOf(ctx, s.pool, id); err != nil {
		return r, err
	}
	err = s.pool.QueryRow(ctx, "SELECT document FROM approval_rounds WHERE id = $1", id).Scan(&r.Document)
	return r, err
}

// approvedVersionOf finds the version a round's text is, if it is one already.
func approvedVersionOf(ctx context.Context, tx pgx.Tx, r ApprovalRound) (*int, error) {
	var number int
	err := tx.QueryRow(ctx, `SELECT v.number FROM page_versions v JOIN approval_rounds a ON a.id = $1
		WHERE v.page_id = a.page_id AND v.title = a.title AND v.document = a.document
		ORDER BY v.number DESC LIMIT 1`, r.ID).Scan(&number)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &number, err
}

func (s *Service) finishIfApproved(ctx context.Context, tx pgx.Tx, r ApprovalRound) error {
	var waiting int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM approval_answers
		WHERE round_id = $1 AND answer IS DISTINCT FROM 'approve'`, r.ID).Scan(&waiting)
	if err != nil || waiting > 0 {
		return err
	}
	version, err := approvedVersionOf(ctx, tx, r)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE approval_rounds SET status = 'approved', approved_version = $2, closed_at = now() WHERE id = $1", r.ID, version); err != nil {
		return err
	}
	_, err = outbox.Enqueue(ctx, tx, "page.approved", map[string]any{"page_id": r.PageID, "round_id": r.ID, "user_id": r.AskedBy})
	return err
}

// AnswerApproval records what a person says. They can change their mind until the
// round ends.
func (s *Service) AnswerApproval(ctx context.Context, c identity.Caller, id uuid.UUID, answer, comment string) error {
	if answer != AnswerApprove && answer != AnswerChanges {
		return apperr.Invalid("The answer is approve or changes.")
	}
	if utf8.RuneCountInString(comment) > maxApprovalMessage {
		return apperr.Invalid("A comment can have at most 2000 characters.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := s.roundFor(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if r.Status != roundOpen {
			return apperr.Conflict("round_closed", "This round has ended.")
		}
		var asked int
		var named bool
		err = tx.QueryRow(ctx, "SELECT count(*), COALESCE(bool_or(user_id = $2), false) FROM approval_answers WHERE round_id = $1", id, c.UserID).Scan(&asked, &named)
		if err != nil {
			return err
		}
		if !named {
			return apperr.Forbidden()
		}
		if answer == AnswerApprove && r.AskedBy == c.UserID && asked > 1 {
			return apperr.Conflict("own_round", "You asked for this approval, so the others decide.")
		}
		if _, err := tx.Exec(ctx, "UPDATE approval_answers SET answer = $3, comment = $4, answered_at = now() WHERE round_id = $1 AND user_id = $2",
			id, c.UserID, answer, strings.TrimSpace(comment)); err != nil {
			return err
		}
		p, err := load(ctx, tx, r.PageID)
		if err != nil {
			return err
		}
		if err := record(ctx, tx, c, "page.approval_answered", p, map[string]any{"answer": answer}); err != nil {
			return err
		}
		return s.finishIfApproved(ctx, tx, r)
	})
}

// CloseApproval ends a round without a result.
func (s *Service) CloseApproval(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := s.roundFor(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, r.PageID, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE approval_rounds SET status = 'closed', closed_at = now() WHERE id = $1 AND status = 'open'", id); err != nil {
			return err
		}
		p, err := load(ctx, tx, r.PageID)
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.approval_closed", p, nil)
	})
}

// RemoveApprover takes a disabled person out of a round, so it can still end.
func (s *Service) RemoveApprover(ctx context.Context, c identity.Caller, id, user uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		r, err := s.roundFor(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if r.AskedBy != c.UserID {
			return apperr.Forbidden()
		}
		if _, err := tx.Exec(ctx, "DELETE FROM approval_answers WHERE round_id = $1 AND user_id = $2", id, user); err != nil {
			return err
		}
		return s.finishIfApproved(ctx, tx, r)
	})
}

// requireApproved refuses a publish in a space that demands approval, unless this
// exact text was approved.
func (s *Service) requireApproved(ctx context.Context, tx pgx.Tx, p Page, title string, doc document.Node) error {
	var required bool
	if err := tx.QueryRow(ctx, "SELECT approval_required FROM spaces WHERE id = $1", p.SpaceID).Scan(&required); err != nil {
		return err
	}
	if !required {
		return nil
	}
	raw, err := document.Marshal(doc)
	if err != nil {
		return err
	}
	var approved bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM approval_rounds r
		WHERE r.page_id = $1 AND r.status = 'approved' AND r.title = $2 AND r.document = $3::jsonb
		  AND NOT EXISTS (SELECT 1 FROM space_approvers sa JOIN pages pg ON pg.id = r.page_id
		                  WHERE sa.space_id = pg.space_id
		                    AND sa.user_id NOT IN (SELECT user_id FROM approval_answers x WHERE x.round_id = r.id)))`, p.ID, title, raw).Scan(&approved)
	if err != nil {
		return err
	}
	if !approved {
		return apperr.Conflict("approval_required", "This space needs approval first, and this exact text was not approved.")
	}
	return nil
}

// endRoundsOnPublish ties an approved round to the version with its text.
func endRoundsOnPublish(ctx context.Context, tx pgx.Tx, id uuid.UUID, version int, title string, doc document.Node) error {
	raw, err := document.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE approval_rounds SET approved_version = $2
		WHERE page_id = $1 AND status = 'approved' AND approved_version IS NULL AND title = $3 AND document = $4::jsonb`, id, version, title, raw)
	return err
}

// SetApprovalRule sets whether a space needs approval, and who always answers.
func (s *Service) SetApprovalRule(ctx context.Context, c identity.Caller, spaceID uuid.UUID, required bool, approvers []uuid.UUID) error {
	if len(approvers) > maxApprovers {
		return apperr.Invalid("A space can have at most ten fixed approvers.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceApprovals); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET approval_required = $2 WHERE id = $1", spaceID, required); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_approvers WHERE space_id = $1", spaceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO space_approvers (space_id, user_id)
			SELECT $1, id FROM unnest($2::uuid[]) AS id ON CONFLICT DO NOTHING`, spaceID, approvers); err != nil {
			return err
		}
		return recordSpace(ctx, tx, c, spaceID, "space.approval_rule_changed", map[string]any{"required": required, "approvers": len(approvers)})
	})
}
