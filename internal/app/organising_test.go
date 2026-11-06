// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const nextYear = "2099-01-01"

func strs(v any) []string {
	out := []string{}
	list, _ := v.([]any)
	for _, s := range list {
		if text, ok := s.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func treeTitles(u *user, space string) []string {
	u.e.t.Helper()
	return titlesIn(u.ok("GET", "/api/v1/spaces/"+space+"/tree", nil), "pages")
}

// team makes an owner, an editor and a viewer in one space.
func (e *env) team(key string) (owner, editor, viewer *user, space string) {
	e.t.Helper()
	owner = e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor = e.user("editor@example.com", identity.PermUseProduct)
	viewer = e.user("viewer@example.com", identity.PermUseProduct)
	space, _ = owner.space(key)
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	return owner, editor, viewer, space
}

func TestHandingOverAPageGivesNoRightToDeleteIt(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "Handed over", "x")
	if got := viewer.ok("GET", "/api/v1/pages/"+page, nil); got["owner_id"] != owner.id || got["owner_name"] != "Test owner@example.com" {
		t.Fatalf("owner: %v", got)
	}
	viewer.want(403, "PUT", "/api/v1/pages/"+page+"/owner", map[string]any{"user_id": viewer.id})
	owner.ok("PUT", "/api/v1/pages/"+page+"/owner", map[string]any{"user_id": editor.id})
	if viewer.ok("GET", "/api/v1/pages/"+page, nil)["owner_id"] != editor.id {
		t.Fatal("the owner did not change")
	}
	if e.auditActions("page.owner_changed") != 1 || e.outboxCount("page.owner_changed") != 1 {
		t.Fatal("no audit or outbox event")
	}
	// Delete own stays with the creator: a new owner gets no new right.
	editor.want(403, "DELETE", "/api/v1/pages/"+page, nil)

	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	owner.want(400, "PUT", "/api/v1/pages/"+page+"/owner", map[string]any{"user_id": stranger.id})
}

func TestStatusFollowsTheListOfTheSpace(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "Staged", "x")

	editor.ok("PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "Being written", "color": "blue"})
	got := viewer.ok("GET", "/api/v1/pages/"+page, nil)
	if got["stage_text"] != "Being written" || got["stage_color"] != "blue" {
		t.Fatalf("stage: %v", got)
	}
	if got["current_version"].(float64) != 1 {
		t.Fatal("a status must not make a version")
	}
	viewer.want(403, "PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "Being written", "color": "blue"})
	editor.want(400, "PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "My own word", "color": "blue"})
	editor.want(400, "PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "Being written", "color": "#ff0000"})

	owner.ok("PUT", "/api/v1/spaces/"+space+"/statuses", map[string]any{"free": true, "statuses": []map[string]any{{"text": "Ready", "color": "green"}}})
	editor.ok("PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "My own word", "color": "yellow"})
	editor.want(400, "PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": strings.Repeat("x", 31), "color": "yellow"})
	editor.ok("PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "", "color": ""})
	if viewer.ok("GET", "/api/v1/pages/"+page, nil)["stage_text"] != "" {
		t.Fatal("an empty status must clear it")
	}
	if e.auditActions("page.stage_changed") != 3 {
		t.Fatalf("audit: %d", e.auditActions("page.stage_changed"))
	}
	tooMany := make([]map[string]any, 7)
	for i := range tooMany {
		tooMany[i] = map[string]any{"text": "x", "color": "green"}
	}
	owner.want(400, "PUT", "/api/v1/spaces/"+space+"/statuses", map[string]any{"statuses": tooMany})
	editor.want(403, "PUT", "/api/v1/spaces/"+space+"/statuses", map[string]any{"statuses": []any{}})
}

func TestVerifiedBelongsToTheOwnerAndNotesALaterChange(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := editor.page(space, nil, "Checked", "x")
	other := e.user("other@example.com", identity.PermUseProduct)
	owner.grant(space, "user", other.id, permissions.TemplateEditor)

	other.want(403, "PUT", "/api/v1/pages/"+page+"/verification", map[string]any{"until": nextYear})
	editor.want(400, "PUT", "/api/v1/pages/"+page+"/verification", map[string]any{"until": "2001-01-01"})
	editor.want(400, "PUT", "/api/v1/pages/"+page+"/verification", map[string]any{"until": "soon"})
	editor.ok("PUT", "/api/v1/pages/"+page+"/verification", map[string]any{"until": nextYear})
	verified := viewer.ok("GET", "/api/v1/pages/"+page, nil)["verified"].(map[string]any)
	if verified["by_name"] != "Test editor@example.com" || verified["until"] != nextYear || verified["changed_after"] != false {
		t.Fatalf("verified: %v", verified)
	}
	editor.publish(page, "Checked", 1, para("new text"))
	verified = viewer.ok("GET", "/api/v1/pages/"+page, nil)["verified"].(map[string]any)
	if verified["changed_after"] != true {
		t.Fatalf("a later edit must show: %v", verified)
	}
	owner.ok("DELETE", "/api/v1/pages/"+page+"/verification", nil)
	if viewer.ok("GET", "/api/v1/pages/"+page, nil)["verified"] != nil {
		t.Fatal("the mark stayed")
	}
}

func TestReviewDatesRemindTheOwnerOnce(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "Reviewed", "x")
	viewer.want(403, "PUT", "/api/v1/pages/"+page+"/review", map[string]any{"on": "2099-01-01"})
	owner.want(400, "PUT", "/api/v1/pages/"+page+"/review", map[string]any{"on": "tomorrow"})
	owner.ok("PUT", "/api/v1/pages/"+page+"/review", map[string]any{"on": "2020-01-01"})
	if owner.ok("GET", "/api/v1/pages/"+page, nil)["review_on"] != "2020-01-01" {
		t.Fatal("no review date")
	}
	ctx := t.Context()
	if err := e.app.Pages().RemindReviews(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Pages().RemindReviews(ctx); err != nil {
		t.Fatal(err)
	}
	if got := e.outboxCount("page.review_due"); got != 1 {
		t.Fatalf("a reminder must go out once: %d", got)
	}

	owner.ok("PUT", "/api/v1/spaces/"+space+"/review", map[string]any{"months": 6})
	later := owner.page(space, nil, "With a default", "x")
	want := time.Now().AddDate(0, 6, 0).Format(time.DateOnly)
	if got := owner.ok("GET", "/api/v1/pages/"+later, nil)["review_on"]; got != want {
		t.Fatalf("default review date: %v, want %v", got, want)
	}
}

func TestLabelsAreCleanedAndNeverRevealHiddenPages(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	open := owner.page(space, nil, "Open", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	owner.restrict(hidden, "view", owner.id)

	add := func(u *user, page, label string) {
		t.Helper()
		u.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": label})
	}
	add(editor, open, "Team Plans")
	if got := strs(viewer.ok("GET", "/api/v1/pages/"+open, nil)["labels"]); len(got) != 1 || got[0] != "team-plans" {
		t.Fatalf("labels: %v", got)
	}
	if viewer.ok("GET", "/api/v1/pages/"+open, nil)["current_version"].(float64) != 1 {
		t.Fatal("a label must not make a version")
	}
	viewer.want(403, "POST", "/api/v1/pages/"+open+"/labels", map[string]any{"label": "x"})
	for _, bad := range []string{"", "   ", "with<tag>", strings.Repeat("x", 65), "a/b"} {
		editor.want(400, "POST", "/api/v1/pages/"+open+"/labels", map[string]any{"label": bad})
	}
	add(owner, hidden, "secret-label")
	add(owner, hidden, "team-plans")

	found := viewer.ok("GET", "/api/v1/labels", nil)["labels"].([]any)
	if len(found) != 1 || found[0].(map[string]any)["label"] != "team-plans" || found[0].(map[string]any)["count"].(float64) != 1 {
		t.Fatalf("a hidden page shows through: %v", found)
	}
	if got := titlesIn(viewer.ok("GET", "/api/v1/labels/team-plans/pages", nil), "pages"); len(got) != 1 || got[0] != "Open" {
		t.Fatalf("pages: %v", got)
	}
	if got := viewer.ok("GET", "/api/v1/labels/secret-label/pages", nil)["pages"].([]any); len(got) != 0 {
		t.Fatalf("hidden page through a label: %v", got)
	}
	add(owner, open, "second")
	if got := titlesIn(owner.ok("GET", "/api/v1/labels/team-plans+second/pages", nil), "pages"); len(got) != 1 || got[0] != "Open" {
		t.Fatalf("combined labels: %v", got)
	}

	editor.ok("DELETE", "/api/v1/pages/"+open+"/labels/second", nil)
	if e.auditActions("page.labelled") != 4 || e.auditActions("page.unlabelled") != 1 {
		t.Fatalf("audit: %d %d", e.auditActions("page.labelled"), e.auditActions("page.unlabelled"))
	}

	owner.ok("POST", "/api/v1/spaces/"+space+"/labels/rename", map[string]any{"from": "team-plans", "to": "plans"})
	if got := strs(viewer.ok("GET", "/api/v1/pages/"+open, nil)["labels"]); got[0] != "plans" {
		t.Fatalf("rename: %v", got)
	}
	add(owner, open, "merge-me")
	owner.ok("POST", "/api/v1/spaces/"+space+"/labels/rename", map[string]any{"from": "merge-me", "to": "plans"})
	if got := strs(viewer.ok("GET", "/api/v1/pages/"+open, nil)["labels"]); len(got) != 1 {
		t.Fatalf("merge: %v", got)
	}
	editor.want(403, "POST", "/api/v1/spaces/"+space+"/labels/rename", map[string]any{"from": "plans", "to": "x"})

	owner.ok("PUT", "/api/v1/spaces/"+space+"/labels", map[string]any{"fixed": true, "labels": []string{"allowed"}})
	editor.want(400, "POST", "/api/v1/pages/"+open+"/labels", map[string]any{"label": "not-on-the-list"})
	add(editor, open, "allowed")
}

func TestArchivedPagesAreReadOnlyAndLeaveEveryList(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	home := owner.ok("GET", "/api/v1/spaces/"+space, nil)["home_page_id"].(string)
	page := owner.page(space, nil, "Old plan", "findme")
	owner.want(409, "POST", "/api/v1/pages/"+home+"/archive", map[string]any{"include_children": true})
	viewer.want(403, "POST", "/api/v1/pages/"+page+"/archive", map[string]any{"include_children": true})
	viewer.ok("PUT", "/api/v1/pages/"+page+"/favourite", nil)
	viewer.ok("GET", "/api/v1/pages/"+page, nil)

	editor.ok("POST", "/api/v1/pages/"+page+"/archive", map[string]any{"include_children": true})
	if e.auditActions("page.archived") != 1 || e.outboxCount("page.archived") != 1 {
		t.Fatal("no audit or outbox event")
	}

	// It keeps its address, and says what it is.
	got := viewer.ok("GET", "/api/v1/pages/"+page, nil)
	if got["archived_at"] == nil {
		t.Fatalf("no mark: %v", got)
	}
	// It cannot be changed.
	editor.want(403, "POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "x", "document": para("y"), "base_version": 1})
	editor.want(403, "PATCH", "/api/v1/pages/"+page, map[string]any{"title": "x"})
	editor.want(403, "POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "x"})
	// And it left the lists.
	if has(treeTitles(viewer, space), "Old plan") {
		t.Fatal("still in the tree")
	}
	if has(titlesIn(viewer.ok("GET", "/api/v1/me/favourites", nil), "pages"), "Old plan") {
		t.Fatal("still in favourites")
	}
	if has(titlesIn(viewer.ok("GET", "/api/v1/me/recent", nil), "pages"), "Old plan") {
		t.Fatal("still in recent")
	}
	if len(viewer.ok("GET", "/api/v1/search?q=findme", nil)["results"].([]any)) != 0 {
		t.Fatal("search finds it")
	}
	if got := viewer.ok("GET", "/api/v1/search?q=findme&archived=1", nil)["results"].([]any); len(got) != 1 {
		t.Fatalf("advanced search must be able to: %v", got)
	}
	if got := titlesIn(viewer.ok("GET", "/api/v1/spaces/"+space+"/archive", nil), "pages"); len(got) != 1 {
		t.Fatalf("archive screen: %v", got)
	}

	viewer.want(403, "POST", "/api/v1/pages/"+page+"/unarchive", map[string]any{})
	editor.ok("POST", "/api/v1/pages/"+page+"/unarchive", map[string]any{})
	if !has(treeTitles(viewer, space), "Old plan") {
		t.Fatal("not back in the tree")
	}
}

func TestPagesLeftBehindMoveUpAndKeepTheirRestrictions(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	parent := owner.page(space, nil, "Parent", "x")
	kept := owner.page(space, parent, "Below", "x")
	under := owner.page(space, kept, "Further below", "x")

	// Without the pages below: they move up a level, and stay in the tree.
	editor.ok("POST", "/api/v1/pages/"+parent+"/archive", map[string]any{"include_children": false})
	if owner.ok("GET", "/api/v1/pages/"+kept, nil)["parent_id"] != nil {
		t.Fatal("the page below did not move up")
	}
	if !has(treeTitles(owner, space), "Further below") {
		t.Fatal("the branch below should stay")
	}
	owner.ok("POST", "/api/v1/pages/"+parent+"/unarchive", map[string]any{})

	// A move up that would lift a restriction needs manage restrictions.
	secret := owner.page(space, nil, "Secret parent", "x")
	inside := owner.page(space, secret, "Inside", "x")
	owner.restrict(secret, "view", owner.id, editor.id)
	editor.want(403, "POST", "/api/v1/pages/"+secret+"/archive", map[string]any{"include_children": false})
	if owner.ok("GET", "/api/v1/pages/"+inside, nil)["parent_id"] != secret {
		t.Fatal("a refused archive must change nothing")
	}
	owner.ok("POST", "/api/v1/pages/"+secret+"/archive", map[string]any{"include_children": false})
	_ = under
}

func TestARestoreBelowAnArchivedPageCannotDropARestriction(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	top := owner.page(space, nil, "Top", "x")
	child := owner.page(space, top, "Child", "x")
	owner.restrict(top, "view", owner.id)
	owner.ok("POST", "/api/v1/pages/"+child+"/archive", map[string]any{"include_children": true})
	owner.ok("POST", "/api/v1/pages/"+top+"/archive", map[string]any{"include_children": true})

	out := owner.want(409, "POST", "/api/v1/pages/"+child+"/unarchive", map[string]any{})
	if !strings.Contains(out, "parent_archived") {
		t.Fatalf("got %s", out)
	}
	viewer.want(404, "GET", "/api/v1/pages/"+child, nil)
	owner.ok("POST", "/api/v1/pages/"+child+"/unarchive", map[string]any{"with_parents": true})
	if !has(treeTitles(owner, space), "Top") || !has(treeTitles(owner, space), "Child") {
		t.Fatal("both must be back")
	}
	viewer.want(404, "GET", "/api/v1/pages/"+child, nil)
}

func TestDeletingAPageTakesArchivedPagesBelowItAlong(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	top := owner.page(space, nil, "Top", "x")
	child := owner.page(space, top, "Child", "x")
	owner.ok("POST", "/api/v1/pages/"+child+"/archive", map[string]any{"include_children": true})
	owner.ok("DELETE", "/api/v1/pages/"+top, nil)
	owner.want(404, "GET", "/api/v1/pages/"+child, nil)
}

func TestMoveToAnotherSpaceTakesTheBranchAlong(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, source := e.team("SRC")
	target, _ := owner.space("DST")
	owner.grant(target, "user", editor.id, permissions.TemplateEditor)
	top := editor.page(source, nil, "Moving", "x")
	child := editor.page(source, top, "Below", "x")
	_, body := editor.upload(child, "file.txt", "text")
	var file map[string]any
	_ = json.Unmarshal([]byte(body), &file)
	editor.ok("POST", "/api/v1/pages/"+top+"/labels", map[string]any{"label": "keeps"})

	impact := editor.ok("GET", "/api/v1/pages/"+top+"/move-impact?space_id="+target, nil)
	if impact["pages"].(float64) != 2 || impact["hidden"] != false {
		t.Fatalf("impact: %v", impact)
	}
	editor.ok("POST", "/api/v1/pages/"+top+"/move", map[string]any{"space_id": target, "parent_id": nil})
	if got := owner.ok("GET", "/api/v1/pages/"+child, nil)["space_id"]; got != target {
		t.Fatalf("the child stayed behind: %v", got)
	}
	if !has(treeTitles(owner, target), "Below") || has(treeTitles(owner, source), "Moving") {
		t.Fatal("the trees do not agree")
	}
	if got := strs(owner.ok("GET", "/api/v1/pages/"+top, nil)["labels"]); len(got) != 1 {
		t.Fatalf("labels stay: %v", got)
	}
	owner.want(200, "GET", "/api/v1/attachments/"+file["id"].(string)+"/content", nil)
	// Someone who can only see the old space loses it.
	viewer.want(404, "GET", "/api/v1/pages/"+top, nil)
	if e.auditActions("page.moved") != 1 {
		t.Fatal("no audit event")
	}
}

func TestMoveBetweenSpacesIsRefusedWhenItWouldTouchTooMuch(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, source := e.team("SRC")
	target, _ := owner.space("DST")
	nothing, _ := owner.space("NONE")
	owner.grant(target, "user", editor.id, permissions.TemplateEditor)
	home := owner.ok("GET", "/api/v1/spaces/"+source, nil)["home_page_id"].(string)

	owner.want(409, "POST", "/api/v1/pages/"+home+"/move", map[string]any{"space_id": target})

	branch := editor.page(source, nil, "Branch", "x")
	secret := owner.page(source, branch, "Secret", "x")
	owner.restrict(secret, "view", owner.id)
	if impact := editor.ok("GET", "/api/v1/pages/"+branch+"/move-impact?space_id="+target, nil); impact["hidden"] != true {
		t.Fatalf("impact: %v", impact)
	}
	out := editor.want(409, "POST", "/api/v1/pages/"+branch+"/move", map[string]any{"space_id": target})
	if !strings.Contains(out, "contains_hidden_pages") {
		t.Fatalf("got %s", out)
	}

	plain := editor.page(source, nil, "Plain", "x")
	editor.want(404, "POST", "/api/v1/pages/"+plain+"/move", map[string]any{"space_id": nothing})

	restricted := owner.page(source, nil, "Restricted parent", "x")
	owner.restrict(restricted, "view", owner.id, editor.id)
	inherited := editor.page(source, restricted, "Inherits", "x")
	if impact := editor.ok("GET", "/api/v1/pages/"+inherited+"/move-impact?space_id="+target, nil); !has(strs(impact["restrictions_lost"]), "Restricted parent") {
		t.Fatalf("impact: %v", impact)
	}
	editor.want(403, "POST", "/api/v1/pages/"+inherited+"/move", map[string]any{"space_id": target})

	owner.ok("POST", "/api/v1/spaces/"+target+"/archive", nil)
	owner.want(403, "POST", "/api/v1/pages/"+plain+"/move", map[string]any{"space_id": target})
}

func TestCopyToAnotherSpaceWithItsOwnTitles(t *testing.T) {
	e := newEnv(t)
	owner, _, _, source := e.team("SRC")
	target, _ := owner.space("DST")
	top := owner.page(source, nil, "Plan 2025", "x")
	owner.page(source, top, "Budget 2025", "x")
	owner.ok("POST", "/api/v1/pages/"+top+"/labels", map[string]any{"label": "keeps"})
	owner.restrict(top, "edit", owner.id)

	copied := owner.ok("POST", "/api/v1/pages/"+top+"/copy", map[string]any{
		"space_id": target, "include_children": true, "keep_labels": true, "keep_restrictions": false,
		"prefix": "New ", "find": "2025", "replace": "2026"})
	if copied["title"] != "New Plan 2026" || copied["space_id"] != target {
		t.Fatalf("copy: %v", copied)
	}
	if !has(treeTitles(owner, target), "New Budget 2026") {
		t.Fatalf("children: %v", treeTitles(owner, target))
	}
	if got := strs(owner.ok("GET", "/api/v1/pages/"+copied["id"].(string), nil)["labels"]); len(got) != 1 {
		t.Fatalf("labels: %v", got)
	}
	if owner.ok("GET", "/api/v1/pages/"+copied["id"].(string), nil)["restricted"] != false {
		t.Fatal("restrictions were asked to go")
	}
	plain := owner.ok("POST", "/api/v1/pages/"+top+"/copy", map[string]any{"parent_id": nil})
	if got := strs(owner.ok("GET", "/api/v1/pages/"+plain["id"].(string), nil)["labels"]); len(got) != 0 {
		t.Fatalf("labels are only copied on request: %v", got)
	}
	if plain["title"] != "Copy of Plan 2025" {
		t.Fatalf("title: %v", plain["title"])
	}
}

func TestSeveralPagesMoveTogetherAndEachIsChecked(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	parent := owner.page(space, nil, "New home", "x")
	first := editor.page(space, nil, "First", "x")
	second := editor.page(space, nil, "Second", "x")
	locked := owner.page(space, nil, "Locked", "x")
	owner.restrict(locked, "edit", owner.id)

	got := editor.ok("POST", "/api/v1/pages/bulk-move", map[string]any{
		"page_ids": []string{first, second, locked}, "parent_id": parent})
	if got["moved"].(float64) != 2 || len(got["failed"].([]any)) != 1 {
		t.Fatalf("bulk move: %v", got)
	}
	if editor.ok("GET", "/api/v1/pages/"+first, nil)["parent_id"] != parent {
		t.Fatal("first did not move")
	}
	if owner.ok("GET", "/api/v1/pages/"+locked, nil)["parent_id"] != nil {
		t.Fatal("a selection is not a permission")
	}
}

func TestSortingALevelIsAuditedAndNeedsEdit(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	parent := owner.page(space, nil, "Parent", "x")
	owner.page(space, parent, "Zebra", "x")
	owner.page(space, parent, "Apple", "x")
	viewer.want(403, "POST", "/api/v1/pages/"+parent+"/sort-children", map[string]any{"by": "title"})
	owner.want(400, "POST", "/api/v1/pages/"+parent+"/sort-children", map[string]any{"by": "secret"})
	owner.ok("POST", "/api/v1/pages/"+parent+"/sort-children", map[string]any{"by": "title"})
	got := titlesIn(owner.ok("GET", "/api/v1/pages/"+parent+"/details", nil), "children")
	if strings.Join(got, ",") != "Apple,Zebra" {
		t.Fatalf("order: %v", got)
	}
	if e.auditActions("page.children_sorted") != 1 {
		t.Fatal("no audit event")
	}
}
