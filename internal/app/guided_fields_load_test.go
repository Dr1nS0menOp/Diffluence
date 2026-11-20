// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// TestTheLoadBudgetOfTheDryRun measures TPL-904 on generated data: the dry run of the check
// step within 150 ms for 10 pages and 1 s for 100, at p95. Every page asks questions of
// several kinds, so the answers are checked as a real form's are, including a person and a
// page the server looks up. Run it outside the race detector, with KENWERK_LOAD_BUDGET set.
func TestTheLoadBudgetOfTheDryRun(t *testing.T) {
	if os.Getenv("KENWERK_LOAD_BUDGET") == "" {
		t.Skip("KENWERK_LOAD_BUDGET is not set")
	}
	e := newEnv(t)
	owner, editor, _, space := e.team("LOAD")
	handbook := owner.page(space, nil, "Handbook", "x")
	specs := []map[string]any{
		{"name": "Topic", "kind": "line", "required": true, "max_length": 80},
		{"name": "Hours", "kind": "number", "min": 1, "max": 40, "decimals": 1},
		{"name": "Start", "kind": "date", "earliest": map[string]any{"offset": -30}},
		{"name": "Tools", "kind": "choices", "options": []string{"Laptop", "Phone", "Badge"}},
		{"name": "Buddy", "kind": "person"},
		{"name": "Guide", "kind": "page"},
	}
	template := func(name string, pages int) string {
		tpl := owner.formTemplate(space, name, specs...)
		owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": name, "title_pattern": "{Topic}", "labels": []string{"set"},
			"document": blocksDoc(askAlone("Topic"), askAlone("Hours"), askAlone("Start"), askAlone("Tools"), askAlone("Buddy"), askAlone("Guide"))})
		for i := range pages - 1 {
			owner.entry(tpl, map[string]any{"name": "Page " + strconv.Itoa(i), "parent": "root", "labels": []string{"set"},
				"title_pattern": "{Topic} " + strconv.Itoa(i),
				"document":      blocksDoc(askAlone("Topic"), askAlone("Buddy"), map[string]any{"type": "paragraph", "content": []any{valueOf("set")}})})
		}
		return tpl
	}
	body := func(i int) map[string]any {
		return map[string]any{"space_id": space, "answers": map[string]any{"Topic": "run " + strconv.Itoa(i), "Hours": "7,5",
			"Start": time.Now().Format(time.DateOnly), "Tools": "Laptop\nBadge", "Buddy": editor.id, "Guide": handbook}}
	}
	measure := func(pages int, budget time.Duration) {
		t.Helper()
		tpl := template("Dry "+strconv.Itoa(pages), pages)
		path := "/api/v1/templates/" + tpl + "/sets/check"
		medianUse(owner, 3, path, body, 200) // warm up
		took := make([]time.Duration, 0, 40)
		for i := range 40 {
			started := time.Now()
			if status, out := owner.do("POST", path, body(i)); status != 200 {
				t.Fatalf("dry run of %d: %d %s", pages, status, out)
			}
			took = append(took, time.Since(started))
		}
		got := p95(took)
		t.Logf("the dry run of %d pages: p95 %v, median %v", pages, got.Round(time.Millisecond), took[len(took)/2].Round(time.Millisecond))
		if got > budget {
			t.Errorf("the dry run of %d pages takes %v at p95, budget %v", pages, got, budget)
		}
	}
	measure(10, 150*time.Millisecond)
	measure(100, time.Second)
}
