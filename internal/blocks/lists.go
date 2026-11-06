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
	kind       string
	labels     []string
	excluded   []string
	title      string
	below      *uuid.UUID
	space      *uuid.UUID
	maxDepth   int
	sort       string
	descending bool
	max        int
}

func lower(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(strings.TrimSpace(v))
	}
	return out
}

// The sort key comes from a short list, and is a value here, so no SQL is built.
const filterSQL = `SELECT ` + pageColumns + `
	FROM pages p
	JOIN perm_visible_pages($1) v ON v.page_id = p.id
	JOIN spaces s ON s.id = p.space_id
	LEFT JOIN users u ON u.id = p.updated_by
	LEFT JOIN page_tree t ON t.descendant_id = p.id AND t.ancestor_id = $5
	WHERE p.status = 'published' AND p.archived_at IS NULL AND s.status = 'active' AND p.kind = $9
	  AND (cardinality($2::text[]) = 0 OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($2)))
	  AND NOT EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($3))
	  AND ($4 = '' OR position(lower($4) IN lower(p.title)) > 0)
	  AND ($5::uuid IS NULL OR (t.depth > 0 AND ($10 = 0 OR t.depth <= $10)))
	  AND ($6::uuid IS NULL OR p.space_id = $6)
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
	if f.kind == "" {
		f.kind = "page"
	}
	rows, err := r.s.pool.Query(ctx, filterSQL, r.userID, lower(f.labels), lower(f.excluded), f.title,
		f.below, f.space, f.sort, f.descending, f.kind, f.maxDepth, f.max)
	return scanRows(rows, err, pageRow)
}

// fromSettings reads the filter that many kinds share.
func (r reader) fromSettings(fallback int) filter {
	f := filter{labels: r.list("labels"), excluded: r.list("exclude_labels"), title: r.text("title"),
		space: r.space(), sort: r.text("sort"), descending: r.on("descending"), max: r.max(fallback)}
	if id, err := uuid.Parse(r.text("below")); err == nil {
		f.below = &id
	}
	if f.sort == "" {
		f.sort = "changed"
		f.descending = true
	}
	return f
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
	return Answer{Rows: rows}, err
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
	rows, err := r.pages(ctx, filter{space: r.space(), sort: "changed", descending: true, max: r.max(defaultRows)})
	return Answer{Rows: rows}, err
}

func filterList(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.pages(ctx, r.fromSettings(defaultRows))
	return Answer{Rows: rows}, err
}

func blogPosts(ctx context.Context, r reader) (Answer, error) {
	f := r.fromSettings(defaultRows)
	f.kind = "post"
	rows, err := r.pages(ctx, f)
	return Answer{Rows: rows}, err
}

// reportTable adds the labels of each page, read for those rows only.
func reportTable(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.pages(ctx, r.fromSettings(defaultRows))
	if err != nil {
		return Answer{}, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	labels, err := r.s.pool.Query(ctx, `SELECT page_id, string_agg(label, ', ' ORDER BY label)
		FROM page_labels WHERE page_id = ANY($1) GROUP BY page_id`, ids)
	if err != nil {
		return Answer{}, err
	}
	defer labels.Close()
	byPage := map[uuid.UUID]string{}
	for labels.Next() {
		var id uuid.UUID
		var text string
		if err := labels.Scan(&id, &text); err != nil {
			return Answer{}, err
		}
		byPage[id] = text
	}
	for i := range rows {
		rows[i].Detail = byPage[rows[i].ID]
	}
	return Answer{Rows: rows}, labels.Err()
}

func linkCards(ctx context.Context, r reader) (Answer, error) {
	ids := r.pagesSetting()
	rows, err := r.s.pool.Query(ctx, `SELECT `+pageColumns+` FROM pages p
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		JOIN spaces s ON s.id = p.space_id
		LEFT JOIN users u ON u.id = p.updated_by
		WHERE p.id = ANY($2) AND p.status = 'published' ORDER BY array_position($2, p.id)`, r.userID, ids)
	out, err := scanRows(rows, err, pageRow)
	return Answer{Rows: out}, err
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
		ORDER BY lower(s.name), s.id LIMIT 200`, r.userID, r.on("favourites"))
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
		ORDER BY k.due_on NULLS LAST, k.id LIMIT $6`,
		r.userID, r.space(), r.on("done"), r.on("mine"), lower(r.list("labels")), r.max(defaultRows))
	out, err := scanRows(rows, err, scanTask)
	return Answer{Rows: out}, err
}

func decisionReport(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT p.id, d.text, p.title, p.updated_at FROM page_decisions d
		JOIN pages p ON p.id = d.page_id
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.status = 'published' AND ($2::uuid IS NULL OR p.space_id = $2)
		  AND (cardinality($3::text[]) = 0 OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($3)))
		ORDER BY p.updated_at DESC, p.id, d.position LIMIT $4`, r.userID, r.space(), lower(r.list("labels")), r.max(defaultRows))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		err := row.Scan(&v.ID, &v.Title, &v.Detail, &at)
		v.At = &at
		return err
	})
	return Answer{Rows: out}, err
}

func contributors(ctx context.Context, r reader) (Answer, error) {
	id := r.target()
	if ok, err := r.visible(ctx, id); !ok {
		return Answer{}, err
	}
	rows, err := r.s.pool.Query(ctx, `SELECT v.author_id, COALESCE(u.display_name, ''), count(*), max(v.created_at)
		FROM page_versions v LEFT JOIN users u ON u.id = v.author_id
		WHERE v.page_id = $1 GROUP BY v.author_id, u.display_name ORDER BY count(*) DESC LIMIT $2`, id, r.max(defaultRows))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		err := row.Scan(&v.ID, &v.Title, &v.Count, &at)
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
		WHERE v.page_id = $1 ORDER BY v.number DESC LIMIT $2`, id, r.max(5))
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
		ORDER BY CASE WHEN $3 = 'date' THEN a.updated_at END DESC, CASE WHEN $3 = 'size' THEN v.size END DESC, lower(a.filename)
		LIMIT $4`, id, pattern, r.text("sort"), r.max(defaultRows))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		var size int64
		err := row.Scan(&v.ID, &v.Title, &at, &size)
		v.At, v.Count = &at, int(size)
		return err
	})
	return Answer{Rows: out}, err
}
