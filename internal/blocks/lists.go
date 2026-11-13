// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// filter selects pages. Values in one field mean or, fields mean and, and every
// page has passed perm_visible_pages (spec 012, E5).
type filter struct {
	kinds        []string
	labels       []string
	excluded     []string
	title        string
	below        *uuid.UUID
	space        *uuid.UUID
	maxDepth     int
	sort         string
	descending   bool
	max          int
	text         string
	creators     []string
	contributors []string
	mentioned    []string
	createdAfter *time.Time
	changedAfter *time.Time
	spaceKeys    []string
	favourites   bool
	network      bool
}

func lower(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(strings.TrimSpace(v))
	}
	return out
}

func upper(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToUpper(strings.TrimSpace(v))
	}
	return out
}

// oneOrNone turns a setting that holds one value into a list of none or one.
func oneOrNone(value string) []string {
	if value == "" {
		return nil
	}
	return []string{value}
}

// The sort key comes from a short list, and is a value here, so no SQL is built. A
// person in a filter is a name or an address. It can only narrow what passed the check.
const filterSQL = `SELECT ` + pageColumns + `
	FROM pages p
	JOIN perm_visible_pages($1) v ON v.page_id = p.id
	JOIN spaces s ON s.id = p.space_id
	LEFT JOIN users u ON u.id = p.updated_by
	LEFT JOIN page_tree t ON t.descendant_id = p.id AND t.ancestor_id = $5
	WHERE p.status = 'published' AND p.archived_at IS NULL AND s.status = 'active' AND p.kind = ANY($9)
	  AND (cardinality($2::text[]) = 0 OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($2)))
	  AND NOT EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($3))
	  AND ($4 = '' OR position(lower($4) IN lower(p.title)) > 0)
	  AND ($5::uuid IS NULL OR (t.depth > 0 AND ($10 = 0 OR t.depth <= $10)))
	  AND ($6::uuid IS NULL OR p.space_id = $6)
	  AND ($12 = '' OR EXISTS (SELECT 1 FROM search_documents d WHERE d.page_id = p.id AND d.tsv @@ plainto_tsquery('simple', $12)))
	  AND (cardinality($13::text[]) = 0 OR p.created_by IN (SELECT id FROM users WHERE lower(email) = ANY($13) OR lower(display_name) = ANY($13)))
	  AND (cardinality($14::text[]) = 0 OR EXISTS (SELECT 1 FROM page_versions pv JOIN users c ON c.id = pv.author_id
		WHERE pv.page_id = p.id AND (lower(c.email) = ANY($14) OR lower(c.display_name) = ANY($14))))
	  AND (cardinality($15::text[]) = 0 OR EXISTS (SELECT 1 FROM page_mentions m JOIN users c ON c.id = m.user_id
		WHERE m.page_id = p.id AND (lower(c.email) = ANY($15) OR lower(c.display_name) = ANY($15))))
	  AND ($16::timestamptz IS NULL OR p.created_at >= $16)
	  AND ($17::timestamptz IS NULL OR p.updated_at >= $17)
	  AND (cardinality($18::text[]) = 0 OR s.key = ANY($18))
	  AND (NOT $19 OR s.id IN (SELECT f.space_id FROM space_favourites f WHERE f.user_id = $1))
	  AND (NOT $20 OR p.updated_by IN (SELECT w.followed_id FROM follows w WHERE w.user_id = $1))
	ORDER BY
		CASE WHEN $7 = 'title' AND NOT $8 THEN lower(p.title) END,
		CASE WHEN $7 = 'title' AND $8 THEN lower(p.title) END DESC,
		CASE WHEN $7 = 'created' AND NOT $8 THEN p.created_at END,
		CASE WHEN $7 = 'created' AND $8 THEN p.created_at END DESC,
		CASE WHEN $7 = 'changed' AND NOT $8 THEN p.updated_at END,
		CASE WHEN $7 = 'changed' AND $8 THEN p.updated_at END DESC,
		t.depth, p.position, p.id
	LIMIT $11`

