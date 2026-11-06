// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The tabs of the Recent screen.
const (
	RecentVisited = "visited"
	RecentWorked  = "worked"
	RecentDrafts  = "drafts"
)

const (
	maxRecentPages    = 50
	maxDraftPages     = 200
	maxFavouritePages = 200
	maxRecentSpaces   = 6
	recentChangeCount = 10
)

// ListItem is an entry of a personal list.
type ListItem struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Emoji       string    `json:"emoji"`
	SpaceID     uuid.UUID `json:"space_id"`
	SpaceKey    string    `json:"space_key"`
	SpaceName   string    `json:"space_name"`
	At          time.Time `json:"at"`
	Unpublished bool      `json:"unpublished"`
}

func collectList(rows pgx.Rows, err error) ([]ListItem, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ListItem, error) {
		var v ListItem
		err := r.Scan(&v.ID, &v.Title, &v.Emoji, &v.SpaceID, &v.SpaceKey, &v.SpaceName, &v.At, &v.Unpublished)
		return v, err
	})
	if out == nil {
		out = []ListItem{}
	}
	return out, err
}

// Recent lists pages for one tab of the Recent screen, as far as the user may
// still see them. Every tab goes through perm_listed_pages.
func (s *Service) Recent(ctx context.Context, c identity.Caller, kind string) ([]ListItem, error) {
	switch kind {
	case "", RecentVisited:
		rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, r.viewed_at, p.status = 'draft'
			FROM recent_pages r
			JOIN perm_listed_pages($1) v ON v.page_id = r.page_id
			JOIN pages p ON p.id = r.page_id
			JOIN spaces s ON s.id = p.space_id
			WHERE r.user_id = $1 ORDER BY r.viewed_at DESC LIMIT $2`, c.UserID, maxRecentPages)
		return collectList(rows, err)
	case RecentWorked:
		rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, w.at, false
			FROM (SELECT page_id, max(at) AS at FROM (
					SELECT page_id, created_at AS at FROM page_versions WHERE author_id = $1
					UNION ALL
					SELECT page_id, created_at FROM comments WHERE author_id = $1 AND trash_item_id IS NULL
				) made GROUP BY page_id) w
			JOIN perm_listed_pages($1) v ON v.page_id = w.page_id
			JOIN pages p ON p.id = w.page_id
			JOIN spaces s ON s.id = p.space_id
			ORDER BY w.at DESC LIMIT $2`, c.UserID, maxRecentPages)
		return collectList(rows, err)
	case RecentDrafts:
		rows, err := s.pool.Query(ctx, `SELECT p.id, COALESCE(NULLIF(d.title, ''), p.title), p.emoji, p.space_id, s.key, s.name,
				COALESCE(d.updated_at, p.updated_at), p.status = 'draft'
			FROM pages p
			JOIN perm_listed_pages($1) v ON v.page_id = p.id
			JOIN spaces s ON s.id = p.space_id
			LEFT JOIN drafts d ON d.page_id = p.id AND d.user_id = $1
			WHERE d.page_id IS NOT NULL OR (p.status = 'draft' AND p.created_by = $1)
			ORDER BY COALESCE(d.updated_at, p.updated_at) DESC LIMIT $2`, c.UserID, maxDraftPages)
		return collectList(rows, err)
	}
	return nil, apperr.Invalid("Recent lists are visited, worked or drafts.")
}

// Favourites lists favourite pages the user may still see. New ones come first,
// then the order the person chose.
func (s *Service) Favourites(ctx context.Context, c identity.Caller) ([]ListItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, f.created_at, p.status = 'draft'
		FROM favourites f
		JOIN perm_listed_pages($1) v ON v.page_id = f.page_id
		JOIN pages p ON p.id = f.page_id
		JOIN spaces s ON s.id = p.space_id
		WHERE f.user_id = $1 ORDER BY f.position NULLS FIRST, f.created_at DESC LIMIT $2`, c.UserID, maxFavouritePages)
	return collectList(rows, err)
}

// SetFavourite adds or removes a favourite. It is a personal preference, so it
// is not audited.
func (s *Service) SetFavourite(ctx context.Context, c identity.Caller, id uuid.UUID, on bool) error {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	var err error
	if on {
		_, err = s.pool.Exec(ctx, "INSERT INTO favourites (user_id, page_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", c.UserID, id)
	} else {
		_, err = s.pool.Exec(ctx, "DELETE FROM favourites WHERE user_id = $1 AND page_id = $2", c.UserID, id)
	}
	return err
}

// OrderFavourites stores the order a person chose. It only touches their own
// rows, so ids of anything else simply do nothing.
func (s *Service) OrderFavourites(ctx context.Context, c identity.Caller, pageIDs []uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE favourites f SET position = o.ord
		FROM unnest($2::uuid[]) WITH ORDINALITY AS o(id, ord)
		WHERE f.user_id = $1 AND f.page_id = o.id`, c.UserID, pageIDs)
	return err
}

// RecentSpaceIDs lists the spaces of recently visited pages, newest first.
func (s *Service) RecentSpaceIDs(ctx context.Context, c identity.Caller) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.space_id FROM recent_pages r
		JOIN perm_listed_pages($1) v ON v.page_id = r.page_id
		JOIN pages p ON p.id = r.page_id
		WHERE r.user_id = $1 GROUP BY p.space_id ORDER BY max(r.viewed_at) DESC LIMIT $2`, c.UserID, maxRecentSpaces*2)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// SpaceActivity is what the overview of a space shows about its pages.
type SpaceActivity struct {
	PageCount int        `json:"page_count"`
	Recent    []ListItem `json:"recent"`
}

// Activity counts and lists the pages of a space the user may see.
func (s *Service) Activity(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (SpaceActivity, error) {
	var out SpaceActivity
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return out, err
	}
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id
		WHERE p.space_id = $2 AND p.status = 'published'`, c.UserID, spaceID).Scan(&out.PageCount)
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, p.updated_at, false
		FROM pages p
		JOIN perm_listed_pages($1) v ON v.page_id = p.id
		JOIN spaces s ON s.id = p.space_id
		WHERE p.space_id = $2 AND p.status = 'published'
		ORDER BY p.updated_at DESC, p.id LIMIT $3`, c.UserID, spaceID, recentChangeCount)
	out.Recent, err = collectList(rows, err)
	return out, err
}
