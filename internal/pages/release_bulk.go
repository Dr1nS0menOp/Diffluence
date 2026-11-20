// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/trash"
)

// What a run of a branch or the whole space writes of twins besides their versions (spec 073,
// wave PB2): taking them off, archiving them as the draft did, ordering them as the draft
// orders their pages, and clearing the rows made for a run that did not finish. As in
// release.go, none of these checks a permission: the publishing module has, and has marked
// its transaction as a release.

// TakeOffTwins sends twins to the published space's trash, where its administrators see them
// for the trash retention; it never purges (PUB-440, PUB-441). Twins below them that are not
// taken off move to the nearest twin above that stays, so readers keep them (PUB-440). Each
// group of twins taken off together is one item of the trash.
func (s *Service) TakeOffTwins(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, twins []uuid.UUID) error {
	if len(twins) == 0 {
		return nil
	}
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return err
	}
	if err := lockTree(ctx, tx, spaceID); err != nil {
		return err
	}
	kept, err := keptBelow(ctx, tx, twins)
	if err != nil {
		return err
	}
	for _, k := range kept {
		if err := relink(ctx, tx, k.id, k.parent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE pages SET position = (SELECT COALESCE(max(position) + 1, 0) FROM pages
			WHERE space_id = $2 AND parent_id IS NOT DISTINCT FROM $3 AND id <> $1 AND kind = ANY($4)) WHERE id = $1`,
			k.id, spaceID, k.parent, treeKinds); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT p.id FROM pages p WHERE p.id = ANY($1) AND p.status <> 'trashed'
		AND (p.parent_id IS NULL OR NOT (p.parent_id = ANY($1))) ORDER BY p.id`, twins)
	if err != nil {
		return err
	}
	tops, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, top := range tops {
		p, err := load(ctx, tx, top)
		if err != nil {
			return err
		}
		item, err := trash.Add(ctx, tx, trash.Item{SpaceID: spaceID, ObjectType: trash.TypePage, ObjectID: top,
			Title: p.Title, DeletedBy: c.UserID}, retention)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE pages SET pre_trash_status = status, status = 'trashed', trash_item_id = $2, updated_at = now()
			WHERE id IN (SELECT descendant_id FROM page_tree WHERE ancestor_id = $1) AND status <> 'trashed'`, top, item); err != nil {
			return err
		}
		if err := record(ctx, tx, c, "page.taken_off", p, nil); err != nil {
			return err
		}
	}
	return nil
}

// keptChild is a twin below a twin taken off that stays, with the twin it moves under.
type keptChild struct {
	id     uuid.UUID
	parent *uuid.UUID
}

// keptBelow finds the twins that sit directly below one taken off and stay, and for each the
// nearest twin above it that stays.
func keptBelow(ctx context.Context, tx pgx.Tx, gone []uuid.UUID) ([]keptChild, error) {
	rows, err := tx.Query(ctx, `SELECT c.id,
			(SELECT a.id FROM page_tree t JOIN pages a ON a.id = t.ancestor_id
			 WHERE t.descendant_id = c.id AND t.depth > 0 AND NOT (a.id = ANY($1)) ORDER BY t.depth LIMIT 1)
		FROM pages c WHERE c.parent_id = ANY($1) AND NOT (c.id = ANY($1)) AND c.status <> 'trashed' ORDER BY c.position, c.id`, gone)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (keptChild, error) {
		var k keptChild
		err := r.Scan(&k.id, &k.parent)
		return k, err
	})
}

// SetTwinArchived archives a twin or brings it back, as its draft page was (PUB-1013). It
// touches the twin alone: the pages below have their own pairs.
func (s *Service) SetTwinArchived(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, archived bool) error {
	p, err := load(ctx, tx, id)
	if err != nil {
		return err
	}
	if (p.ArchivedAt != nil) == archived {
		return nil
	}
	action := "page.archived"
	if archived {
		_, err = tx.Exec(ctx, "UPDATE pages SET archived_at = now(), archived_by = $2, updated_at = now() WHERE id = $1", id, c.UserID)
	} else {
		action = "page.unarchived"
		_, err = tx.Exec(ctx, "UPDATE pages SET archived_at = NULL, archived_by = NULL, updated_at = now() WHERE id = $1", id)
	}
	if err != nil {
		return err
	}
	return record(ctx, tx, c, action, p, map[string]any{"pages": 1})
}

// OrderTwins orders the children of a twin parent: first the twins in the order given, which
// is the order of their draft pages, then every other child in the order it had (PUB-412).
func (s *Service) OrderTwins(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, parent *uuid.UUID, first []uuid.UUID) error {
	rows, err := tx.Query(ctx, `SELECT id FROM pages WHERE space_id = $1 AND parent_id IS NOT DISTINCT FROM $2 AND kind = ANY($3)
		AND status <> 'trashed' ORDER BY position, id`, spaceID, parent, treeKinds)
	if err != nil {
		return err
	}
	children, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	order := make([]uuid.UUID, 0, len(children))
	for _, id := range first {
		if slices.Contains(children, id) && !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	for _, id := range children {
		if !slices.Contains(order, id) {
			order = append(order, id)
		}
	}
	if slices.Equal(order, children) {
		return nil
	}
	_, err = tx.Exec(ctx, `UPDATE pages SET position = o.ord - 1
		FROM unnest($1::uuid[]) WITH ORDINALITY AS o(id, ord) WHERE pages.id = o.id AND pages.position IS DISTINCT FROM o.ord - 1`, order)
	return err
}

// DropUnreleasedTwins removes twins a run made that never got a version, because the run was
// cancelled before their batch. Nobody but the releaser ever saw them (PUB-514).
func (s *Service) DropUnreleasedTwins(ctx context.Context, tx pgx.Tx, twins []uuid.UUID) error {
	if len(twins) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, "DELETE FROM pages WHERE id = ANY($1) AND current_version = 0", twins)
	return err
}

// VisiblePaths gives, for each page among list, the titles of the pages above it that a person
// may see, top first, without the home page of the space: the path a list shows beside a page.
func VisiblePaths(ctx context.Context, q db.Querier, user, spaceID uuid.UUID, list []uuid.UUID) (map[uuid.UUID][]string, error) {
	rows, err := q.Query(ctx, `SELECT t.descendant_id, a.title FROM page_tree t JOIN pages a ON a.id = t.ancestor_id
		WHERE t.descendant_id = ANY($1) AND t.depth > 0 AND a.id IS DISTINCT FROM (SELECT home_page_id FROM spaces WHERE id = $2)
		  AND a.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($3) h)
		  AND a.id NOT IN (SELECT ch.page_id FROM perm_control_hidden_pages($3) ch)
		ORDER BY t.descendant_id, t.depth DESC`, list, spaceID, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]string{}
	for rows.Next() {
		var id uuid.UUID
		var title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, err
		}
		out[id] = append(out[id], title)
	}
	return out, rows.Err()
}

// Linking is a page of another space that links into a space.
type Linking struct {
	PageID    uuid.UUID
	Title     string
	SpaceName string
}

// LinkingInto lists the pages of other spaces that link to a page of a space and that a
// person may see, at most limit, by title.
func LinkingInto(ctx context.Context, q db.Querier, user, spaceID uuid.UUID, limit int) ([]Linking, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT src.id, src.title, sp.name FROM page_links l
		JOIN pages t ON t.id = l.target_id AND t.space_id = $1
		JOIN pages src ON src.id = l.source_id AND src.space_id <> $1 AND src.status = 'published'
		JOIN spaces sp ON sp.id = src.space_id
		WHERE src.id IN (SELECT v.page_id FROM perm_visible_pages($2) v)
		ORDER BY src.title, src.id LIMIT $3`, spaceID, user, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Linking, error) {
		var l Linking
		err := r.Scan(&l.PageID, &l.Title, &l.SpaceName)
		return l, err
	})
}

// ChildrenInOrder lists the pages directly below a page, or at the top of a space, in their
// order, for ordering twins as their draft pages are.
func ChildrenInOrder(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, parent *uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM pages WHERE space_id = $1 AND parent_id IS NOT DISTINCT FROM $2 AND kind = ANY($3)
		AND status <> 'trashed' ORDER BY position, id`, spaceID, parent, treeKinds)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
