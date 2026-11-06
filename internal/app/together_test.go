// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/app"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/permissions"
)

func commentBody(text string, extra ...any) map[string]any {
	content := []any{map[string]any{"type": "text", "text": text}}
	content = append(content, extra...)
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": content}}}
}

func mention(id, label string) map[string]any {
	return map[string]any{"type": "mention", "attrs": map[string]any{"id": id, "kind": "user", "label": label}}
}

func (u *user) comment(page, text string) string {
	u.e.t.Helper()
	return u.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody(text)})["id"].(string)
}

func (e *env) deliver() {
	e.t.Helper()
	if err := e.app.DeliverEvents(context.Background()); err != nil {
		e.t.Fatal(err)
	}
}

func (u *user) notifications() []map[string]any {
	u.e.t.Helper()
	u.e.deliver()
	out := []map[string]any{}
	for _, n := range u.ok("GET", "/api/v1/me/notifications", nil)["notifications"].([]any) {
		out = append(out, n.(map[string]any))
	}
	return out
}

func (u *user) unread() int {
	u.e.t.Helper()
	u.e.deliver()
	return int(u.ok("GET", "/api/v1/me/notifications/count", nil)["unread"].(float64))
}

func threads(m map[string]any, key string) []map[string]any {
	out := []map[string]any{}
	for _, c := range m[key].([]any) {
		out = append(out, c.(map[string]any))
	}
	return out
}

func TestACommentLivesRepliesAndLeavesALineWhenRemoved(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	other := e.user("other@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	owner.grant(space, "user", other.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	root := reader.comment(page, "first")
	reply := owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("answer"), "parent_id": root})["id"].(string)
	deep := reader.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("deeper"), "parent_id": reply})
	if deep["parent_id"] != root {
		t.Fatalf("a reply to a reply must join the thread: %v", deep["parent_id"])
	}

	other.want(403, "PATCH", "/api/v1/comments/"+root, map[string]any{"body": commentBody("not mine")})
	got := reader.ok("PATCH", "/api/v1/comments/"+root, map[string]any{"body": commentBody("first, better")})
	if got["edited_at"] == nil {
		t.Fatal("an edit must show")
	}
	owner.ok("PATCH", "/api/v1/comments/"+root, map[string]any{"body": commentBody("first, by the administrator")})

	other.want(403, "DELETE", "/api/v1/comments/"+root, nil)
	reader.ok("DELETE", "/api/v1/comments/"+root, nil)
	list := threads(other.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "threads")
	if len(list) != 1 || list[0]["removed"] != true || list[0]["body"] != nil || len(list[0]["replies"].([]any)) != 2 {
		t.Fatalf("thread after delete: %v", list)
	}
	reader.ok("POST", "/api/v1/comments/"+root+"/undo-delete", nil)
	if threads(other.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "threads")[0]["removed"] != false {
		t.Fatal("undo did not bring the words back")
	}

	reader.ok("DELETE", "/api/v1/comments/"+root, nil)
	other.want(404, "POST", "/api/v1/comments/"+root+"/undo-delete", nil)
	items := owner.ok("GET", "/api/v1/spaces/"+space+"/trash", nil)["items"].([]any)
	trashID := items[0].(map[string]any)["id"].(string)
	if items[0].(map[string]any)["object_type"] != "comment" {
		t.Fatalf("trash: %v", items)
	}
	owner.ok("POST", "/api/v1/trash/"+trashID+"/restore", nil)
	if threads(other.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "threads")[0]["removed"] != false {
		t.Fatal("restoring from the trash did not bring it back")
	}

	reader.ok("DELETE", "/api/v1/comments/"+root, nil)
	trashID = owner.ok("GET", "/api/v1/spaces/"+space+"/trash", nil)["items"].([]any)[0].(map[string]any)["id"].(string)
	owner.ok("DELETE", "/api/v1/trash/"+trashID, nil)
	list = threads(other.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "threads")
	if len(list) != 1 || list[0]["removed"] != true || len(list[0]["replies"].([]any)) != 2 {
		t.Fatalf("after a purge the replies must stay: %v", list)
	}
	if e.auditActions("comment.created") != 3 || e.outboxCount("comment.created") != 3 {
		t.Fatal("no audit or outbox event")
	}
	var words string
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT body::text FROM comments WHERE id = $1", root).Scan(&words); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(words, "first") {
		t.Fatal("a purge must take the words away")
	}
}

func TestACommentIsCheckedAndStaysBehindItsPage(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	owner.restrict(hidden, "view", owner.id)
	other := owner.page(space, nil, "Other", "x")
	comment := owner.comment(hidden, "secret")

	bad := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "script", "content": []any{}}}}
	reader.want(400, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": bad})
	reader.want(400, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody(" ")})
	stranger.want(404, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("x")})
	reader.want(404, "POST", "/api/v1/pages/"+hidden+"/comments", map[string]any{"body": commentBody("x")})
	reader.want(404, "GET", "/api/v1/pages/"+hidden+"/comments", nil)
	reader.want(404, "PATCH", "/api/v1/comments/"+comment, map[string]any{"body": commentBody("x")})
	reader.want(404, "PUT", "/api/v1/comments/"+comment+"/reactions/"+url.PathEscape("👍"), nil)

	here := owner.comment(page, "here")
	reader.want(400, "POST", "/api/v1/pages/"+other+"/comments", map[string]any{"body": commentBody("x"), "parent_id": here})

	image := map[string]any{"type": "image", "attrs": map[string]any{"attachment_id": "018f0000-0000-7000-8000-000000000000", "alt": "x"}}
	foreign := map[string]any{"type": "doc", "content": []any{image}}
	reader.want(400, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": foreign})

	owner.ok("PATCH", "/api/v1/pages/"+page+"/comments-closed", map[string]any{"closed": true})
	reader.want(409, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("x")})
	if len(threads(reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "threads")) != 1 {
		t.Fatal("existing comments must stay readable")
	}
	owner.ok("PATCH", "/api/v1/pages/"+page+"/comments-closed", map[string]any{"closed": false})
	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": false, "reactions": true})
	reader.want(409, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("x")})
	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": true, "reactions": true})

	for range 30 {
		reader.comment(page, "again")
	}
	reader.want(409, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("too many")})
}

func TestInlineThreadsResolveAndStayWithEditorsUntilPublishing(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "some words here")

	anchor := map[string]any{"kind": "text", "exact": "words", "prefix": "some ", "suffix": " here", "offset": 5, "version": 1}
	id := reader.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("what?"), "anchor": anchor})["id"].(string)
	list := reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil)
	inline := threads(list, "inline")
	if len(inline) != 1 || inline[0]["anchor"].(map[string]any)["exact"] != "words" || len(threads(list, "threads")) != 0 {
		t.Fatalf("list: %v", list)
	}

	reader.want(403, "POST", "/api/v1/comments/"+id+"/resolve", nil)
	owner.ok("POST", "/api/v1/comments/"+id+"/resolve", nil)
	if threads(reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "inline")[0]["status"] != "resolved" {
		t.Fatal("not resolved")
	}
	owner.ok("POST", "/api/v1/comments/"+id+"/reopen", nil)

	for name, bad := range map[string]map[string]any{
		"empty": {"kind": "text", "exact": " "},
		"long":  {"kind": "text", "exact": strings.Repeat("x", 1001)},
		"kind":  {"kind": "sound", "exact": "x"},
		"image": {"kind": "image", "attachment_id": "018f0000-0000-7000-8000-000000000000"},
	} {
		if status, out := reader.do("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("x"), "anchor": bad}); status != 400 {
			t.Errorf("%s: %d %s", name, status, out)
		}
	}

	reader.want(403, "POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("x"), "anchor": anchor, "editors_only": true})
	private := owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("while editing"), "anchor": anchor, "editors_only": true})["id"].(string)
	if len(threads(reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "inline")) != 1 {
		t.Fatal("a reader sees a thread for editors")
	}
	reader.want(404, "PUT", "/api/v1/comments/"+private+"/reactions/"+url.PathEscape("👍"), nil)
	owner.publish(page, "Page", 1, para("some words here, and more"))
	if len(threads(reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "inline")) != 2 {
		t.Fatal("publishing must open the thread")
	}

	// A sentence that was never published stays with the editors, with its replies.
	draft := map[string]any{"kind": "text", "exact": "a draft sentence", "version": 2}
	kept := owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("reword"), "anchor": draft, "editors_only": true})["id"].(string)
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("agreed"), "parent_id": kept})
	owner.publish(page, "Page", 2, para("some words here, and much more"))
	if body := reader.want(200, "GET", "/api/v1/pages/"+page+"/comments", nil); strings.Contains(body, "a draft sentence") || strings.Contains(body, "agreed") {
		t.Fatalf("words that were never published reached a reader: %s", body)
	}
	if len(threads(owner.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "inline")) != 3 {
		t.Fatal("the editors keep the thread")
	}
	owner.publish(page, "Page", 3, para("some words here, and a draft sentence too"))
	opened := threads(reader.ok("GET", "/api/v1/pages/"+page+"/comments", nil), "inline")
	if len(opened) != 3 || len(opened[2]["replies"].([]any)) != 1 {
		t.Fatalf("publishing the words must open the thread and its replies: %v", opened)
	}
}

