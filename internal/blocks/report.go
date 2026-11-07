// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
)

var (
	reportColumnNames    = []string{"creator", "changed", "labels", "status", "comments", "reactions"}
	defaultReportColumns = []string{"creator", "changed", "labels"}
)

// reportColumns keeps what a writer chose that exists, in their order.
func reportColumns(chosen []string) []string {
	out := []string{}
	for _, name := range chosen {
		if slices.Contains(reportColumnNames, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return defaultReportColumns
	}
	return out
}

// pageFacts is what a report knows about one page beyond the row.
type pageFacts struct {
	creator, labels, status string
	comments, reactions     int
}

// reportFacts reads the columns for rows that passed the check. A comment for editors
// counts for nobody here, because one table goes to every reader.
func (r reader) reportFacts(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]pageFacts, error) {
	found, err := r.s.pool.Query(ctx, `SELECT p.id, COALESCE(u.display_name, ''), p.stage_text,
			COALESCE((SELECT string_agg(l.label, ', ' ORDER BY l.label) FROM page_labels l WHERE l.page_id = p.id), ''),
			(SELECT count(*) FROM comments c WHERE c.page_id = p.id AND NOT c.editors_only AND NOT c.purged AND c.trash_item_id IS NULL),
			(SELECT count(*) FROM reactions x WHERE x.target_type = 'page' AND x.target_id = p.id)
		FROM pages p LEFT JOIN users u ON u.id = p.created_by WHERE p.id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer found.Close()
	out := map[uuid.UUID]pageFacts{}
	for found.Next() {
		var id uuid.UUID
		var f pageFacts
		if err := found.Scan(&id, &f.creator, &f.status, &f.labels, &f.comments, &f.reactions); err != nil {
			return nil, err
		}
		out[id] = f
	}
	return out, found.Err()
}

func reportCell(column string, row Row, f pageFacts) string {
	switch column {
	case "creator":
		return f.creator
	case "changed":
		if row.At != nil {
			return row.At.UTC().Format(time.RFC3339)
		}
		return ""
	case "labels":
		return f.labels
	case "status":
		return f.status
	case "comments":
		return strconv.Itoa(f.comments)
	}
	return strconv.Itoa(f.reactions)
}

// reportTable is the selection of a filter as a table. The keys go to the screen, which
// has the words for them.
func reportTable(ctx context.Context, r reader) (Answer, error) {
	rows, err := r.pages(ctx, r.fromSettings(defaultRows))
	if err != nil {
		return Answer{}, err
	}
	columns := reportColumns(r.list("columns"))
	facts, err := r.reportFacts(ctx, rowIDs(rows))
	if err != nil {
		return Answer{}, err
	}
	for i := range rows {
		rows[i].Cells = make([]string, len(columns))
		for j, column := range columns {
			rows[i].Cells[j] = reportCell(column, rows[i], facts[rows[i].ID])
		}
	}
	return Answer{Rows: rows, ColumnKeys: columns}, nil
}
