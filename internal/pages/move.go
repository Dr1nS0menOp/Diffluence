// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	maxBulkPages = 200
	// largeCopyPages is where a copy stops being something to wait for (PAG-050).
	largeCopyPages = 50
)

// lockSpaces takes the tree locks of two spaces in a fixed order, so two moves in
// opposite directions cannot wait for each other.
func lockSpaces(ctx context.Context, tx pgx.Tx, a, b uuid.UUID) error {
	if a == b {
		return lockTree(ctx, tx, a)
	}
	first, second := a, b
	if first.String() > second.String() {
		first, second = second, first
	}
	if err := lockTree(ctx, tx, first); err != nil {
		return err
	}
	return lockTree(ctx, tx, second)
}

func isHomePage(ctx context.Context, q db.Querier, p Page) (bool, error) {
	var home bool
	err := q.QueryRow(ctx, "SELECT home_page_id IS NOT DISTINCT FROM $2 FROM spaces WHERE id = $1", p.SpaceID, p.ID).Scan(&home)
	return home, err
}

// createPermissionsOf lists the permissions that make the kinds of item in a branch, each
// once and in a fixed order.
func createPermissionsOf(ctx context.Context, q db.Querier, tree []uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, "SELECT DISTINCT kind FROM pages WHERE id = ANY($1)", tree)
	if err != nil {
		return nil, err
	}
	kinds, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	var out []string
	for _, kind := range kinds {
		if p := permissions.CreatePermission(kind); !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	slices.Sort(out)
	return out, nil
}

// branchHasHiddenPages reports whether the person cannot see all of a branch.
func (s *Service) branchHasHiddenPages(ctx context.Context, q db.Querier, userID uuid.UUID, tree []uuid.UUID) (bool, error) {
	for _, id := range tree {
		d, err := s.perms.CheckPage(ctx, q, userID, id, permissions.ActionPageView, uuid.Nil)
		if err != nil {
			return false, err
		}
		if !d.Visible {
			return true, nil
		}
	}
	return false, nil
}

// inheritedChange compares the view restrictions above a page now and after a move.
func inheritedChange(ctx context.Context, q db.Querier, p Page, parent *uuid.UUID) (lost, gained []uuid.UUID, err error) {
	var before, after []uuid.UUID
	if p.ParentID != nil {
		if before, err = viewRestricted(ctx, q, *p.ParentID); err != nil {
			return nil, nil, err
		}
	}
	if parent != nil {
		if after, err = viewRestricted(ctx, q, *parent); err != nil {
			return nil, nil, err
		}
	}
	for _, id := range before {
		if !slices.Contains(after, id) {
			lost = append(lost, id)
		}
	}
	for _, id := range after {
		if !slices.Contains(before, id) {
			gained = append(gained, id)
		}
	}
	return lost, gained, nil
}

// tablesWithSpace keep a space_id beside their page. A test walks the schema, so a new
// table cannot be forgotten here.
var tablesWithSpace = []string{"comments", "page_tasks", "notifications", "access_requests"}

// moveRowsOfPages keeps what belongs to moved pages in step with them, also what is
// in the trash: an item stays with the administrators of the space its page is in.
func moveRowsOfPages(ctx context.Context, tx pgx.Tx, pageIDs []uuid.UUID, spaceID uuid.UUID) error {
	for _, table := range tablesWithSpace {
		sql := "UPDATE " + pgx.Identifier{table}.Sanitize() + " SET space_id = $2 WHERE page_id = ANY($1)"
		if _, err := tx.Exec(ctx, sql, pageIDs, spaceID); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, `UPDATE trash_items SET space_id = $2
		WHERE object_id = ANY($1)
		   OR object_id IN (SELECT id FROM attachments WHERE page_id = ANY($1))
		   OR object_id IN (SELECT id FROM comments WHERE page_id = ANY($1))`, pageIDs, spaceID)
	return err
}