func TestAMentionStoresTheIdAndNeverTheName(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateEditor)
	rename := func(name string) {
		t.Helper()
		if _, err := e.app.Pool().Exec(ctx, "UPDATE users SET display_name = $2 WHERE id = $1", anna.id, name); err != nil {
			t.Fatal(err)
		}
	}
	rename("Zsofia Quartermain")
	page := owner.page(space, nil, "Page", "x")
	body := commentBody("hello ", mention(anna.id, "Zsofia Quartermain"))
	owner.publish(page, "Page", 1, body)
	owner.ok("PUT", "/api/v1/pages/"+page+"/draft", map[string]any{"title": "Page", "document": body, "base_version": 2})
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": body})

	var stored int
	err := e.app.Pool().QueryRow(ctx, `SELECT (SELECT count(*) FROM page_versions WHERE document::text LIKE '%Quartermain%')
		+ (SELECT count(*) FROM drafts WHERE document::text LIKE '%Quartermain%')
		+ (SELECT count(*) FROM comments WHERE body::text LIKE '%Quartermain%')
		+ (SELECT count(*) FROM search_documents WHERE body LIKE '%Quartermain%')`).Scan(&stored)
	if err != nil {
		t.Fatal(err)
	}
	if stored != 0 {
		t.Fatalf("a stored name can never be anonymised: %d rows", stored)
	}

	rename("Zsofia Newname")
	for name, path := range map[string]string{
		"page":     "/api/v1/pages/" + page,
		"draft":    "/api/v1/pages/" + page + "/draft",
		"version":  "/api/v1/pages/" + page + "/versions/2",
		"compare":  "/api/v1/pages/" + page + "/compare?from=1&to=2",
		"comments": "/api/v1/pages/" + page + "/comments",
		"export":   "/api/v1/pages/" + page + "/export.md",
	} {
		if out := owner.want(200, "GET", path, nil); !strings.Contains(out, "Zsofia Newname") {
			t.Errorf("%s must show the name of today: %s", name, out)
		}
	}
	if out := owner.want(200, "GET", "/api/v1/search?q=Newname", nil); !strings.Contains(out, `"title":"Page"`) {
		t.Fatalf("search must find a page by a person it mentions: %s", out)
	}
	if out := owner.want(200, "GET", "/api/v1/search?q=hello+Newname", nil); !strings.Contains(out, `"title":"Page"`) {
		t.Fatalf("a name and a word of the page must work together: %s", out)
	}
	if out := owner.want(200, "GET", "/api/v1/search?q=Quartermain", nil); strings.Contains(out, `"title":"Page"`) {
		t.Fatalf("the old name must be gone: %s", out)
	}
}

func TestRemovingAVersionBlanksTheQuotesOfItsWords(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "the password is hunter2 for now")
	secret := map[string]any{"kind": "text", "exact": "hunter2", "prefix": "the password is ", "suffix": " for now", "offset": 16, "version": 1}
	stays := map[string]any{"kind": "text", "exact": "for now", "prefix": "hunter2 ", "offset": 24, "version": 1}
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("oops"), "anchor": secret})
	owner.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("fine"), "anchor": stays})
	owner.publish(page, "Page", 1, para("the password is gone for now"))

	parse := func(text string) document.Node {
		t.Helper()
		raw, err := json.Marshal(para(text))
		if err != nil {
			t.Fatal(err)
		}
		n, err := document.Normalize(raw)
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	caller := identity.Caller{UserID: uuid.MustParse(owner.id)}
	err := pgx.BeginFunc(ctx, e.app.Pool(), func(tx pgx.Tx) error {
		return comments.BlankQuotes(ctx, tx, caller, uuid.MustParse(page), parse("the password is hunter2 for now"), parse("the password is gone for now"))
	})
	if err != nil {
		t.Fatal(err)
	}
	list := owner.ok("GET", "/api/v1/pages/"+page+"/comments", nil)
	inline := threads(list, "inline")
	if len(inline) != 2 {
		t.Fatalf("a thread must stay when its quote goes: %v", list)
	}
	if body := owner.want(200, "GET", "/api/v1/pages/"+page+"/comments", nil); strings.Contains(body, "the password is") || !strings.Contains(body, "oops") {
		t.Fatalf("the words of a removed version live on: %s", body)
	}
	if inline[1]["anchor"].(map[string]any)["exact"] != "for now" {
		t.Fatalf("words that are still on the page stay: %v", inline[1])
	}
	if e.auditActions("comment.quote_blanked") != 1 || e.outboxCount("comment.quote_blanked") != 1 {
		t.Fatal("no audit or outbox event")
	}
}

func TestAReactionChangesNothingInAnArchivedSpace(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	thumb := url.PathEscape("👍")
	comment := reader.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("hello")})["id"].(string)
	reader.ok("PUT", "/api/v1/pages/"+page+"/reactions/"+thumb, nil)
	if reader.ok("GET", "/api/v1/pages/"+page, nil)["can"].(map[string]any)["reaction.set"] != true {
		t.Fatal("the page must say that a reaction is possible")
	}

	owner.ok("POST", "/api/v1/spaces/"+space+"/archive", nil)
	reader.want(403, "PUT", "/api/v1/pages/"+page+"/reactions/"+url.PathEscape("🚀"), nil)
	reader.want(403, "DELETE", "/api/v1/pages/"+page+"/reactions/"+thumb, nil)
	reader.want(403, "PUT", "/api/v1/comments/"+comment+"/reactions/"+thumb, nil)
	if len(reader.ok("GET", "/api/v1/pages/"+page+"/reactions", nil)["reactions"].([]any)) != 1 {
		t.Fatal("an archived space must keep what it has")
	}
	if reader.ok("GET", "/api/v1/pages/"+page, nil)["can"].(map[string]any)["reaction.set"] != false {
		t.Fatal("the page must say that a reaction is not possible")
	}
}

