// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func (u *user) saveDraft(page, title string, base int, text string) {
	u.e.t.Helper()
	u.ok("PUT", "/api/v1/pages/"+page+"/draft", map[string]any{"title": title, "document": para(text), "base_version": base})
}

func (e *env) makeDue(page string) {
	e.t.Helper()
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE scheduled_publications SET publish_at = now() - interval '1 minute' WHERE page_id = $1", page); err != nil {
		e.t.Fatal(err)
	}
}

func TestSilentUpdatesSayThatTheyAreSilent(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "Quiet", "x")
	owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Quiet", "document": para("y"), "base_version": 1, "silent": true})
	var silent bool
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT (payload->>'silent')::boolean FROM outbox_events
		WHERE type = 'page.published' ORDER BY created_at DESC LIMIT 1`).Scan(&silent); err != nil {
		t.Fatal(err)
	}
	if !silent {
		t.Fatal("the event must say it is silent, or watchers get a notification")
	}
	if len(owner.ok("GET", "/api/v1/pages/"+page+"/versions", nil)["versions"].([]any)) != 2 {
		t.Fatal("the history must still show the version")
	}
}

func TestPublishingCanSetAStatus(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "Staged", "x")
	owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Staged", "document": para("y"), "base_version": 1,
		"stage": map[string]any{"text": "Needs review", "color": "purple"}})
	if owner.ok("GET", "/api/v1/pages/"+page, nil)["stage_text"] != "Needs review" {
		t.Fatal("no status")
	}
}

func TestScheduledPublishingUsesTheLatestDraft(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := editor.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Later"})["id"].(string)
	editor.saveDraft(page, "Later", 0, "first words")

	editor.want(400, "PUT", "/api/v1/pages/"+page+"/schedule", map[string]any{"at": "2001-01-01T00:00:00Z"})
	viewer.want(404, "PUT", "/api/v1/pages/"+page+"/schedule", map[string]any{"at": "2099-01-01T00:00:00Z"})
	editor.ok("PUT", "/api/v1/pages/"+page+"/schedule", map[string]any{"at": "2099-01-01T00:00:00Z"})
	if editor.ok("GET", "/api/v1/pages/"+page, nil)["scheduled"] == nil {
		t.Fatal("no mark")
	}
	if err := e.app.Pages().PublishDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if editor.ok("GET", "/api/v1/pages/"+page, nil)["status"] != "draft" {
		t.Fatal("published too soon")
	}

	editor.saveDraft(page, "Later", 0, "later words")
	e.makeDue(page)
	if err := e.app.Pages().PublishDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	got := viewer.ok("GET", "/api/v1/pages/"+page, nil)
	if got["status"] != "published" || !strings.Contains(mustJSON(t, got["document"]), "later words") {
		t.Fatalf("the draft of that moment must go out: %v", got)
	}
	if got["scheduled"] != nil {
		t.Fatal("the mark stayed")
	}
	_ = owner
}

// hideFromAllBut writes a restriction nobody in the test could set themselves,
// because the API refuses a list that locks out the person who sets it.
func (e *env) hideFromAllBut(page string, u *user) {
	e.t.Helper()
	if _, err := e.app.Pool().Exec(context.Background(), `INSERT INTO page_restrictions (page_id, kind, subject_type, subject_id)
		VALUES ($1, 'view', 'user', $2)`, page, u.id); err != nil {
		e.t.Fatal(err)
	}
}

// runJob works off the queue and returns what a job reported.
func (e *env) runJob(u *user, id string) map[string]any {
	e.t.Helper()
	if _, err := e.app.Jobs().RunOnce(context.Background()); err != nil {
		e.t.Fatal(err)
	}
	got := u.ok("GET", "/api/v1/jobs/"+id, nil)
	if got["status"] != "done" {
		e.t.Fatalf("job: %v", got)
	}
	return got["result"].(map[string]any)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestScheduledPublishingStopsWhenTheRightsAreGone(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	page := owner.page(space, nil, "Shared", "x")
	editor.saveDraft(page, "Shared", 1, "not to be published")
	editor.ok("PUT", "/api/v1/pages/"+page+"/schedule", map[string]any{"at": "2099-01-01T00:00:00Z"})
	owner.grant(space, "user", editor.id, permissions.TemplateViewer)
	e.makeDue(page)
	if err := e.app.Pages().PublishDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if owner.ok("GET", "/api/v1/pages/"+page, nil)["current_version"].(float64) != 1 {
		t.Fatal("a scheduled publish must follow the rights of that moment")
	}
	if e.outboxCount("page.schedule_failed") != 1 {
		t.Fatal("the person must be told")
	}
}

func TestAPageWithoutATitleCannotBePublished(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "", "untitled": true})
	owner.want(400, "POST", "/api/v1/pages", map[string]any{"space_id": space, "title": ""})
	owner.want(400, "POST", "/api/v1/pages/"+page["id"].(string)+"/publish", map[string]any{"title": "", "document": para("x"), "base_version": 0})
}

func TestApprovalRunsFromAskingToApproved(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := editor.page(space, nil, "Policy", "agreed text")

	viewer.want(403, "POST", "/api/v1/pages/"+page+"/approvals", map[string]any{"user_ids": []string{owner.id}})
	editor.want(400, "POST", "/api/v1/pages/"+page+"/approvals", map[string]any{"user_ids": []string{}})
	round := editor.ok("POST", "/api/v1/pages/"+page+"/approvals", map[string]any{"user_ids": []string{owner.id, viewer.id}, "message": "Please check"})
	id := round["round_id"].(string)
	if len(round["cannot_see"].([]any)) != 0 {
		t.Fatalf("cannot see: %v", round)
	}
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	stranger.want(404, "PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "approve"})
	editor.want(403, "PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "approve"})

	viewer.ok("PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "changes", "comment": "Not yet"})
	mark := owner.ok("GET", "/api/v1/pages/"+page, nil)["approval"].(map[string]any)
	if mark["status"] != "open" || mark["must_answer"] != true {
		t.Fatalf("mark: %v", mark)
	}
	viewer.ok("PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "approve"})
	owner.ok("PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "approve"})
	mark = viewer.ok("GET", "/api/v1/pages/"+page, nil)["approval"].(map[string]any)
	if mark["status"] != "approved" || mark["is_current"] != true || mark["approved_version"].(float64) != 1 {
		t.Fatalf("approved: %v", mark)
	}
	owner.want(409, "PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "changes"})

	editor.publish(page, "Policy", 1, para("changed text"))
	mark = viewer.ok("GET", "/api/v1/pages/"+page, nil)["approval"].(map[string]any)
	if mark["is_current"] != false || mark["approved_version"].(float64) != 1 {
		t.Fatalf("a new version removes the mark: %v", mark)
	}
	rounds := viewer.ok("GET", "/api/v1/pages/"+page+"/approvals", nil)["rounds"].([]any)
	if len(rounds) != 1 || len(rounds[0].(map[string]any)["answers"].([]any)) != 2 {
		t.Fatalf("rounds stay on record: %v", rounds)
	}
}

func TestASpaceCanRequireApprovalOfTheExactText(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := editor.page(space, nil, "Rule", "x")
	owner.ok("PUT", "/api/v1/spaces/"+space+"/approvals", map[string]any{"required": true, "approvers": []string{owner.id}})

	editor.saveDraft(page, "Rule", 1, "new text")
	out := editor.want(409, "POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Rule", "document": para("new text"), "base_version": 1})
	if !strings.Contains(out, "approval_required") {
		t.Fatalf("got %s", out)
	}

	// Without the fixed approver the round does not count.
	id := editor.ok("POST", "/api/v1/pages/"+page+"/approvals", map[string]any{"user_ids": []string{viewer.id}})["round_id"].(string)
	viewer.ok("PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "approve"})
	editor.want(409, "POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Rule", "document": para("new text"), "base_version": 1})

	id = editor.ok("POST", "/api/v1/pages/"+page+"/approvals", map[string]any{"user_ids": []string{owner.id}})["round_id"].(string)
	owner.ok("PUT", "/api/v1/approvals/"+id+"/answer", map[string]any{"answer": "approve"})
	editor.want(409, "POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Rule", "document": para("other text"), "base_version": 1})
	editor.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Rule", "document": para("new text"), "base_version": 1})
	mark := viewer.ok("GET", "/api/v1/pages/"+page, nil)["approval"].(map[string]any)
	if mark["is_current"] != true {
		t.Fatalf("the approved text is now the current version: %v", mark)
	}
}

func TestANamedApproverCanReadAnUnpublishedPage(t *testing.T) {
	e := newEnv(t)
	_, editor, viewer, space := e.team("ENG")
	page := editor.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Unpublished"})["id"].(string)
	editor.saveDraft(page, "Unpublished", 0, "frozen words")
	id := editor.ok("POST", "/api/v1/pages/"+page+"/approvals", map[string]any{"user_ids": []string{viewer.id}})["round_id"].(string)
	viewer.want(404, "GET", "/api/v1/pages/"+page, nil)
	got := viewer.ok("GET", "/api/v1/approvals/"+id, nil)
	if !strings.Contains(mustJSON(t, got["document"]), "frozen words") {
		t.Fatalf("round: %v", got)
	}
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	stranger.want(404, "GET", "/api/v1/approvals/"+id, nil)
}

func TestContentScreenFiltersAndReportsWhatWorked(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	home := owner.ok("GET", "/api/v1/spaces/"+space, nil)["home_page_id"].(string)
	first := editor.page(space, nil, "First", "x")
	second := owner.page(space, nil, "Second", "x")
	secret := editor.page(space, nil, "Hidden from the owner", "x")
	e.hideFromAllBut(secret, editor)
	owner.ok("POST", "/api/v1/pages/"+first+"/labels", map[string]any{"label": "tidy"})

	editor.want(403, "GET", "/api/v1/spaces/"+space+"/content", nil)
	all := owner.ok("GET", "/api/v1/spaces/"+space+"/content", nil)
	if all["total"].(float64) != 3 || has(titlesIn(all, "pages"), "Hidden from the owner") {
		t.Fatalf("content: %v", all)
	}
	if got := titlesIn(owner.ok("GET", "/api/v1/spaces/"+space+"/content?label=tidy", nil), "pages"); len(got) != 1 || got[0] != "First" {
		t.Fatalf("label filter: %v", got)
	}
	if got := titlesIn(owner.ok("GET", "/api/v1/spaces/"+space+"/content?owner_id="+editor.id, nil), "pages"); len(got) != 1 {
		t.Fatalf("owner filter: %v", got)
	}

	action := map[string]any{"action": "label_add", "label": "bulk", "page_ids": []string{first, second, secret, home}}
	plan := owner.ok("POST", "/api/v1/spaces/"+space+"/content/preview", action)
	if plan["pages"].(float64) != 4 || plan["allowed"].(float64) != 3 || len(plan["refused"].([]any)) != 1 {
		t.Fatalf("plan: %v", plan)
	}
	job := owner.ok("POST", "/api/v1/spaces/"+space+"/content/run", action)["job_id"].(string)
	result := e.runJob(owner, job)
	if result["done"].(float64) != 3 || len(result["failed"].([]any)) != 1 {
		t.Fatalf("report: %v", result)
	}
	if has(strs(editor.ok("GET", "/api/v1/pages/"+secret, nil)["labels"]), "bulk") {
		t.Fatal("a selection is not a permission")
	}
}

func TestContentFiltersByTheDayOfChangeAndOfCreation(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	old := owner.page(space, nil, "Old", "x")
	recent := owner.page(space, nil, "Recent", "x")
	e.sql("UPDATE pages SET created_at = '2024-03-01T10:00:00Z', updated_at = '2024-05-10T23:30:00Z' WHERE id = $1", old)
	e.sql("UPDATE pages SET created_at = '2026-01-15T10:00:00Z', updated_at = '2026-02-01T08:00:00Z' WHERE id = $1", recent)
	content := "/api/v1/spaces/" + space + "/content?"

	owner.want(400, "GET", content+"changed_from=last+week", nil)
	if got := titlesIn(owner.ok("GET", content+"changed_until=2024-05-10", nil), "pages"); len(got) != 1 || got[0] != "Old" {
		t.Fatalf("the last day counts whole: %v", got)
	}
	if got := titlesIn(owner.ok("GET", content+"changed_from=2024-05-11", nil), "pages"); has(got, "Old") || !has(got, "Recent") {
		t.Fatalf("changed from: %v", got)
	}
	if got := titlesIn(owner.ok("GET", content+"created_from=2026-01-15&created_until=2026-01-15", nil), "pages"); len(got) != 1 || got[0] != "Recent" {
		t.Fatalf("created on one day: %v", got)
	}
}

func TestRemovingOneVersionLeavesAMark(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner, _, _, space := e.team("ENG")
	owner.grant(space, "user", admin.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Secret leaked", "a password")
	owner.publish(page, "Secret leaked", 1, para("clean text"))

	owner.want(403, "DELETE", "/api/v1/admin/pages/"+page+"/versions/1", map[string]any{"reason": "x"})
	admin.want(400, "DELETE", "/api/v1/admin/pages/"+page+"/versions/1", map[string]any{"reason": " "})
	admin.want(409, "DELETE", "/api/v1/admin/pages/"+page+"/versions/2", map[string]any{"reason": "x"})
	admin.ok("DELETE", "/api/v1/admin/pages/"+page+"/versions/1", map[string]any{"reason": "a leaked password"})

	versions := owner.ok("GET", "/api/v1/pages/"+page+"/versions", nil)["versions"].([]any)
	if len(versions) != 2 || versions[1].(map[string]any)["removed"] != true {
		t.Fatalf("versions: %v", versions)
	}
	if out := owner.want(409, "GET", "/api/v1/pages/"+page+"/compare?from=1&to=2", nil); !strings.Contains(out, "version_removed") {
		t.Fatalf("compare: %s", out)
	}
	if e.auditActions("page.version_removed") != 1 {
		t.Fatal("no audit event")
	}
}

func TestWhatChangedKnowsTheLastVersionAPersonSaw(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "News", "x")
	if viewer.ok("GET", "/api/v1/pages/"+page, nil)["seen_version"].(float64) != 0 {
		t.Fatal("a first visit shows nothing")
	}
	owner.publish(page, "News", 1, para("y"))
	if got := viewer.ok("GET", "/api/v1/pages/"+page, nil)["seen_version"].(float64); got != 1 {
		t.Fatalf("seen: %v", got)
	}
	if got := viewer.ok("GET", "/api/v1/pages/"+page, nil)["seen_version"].(float64); got != 2 {
		t.Fatalf("seen: %v", got)
	}
}

func TestSpaceKeysCanChangeAndOldOnesKeepWorking(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("OLD")
	owner.space("OTHER")
	editor.want(403, "PUT", "/api/v1/spaces/"+space+"/key", map[string]any{"key": "NEW"})
	owner.want(409, "PUT", "/api/v1/spaces/"+space+"/key", map[string]any{"key": "other"})
	owner.want(400, "PUT", "/api/v1/spaces/"+space+"/key", map[string]any{"key": "!"})
	owner.ok("PUT", "/api/v1/spaces/"+space+"/key", map[string]any{"key": "NEW"})
	if got := viewer.ok("GET", "/api/v1/space-keys/OLD", nil); got["key"] != "NEW" {
		t.Fatalf("old address: %v", got)
	}
	out := owner.want(409, "POST", "/api/v1/spaces", map[string]any{"key": "old", "name": "x"})
	if !strings.Contains(out, "key_taken") {
		t.Fatalf("an old key stays taken: %s", out)
	}
	owner.ok("DELETE", "/api/v1/spaces/"+space+"/old-keys/old", nil)
	owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "OLD", "name": "New owner of the key"})
	if e.auditActions("space.key_changed") != 1 {
		t.Fatal("no audit event")
	}
}

func TestFeaturesHideWithoutDeleting(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	post := editor.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "News", "kind": "post"})["id"].(string)
	editor.publish(post, "News", 0, para("x"))

	editor.want(403, "PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"blog": false})
	owner.want(400, "PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"nonsense": false})
	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"blog": false, "stages": false})
	if owner.ok("GET", "/api/v1/spaces/"+space, nil)["features"].(map[string]any)["blog"] != false {
		t.Fatal("still on")
	}
	out := editor.want(409, "POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "More", "kind": "post"})
	if !strings.Contains(out, "feature_off") {
		t.Fatalf("got %s", out)
	}
	page := owner.page(space, nil, "Plain", "x")
	owner.want(409, "PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "Being written", "color": "blue"})
	editor.ok("GET", "/api/v1/pages/"+post, nil)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"blog": true})
	if len(editor.ok("GET", "/api/v1/spaces/"+space+"/posts", nil)["posts"].([]any)) != 1 {
		t.Fatal("turning it on brings everything back")
	}

	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	admin.ok("PUT", "/api/v1/admin/features", map[string]any{"off": []string{"blog"}, "new_off": []string{"reactions"}})
	if owner.ok("GET", "/api/v1/spaces/"+space, nil)["features"].(map[string]any)["blog"] != false {
		t.Fatal("the instance decides first")
	}
	fresh, _ := owner.space("NEW")
	if owner.ok("GET", "/api/v1/spaces/"+fresh, nil)["features"].(map[string]any)["reactions"] != false {
		t.Fatal("a new space starts with the defaults")
	}
}

func TestSpaceOwnersHomeAndPicture(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "New home", "x")
	draft := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Draft"})["id"].(string)

	owner.ok("PUT", "/api/v1/spaces/"+space+"/owners", map[string]any{"user_ids": []string{viewer.id}})
	overview := viewer.ok("GET", "/api/v1/spaces/"+space+"/overview", nil)
	if got := overview["owners"].([]any); len(got) != 1 || got[0].(map[string]any)["name"] != "Test viewer@example.com" {
		t.Fatalf("owners: %v", got)
	}
	editor.want(403, "PUT", "/api/v1/spaces/"+space+"/home", map[string]any{"page_id": page})
	owner.want(400, "PUT", "/api/v1/spaces/"+space+"/home", map[string]any{"page_id": draft})
	owner.ok("PUT", "/api/v1/spaces/"+space+"/home", map[string]any{"page_id": page})
	if viewer.ok("GET", "/api/v1/spaces/"+space, nil)["home_page_id"] != page {
		t.Fatal("no new home")
	}

	put := func(u *user, content string) int {
		t.Helper()
		resp, _ := e.do(u.c, "POST", "/api/v1/spaces/"+space+"/icon", content, "Content-Type", "application/octet-stream")
		return resp.StatusCode
	}
	if got := put(owner, "<svg onload=alert(1)>"); got != 400 {
		t.Fatalf("a file that is not an image: %d", got)
	}
	if got := put(viewer, pngHeader); got != 403 {
		t.Fatalf("viewer: %d", got)
	}
	if got := put(owner, pngHeader); got != 204 {
		t.Fatalf("picture: %d", got)
	}
	resp, _ := e.do(viewer.c, "GET", "/api/v1/spaces/"+space+"/icon", "")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("serving: %d %v", resp.StatusCode, resp.Header)
	}
	if viewer.ok("GET", "/api/v1/spaces/"+space, nil)["has_icon"] != true {
		t.Fatal("no mark")
	}
}

func TestBlogPostsStayOutOfTheTree(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	before := len(treeTitles(owner, space))
	post := editor.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "First news", "kind": "post"})["id"].(string)
	editor.publish(post, "First news", 0, para("Hello readers"))
	editor.want(400, "POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "x", "kind": "post",
		"parent_id": owner.ok("GET", "/api/v1/spaces/"+space, nil)["home_page_id"]})
	if len(treeTitles(owner, space)) != before {
		t.Fatal("a post is in the tree")
	}
	list := viewer.ok("GET", "/api/v1/spaces/"+space+"/posts", nil)
	got := list["posts"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["title"] != "First news" || !strings.Contains(got[0].(map[string]any)["excerpt"].(string), "Hello readers") {
		t.Fatalf("posts: %v", list)
	}
	first := viewer.ok("GET", "/api/v1/pages/"+post, nil)["published_at"]
	time.Sleep(10 * time.Millisecond)
	editor.publish(post, "First news", 1, para("edited"))
	if viewer.ok("GET", "/api/v1/pages/"+post, nil)["published_at"] != first {
		t.Fatal("the first publish sets the date, and later versions keep it")
	}
	if len(viewer.ok("GET", "/api/v1/posts", nil)["posts"].([]any)) != 1 {
		t.Fatal("all posts")
	}
	secret := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Hidden post", "kind": "post"})["id"].(string)
	owner.publish(secret, "Hidden post", 0, para("x"))
	owner.restrict(secret, "view", owner.id)
	if has(titlesIn(viewer.ok("GET", "/api/v1/posts", nil), "posts"), "Hidden post") {
		t.Fatal("a hidden post is listed")
	}
}

func TestPageAndFolderTurnIntoEachOther(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	home := owner.ok("GET", "/api/v1/spaces/"+space, nil)["home_page_id"].(string)
	page := owner.page(space, nil, "Becomes a folder", "old words")
	owner.page(space, page, "Child", "x")
	owner.want(409, "POST", "/api/v1/pages/"+home+"/kind", map[string]any{"kind": "folder"})
	viewer.want(403, "POST", "/api/v1/pages/"+page+"/kind", map[string]any{"kind": "folder"})
	owner.ok("POST", "/api/v1/pages/"+page+"/kind", map[string]any{"kind": "folder"})
	if viewer.ok("GET", "/api/v1/pages/"+page, nil)["kind"] != "folder" {
		t.Fatal("no folder")
	}
	owner.ok("POST", "/api/v1/pages/"+page+"/kind", map[string]any{"kind": "page"})
	if !strings.Contains(mustJSON(t, viewer.ok("GET", "/api/v1/pages/"+page, nil)["document"]), "old words") {
		t.Fatal("the old text comes back")
	}
	owner.want(409, "POST", "/api/v1/pages/"+page+"/kind", map[string]any{"kind": "post"})

	fresh := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Group", "kind": "folder"})
	if fresh["status"] != "published" {
		t.Fatalf("a folder has nothing to write: %v", fresh)
	}
	link := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Elsewhere", "kind": "link", "link": "https://example.com/x"})
	if link["link_target"] != "https://example.com/x" || !has(treeTitles(viewer, space), "Elsewhere") {
		t.Fatalf("link entry: %v", link)
	}
	owner.want(400, "POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Bad", "kind": "link", "link": "javascript:alert(1)"})
	owner.want(400, "POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Bad", "kind": "template"})
}

func TestSameTitleOnlyAnswersWithWhatThePersonCanSee(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	hidden := owner.page(space, nil, "Twin", "x")
	owner.restrict(hidden, "view", owner.id)
	if viewer.ok("GET", "/api/v1/same-title?space_id="+space+"&title=twin", nil)["page"] != nil {
		t.Fatal("a hidden title leaked")
	}
	if owner.ok("GET", "/api/v1/same-title?space_id="+space+"&title=twin", nil)["page"] == nil {
		t.Fatal("the owner should get a warning")
	}
}

func TestOthersEditingAreNamed(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	page := owner.page(space, nil, "Together", "x")
	editor.saveDraft(page, "Together", 1, "y")
	got := owner.ok("GET", "/api/v1/pages/"+page+"/editors", nil)["editors"].([]any)
	if len(got) != 1 || got[0].(map[string]any)["name"] != "Test editor@example.com" {
		t.Fatalf("editors: %v", got)
	}
	if len(editor.ok("GET", "/api/v1/pages/"+page+"/editors", nil)["editors"].([]any)) != 0 {
		t.Fatal("a person is not warned about themselves")
	}
}

func TestOldDraftsAreWarnedAboutAndThenRemoved(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.page(space, nil, "Old draft", "x")
	owner.saveDraft(page, "Old draft", 1, "y")
	ctx := t.Context()
	if _, err := e.app.Pool().Exec(ctx, "UPDATE drafts SET updated_at = now() - interval '360 days'"); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Pages().ExpireDrafts(ctx); err != nil {
		t.Fatal(err)
	}
	if e.outboxCount("page.draft_expiring") != 1 {
		t.Fatal("no warning")
	}
	if owner.ok("GET", "/api/v1/pages/"+page, nil)["has_draft"] != true {
		t.Fatal("the warning comes first")
	}
	if err := e.app.Pages().ExpireDrafts(ctx); err != nil {
		t.Fatal(err)
	}
	if e.outboxCount("page.draft_expiring") != 1 {
		t.Fatal("one warning is enough")
	}
	if _, err := e.app.Pool().Exec(ctx, "UPDATE drafts SET updated_at = now() - interval '370 days'"); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Pages().ExpireDrafts(ctx); err != nil {
		t.Fatal(err)
	}
	if owner.ok("GET", "/api/v1/pages/"+page, nil)["has_draft"] != false {
		t.Fatal("the draft stayed")
	}
}

func TestDocumentFieldsNeverReachAReader(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	page := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Filled"})["id"].(string)
	withField := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "templateField", "attrs": map[string]any{"name": "Topic", "kind": "line"}}}}}}
	withHint := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": "Words "},
		map[string]any{"type": "instruction", "attrs": map[string]any{"text": "Write here"}}}}}}
	owner.ok("PUT", "/api/v1/pages/"+page+"/draft", map[string]any{"title": "Filled", "document": withField, "base_version": 0})
	owner.want(400, "POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Filled", "document": withField, "base_version": 0})
	owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Filled", "document": withHint, "base_version": 0})
	if got := mustJSON(t, owner.ok("GET", "/api/v1/pages/"+page, nil)["document"]); strings.Contains(got, "Write here") {
		t.Fatalf("a reader must never see instruction text: %s", got)
	}
}

func TestSearchFiltersNeverRevealHiddenPages(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	viewer2 := e.user("people@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	owner.grant(space, "user", viewer2.id, permissions.TemplateViewer)
	first := editor.page(space, nil, "Quarterly holiday plan", "budget for the year")
	second := owner.page(space, first, "Budget notes", "open plan offices")
	hidden := owner.page(space, nil, "Hidden budget", "secretword inside")
	post := editor.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Budget news", "kind": "post"})["id"].(string)
	editor.publish(post, "Budget news", 0, para("x"))
	owner.ok("POST", "/api/v1/pages/"+first+"/labels", map[string]any{"label": "hr"})
	owner.ok("POST", "/api/v1/pages/"+hidden+"/labels", map[string]any{"label": "hr"})
	owner.ok("PUT", "/api/v1/pages/"+second+"/stage", map[string]any{"text": "Being written", "color": "blue"})
	owner.restrict(hidden, "view", owner.id)

	titles := func(u *user, query string) []string {
		t.Helper()
		return titlesIn(u.ok("GET", "/api/v1/search?"+query, nil), "results")
	}
	if got := titles(viewer, "q=budget"); len(got) != 3 || has(got, "Hidden budget") {
		t.Fatalf("budget: %v", got)
	}
	if got := titles(viewer, "q=budget&kind=post"); len(got) != 1 || got[0] != "Budget news" {
		t.Fatalf("kind: %v", got)
	}
	if got := titles(viewer, "q=budget&label=hr"); len(got) != 1 || got[0] != "Quarterly holiday plan" {
		t.Fatalf("label: %v", got)
	}
	if got := titles(viewer, "label=hr"); len(got) != 1 {
		t.Fatalf("a filter alone finds pages too, without hidden ones: %v", got)
	}
	if got := titles(viewer, "q=budget&stage=Being+written"); len(got) != 1 || got[0] != "Budget notes" {
		t.Fatalf("stage: %v", got)
	}
	if got := titles(viewer, "q=budget&below="+first); len(got) != 2 {
		t.Fatalf("below: %v", got)
	}
	if got := titles(viewer, "q=budget&titles=1"); has(got, "Quarterly holiday plan") {
		t.Fatalf("titles only: %v", got)
	}
	if got := titles(viewer, "q=budget&contributor="+owner.id); len(got) != 1 || got[0] != "Budget notes" {
		t.Fatalf("contributor: %v", got)
	}
	if got := titles(viewer, `q=`+url.QueryEscape(`"open plan"`)); len(got) != 1 || got[0] != "Budget notes" {
		t.Fatalf("phrase: %v", got)
	}
	if got := titles(viewer, "q="+url.QueryEscape("budget AND NOT news")); has(got, "Budget news") {
		t.Fatalf("NOT: %v", got)
	}
	if got := titles(viewer, "q="+url.QueryEscape("label:hr budget")); len(got) != 1 {
		t.Fatalf("field in the box: %v", got)
	}
	if got := titles(viewer, "q="+url.QueryEscape("bud?et")); len(got) != 3 {
		t.Fatalf("wildcard: %v", got)
	}
	if got := titles(viewer, "q="+url.QueryEscape("bu*t AND holiday")); len(got) != 1 {
		t.Fatalf("wildcard inside a word: %v", got)
	}
	if got := titles(viewer2, "q="+url.QueryEscape("author:editor budget")); !has(got, "Budget news") || has(got, "Budget notes") {
		t.Fatalf("author: %v", got)
	}
	if got := titles(viewer, "q="+url.QueryEscape("author:nobody budget")); len(got) != 3 {
		t.Fatalf("without view profiles author is ignored: %v", got)
	}
	for _, odd := range []string{"(((", `"`, "AND", ":", "*", "'; DROP TABLE pages; --", "title:", "\x00"} {
		viewer.want(200, "GET", "/api/v1/search?q="+url.QueryEscape(odd), nil)
	}

	got := viewer.ok("GET", "/api/v1/search?q=budget&sort=date", nil)
	if got["total"].(float64) != 3 || got["capped"] != false {
		t.Fatalf("total: %v", got)
	}
	crumbs := viewer.ok("GET", "/api/v1/search?q=offices", nil)["results"].([]any)[0].(map[string]any)["breadcrumbs"].([]any)
	if len(crumbs) != 1 || crumbs[0].(map[string]any)["title"] != "Quarterly holiday plan" {
		t.Fatalf("breadcrumbs: %v", crumbs)
	}

	suggested := viewer.ok("GET", "/api/v1/search?q=holliday", nil)["suggestion"]
	if suggested != "holiday" {
		t.Fatalf("suggestion: %v", suggested)
	}
	if got := viewer.ok("GET", "/api/v1/search?q=secretwrod", nil)["suggestion"]; got != "" {
		t.Fatalf("a suggestion leaked a word from a hidden page: %v", got)
	}
	if got := viewer.ok("GET", "/api/v1/search?q=hiden", nil)["suggestion"]; got != "" {
		t.Fatalf("a suggestion leaked a hidden title: %v", got)
	}
}

func TestSearchLeavesArchivedSpacesOutUnlessAsked(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.page(space, nil, "Findable", "x")
	owner.ok("POST", "/api/v1/spaces/"+space+"/archive", nil)
	if got := viewer.ok("GET", "/api/v1/search?q=findable", nil)["results"].([]any); len(got) != 0 {
		t.Fatalf("archived space: %v", got)
	}
	if got := viewer.ok("GET", "/api/v1/search?q=findable&archived=1", nil)["results"].([]any); len(got) != 1 {
		t.Fatalf("with the filter: %v", got)
	}
}
