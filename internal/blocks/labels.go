// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func scanLabel(row pgx.CollectableRow, v *Row) error {
	return row.Scan(&v.Title, &v.Count)
}

// A label counts through the pages a reader may see, so a label that only hidden pages
// carry does not exist for them.
const visibleLabels = `FROM page_labels l
	JOIN perm_visible_pages($1) v ON v.page_id = l.page_id
	JOIN pages p ON p.id = l.page_id
	WHERE p.status = 'published' AND p.archived_at IS NULL`

func popularLabels(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT l.label, count(*) `+visibleLabels+`
		AND ($2::uuid IS NULL OR p.space_id = $2)
		GROUP BY l.label ORDER BY count(*) DESC, l.label LIMIT $3`, r.userID, r.space(), r.max(defaultLabels))
	out, err := scanRows(rows, err, scanLabel)
	return Answer{Rows: out}, err
}

// relatedLabels lists labels on pages that share a label with this page, without the
// labels of the page itself.
func relatedLabels(ctx context.Context, r reader) (Answer, error) {
	id := r.target()
	if ok, err := r.visible(ctx, id); !ok {
		return Answer{}, err
	}
	rows, err := r.s.pool.Query(ctx, `SELECT l.label, count(*) `+visibleLabels+`
		AND l.page_id <> $2
		AND l.page_id IN (SELECT o.page_id FROM page_labels o WHERE o.label IN (SELECT m.label FROM page_labels m WHERE m.page_id = $2))
		AND l.label NOT IN (SELECT m.label FROM page_labels m WHERE m.page_id = $2)
		GROUP BY l.label ORDER BY count(*) DESC, l.label LIMIT $3`, r.userID, id, r.max(defaultLabels))
	out, err := scanRows(rows, err, scanLabel)
	return Answer{Rows: out}, err
}

func recentLabels(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.s.pool.Query(ctx, `SELECT l.label, count(*) `+visibleLabels+`
		AND ($2::uuid IS NULL OR p.space_id = $2)
		GROUP BY l.label ORDER BY max(l.created_at) DESC, l.label LIMIT $3`, r.userID, r.space(), r.max(defaultLabels))
	out, err := scanRows(rows, err, scanLabel)
	return Answer{Rows: out}, err
}