func TestFollowingGoesByTheRuleForSeeingPeople(t *testing.T) {
	e := newEnv(t)
	seer := e.user("seer@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	blind := e.user("blind@example.com", identity.PermUseProduct)
	person := e.user("person@example.com", identity.PermUseProduct)
	missing := "018f0000-0000-7000-8000-000000000000"

	known := blind.want(404, "PUT", "/api/v1/users/"+person.id+"/follow", nil)
	unknown := blind.want(404, "PUT", "/api/v1/users/"+missing+"/follow", nil)
	if known != unknown {
		t.Fatalf("the answer must not tell who exists: %s and %s", known, unknown)
	}
	if len(blind.ok("GET", "/api/v1/me/following", nil)["following"].([]any)) != 0 {
		t.Fatal("following without view profiles")
	}
	seer.want(404, "PUT", "/api/v1/users/"+missing+"/follow", nil)
	seer.ok("PUT", "/api/v1/users/"+person.id+"/follow", nil)
	if len(seer.ok("GET", "/api/v1/me/following", nil)["following"].([]any)) != 1 {
		t.Fatal("a person with view profiles can follow")
	}
	// Stopping tells nothing, so it answers the same for anyone.
	blind.ok("DELETE", "/api/v1/users/"+person.id+"/follow", nil)
	blind.ok("DELETE", "/api/v1/users/"+missing+"/follow", nil)
}

func levelsOf(request map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, entry := range request["levels"].([]any) {
		level := entry.(map[string]any)
		out[level["title"].(string)] = level
	}
	return out
}

func TestAllowingAccessNamesEveryLevelAndNeedsThemAll(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	manager := e.user("manager@example.com", identity.PermUseProduct)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject":     map[string]any{"type": "user", "id": manager.id},
		"permissions": []string{permissions.SpaceView, permissions.RestrictionManage},
	})
	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": true, "reactions": true, "access_requests": true})
	top := owner.page(space, nil, "Branch", "x")
	middle := owner.page(space, top, "Middle", "x")
	leaf := owner.page(space, middle, "Leaf", "x")
	owner.restrict(top, "view", owner.id, manager.id)
	owner.restrict(leaf, "view", owner.id, manager.id)

	reader.ok("POST", "/api/v1/pages/"+leaf+"/access-requests", map[string]any{"kind": "view"})
	open := owner.ok("GET", "/api/v1/pages/"+leaf+"/access-requests", nil)["requests"].([]any)[0].(map[string]any)
	levels := levelsOf(open)
	if len(levels) != 2 || levels["Branch"]["pages"].(float64) != 3 || levels["Leaf"]["pages"].(float64) != 1 || open["can_allow"] != true {
		t.Fatalf("the answer must name each level and what it opens: %v", open)
	}

	// A level someone may not manage: they learn that it is there, and nothing else.
	setStatus := func(status string) {
		t.Helper()
		if _, err := e.app.Pool().Exec(ctx, "UPDATE pages SET status = $2 WHERE id = $1", top, status); err != nil {
			t.Fatal(err)
		}
	}
	setStatus("draft")
	id := open["id"].(string)
	seen := manager.ok("GET", "/api/v1/access-requests/"+id, nil)
	hidden := levelsOf(seen)[""]
	if seen["can_allow"] != false || hidden == nil || hidden["can_manage"] != false || hidden["pages"].(float64) != 0 {
		t.Fatalf("a level out of reach: %v", seen)
	}
	manager.want(409, "POST", "/api/v1/access-requests/"+id+"/allow", map[string]any{})
	reader.want(404, "GET", "/api/v1/pages/"+leaf, nil)

	setStatus("published")
	manager.ok("POST", "/api/v1/access-requests/"+id+"/allow", map[string]any{})
	reader.want(200, "GET", "/api/v1/pages/"+leaf, nil)
	reader.want(200, "GET", "/api/v1/pages/"+middle, nil)
}

func TestAllowingOpensTheSpaceAndEveryRestriction(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": true, "reactions": true, "access_requests": true})
	page := owner.page(space, nil, "Page", "x")
	owner.restrict(page, "view", owner.id)

	outsider.ok("POST", "/api/v1/pages/"+page+"/access-requests", map[string]any{"kind": "view"})
	open := owner.ok("GET", "/api/v1/pages/"+page+"/access-requests", nil)["requests"].([]any)[0].(map[string]any)
	if open["effect"] != "space_editor" || len(open["levels"].([]any)) != 1 {
		t.Fatalf("both the space and the restriction must show: %v", open)
	}
	// Managing restrictions is not enough: this widens the whole space.
	manager := e.user("manager@example.com", identity.PermUseProduct)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject":     map[string]any{"type": "user", "id": manager.id},
		"permissions": []string{permissions.SpaceView, permissions.RestrictionManage},
	})
	owner.restrict(page, "view", owner.id, manager.id)
	manager.want(403, "GET", "/api/v1/access-requests/"+open["id"].(string), nil)
	manager.want(403, "POST", "/api/v1/access-requests/"+open["id"].(string)+"/allow", map[string]any{})
	owner.ok("POST", "/api/v1/access-requests/"+open["id"].(string)+"/allow", map[string]any{})
	outsider.want(200, "GET", "/api/v1/pages/"+page, nil)
}

func TestReactionsHaveLimitsAndCanBeTurnedOff(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := admin.space("ENG")
	admin.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := admin.page(space, nil, "Page", "x")
	path := "/api/v1/pages/" + page + "/reactions/"
	thumb := url.PathEscape("👍")

	reader.ok("PUT", path+thumb, nil)
	reader.ok("PUT", path+thumb, nil)
	got := admin.ok("GET", "/api/v1/pages/"+page+"/reactions", nil)
	counts := got["reactions"].([]any)
	if len(counts) != 1 || counts[0].(map[string]any)["count"].(float64) != 1 || counts[0].(map[string]any)["mine"] != false {
		t.Fatalf("counts: %v", counts)
	}
	who := got["who"].([]any)[0].(map[string]any)["names"].([]any)
	if len(who) != 1 || who[0] != "Test reader@example.com" {
		t.Fatalf("who: %v", who)
	}
	reader.ok("DELETE", path+thumb, nil)
	if len(admin.ok("GET", "/api/v1/pages/"+page+"/reactions", nil)["reactions"].([]any)) != 0 {
		t.Fatal("not removed")
	}
	reader.want(400, "PUT", path+"hello", nil)
	reader.want(400, "PUT", path+url.PathEscape("<b>"), nil)
	for r := '😀'; r < '😀'+20; r++ {
		reader.ok("PUT", path+url.PathEscape(string(r)), nil)
	}
	reader.want(409, "PUT", path+url.PathEscape("🚀"), nil)
	if e.auditActions("reaction.added") != 21 || e.outboxCount("reaction.added") != 21 {
		t.Fatal("no audit or outbox event")
	}

	admin.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": true, "reactions": false})
	reader.want(409, "PUT", path+thumb, nil)
	if len(admin.ok("GET", "/api/v1/pages/"+page+"/reactions", nil)["reactions"].([]any)) != 0 {
		t.Fatal("reactions must stop showing")
	}
	admin.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": true, "reactions": true})
	if len(admin.ok("GET", "/api/v1/pages/"+page+"/reactions", nil)["reactions"].([]any)) != 20 {
		t.Fatal("turning off must delete nothing")
	}
	admin.ok("PUT", "/api/v1/admin/settings", map[string]any{"reactions_enabled": false})
	reader.want(409, "PUT", path+thumb, nil)
}

