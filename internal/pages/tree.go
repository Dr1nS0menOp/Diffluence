// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/trash"
)

func subtree(ctx context.Context, q db.Querier, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, "SELECT descendant_id FROM page_tree WHERE ancestor_id = $1 ORDER BY depth", id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// viewRestricted returns the pages at or above id that carry a view restriction.
func viewRestricted(ctx context.Context, q db.Querier, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT t.ancestor_id FROM page_tree t
		JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view'
		WHERE t.descendant_id = $1`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// MoveInput says where a page goes: under ParentID (nil = top level), before
// sibling BeforeID (nil = at the end), and to another space when SpaceID is set.
type MoveInput struct {
	SpaceID  *uuid.UUID
	ParentID *uuid.UUID
	BeforeID *uuid.UUID
}

// Move changes the parent and/or position of a page.
func (s *Service) Move(ctx context.Context, c identity.Caller, id uuid.UUID, in MoveInput) (Page, error) {
	var out Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if p.Kind == KindPost && in.ParentID != nil {
			return apperr.Invalid("A blog post has no place in the tree.")
		}
		if in.SpaceID != nil && *in.SpaceID != p.SpaceID {
			out, err = s.moveToSpace(ctx, tx, c, p, in)
			return err
		}
		if err := lockTree(ctx, tx, p.SpaceID); err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageCreate, uuid.Nil); err != nil {
			return err
		}
		sameParent := (p.ParentID == nil && in.ParentID == nil) ||
			(p.ParentID != nil && in.ParentID != nil && *p.ParentID == *in.ParentID)
		if !sameParent {
			if err := s.reparent(ctx, tx, c, p, in.ParentID); err != nil {
				return err
			}
		}
		if err := reposition(ctx, tx, p, in); err != nil {
			return err
		}
		out, err = load(ctx, tx, id)
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.moved", out, map[string]any{"from_parent": p.ParentID, "to_parent": in.ParentID})
	})
	return out, err
}

func (s *Service) reparent(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, parent *uuid.UUID) error {
	var before, after []uuid.UUID
	var err error
	if p.ParentID != nil {
		if before, err = viewRestricted(ctx, tx, *p.ParentID); err != nil {
			return err
		}
	}
	if parent != nil {
		if err := s.checkParent(ctx, tx, c, p.SpaceID, *parent); err != nil {
			return err
		}
		tree, err := subtree(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		if slices.Contains(tree, *parent) {
			return apperr.Conflict("move_into_self", "A page cannot be moved under itself.")
		}
		if after, err = viewRestricted(ctx, tx, *parent); err != nil {
			return err
		}
	}
	// A move must not quietly lift a restriction the page inherited.
	for _, r := range before {
		if !slices.Contains(after, r) {
			if err := s.perms.RequirePage(ctx, tx, c.UserID, p.ID, permissions.RestrictionManage, uuid.Nil); err != nil {
				return err
			}
			break
		}
	}
	return relink(ctx, tx, p.ID, parent)
}

// relink detaches a branch from its old ancestors and attaches it to the new ones.
func relink(ctx context.Context, tx pgx.Tx, id uuid.UUID, parent *uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM page_tree
		WHERE descendant_id IN (SELECT descendant_id FROM page_tree WHERE ancestor_id = $1)
		  AND ancestor_id NOT IN (SELECT descendant_id FROM page_tree WHERE ancestor_id = $1)`, id); err != nil {
		return err
	}
	if parent != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO page_tree (ancestor_id, descendant_id, depth)
			SELECT a.ancestor_id, d.descendant_id, a.depth + d.depth + 1
			FROM page_tree a CROSS JOIN page_tree d
			WHERE a.descendant_id = $1 AND d.ancestor_id = $2`, *parent, id); err != nil {
			return err
		}
	}
	_, err := tx.Exec(ctx, "UPDATE pages SET parent_id = $2, updated_at = now() WHERE id = $1", id, parent)
	return err
}

// reposition rewrites the order of the new siblings with the page in its place.
func reposition(ctx context.Context, tx pgx.Tx, p Page, in MoveInput) error {
	rows, err := tx.Query(ctx, `SELECT id FROM pages
		WHERE space_id = $1 AND parent_id IS NOT DISTINCT FROM $2 AND id <> $3 AND kind = ANY($4) ORDER BY position, id`,
		p.SpaceID, in.ParentID, p.ID, treeKinds)
	if err != nil {
		return err
	}
	siblings, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	at := len(siblings)
	if in.BeforeID != nil {
		at = slices.Index(siblings, *in.BeforeID)
		if at < 0 {
			return apperr.Invalid("The page to move before is not a sibling.")
		}
	}
	order := slices.Insert(siblings, at, p.ID)
	_, err = tx.Exec(ctx, `UPDATE pages SET position = o.ord - 1
		FROM unnest($1::uuid[]) WITH ORDINALITY AS o(id, ord) WHERE pages.id = o.id`, order)
	return err
}

// TitleChange changes the titles of copied pages together (PAG-049).
type TitleChange struct {
	Prefix  string
	Suffix  string
	Find    string
	Replace string
}

func (t TitleChange) isSet() bool {
	return t.Prefix != "" || t.Suffix != "" || t.Find != ""
}

func (t TitleChange) apply(title string) string {
	if t.Find != "" {
		title = strings.ReplaceAll(title, t.Find, t.Replace)
	}
	return t.Prefix + title + t.Suffix
}

// CopyInput says what to copy and where.
type CopyInput struct {
	SpaceID         *uuid.UUID
	ParentID        *uuid.UUID
	IncludeChildren bool
	KeepLabels      bool
	// KeepRestrictions defaults to yes: dropping them needs manage restrictions.
	KeepRestrictions *bool
	Titles           TitleChange
}

func (in CopyInput) keepRestrictions() bool {
	return in.KeepRestrictions == nil || *in.KeepRestrictions
}

// copyPlan is what stays the same for every page of a copy.
type copyPlan struct {
	CopyInput
	spaceID uuid.UUID
}

// Copy creates an independent copy of a page.
func (s *Service) Copy(ctx context.Context, c identity.Caller, id uuid.UUID, in CopyInput) (Page, error) {
	var out Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		plan := copyPlan{CopyInput: in, spaceID: p.SpaceID}
		if in.SpaceID != nil {
			plan.spaceID = *in.SpaceID
		}
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, plan.spaceID, permissions.PageCreate); err != nil {
			return err
		}
		if p.CurrentVersion == 0 {
			return apperr.Conflict("not_published", "Publish the page before copying it.")
		}
		if err := lockSpaces(ctx, tx, p.SpaceID, plan.spaceID); err != nil {
			return err
		}
		if in.ParentID != nil {
			if err := s.checkParent(ctx, tx, c, plan.spaceID, *in.ParentID); err != nil {
				return err
			}
		}
		// Like a move: a copy must not quietly lift what the page inherits. Several
		// levels add up and cannot become one list, so such a copy can only be refused.
		lost, _, err := inheritedChange(ctx, tx, p, in.ParentID)
		if err != nil {
			return err
		}
		if len(lost) > 0 {
			if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.RestrictionManage, uuid.Nil); err != nil {
				return err
			}
		}
		title := "Copy of " + p.Title
		if in.Titles.isSet() {
			title = in.Titles.apply(p.Title)
		}
		if out, err = s.copyOne(ctx, tx, c, p, in.ParentID, title, plan); err != nil {
			return err
		}
		if !in.IncludeChildren {
			return nil
		}
		return s.copyChildren(ctx, tx, c, p.ID, out.ID, plan)
	})
	return out, err
}

func (s *Service) copyOne(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, parent *uuid.UUID, title string, plan copyPlan) (Page, error) {
	if !plan.keepRestrictions() {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, p.ID, permissions.RestrictionManage, uuid.Nil); err != nil {
			return Page{}, err
		}
	}
	if r := []rune(title); len(r) > maxTitleRunes {
		title = string(r[:maxTitleRunes])
	}
	title, err := cleanTitle(title)
	if err != nil {
		return Page{}, err
	}
	doc, err := currentDocument(ctx, tx, p)
	if err != nil {
		return Page{}, err
	}
	// The row comes first so attachments have a page to belong to; the first
	// version follows once images point at the copied files.
	n, err := s.insert(ctx, tx, c, newPage{spaceID: plan.spaceID, parent: parent, title: title, author: c.UserID, kind: p.Kind, link: p.LinkTarget})
	if err != nil {
		return Page{}, err
	}
	mapping, err := s.attachments.CopyAll(ctx, tx, c, p.ID, n.ID, plan.spaceID)
	if err != nil {
		return Page{}, err
	}
	document.RemapAttachments(&doc, mapping)
	if plan.keepRestrictions() {
		if _, err := tx.Exec(ctx, `INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id)
			SELECT $2::uuid, kind, subject_type, subject_id FROM page_restrictions WHERE page_id = $1`, p.ID, n.ID); err != nil {
			return Page{}, err
		}
	}
	if plan.KeepLabels {
		if _, err := tx.Exec(ctx, `INSERT INTO page_labels (page_id, label, created_by)
			SELECT $2::uuid, label, $3 FROM page_labels WHERE page_id = $1`, p.ID, n.ID, c.UserID); err != nil {
			return Page{}, err
		}
	}
	// A copy is a new page: its tasks are its own, and nobody is told about them twice.
	if n, err = s.appendVersion(ctx, tx, c, n, newVersion{Title: title, Doc: doc, Author: c.UserID, Silent: true}); err != nil {
		return Page{}, err
	}
	return n, record(ctx, tx, c, "page.copied", n, map[string]any{"copied_from": p.ID})
}

// childrenToCopy lists what the user can see; hidden pages stay behind.
func childrenToCopy(ctx context.Context, q db.Querier, userID, from uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT p.id FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id
		WHERE p.parent_id = $2 AND p.status = 'published' ORDER BY p.position, p.id`, userID, from)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (s *Service) copyChildren(ctx context.Context, tx pgx.Tx, c identity.Caller, from, to uuid.UUID, plan copyPlan) error {
	children, err := childrenToCopy(ctx, tx, c.UserID, from)
	if err != nil {
		return err
	}
	for _, child := range children {
		p, err := load(ctx, tx, child)
		if err != nil {
			return err
		}
		n, err := s.copyOne(ctx, tx, c, p, &to, plan.Titles.apply(p.Title), plan)
		if err != nil {
			return err
		}
		if err := s.copyChildren(ctx, tx, c, child, n.ID, plan); err != nil {
			return err
		}
	}
	return nil
}

