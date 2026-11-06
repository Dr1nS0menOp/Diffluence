// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func cell(text string) map[string]any {
	return map[string]any{"type": "tableCell", "content": []any{para2(text)}}
}

func para2(text string) map[string]any {
	return map[string]any{"type": "paragraph", "content": []any{map[string]any{"type": "text", "text": text}}}
}

func facts(pairs ...[2]string) map[string]any {
	rows := []any{}
	for _, p := range pairs {
		rows = append(rows, map[string]any{"type": "tableRow", "content": []any{cell(p[0]), cell(p[1])}})
	}
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "pageFacts",
		"content": []any{map[string]any{"type": "table", "content": rows}}}}}
}

func dynamicDoc(kind string, settings map[string]any) map[string]any {
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "dynamic",
		"attrs": map[string]any{"kind": kind, "settings": settings}}}}
}

func (u *user) block(kind, page string, settings map[string]any) map[string]any {
	u.e.t.Helper()
	return u.ok("POST", "/api/v1/blocks/"+kind, map[string]any{"page_id": page, "settings": settings})
}

// A block is a list like any other: it must walk past what a reader may not see.
func TestNoBlockShowsAHiddenPage(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, home := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, home, "Open page", "x")
	secret := owner.page(space, home, "Secret page", "x")
	owner.publish(open, "Open page", 1, facts([2]string{"Owner", "Anna"}))
	owner.publish(secret, "Secret page", 1, facts([2]string{"Owner", "Hidden name"}))
	owner.restrict(secret, "view", owner.id)

	for kind, settings := range map[string]map[string]any{
		"child_pages":    {},
		"page_tree":      {},
		"space_index":    {},
		"recent_changes": {"this_space": true},
		"filter_list":    {"this_space": true},
		"report_table":   {"this_space": true},
		"facts_report":   {"this_space": true},
		"link_cards":     {"pages": []any{open, secret}},
	} {
		got := reader.want(200, "POST", "/api/v1/blocks/"+kind, map[string]any{"page_id": home, "settings": settings})
		if strings.Contains(got, "Secret page") || strings.Contains(got, "Hidden name") {
			t.Errorf("%s shows a hidden page: %s", kind, got)
		}
		if !strings.Contains(got, "Open page") {
			t.Errorf("%s lost the open page: %s", kind, got)
		}
	}
	if rows := reader.block("child_pages", secret, nil)["rows"].([]any); len(rows) != 0 {
		t.Fatalf("a block on a hidden page answered: %v", rows)
	}
	if rows := reader.block("history_here", home, map[string]any{"page": secret})["rows"].([]any); len(rows) != 0 {
		t.Fatalf("a setting pointing at a hidden page answered: %v", rows)
	}
	if status, _ := reader.do("POST", "/api/v1/blocks/secret_kind", map[string]any{}); status != 400 {
		t.Fatalf("an unknown kind: %d", status)
	}
}

func TestFactsReportSortsByValue(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("ENG")
	for title, value := range map[string]string{"Nine": "9", "Ten": "10", "Word": "many"} {
		id := owner.page(space, nil, title, "x")
		owner.publish(id, title, 1, facts([2]string{"Size", value}))
	}
	got := owner.block("facts_report", home, map[string]any{"this_space": true, "sort_key": "Size"})
	rows := got["rows"].([]any)
	order := []string{}
	for _, r := range rows {
		order = append(order, r.(map[string]any)["title"].(string))
	}
	if strings.Join(order, ",") != "Nine,Ten,Word" {
		t.Fatalf("order: %v", order)
	}
	if cols := got["columns"].([]any); len(cols) != 1 || cols[0] != "Size" {
		t.Fatalf("columns: %v", cols)
	}
	// Removing the block removes its rows at the next publish.
	id := rows[0].(map[string]any)["id"].(string)
	owner.publish(id, "Nine", 2, para("no facts"))
	if rows := owner.block("facts_report", home, map[string]any{"this_space": true})["rows"].([]any); len(rows) != 2 {
		t.Fatalf("a removed fact stayed: %v", rows)
	}
}

