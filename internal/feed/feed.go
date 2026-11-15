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
	// BeforeID is the page of the last item seen; with Before it makes the cursor exact.
	BeforeID uuid.UUID
	Limit    int
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
	// Pages is the number of pages of a run of a release the person may see; the item is then
	// of kind released, on the home page of the published space, with the note as its excerpt.
	Pages int `json:"pages,omitempty"`
}

// Service reads feeds.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// visible asks whether the page of a row may be seen. As a plain IN or EXISTS, PostgreSQL
// inlines the rule and works out the person's spaces again for every candidate, 6 ms each,
// or builds the whole set first and gives up reading the newest versions first: 230 to
// 360 ms for twenty items on real data (spec 061). Inside an expression the set is built
// once, as a hash, about 30 ms, and the newest-first walk stays. perm_visible_pages stays
// the rule.
func visible(column string) string {
	return `(CASE WHEN ` + column + ` IN (SELECT page_id FROM perm_visible_pages($1)) THEN true END)`
}

// readable keeps a version that readers may see: none above the current one, which on a
// controlled page waits for review (spec 064). As a subquery per version it keeps the
// newest-first walk.
func readable(version string) string {
	return version + `.number <= (SELECT cv.current_version FROM pages cv WHERE cv.id = ` + version + `.page_id)`
}

// One row per page: its newest version that was not silent. A silent version reached
// nobody, so it does not show here either. The versions are read newest first, and each
// is asked whether it is the newest of its page and whether its page is visible, so a
// page of twenty items reads about twenty versions (spec 050).
var changesSQL = `WITH changes AS (
		SELECT v.page_id, v.author_id, v.created_at AS at,
			CASE WHEN v.number = 1 THEN 'created' ELSE 'changed' END AS kind, NULL::text AS note, 0 AS pages
		FROM page_versions v
		WHERE NOT v.silent AND ` + readable("v") + `
		  AND NOT EXISTS (SELECT 1 FROM page_versions n WHERE n.page_id = v.page_id AND n.number > v.number AND NOT n.silent
			AND ` + readable("n") + `)
		  AND ` + visible("v.page_id") + `
		  AND NOT ` + byRun("v") + `
		UNION ALL ` + runItems + `
	)` + changesTail

// byRun asks whether a run of a release of many pages wrote a version. Such a run is one item
// of its own, so its pages do not show one by one (spec 073, PUB-810). The start of a draft space
// wrote no version readers have, so it hides none and is no item (PUB-1302).
func byRun(version string) string {
	return `EXISTS (SELECT 1 FROM release_run_pages rp JOIN release_runs rr ON rr.id = rp.run_id AND rr.scope NOT IN ('page', 'start')
		WHERE rp.twin_page_id = ` + version + `.page_id AND rp.twin_version = ` + version + `.number)`
}

// runItems are the runs of a release of many pages, each one item on the home page of the
// published space, with the note and the number of its pages the person may see (PUB-810). A
// silent run is in the feed too, as it only keeps people from being told (PUB-821).
var runItems = `SELECT pub.home_page_id, r.actor_id, r.finished_at, 'released', r.note,
			(SELECT count(*)::int FROM release_run_pages rp WHERE rp.run_id = r.id AND rp.outcome = 'released'
				AND ` + visible("rp.twin_page_id") + `)
		FROM release_runs r JOIN space_pairs sp ON sp.id = r.pair_id JOIN spaces pub ON pub.id = sp.published_space_id
		WHERE r.scope NOT IN ('page', 'start') AND r.status = 'done' AND pub.home_page_id IS NOT NULL AND ` + visible("pub.home_page_id")

