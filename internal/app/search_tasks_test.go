// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"strings"
	"testing"
	"time"
)

// TestTaskSearchFollowsThePageOfTheTask is COL-040 with NAV-005 and NAV-006: a task is found by
// the words of its text, and only where the reader may see the version of the page that holds
// it. A task on a restricted page, on a controlled page never approved, in a working version
// waiting for review, on an archived page or in the trash is neither listed nor counted
// (spec 086).
func TestTaskSearchFollowsThePageOfTheTask(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "TSK")
	due := time.Now().UTC().AddDate(0, 0, 7).Format(time.DateOnly)
	open := w.author.page(w.space, w.home, "Boat plan", "x")
	w.author.publish(open, "Boat plan", 1, taskDoc(map[string]any{"checked": false}, "Order the walrusword boards ", mention(w.reader.id, "Reader"), dateNode(due)))

	restricted := w.author.page(w.space, w.home, "Restricted plan", "x")
	w.author.publish(restricted, "Restricted plan", 1, taskDoc(map[string]any{"checked": false}, "walrusword behind a restriction"))
	w.admin.restrict(restricted, "view", w.admin.id, w.author.id)

	never := w.controlled("Never approved plan", "x")
	w.publishDoc(never, "Never approved plan", taskDoc(map[string]any{"checked": false}, "walrusword waiting for a first approval"))

	working := w.controlled("Working plan", "approved text")
	w.approve(working)
	w.publishDoc(working, "Working plan", taskDoc(map[string]any{"checked": false}, "walrusword only in the working version"))

	archived := w.author.page(w.space, w.home, "Archived plan", "x")
	w.author.publish(archived, "Archived plan", 1, taskDoc(map[string]any{"checked": true}, "walrusword on an archived page"))
	w.author.ok("POST", "/api/v1/pages/"+archived+"/archive", map[string]any{})

	trashed := w.author.page(w.space, w.home, "Trashed plan", "x")
	w.author.publish(trashed, "Trashed plan", 1, taskDoc(map[string]any{"checked": false}, "walrusword in the trash"))
	w.author.ok("DELETE", "/api/v1/pages/"+trashed, nil)

	for _, u := range []*user{w.reader, w.guest} {
		total, _, rows := searchAnswer(u, "kind=task&q=walrusword")
		if total != 1 || len(rows) != 1 || rows[0]["page_id"] != open {
			t.Fatalf("%s: a hidden task is found or counted: total %v, rows %v", u.email, total, rows)
		}
		hit := rows[0]
		task, _ := hit["task"].(map[string]any)
		if task == nil || task["done"] != false || task["due"] != due || task["assignee_name"] != "Test "+w.reader.email {
			t.Fatalf("a task result says what the task is: %v", hit)
		}
		if hit["title"] != "Boat plan" || hit["space_name"] == "" || !strings.Contains(hit["snippet"].(string), "[[[walrusword]]]") {
			t.Fatalf("a task result names its page and marks the words: %v", hit)
		}
		if total, _, _ := searchAnswer(u, "kind=task&q=walrusword&archived=1"); total != 2 {
			t.Fatalf("%s: a task on an archived page is found when asked for: %v", u.email, total)
		}
	}
	// Whoever may see the page finds its task; a working version's task waits for its approval.
	total, _, rows := searchAnswer(w.author, "kind=task&q=walrusword")
	if total != 3 || len(rows) != 3 {
		t.Fatalf("the author finds what they may see: total %v, rows %v", total, rows)
	}
	for _, row := range rows {
		if row["page_id"] == working || row["page_id"] == trashed || row["page_id"] == archived {
			t.Fatalf("the author finds a task they should not: %v", row)
		}
	}
	// Open tasks come first.
	if _, _, rows := searchAnswer(w.author, "kind=task&q=walrusword&archived=1"); rows[len(rows)-1]["page_id"] != archived {
		t.Fatalf("a done task comes after the open ones: %v", rows)
	}
}

// TestTaskSearchNeedsWords is COL-040: tasks have no filters of their own to list them by, so
// a search without words lists none, as for comments.
func TestTaskSearchNeedsWords(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("TSW")
	page := owner.page(space, nil, "Plan", "x")
	owner.publish(page, "Plan", 1, taskDoc(map[string]any{"checked": false}, "Paint the boat"))
	if total, _, rows := searchAnswer(owner, "kind=task&space_id="+space); total != 0 || len(rows) != 0 {
		t.Fatalf("a search without words lists tasks: %v %v", total, rows)
	}
	if total, _, _ := searchAnswer(owner, "kind=task&q=paint"); total != 1 {
		t.Fatalf("a task is found by its words: %v", total)
	}
}