func TestIncludesCheckEveryLevelAndStopOnALoop(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, home := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	a := owner.page(space, nil, "A", "x")
	b := owner.page(space, nil, "B", "words of b")
	secret := owner.page(space, nil, "Secret", "secret words")
	owner.publish(a, "A", 1, dynamicDoc(document.KindEmbeddedPage, map[string]any{"page": secret}))
	owner.restrict(secret, "view", owner.id)

	if got := reader.want(200, "POST", "/api/v1/blocks/embedded_page", map[string]any{"page_id": home, "settings": map[string]any{"page": a}}); strings.Contains(got, "secret words") {
		t.Fatalf("a hidden level showed: %s", got)
	}
	if got := owner.want(200, "POST", "/api/v1/blocks/embedded_page", map[string]any{"page_id": home, "settings": map[string]any{"page": a}}); !strings.Contains(got, "secret words") {
		t.Fatalf("the owner should see it: %s", got)
	}

	owner.publish(a, "A", 2, dynamicDoc(document.KindEmbeddedPage, map[string]any{"page": b}))
	owner.publish(b, "B", 1, dynamicDoc(document.KindEmbeddedPage, map[string]any{"page": a}))
	if got := owner.want(200, "POST", "/api/v1/blocks/embedded_page", map[string]any{"page_id": home, "settings": map[string]any{"page": a}}); !strings.Contains(got, "shows itself") {
		t.Fatalf("a loop must stop with a message: %s", got)
	}

	chain := owner.page(space, nil, "Deep 0", "bottom")
	for i := 1; i <= 7; i++ {
		next := owner.page(space, nil, "Deep", "x")
		owner.publish(next, "Deep", 1, dynamicDoc(document.KindEmbeddedPage, map[string]any{"page": chain}))
		chain = next
	}
	if got := owner.want(200, "POST", "/api/v1/blocks/embedded_page", map[string]any{"page_id": home, "settings": map[string]any{"page": chain}}); !strings.Contains(got, "five levels") || strings.Contains(got, "bottom") {
		t.Fatalf("the depth limit did not hold: %s", got)
	}

	used := owner.ok("GET", "/api/v1/pages/"+b+"/used-by", nil)
	if !has(titlesIn(used, "pages"), "A") {
		t.Fatalf("where used: %v", used)
	}
}

func TestExcerptShowsOnlyItsOwnBlocks(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("ENG")
	source := owner.page(space, nil, "Source", "x")
	owner.publish(source, "Source", 1, map[string]any{"type": "doc", "content": []any{
		para2("outside"),
		map[string]any{"type": "excerpt", "attrs": map[string]any{"name": "intro"}, "content": []any{para2("inside")}},
	}})
	got := owner.want(200, "POST", "/api/v1/blocks/show_excerpt", map[string]any{"page_id": home,
		"settings": map[string]any{"page": source, "name": "intro"}})
	if !strings.Contains(got, "inside") || strings.Contains(got, "outside") {
		t.Fatalf("excerpt: %s", got)
	}
}

func TestBadDynamicBlocksAreRefused(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "P", "x")
	body := func(doc map[string]any) map[string]any {
		return map[string]any{"title": "P", "document": doc, "base_version": 1, "overwrite": true}
	}
	owner.want(400, "POST", "/api/v1/pages/"+page+"/publish", body(dynamicDoc("secret_kind", nil)))
	owner.ok("POST", "/api/v1/pages/"+page+"/publish", body(dynamicDoc("child_pages", map[string]any{"max": 100000, "evil": "x", "page": "not an id"})))
	stored := owner.want(200, "GET", "/api/v1/pages/"+page, nil)
	if strings.Contains(stored, "evil") || strings.Contains(stored, "100000") || strings.Contains(stored, "not an id") {
		t.Fatalf("settings passed the gate: %s", stored)
	}
}