// moveToSpace moves a whole branch to another space. Someone who cannot see all of
// a branch cannot change who reads it (spec 011, D13).
func (s *Service) moveToSpace(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, in MoveInput) (Page, error) {
	target := *in.SpaceID
	if err := lockSpaces(ctx, tx, p.SpaceID, target); err != nil {
		return Page{}, err
	}
	for _, action := range []string{permissions.PageMoveOut, permissions.CreatePermission(p.Kind)} {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, p.ID, action, uuid.Nil); err != nil {
			return Page{}, err
		}
	}
	home, err := isHomePage(ctx, tx, p)
	if err != nil {
		return Page{}, err
	}
	if home {
		return Page{}, apperr.Conflict("home_page", "The home page of a space cannot move.")
	}
	tree, err := subtree(ctx, tx, p.ID)
	if err != nil {
		return Page{}, err
	}
	// The branch arrives as new items in the target, so each kind in it asks for the
	// permission that makes that kind there (spec 045).
	needed, err := createPermissionsOf(ctx, tx, tree)
	if err != nil {
		return Page{}, err
	}
	for _, action := range append([]string{permissions.SpaceView}, needed...) {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, target, action); err != nil {
			return Page{}, err
		}
	}
	hidden, err := s.branchHasHiddenPages(ctx, tx, c.UserID, tree)
	if err != nil {
		return Page{}, err
	}
	if hidden {
		return Page{}, apperr.Conflict("contains_hidden_pages", "This page has pages below it that you cannot see, so it cannot move to another space.")
	}
	if in.ParentID != nil {
		if err := s.checkParent(ctx, tx, c, placement{spaceID: target, parent: *in.ParentID, kind: p.Kind}); err != nil {
			return Page{}, err
		}
	}
	lost, _, err := inheritedChange(ctx, tx, p, in.ParentID)
	if err != nil {
		return Page{}, err
	}
	if len(lost) > 0 {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, p.ID, permissions.RestrictionManage, uuid.Nil); err != nil {
			return Page{}, err
		}
	}
	if err := relink(ctx, tx, p.ID, in.ParentID); err != nil {
		return Page{}, err
	}
	if _, err := tx.Exec(ctx, "UPDATE pages SET space_id = $2, updated_at = now() WHERE id = ANY($1)", tree, target); err != nil {
		return Page{}, err
	}
	if err := s.attachments.MoveToSpace(ctx, tx, tree, target); err != nil {
		return Page{}, err
	}
	if err := moveRowsOfPages(ctx, tx, tree, target); err != nil {
		return Page{}, err
	}
	moved := p
	moved.SpaceID = target
	if err := reposition(ctx, tx, moved, in); err != nil {
		return Page{}, err
	}
	out, err := load(ctx, tx, p.ID)
	if err != nil {
		return Page{}, err
	}
	return out, record(ctx, tx, c, "page.moved", out, map[string]any{"from_space": p.SpaceID, "to_space": target, "pages": len(tree)})
}

// MoveImpact is what a move would change, said before it happens (PAG-044). It
// names restrictions and spaces, never the people of a target space.
type MoveImpact struct {
	Pages              int      `json:"pages"`
	Hidden             bool     `json:"hidden"`
	RestrictionsLost   []string `json:"restrictions_lost"`
	RestrictionsGained []string `json:"restrictions_gained"`
	NeedsManage        bool     `json:"needs_manage"`
	OtherSpace         bool     `json:"other_space"`
	// Fits is false when the target space has no room for the files of the branch.
	Fits bool `json:"fits"`
}

func titlesOf(ctx context.Context, q db.Querier, ids []uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, "SELECT title FROM pages WHERE id = ANY($1) ORDER BY lower(title)", ids)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

// MoveImpact says what moving a page would change.
func (s *Service) MoveImpact(ctx context.Context, c identity.Caller, id uuid.UUID, spaceID, parent *uuid.UUID) (MoveImpact, error) {
	var out MoveImpact
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return out, err
	}
	p, err := load(ctx, s.pool, id)
	if err != nil {
		return out, err
	}
	// Moving to another space asks what the move itself asks (spec 040).
	action := permissions.PageEdit
	if spaceID != nil && *spaceID != p.SpaceID {
		action = permissions.PageMoveOut
	}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, action, uuid.Nil); err != nil {
		return out, err
	}
	if spaceID != nil && *spaceID != p.SpaceID {
		out.OtherSpace = true
		if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, *spaceID, permissions.SpaceView); err != nil {
			return out, err
		}
	}
	if parent != nil {
		if err := s.perms.RequirePage(ctx, s.pool, c.UserID, *parent, permissions.ActionPageView, uuid.Nil); err != nil {
			return out, err
		}
	}
	tree, err := subtree(ctx, s.pool, id)
	if err != nil {
		return out, err
	}
	out.Pages = len(tree)
	out.Fits = true
	if out.OtherSpace {
		if out.Hidden, err = s.branchHasHiddenPages(ctx, s.pool, c.UserID, tree); err != nil {
			return out, err
		}
		if out.Fits, err = attachments.FitsMove(ctx, s.pool, tree, *spaceID); err != nil {
			return out, err
		}
	}
	lost, gained, err := inheritedChange(ctx, s.pool, p, parent)
	if err != nil {
		return out, err
	}
	if out.RestrictionsLost, err = titlesOf(ctx, s.pool, lost); err != nil {
		return out, err
	}
	if out.RestrictionsGained, err = titlesOf(ctx, s.pool, gained); err != nil {
		return out, err
	}
	if len(lost) > 0 {
		d, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, permissions.RestrictionManage, uuid.Nil)
		if err != nil {
			return out, err
		}
		out.NeedsManage = !d.Allowed
	}
	return out, nil
}