func (r reader) pages(ctx context.Context, f filter) ([]Row, error) {
	if len(f.kinds) == 0 {
		f.kinds = []string{"page"}
	}
	rows, err := r.s.pool.Query(ctx, filterSQL, r.userID, lower(f.labels), lower(f.excluded), f.title,
		f.below, f.space, f.sort, f.descending, f.kinds, f.maxDepth, f.max, f.text, lower(f.creators),
		lower(f.contributors), lower(f.mentioned), f.createdAfter, f.changedAfter, upper(f.spaceKeys),
		f.favourites, f.network)
	return scanRows(rows, err, pageRow)
}

// date reads a setting the gate has checked. Without one there is no limit.
func (r reader) date(key string) *time.Time {
	d, err := time.Parse(time.DateOnly, r.text(key))
	if err != nil {
		return nil
	}
	return &d
}

func kindsOf(contentType string) []string {
	switch contentType {
	case "post":
		return []string{"post"}
	case "all":
		return []string{"page", "post"}
	}
	return nil
}

// fromSettings reads the filter that many kinds share.
func (r reader) fromSettings(fallback int) filter {
	f := filter{labels: r.list("labels"), excluded: r.list("exclude_labels"), title: r.text("title"),
		space: r.space(), sort: r.text("sort"), descending: r.on("descending"), max: r.max(fallback),
		kinds: kindsOf(r.text("content_type")), text: r.text("text"), creators: oneOrNone(r.text("creator")),
		contributors: oneOrNone(r.text("contributor")), mentioned: oneOrNone(r.text("mentioned")),
		createdAfter: r.date("created_after"), changedAfter: r.date("changed_after"), spaceKeys: r.list("spaces")}
	if id, err := uuid.Parse(r.text("below")); err == nil {
		f.below = &id
		if r.on("direct") {
			f.maxDepth = 1
		}
	}
	if f.sort == "" {
		f.sort = "changed"
		f.descending = true
	}
	return f
}

// fillPreviews adds the first words of each page. The rows have passed the check, and
// nothing else is read.
func (r reader) fillPreviews(ctx context.Context, rows []Row) error {
	found, err := r.s.pool.Query(ctx, `SELECT page_id, left(body, $2) FROM search_documents WHERE page_id = ANY($1)`,
		rowIDs(rows), previewRunes)
	if err != nil {
		return err
	}
	defer found.Close()
	byPage := map[uuid.UUID]string{}
	for found.Next() {
		var id uuid.UUID
		var text string
		if err := found.Scan(&id, &text); err != nil {
			return err
		}
		byPage[id] = text
	}
	for i := range rows {
		rows[i].Detail = byPage[rows[i].ID]
	}
	return found.Err()
}

func (r reader) withPreview(ctx context.Context, rows []Row) error {
	if !r.on("preview") {
		return nil
	}
	return r.fillPreviews(ctx, rows)
}

func rowIDs(rows []Row) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids
}

func childPages(ctx context.Context, r reader) (Answer, error) {
	parent := r.target()
	if ok, err := r.visible(ctx, parent); !ok {
		return Answer{}, err
	}
	levels, _ := r.settings["levels"].(int)
	if levels == 0 {
		levels = 1
	}
	rows, err := r.pages(ctx, filter{below: &parent, maxDepth: levels, sort: r.text("sort"),
		descending: r.on("descending"), max: r.max(defaultRows)})
	if err != nil {
		return Answer{}, err
	}
	return Answer{Rows: rows}, r.withPreview(ctx, rows)
}

func pageTree(ctx context.Context, r reader) (Answer, error) {
	parent := r.target()
	if ok, err := r.visible(ctx, parent); !ok {
		return Answer{}, err
	}
	rows, err := r.pages(ctx, filter{below: &parent, max: maxRows})
	return Answer{Rows: rows}, err
}

func spaceIndex(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.pages(ctx, filter{space: &r.spaceID, sort: "title", max: maxRows})
	return Answer{Rows: rows}, err
}

func recentChanges(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.pages(ctx, filter{space: r.space(), sort: "changed", descending: true, max: r.max(defaultRows),
		spaceKeys: r.list("spaces")})
	return Answer{Rows: rows}, err
}

