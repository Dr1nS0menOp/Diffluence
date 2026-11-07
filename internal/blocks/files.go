// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// spaceFiles lists the files of the space this block stands in. A file is as visible as
// its page, so a restricted page hides its files here too.
func spaceFiles(ctx context.Context, r reader) (Answer, error) {
	if r.spaceID == uuid.Nil {
		return Answer{}, nil
	}
	ending := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(r.text("file_type"))), ".")
	rows, err := r.s.pool.Query(ctx, `SELECT a.id, a.filename, a.updated_at, f.size, p.title FROM attachments a
		JOIN attachment_versions f ON f.attachment_id = a.id AND f.number = a.current_version
		JOIN pages p ON p.id = a.page_id
		JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE a.space_id = $2 AND a.status = 'active' AND p.status = 'published'
		  AND ($3 = '' OR lower(a.filename) LIKE '%.' || $3)
		  AND (cardinality($4::text[]) = 0 OR EXISTS (SELECT 1 FROM attachment_labels l WHERE l.attachment_id = a.id AND l.label = ANY($4)))
		ORDER BY a.updated_at DESC, a.id LIMIT $5`, r.userID, r.spaceID, ending, lower(r.list("labels")), r.max(defaultRows))
	out, err := scanRows(rows, err, func(row pgx.CollectableRow, v *Row) error {
		var at time.Time
		var size int64
		err := row.Scan(&v.ID, &v.Title, &at, &size, &v.Detail)
		v.At, v.Count = &at, int(size)
		return err
	})
	return Answer{Rows: out}, err
}