func TestAMentionNotifiesOnceAndNeverLeaks(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	doc := commentBody("hello ", mention(anna.id, "Anna"), mention(outsider.id, "Outsider"))
	owner.publish(page, "Page", 1, doc)
	got := anna.notifications()
	if len(got) != 1 || got[0]["type"] != "mention" || got[0]["reason"] != "mentioned" || got[0]["page_title"] != "Page" {
		t.Fatalf("mention: %v", got)
	}
	if len(outsider.notifications()) != 0 || outsider.unread() != 0 {
		t.Fatal("someone without access heard about a page")
	}
	owner.publish(page, "Page", 2, doc)
	if anna.unread() != 1 {
		t.Fatal("a mention must notify once per version it is new in")
	}

	// A notification row holds ids, never a title or a sentence.
	var rows int
	err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM notifications WHERE data::text LIKE '%Page%' OR group_key LIKE '%Page%'").Scan(&rows)
	if err != nil || rows != 0 {
		t.Fatalf("a row holds words: %d %v", rows, err)
	}

	// The name follows a rename, while the stored version stays what it was.
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE users SET display_name = 'Anna Visser' WHERE id = $1", anna.id); err != nil {
		t.Fatal(err)
	}
	if body := owner.want(200, "GET", "/api/v1/pages/"+page, nil); !strings.Contains(body, "Anna Visser") {
		t.Fatalf("name does not follow: %s", body)
	}

	third := e.user("third@example.com", identity.PermUseProduct)
	owner.grant(space, "user", third.id, permissions.TemplateViewer)
	owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Page", "base_version": 3, "silent": true,
		"document": commentBody("fix ", mention(third.id, "Third"))})
	if third.unread() != 1 {
		t.Fatal("a silent update still reaches a new mention: it is addressed to a person")
	}

	id := owner.comment(page, "plain")
	owner.ok("PATCH", "/api/v1/comments/"+id, map[string]any{"body": commentBody("now ", mention(anna.id, "Anna"))})
	if got := anna.notifications()[0]["count"].(float64); got != 2 {
		t.Fatalf("a mention added by an edit must notify: %v", got)
	}
	owner.ok("PATCH", "/api/v1/comments/"+id, map[string]any{"body": commentBody("still ", mention(anna.id, "Anna"))})
	if got := anna.notifications()[0]["count"].(float64); got != 2 {
		t.Fatal("an edit must only notify new mentions")
	}
}

func TestGroupMentionsAreOffAndHaveALimit(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	member := e.user("member@example.com", identity.PermUseProduct)
	space, _ := admin.space("ENG")
	admin.grant(space, "all_members", "", permissions.TemplateViewer)
	group := admin.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Team"})["id"].(string)
	admin.ok("PUT", "/api/v1/admin/groups/"+group+"/members/"+member.id, nil)
	page := admin.page(space, nil, "Page", "x")
	groupMention := map[string]any{"type": "mention", "attrs": map[string]any{"id": group, "kind": "group", "label": "Team"}}

	admin.publish(page, "Page", 1, commentBody("hello ", groupMention))
	if member.unread() != 0 {
		t.Fatal("group mentions are off by default")
	}
	admin.ok("PUT", "/api/v1/admin/settings", map[string]any{"group_mentions": true})
	second := admin.page(space, nil, "Second", "x")
	admin.publish(second, "Second", 1, commentBody("hello ", groupMention))
	if member.unread() != 1 {
		t.Fatal("an allowed group mention must reach its members")
	}
	if body := admin.want(200, "GET", "/api/v1/pages/"+page+"/mention-candidates?q=Team", nil); !strings.Contains(body, "Team") {
		t.Fatalf("candidates: %s", body)
	}
	for i := range 51 {
		extra := e.user("m"+string(rune('a'+i%26))+string(rune('a'+i/26))+"@example.com", identity.PermUseProduct)
		admin.ok("PUT", "/api/v1/admin/groups/"+group+"/members/"+extra.id, nil)
	}
	third := admin.page(space, nil, "Third", "x")
	before := member.unread()
	admin.publish(third, "Third", 1, commentBody("hello ", groupMention))
	if member.unread() != before {
		t.Fatal("a group above the limit must not be reached")
	}
	if body := admin.want(200, "GET", "/api/v1/pages/"+page+"/mention-candidates?q=Team", nil); strings.Contains(body, "Team") {
		t.Fatal("the list must not offer a group above the limit")
	}
}

func TestWatchersHearOnceAndLosingAccessEndsIt(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	branch := e.user("branch@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	owner.grant(space, "user", branch.id, permissions.TemplateViewer)
	parent := owner.page(space, nil, "Parent", "x")
	child := owner.page(space, parent, "Child", "x")
	e.deliver()

	reader.ok("PUT", "/api/v1/spaces/"+space+"/watch", nil)
	branch.ok("PUT", "/api/v1/pages/"+parent+"/watch?scope=branch", nil)
	if branch.ok("GET", "/api/v1/pages/"+parent+"/watch", nil)["branch"] != true {
		t.Fatal("watch state")
	}
	branch.want(400, "PUT", "/api/v1/pages/"+parent+"/watch?scope=everything", nil)

	owner.publish(child, "Child", 1, para("changed"))
	if reader.unread() != 1 || branch.unread() != 1 {
		t.Fatalf("watchers: %d %d", reader.unread(), branch.unread())
	}
	if owner.unread() != 0 {
		t.Fatal("nobody hears about their own action")
	}

	owner.ok("POST", "/api/v1/pages/"+child+"/publish", map[string]any{"title": "Child", "base_version": 2, "silent": true, "document": para("a small fix")})
	if reader.unread() != 1 {
		t.Fatal("a silent update must reach nobody")
	}
	versions := owner.ok("GET", "/api/v1/pages/"+child+"/versions", nil)["versions"].([]any)
	if len(versions) != 3 {
		t.Fatal("the history must show a silent version")
	}

	// Read rows of one page with the same type group while unread.
	owner.publish(child, "Child", 3, para("again"))
	got := reader.notifications()
	if len(got) != 1 || got[0]["count"].(float64) != 2 {
		t.Fatalf("grouping: %v", got)
	}

	owner.restrict(child, "view", owner.id)
	if reader.unread() != 0 || len(reader.notifications()) != 0 {
		t.Fatal("a row about a hidden page must disappear")
	}
	owner.publish(child, "Child", 4, para("secret"))
	if reader.unread() != 0 {
		t.Fatal("a person without access must hear nothing")
	}
	var stored int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM notifications WHERE user_id = $1", reader.id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 2 {
		t.Fatalf("a new row was made for someone without access: %d", stored)
	}
	owner.restrict(child, "view")
	if reader.unread() != 1 {
		t.Fatal("old rows come back with access")
	}
	list := reader.ok("GET", "/api/v1/me/watches", nil)["watches"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["type"] != "space" {
		t.Fatalf("watches: %v", list)
	}
	reader.ok("DELETE", "/api/v1/spaces/"+space+"/watch", nil)
	if len(reader.ok("GET", "/api/v1/me/watches", nil)["watches"].([]any)) != 0 {
		t.Fatal("stop did not stop")
	}
}

func TestAutomaticWatchingAndAdministratorsChangingWatchers(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	reader.comment(page, "hello")
	e.deliver()
	if reader.ok("GET", "/api/v1/pages/"+page+"/watch", nil)["page"] != true {
		t.Fatal("commenting must start watching")
	}
	if owner.ok("GET", "/api/v1/pages/"+page+"/watch", nil)["page"] != true {
		t.Fatal("creating must start watching")
	}
	reader.want(403, "GET", "/api/v1/pages/"+page+"/watchers", nil)
	owner.ok("DELETE", "/api/v1/pages/"+page+"/watchers/"+reader.id, nil)
	if e.auditActions("watcher.removed") != 1 {
		t.Fatal("not audited")
	}
	if len(owner.ok("GET", "/api/v1/pages/"+page+"/watchers", nil)["watchers"].([]any)) != 1 {
		t.Fatal("watcher stayed")
	}
	reader.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)
	if len(owner.ok("GET", "/api/v1/pages/"+page+"/watchers", nil)["watchers"].([]any)) != 2 {
		t.Fatal("a person who was removed may watch again")
	}

	settings := owner.ok("GET", "/api/v1/me/notification-settings", nil)
	settings["auto_watch_edit"] = false
	settings["auto_watch_create"] = false
	owner.ok("PUT", "/api/v1/me/notification-settings", settings)
	other := owner.page(space, nil, "Other", "x")
	e.deliver()
	if owner.ok("GET", "/api/v1/pages/"+other+"/watch", nil)["page"] != false {
		t.Fatal("the preference must stop automatic watching")
	}
}

func TestDeliveringAnEventTwiceGivesOneRow(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	owner.publish(page, "Page", 1, commentBody("hello ", mention(reader.id, "R")))
	e.deliver()
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE outbox_events SET published_at = NULL"); err != nil {
		t.Fatal(err)
	}
	e.deliver()
	if reader.unread() != 1 {
		t.Fatalf("rows: %d", reader.unread())
	}
}

