// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/blocks"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// plant is the word every planted thing of the hidden page holds. No answer to the
// reader, and after the purge no row, may hold it.
const plant = "quokka"

// hiddenWorld is a space with a page the reader may see, and a page beside it that only
// its owner may see, which has one of everything (PRM-171).
type hiddenWorld struct {
	e              *env
	owner, reader  *user
	space, home    string
	open, hidden   string
	ids            map[string]string
	blockKindsSeen int
}

func newHiddenWorld(t *testing.T) hiddenWorld {
	t.Helper()
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermViewProfiles)
	reader := e.user("reader@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	space, home := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, home, "Open plan", "Words of the open page")
	owner.publish(open, "Open plan", 1, map[string]any{"type": "doc", "content": []any{
		para2("Words of the open page"),
		map[string]any{"type": "excerpt", "attrs": map[string]any{"name": "intro"}, "content": []any{para2("shared intro")}},
	}})

	hidden := owner.page(space, home, "Hidden "+plant+"title", "x")
	owner.restrict(hidden, "view", owner.id)
	owner.publish(hidden, "Hidden "+plant+"title", 1, map[string]any{"type": "doc", "content": []any{
		para2(plant + "body"),
		map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "text", "text": plant + "link "}, pageLink(open),
			map[string]any{"type": "text", "text": " "}, mention(reader.id, "Reader")}},
		taskDoc(map[string]any{"checked": false}, plant+"task")["content"].([]any)[0],
		dynamicDoc("show_excerpt", map[string]any{"page": open, "name": "intro"})["content"].([]any)[0],
	}})
	owner.ok("POST", "/api/v1/pages/"+hidden+"/labels", map[string]any{"label": plant + "label"})
	owner.comment(hidden, plant+"comment")
	if status, out := owner.upload(hidden, plant+"file.txt", plant+"content"); status != 201 {
		t.Fatalf("upload: %d %s", status, out)
	}
	if status, out := owner.do("PUT", "/api/v1/pages/"+hidden+"/reactions/"+url.PathEscape("👍"), nil); status >= 300 {
		t.Fatalf("reaction: %d %s", status, out)
	}
	// Something the reader may see on the same day, so lists are not simply empty.
	owner.comment(open, "an open comment")
	owner.ok("POST", "/api/v1/pages/"+open+"/labels", map[string]any{"label": "openlabel"})

	ids := map[string]string{"space": space, "page": open, "owner": owner.id}
	return hiddenWorld{e: e, owner: owner, reader: reader, space: space, home: home, open: open, hidden: hidden, ids: ids}
}

// unwalked are the list routes the walk cannot fill in, each with the reason.
var unwalked = map[string]string{
	"GET /api/v1/admin/settings/{section}":    "administrators only, and holds settings, not content",
	"GET /api/v1/templates/{id}/files/{file}": "a file of a template, which follows the rule of its template (TestTemplateFilesFollowTheRuleOfTheTemplate)",
}

// walkAnswer is one route as the reader saw it.
type walkAnswer struct {
	status int
	body   string
}

// walk calls every list route as the reader: every GET of the API, and every dynamic block
// of the server on the open page. A route with a part it cannot fill must be in unwalked.
func (w *hiddenWorld) walk(t *testing.T) map[string]walkAnswer {
	t.Helper()
	out := map[string]walkAnswer{}
	for _, r := range w.e.app.Routes() {
		method, path, _ := strings.Cut(r.Pattern, " ")
		if method != "GET" || !strings.HasPrefix(path, "/api/v1/") {
			continue
		}
		if _, skip := unwalked[r.Pattern]; skip {
			continue
		}
		if strings.Contains(path, "/users/{id}") || strings.Contains(path, "/people/{id}") {
			path = strings.ReplaceAll(path, "{id}", w.owner.id)
		}
		_, path = objectRoute("GET "+path, w.ids)
		if strings.Contains(path, "{") {
			t.Fatalf("%s has a part the walk cannot fill: add it to unwalked with a reason", r.Pattern)
		}
		status, body := w.reader.do("GET", path, nil)
		out[r.Pattern] = walkAnswer{status, body}
	}
	w.blockKindsSeen = 0
	for _, kind := range blocks.Kinds() {
		status, body := w.reader.do("POST", "/api/v1/blocks/"+kind, map[string]any{"page_id": w.open,
			"settings": map[string]any{"page": w.open, "space": w.space}})
		out["POST /api/v1/blocks/"+kind] = walkAnswer{status, body}
		w.blockKindsSeen++
	}
	status, body := w.reader.do("POST", "/api/v1/pages/link-facts", map[string]any{"ids": []string{w.open, w.hidden}})
	out["POST /api/v1/pages/link-facts"] = walkAnswer{status, body}
	// Searches, with words that are on the hidden page and a misspelt one that a
	// suggestion could correct to it (PRM-147). What was asked is taken out of the answer
	// before it is read, because an answer may say what it was asked.
	for _, q := range []string{"Hidden", plant + "body", plant + "titel", plant + "label", plant + "task", plant + "comment"} {
		for _, path := range []string{"/api/v1/search?q=", "/api/v1/search/quick?q=", "/api/v1/labels?q=", "/api/v1/me/tasks?q="} {
			status, body := w.reader.do("GET", path+url.QueryEscape(q), nil)
			out["GET "+path+q] = walkAnswer{status, strings.ReplaceAll(body, q, "")}
		}
	}
	return out
}

