// SPDX-License-Identifier: Apache-2.0

package search

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// window is how many of the newest pages a search reads word by word before it goes to
// the index of words. For a common word that finds a page of results at once.
const window = "2000"

// mentionLimit bounds the pages found through a name, as a name can be on many pages.
const mentionLimit = 2000

// tier is a step in the order of the results. Within a tier, newest first. Nothing is left
// out, however old: it only stands further on.
type tier struct {
	match string
	// readsText says that the tier needs the words of the page, which cost far more to
	// read than the row in search_pages.
	readsText bool
	// byWords is the tier for the index of words: what is left to check once the index has
	// given its rows. Empty for a tier that the index of words does not serve.
	byWords string
	// order replaces newest first, for a tier that sorts by something else first.
	order string
}

const (
	matchesWords = `to_tsquery('simple', @words)`
	notAsTyped   = ` AND lower(w.title) <> lower(@text)`
)

var (
	// exactTier is the title as it was typed.
	exactTier = tier{match: `lower(w.title) = lower(@text)`}
	// titleTier is every word in the title. A title holds all the words of plain typing.
	titleTier = tier{match: `w.words @@ ` + matchesWords + notAsTyped}
	// languageTitleTier checks the rest of the page too: NOT looks at more than the title.
	languageTitleTier = tier{match: `w.words @@ ` + matchesWords + ` AND d.tsv @@ ` + matchesWords, readsText: true}
	textTier          = tier{match: `d.tsv @@ ` + matchesWords + ` AND NOT w.words @@ ` + matchesWords, readsText: true, byWords: `NOT w.words @@ ` + matchesWords}
	anyTier           = tier{match: `d.tsv @@ ` + matchesWords, readsText: true, byWords: `true`}
	// mentionTier stands apart: an OR with the words would keep the index of words from
	// being used, for every search that matches a name.
	mentionTier = tier{match: `w.page_id = ANY(@mentioned) AND NOT d.tsv @@ ` + matchesWords, readsText: true}
	// containsTier is a title that holds the typed text inside a word, such as "port" in
	// "Quarterly report", through the index of trigrams (spec 047, D13).
	containsTier = tier{match: `lower(w.title) LIKE @like AND NOT w.words @@ ` + matchesWords + notAsTyped}
	// nearTier is a title near the typed words: a letter dropped, doubled, swapped or wrong.
	// The closest come first, and within the same closeness the newest.
	nearTier = tier{match: `lower(w.title) %> lower(@text) AND lower(w.title) NOT LIKE @like AND NOT w.words @@ ` + matchesWords + notAsTyped,
		order: ` ORDER BY word_similarity(lower(@text), lower(w.title)) DESC, w.updated_at DESC, w.page_id DESC LIMIT @need`}
)

// Typed text of fewer letters has too few trigrams to mean anything, and longer text is
// no name (spec 047, D15). A typo needs more: below five letters, a word that only starts
// with the same two letters already counts as near ("week" and "Welcome").
const (
	minNearRunes = 3
	minTypoRunes = 5
	maxNearRunes = 64
)

// NearThreshold is how close a title must be to count as a typo of the typed words. It was
// settled on the pairs and decoys in testdata/typos.txt (spec 047, D16): at 0.4, 42 of 45
// typos are found and no decoy comes above 0.25; at 0.5 only 37 were found.
const NearThreshold = "0.4"

// wantsNear says whether the near tiers run: plain typing of a sensible length. The search
// syntax keeps its exact meaning (D18).
func wantsNear(parsed Parsed, text string) bool {
	return parsed.Plain && NearLength(text)
}

// NearLength says whether typed text has a length worth looking for inside names.
func NearLength(text string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(text))
	return n >= minNearRunes && n <= maxNearRunes
}

// TypoLength says whether typed text is long enough to look for names it is a typo of.
func TypoLength(text string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(text))
	return n >= minTypoRunes && n <= maxNearRunes
}