func TestOwnActionsOnlyWhenAskedFor(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "x")
	owner.publish(page, "Page", 1, para("watched by my own page"))
	if owner.unread() != 0 {
		t.Fatal("nobody hears about their own action")
	}
	settings := owner.ok("GET", "/api/v1/me/notification-settings", nil)
	settings["notify_own"] = true
	owner.ok("PUT", "/api/v1/me/notification-settings", settings)
	owner.publish(page, "Page", 2, para("mine"))
	if owner.unread() == 0 {
		t.Fatal("the preference must bring own actions back")
	}
}

func TestNotificationsAreYourOwn(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	owner.publish(page, "Page", 1, commentBody("hi ", mention(reader.id, "R")))
	id := reader.notifications()[0]["id"].(string)

	owner.want(404, "PUT", "/api/v1/me/notifications/"+id+"/read", nil)
	reader.ok("PUT", "/api/v1/me/notifications/"+id+"/read", nil)
	if reader.unread() != 0 {
		t.Fatal("still unread")
	}
	reader.ok("PUT", "/api/v1/me/notifications/"+id+"/unread", nil)
	if reader.unread() != 1 {
		t.Fatal("not unread again")
	}
	reader.ok("POST", "/api/v1/me/notifications/read-all", nil)
	if reader.unread() != 0 {
		t.Fatal("read all")
	}
	reader.want(400, "GET", "/api/v1/me/notifications?type=secret", nil)

	settings := reader.ok("GET", "/api/v1/me/notification-settings", nil)
	channels := settings["channels"].(map[string]any)
	channels["mention"] = "off"
	reader.ok("PUT", "/api/v1/me/notification-settings", settings)
	owner.publish(page, "Page", 2, commentBody("one ", mention(reader.id, "R"), mention(owner.id, "O")))
	owner.publish(page, "Page", 3, para("gone"))
	owner.publish(page, "Page", 4, commentBody("two ", mention(reader.id, "R")))
	if reader.unread() != 0 {
		t.Fatal("a type that is off must make no row")
	}
	for name, change := range map[string]func(map[string]any){
		"channel":  func(m map[string]any) { m["channels"] = map[string]any{"mention": "pigeon"} },
		"type":     func(m map[string]any) { m["channels"] = map[string]any{"secret": "app"} },
		"digest":   func(m map[string]any) { m["digest_frequency"] = "hourly" },
		"hour":     func(m map[string]any) { m["digest_hour"] = 24 },
		"zone":     func(m map[string]any) { m["time_zone"] = "Mars/Olympus" },
		"quiet":    func(m map[string]any) { m["quiet_start"] = "late" },
		"pause":    func(m map[string]any) { m["paused_until"] = "soon" },
		"comments": func(m map[string]any) { m["comment_order"] = "random" },
	} {
		body := reader.ok("GET", "/api/v1/me/notification-settings", nil)
		change(body)
		if status, out := reader.do("PUT", "/api/v1/me/notification-settings", body); status != 400 {
			t.Errorf("%s: %d %s", name, status, out)
		}
	}
}

func taskDoc(attrs map[string]any, text string, extra ...any) map[string]any {
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "taskList", "content": []any{
		map[string]any{"type": "taskItem", "attrs": attrs, "content": []any{
			map[string]any{"type": "paragraph", "content": append([]any{map[string]any{"type": "text", "text": text}}, extra...)}}}}}}}
}

func dateNode(value string) map[string]any {
	return map[string]any{"type": "date", "attrs": map[string]any{"value": value}}
}

func tasksOf(u *user, query string) []map[string]any {
	u.e.t.Helper()
	return threads(u.ok("GET", "/api/v1/me/tasks?"+query, nil), "tasks")
}

func TestTasksFollowTheDocument(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateEditor)
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	today := time.Now().UTC().Format(time.DateOnly)
	owner.publish(page, "Page", 1, taskDoc(map[string]any{"checked": false}, "Write it ", mention(anna.id, "Anna"), dateNode(today)))
	mine := tasksOf(anna, "")
	if len(mine) != 1 || !strings.Contains(mine[0]["text"].(string), "Write it") || mine[0]["due"] != today {
		t.Fatalf("my tasks: %v", mine)
	}
	if len(tasksOf(owner, "tab=created")) != 1 || len(tasksOf(owner, "")) != 0 {
		t.Fatal("tabs")
	}
	got := anna.notifications()
	if len(got) != 1 || got[0]["type"] != "task_assigned" {
		t.Fatalf("assigned: %v", got)
	}

	if err := e.app.Notify().RemindDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Notify().RemindDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	var due int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM notifications WHERE type = 'task_due'").Scan(&due); err != nil || due != 1 {
		t.Fatalf("a reminder goes out once: %d %v", due, err)
	}

	task := mine[0]["id"].(string)
	viewer.want(403, "POST", "/api/v1/tasks/"+task+"/done", map[string]any{"done": true})
	anna.ok("POST", "/api/v1/tasks/"+task+"/done", map[string]any{"done": true})
	if len(tasksOf(anna, "")) != 0 || len(tasksOf(anna, "status=done")) != 1 {
		t.Fatal("the tick must show in the list")
	}
	if body := anna.want(200, "GET", "/api/v1/pages/"+page, nil); !strings.Contains(body, `"checked":true`) {
		t.Fatalf("the tick must change the document: %s", body)
	}
	if e.auditActions("task.done") != 1 {
		t.Fatal("not audited")
	}
	before := owner.unread()
	if owner.ok("GET", "/api/v1/pages/"+page, nil)["current_version"].(float64) != 3 {
		t.Fatal("a tick must make a version")
	}
	if owner.unread() != before {
		t.Fatal("a tick is not news")
	}

	copied := owner.ok("POST", "/api/v1/pages/"+page+"/copy", map[string]any{})["id"].(string)
	if len(tasksOf(anna, "status=all")) != 2 {
		t.Fatal("a copy has its own tasks")
	}
	owner.ok("DELETE", "/api/v1/pages/"+copied, nil)
	if len(tasksOf(anna, "status=all")) != 1 {
		t.Fatal("a task in the trash must disappear")
	}
	owner.ok("POST", "/api/v1/pages/"+page+"/versions/2/restore", nil)
	if len(tasksOf(anna, "")) != 1 {
		t.Fatal("restoring a version must bring the open task back")
	}
	owner.restrict(page, "view", owner.id)
	if len(tasksOf(anna, "status=all")) != 0 {
		t.Fatal("a task out of reach must disappear")
	}
	anna.want(400, "GET", "/api/v1/me/tasks?tab=secret", nil)
}

func TestSharingOnlyNotifies(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	people := []map[string]any{{"type": "user", "id": reader.id}, {"type": "user", "id": outsider.id}}
	got := owner.ok("POST", "/api/v1/pages/"+page+"/share", map[string]any{"people": people, "message": "have a look", "notify": true})
	if got["notified"].(float64) != 1 || len(got["no_access"].([]any)) != 1 {
		t.Fatalf("share: %v", got)
	}
	if rows := reader.notifications(); len(rows) != 1 || rows[0]["type"] != "share" || rows[0]["data"].(map[string]any)["message"] != "have a look" {
		t.Fatalf("rows: %v", rows)
	}
	if outsider.unread() != 0 {
		t.Fatal("someone without access must get nothing")
	}
	outsider.want(404, "GET", "/api/v1/pages/"+page, nil)
	if e.auditActions("page.shared") != 1 {
		t.Fatal("not audited")
	}
	var details string
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT payload FROM audit_events WHERE action = 'page.shared'").Scan(&details); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(details, "have a look") {
		t.Fatal("the message must stay out of the audit log")
	}

	owner.want(400, "POST", "/api/v1/pages/"+page+"/share", map[string]any{"people": people, "message": strings.Repeat("x", 501), "notify": true})
	owner.want(400, "POST", "/api/v1/pages/"+page+"/share", map[string]any{"people": []map[string]any{{"type": "all_members", "id": "00000000-0000-0000-0000-000000000000"}}, "notify": true})
	many := []map[string]any{}
	for range 31 {
		many = append(many, map[string]any{"type": "user", "id": uuid.NewString()})
	}
	owner.want(400, "POST", "/api/v1/pages/"+page+"/share", map[string]any{"people": many, "notify": true})

	access := reader.ok("GET", "/api/v1/pages/"+page+"/access", nil)
	if access["restricted"] != false || access["total"].(float64) != 2 {
		t.Fatalf("access: %v", access)
	}
	owner.restrict(page, "view", owner.id)
	if got := owner.ok("GET", "/api/v1/pages/"+page+"/access", nil); got["restricted"] != true || got["total"].(float64) != 1 {
		t.Fatalf("restricted access: %v", got)
	}
}

