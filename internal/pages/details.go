// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	maxListedLinks    = 200
	maxListedChildren = 200
	maxImpactLinks    = 10
)

// PageRef points at a page the caller may see.
type PageRef struct {
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	Emoji     string    `json:"emoji"`
	SpaceKey  string    `json:"space_key"`
	SpaceName string    `json:"space_name"`
}

// Contributor is someone who published a version of a page.
type Contributor struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Versions int       `json:"versions"`
}

// Details are the facts around a page, for the Details panel.
type Details struct {
	CreatedAt     time.Time     `json:"created_at"`
	CreatedByName string        `json:"created_by_name"`
	UpdatedAt     time.Time     `json:"updated_at"`
	UpdatedByName string        `json:"updated_by_name"`
	Contributors  []Contributor `json:"contributors"`
	Incoming      []PageRef     `json:"incoming"`
	Outgoing      []PageRef     `json:"outgoing"`
	Children      []PageRef     `json:"children"`
}

// Impact is what a delete would touch, counted from what the caller can see.
type Impact struct {
	Pages         int       `json:"pages"`
	Incoming      []PageRef `json:"incoming"`
	IncomingTotal int       `json:"incoming_total"`
}

func collectRefs(rows pgx.Rows, err error) ([]PageRef, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PageRef, error) {
		var v PageRef
		err := r.Scan(&v.ID, &v.Title, &v.Emoji, &v.SpaceKey, &v.SpaceName)
		return v, err
	})
	if out == nil {
		out = []PageRef{}
	}
	return out, err
}

// refColumns and refJoins read a page through perm_listed_pages, the only way a
// list sees content. A page the caller may not see can never become a reference.
const (
	refColumns = "p.id, p.title, p.emoji, s.key, s.name"
	refJoins   = `JOIN perm_listed_pages($1) v ON v.page_id = p.id
		JOIN spaces s ON s.id = p.space_id`
)

// maxReachableIDs is more links than a page can sensibly hold.
const maxReachableIDs = 500

