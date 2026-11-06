// SPDX-License-Identifier: Apache-2.0

// Package search is PostgreSQL full-text search (specs 003 and 011). Results only
// come through perm_visible_pages, the same rule lists use, so search cannot show
// more than the user may see.
package search

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/db"
)

const (
	// maxBodyBytes keeps the text below PostgreSQL's 1 MiB tsvector limit.
	maxBodyBytes = 400_000
	// totalLimit: an exact count over a large instance costs more than it tells.
	totalLimit = 1000
	// maxSpellingTitles bounds the words a spelling suggestion looks at.
	maxSpellingTitles = 5000
	maxSpellingEdits  = 2
	defaultLimit      = 25
	maxLimit          = 100
	// queryCanceled is PostgreSQL's code for a statement that ran out of time.
	queryCanceled = "57014"
)

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

// excerptRunes is how much of a text a list shows.
const excerptRunes = 240

// Excerpts gives the start of the text of pages, for a list. The caller has
// already checked that the person may see them.
func Excerpts(ctx context.Context, q db.Querier, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := q.Query(ctx, "SELECT page_id, left(body, $2) FROM search_documents WHERE page_id = ANY($1)", ids, excerptRunes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]string{}
	for rows.Next() {
		var id uuid.UUID
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, err
		}
		out[id] = text
	}
	return out, rows.Err()
}

// Query is a search request. Zero values mean "no filter".
type Query struct {
	Text         string
	SpaceID      uuid.UUID
	AuthorID     uuid.UUID
	Since        time.Time
	Until        time.Time
	Limit        int
	Offset       int
	Kinds        []string
	Labels       []string
	Contributors []uuid.UUID
	Stage        string
	Below        uuid.UUID
	TitlesOnly   bool
	ByDate       bool
	// IncludeArchived adds what lists leave out: archived pages and spaces (ORG-020).
	IncludeArchived bool
	// MayNamePeople lets author: in the search box match names. It follows "view
	// profiles", and otherwise the field is ignored.
	MayNamePeople bool
}

// Crumb is one step of the way to a result.
type Crumb struct {
	ID    uuid.UUID `json:"id"`
	Title string    `json:"title"`
}