func TestAskingForAccessReachesSomeoneWhoMayGrantIt(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	outsider := e.user("outsider@example.com", identity.PermUseProduct)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	page := owner.page(space, nil, "Page", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	owner.restrict(hidden, "view", owner.id)

	// Asking to edit: the space is what blocks, so allowing gives Editor in the space.
	reader.ok("POST", "/api/v1/pages/"+page+"/access-requests", map[string]any{"kind": "edit"})
	reader.want(409, "POST", "/api/v1/pages/"+page+"/access-requests", map[string]any{"kind": "edit"})
	rows := owner.notifications()
	if len(rows) != 1 || rows[0]["type"] != "access_request" {
		t.Fatalf("rows: %v", rows)
	}
	request := rows[0]["data"].(map[string]any)["request_id"].(string)
	if got := owner.ok("GET", "/api/v1/access-requests/"+request, nil); got["effect"] != "space_editor" {
		t.Fatalf("describe: %v", got)
	}
	editor.want(403, "POST", "/api/v1/access-requests/"+request+"/allow", map[string]any{})
	owner.ok("POST", "/api/v1/access-requests/"+request+"/allow", map[string]any{})
	if reader.ok("GET", "/api/v1/pages/"+page, nil)["can"].(map[string]any)["page.edit"] != true {
		t.Fatal("allowing must give what the dialog said")
	}
	owner.want(409, "POST", "/api/v1/access-requests/"+request+"/refuse", map[string]any{})
	if len(reader.notifications()) != 1 {
		t.Fatal("the person who asked must hear the answer")
	}
	if e.auditActions("access.requested") != 1 || e.auditActions("access.allowed") != 1 {
		t.Fatal("every step is audited")
	}

	// A page a person may not see stays not found, until the space allows requests.
	reader.want(404, "POST", "/api/v1/pages/"+hidden+"/access-requests", map[string]any{"kind": "view"})
	outsider.want(404, "POST", "/api/v1/pages/"+page+"/access-requests", map[string]any{"kind": "view"})
	if body := reader.want(404, "GET", "/api/v1/pages/"+hidden, nil); strings.Contains(body, "not_found_ask") {
		t.Fatal("the answer must not offer anything")
	}
	owner.ok("PUT", "/api/v1/spaces/"+space+"/features", map[string]any{"comments": true, "reactions": true, "access_requests": true})
	if body := reader.want(404, "GET", "/api/v1/pages/"+hidden, nil); !strings.Contains(body, "not_found_ask") {
		t.Fatalf("no offer: %s", body)
	}
	reader.ok("POST", "/api/v1/pages/"+hidden+"/access-requests", map[string]any{"kind": "view"})
	open := owner.ok("GET", "/api/v1/pages/"+hidden+"/access-requests", nil)["requests"].([]any)
	if len(open) != 1 || open[0].(map[string]any)["effect"] != "restriction" {
		t.Fatalf("open: %v", open)
	}
	id := open[0].(map[string]any)["id"].(string)
	owner.ok("POST", "/api/v1/access-requests/"+id+"/refuse", map[string]any{"note": "not now"})
	reader.want(404, "GET", "/api/v1/pages/"+hidden, nil)
	reader.want(409, "POST", "/api/v1/pages/"+hidden+"/access-requests", map[string]any{"kind": "view"})
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE access_requests SET answered_at = now() - interval '8 days'"); err != nil {
		t.Fatal(err)
	}
	reader.ok("POST", "/api/v1/pages/"+hidden+"/access-requests", map[string]any{"kind": "view"})
	id = owner.ok("GET", "/api/v1/pages/"+hidden+"/access-requests", nil)["requests"].([]any)[0].(map[string]any)["id"].(string)
	owner.ok("POST", "/api/v1/access-requests/"+id+"/allow", map[string]any{})
	reader.want(200, "GET", "/api/v1/pages/"+hidden, nil)
	outsider.want(404, "GET", "/api/v1/pages/"+hidden, nil)
	reader.want(400, "POST", "/api/v1/pages/"+page+"/access-requests", map[string]any{"kind": "own"})
}

// fakeSender keeps what would have left.
type fakeSender struct {
	mu   sync.Mutex
	sent []mail.Message
	fail bool
}

func (f *fakeSender) Send(_ context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return context.DeadlineExceeded
	}
	f.sent = append(f.sent, m)
	return nil
}

func (e *env) mailer(f *fakeSender) *mail.Service {
	s := mail.NewService(e.app.Pool(), f, e.app.Logger())
	s.SetRenderer(e.app.Notify().Render)
	return s
}

func (e *env) dueNow() {
	e.t.Helper()
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE mail_queue SET next_attempt_at = now() - interval '1 second' WHERE status = 'queued'"); err != nil {
		e.t.Fatal(err)
	}
}

func TestMailIsWrittenWhenSentAndChecksAccessAgain(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	gone := e.user("gone@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	owner.grant(space, "user", gone.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Roadmap", "x")
	second := owner.page(space, nil, "Second", "x")
	owner.publish(page, "Roadmap", 1, commentBody("hi ", mention(anna.id, "Anna"), mention(gone.id, "Gone")))
	owner.publish(second, "Second", 1, commentBody("hi ", mention(anna.id, "Anna")))
	e.deliver()

	f := &fakeSender{}
	queue := e.mailer(f)
	if n, err := queue.RunOnce(context.Background()); err != nil || n != 0 {
		t.Fatalf("mail must wait a minute, so mentions become one mail: %d %v", n, err)
	}
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": gone.id}, "permissions": []string{}})
	e.dueNow()
	if _, err := queue.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0].To != "anna@example.com" {
		t.Fatalf("sent: %+v", f.sent)
	}
	m := f.sent[0]
	if !strings.Contains(m.Text, "Roadmap") || !strings.Contains(m.Text, "Second") {
		t.Fatalf("one mail for two mentions: %s", m.Text)
	}
	if !strings.Contains(m.Unsubscribe, "/api/v1/mail/unsubscribe/") {
		t.Fatalf("unsubscribe: %q", m.Unsubscribe)
	}
	if strings.Contains(m.Text, "what=watch") {
		t.Fatalf("a mention has nothing to stop watching: %s", m.Text)
	}
	var dropped int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM mail_queue WHERE status = 'dropped'").Scan(&dropped); err != nil || dropped != 1 {
		t.Fatalf("a mail for someone who lost access must drop: %d %v", dropped, err)
	}
}

