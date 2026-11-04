// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
)

// Service loads facts and answers permission questions.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// LoadUser reads what evaluation needs to know about a user.
func (s *Service) LoadUser(ctx context.Context, q db.Querier, id uuid.UUID) (UserFacts, error) {
	u := UserFacts{ID: id}
	var status string
	err := q.QueryRow(ctx, "SELECT type, status FROM users WHERE id = $1", id).Scan(&u.Type, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, nil // an unknown user is simply inactive
	}
	if err != nil {
		return u, err
	}
	u.Active = status == "active"
	rows, err := q.Query(ctx, "SELECT group_id FROM group_members WHERE user_id = $1", id)
	if err != nil {
		return u, err
	}
	if u.Groups, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
		return u, err
	}
	rows, err = q.Query(ctx, `SELECT DISTINCT permission FROM global_grants
		WHERE (subject_type = 'user' AND subject_id = $1)
		   OR (subject_type = 'group' AND subject_id = ANY($2))`, id, u.Groups)
	if err != nil {
		return u, err
	}
	u.Global, err = pgx.CollectRows(rows, pgx.RowTo[string])
	return u, err
}

// LoadSpace reads a space and its grants.
func (s *Service) LoadSpace(ctx context.Context, q db.Querier, id uuid.UUID) (SpaceFacts, error) {
	f := SpaceFacts{ID: id}
	err := q.QueryRow(ctx, "SELECT status FROM spaces WHERE id = $1", id).Scan(&f.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, apperr.NotFound()
	}
	if err != nil {
		return f, err
	}
	rows, err := q.Query(ctx, "SELECT subject_type, subject_id, permission FROM space_grants WHERE space_id = $1", id)
	if err != nil {
		return f, err
	}
	f.Grants, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Grant, error) {
		var g Grant
		err := r.Scan(&g.Subject.Type, &g.Subject.ID, &g.Permission)
		return g, err
	})
	return f, err
}

// LoadPage reads a page, its space and the restrictions that apply to it.
func (s *Service) LoadPage(ctx context.Context, q db.Querier, id uuid.UUID) (SpaceFacts, PageFacts, error) {
	p := PageFacts{ID: id}
	var spaceID uuid.UUID
	err := q.QueryRow(ctx, "SELECT space_id, status, created_by FROM pages WHERE id = $1", id).
		Scan(&spaceID, &p.Status, &p.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return SpaceFacts{}, p, apperr.NotFound()
	}
	if err != nil {
		return SpaceFacts{}, p, err
	}
	space, err := s.LoadSpace(ctx, q, spaceID)
	if err != nil {
		return space, p, err
	}
	rows, err := q.Query(ctx, `SELECT r.page_id, r.kind, r.subject_type, r.subject_id
		FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id
		WHERE t.descendant_id = $1 AND (r.kind = 'view' OR t.depth = 0)
		ORDER BY t.depth DESC, r.page_id`, id)
	if err != nil {
		return space, p, err
	}
	defer rows.Close()
	for rows.Next() {
		var pageID uuid.UUID
		var kind string
		var sub Subject
		if err := rows.Scan(&pageID, &kind, &sub.Type, &sub.ID); err != nil {
			return space, p, err
		}
		if kind == "edit" {
			p.EditSubjects = append(p.EditSubjects, sub)
			continue
		}
		if n := len(p.ViewChain); n > 0 && p.ViewChain[n-1].PageID == pageID {
			p.ViewChain[n-1].Subjects = append(p.ViewChain[n-1].Subjects, sub)
		} else {
			p.ViewChain = append(p.ViewChain, RestrictionLevel{PageID: pageID, Subjects: []Subject{sub}})
		}
	}
	return space, p, rows.Err()
}

// CheckSpace evaluates a space-level action.
func (s *Service) CheckSpace(ctx context.Context, q db.Querier, userID, spaceID uuid.UUID, action string) (Decision, error) {
	u, err := s.LoadUser(ctx, q, userID)
	if err != nil {
		return Decision{}, err
	}
	sp, err := s.LoadSpace(ctx, q, spaceID)
	if err != nil {
		return Decision{}, err
	}
	return Evaluate(Input{User: u, Space: sp, Action: action}), nil
}

// CheckPage evaluates an action on a page. owner matters only for "delete own"
// on objects other than the page; pass uuid.Nil to use the page's creator.
func (s *Service) CheckPage(ctx context.Context, q db.Querier, userID, pageID uuid.UUID, action string, owner uuid.UUID) (Decision, error) {
	u, err := s.LoadUser(ctx, q, userID)
	if err != nil {
		return Decision{}, err
	}
	sp, p, err := s.LoadPage(ctx, q, pageID)
	if err != nil {
		return Decision{}, err
	}
	if owner == uuid.Nil {
		owner = p.CreatedBy
	}
	return Evaluate(Input{User: u, Space: sp, Page: &p, Action: action, Owner: owner}), nil
}

func refusal(d Decision, denied apperr.Denied) error {
	e := apperr.Forbidden()
	if !d.Visible {
		e = apperr.NotFound()
	}
	e.Denied = &denied
	return e
}

// RequireSpace returns nil when allowed, otherwise a not-found or forbidden
// error that the HTTP layer audits as denied.
func (s *Service) RequireSpace(ctx context.Context, q db.Querier, userID, spaceID uuid.UUID, action string) error {
	d, err := s.CheckSpace(ctx, q, userID, spaceID, action)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return refusal(d, apperr.Denied{Action: action, ObjectType: "space", ObjectID: spaceID.String(), SpaceID: spaceID.String()})
	}
	return nil
}

// RequirePage is RequireSpace for an action on a page.
func (s *Service) RequirePage(ctx context.Context, q db.Querier, userID, pageID uuid.UUID, action string, owner uuid.UUID) error {
	d, err := s.CheckPage(ctx, q, userID, pageID, action, owner)
	if err != nil {
		return err
	}
	if !d.Allowed {
		return refusal(d, apperr.Denied{Action: action, ObjectType: "page", ObjectID: pageID.String()})
	}
	return nil
}