// "For you" holds only pages a person watches, pages below a branch they watch, pages
// of spaces they watch, and pages of people they follow. Walking every version newest
// first and dropping almost all of them took over two minutes on 115,000 versions
// (spec 061), so those pages are gathered first, and each gives its newest version that
// was not silent. The filter of changesTail still decides, so the rule is the same. The
// current version of each candidate bounds its walk, so a version under review never shows
// (spec 064); asked per version instead, it cost the watcher of every page 200 ms more.
var forYouSQL = `WITH candidates AS (
		SELECT w.target_id AS page_id FROM watches w WHERE w.user_id = $1 AND w.target_type = 'page'
		UNION
		SELECT p.id FROM watches w JOIN pages p ON p.space_id = w.target_id
		WHERE w.user_id = $1 AND w.target_type = 'space'
		UNION
		SELECT t.descendant_id FROM watches w JOIN page_tree t ON t.ancestor_id = w.target_id
		WHERE w.user_id = $1 AND w.target_type = 'branch'
		UNION
		SELECT v.page_id FROM follows f JOIN page_versions v ON v.author_id = f.followed_id
		WHERE f.user_id = $1
	),
	changes AS (
		SELECT v.page_id, v.author_id, v.created_at AS at,
			CASE WHEN v.number = 1 THEN 'created' ELSE 'changed' END AS kind, NULL::text AS note, 0 AS pages
		FROM candidates k
		JOIN pages cp ON cp.id = k.page_id
		CROSS JOIN LATERAL (
			SELECT n.page_id, n.author_id, n.created_at, n.number FROM page_versions n
			WHERE n.page_id = k.page_id AND NOT n.silent AND n.number <= cp.current_version
			ORDER BY n.number DESC LIMIT 1) v
		WHERE ` + visible("v.page_id") + ` AND NOT ` + byRun("v") + `
		UNION ALL ` + runItems + ` AND pub.id IN (SELECT w.target_id FROM watches w WHERE w.user_id = $1 AND w.target_type = 'space')
	)` + changesTail

// approvedByAPerson is a controlled page whose version readers see was approved by a person.
// Readers learn of it as approved, never of the working changes after it (spec 064, DOC-570).
// It is asked of the rows of one page of the feed only, so the newest-first walk of versions
// stays as it was; the item keeps the time its version was written (DC3-23).
func approvedByAPerson(page string) string {
	return `EXISTS (SELECT 1 FROM page_control pa WHERE pa.page_id = ` + page + ` AND pa.approved_by IS NOT NULL)`
}

// changesTail turns changes into items, and applies the space, the cursor and, for
// "for you", who made the change and why the person sees it. Whether an item was approved is
// asked outside, of the rows of this page of the feed only; a release run stays what it is.
var changesTail = `
	SELECT f.page_id, f.title, f.emoji, f.key, f.name, f.excerpt, f.author,
		CASE WHEN f.kind <> 'released' AND ` + approvedByAPerson("f.page_id") + ` THEN 'approved' ELSE f.kind END, f.at, f.reactions, f.comments,
		f.pages
	FROM (SELECT c.page_id, p.title, p.emoji, s.key, s.name, COALESCE(c.note, left(d.body, $5), '') AS excerpt, COALESCE(u.display_name, '') AS author,
		c.kind, c.at,
		(SELECT count(*) FROM reactions r WHERE r.target_type = 'page' AND r.target_id = c.page_id) AS reactions,
		(SELECT count(*) FROM comments m WHERE m.page_id = c.page_id AND m.trash_item_id IS NULL AND NOT m.purged AND NOT m.editors_only) AS comments,
		c.pages
	FROM changes c
	JOIN pages p ON p.id = c.page_id AND p.status = 'published'
	JOIN spaces s ON s.id = p.space_id AND s.status = 'active'
	LEFT JOIN search_documents d ON d.page_id = c.page_id
	LEFT JOIN users u ON u.id = c.author_id
	WHERE ($2::uuid IS NULL OR p.space_id = $2) AND (c.kind <> 'released' OR c.pages > 0)
	  AND ($3::timestamptz IS NULL OR c.at < $3 OR (c.at = $3 AND $7::uuid IS NOT NULL AND c.page_id > $7))
	  AND (NOT $6 OR (c.author_id <> $1 AND (
		c.author_id IN (SELECT followed_id FROM follows WHERE user_id = $1)
		OR EXISTS (SELECT 1 FROM watches w WHERE w.user_id = $1 AND (
			(w.target_type = 'page' AND w.target_id = c.page_id)
			OR (w.target_type = 'space' AND w.target_id = p.space_id)
			OR (w.target_type = 'branch' AND w.target_id IN
				(SELECT t.ancestor_id FROM page_tree t WHERE t.descendant_id = c.page_id)))))))
	ORDER BY c.at DESC, c.page_id LIMIT $4) f
	ORDER BY f.at DESC, f.page_id`