func TestAMailAboutSomethingWatchedHasALinkToStopWatching(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Roadmap", "x")
	e.deliver()
	anna.ok("PUT", "/api/v1/spaces/"+space+"/watch", nil)
	anna.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)
	settings := anna.ok("GET", "/api/v1/me/notification-settings", nil)
	settings["channels"].(map[string]any)["page_changed"] = "app_mail"
	anna.ok("PUT", "/api/v1/me/notification-settings", settings)
	owner.publish(page, "Roadmap", 1, para("changed"))
	e.deliver()

	f := &fakeSender{}
	e.dueNow()
	if _, err := e.mailer(f).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 {
		t.Fatalf("sent: %+v", f.sent)
	}
	link := regexp.MustCompile(`/unsubscribe/([A-Za-z0-9_-]+)\?what=watch`).FindStringSubmatch(f.sent[0].Text)
	if link == nil || !strings.Contains(f.sent[0].HTML, "what=watch") {
		t.Fatalf("no link to stop watching:\n%s\n%s", f.sent[0].Text, f.sent[0].HTML)
	}

	// The link in the mail works without a session, as the one for all mail does.
	if resp, _ := e.do(e.client(), "POST", "/api/v1/mail/unsubscribe/"+link[1], ""); resp.StatusCode != 200 {
		t.Fatalf("post: %d", resp.StatusCode)
	}
	state := anna.ok("GET", "/api/v1/pages/"+page+"/watch", nil)
	if state["page"] != false || state["space"] != false {
		t.Fatalf("whatever brought this page must stop: %v", state)
	}
	if anna.ok("GET", "/api/v1/me/notification-settings", nil)["channels"].(map[string]any)["page_changed"] != "app_mail" {
		t.Fatal("stopping one watch must leave the settings alone")
	}
}

func TestMailRetriesThenGivesUpAndTheAdminSeesNoAddresses(t *testing.T) {
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := admin.space("ENG")
	admin.grant(space, "user", anna.id, permissions.TemplateViewer)
	page := admin.page(space, nil, "Page", "x")
	admin.publish(page, "Page", 1, commentBody("hi ", mention(anna.id, "Anna")))
	e.deliver()
	f := &fakeSender{fail: true}
	queue := e.mailer(f)
	for range 5 {
		e.dueNow()
		if _, err := queue.RunOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	body := admin.want(200, "GET", "/api/v1/admin/mail", nil)
	if !strings.Contains(body, `"status":"failed"`) || strings.Contains(body, "anna@example.com") {
		t.Fatalf("queue: %s", body)
	}
	anna.want(403, "GET", "/api/v1/admin/mail", nil)
	admin.want(409, "POST", "/api/v1/admin/mail/test", nil)
}

func TestPreferencesDecideWhenMailGoes(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)

	settings := anna.ok("GET", "/api/v1/me/notification-settings", nil)
	now := time.Now().UTC()
	settings["quiet_start"] = now.Add(-time.Hour).Format("15:04")
	settings["quiet_end"] = now.Add(time.Hour).Format("15:04")
	anna.ok("PUT", "/api/v1/me/notification-settings", settings)

	page := owner.page(space, nil, "Page", "x")
	owner.publish(page, "Page", 1, commentBody("hi ", mention(anna.id, "Anna")))
	e.deliver()
	f := &fakeSender{}
	queue := e.mailer(f)
	e.dueNow()
	if _, err := queue.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 0 {
		t.Fatal("quiet hours must hold mail")
	}
	var status string
	var later bool
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT status, next_attempt_at > now() FROM mail_queue").Scan(&status, &later); err != nil {
		t.Fatal(err)
	}
	if status != "queued" || !later {
		t.Fatalf("held mail: %s %v", status, later)
	}

	settings["quiet_start"], settings["quiet_end"] = "", ""
	settings["paused_until"] = now.AddDate(0, 0, 7).Format(time.DateOnly)
	anna.ok("PUT", "/api/v1/me/notification-settings", settings)
	e.dueNow()
	if _, err := queue.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 0 {
		t.Fatal("a pause must stop mail")
	}
}

func TestDigestGroupsNewThingsAndStaysAwayWhenThereAreNone(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Roadmap", "x")
	anna.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)

	settings := anna.ok("GET", "/api/v1/me/notification-settings", nil)
	settings["digest_hour"] = time.Now().UTC().Hour()
	settings["time_zone"] = "UTC"
	anna.ok("PUT", "/api/v1/me/notification-settings", settings)

	if err := e.app.Notify().SendDigests(context.Background()); err != nil {
		t.Fatal(err)
	}
	var queued int
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT count(*) FROM mail_queue").Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("nothing new, nothing queued: %d %v", queued, err)
	}
	owner.publish(page, "Roadmap", 1, para("changed"))
	e.deliver()
	if err := e.app.Notify().SendDigests(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.app.Notify().SendDigests(context.Background()); err != nil {
		t.Fatal(err)
	}
	f := &fakeSender{}
	if _, err := e.mailer(f).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || !strings.Contains(f.sent[0].Text, "Space ENG") || !strings.Contains(f.sent[0].Text, "Roadmap") {
		t.Fatalf("digest: %+v", f.sent)
	}
}

func TestUnsubscribeNeedsNoSessionAndCannotBeForged(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	owner.publish(page, "Page", 1, commentBody("hi ", mention(anna.id, "A")))
	e.deliver()
	f := &fakeSender{}
	e.dueNow()
	if _, err := e.mailer(f).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	address := f.sent[0].Unsubscribe
	path := address[strings.Index(address, "/api/"):]

	// A plain client without a cookie, as a mail provider's is.
	anon := e.client()
	if resp, _ := e.do(anon, "GET", path, ""); resp.StatusCode == 200 {
		t.Fatal("GET must never carry it out")
	}
	mentionChannel := func() any {
		return anna.ok("GET", "/api/v1/me/notification-settings", nil)["channels"].(map[string]any)["mention"]
	}
	if mentionChannel() != "app_mail" {
		t.Fatal("a GET changed something")
	}
	if resp, _ := e.do(anon, "POST", path[:len(path)-2]+"xx", "List-Unsubscribe=One-Click", "Content-Type", "application/x-www-form-urlencoded"); resp.StatusCode != 200 {
		t.Fatalf("a bad token gets the same plain answer: %d", resp.StatusCode)
	}
	if mentionChannel() != "app_mail" {
		t.Fatal("a tampered token worked")
	}
	forged := base64.RawURLEncoding.EncodeToString([]byte(`{"u":"` + anna.id + `","s":"all"}`))
	if resp, _ := e.do(anon, "POST", "/api/v1/mail/unsubscribe/"+forged, "List-Unsubscribe=One-Click", "Content-Type", "application/x-www-form-urlencoded"); resp.StatusCode != 200 {
		t.Fatalf("a forged token gets the same plain answer: %d", resp.StatusCode)
	}
	if mentionChannel() != "app_mail" {
		t.Fatal("a token made by hand worked")
	}
	if resp, _ := e.do(anon, "POST", path, "List-Unsubscribe=One-Click", "Content-Type", "application/x-www-form-urlencoded"); resp.StatusCode != 200 {
		t.Fatalf("post: %d", resp.StatusCode)
	}
	if mentionChannel() != "app" {
		t.Fatalf("mail did not stop: %v", mentionChannel())
	}
}

func TestAPageYouCommentedOnCountsAsWorkedOn(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Talked about", "x")
	workedOn := func() []string { return titlesIn(reader.ok("GET", "/api/v1/me/recent?kind=worked", nil), "pages") }
	if has(workedOn(), "Talked about") {
		t.Fatal("reading is not working on a page")
	}
	comment := reader.ok("POST", "/api/v1/pages/"+page+"/comments", map[string]any{"body": commentBody("hello")})["id"].(string)
	if !has(workedOn(), "Talked about") {
		t.Fatal("a comment must count")
	}
	reader.ok("DELETE", "/api/v1/comments/"+comment, nil)
	if has(workedOn(), "Talked about") {
		t.Fatal("a removed comment must not count")
	}
}

func feedTitles(u *user, query string) []string {
	u.e.t.Helper()
	return titlesIn(u.ok("GET", "/api/v1/feed?"+query, nil), "items")
}