// TestNoListShowsAHiddenPageOrWhatHangsOnIt is the list walk (PRM-171) and the count
// walk (PRM-172): no answer to the reader holds a planted word or the id of the hidden
// page, and every number is what it is when that page does not exist at all.
func TestNoListShowsAHiddenPageOrWhatHangsOnIt(t *testing.T) {
	w := newHiddenWorld(t)
	w.walk(t) // once to settle what reading itself records, such as recent pages
	before := w.walk(t)
	if len(before) < 150 || w.blockKindsSeen < 5 {
		t.Fatalf("the walk saw only %d routes and %d blocks", len(before), w.blockKindsSeen)
	}
	answered := 0
	for route, a := range before {
		if a.status == 200 {
			answered++
		}
		if strings.Contains(strings.ToLower(a.body), plant) || strings.Contains(a.body, w.hidden) {
			t.Errorf("%s shows the hidden page to the reader: %s", route, a.body)
		}
	}
	if answered < 60 {
		t.Fatalf("only %d routes answered the reader", answered)
	}

	w.purgeHidden(t)
	after := w.walk(t)
	compared := 0
	for route, a := range before {
		b := after[route]
		if a.status != b.status {
			t.Errorf("%s answers %d with the hidden page and %d without it", route, a.status, b.status)
			continue
		}
		x, y := numbersIn(a.body), numbersIn(b.body)
		if !slices.Equal(x, y) {
			t.Errorf("%s counts the hidden page: %v, without it %v", route, x, y)
		}
		compared += len(x)
	}
	if compared < 50 {
		t.Fatalf("only %d numbers compared", compared)
	}
	t.Logf("%d answers read, %d of them 200, %d numbers compared", len(before), answered, compared)
}

// numbersIn lists every number in a JSON answer with where it sits, sorted.
func numbersIn(body string) []string {
	var v any
	if json.Unmarshal([]byte(body), &v) != nil {
		return nil
	}
	var out []string
	var visit func(at string, v any)
	visit = func(at string, v any) {
		switch x := v.(type) {
		case float64:
			out = append(out, fmt.Sprintf("%s=%v", at, x))
		case map[string]any:
			for k, val := range x {
				visit(at+"."+k, val)
			}
		case []any:
			out = append(out, fmt.Sprintf("%s#=%d", at, len(x)))
			for _, val := range x {
				visit(at+"[]", val)
			}
		}
	}
	visit("", v)
	sort.Strings(out)
	return out
}

// purgeHidden deletes the hidden page and empties it from the trash, as its owner.
func (w *hiddenWorld) purgeHidden(t *testing.T) {
	t.Helper()
	w.owner.want(204, "DELETE", "/api/v1/pages/"+w.hidden, nil)
	for _, it := range trashList(w.owner, w.space, "") {
		if it["object_id"] == w.hidden {
			w.owner.want(204, "DELETE", "/api/v1/trash/"+it["id"].(string), nil)
			return
		}
	}
	t.Fatal("the hidden page is not in the trash")
}

// keptAfterPurge are the columns that keep a purged page by decision.
var keptAfterPurge = map[string]string{
	"audit_personal.data": "the personal half of the audit log, which keeps what happened (ADR-029) until anonymising",
}