// activityTabs is recent changes for one tab: everything, favourite spaces, or the
// people the reader follows. Each tab asks again, so each is a list like any other.
func activityTabs(ctx context.Context, r reader) (Answer, error) {
	scope := r.text("scope")
	rows, err := r.pages(ctx, filter{kinds: []string{"page", "post"}, sort: "changed", descending: true,
		max: r.max(defaultRows), favourites: scope == "favourites", network: scope == "network"})
	return Answer{Rows: rows}, err
}

func popularPages(ctx context.Context, r reader) (Answer, error) {
	days, _ := r.settings["days"].(int)
	if days == 0 {
		days = defaultPopularDays
	}
	found, err := r.s.analytics.PopularPages(ctx, r.caller, r.space(), days, r.max(defaultRows))
	if err != nil {
		return Answer{}, err
	}
	rows := make([]Row, len(found))
	for i, page := range found {
		rows[i] = Row{ID: page.ID, Title: page.Title, Count: page.Count}
	}
	return Answer{Rows: rows}, nil
}

func filterList(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.pages(ctx, r.fromSettings(defaultRows))
	if err != nil {
		return Answer{}, err
	}
	return Answer{Rows: rows}, r.withPreview(ctx, rows)
}

func blogPosts(ctx context.Context, r reader) (Answer, error) {
	f := r.fromSettings(defaultRows)
	f.kinds = []string{"post"}
	f.creators = r.list("authors")
	if days, _ := r.settings["days"].(int); days > 0 {
		since := time.Now().AddDate(0, 0, -days)
		f.createdAfter = &since
	}
	rows, err := r.pages(ctx, f)
	return Answer{Rows: rows}, err
}

// linkCards shows chosen pages, or the pages of a filter when none are chosen.
func linkCards(ctx context.Context, r reader) (Answer, error) {
	out, err := r.cardRows(ctx)
	if err != nil {
		return Answer{}, err
	}
	if err := r.fillPreviews(ctx, out); err != nil {
		return Answer{}, err
	}
	return Answer{Rows: out}, r.fillImages(ctx, out)
}

func (r reader) cardRows(ctx context.Context) ([]Row, error) {
	ids := r.pagesSetting()
	if len(ids) == 0 {
		return r.pages(ctx, r.fromSettings(defaultCards))
	}
	rows, err := r.s.pool.Query(ctx, `SELECT `+pageColumns+` FROM pages p
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		JOIN spaces s ON s.id = p.space_id
		LEFT JOIN users u ON u.id = p.updated_by
		WHERE p.id = ANY($2) AND p.status = 'published' ORDER BY array_position($2, p.id)`, r.userID, ids)
	return scanRows(rows, err, pageRow)
}

// fillImages adds the header picture of each page. The rows have passed the check, and
// the file route checks again when the browser asks for the picture.
func (r reader) fillImages(ctx context.Context, rows []Row) error {
	found, err := r.s.pool.Query(ctx, `SELECT id, header_attachment_id FROM pages
		WHERE id = ANY($1) AND header_attachment_id IS NOT NULL`, rowIDs(rows))
	if err != nil {
		return err
	}
	defer found.Close()
	byPage := map[uuid.UUID]uuid.UUID{}
	for found.Next() {
		var page, image uuid.UUID
		if err := found.Scan(&page, &image); err != nil {
			return err
		}
		byPage[page] = image
	}
	for i := range rows {
		if image, ok := byPage[rows[i].ID]; ok {
			rows[i].Image = &image
		}
	}
	return found.Err()
}

func (r reader) pagesSetting() []uuid.UUID {
	out := []uuid.UUID{}
	for _, s := range r.list("pages") {
		if id, err := uuid.Parse(s); err == nil {
			out = append(out, id)
		}
	}
	return out
}