// Result is one found page.
type Result struct {
	PageID      uuid.UUID `json:"page_id"`
	SpaceID     uuid.UUID `json:"space_id"`
	SpaceKey    string    `json:"space_key"`
	SpaceName   string    `json:"space_name"`
	Title       string    `json:"title"`
	Emoji       string    `json:"emoji"`
	Kind        string    `json:"kind"`
	StageText   string    `json:"stage_text"`
	StageColor  string    `json:"stage_color"`
	Archived    bool      `json:"archived"`
	Snippet     string    `json:"snippet"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   uuid.UUID `json:"updated_by"`
	Breadcrumbs []Crumb   `json:"breadcrumbs"`
	// CommentID is set when a comment was found, and the result links to it.
	CommentID *uuid.UUID `json:"comment_id,omitempty"`
}

// Results is a page of results, with how many there are in all.
type Results struct {
	Results    []Result `json:"results"`
	Total      int      `json:"total"`
	Capped     bool     `json:"capped"`
	Suggestion string   `json:"suggestion"`
}

// Service runs searches.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

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

// mentionedNames is the words of a page with the names of those it mentions. The names
// come from page_mentions at search time: the index holds no names, so it follows a
// rename and a person can be anonymised (spec 010, D6).
const mentionedNames = `SELECT m.page_id, to_tsvector('simple', string_agg(COALESCE(u.display_name, g.name, ''), ' ')) AS tsv
	FROM page_mentions m
	LEFT JOIN users u ON u.id = m.user_id
	LEFT JOIN groups g ON g.id = m.user_id
	GROUP BY m.page_id`

// searchFilters is shared by the results and their count. A kind of template can
// never get here: perm_visible_pages leaves it out.
const searchFilters = `
	JOIN pages p ON p.id = d.page_id AND p.status = 'published'
	JOIN perm_visible_pages($1) v ON v.page_id = p.id
	JOIN spaces s ON s.id = p.space_id
	WHERE ($3::uuid IS NULL OR p.space_id = $3)
	  AND ($4::uuid IS NULL OR p.updated_by = $4)
	  AND ($5::timestamptz IS NULL OR p.updated_at >= $5)
	  AND ($6::timestamptz IS NULL OR p.updated_at < $6)
	  AND ($7::boolean OR (s.status = 'active' AND p.archived_at IS NULL))
	  AND ($8::text[] IS NULL OR p.kind = ANY($8))
	  AND ($9::text[] IS NULL OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($9)))
	  AND ($10::uuid[] IS NULL OR p.created_by = ANY($10)
	       OR EXISTS (SELECT 1 FROM page_versions w WHERE w.page_id = p.id AND w.author_id = ANY($10)))
	  AND ($11 = '' OR p.stage_text = $11)
	  AND ($12::uuid IS NULL OR EXISTS (SELECT 1 FROM page_tree t WHERE t.ancestor_id = $12 AND t.descendant_id = p.id))
	  AND ($13::text[] IS NULL OR lower(s.key) = ANY($13))
	  AND ($14::text[] IS NULL OR EXISTS (SELECT 1 FROM unnest($14::text[]) a
	       JOIN users u ON position(lower(a) IN lower(u.display_name)) > 0 WHERE u.id = p.updated_by OR u.id = p.created_by))
	  AND ($15::text[] IS NULL OR NOT EXISTS (SELECT 1 FROM unnest($15::text[]) w WHERE NOT (d.title || ' ' || d.body) ~* w))`

// Two versions of the same search: one with words, one with filters alone.
const (
	withWords = `FROM to_tsquery('simple', $2) AS q(query)
	JOIN LATERAL (
		SELECT x.page_id FROM search_documents x WHERE x.tsv @@ q.query
		UNION
		SELECT n.page_id FROM (` + mentionedNames + `) n
		JOIN search_documents x ON x.page_id = n.page_id WHERE (x.tsv || n.tsv) @@ q.query
	) h ON true
	JOIN search_documents d ON d.page_id = h.page_id` + searchFilters
	withoutWords = `FROM (SELECT $2::text) AS q(query)
	JOIN search_documents d ON true` + searchFilters

	resultColumns = `SELECT p.id, p.space_id, s.key, s.name, p.title, p.emoji, p.kind, p.stage_text, p.stage_color,
		p.archived_at IS NOT NULL OR s.status <> 'active', `
	headline     = `ts_headline('simple', d.body, q.query, 'StartSel=[[[, StopSel=]]], MaxFragments=2, MaxWords=25, MinWords=8, FragmentDelimiter=" … "')`
	resultEnd    = `, p.updated_at, p.updated_by `
	orderByWords = ` ORDER BY CASE WHEN $16 THEN p.updated_at END DESC NULLS LAST, ts_rank(d.tsv, q.query) DESC, p.updated_at DESC, p.id LIMIT $17 OFFSET $18`
	orderByDate  = ` ORDER BY CASE WHEN $16 THEN 0 ELSE 0 END, p.updated_at DESC, p.id LIMIT $17 OFFSET $18`

	wordsSQL      = resultColumns + headline + resultEnd + withWords + orderByWords
	filtersSQL    = resultColumns + "left(d.body, 200)" + resultEnd + withoutWords + orderByDate
	wordsCount    = `SELECT count(*) FROM (SELECT 1 ` + withWords + ` LIMIT $16) c`
	filtersCount  = `SELECT count(*) FROM (SELECT 1 ` + withoutWords + ` LIMIT $16) c`
	statementTime = "SET LOCAL statement_timeout = 3000"
)

// KindComment asks for comments instead of pages (ORG-024).
const KindComment = "comment"

// A comment is found through perm_visible_comments and nothing else, so a thread for
// editors or a comment in the trash stays out (PRM-077). The words are read the way
// the index on comments reads them.
const (
	commentWords = `jsonb_path_query_array(c.body, '$.**.text')`
	commentsFrom = `FROM to_tsquery('simple', $2) AS q(query)
	JOIN comments c ON jsonb_to_tsvector('simple', ` + commentWords + `, '["string"]') @@ q.query
	JOIN perm_visible_comments($1) v ON v.comment_id = c.id
	JOIN pages p ON p.id = c.page_id
	JOIN spaces s ON s.id = p.space_id
	WHERE ($3::uuid IS NULL OR p.space_id = $3)
	  AND ($4::uuid IS NULL OR c.author_id = $4)
	  AND ($5::timestamptz IS NULL OR c.created_at >= $5)
	  AND ($6::timestamptz IS NULL OR c.created_at < $6)
	  AND ($7::boolean OR (s.status = 'active' AND p.archived_at IS NULL))`
	commentsSQL = `SELECT c.id, p.id, p.space_id, s.key, s.name, p.title, p.emoji, p.kind, p.stage_text, p.stage_color,
		p.archived_at IS NOT NULL OR s.status <> 'active',
		ts_headline('simple', (SELECT string_agg(w, ' ') FROM jsonb_array_elements_text(` + commentWords + `) AS w), q.query,
			'StartSel=[[[, StopSel=]]], MaxFragments=2, MaxWords=25, MinWords=8, FragmentDelimiter=" … "'),
		c.created_at, c.author_id ` + commentsFrom + ` ORDER BY c.created_at DESC, c.id LIMIT $8 OFFSET $9`
	commentsCount = `SELECT count(*) FROM (SELECT 1 ` + commentsFrom + ` LIMIT $8) n`
)

func nilIfEmpty[T any](list []T) []T {
	if len(list) == 0 {
		return nil
	}
	return list
}

func filterArgs(userID uuid.UUID, q Query, parsed Parsed) []any {
	labels := append(slices.Clone(q.Labels), parsed.Labels...)
	var authors []string
	if q.MayNamePeople {
		authors = parsed.Authors
	}
	return []any{userID, parsed.TSQuery, optID(q.SpaceID), optID(q.AuthorID), optTime(q.Since), optTime(q.Until), q.IncludeArchived,
		nilIfEmpty(q.Kinds), nilIfEmpty(labels), nilIfEmpty(q.Contributors), q.Stage, optID(q.Below),
		nilIfEmpty(parsed.Spaces), nilIfEmpty(authors), nilIfEmpty(parsed.Patterns)}
}

func hasFilters(q Query, parsed Parsed) bool {
	return len(q.Labels)+len(parsed.Labels)+len(parsed.Spaces)+len(q.Contributors) > 0 || q.Stage != "" || q.Below != uuid.Nil
}

func scanResults(rows pgx.Rows) ([]Result, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Result, error) {
		var v Result
		err := r.Scan(&v.PageID, &v.SpaceID, &v.SpaceKey, &v.SpaceName, &v.Title, &v.Emoji, &v.Kind, &v.StageText, &v.StageColor,
			&v.Archived, &v.Snippet, &v.UpdatedAt, &v.UpdatedBy)
		return v, err
	})
}

// run does the two queries. Wildcards have no index, so they get a time limit, and
// running out of it simply finds nothing: a search never gives an error (ORG-032).
func (s *Service) run(ctx context.Context, sqlText, countText string, args, page []any, scan func(pgx.Rows) ([]Result, error)) (Results, error) {
	out := Results{Results: []Result{}}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, statementTime); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sqlText, append(slices.Clone(args), page...)...)
		if err != nil {
			return err
		}
		if out.Results, err = scan(rows); err != nil {
			return err
		}
		total := 0
		if err := tx.QueryRow(ctx, countText, append(slices.Clone(args), totalLimit+1)...).Scan(&total); err != nil {
			return err
		}
		out.Total = min(total, totalLimit)
		out.Capped = total > totalLimit
		return nil
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == queryCanceled {
		return Results{Results: []Result{}}, nil
	}
	if out.Results == nil {
		out.Results = []Result{}
	}
	return out, err
}

func (s *Service) addBreadcrumbs(ctx context.Context, list []Result) error {
	ids := make([]uuid.UUID, len(list))
	for i, r := range list {
		ids[i] = r.PageID
	}
	rows, err := s.pool.Query(ctx, `SELECT t.descendant_id, p.id, p.title FROM page_tree t
		JOIN pages p ON p.id = t.ancestor_id
		WHERE t.descendant_id = ANY($1) AND t.depth > 0 ORDER BY t.descendant_id, t.depth DESC`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	crumbs := map[uuid.UUID][]Crumb{}
	for rows.Next() {
		var page uuid.UUID
		var c Crumb
		if err := rows.Scan(&page, &c.ID, &c.Title); err != nil {
			return err
		}
		crumbs[page] = append(crumbs[page], c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range list {
		list[i].Breadcrumbs = crumbs[list[i].PageID]
		if list[i].Breadcrumbs == nil {
			list[i].Breadcrumbs = []Crumb{}
		}
	}
	return nil
}

// Search returns the pages the user may view that match the query.
func (s *Service) Search(ctx context.Context, userID uuid.UUID, q Query) (Results, error) {
	parsed := Parse(q.Text, q.TitlesOnly)
	if q.Limit <= 0 || q.Limit > maxLimit {
		q.Limit = defaultLimit
	}
	if slices.Contains(q.Kinds, KindComment) {
		return s.searchComments(ctx, userID, q, parsed)
	}
	args := filterArgs(userID, q, parsed)
	sqlText, countText := wordsSQL, wordsCount
	if parsed.TSQuery == "" {
		if !hasFilters(q, parsed) {
			return Results{Results: []Result{}}, nil
		}
		sqlText, countText = filtersSQL, filtersCount
	}
	out, err := s.run(ctx, sqlText, countText, args, []any{q.ByDate, q.Limit, max(q.Offset, 0)}, scanResults)
	if err != nil {
		return out, err
	}
	if err := s.addBreadcrumbs(ctx, out.Results); err != nil {
		return out, err
	}
	if out.Total == 0 && parsed.TSQuery != "" {
		out.Suggestion, err = s.spelling(ctx, userID, q.Text)
	}
	return out, err
}

func scanComments(rows pgx.Rows) ([]Result, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Result, error) {
		var v Result
		var id uuid.UUID
		err := r.Scan(&id, &v.PageID, &v.SpaceID, &v.SpaceKey, &v.SpaceName, &v.Title, &v.Emoji, &v.Kind, &v.StageText, &v.StageColor,
			&v.Archived, &v.Snippet, &v.UpdatedAt, &v.UpdatedBy)
		v.CommentID = &id
		return v, err
	})
}

// searchComments needs words: a comment has no filters of its own to list it by.
func (s *Service) searchComments(ctx context.Context, userID uuid.UUID, q Query, parsed Parsed) (Results, error) {
	if parsed.TSQuery == "" {
		return Results{Results: []Result{}}, nil
	}
	args := []any{userID, parsed.TSQuery, optID(q.SpaceID), optID(q.AuthorID), optTime(q.Since), optTime(q.Until), q.IncludeArchived}
	out, err := s.run(ctx, commentsSQL, commentsCount, args, []any{q.Limit, max(q.Offset, 0)}, scanComments)
	if err != nil {
		return out, err
	}
	return out, s.addBreadcrumbs(ctx, out.Results)
}

// spelling looks for a similar word in titles the person may see, and nowhere else:
// a word from all text would reveal what is on hidden pages (spec 011, D20).
func (s *Service) spelling(ctx context.Context, userID uuid.UUID, text string) (string, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.title FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id
		WHERE p.status = 'published' ORDER BY p.updated_at DESC LIMIT $2`, userID, maxSpellingTitles)
	if err != nil {
		return "", err
	}
	titles, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	return suggest(text, titles), nil
}

