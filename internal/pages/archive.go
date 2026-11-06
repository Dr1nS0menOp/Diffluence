// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const maxArchivedPages = 1000

// directChildren lists the pages right below a page that are still in the tree.
func directChildren(ctx context.Context, q db.Querier, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT id FROM pages
		WHERE parent_id = $1 AND status <> 'trashed' AND archived_at IS NULL ORDER BY position, id`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// moveUp hands a page to a new parent because its old parent leaves the tree. A
// restriction it would lose needs the right to change it (PAG-071).
func (s *Service) moveUp(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, parent *uuid.UUID) error {
	p, err := load(ctx, tx, id)
	if err != nil {
		return err
	}
	lost, _, err := inheritedChange(ctx, tx, p, parent)
	if err != nil {
		return err
	}
	if len(lost) > 0 {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.RestrictionManage, uuid.Nil); err != nil {
			return err
		}
	}
	if err := relink(ctx, tx, id, parent); err != nil {
		return err
	}
	return reposition(ctx, tx, p, MoveInput{ParentID: parent})
}

// archiveBranch archives a page and, with children, what the person may archive
// below it. What they may not archive stays in the tree, a level higher.
func (s *Service) archiveBranch(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, parent *uuid.UUID, withChildren bool, at time.Time) (int, error) {
	if _, err := tx.Exec(ctx, "UPDATE pages SET archived_at = $2, archived_by = $3, updated_at = now() WHERE id = $1", id, at, c.UserID); err != nil {
		return 0, err
	}
	archived := 1
	children, err := directChildren(ctx, tx, id)
	if err != nil {
		return 0, err
	}
	for _, child := range children {
		allowed := false
		if withChildren {
			d, err := s.perms.CheckPage(ctx, tx, c.UserID, child, permissions.PageArchive, uuid.Nil)
			if err != nil {
				return 0, err
			}
			allowed = d.Allowed
		}
		if !allowed {
			if err := s.moveUp(ctx, tx, c, child, parent); err != nil {
				return 0, err
			}
			continue
		}
		n, err := s.archiveBranch(ctx, tx, c, child, parent, true, at)
		if err != nil {
			return 0, err
		}
		archived += n
	}
	return archived, nil
}

// Archive takes a page out of the way. It keeps its address and its place in
// page_tree, so it keeps the restrictions it inherits while it is away (D11).
func (s *Service) Archive(ctx context.Context, c identity.Caller, id uuid.UUID, withChildren bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageArchive, uuid.Nil); err != nil {
			return err
		}
		if err := lockTree(ctx, tx, p.SpaceID); err != nil {
			return err
		}
		home, err := isHomePage(ctx, tx, p)
		if err != nil {
			return err
		}
		if home {
			return apperr.Conflict("home_page", "The home page of a space cannot be archived.")
		}
		if p.ArchivedAt != nil {
			return apperr.Conflict("already_archived", "This page is in the archive already.")
		}
		var at time.Time
		if err := tx.QueryRow(ctx, "SELECT now()").Scan(&at); err != nil {
			return err
		}
		n, err := s.archiveBranch(ctx, tx, c, id, p.ParentID, withChildren, at)
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.archived", p, map[string]any{"pages": n})
	})
}

// archivedParents lists the pages above a page that are in the archive, top first.
func archivedParents(ctx context.Context, q db.Querier, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT p.id FROM page_tree t JOIN pages p ON p.id = t.ancestor_id
		WHERE t.descendant_id = $1 AND t.depth > 0 AND p.archived_at IS NOT NULL ORDER BY t.depth DESC`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// restoreOne brings back a page and what was archived in the same action.
func restoreOne(ctx context.Context, tx pgx.Tx, id uuid.UUID, at *time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE pages SET archived_at = NULL, archived_by = NULL, updated_at = now()
		WHERE archived_at = $2 AND id IN (SELECT descendant_id FROM page_tree WHERE ancestor_id = $1)`, id, at)
	return err
}

// Unarchive brings a page back. Below a page that is still archived it is refused
// unless that page comes back too: moving it elsewhere would drop what it inherits
// (spec 011, D12).
func (s *Service) Unarchive(ctx context.Context, c identity.Caller, id uuid.UUID, withParents bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageArchive, uuid.Nil); err != nil {
			return err
		}
		if err := lockTree(ctx, tx, p.SpaceID); err != nil {
			return err
		}
		if p.ArchivedAt == nil {
			return apperr.Conflict("not_archived", "This page is not in the archive.")
		}
		parents, err := archivedParents(ctx, tx, id)
		if err != nil {
			return err
		}
		if len(parents) > 0 && !withParents {
			return apperr.Conflict("parent_archived", "The page above this one is in the archive too. Restore both.")
		}
		for _, parent := range parents {
			if err := s.perms.RequirePage(ctx, tx, c.UserID, parent, permissions.PageArchive, uuid.Nil); err != nil {
				return err
			}
			top, err := load(ctx, tx, parent)
			if err != nil {
				return err
			}
			if err := restoreOne(ctx, tx, parent, top.ArchivedAt); err != nil {
				return err
			}
		}
		if err := restoreOne(ctx, tx, id, p.ArchivedAt); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.unarchived", p, nil)
	})
}

// ArchivedPage is an entry of the archive of a space.
type ArchivedPage struct {
	ID             uuid.UUID  `json:"id"`
	ParentID       *uuid.UUID `json:"parent_id"`
	Title          string     `json:"title"`
	ArchivedAt     time.Time  `json:"archived_at"`
	ArchivedByName string     `json:"archived_by_name"`
}

// Archived lists the archived pages of a space the person may see.
func (s *Service) Archived(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]ArchivedPage, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.parent_id, p.title, p.archived_at, COALESCE(u.display_name, '')
		FROM pages p JOIN perm_visible_pages($1) v ON v.page_id = p.id
		LEFT JOIN users u ON u.id = p.archived_by
		WHERE p.space_id = $2 AND p.archived_at IS NOT NULL
		ORDER BY p.archived_at DESC, p.id LIMIT $3`, c.UserID, spaceID, maxArchivedPages)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ArchivedPage])
	if out == nil {
		out = []ArchivedPage{}
	}
	return out, err
}