// LikeOf is the typed text as a LIKE pattern that holds it anywhere. % and _ are taken
// as letters.
func LikeOf(text string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(strings.TrimSpace(text)))
	return "%" + escaped + "%"
}

// rowScope is what can be told from search_pages alone, so a walk through the newest
// pages counts only pages this search can show.
const rowScope = `w.space_id = ANY(@spaces) AND w.published AND w.kind <> 'template'
	AND (@archived::boolean OR NOT w.archived)
	AND (@since::timestamptz IS NULL OR w.updated_at >= @since)
	AND (@until::timestamptz IS NULL OR w.updated_at < @until)
	AND (@kinds::text[] IS NULL OR w.kind = ANY(@kinds))`

// notHidden is here for speed. The rule that counts is perm_visible_pages in pickedRows,
// which every result passes.
const notHidden = ` AND w.page_id NOT IN (SELECT unnest(@hidden::uuid[]))`

// pageFilters need the page itself. Most searches have none of them.
const pageFilters = `
	AND (@author::uuid IS NULL OR p.updated_by = @author)
	AND (@labels::text[] IS NULL OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY(@labels))
	     OR EXISTS (SELECT 1 FROM attachments fa JOIN attachment_labels fl ON fl.attachment_id = fa.id
	                WHERE fa.page_id = p.id AND fa.status = 'active' AND fl.label = ANY(@labels)))
	AND (@contributors::uuid[] IS NULL OR p.created_by = ANY(@contributors)
	     OR EXISTS (SELECT 1 FROM page_versions pv WHERE pv.page_id = p.id AND pv.author_id = ANY(@contributors)))
	AND (@stage = '' OR p.stage_text = @stage)
	AND (@below::uuid IS NULL OR EXISTS (SELECT 1 FROM page_tree t WHERE t.ancestor_id = @below AND t.descendant_id = p.id))
	AND (@spacekeys::text[] IS NULL OR w.space_id IN (SELECT ks.id FROM spaces ks WHERE lower(ks.key) = ANY(@spacekeys)))
	AND (@authors::text[] IS NULL OR EXISTS (SELECT 1 FROM unnest(@authors::text[]) a
	     JOIN users u ON position(lower(a) IN lower(u.display_name)) > 0 WHERE u.id = p.updated_by OR u.id = p.created_by))`

const wildcardFilter = `
	AND NOT EXISTS (SELECT 1 FROM unnest(@patterns::text[]) pt WHERE NOT (d.title || ' ' || d.body) ~* pt)`

const (
	newest = ` ORDER BY w.updated_at DESC, w.page_id DESC LIMIT @need`
	// everyRowOrder sorts on an expression on purpose. On the plain column the planner
	// may walk the dates and read the text of all pages, for a common word that only old
	// pages hold. Here it has to start from the index of words (spec 028, D3).
	everyRowOrder = ` ORDER BY w.updated_at + interval '0' DESC, w.page_id DESC LIMIT @need`
	recentRows    = `(SELECT * FROM search_pages w WHERE ` + rowScope + ` ORDER BY w.updated_at DESC, w.page_id DESC LIMIT ` + window + `) w`
)

// sqlOf puts together the statement of one step. Everything in it is a constant of this
// file: what was typed only ever travels as an argument.
type sqlOf struct {
	pageFilter bool
	wildcards  bool
}

func (s sqlOf) tail(t tier) string {
	where := t.match + notHidden
	if s.pageFilter {
		where += pageFilters
	}
	if s.wildcards {
		where += wildcardFilter
	}
	return where
}

func (s sqlOf) joins(t tier) string {
	out := ""
	if t.readsText || s.wildcards {
		out += ` JOIN search_documents d ON d.page_id = w.page_id`
	}
	if s.pageFilter {
		out += ` JOIN pages p ON p.id = w.page_id`
	}
	return out
}