func TestTheFeedShowsOnlyWhatAPersonMaySee(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, nil, "Open", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	owner.restrict(hidden, "view", owner.id)

	recent := feedTitles(reader, "tab=recent")
	if !has(recent, "Open") || has(recent, "Hidden") {
		t.Fatalf("recent: %v", recent)
	}
	if len(feedTitles(reader, "tab=for_you")) != 0 {
		t.Fatal("nothing is watched yet")
	}
	reader.ok("PUT", "/api/v1/spaces/"+space+"/watch", nil)
	if !has(feedTitles(reader, "tab=for_you"), "Open") {
		t.Fatal("a watched space must feed For you")
	}
	if len(feedTitles(owner, "tab=for_you")) != 0 {
		t.Fatal("own actions must stay out of For you")
	}

	other := e.user("other@example.com", identity.PermUseProduct)
	owner.grant(space, "user", other.id, permissions.TemplateEditor)
	followed := other.page(space, nil, "Followed", "x")
	reader.ok("DELETE", "/api/v1/spaces/"+space+"/watch", nil)
	reader.ok("PUT", "/api/v1/users/"+other.id+"/follow", nil)
	got := feedTitles(reader, "tab=for_you")
	if !has(got, "Followed") || has(got, "Open") {
		t.Fatalf("following: %v", got)
	}
	reader.want(400, "PUT", "/api/v1/users/"+reader.id+"/follow", nil)
	reader.want(404, "PUT", "/api/v1/users/"+space+"/follow", nil)

	owner.ok("POST", "/api/v1/pages/"+open+"/publish", map[string]any{"title": "Open", "base_version": 1, "silent": true, "document": para("small")})
	item := reader.ok("GET", "/api/v1/feed?tab=recent", nil)["items"].([]any)
	for _, row := range item {
		if m := row.(map[string]any); m["title"] == "Open" && m["kind"] != "created" {
			t.Fatalf("a silent version must not show: %v", m)
		}
	}

	if len(feedTitles(reader, "tab=popular")) != 0 {
		t.Fatal("no reactions yet")
	}
	reader.ok("PUT", "/api/v1/pages/"+followed+"/reactions/"+url.PathEscape("👍"), nil)
	owner.comment(hidden, "secret")
	popular := feedTitles(reader, "tab=popular")
	if !has(popular, "Followed") || has(popular, "Hidden") {
		t.Fatalf("popular: %v", popular)
	}
	reader.want(400, "GET", "/api/v1/feed?tab=secret", nil)
	reader.want(400, "GET", "/api/v1/feed?before=soon", nil)
}

func TestCandidatesMarkWhoCannotSeeThePage(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermViewProfiles)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	e.user("outsider@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	list := threads(owner.ok("GET", "/api/v1/pages/"+page+"/mention-candidates?q=example", nil), "candidates")
	if len(list) != 3 || list[0]["can_view"] != true || list[2]["can_view"] != false {
		t.Fatalf("candidates: %v", list)
	}
	// Without "view profiles", only people with a grant in this space.
	got := reader.want(200, "GET", "/api/v1/pages/"+page+"/mention-candidates?q=", nil)
	if strings.Contains(got, "outsider") || !strings.Contains(got, "owner@example.com") {
		t.Fatalf("no view profiles: %s", got)
	}
}

// The real path once: outbox, relay, JetStream, consumer, notification.
func TestEventsReachPeopleThroughTheRealQueue(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func(a *app.App) { done <- a.Serve(ctx, ln) }(e.app)
	t.Cleanup(func() {
		stop()
		<-done
	})

	owner.publish(page, "Page", 1, commentBody("hello ", mention(anna.id, "Anna")))
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if anna.ok("GET", "/api/v1/me/notifications/count", nil)["unread"].(float64) == 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the event never arrived")
}

// watchedPageMail publishes a change that a watcher hears about by mail right away.
func watchedPageMail(t *testing.T, excerpts bool, change func(owner, anna *user, page string)) (*env, *fakeSender) {
	t.Helper()
	e := newEnv(t)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	values := admin.ok("GET", "/api/v1/admin/settings", nil)
	values["mail_excerpts"] = excerpts
	admin.ok("PUT", "/api/v1/admin/settings", values)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", anna.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Roadmap", "the old plan")
	e.deliver()
	anna.ok("PUT", "/api/v1/pages/"+page+"/watch", nil)
	settings := anna.ok("GET", "/api/v1/me/notification-settings", nil)
	settings["channels"].(map[string]any)["page_changed"] = "app_mail"
	anna.ok("PUT", "/api/v1/me/notification-settings", settings)
	change(owner, anna, page)
	e.deliver()
	f := &fakeSender{}
	e.dueNow()
	if _, err := e.mailer(f).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e, f
}

func TestAMailAboutAChangeShowsTheChange(t *testing.T) {
	_, f := watchedPageMail(t, true, func(owner, _ *user, page string) {
		owner.publish(page, "Roadmap", 1, para(`the new <script>alert("x")</script> plan`))
	})
	if len(f.sent) != 1 {
		t.Fatalf("sent: %+v", f.sent)
	}
	m := f.sent[0]
	if !strings.Contains(m.Text, "[-old-]") || !strings.Contains(m.Text, "{+new") {
		t.Fatalf("no changes in the text: %s", m.Text)
	}
	if !strings.Contains(m.HTML, ">old</del>") || !strings.Contains(m.HTML, "<ins") {
		t.Fatalf("no changes in the HTML: %s", m.HTML)
	}
	if strings.Contains(m.HTML, "<script") || !strings.Contains(m.HTML, "&lt;script&gt;") {
		t.Fatalf("words from a page reached the mail unescaped: %s", m.HTML)
	}
}

func TestAMailShowsNoChangesWhenTheInstanceAllowsNoContent(t *testing.T) {
	_, f := watchedPageMail(t, false, func(owner, _ *user, page string) {
		owner.publish(page, "Roadmap", 1, para("the secret new plan"))
	})
	if len(f.sent) != 1 {
		t.Fatalf("sent: %+v", f.sent)
	}
	if m := f.sent[0]; strings.Contains(m.Text+m.HTML, "secret") || !strings.Contains(m.Text, "Roadmap") {
		t.Fatalf("content in a mail that may hold none:\n%s\n%s", m.Text, m.HTML)
	}
}

func TestAMailAboutAChangeGoesNowhereWhenThePageBecameHidden(t *testing.T) {
	_, f := watchedPageMail(t, true, func(owner, _ *user, page string) {
		owner.publish(page, "Roadmap", 1, para("the secret new plan"))
		owner.e.deliver()
		owner.restrict(page, "view", owner.id)
	})
	if len(f.sent) != 0 {
		t.Fatalf("a person who lost access got the changes: %+v", f.sent)
	}
}

func TestAMailNeverShowsAVersionThatWasRemoved(t *testing.T) {
	_, f := watchedPageMail(t, true, func(owner, _ *user, page string) {
		owner.publish(page, "Roadmap", 1, para("the safe plan"))
		admin := owner.e.user("second@example.com", identity.AllGlobalPermissions...)
		admin.ok("DELETE", "/api/v1/admin/pages/"+page+"/versions/1", map[string]any{"reason": "A leaked password"})
	})
	if len(f.sent) != 1 {
		t.Fatalf("sent: %+v", f.sent)
	}
	if m := f.sent[0]; strings.Contains(m.Text+m.HTML, "old") {
		t.Fatalf("words of a removed version reached a mail:\n%s\n%s", m.Text, m.HTML)
	}
}

func TestADigestShowsEverythingSinceTheOldestChange(t *testing.T) {
	_, f := watchedPageMail(t, true, func(owner, _ *user, page string) {
		owner.publish(page, "Roadmap", 1, para("the middle plan"))
		owner.publish(page, "Roadmap", 2, para("the final plan"))
	})
	if len(f.sent) != 1 {
		t.Fatalf("two changes must be one mail: %+v", f.sent)
	}
	if m := f.sent[0]; !strings.Contains(m.Text, "[-old-]") || !strings.Contains(m.Text, "{+final+}") {
		t.Fatalf("the mail must compare with the first version the person missed: %s", m.Text)
	}
}