// Popular pages count what happened to them in the last days, among comments that
// the person may see. Only a page with a reaction or comment in those days can score, so
// those pages are the candidates, and only they are asked whether they are visible
// (spec 050). The reactions and comments of those days are counted in one pass: counted
// page by page, 20,000 active pages took about 300 ms on real data (spec 061).
var popularSQL = `WITH counted AS (
		SELECT page_id, sum(reactions)::bigint AS reactions, sum(comments)::bigint AS comments FROM (
			SELECT r.target_id AS page_id, 1 AS reactions, 0 AS comments FROM reactions r
			WHERE r.target_type = 'page' AND r.created_at > now() - make_interval(days => $4)
			UNION ALL
			SELECT m.page_id, 0, 1 FROM comments m
			WHERE m.trash_item_id IS NULL AND NOT m.purged AND NOT m.editors_only AND m.created_at > now() - make_interval(days => $4)
		) recent
		GROUP BY page_id
	),
	scores AS (
		SELECT v.page_id, v.reactions, v.comments
		FROM counted v
		WHERE ` + visible("v.page_id") + `
	),
	-- The top pages first; the excerpt and the name are read for those alone.
	top AS (
		SELECT c.page_id, c.reactions, c.comments, p.title, p.emoji, p.updated_at, p.updated_by, s.key, s.name
		FROM scores c
		JOIN pages p ON p.id = c.page_id AND p.status = 'published'
		JOIN spaces s ON s.id = p.space_id AND s.status = 'active'
		WHERE c.reactions + c.comments > 0 AND ($2::uuid IS NULL OR p.space_id = $2)
		ORDER BY c.reactions + c.comments DESC, p.updated_at DESC, c.page_id LIMIT $3
	)
	SELECT t.page_id, t.title, t.emoji, t.key, t.name, COALESCE(left(d.body, $5), ''), COALESCE(u.display_name, ''), 'popular', t.updated_at,
		t.reactions, t.comments, 0
	FROM top t
	LEFT JOIN search_documents d ON d.page_id = t.page_id
	LEFT JOIN users u ON u.id = t.updated_by
	ORDER BY t.reactions + t.comments DESC, t.updated_at DESC, t.page_id`

func scan(r pgx.CollectableRow) (Item, error) {
	var v Item
	err := r.Scan(&v.PageID, &v.Title, &v.Emoji, &v.SpaceKey, &v.SpaceName, &v.Excerpt, &v.AuthorName, &v.Kind, &v.At, &v.Reactions, &v.Comments, &v.Pages)
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
	var beforeID *uuid.UUID
	if q.BeforeID != uuid.Nil {
		beforeID = &q.BeforeID
	}
	var rows pgx.Rows
	var err error
	switch q.Tab {
	case TabPopular:
		rows, err = s.pool.Query(ctx, popularSQL, c.UserID, space, limit, popularDays, excerptChars)
	case TabForYou:
		rows, err = s.pool.Query(ctx, forYouSQL, c.UserID, space, before, limit, excerptChars, true, beforeID)
	default:
		rows, err = s.pool.Query(ctx, changesSQL, c.UserID, space, before, limit, excerptChars, false, beforeID)
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