func (s sqlOf) recent(t tier) string {
	return `SELECT w.page_id FROM ` + recentRows + s.joins(t) + ` WHERE ` + s.tail(t) + newest
}

// wordHits asks the index of words and nothing else. OFFSET 0 keeps it apart: joined with the
// rest, the planner may read the words of every page instead, which took 200 ms.
const wordHits = `(SELECT x.page_id FROM search_documents x WHERE x.tsv @@ ` + matchesWords + ` OFFSET 0) h
	JOIN search_pages w ON w.page_id = h.page_id`

// titles reads a title tier in one step. A title is small, so even a walk past many of them
// costs little, and the index of titles settles the rest. The planner may choose here.
func (s sqlOf) titles(t tier) string {
	order := newest
	if t.order != "" {
		order = t.order
	}
	return `SELECT w.page_id FROM search_pages w` + s.joins(t) + ` WHERE ` + rowScope + ` AND ` + s.tail(t) + order
}

func (s sqlOf) every(t tier) string {
	if t.byWords == "" {
		return `SELECT w.page_id FROM search_pages w` + s.joins(t) + ` WHERE ` + rowScope + ` AND ` + s.tail(t) + everyRowOrder
	}
	rest := tier{match: t.byWords}
	return `SELECT w.page_id FROM ` + wordHits + s.joins(rest) + ` WHERE ` + rowScope + ` AND ` + s.tail(rest) + everyRowOrder
}

// windowIsFull tells whether the newest pages run past the window. If not, the window
// held every page of this search, and there is no need to look further.
const windowIsFull = `SELECT count(*) >= ` + window + ` FROM (SELECT 1 FROM search_pages w WHERE ` + rowScope + ` LIMIT ` + window + `) c`

// visibleSpaces and hiddenPages only narrow the search. The rule itself is applied to
// every result, in pickedRows.
const (
	visibleSpaces = `SELECT g.space_id FROM perm_spaces(@user, 'space.view') g JOIN spaces s ON s.id = g.space_id
	WHERE (@archived::boolean OR s.status = 'active') AND (@space::uuid IS NULL OR g.space_id = @space)`
	hiddenPages = `SELECT h.page_id FROM perm_hidden_pages(@user) h`
)

// mentionHits finds pages through the names they mention. The index holds no names, so a
// rename and an anonymised person are followed (spec 010, D6). It starts from people whose
// name holds a typed word, so a search for anything else costs next to nothing.
const mentionHits = `SELECT n.page_id FROM (
		SELECT m.page_id, to_tsvector('simple', string_agg(COALESCE(u.display_name, g.name, ''), ' ')) AS tsv
		FROM page_mentions m
		LEFT JOIN users u ON u.id = m.user_id
		LEFT JOIN groups g ON g.id = m.user_id
		WHERE m.page_id IN (SELECT f.page_id FROM page_mentions f WHERE f.user_id IN (
			SELECT x.id FROM users x WHERE to_tsvector('simple', x.display_name) @@ to_tsquery('simple', @names)
			UNION ALL
			SELECT y.id FROM groups y WHERE to_tsvector('simple', y.name) @@ to_tsquery('simple', @names)))
		GROUP BY m.page_id
	) n JOIN search_documents x ON x.page_id = n.page_id
	WHERE (x.tsv || n.tsv) @@ to_tsquery('simple', @words) LIMIT @mentionlimit`

// pickedRows reads what is shown. perm_visible_pages is the rule, for each row on its own
// and at this moment, so a permission that just changed counts at once. The rows are named
// inside the subquery, so the rule looks at those rows and not at every page there is.
const pickedRows = resultColumns + `CASE WHEN @snippets THEN ts_headline('simple', d.body, to_tsquery('simple', @words), ` + headlineOptions + `) ELSE '' END` + resultEnd + `
	FROM unnest(@ids::uuid[]) WITH ORDINALITY o(id, place)
	JOIN pages p ON p.id = o.id AND p.status = 'published'
	JOIN (SELECT v.page_id FROM perm_visible_pages(@user) v WHERE v.page_id = ANY(@ids)) v ON v.page_id = p.id
	JOIN spaces s ON s.id = p.space_id
	JOIN search_documents d ON d.page_id = p.id
	ORDER BY o.place`

