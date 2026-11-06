// SPDX-License-Identifier: Apache-2.0

package analytics

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/csvout"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Period is the stretch of days a report covers, both ends included.
type Period struct {
	From time.Time
	To   time.Time
}

// PeriodOf reads a period from two dates. Empty means the last 30 days.
func (s *Service) PeriodOf(from, to string) (Period, error) {
	p := Period{From: s.today().AddDate(0, 0, -29), To: s.today()}
	var err error
	if from != "" {
		if p.From, err = time.Parse(time.DateOnly, from); err != nil {
			return p, apperr.Invalid("A period is two dates.")
		}
	}
	if to != "" {
		if p.To, err = time.Parse(time.DateOnly, to); err != nil {
			return p, apperr.Invalid("A period is two dates.")
		}
	}
	if p.To.Before(p.From) {
		return p, apperr.Invalid("A period ends after it starts.")
	}
	return p, nil
}

// Day is one point of a chart.
type Day struct {
	Day     string `json:"day"`
	Views   int    `json:"views"`
	Readers int    `json:"readers"`
	Engaged int    `json:"engaged"`
}

// LinkCount is a page and how many readers went that way.
type LinkCount struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
	Count int       `json:"count"`
}

// Reader is one person in name mode.
type Reader struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	FirstAt     time.Time `json:"first_at"`
	LastAt      time.Time `json:"last_at"`
	Views       int       `json:"views"`
	LastVersion int       `json:"last_version"`
}

// MentionedReader says whether someone mentioned in the page has read it.
type MentionedReader struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Read bool      `json:"read"`
}

// PageInsights is the screen of one page.
type PageInsights struct {
	Enabled   bool              `json:"enabled"`
	Mode      string            `json:"mode"`
	Views     int               `json:"views"`
	Days      []Day             `json:"days"`
	From      []LinkCount       `json:"from"`
	To        []LinkCount       `json:"to"`
	Readers   []Reader          `json:"readers"`
	Mentioned []MentionedReader `json:"mentioned"`
}

func collect[T any](rows pgx.Rows, err error, scan func(pgx.CollectableRow) (T, error)) ([]T, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scan)
	if out == nil {
		out = []T{}
	}
	return out, err
}

func scanLink(r pgx.CollectableRow) (LinkCount, error) {
	var l LinkCount
	err := r.Scan(&l.ID, &l.Title, &l.Count)
	return l, err
}

// PageReport returns the insights of a page for someone with view analytics.
func (s *Service) PageReport(ctx context.Context, c identity.Caller, id uuid.UUID, p Period) (PageInsights, error) {
	var out PageInsights
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.AnalyticsView, uuid.Nil); err != nil {
		return out, err
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return out, err
	}
	out.Enabled, out.Mode = st.Enabled, st.Mode
	rows, err := s.pool.Query(ctx, `SELECT to_char(day, 'YYYY-MM-DD'), views, readers, engaged FROM page_view_days
		WHERE page_id = $1 AND day BETWEEN $2 AND $3 ORDER BY day`, id, p.From, p.To)
	if out.Days, err = collect(rows, err, func(r pgx.CollectableRow) (Day, error) {
		var d Day
		err := r.Scan(&d.Day, &d.Views, &d.Readers, &d.Engaged)
		return d, err
	}); err != nil {
		return out, err
	}
	for _, d := range out.Days {
		out.Views += d.Views
	}
	rows, err = s.pool.Query(ctx, `SELECT p.id, p.title, sum(l.count)::int FROM page_view_links l
		JOIN perm_visible_pages($1) v ON v.page_id = l.from_id JOIN pages p ON p.id = l.from_id
		WHERE l.to_id = $2 AND l.day BETWEEN $3 AND $4 GROUP BY p.id, p.title ORDER BY 3 DESC LIMIT 10`, c.UserID, id, p.From, p.To)
	if out.From, err = collect(rows, err, scanLink); err != nil {
		return out, err
	}
	rows, err = s.pool.Query(ctx, `SELECT p.id, p.title, sum(l.count)::int FROM page_view_links l
		JOIN perm_visible_pages($1) v ON v.page_id = l.to_id JOIN pages p ON p.id = l.to_id
		WHERE l.from_id = $2 AND l.day BETWEEN $3 AND $4 GROUP BY p.id, p.title ORDER BY 3 DESC LIMIT 10`, c.UserID, id, p.From, p.To)
	if out.To, err = collect(rows, err, scanLink); err != nil {
		return out, err
	}
	out.Readers, out.Mentioned = []Reader{}, []MentionedReader{}
	if st.Mode != ModeNames {
		return out, nil
	}
	rows, err = s.pool.Query(ctx, `SELECT v.user_id, COALESCE(u.display_name, ''), v.first_at, v.last_at, v.views, v.last_version
		FROM page_view_people v LEFT JOIN users u ON u.id = v.user_id
		WHERE v.page_id = $1 ORDER BY v.last_at DESC LIMIT $2`, id, maxListRows)
	if out.Readers, err = collect(rows, err, func(r pgx.CollectableRow) (Reader, error) {
		var v Reader
		err := r.Scan(&v.ID, &v.Name, &v.FirstAt, &v.LastAt, &v.Views, &v.LastVersion)
		return v, err
	}); err != nil {
		return out, err
	}
	rows, err = s.pool.Query(ctx, `SELECT m.user_id, COALESCE(u.display_name, ''),
			EXISTS (SELECT 1 FROM page_view_people v WHERE v.page_id = m.page_id AND v.user_id = m.user_id)
		FROM page_mentions m LEFT JOIN users u ON u.id = m.user_id WHERE m.page_id = $1 ORDER BY 2`, id)
	out.Mentioned, err = collect(rows, err, func(r pgx.CollectableRow) (MentionedReader, error) {
		var v MentionedReader
		err := r.Scan(&v.ID, &v.Name, &v.Read)
		return v, err
	})
	return out, err
}