// TestAPurgedPageLeavesNothingBehind is the purge walk (PRM-176): after a purge, no uuid
// column holds the id of the page, and no text or JSON column a planted word. A new
// table is walked the moment it exists. The audit log keeps what happened (ADR-029).
func TestAPurgedPageLeavesNothingBehind(t *testing.T) {
	w := newHiddenWorld(t)
	w.purgeHidden(t)
	ctx := context.Background()
	pool := w.e.app.Pool()
	rows, err := pool.Query(ctx, `SELECT table_name, column_name, udt_name FROM information_schema.columns
		WHERE table_schema = 'public' AND udt_name IN ('uuid', '_uuid', 'text', '_text', 'varchar', 'json', 'jsonb', 'tsvector')
		  AND table_name NOT IN ('audit_events', 'audit_pending', 'schema_migrations')
		  AND table_name IN (SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE')
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	type column struct{ table, name, typ string }
	columns, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (column, error) {
		var c column
		err := r.Scan(&c.table, &c.name, &c.typ)
		return c, err
	})
	if err != nil {
		t.Fatal(err)
	}
	walked := 0
	for _, c := range columns {
		needle := plant
		if c.typ == "uuid" || c.typ == "_uuid" {
			needle = w.hidden
		}
		query := fmt.Sprintf("SELECT count(*) FROM %s WHERE lower(%s::text) LIKE '%%' || $1 || '%%'",
			pgx.Identifier{c.table}.Sanitize(), pgx.Identifier{c.name}.Sanitize())
		var n int
		if err := pool.QueryRow(ctx, query, needle).Scan(&n); err != nil {
			t.Fatalf("%s.%s: %v", c.table, c.name, err)
		}
		walked++
		if _, decided := keptAfterPurge[c.table+"."+c.name]; n > 0 && !decided {
			t.Errorf("the purged page is still in %s.%s, in %d rows", c.table, c.name, n)
		}
	}
	if walked < 200 {
		t.Fatalf("only %d columns walked", walked)
	}
	t.Logf("%d columns walked", walked)
}

// TestNoPlaceChangeOpensAPage is restriction survival (PRM-174): every operation that
// changes the place of a page leaves it hidden from whoever could not see it before, unless
// the person acting may manage restrictions.
func TestNoPlaceChangeOpensAPage(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("editor@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	other, _ := owner.space("OUT")
	for _, s := range []string{space, other} {
		owner.grant(s, "user", editor.id, permissions.TemplateEditor)
		owner.grant(s, "user", outsider.id, permissions.TemplateViewer)
	}
	parent := owner.page(space, nil, "Closed", "x")
	owner.restrict(parent, "view", owner.id, editor.id)
	// The editor made the page, so may delete it, and may not manage restrictions.
	child := editor.page(space, parent, "Below", "x")
	hidden := func(why string) {
		t.Helper()
		outsider.want(404, "GET", "/api/v1/pages/"+child, nil)
		if got := outsider.want(200, "GET", "/api/v1/search?q=Below", nil); strings.Contains(got, child) {
			t.Fatalf("%s: the page shows in search: %s", why, got)
		}
	}
	hidden("at the start")

	// Failure paths first: what would drop the inherited restriction is refused.
	editor.want(403, "POST", "/api/v1/pages/"+child+"/move", map[string]any{"parent_id": nil})
	hidden("after a refused move to the top")
	editor.want(403, "POST", "/api/v1/pages/"+child+"/move", map[string]any{"space_id": other})
	hidden("after a refused move to another space")
	editor.want(403, "POST", "/api/v1/pages/"+child+"/copy", map[string]any{"parent_id": nil})
	hidden("after a refused copy")

	// What keeps the page where it is keeps it closed.
	editor.ok("POST", "/api/v1/pages/"+child+"/archive", map[string]any{"include_children": true})
	hidden("archived")
	editor.ok("POST", "/api/v1/pages/"+child+"/unarchive", map[string]any{})
	hidden("brought back from the archive")
	editor.want(204, "DELETE", "/api/v1/pages/"+child, nil)
	editor.ok("POST", "/api/v1/pages/"+child+"/undo-delete", nil)
	hidden("after delete and undo")
	editor.want(204, "DELETE", "/api/v1/pages/"+child, nil)
	for _, it := range trashList(owner, space, "") {
		if it["object_id"] == child {
			owner.ok("POST", "/api/v1/trash/"+it["id"].(string)+"/restore", nil)
		}
	}
	hidden("restored from the trash")
	copied := editor.ok("POST", "/api/v1/pages/"+child+"/copy", map[string]any{"parent_id": parent})["id"].(string)
	outsider.want(404, "GET", "/api/v1/pages/"+copied, nil)

	// The person who may manage restrictions can open it, and that is the exception.
	owner.ok("POST", "/api/v1/pages/"+child+"/move", map[string]any{"parent_id": nil})
	outsider.want(200, "GET", "/api/v1/pages/"+child, nil)
}