// picking is one search for words on its way through the tiers.
type picking struct {
	tx   pgx.Tx
	args pgx.NamedArgs
	sql  sqlOf
	// page is the rows this request shows. want looks further, for the total.
	page int
	want int
	ids  []uuid.UUID
	// full is whether the window ran out before the pages did, once that is known.
	full *bool
	// stopped says that the page was filled before the total was known.
	stopped bool
}

func (p *picking) read(ctx context.Context, sql string, need int) ([]uuid.UUID, error) {
	p.args["need"] = need
	rows, err := p.tx.Query(ctx, sql, p.args)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (p *picking) windowFull(ctx context.Context) (bool, error) {
	if p.full != nil {
		return *p.full, nil
	}
	var full bool
	if err := p.tx.QueryRow(ctx, windowIsFull, p.args).Scan(&full); err != nil {
		return false, err
	}
	p.full = &full
	return full, nil
}

// add takes what one tier finds. First the newest pages, which settles a common word in
// milliseconds. A word that does not fill the page there is rare among them, so the index
// of words has few rows to give, whatever their age.
func (p *picking) add(ctx context.Context, t tier) error {
	need := p.want - len(p.ids)
	if need <= 0 || p.stopped {
		return nil
	}
	found, err := p.read(ctx, p.sql.recent(t), need)
	if err != nil {
		return err
	}
	if len(found) < need && len(p.ids)+len(found) >= p.page {
		// The window is the start of the whole list, so these rows are right as they are.
		// Going on would cost far more than a total is worth.
		p.ids = append(p.ids, found...)
		p.stopped = true
		return nil
	}
	if len(found) < need {
		full, err := p.windowFull(ctx)
		if err != nil {
			return err
		}
		if full {
			if found, err = p.read(ctx, p.sql.every(t), need); err != nil {
				return err
			}
		}
	}
	p.ids = append(p.ids, found...)
	return nil
}

// addAll takes a tier that is small by nature, so there is no need for the window.
func (p *picking) addAll(ctx context.Context, sql func(tier) string, t tier) error {
	need := p.want - len(p.ids)
	if need <= 0 || p.stopped {
		return nil
	}
	found, err := p.read(ctx, sql(t), need)
	if err != nil {
		return err
	}
	p.ids = append(p.ids, found...)
	return nil
}

func wordArgs(userID uuid.UUID, q Query, parsed Parsed) pgx.NamedArgs {
	return pgx.NamedArgs{
		"user": userID, "words": parsed.TSQuery, "names": parsed.AnyTerm, "text": strings.TrimSpace(q.Text),
		"space": optID(q.SpaceID), "author": optID(q.AuthorID), "since": optTime(q.Since), "until": optTime(q.Until),
		"archived": q.IncludeArchived, "kinds": nilIfEmpty(q.Kinds), "labels": nilIfEmpty(allLabels(q, parsed)),
		"contributors": nilIfEmpty(q.Contributors), "stage": q.Stage, "below": optID(q.Below),
		"spacekeys": nilIfEmpty(parsed.Spaces), "authors": nilIfEmpty(allowedAuthors(q, parsed)),
		"patterns": nilIfEmpty(parsed.Patterns), "mentionlimit": mentionLimit, "snippets": !q.Quick,
		"like": LikeOf(q.Text),
	}
}

func hasPageFilter(q Query, parsed Parsed) bool {
	return q.AuthorID != uuid.Nil || len(allLabels(q, parsed)) > 0 || len(q.Contributors) > 0 || q.Stage != "" ||
		q.Below != uuid.Nil || len(parsed.Spaces) > 0 || len(allowedAuthors(q, parsed)) > 0
}

func listOf(ctx context.Context, tx pgx.Tx, sql string, args pgx.NamedArgs) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, sql, args)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// prepare works out once what every step of the search uses.
func prepare(ctx context.Context, tx pgx.Tx, args pgx.NamedArgs) (bool, error) {
	if _, err := tx.Exec(ctx, wordSettings); err != nil {
		return false, err
	}
	spaces, err := listOf(ctx, tx, visibleSpaces, args)
	if err != nil || len(spaces) == 0 {
		return false, err
	}
	args["spaces"] = spaces
	if args["hidden"], err = listOf(ctx, tx, hiddenPages, args); err != nil {
		return false, err
	}
	if args["mentioned"], err = listOf(ctx, tx, mentionHits, args); err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, pickSettings)
	return err == nil, err
}