func breadcrumbs(ctx context.Context, r reader) (Answer, error) {
	id := r.target()
	if ok, err := r.visible(ctx, id); !ok {
		return Answer{}, err
	}
	rows, err := r.s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji FROM page_tree t
		JOIN pages p ON p.id = t.ancestor_id
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE t.descendant_id = $2 ORDER BY t.depth DESC`, r.userID, id)
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		return row.Scan(&v.ID, &v.Title, &v.Emoji)
	})
	return Answer{Rows: out}, err
}

func spacesList(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT s.id, s.name, s.key FROM spaces s
		WHERE s.status = 'active' AND s.id IN (SELECT space_id FROM perm_spaces($1, 'space.view'))
		  AND (NOT $2 OR s.id IN (SELECT f.space_id FROM space_favourites f WHERE f.user_id = $1))
		  AND (cardinality($3::text[]) = 0 OR EXISTS (SELECT 1 FROM space_category_links l
			JOIN space_categories c ON c.id = l.category_id WHERE l.space_id = s.id AND lower(c.name) = ANY($3)))
		ORDER BY lower(s.name), s.id LIMIT 200`, r.userID, r.on("favourites"), lower(r.list("categories")))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		return row.Scan(&v.ID, &v.Title, &v.SpaceKey)
	})
	return Answer{Rows: out}, err
}

func labelsList(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT l.label, count(*) FROM page_labels l
		JOIN perm_visible_pages($1) v ON v.page_id = l.page_id
		JOIN pages p ON p.id = l.page_id
		WHERE p.status = 'published' AND ($2::uuid IS NULL OR p.space_id = $2)
		GROUP BY l.label ORDER BY l.label LIMIT $3`, r.userID, r.space(), r.max(maxRows))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		return row.Scan(&v.Title, &v.Count)
	})
	return Answer{Rows: out}, err
}

func myFavourites(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT `+pageColumns+` FROM favourites f
		JOIN pages p ON p.id = f.page_id
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		JOIN spaces s ON s.id = p.space_id
		LEFT JOIN users u ON u.id = p.updated_by
		WHERE f.user_id = $1 ORDER BY f.position NULLS FIRST, f.created_at DESC LIMIT $2`, r.userID, r.max(defaultRows))
	out, err := scanRows(rows, err, pageRow)
	return Answer{Rows: out}, err
}

func myDrafts(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT `+pageColumns+` FROM pages p
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		JOIN spaces s ON s.id = p.space_id
		LEFT JOIN users u ON u.id = p.updated_by
		LEFT JOIN drafts d ON d.page_id = p.id AND d.user_id = $1
		WHERE d.page_id IS NOT NULL OR (p.status = 'draft' AND p.created_by = $1)
		ORDER BY COALESCE(d.updated_at, p.updated_at) DESC LIMIT $2`, r.userID, r.max(defaultRows))
	out, err := scanRows(rows, err, pageRow)
	return Answer{Rows: out}, err
}

const taskColumns = `k.id, k.text, k.done, k.due_on, p.id, p.title, COALESCE(u.display_name, '')`

func scanTask(row pgx.CollectableRow, v *Row) error {
	var due *time.Time
	var done bool
	var pageID uuid.UUID
	var pageTitle string
	if err := row.Scan(&v.ID, &v.Title, &done, &due, &pageID, &pageTitle, &v.By); err != nil {
		return err
	}
	v.Done, v.At, v.ParentID, v.Detail = &done, due, &pageID, pageTitle
	return nil
}

func myTasks(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT `+taskColumns+` FROM page_tasks k
		JOIN pages p ON p.id = k.page_id
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		LEFT JOIN users u ON u.id = k.assignee_id
		WHERE k.assignee_id = $1 AND NOT k.done ORDER BY k.due_on NULLS LAST, k.id LIMIT $2`, r.userID, r.max(defaultRows))
	out, err := scanRows(rows, err, scanTask)
	return Answer{Rows: out}, err
}

