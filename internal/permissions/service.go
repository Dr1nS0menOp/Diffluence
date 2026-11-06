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
	// An account past its end date is not active, from that moment on.
	err := q.QueryRow(ctx, `SELECT type, status = 'active' AND (expires_at IS NULL OR expires_at > now())
		FROM users WHERE id = $1`, id).Scan(&u.Type, &u.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, nil // an unknown user is simply inactive
	}
	if err != nil {
		return u, err
	}
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
	err := q.QueryRow(ctx, "SELECT space_id, status, created_by, kind, archived_at IS NOT NULL FROM pages WHERE id = $1", id).
		Scan(&spaceID, &p.Status, &p.CreatedBy, &p.Kind, &p.Archived)
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

// CheckPerson decides whether a viewer may see a person. An unknown id is simply
// someone they may not see.
func (s *Service) CheckPerson(ctx context.Context, q db.Querier, viewerID, targetID uuid.UUID) (bool, error) {
	viewer, err := s.LoadUser(ctx, q, viewerID)
	if err != nil {
		return false, err
	}
	target := PersonFacts{ID: targetID}
	err = q.QueryRow(ctx, `SELECT t.type, EXISTS (
			SELECT 1 FROM space_grants a
			JOIN spaces sp ON sp.id = a.space_id AND sp.status <> 'trashed'
			JOIN space_grants b ON b.space_id = a.space_id AND b.permission = 'space.view'
			WHERE a.permission = 'space.view'
			  AND ((a.subject_type = 'user' AND a.subject_id = $1)
			    OR (a.subject_type = 'group' AND a.subject_id IN (SELECT group_id FROM group_members WHERE user_id = $1)))
			  AND ((b.subject_type = 'user' AND b.subject_id = t.id)
			    OR (b.subject_type = 'group' AND b.subject_id IN (SELECT group_id FROM group_members WHERE user_id = t.id))))
		FROM users t WHERE t.id = $2`, viewerID, targetID).Scan(&target.Type, &target.SharedSpace)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return EvaluatePerson(viewer, target), nil
}

// Administrators names who holds administer in each space, directly or through a
// group. Everyone who can see a space may know who to ask (SPC-013).
func (s *Service) Administrators(ctx context.Context, q db.Querier, spaceIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	rows, err := q.Query(ctx, `SELECT g.space_id, COALESCE(u.display_name, gr.name)
		FROM space_grants g
		LEFT JOIN users u ON g.subject_type = 'user' AND u.id = g.subject_id
		LEFT JOIN groups gr ON g.subject_type = 'group' AND gr.id = g.subject_id
		WHERE g.permission = $1 AND g.space_id = ANY($2) AND (u.id IS NOT NULL OR gr.id IS NOT NULL)
		ORDER BY 2`, SpaceAdminister, spaceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]string{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = append(out[id], name)
	}
	return out, rows.Err()
}