// CopyImpact is what a copy would lose, said before it happens.
type CopyImpact struct {
	RestrictionsLost []string `json:"restrictions_lost"`
	NeedsManage      bool     `json:"needs_manage"`
	// Fits is false when the target space has no room for the files of the page.
	Fits bool `json:"fits"`
}

// CopyImpact says which restrictions from above a copy would not take along.
func (s *Service) CopyImpact(ctx context.Context, c identity.Caller, id uuid.UUID, spaceID, parent *uuid.UUID) (CopyImpact, error) {
	var out CopyImpact
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return out, err
	}
	p, err := load(ctx, s.pool, id)
	if err != nil {
		return out, err
	}
	target := p.SpaceID
	if spaceID != nil && *spaceID != p.SpaceID {
		if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, *spaceID, permissions.SpaceView); err != nil {
			return out, err
		}
		target = *spaceID
	}
	if parent != nil {
		if err := s.perms.RequirePage(ctx, s.pool, c.UserID, *parent, permissions.ActionPageView, uuid.Nil); err != nil {
			return out, err
		}
	}
	if out.Fits, err = attachments.FitsCopy(ctx, s.pool, id, target); err != nil {
		return out, err
	}
	lost, _, err := inheritedChange(ctx, s.pool, p, parent)
	if err != nil {
		return out, err
	}
	if out.RestrictionsLost, err = titlesOf(ctx, s.pool, lost); err != nil {
		return out, err
	}
	if len(lost) > 0 {
		d, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, permissions.RestrictionManage, uuid.Nil)
		if err != nil {
			return out, err
		}
		out.NeedsManage = !d.Allowed
	}
	return out, nil
}

// BulkFailure says why one page of a selection did not work.
type BulkFailure struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// reasonOf gives words for a refusal, and nothing about what went wrong inside.
func reasonOf(err error) string {
	var e *apperr.Error
	if errors.As(err, &e) && e.Message != "" {
		return e.Message
	}
	return "This page could not be changed."
}

// BulkMove moves pages one by one. A selection is never a permission: each page
// goes through Move, and one refusal does not stop the rest.
func (s *Service) BulkMove(ctx context.Context, c identity.Caller, ids []uuid.UUID, in MoveInput) (int, []BulkFailure, error) {
	if len(ids) == 0 || len(ids) > maxBulkPages {
		return 0, nil, apperr.Invalid("Choose between 1 and 200 pages.")
	}
	moved := 0
	failed := []BulkFailure{}
	for _, id := range ids {
		if _, err := s.Move(ctx, c, id, in); err != nil {
			failed = append(failed, BulkFailure{ID: id, Reason: reasonOf(err)})
			continue
		}
		moved++
	}
	return moved, failed, nil
}

// SortChildren puts the pages below a page in a new order, for good.
func (s *Service) SortChildren(ctx context.Context, c identity.Caller, id uuid.UUID, by string) error {
	if by != "title" && by != "created" && by != "changed" {
		return apperr.Invalid("Pages sort by title, created or changed.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := lockTree(ctx, tx, p.SpaceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE pages SET position = o.ord FROM (
				SELECT id, row_number() OVER (ORDER BY
					CASE WHEN $2 = 'title' THEN lower(title) END,
					CASE WHEN $2 = 'created' THEN created_at END,
					CASE WHEN $2 = 'changed' THEN updated_at END DESC, id) - 1 AS ord
				FROM pages WHERE parent_id = $1 AND kind = ANY($3)) o
			WHERE pages.id = o.id`, id, by, treeKinds); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("page.children_sorted", "page", id.String(), p.SpaceID.String(), map[string]any{"by": by}))
		return err
	})
}
