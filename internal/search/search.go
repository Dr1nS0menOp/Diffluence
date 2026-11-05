// SPDX-License-Identifier: Apache-2.0

// Package search is PostgreSQL full-text search (spec 003). Results only come
// through perm_visible_pages, the same rule lists use, so search cannot show
// more than the user may see.
package search

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

// maxBodyBytes keeps the text below PostgreSQL's 1 MiB tsvector limit.
const maxBodyBytes = 400_000

// HighlightStart and HighlightEnd surround matches in a snippet. The client
// turns them into highlighting; snippets are plain text, never HTML.
const (
	HighlightStart = "[[["
	HighlightEnd   = "]]]"
)

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// Index stores the searchable text of a page, in the caller's transaction.
func Index(ctx context.Context, q db.Querier, pageID uuid.UUID, title, body string) error {
	_, err := q.Exec(ctx, `INSERT INTO search_documents (page_id, title, body) VALUES ($1, $2, $3)
		ON CONFLICT (page_id) DO UPDATE SET title = EXCLUDED.title, body = EXCLUDED.body, updated_at = now()`,
		pageID, title, truncate(body, maxBodyBytes))
	return err
}

// SetAttachmentNames updates the attachment names of a page in the index.
func SetAttachmentNames(ctx context.Context, q db.Querier, pageID uuid.UUID, names []string) error {
	_, err := q.Exec(ctx, "UPDATE search_documents SET attachment_names = $2, updated_at = now() WHERE page_id = $1",
		pageID, truncate(strings.Join(names, " "), maxBodyBytes))
	return err
}

// Query is a search request. Zero values mean "no filter".
type Query struct {
	Text     string
	SpaceID  uuid.UUID
	AuthorID uuid.UUID
	Since    time.Time
	Until    time.Time
	Limit    int
	Offset   int
	// ActiveSpacesOnly leaves out archived spaces, as quick search does (SPC-069).
	ActiveSpacesOnly bool
}

// Result is one found page.
type Result struct {
	PageID    uuid.UUID `json:"page_id"`
	SpaceID   uuid.UUID `json:"space_id"`
	SpaceKey  string    `json:"space_key"`
	SpaceName string    `json:"space_name"`
	Title     string    `json:"title"`
	Snippet   string    `json:"snippet"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy uuid.UUID `json:"updated_by"`
}

// Service runs searches.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// tsquery turns user input into a safe query: every term is letters and digits
// only, so nothing in the input can act as query syntax. All terms must match,
// and the last one also matches as a prefix.
func tsquery(input string) string {
	fields := strings.FieldsFunc(input, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(fields) > 20 {
		fields = fields[:20]
	}
	if len(fields) == 0 {
		return ""
	}
	terms := make([]string, len(fields))
	for i, f := range fields {
		terms[i] = "'" + strings.ToLower(f) + "'"
	}
	terms[len(terms)-1] += ":*"
	return strings.Join(terms, " & ")
}

func optTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func optID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

const searchSQL = `SELECT p.id, p.space_id, s.key, s.name, p.title,
		ts_headline('simple', d.body, q.query,
			'StartSel=[[[, StopSel=]]], MaxFragments=2, MaxWords=25, MinWords=8, FragmentDelimiter=" … "'),
		p.updated_at, p.updated_by
	FROM to_tsquery('simple', $2) AS q(query)
	JOIN search_documents d ON d.tsv @@ q.query
	JOIN pages p ON p.id = d.page_id AND p.status = 'published'
	JOIN perm_visible_pages($1) v ON v.page_id = p.id
	JOIN spaces s ON s.id = p.space_id
	WHERE ($3::uuid IS NULL OR p.space_id = $3)
	  AND ($4::uuid IS NULL OR p.updated_by = $4)
	  AND ($5::timestamptz IS NULL OR p.updated_at >= $5)
	  AND ($6::timestamptz IS NULL OR p.updated_at < $6)
	  AND (NOT $9::boolean OR s.status = 'active')
	ORDER BY ts_rank(d.tsv, q.query) DESC, p.updated_at DESC, p.id
	LIMIT $7 OFFSET $8`

// Search returns the pages the user may view that match the query.
func (s *Service) Search(ctx context.Context, userID uuid.UUID, q Query) ([]Result, error) {
	text := tsquery(q.Text)
	if text == "" {
		return []Result{}, nil
	}
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 25
	}
	rows, err := s.pool.Query(ctx, searchSQL, userID, text, optID(q.SpaceID), optID(q.AuthorID),
		optTime(q.Since), optTime(q.Until), q.Limit, max(q.Offset, 0), q.ActiveSpacesOnly)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Result, error) {
		var v Result
		err := r.Scan(&v.PageID, &v.SpaceID, &v.SpaceKey, &v.SpaceName, &v.Title, &v.Snippet, &v.UpdatedAt, &v.UpdatedBy)
		return v, err
	})
	if out == nil {
		out = []Result{}
	}
	return out, err
}
