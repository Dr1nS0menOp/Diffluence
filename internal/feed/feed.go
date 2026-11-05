// SPDX-License-Identifier: Apache-2.0

// Package feed is the start screen (spec 010, C14). It is a view over versions and
// comments, with no table of its own, so it needs no backfill and has no lag (D17).
// Everything goes through perm_visible_pages, so it shows only what a person may see.
package feed

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
)

// The tabs of the start screen.
const (
	TabForYou  = "for_you"
	TabRecent  = "recent"
	TabPopular = "popular"
)

const (
	defaultItems = 20
	maxItems     = 50
	excerptChars = 200
	popularDays  = 7
)

// Query asks for a part of a feed.
type Query struct {
	Tab     string
	SpaceID uuid.UUID
	Before  time.Time
	Limit   int
}

// Item is one entry of a feed.
type Item struct {
	PageID     uuid.UUID `json:"page_id"`
	Title      string    `json:"title"`
	Emoji      string    `json:"emoji"`
	SpaceKey   string    `json:"space_key"`
	SpaceName  string    `json:"space_name"`
	Excerpt    string    `json:"excerpt"`
	AuthorName string    `json:"author_name"`
	Kind       string    `json:"kind"`
	At         time.Time `json:"at"`
	Reactions  int       `json:"reactions"`
	Comments   int       `json:"comments"`
}

// Service reads feeds.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// One row per page: its newest version that was not silent. A silent version reached
// nobody, so it does not show here either.
const changesSQL = `WITH visible AS (SELECT page_id FROM perm_visible_pages($1)),
	changes AS (
		SELECT DISTINCT ON (v.page_id) v.page_id, v.author_id, v.created_at AS at,
			CASE WHEN v.number = 1 THEN 'created' ELSE 'changed' END AS kind
		FROM page_versions v JOIN visible ON visible.page_id = v.page_id
		WHERE NOT v.silent
		ORDER BY v.page_id, v.number DESC
	)
	SELECT c.page_id, p.title, p.emoji, s.key, s.name, COALESCE(left(d.body, $5), ''), COALESCE(u.display_name, ''), c.kind, c.at,
		(SELECT count(*) FROM reactions r WHERE r.target_type = 'page' AND r.target_id = c.page_id),
		(SELECT count(*) FROM comments m WHERE m.page_id = c.page_id AND m.trash_item_id IS NULL AND NOT m.purged AND NOT m.editors_only)
	FROM changes c
	JOIN pages p ON p.id = c.page_id AND p.status = 'published'
	JOIN spaces s ON s.id = p.space_id AND s.status = 'active'
	LEFT JOIN search_documents d ON d.page_id = c.page_id
	LEFT JOIN users u ON u.id = c.author_id
	WHERE ($2::uuid IS NULL OR p.space_id = $2)
	  AND ($3::timestamptz IS NULL OR c.at < $3)
	  AND (NOT $6 OR (c.author_id <> $1 AND (
		c.author_id IN (SELECT followed_id FROM follows WHERE user_id = $1)
		OR EXISTS (SELECT 1 FROM watches w WHERE w.user_id = $1 AND (
			(w.target_type = 'page' AND w.target_id = c.page_id)
			OR (w.target_type = 'space' AND w.target_id = p.space_id)
			OR (w.target_type = 'branch' AND w.target_id IN
				(SELECT t.ancestor_id FROM page_tree t WHERE t.descendant_id = c.page_id)))))))
	ORDER BY c.at DESC, c.page_id LIMIT $4`

// Popular pages count what happened to them in the last days, among comments that
// the person may see.
const popularSQL = `WITH visible AS (SELECT page_id FROM perm_visible_pages($1)),
	scores AS (
		SELECT v.page_id,
			(SELECT count(*) FROM reactions r WHERE r.target_type = 'page' AND r.target_id = v.page_id
				AND r.created_at > now() - make_interval(days => $4)) AS reactions,
			(SELECT count(*) FROM comments m WHERE m.page_id = v.page_id AND m.trash_item_id IS NULL AND NOT m.purged
				AND NOT m.editors_only AND m.created_at > now() - make_interval(days => $4)) AS comments
		FROM visible v
	)
	SELECT c.page_id, p.title, p.emoji, s.key, s.name, COALESCE(left(d.body, $5), ''), COALESCE(u.display_name, ''), 'popular', p.updated_at,
		c.reactions, c.comments
	FROM scores c
	JOIN pages p ON p.id = c.page_id AND p.status = 'published'
	JOIN spaces s ON s.id = p.space_id AND s.status = 'active'
	LEFT JOIN search_documents d ON d.page_id = c.page_id
	LEFT JOIN users u ON u.id = p.updated_by
	WHERE c.reactions + c.comments > 0 AND ($2::uuid IS NULL OR p.space_id = $2)
	ORDER BY c.reactions + c.comments DESC, p.updated_at DESC, c.page_id LIMIT $3`

func scan(r pgx.CollectableRow) (Item, error) {
	var v Item
	err := r.Scan(&v.PageID, &v.Title, &v.Emoji, &v.SpaceKey, &v.SpaceName, &v.Excerpt, &v.AuthorName, &v.Kind, &v.At, &v.Reactions, &v.Comments)
	return v, err
}

// List returns one part of a feed.
func (s *Service) List(ctx context.Context, c identity.Caller, q Query) ([]Item, error) {
	if q.Tab == "" {
		q.Tab = TabForYou
	}
	if !slices.Contains([]string{TabForYou, TabRecent, TabPopular}, q.Tab) {
		return nil, apperr.Invalid("That is not a tab of the start screen.")
	}
	limit := q.Limit
	if limit <= 0 || limit > maxItems {
		limit = defaultItems
	}
	var space *uuid.UUID
	if q.SpaceID != uuid.Nil {
		space = &q.SpaceID
	}
	var before *time.Time
	if !q.Before.IsZero() {
		before = &q.Before
	}
	var rows pgx.Rows
	var err error
	if q.Tab == TabPopular {
		rows, err = s.pool.Query(ctx, popularSQL, c.UserID, space, limit, popularDays, excerptChars)
	} else {
		rows, err = s.pool.Query(ctx, changesSQL, c.UserID, space, before, limit, excerptChars, q.Tab == TabForYou)
	}
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scan)
	if out == nil {
		out = []Item{}
	}
	return out, err
}
