// SPDX-License-Identifier: Apache-2.0

package spaces

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The filters and sort orders of the spaces directory.
const (
	KindAll        = "all"
	KindFavourites = "favourites"
	KindArchived   = "archived"
	KindTeam       = "team"
	KindPersonal   = "personal"

	SortName    = "name"
	SortChanged = "changed"
	SortPages   = "pages"
)

const (
	defaultDirectoryRows = 50
	maxDirectoryRows     = 100
	maxMenuSpaces        = 6
	overviewPosts        = 3
)

// DirectoryQuery asks the spaces directory for a part of the list.
type DirectoryQuery struct {
	Text     string
	Kind     string
	Sort     string
	Category uuid.UUID
	Limit    int
	Offset   int
}

func (q DirectoryQuery) validated() (DirectoryQuery, error) {
	if q.Kind == "" {
		q.Kind = KindAll
	}
	if q.Sort == "" {
		q.Sort = SortName
	}
	if !slices.Contains([]string{KindAll, KindFavourites, KindArchived, KindTeam, KindPersonal}, q.Kind) {
		return q, apperr.Invalid("That is not a kind of space.")
	}
	if !slices.Contains([]string{SortName, SortChanged, SortPages}, q.Sort) {
		return q, apperr.Invalid("Spaces sort by name, last change or number of pages.")
	}
	if q.Limit <= 0 || q.Limit > maxDirectoryRows {
		q.Limit = defaultDirectoryRows
	}
	q.Offset = max(q.Offset, 0)
	return q, nil
}

// DirectoryRow is a space with what the directory shows about it.
type DirectoryRow struct {
	Space
	PageCount      int       `json:"page_count"`
	LastChanged    time.Time `json:"last_changed"`
	Administrators []string  `json:"administrators"`
}

// The page counts only cover pages this person may see, so the directory cannot
// tell how much is hidden.
const directorySQL = `WITH listed AS (
		SELECT sp.id FROM spaces sp
		WHERE sp.id IN (SELECT space_id FROM perm_spaces($1, 'space.view'))
		  AND ($2 = '' OR position(lower($2) IN lower(sp.name)) > 0 OR position(lower($2) IN lower(sp.key)) > 0)
		  AND CASE $3
		        WHEN 'archived' THEN sp.status = 'archived'
		        WHEN 'favourites' THEN sp.status = 'active'
		            AND sp.id IN (SELECT f.space_id FROM space_favourites f WHERE f.user_id = $1)
		        WHEN 'team' THEN sp.status = 'active' AND sp.kind = 'team'
		        WHEN 'personal' THEN sp.status = 'active' AND sp.kind = 'personal'
		        ELSE sp.status = 'active' END
		  AND ($7::uuid IS NULL OR sp.id IN (SELECT l.space_id FROM space_category_links l WHERE l.category_id = $7))
	), counted AS (
		SELECT p.space_id, count(*) AS pages, max(p.updated_at) AS changed
		FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id
		WHERE p.status = 'published' AND p.space_id IN (SELECT id FROM listed)
		GROUP BY p.space_id
	)
	SELECT s.id, s.key, s.name, s.description, s.icon, s.status, s.home_page_id, s.created_by, s.created_at, s.updated_at,
		s.icon_object <> '', s.title_align, s.review_months, s.approval_required,
		s.kind, s.owner_id, f.user_id IS NOT NULL, COALESCE(c.pages, 0), COALESCE(c.changed, s.updated_at), count(*) OVER ()
	FROM listed l
	JOIN spaces s ON s.id = l.id
	LEFT JOIN counted c ON c.space_id = s.id
	LEFT JOIN space_favourites f ON f.space_id = s.id AND f.user_id = $1
	ORDER BY
		CASE WHEN $4 = 'changed' THEN COALESCE(c.changed, s.updated_at) END DESC NULLS LAST,
		CASE WHEN $4 = 'pages' THEN COALESCE(c.pages, 0) END DESC NULLS LAST,
		lower(s.name), s.id
	LIMIT $5 OFFSET $6`

// Directory lists spaces for the directory screen, and how many there are in all.
func (s *Service) Directory(ctx context.Context, c identity.Caller, in DirectoryQuery) ([]DirectoryRow, int, error) {
	q, err := in.validated()
	if err != nil {
		return nil, 0, err
	}
	var category *uuid.UUID
	if q.Category != uuid.Nil {
		category = &q.Category
	}
	rows, err := s.pool.Query(ctx, directorySQL, c.UserID, q.Text, q.Kind, q.Sort, q.Limit, q.Offset, category)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (DirectoryRow, error) {
		var d DirectoryRow
		err := r.Scan(&d.ID, &d.Key, &d.Name, &d.Description, &d.Icon, &d.Status, &d.HomePageID, &d.CreatedBy,
			&d.CreatedAt, &d.UpdatedAt, &d.HasIcon, &d.TitleAlign, &d.ReviewMonths, &d.ApprovalRequired,
			&d.Kind, &d.OwnerID, &d.Favourite, &d.PageCount, &d.LastChanged, &total)
		return d, err
	})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(list))
	for i, d := range list {
		ids[i] = d.ID
	}
	admins, err := s.perms.Administrators(ctx, s.pool, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range list {
		list[i].Administrators = admins[list[i].ID]
		if list[i].Administrators == nil {
			list[i].Administrators = []string{}
		}
	}
	if list == nil {
		list = []DirectoryRow{}
	}
	return list, total, nil
}

