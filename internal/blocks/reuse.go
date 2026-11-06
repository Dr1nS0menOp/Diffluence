// SPDX-License-Identifier: Apache-2.0

package blocks

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/document"
)

// factsReport makes one table of the facts of the pages that pass the filter. A
// fact of a page the reader may not see never counts, because the filter comes first.
func factsReport(ctx context.Context, r reader) (Answer, error) {
	pages, err := r.pages(ctx, r.fromSettings(maxRows))
	if err != nil {
		return Answer{}, err
	}
	ids := make([]uuid.UUID, len(pages))
	for i, p := range pages {
		ids[i] = p.ID
	}
	rows, err := r.s.pool.Query(ctx, `SELECT page_id, key, value FROM page_facts
		WHERE page_id = ANY($1) AND facts_id = $2 ORDER BY position`, ids, r.text("facts_id"))
	if err != nil {
		return Answer{}, err
	}
	defer rows.Close()
	columns := r.list("columns")
	wanted := len(columns) > 0
	values := map[uuid.UUID]map[string]string{}
	for rows.Next() {
		var id uuid.UUID
		var key, value string
		if err := rows.Scan(&id, &key, &value); err != nil {
			return Answer{}, err
		}
		if !wanted && !slices.Contains(columns, key) {
			columns = append(columns, key)
		}
		if values[id] == nil {
			values[id] = map[string]string{}
		}
		if old := values[id][key]; old != "" {
			value = old + ", " + value // two rows with one key: both show, in one cell
		}
		values[id][key] = value
	}
	if err := rows.Err(); err != nil {
		return Answer{}, err
	}
	out := []Row{}
	for _, p := range pages {
		if values[p.ID] == nil {
			continue
		}
		p.Cells = make([]string, len(columns))
		for i, key := range columns {
			p.Cells[i] = values[p.ID][key]
		}
		out = append(out, p)
	}
	sortFacts(out, slices.Index(columns, r.text("sort_key")), r.on("descending"))
	return Answer{Rows: out, Columns: columns}, nil
}

// showInclude shows text from another page. The server walks includes, so no client
// can skip the check at a level, and a loop stops in one place (spec 012, D10).
func showInclude(ctx context.Context, r reader) (Answer, error) {
	include := document.Include{Page: r.target(), Name: r.text("name")}
	seen := map[uuid.UUID]bool{}
	if r.pageID != uuid.Nil {
		seen[r.pageID] = true
	}
	blocks, source, err := r.expand(ctx, include, seen, 1)
	if err != nil || source == nil {
		return Answer{}, err
	}
	raw, err := document.Marshal(document.Node{Type: document.TypeDoc, Content: blocks})
	return Answer{Document: raw, Source: source}, err
}

func messageBlock(text string) []document.Node {
	return []document.Node{{Type: document.TypeParagraph, Content: []document.Node{{Type: document.TypeText, Text: text}}}}
}

// expand returns the blocks of one include. No rights at a level means nothing at all
// from that level, not even a message.
func (r reader) expand(ctx context.Context, in document.Include, seen map[uuid.UUID]bool, depth int) ([]document.Node, *Row, error) {
	if seen[in.Page] {
		return messageBlock("This page shows itself here, so it stops."), &Row{ID: in.Page}, nil
	}
	if depth > maxDepth {
		return messageBlock("Text is shown at most five levels deep."), &Row{ID: in.Page}, nil
	}
	if ok, err := r.visible(ctx, in.Page); !ok {
		return nil, nil, err
	}
	var raw []byte
	source := &Row{ID: in.Page}
	err := r.s.pool.QueryRow(ctx, `SELECT v.document, p.title FROM pages p
		JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version WHERE p.id = $1`, in.Page).Scan(&raw, &source.Title)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	doc, err := document.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	blocks := doc.Content
	if in.Name != "" {
		found, ok := document.Excerpt(doc, in.Name)
		if !ok {
			return nil, nil, nil
		}
		blocks = found
	}
	inner := map[uuid.UUID]bool{in.Page: true}
	for id := range seen {
		inner[id] = true
	}
	out, err := r.expandAll(ctx, blocks, inner, depth)
	return out, source, err
}

func (r reader) expandAll(ctx context.Context, nodes []document.Node, seen map[uuid.UUID]bool, depth int) ([]document.Node, error) {
	out := make([]document.Node, 0, len(nodes))
	for _, n := range nodes {
		if include, ok := document.IncludeOf(n); ok {
			blocks, _, err := r.expand(ctx, include, seen, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, blocks...)
			continue
		}
		children, err := r.expandAll(ctx, n.Content, seen, depth)
		if err != nil {
			return nil, err
		}
		n.Content = children
		out = append(out, n)
	}
	return out, nil
}