// pick walks the tiers. A search by date has one tier: all of it.
func (p *picking) pick(ctx context.Context, q Query, parsed Parsed) error {
	if q.ByDate {
		if err := p.add(ctx, anyTier); err != nil {
			return err
		}
		return p.addMentioned(ctx)
	}
	title := languageTitleTier
	if parsed.Plain {
		title = titleTier
		if err := p.addAll(ctx, p.sql.every, exactTier); err != nil {
			return err
		}
	}
	if err := p.addAll(ctx, p.sql.titles, title); err != nil {
		return err
	}
	near := wantsNear(parsed, q.Text)
	if near {
		if err := p.addAll(ctx, p.sql.titles, containsTier); err != nil {
			return err
		}
	}
	// In the box a typo in a title is what is looked for; in a full search a real word in
	// the text is the surer find (spec 047, D13).
	typos := near && TypoLength(q.Text)
	if typos && q.Quick {
		if err := p.addAll(ctx, p.sql.titles, nearTier); err != nil {
			return err
		}
	}
	if err := p.add(ctx, textTier); err != nil {
		return err
	}
	if typos && !q.Quick {
		if err := p.addAll(ctx, p.sql.titles, nearTier); err != nil {
			return err
		}
	}
	return p.addMentioned(ctx)
}

func (p *picking) addMentioned(ctx context.Context) error {
	if mentioned, ok := p.args["mentioned"].([]uuid.UUID); !ok || len(mentioned) == 0 {
		return nil
	}
	return p.addAll(ctx, p.sql.every, mentionTier)
}

// searchWords finds pages by their words (spec 028). Rows are picked by what is cheap to
// know, and only the rows of this page of results are read in full. The picking looks a
// little past the page, which is also how the total is known.
func (s *Service) searchWords(ctx context.Context, userID uuid.UUID, q Query, parsed Parsed) (Results, error) {
	out := Results{Results: []Result{}}
	offset := max(q.Offset, 0)
	args := wordArgs(userID, q, parsed)
	want := offset + q.Limit
	if !q.Quick {
		want = max(want+1, totalLimit+1)
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		ok, err := prepare(ctx, tx, args)
		if err != nil || !ok {
			return err
		}
		p := &picking{tx: tx, args: args, page: offset + q.Limit, want: want,
			sql: sqlOf{pageFilter: hasPageFilter(q, parsed), wildcards: len(parsed.Patterns) > 0}}
		if err := p.pick(ctx, q, parsed); err != nil {
			return err
		}
		out.Total = min(len(p.ids), totalLimit)
		out.Capped = len(p.ids) > totalLimit || p.stopped
		shown := p.ids[min(offset, len(p.ids)):min(offset+q.Limit, len(p.ids))]
		if len(shown) == 0 {
			return nil
		}
		args["ids"] = shown
		rows, err := tx.Query(ctx, pickedRows, args)
		if err != nil {
			return err
		}
		out.Results, err = scanResults(rows)
		return err
	})
	if timedOut(err) {
		return Results{Results: []Result{}, TimedOut: true}, nil
	}
	if out.Results == nil {
		out.Results = []Result{}
	}
	return out, err
}