// ContentRow is a row of the content table of a space.
type ContentRow struct {
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	LastViewed *string   `json:"last_viewed"`
	Comments   int       `json:"comments"`
	Readers    int       `json:"readers"`
	Views      int       `json:"views"`
}

// SpaceReport lists what the space's pages did. Every row goes through
// perm_visible_pages for the person looking (PLT-006).
func (s *Service) SpaceReport(ctx context.Context, c identity.Caller, spaceID uuid.UUID, p Period) ([]ContentRow, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.AnalyticsView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT pg.id, pg.title, pg.created_at, pg.updated_at,
			(SELECT to_char(max(d.day), 'YYYY-MM-DD') FROM page_view_days d WHERE d.page_id = pg.id),
			(SELECT count(*) FROM comments cm WHERE cm.page_id = pg.id),
			COALESCE((SELECT sum(d.readers) FROM page_view_days d WHERE d.page_id = pg.id AND d.day BETWEEN $3 AND $4), 0)::int,
			COALESCE((SELECT sum(d.views) FROM page_view_days d WHERE d.page_id = pg.id AND d.day BETWEEN $3 AND $4), 0)::int
		FROM pages pg JOIN perm_visible_pages($1) v ON v.page_id = pg.id
		WHERE pg.space_id = $2 AND pg.status = 'published'
		ORDER BY 8 DESC, pg.updated_at DESC LIMIT $5`, c.UserID, spaceID, p.From, p.To, maxListRows)
	return collect(rows, err, func(r pgx.CollectableRow) (ContentRow, error) {
		var v ContentRow
		err := r.Scan(&v.ID, &v.Title, &v.CreatedAt, &v.UpdatedAt, &v.LastViewed, &v.Comments, &v.Readers, &v.Views)
		return v, err
	})
}

// WriteContentCSV exports the content table.
func WriteContentCSV(w io.Writer, list []ContentRow) error {
	out, err := csvout.New(w, "title", "created", "last changed", "last viewed", "comments", "readers", "views")
	if err != nil {
		return err
	}
	for _, r := range list {
		viewed := ""
		if r.LastViewed != nil {
			viewed = *r.LastViewed
		}
		if err := out.Row(r.Title, r.CreatedAt.Format(time.DateOnly), r.UpdatedAt.Format(time.DateOnly), viewed,
			strconv.Itoa(r.Comments), strconv.Itoa(r.Readers), strconv.Itoa(r.Views)); err != nil {
			return err
		}
	}
	return out.Close()
}

// Popular lists the most viewed pages of a space, for its overview (SPC-019).
func (s *Service) Popular(ctx context.Context, c identity.Caller, spaceID uuid.UUID, limit int) ([]LinkCount, error) {
	rows, err := s.pool.Query(ctx, `SELECT pg.id, pg.title, sum(d.views)::int FROM page_view_days d
		JOIN pages pg ON pg.id = d.page_id JOIN perm_visible_pages($1) v ON v.page_id = pg.id
		WHERE pg.space_id = $2 AND d.day >= $3 GROUP BY pg.id, pg.title ORDER BY 3 DESC LIMIT $4`,
		c.UserID, spaceID, s.today().AddDate(0, 0, -29), limit)
	return collect(rows, err, scanLink)
}

// ViewCount is the number of the byline, for those who may see it (PAG-105). The
// second answer says whether this person may.
func (s *Service) ViewCount(ctx context.Context, c identity.Caller, id uuid.UUID) (int, bool, error) {
	st, err := s.Settings(ctx)
	if err != nil || !st.Enabled {
		return 0, false, err
	}
	d, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, permissions.AnalyticsView, uuid.Nil)
	if err != nil || !d.Allowed {
		return 0, false, err
	}
	var views int
	err = s.pool.QueryRow(ctx, `SELECT COALESCE(sum(views), 0)::int FROM page_view_days WHERE page_id = $1 AND day >= $2`,
		id, s.today().AddDate(0, 0, -29)).Scan(&views)
	return views, err == nil, err
}

// Stale lists the pages of a space that nobody changed for a year (PLT-020).
func (s *Service) Stale(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]LinkCount, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.AnalyticsView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT pg.id, pg.title, 0 FROM pages pg JOIN perm_visible_pages($1) v ON v.page_id = pg.id
		WHERE pg.space_id = $2 AND pg.status = 'published' AND pg.updated_at < $3 ORDER BY pg.updated_at LIMIT 10`,
		c.UserID, spaceID, s.now().AddDate(-1, 0, 0))
	return collect(rows, err, scanLink)
}