// Mine is what the Spaces menu shows first.
type Mine struct {
	Favourites []Space `json:"favourites"`
	Recent     []Space `json:"recent"`
}

// Mine lists the favourite and recently visited spaces of a person. Both go
// through perm_spaces, so a space they lost access to disappears at once.
func (s *Service) Mine(ctx context.Context, c identity.Caller) (Mine, error) {
	var out Mine
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM spaces
		WHERE status = 'active' AND id IN (SELECT space_id FROM perm_spaces($1, 'space.view'))
		  AND id IN (SELECT space_id FROM space_favourites WHERE user_id = $1)
		ORDER BY lower(name), id`, c.UserID)
	if out.Favourites, err = collect(rows, err); err != nil {
		return out, err
	}
	recent, err := s.pages.RecentSpaceIDs(ctx, c)
	if err != nil {
		return out, err
	}
	rows, err = s.pool.Query(ctx, `SELECT `+columns+` FROM spaces
		WHERE status = 'active' AND id = ANY($2) AND id IN (SELECT space_id FROM perm_spaces($1, 'space.view'))
		  AND id NOT IN (SELECT space_id FROM space_favourites WHERE user_id = $1)
		ORDER BY array_position($2, id) LIMIT $3`, c.UserID, recent, maxMenuSpaces)
	out.Recent, err = collect(rows, err)
	return out, err
}

// SetFavourite stars or unstars a space. It is personal, so it is not audited.
func (s *Service) SetFavourite(ctx context.Context, c identity.Caller, id uuid.UUID, on bool) error {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, id, permissions.SpaceView); err != nil {
		return err
	}
	var err error
	if on {
		_, err = s.pool.Exec(ctx, "INSERT INTO space_favourites (user_id, space_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", c.UserID, id)
	} else {
		_, err = s.pool.Exec(ctx, "DELETE FROM space_favourites WHERE user_id = $1 AND space_id = $2", c.UserID, id)
	}
	return err
}

// Overview is what the overview screen of a space shows.
type Overview struct {
	pages.SpaceActivity
	Administrators []string     `json:"administrators"`
	Owners         []Person     `json:"owners"`
	Posts          []pages.Post `json:"posts"`
}

// Overview summarises a space for someone who may view it.
func (s *Service) Overview(ctx context.Context, c identity.Caller, id uuid.UUID) (Overview, error) {
	activity, err := s.pages.Activity(ctx, c, id)
	if err != nil {
		return Overview{}, err
	}
	admins, err := s.perms.Administrators(ctx, s.pool, []uuid.UUID{id})
	if err != nil {
		return Overview{}, err
	}
	out := Overview{SpaceActivity: activity, Administrators: admins[id]}
	if out.Administrators == nil {
		out.Administrators = []string{}
	}
	if out.Owners, err = s.Owners(ctx, s.pool, id); err != nil {
		return out, err
	}
	on, err := s.features.Enabled(ctx, s.pool, id, features.Blog)
	if err != nil {
		return out, err
	}
	out.Posts = []pages.Post{}
	if on {
		out.Posts, _, err = s.pages.Posts(ctx, c, &id, overviewPosts, 0)
	}
	return out, err
}

// Find looks for active spaces by name or key, for quick search.
func (s *Service) Find(ctx context.Context, c identity.Caller, text string, limit int) ([]Space, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM spaces
		WHERE status = 'active' AND id IN (SELECT space_id FROM perm_spaces($1, 'space.view'))
		  AND (position(lower($2) IN lower(name)) > 0 OR position(lower($2) IN lower(key)) > 0)
		ORDER BY lower(name), id LIMIT $3`, c.UserID, text, limit)
	return collect(rows, err)
}

// NoneExist reports whether the instance has no space at all, also not in the trash.
func (s *Service) NoneExist(ctx context.Context) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM spaces)").Scan(&found)
	return !found, err
}

// KeyFree reports whether a key can still be used. A space in the trash keeps its key.
func (s *Service) KeyFree(ctx context.Context, key string) (bool, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, "SELECT id FROM spaces WHERE lower(key) = lower($1)", key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return false, err
}