func taskReport(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT `+taskColumns+` FROM page_tasks k
		JOIN pages p ON p.id = k.page_id
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		LEFT JOIN users u ON u.id = k.assignee_id
		WHERE ($2::uuid IS NULL OR k.space_id = $2) AND k.done = $3 AND (NOT $4 OR k.assignee_id = $1)
		  AND (cardinality($5::text[]) = 0 OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($5)))
		  AND ($7 = '' OR k.creator_id IN (SELECT id FROM users WHERE lower(email) = lower($7) OR lower(display_name) = lower($7)))
		  AND ($8::timestamptz IS NULL OR k.created_at >= $8)
		ORDER BY k.due_on NULLS LAST, k.id LIMIT $6`,
		r.userID, r.space(), r.on("done"), r.on("mine"), lower(r.list("labels")), r.max(defaultRows),
		r.text("created_by"), r.date("created_after"))
	out, err := scanRows(rows, err, scanTask)
	return Answer{Rows: out}, err
}

// decisionReport filters by page and person. A decision has no person of its own, so a
// person is one who is named in it, or who created the page (spec 026).
func decisionReport(ctx context.Context, r reader) (Answer, error) {
	var page *uuid.UUID
	if id, err := uuid.Parse(r.text("page")); err == nil {
		page = &id
	}
	rows, err := r.s.pool.Query(ctx, `SELECT p.id, d.text, p.title, p.updated_at FROM page_decisions d
		JOIN pages p ON p.id = d.page_id
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.status = 'published' AND ($2::uuid IS NULL OR p.space_id = $2)
		  AND (cardinality($3::text[]) = 0 OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($3)))
		  AND ($5::uuid IS NULL OR p.id IN (SELECT descendant_id FROM page_tree WHERE ancestor_id = $5))
		  AND ($6 = '' OR EXISTS (SELECT 1 FROM users c WHERE (lower(c.email) = lower($6) OR lower(c.display_name) = lower($6))
			AND (c.id = p.created_by OR position(lower(c.display_name) IN lower(d.text)) > 0)))
		ORDER BY p.updated_at DESC, p.id, d.position LIMIT $4`,
		r.userID, r.space(), lower(r.list("labels")), r.max(defaultRows), page, r.text("person"))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		err := row.Scan(&v.ID, &v.Title, &v.Detail, &at)
		v.At = &at
		return err
	})
	return Answer{Rows: out}, err
}

func historyHere(ctx context.Context, r reader) (Answer, error) {
	id := r.target()
	if ok, err := r.visible(ctx, id); !ok {
		return Answer{}, err
	}
	rows, err := r.s.pool.Query(ctx, `SELECT v.number, v.summary, v.created_at, COALESCE(u.display_name, '')
		FROM page_versions v LEFT JOIN users u ON u.id = v.author_id
		WHERE v.page_id = $1 AND v.number <= (SELECT current_version FROM pages WHERE id = $1)
		ORDER BY v.number DESC LIMIT $2`, id, r.max(5)) // a version waiting for review is not history yet (spec 064)
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		err := row.Scan(&v.Count, &v.Title, &at, &v.By)
		v.ID, v.At = id, &at
		return err
	})
	return Answer{Rows: out}, err
}

func attachmentsHere(ctx context.Context, r reader) (Answer, error) {
	id := r.target()
	if ok, err := r.visible(ctx, id); !ok {
		return Answer{}, err
	}
	pattern := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(r.text("pattern"), `\`, `\\`), "%", `\%`), "*", "%")
	if pattern == "" {
		pattern = "%"
	}
	rows, err := r.s.pool.Query(ctx, `SELECT a.id, a.filename, a.updated_at, v.size FROM attachments a
		JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version
		WHERE a.page_id = $1 AND a.status = 'active' AND a.filename ILIKE $2
		  AND (cardinality($5::text[]) = 0 OR EXISTS (SELECT 1 FROM attachment_labels l WHERE l.attachment_id = a.id AND l.label = ANY($5)))
		ORDER BY CASE WHEN $3 = 'date' THEN a.updated_at END DESC, CASE WHEN $3 = 'size' THEN v.size END DESC, lower(a.filename)
		LIMIT $4`, id, pattern, r.text("sort"), r.max(defaultRows), lower(r.list("labels")))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		var size int64
		err := row.Scan(&v.ID, &v.Title, &at, &size)
		v.At, v.Count = &at, int(size)
		return err
	})
	return Answer{Rows: out}, err
}