// PersonRow is one person in a space, in name mode only (PLT-018).
type PersonRow struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Pages    int       `json:"pages"`
	Edits    int       `json:"edits"`
	Comments int       `json:"comments"`
	Views    int       `json:"views"`
}

// SpacePeople lists people in a space. It answers nothing in counts mode.
func (s *Service) SpacePeople(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]PersonRow, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.AnalyticsView); err != nil {
		return nil, err
	}
	st, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	if st.Mode != ModeNames {
		return []PersonRow{}, nil
	}
	rows, err := s.pool.Query(ctx, `WITH visible AS (
			SELECT pg.id, pg.created_by FROM pages pg JOIN perm_visible_pages($1) v ON v.page_id = pg.id WHERE pg.space_id = $2
		)
		SELECT x.user_id, COALESCE(u.display_name, ''), sum(x.pages)::int, sum(x.edits)::int, sum(x.comments)::int, sum(x.views)::int
		FROM (
			SELECT created_by AS user_id, 1 AS pages, 0 AS edits, 0 AS comments, 0 AS views FROM visible
			UNION ALL SELECT pv.author_id, 0, 1, 0, 0 FROM page_versions pv JOIN visible g ON g.id = pv.page_id
			UNION ALL SELECT cm.author_id, 0, 0, 1, 0 FROM comments cm JOIN visible g ON g.id = cm.page_id
			UNION ALL SELECT w.user_id, 0, 0, 0, w.views FROM page_view_people w JOIN visible g ON g.id = w.page_id
		) x JOIN users u ON u.id = x.user_id
		GROUP BY x.user_id, u.display_name ORDER BY 6 DESC, 2 LIMIT $3`, c.UserID, spaceID, maxListRows)
	return collect(rows, err, func(r pgx.CollectableRow) (PersonRow, error) {
		var v PersonRow
		err := r.Scan(&v.ID, &v.Name, &v.Pages, &v.Edits, &v.Comments, &v.Views)
		return v, err
	})
}

// InstanceDay is one day of the instance. The route requires administrator.
type InstanceDay struct {
	Day         string `json:"day"`
	ActiveUsers int    `json:"active_users"`
	Views       int    `json:"views"`
	NewPages    int    `json:"new_pages"`
	Comments    int    `json:"comments"`
	Searches    int    `json:"searches"`
}

// InstanceReport lists closed days. The route requires administrator.
func (s *Service) InstanceReport(ctx context.Context, p Period) ([]InstanceDay, error) {
	rows, err := s.pool.Query(ctx, `SELECT to_char(day, 'YYYY-MM-DD'), active_users, views, new_pages, comments, searches
		FROM analytics_days WHERE day BETWEEN $1 AND $2 ORDER BY day`, p.From, p.To)
	return collect(rows, err, func(r pgx.CollectableRow) (InstanceDay, error) {
		var v InstanceDay
		err := r.Scan(&v.Day, &v.ActiveUsers, &v.Views, &v.NewPages, &v.Comments, &v.Searches)
		return v, err
	})
}

// WriteInstanceCSV exports the days of the instance.
func WriteInstanceCSV(w io.Writer, list []InstanceDay) error {
	out, err := csvout.New(w, "day", "active users", "views", "new pages", "comments", "searches")
	if err != nil {
		return err
	}
	for _, d := range list {
		if err := out.Row(d.Day, strconv.Itoa(d.ActiveUsers), strconv.Itoa(d.Views), strconv.Itoa(d.NewPages),
			strconv.Itoa(d.Comments), strconv.Itoa(d.Searches)); err != nil {
			return err
		}
	}
	return out.Close()
}

// Term is a search term, without any person.
type Term struct {
	Term     string `json:"term"`
	Searches int    `json:"searches"`
	Empty    int    `json:"empty"`
}

// Terms lists what was searched and what found nothing. A term can hold a name, so
// rare ones never show. The route requires administrator.
func (s *Service) Terms(ctx context.Context, p Period) ([]Term, error) {
	rows, err := s.pool.Query(ctx, `SELECT term, sum(searches)::int, sum(empty)::int FROM search_terms
		WHERE day BETWEEN $1 AND $2 GROUP BY term HAVING sum(searches) >= $3 ORDER BY 2 DESC, 1 LIMIT $4`,
		p.From, p.To, minTermUses, maxListRows)
	return collect(rows, err, func(r pgx.CollectableRow) (Term, error) {
		var v Term
		err := r.Scan(&v.Term, &v.Searches, &v.Empty)
		return v, err
	})
}