// Reachable answers which of these pages the caller can open. A page that is hidden and
// a page that is gone are both simply left out, so the two cannot be told apart.
func (s *Service) Reachable(ctx context.Context, c identity.Caller, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) > maxReachableIDs {
		return nil, apperr.Invalid("That is too many pages to ask about at once.")
	}
	rows, err := s.pool.Query(ctx, "SELECT v.page_id FROM perm_visible_pages($1) v WHERE v.page_id = ANY($2)", c.UserID, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// syncLinks makes page_links follow the current version, in the transaction that
// publishes it.
func syncLinks(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, doc document.Node) error {
	if _, err := tx.Exec(ctx, "DELETE FROM page_links WHERE source_id = $1", pageID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_links (source_id, target_id)
		SELECT $1, target FROM unnest($2::uuid[]) AS target`, pageID, document.LinkedPages(doc)); err != nil {
		return err
	}
	return syncPlaceholders(ctx, tx, pageID, doc)
}

func incomingLinks(ctx context.Context, q db.Querier, userID, pageID uuid.UUID) ([]PageRef, error) {
	rows, err := q.Query(ctx, `SELECT `+refColumns+` FROM page_links l
		JOIN pages p ON p.id = l.source_id `+refJoins+`
		WHERE l.target_id = $2 AND l.source_id <> $2 AND p.status = 'published'
		ORDER BY lower(p.title), p.id LIMIT $3`, userID, pageID, maxListedLinks)
	return collectRefs(rows, err)
}

func outgoingLinks(ctx context.Context, q db.Querier, userID, pageID uuid.UUID) ([]PageRef, error) {
	rows, err := q.Query(ctx, `SELECT `+refColumns+` FROM page_links l
		JOIN pages p ON p.id = l.target_id `+refJoins+`
		WHERE l.source_id = $2 AND l.target_id <> $2 AND p.status = 'published'
		ORDER BY lower(p.title), p.id LIMIT $3`, userID, pageID, maxListedLinks)
	return collectRefs(rows, err)
}

func childPages(ctx context.Context, q db.Querier, userID, pageID uuid.UUID) ([]PageRef, error) {
	rows, err := q.Query(ctx, `SELECT `+refColumns+` FROM pages p `+refJoins+`
		WHERE p.parent_id = $2 AND p.status = 'published' AND p.kind = ANY($4)
		ORDER BY p.position, p.id LIMIT $3`, userID, pageID, maxListedChildren, treeKinds)
	return collectRefs(rows, err)
}

func contributors(ctx context.Context, q db.Querier, pageID uuid.UUID) ([]Contributor, error) {
	rows, err := q.Query(ctx, `SELECT v.author_id, COALESCE(u.display_name, ''), count(*)
		FROM page_versions v LEFT JOIN users u ON u.id = v.author_id
		WHERE v.page_id = $1 GROUP BY v.author_id, u.display_name
		ORDER BY count(*) DESC, lower(COALESCE(u.display_name, ''))`, pageID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Contributor, error) {
		var v Contributor
		err := r.Scan(&v.ID, &v.Name, &v.Versions)
		return v, err
	})
	if out == nil {
		out = []Contributor{}
	}
	return out, err
}

// Summary is a page without its text, for a menu or a dialog. Unlike Get it does
// not count as a visit: opening a menu in the tree is not reading a page.
type Summary struct {
	Page
	Favourite bool            `json:"favourite"`
	Can       map[string]bool `json:"can"`
}

// PageSummary returns what a menu needs to know about a page.
func (s *Service) PageSummary(ctx context.Context, c identity.Caller, id uuid.UUID) (Summary, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return Summary{}, err
	}
	p, err := load(ctx, s.pool, id)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{Page: p}
	err = s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM favourites WHERE page_id = $1 AND user_id = $2)", id, c.UserID).Scan(&out.Favourite)
	if err != nil {
		return out, err
	}
	out.Can, err = s.capabilities(ctx, c, p)
	return out, err
}

// PageDetails returns the facts around a page.
func (s *Service) PageDetails(ctx context.Context, c identity.Caller, id uuid.UUID) (Details, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return Details{}, err
	}
	p, err := load(ctx, s.pool, id)
	if err != nil {
		return Details{}, err
	}
	d := Details{CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
	if d.CreatedByName, d.UpdatedByName, err = s.authorNames(ctx, p); err != nil {
		return d, err
	}
	if d.Contributors, err = contributors(ctx, s.pool, id); err != nil {
		return d, err
	}
	if d.Incoming, err = incomingLinks(ctx, s.pool, c.UserID, id); err != nil {
		return d, err
	}
	if d.Outgoing, err = outgoingLinks(ctx, s.pool, c.UserID, id); err != nil {
		return d, err
	}
	d.Children, err = childPages(ctx, s.pool, c.UserID, id)
	return d, err
}

// DeleteImpact says what deleting a page would touch. Pages in the same branch
// go along, so their links do not count as links that break.
func (s *Service) DeleteImpact(ctx context.Context, c identity.Caller, id uuid.UUID) (Impact, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageDelete, uuid.Nil); err != nil {
		return Impact{}, err
	}
	var out Impact
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM page_tree t
		JOIN perm_visible_pages($1) v ON v.page_id = t.descendant_id WHERE t.ancestor_id = $2`, c.UserID, id).Scan(&out.Pages)
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+refColumns+` FROM page_links l
		JOIN pages p ON p.id = l.source_id `+refJoins+`
		WHERE l.target_id = $2 AND p.status = 'published'
		  AND l.source_id NOT IN (SELECT descendant_id FROM page_tree WHERE ancestor_id = $2)
		ORDER BY lower(p.title), p.id`, c.UserID, id)
	all, err := collectRefs(rows, err)
	if err != nil {
		return out, err
	}
	out.IncomingTotal = len(all)
	out.Incoming = all[:min(len(all), maxImpactLinks)]
	return out, nil
}