// suggest replaces words that are close to a word in a title. It answers with
// nothing when no word changed.
func suggest(text string, titles []string) string {
	known := map[string]bool{}
	for _, title := range titles {
		for _, word := range words(title) {
			known[strings.ToLower(word)] = true
		}
	}
	changed := false
	typed := words(text)
	out := make([]string, len(typed))
	for i, word := range typed {
		lower := strings.ToLower(word)
		out[i] = lower
		if known[lower] {
			continue
		}
		if close := closestWord(lower, known); close != "" {
			out[i] = close
			changed = true
		}
	}
	if !changed {
		return ""
	}
	return strings.Join(out, " ")
}

func closestWord(word string, known map[string]bool) string {
	best, bestEdits := "", maxSpellingEdits+1
	for candidate := range known {
		edits := editDistance(word, candidate)
		if edits < bestEdits || (edits == bestEdits && candidate < best) {
			best, bestEdits = candidate, edits
		}
	}
	return best
}

// editDistance counts the changes between two words.
func editDistance(a, b string) int {
	x, y := []rune(a), []rune(b)
	previous := make([]int, len(y)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(x); i++ {
		current := make([]int, len(y)+1)
		current[0] = i
		for j := 1; j <= len(y); j++ {
			cost := 1
			if x[i-1] == y[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(y)]
}
