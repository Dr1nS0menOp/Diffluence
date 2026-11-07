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

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
)

const (
	// maxBodyBytes keeps the text below PostgreSQL's 1 MiB tsvector limit.
	maxBodyBytes = 400_000
	// totalLimit: an exact count over a large instance costs more than it tells. Past it a
	// search says "more than", and goes on as far as a person keeps asking.
	totalLimit = 200
	// maxSpellingTitles bounds the words a spelling suggestion looks at.
	maxSpellingTitles = 5000
	maxSpellingEdits  = 2
	defaultLimit      = 25
	maxLimit          = 100
	// slotPatience is how long a search waits for its turn.
	slotPatience = time.Second
	// searchShare: one connection in this many can search at the same time.
	searchShare = 4
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

// RefreshNames puts the names of the files and the labels of a page in the index.
func RefreshNames(ctx context.Context, q db.Querier, pageID uuid.UUID) error {
	_, err := q.Exec(ctx, "UPDATE search_documents SET attachment_names = left(search_names($1), $2), updated_at = now() WHERE page_id = $1",
		pageID, maxBodyBytes)
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
	// Quick is the box while typing: titles only, so no snippets, count or spelling.
	Quick bool
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
	// TimedOut says that the search gave up. No results then means "not known", never
	// "nothing there".
	TimedOut bool `json:"timed_out"`
}

// Service runs searches.
type Service struct {
	pool *pgxpool.Pool
	// slots is the share of the connections for search. However many search at once, the
	// rest stays free for saving and reading (ARC-233). The share is small because the
	// limit is CPU: a search takes milliseconds, so few at a time still answer many.
	slots    chan struct{}
	patience time.Duration
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool) *Service {
	share := max(int(pool.Config().MaxConns)/searchShare, 1)
	return &Service{pool: pool, slots: make(chan struct{}, share), patience: slotPatience}
}

// enter waits a moment for a slot. A queue that grows without end helps nobody: past
// this the answer is that it is busy, at once, which the person can act on (ARC-234).
func (s *Service) enter(ctx context.Context) error {
	timer := time.NewTimer(s.patience)
	defer timer.Stop()
	select {
	case s.slots <- struct{}{}:
		return nil
	case <-timer.C:
		return apperr.Busy("Search is very busy right now. Try again in a moment.")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) leave() { <-s.slots }

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
	  AND ($9::text[] IS NULL OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($9))
	       OR EXISTS (SELECT 1 FROM attachments fa JOIN attachment_labels fl ON fl.attachment_id = fa.id
	                  WHERE fa.page_id = p.id AND fa.status = 'active' AND fl.label = ANY($9)))
	  AND ($10::uuid[] IS NULL OR p.created_by = ANY($10)
	       OR EXISTS (SELECT 1 FROM page_versions w WHERE w.page_id = p.id AND w.author_id = ANY($10)))
	  AND ($11 = '' OR p.stage_text = $11)
	  AND ($12::uuid IS NULL OR EXISTS (SELECT 1 FROM page_tree t WHERE t.ancestor_id = $12 AND t.descendant_id = p.id))
	  AND ($13::text[] IS NULL OR lower(s.key) = ANY($13))
	  AND ($14::text[] IS NULL OR EXISTS (SELECT 1 FROM unnest($14::text[]) a
	       JOIN users u ON position(lower(a) IN lower(u.display_name)) > 0 WHERE u.id = p.updated_by OR u.id = p.created_by))
	  AND ($15::text[] IS NULL OR NOT EXISTS (SELECT 1 FROM unnest($15::text[]) w WHERE NOT (d.title || ' ' || d.body) ~* w))`

// A search with filters alone has no words to pick by, and is read in one go.
const (
	withoutWords = `FROM (SELECT $2::text) AS q(query)
	JOIN search_documents d ON true` + searchFilters

	resultColumns = `SELECT p.id, p.space_id, s.key, s.name, p.title, p.emoji, p.kind, p.stage_text, p.stage_color,
		p.archived_at IS NOT NULL OR s.status <> 'active', `
	headlineOptions = `'StartSel=[[[, StopSel=]]], MaxFragments=2, MaxWords=25, MinWords=8, FragmentDelimiter=" … "'`
	resultEnd       = `, p.updated_at, p.updated_by `
	orderByDate     = ` ORDER BY CASE WHEN $16 THEN 0 ELSE 0 END, p.updated_at DESC, p.id LIMIT $17 OFFSET $18`

	filtersSQL    = resultColumns + "left(d.body, 200)" + resultEnd + withoutWords + orderByDate
	filtersCount  = `SELECT count(*) FROM (SELECT 1 ` + withoutWords + ` LIMIT $16) c`
	statementTime = "SET LOCAL statement_timeout = 3000"
	// wordSettings go in one round trip. A cached plan cannot know how common a word is,
	// and the right plan for a word in every page is the wrong one for a word in ten. A
	// search is short and many run at once, so workers only take CPU from everyone else.
	wordSettings = statementTime + `; SET LOCAL plan_cache_mode = force_custom_plan;
		SET LOCAL max_parallel_workers_per_gather = 0`
	// pickSettings: the planner does not count what it costs to unpack the words of a row.
	// For a word in a fifth of the pages it reads them all, 175 ms against 13 ms through the
	// index. Only for picking: on the small tables before it, a scan is the right plan.
	pickSettings = "SET LOCAL enable_seqscan = off"
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
	return []any{userID, parsed.TSQuery, optID(q.SpaceID), optID(q.AuthorID), optTime(q.Since), optTime(q.Until), q.IncludeArchived,
		nilIfEmpty(q.Kinds), nilIfEmpty(allLabels(q, parsed)), nilIfEmpty(q.Contributors), q.Stage, optID(q.Below),
		nilIfEmpty(parsed.Spaces), nilIfEmpty(allowedAuthors(q, parsed)), nilIfEmpty(parsed.Patterns)}
}

func allLabels(q Query, parsed Parsed) []string {
	return append(slices.Clone(q.Labels), parsed.Labels...)
}

func allowedAuthors(q Query, parsed Parsed) []string {
	if q.MayNamePeople {
		return parsed.Authors
	}
	return nil
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

// timedOut is PostgreSQL saying that a statement ran out of time.
func timedOut(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == queryCanceled
}

// run does the two queries of a search without a way to pick rows first. Running out of
// time gives no error, because a search never does (ORG-032), but it does not pass for
// an empty answer either: TimedOut says so.
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
	if timedOut(err) {
		return Results{Results: []Result{}, TimedOut: true}, nil
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
	if err := s.enter(ctx); err != nil {
		return Results{Results: []Result{}}, err
	}
	defer s.leave()
	parsed := Parse(q.Text, q.TitlesOnly)
	if q.Limit <= 0 || q.Limit > maxLimit {
		q.Limit = defaultLimit
	}
	if slices.Contains(q.Kinds, KindComment) {
		return s.searchComments(ctx, userID, q, parsed)
	}
	out, err := s.searchWithoutPicking(ctx, userID, q, parsed)
	if parsed.TSQuery != "" {
		out, err = s.searchWords(ctx, userID, q, parsed)
	}
	if err != nil {
		return out, err
	}
	if err := s.addBreadcrumbs(ctx, out.Results); err != nil {
		return out, err
	}
	if out.Total == 0 && !out.TimedOut && parsed.TSQuery != "" && !q.Quick {
		out.Suggestion, err = s.spelling(ctx, userID, q.Text)
	}
	return out, err
}

// searchWithoutPicking lists what filters alone find.
func (s *Service) searchWithoutPicking(ctx context.Context, userID uuid.UUID, q Query, parsed Parsed) (Results, error) {
	if parsed.TSQuery != "" || !hasFilters(q, parsed) {
		return Results{Results: []Result{}}, nil
	}
	return s.run(ctx, filtersSQL, filtersCount, filterArgs(userID, q, parsed), []any{q.ByDate, q.Limit, max(q.Offset, 0)}, scanResults)
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
	rows, err := s.pool.Query(ctx, `SELECT w.title FROM search_pages w
		WHERE w.published AND NOT w.archived AND w.kind <> 'template'
		  AND w.space_id IN (SELECT g.space_id FROM perm_spaces($1, 'space.view') g)
		  AND w.page_id NOT IN (SELECT h.page_id FROM perm_hidden_pages($1) h)
		ORDER BY w.updated_at DESC, w.page_id DESC LIMIT $2`, userID, maxSpellingTitles)
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