// Delete sends a page and everything below it to the trash. It is refused when
// the user cannot delete every page in the subtree, so nobody removes content
// they cannot see.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageDelete, uuid.Nil); err != nil {
			return err
		}
		if err := lockTree(ctx, tx, p.SpaceID); err != nil {
			return err
		}
		var isHome bool
		if err := tx.QueryRow(ctx, "SELECT home_page_id IS NOT DISTINCT FROM $2 FROM spaces WHERE id = $1", p.SpaceID, id).Scan(&isHome); err != nil {
			return err
		}
		if isHome {
			return apperr.Conflict("home_page", "The home page of a space cannot be deleted.")
		}
		rows, err := tx.Query(ctx, `SELECT p.id FROM pages p JOIN page_tree t ON t.descendant_id = p.id
			WHERE t.ancestor_id = $1 AND p.status <> 'trashed'`, id)
		if err != nil {
			return err
		}
		tree, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		for _, page := range tree {
			d, err := s.perms.CheckPage(ctx, tx, c.UserID, page, permissions.ActionPageDelete, uuid.Nil)
			if err != nil {
				return err
			}
			if !d.Allowed {
				return apperr.Conflict("contains_protected_pages",
					"This page has pages below it that you cannot delete. Move them away or ask a space admin.")
			}
		}
		item, err := trash.Add(ctx, tx, trash.Item{SpaceID: p.SpaceID, ObjectType: trash.TypePage, ObjectID: id,
			Title: p.Title, DeletedBy: c.UserID}, retention)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE pages SET pre_trash_status = status, status = 'trashed', trash_item_id = $2, updated_at = now()
			WHERE id = ANY($1)`, tree, item); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.trashed", p, map[string]any{"pages": len(tree)})
	})
}

// Restore implements trash.Handler.
func (s *Service) Restore(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	p, err := load(ctx, tx, it.ObjectID)
	if err != nil {
		return err
	}
	if err := lockTree(ctx, tx, p.SpaceID); err != nil {
		return err
	}
	if p.ParentID != nil {
		parent, err := load(ctx, tx, *p.ParentID)
		if err != nil {
			return err
		}
		if parent.Status == StatusTrashed {
			// Restoring here would drop restrictions inherited from the parent.
			return apperr.Conflict("parent_in_trash", "The parent of this page is in the trash. Restore the parent first.")
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE pages SET status = pre_trash_status, pre_trash_status = '', trash_item_id = NULL, updated_at = now()
		WHERE trash_item_id = $1`, it.ID); err != nil {
		return err
	}
	return record(ctx, tx, c, "page.restored", p, nil)
}

// Purge implements trash.Handler: the page and everything below it go for good.
// Pages below that were trashed earlier go too, so nothing can lose a
// restriction it inherited.
func (s *Service) Purge(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	tree, err := subtree(ctx, tx, it.ObjectID)
	if err != nil {
		return err
	}
	if len(tree) == 0 {
		return nil // already gone with a parent
	}
	if err := attachments.QueueDeletionForPages(ctx, tx, tree); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, "SELECT id FROM attachments WHERE page_id = ANY($1)", tree)
	if err != nil {
		return err
	}
	files, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	if err := trash.RemoveFor(ctx, tx, append(files, tree...)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM pages WHERE id = ANY($1)", tree); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("page.purged", "page", it.ObjectID.String(), it.SpaceID.String(),
		map[string]any{"title": it.Title, "pages": len(tree)})); err != nil {
		return err
	}
	_, err = outbox.Enqueue(ctx, tx, "page.purged", map[string]any{"page_id": it.ObjectID, "space_id": it.SpaceID})
	return err
}
