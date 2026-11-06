// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func linkTo(target, text string) map[string]any {
	return map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{map[string]any{
			"type": "text", "text": text,
			"marks": []any{map[string]any{"type": "link", "attrs": map[string]any{"href": "/pages/" + target}}},
		}}}}}
}

func (u *user) publish(page, title string, base int, doc map[string]any) {
	u.e.t.Helper()
	u.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": title, "document": doc, "base_version": base})
}

func titlesIn(m map[string]any, key string) []string {
	list, _ := m[key].([]any)
	out := []string{}
	for _, item := range list {
		if row, ok := item.(map[string]any); ok {
			if title, ok := row["title"].(string); ok {
				out = append(out, title)
			} else if name, ok := row["name"].(string); ok {
				out = append(out, name)
			}
		}
	}
	return out
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestPreferencesFollowThePersonAndRefuseUnknownValues(t *testing.T) {
	e := newEnv(t)
	anna := e.user("anna@example.com", identity.PermUseProduct)
	bob := e.user("bob@example.com", identity.PermUseProduct)

	got := anna.ok("GET", "/api/v1/me/preferences", nil)
	if got["theme"] != "system" || got["shortcuts"] != true || got["contents"] != true || got["hover_cards"] != true {
		t.Fatalf("defaults: %v", got)
	}
	anna.ok("PUT", "/api/v1/me/preferences", map[string]any{"theme": "dark", "shortcuts": false, "contents": true,
		"hover_cards": true, "sidebar_width": 20, "tree_view": "updated"})

	me := anna.ok("GET", "/api/v1/me", nil)
	if me["preferences"].(map[string]any)["theme"] != "dark" {
		t.Fatalf("me does not carry the preferences: %v", me)
	}
	if bob.ok("GET", "/api/v1/me/preferences", nil)["theme"] != "system" {
		t.Fatal("a preference leaked to someone else")
	}

	valid := func(key string, value any) map[string]any {
		body := map[string]any{"theme": "dark", "shortcuts": true, "contents": true, "hover_cards": true,
			"sidebar_width": 20, "tree_view": "tree"}
		body[key] = value
		return body
	}
	anna.ok("PUT", "/api/v1/me/preferences", valid("theme", "light"))
	for name, body := range map[string]map[string]any{
		"theme":     valid("theme", "neon"),
		"narrow":    valid("sidebar_width", 13),
		"wide":      valid("sidebar_width", 33),
		"tree view": valid("tree_view", "secret"),
		"unknown":   valid("something", true),
	} {
		if status, out := anna.do("PUT", "/api/v1/me/preferences", body); status != 400 {
			t.Errorf("%s: got %d: %s", name, status, out)
		}
	}
}

func TestFavouriteSpacesAndOwnOrderOfFavouritePages(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	first := owner.page(space, nil, "First", "x")
	second := owner.page(space, nil, "Second", "x")
	third := owner.page(space, nil, "Third", "x")
	for _, id := range []string{first, second, third} {
		owner.ok("PUT", "/api/v1/pages/"+id+"/favourite", nil)
	}
	owner.ok("PUT", "/api/v1/me/favourites/order", map[string]any{"page_ids": []string{third, first, second}})
	got := titlesIn(owner.ok("GET", "/api/v1/me/favourites", nil), "pages")
	if strings.Join(got, ",") != "Third,First,Second" {
		t.Fatalf("order: %v", got)
	}

	owner.ok("PUT", "/api/v1/spaces/"+space+"/favourite", nil)
	mine := owner.ok("GET", "/api/v1/me/spaces", nil)
	if !has(titlesIn(mine, "favourites"), "Space ENG") {
		t.Fatalf("favourite space missing: %v", mine)
	}
	if owner.ok("GET", "/api/v1/spaces/"+space, nil)["favourite"] != true {
		t.Fatal("space does not say it is a favourite")
	}
	owner.ok("DELETE", "/api/v1/spaces/"+space+"/favourite", nil)
	if has(titlesIn(owner.ok("GET", "/api/v1/me/spaces", nil), "favourites"), "Space ENG") {
		t.Fatal("removed favourite is still listed")
	}
}

func TestStarredSpaceDisappearsWhenAccessEnds(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	guest := e.user("guest@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", guest.id, permissions.TemplateViewer)
	guest.ok("PUT", "/api/v1/spaces/"+space+"/favourite", nil)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": guest.id}, "permissions": []string{}})
	if has(titlesIn(guest.ok("GET", "/api/v1/me/spaces", nil), "favourites"), "Space ENG") {
		t.Fatal("a space without access stayed in the list")
	}
}

func TestRecentListsVisitedWorkedOnAndDrafts(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateEditor)
	written := owner.page(space, nil, "Written", "x")

	reader.ok("GET", "/api/v1/pages/"+written, nil)
	if !has(titlesIn(reader.ok("GET", "/api/v1/me/recent", nil), "pages"), "Written") {
		t.Fatal("visited page missing")
	}
	if has(titlesIn(reader.ok("GET", "/api/v1/me/recent?kind=worked", nil), "pages"), "Written") {
		t.Fatal("reading is not working on a page")
	}
	if !has(titlesIn(owner.ok("GET", "/api/v1/me/recent?kind=worked", nil), "pages"), "Written") {
		t.Fatal("published page missing from worked on")
	}

	reader.ok("PUT", "/api/v1/pages/"+written+"/draft", map[string]any{"title": "Draft title", "document": para("x"), "base_version": 1})
	unpublished := reader.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Not yet"})["id"].(string)
	drafts := reader.ok("GET", "/api/v1/me/recent?kind=drafts", nil)
	titles := titlesIn(drafts, "pages")
	if !has(titles, "Draft title") || !has(titles, "Not yet") {
		t.Fatalf("drafts: %v", titles)
	}
	if has(titlesIn(owner.ok("GET", "/api/v1/me/recent?kind=drafts", nil), "pages"), "Not yet") {
		t.Fatal("someone else's draft is listed")
	}
	reader.ok("DELETE", "/api/v1/pages/"+unpublished, nil)
	if has(titlesIn(reader.ok("GET", "/api/v1/me/recent?kind=drafts", nil), "pages"), "Not yet") {
		t.Fatal("a deleted page stayed in drafts")
	}
	reader.want(400, "GET", "/api/v1/me/recent?kind=everything", nil)
}

func TestDirectoryCountsOnlyWhatThePersonCanSee(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "OLD", "name": "Old space"})
	old := owner.ok("GET", "/api/v1/space-keys/OLD", nil)["id"].(string)
	owner.ok("POST", "/api/v1/spaces/"+old+"/archive", nil)
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	owner.page(space, nil, "Open", "x")
	hidden := owner.page(space, nil, "Hidden", "x")
	owner.restrict(hidden, "view", owner.id)

	row := func(u *user, query string) map[string]any {
		t.Helper()
		list := u.ok("GET", "/api/v1/space-directory?"+query, nil)["spaces"].([]any)
		if len(list) != 1 {
			t.Fatalf("%s: %d rows", query, len(list))
		}
		return list[0].(map[string]any)
	}
	if got := row(owner, "q=ENG")["page_count"].(float64); got != 3 {
		t.Fatalf("owner sees %v pages", got)
	}
	got := row(reader, "q=eng")
	if got["page_count"].(float64) != 2 {
		t.Fatalf("a hidden page was counted: %v", got)
	}
	if admins := got["administrators"].([]any); len(admins) != 1 || admins[0] != "Test owner@example.com" {
		t.Fatalf("administrators: %v", admins)
	}
	if strings.Contains(stranger.want(200, "GET", "/api/v1/space-directory", nil), "ENG") {
		t.Fatal("a stranger sees a space")
	}

	all := owner.ok("GET", "/api/v1/space-directory", nil)
	if all["total"].(float64) != 1 {
		t.Fatalf("an archived space is listed by default: %v", all)
	}
	if row(owner, "kind=archived")["key"] != "OLD" {
		t.Fatal("the archived filter does not find it")
	}
	owner.want(400, "GET", "/api/v1/space-directory?sort=secret", nil)
	owner.want(400, "GET", "/api/v1/space-directory?kind=secret", nil)
}

func TestOverviewIsLimitedToVisiblePages(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	owner.page(space, nil, "Open page", "x")
	hidden := owner.page(space, nil, "Hidden page", "x")
	owner.restrict(hidden, "view", owner.id)

	got := reader.ok("GET", "/api/v1/spaces/"+space+"/overview", nil)
	if got["page_count"].(float64) != 2 {
		t.Fatalf("count: %v", got["page_count"])
	}
	titles := titlesIn(got, "recent")
	if !has(titles, "Open page") || has(titles, "Hidden page") {
		t.Fatalf("recent changes: %v", titles)
	}
}

func TestShortcutsAreSafeAndBelongToSpaceAdministrators(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, home := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)

	links := []map[string]any{
		{"label": "Runbook", "target": "/pages/" + home},
		{"label": "Status", "target": "https://status.example.com"},
	}
	owner.ok("PUT", "/api/v1/spaces/"+space+"/shortcuts", map[string]any{"shortcuts": links})
	got := reader.ok("GET", "/api/v1/spaces/"+space+"/shortcuts", nil)["shortcuts"].([]any)
	if len(got) != 2 || got[0].(map[string]any)["label"] != "Runbook" {
		t.Fatalf("shortcuts: %v", got)
	}
	reader.want(403, "PUT", "/api/v1/spaces/"+space+"/shortcuts", map[string]any{"shortcuts": links})

	for _, target := range []string{"javascript:alert(1)", "//evil.example.com", "data:text/html,x", "ftp://x.example.com"} {
		owner.want(400, "PUT", "/api/v1/spaces/"+space+"/shortcuts", map[string]any{
			"shortcuts": []map[string]any{{"label": "x", "target": target}}})
	}
	owner.want(400, "PUT", "/api/v1/spaces/"+space+"/shortcuts", map[string]any{
		"shortcuts": []map[string]any{{"label": "", "target": "/pages/" + home}}})
	tooMany := make([]map[string]any, 21)
	for i := range tooMany {
		tooMany[i] = map[string]any{"label": "x", "target": "/"}
	}
	owner.want(400, "PUT", "/api/v1/spaces/"+space+"/shortcuts", map[string]any{"shortcuts": tooMany})
	if e.auditActions("space.shortcuts_changed") != 1 {
		t.Fatal("a change of shortcuts must be audited, and a refusal must not")
	}
}

func TestPageLookIsCheckedAndAudited(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	viewer := e.user("viewer@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", viewer.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")
	other := owner.page(space, nil, "Other", "x")

	look := map[string]any{"emoji": "🌊", "full_width": true, "title_align": "center", "hide_children": true, "header_position": 30}
	owner.ok("PATCH", "/api/v1/pages/"+page+"/look", look)
	got := viewer.ok("GET", "/api/v1/pages/"+page, nil)
	if got["emoji"] != "🌊" || got["full_width"] != true || got["title_align"] != "center" || got["hide_children"] != true {
		t.Fatalf("look: %v", got)
	}
	if got["current_version"].(float64) != 1 {
		t.Fatal("the look of a page must not make a version")
	}
	if e.auditActions("page.look_changed") != 1 || e.outboxCount("page.look_changed") != 1 {
		t.Fatal("no audit or outbox event")
	}
	viewer.want(403, "PATCH", "/api/v1/pages/"+page+"/look", look)

	for name, bad := range map[string]map[string]any{
		"long emoji": {"emoji": "this is not an emoji", "title_align": "start", "header_position": 50},
		"markup":     {"emoji": "<b>", "title_align": "start", "header_position": 50},
		"align":      {"title_align": "justify", "header_position": 50},
		"position":   {"title_align": "start", "header_position": 101},
	} {
		if status, out := owner.do("PATCH", "/api/v1/pages/"+page+"/look", bad); status != 400 {
			t.Errorf("%s: got %d: %s", name, status, out)
		}
	}

	_, body := owner.upload(page, "head.png", pngHeader)
	var image map[string]any
	_ = json.Unmarshal([]byte(body), &image)
	_, body = owner.upload(page, "notes.txt", "plain text")
	var text map[string]any
	_ = json.Unmarshal([]byte(body), &text)
	_, body = owner.upload(other, "elsewhere.png", pngHeader)
	var elsewhere map[string]any
	_ = json.Unmarshal([]byte(body), &elsewhere)

	header := func(id any) map[string]any {
		return map[string]any{"title_align": "start", "header_position": 50, "header_attachment_id": id}
	}
	owner.ok("PATCH", "/api/v1/pages/"+page+"/look", header(image["id"]))
	if viewer.ok("GET", "/api/v1/pages/"+page, nil)["header_attachment_id"] != image["id"] {
		t.Fatal("header image not stored")
	}
	owner.want(400, "PATCH", "/api/v1/pages/"+page+"/look", header(text["id"]))
	owner.want(400, "PATCH", "/api/v1/pages/"+page+"/look", header(elsewhere["id"]))

	// A deleted file is no longer a header.
	owner.ok("DELETE", "/api/v1/attachments/"+image["id"].(string), nil)
	if viewer.ok("GET", "/api/v1/pages/"+page, nil)["header_attachment_id"] != nil {
		t.Fatal("the header stayed after the file was deleted")
	}
}

func TestDetailsNeverRevealHiddenPages(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	target := owner.page(space, nil, "Target", "x")
	open := owner.page(space, nil, "Open source", "x")
	hidden := owner.page(space, nil, "Hidden source", "x")
	child := owner.page(space, target, "Child", "x")
	hiddenChild := owner.page(space, target, "Hidden child", "x")
	owner.publish(open, "Open source", 1, linkTo(target, "see target"))
	owner.publish(hidden, "Hidden source", 1, linkTo(target, "see target"))
	owner.publish(target, "Target", 1, linkTo(hidden, "see hidden"))
	owner.restrict(hidden, "view", owner.id)
	owner.restrict(hiddenChild, "view", owner.id)

	got := reader.ok("GET", "/api/v1/pages/"+target+"/details", nil)
	incoming := titlesIn(got, "incoming")
	if !has(incoming, "Open source") || has(incoming, "Hidden source") {
		t.Fatalf("incoming: %v", incoming)
	}
	if outgoing := titlesIn(got, "outgoing"); len(outgoing) != 0 {
		t.Fatalf("outgoing shows a hidden page: %v", outgoing)
	}
	children := titlesIn(got, "children")
	if !has(children, "Child") || has(children, "Hidden child") {
		t.Fatalf("children: %v", children)
	}
	contributors := got["contributors"].([]any)
	if len(contributors) != 1 || contributors[0].(map[string]any)["versions"].(float64) != 2 {
		t.Fatalf("contributors: %v", contributors)
	}
	if !has(titlesIn(owner.ok("GET", "/api/v1/pages/"+target+"/details", nil), "outgoing"), "Hidden source") {
		t.Fatal("the owner should see the link")
	}
	_ = child
}

func TestLinksFollowTheCurrentVersion(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	target := owner.page(space, nil, "Target", "x")
	source := owner.page(space, nil, "Source", "x")
	incoming := func() []string {
		return titlesIn(owner.ok("GET", "/api/v1/pages/"+target+"/details", nil), "incoming")
	}

	owner.publish(source, "Source", 1, linkTo(target+"#heading", "with a heading"))
	if !has(incoming(), "Source") {
		t.Fatal("a link with a heading must count")
	}
	owner.publish(source, "Source", 2, para("no link now"))
	if len(incoming()) != 0 {
		t.Fatal("a removed link stayed")
	}
	owner.ok("POST", "/api/v1/pages/"+source+"/versions/2/restore", nil)
	if !has(incoming(), "Source") {
		t.Fatal("restoring a version must bring its links back")
	}
	owner.ok("DELETE", "/api/v1/pages/"+source, nil)
	if len(incoming()) != 0 {
		t.Fatal("a page in the trash still counts")
	}
}

func TestImpactNamesWhatADeleteTouches(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	parent := editor.page(space, nil, "Parent", "x")
	editor.page(space, parent, "Child", "x")
	linker := owner.page(space, nil, "Linker", "x")
	secret := owner.page(space, nil, "Secret linker", "x")
	owner.publish(linker, "Linker", 1, linkTo(parent, "x"))
	owner.publish(secret, "Secret linker", 1, linkTo(parent, "x"))
	owner.restrict(secret, "view", owner.id)

	got := editor.ok("GET", "/api/v1/pages/"+parent+"/impact", nil)
	if got["pages"].(float64) != 2 {
		t.Fatalf("pages: %v", got)
	}
	links := titlesIn(got, "incoming")
	if !has(links, "Linker") || has(links, "Secret linker") {
		t.Fatalf("incoming: %v", links)
	}
}

func TestUndoOfADeleteIsForTheOnePersonWhoDeleted(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("editor@example.com", identity.PermUseProduct)
	other := e.user("other@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", editor.id, permissions.TemplateEditor)
	owner.grant(space, "user", other.id, permissions.TemplateEditor)

	page := editor.page(space, nil, "Mine", "x")
	child := editor.page(space, page, "Below", "x")
	editor.ok("DELETE", "/api/v1/pages/"+page, nil)
	other.want(404, "POST", "/api/v1/pages/"+page+"/undo-delete", nil)
	owner.want(404, "POST", "/api/v1/pages/"+page+"/undo-delete", nil)
	editor.ok("POST", "/api/v1/pages/"+page+"/undo-delete", nil)
	if editor.ok("GET", "/api/v1/pages/"+child, nil)["status"] != "published" {
		t.Fatal("the page below did not come back")
	}
	if e.auditActions("page.restored") != 1 {
		t.Fatal("an undo must be audited as a restore")
	}
	editor.want(404, "POST", "/api/v1/pages/"+page+"/undo-delete", nil)

	editor.ok("DELETE", "/api/v1/pages/"+page, nil)
	ctx := e.t.Context()
	if _, err := e.app.Pool().Exec(ctx, "UPDATE trash_items SET deleted_at = now() - interval '61 seconds'"); err != nil {
		t.Fatal(err)
	}
	editor.want(404, "POST", "/api/v1/pages/"+page+"/undo-delete", nil)
}

func TestQuickSearchShowsGroupsWithoutRevealingAnything(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermViewProfiles)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("MARS")
	old, _ := owner.space("MARSOLD")
	owner.page(old, nil, "Mars archived", "x")
	owner.grant(old, "user", reader.id, permissions.TemplateViewer)
	owner.ok("POST", "/api/v1/spaces/"+old+"/archive", nil)
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, nil, "Mars plan", "x")
	hidden := owner.page(space, nil, "Mars secret", "x")
	owner.restrict(hidden, "view", owner.id)
	owner.upload(open, "mars-map.txt", "text")
	owner.upload(hidden, "mars-hidden.txt", "text")

	got := reader.ok("GET", "/api/v1/search/quick?q=mars", nil)
	if pages := titlesIn(got, "pages"); !has(pages, "Mars plan") || has(pages, "Mars secret") || has(pages, "Mars archived") {
		t.Fatalf("pages: %v", pages)
	}
	spaces := got["spaces"].([]any)
	if len(spaces) != 1 || spaces[0].(map[string]any)["key"] != "MARS" {
		t.Fatalf("spaces: %v", spaces)
	}
	files := got["files"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["filename"] != "mars-map.txt" {
		t.Fatalf("files: %v", files)
	}
	if people := reader.ok("GET", "/api/v1/search/quick?q=owner", nil)["people"].([]any); len(people) != 0 {
		t.Fatalf("a person without view profiles got people: %v", people)
	}
	people := owner.ok("GET", "/api/v1/search/quick?q=reader", nil)["people"].([]any)
	if len(people) != 1 {
		t.Fatalf("people: %v", people)
	}
	if person := people[0].(map[string]any); person["name"] != "Test reader@example.com" || len(person) != 2 {
		t.Fatalf("quick search must only give a name and an id: %v", person)
	}

	other := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "OTHER", "name": "Other"})["id"].(string)
	inOther := owner.ok("GET", "/api/v1/search/quick?q=mars&space_id="+other, nil)
	if len(titlesIn(inOther, "pages")) != 0 {
		t.Fatal("the chip did not limit the search")
	}
	owner.want(400, "GET", "/api/v1/search/quick?q=mars&space_id=nonsense", nil)
}

func TestTreeRowsTellWhatTheRowShows(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateEditor)
	page := owner.page(space, nil, "Row", "x")
	owner.restrict(page, "edit", owner.id)
	owner.ok("PATCH", "/api/v1/pages/"+page+"/look", map[string]any{"emoji": "🌊", "title_align": "start", "header_position": 50})
	reader.ok("GET", "/api/v1/pages/"+page, nil)

	find := func(u *user) map[string]any {
		t.Helper()
		for _, n := range u.ok("GET", "/api/v1/spaces/"+space+"/tree", nil)["pages"].([]any) {
			if row := n.(map[string]any); row["title"] == "Row" {
				return row
			}
		}
		t.Fatal("row not found")
		return nil
	}
	got := find(reader)
	if got["emoji"] != "🌊" || got["restricted"] != true || got["updated_by_name"] != "Test owner@example.com" {
		t.Fatalf("row: %v", got)
	}
	if got["visited_at"] == nil {
		t.Fatal("the row should know when this person visited it")
	}
	if find(owner)["visited_at"] != nil {
		t.Fatal("a visit by someone else must not show")
	}
	owner.ok("PUT", "/api/v1/pages/"+page+"/draft", map[string]any{"title": "Row", "document": para("x"), "base_version": 1})
	if find(owner)["has_draft"] != true || find(reader)["has_draft"] != false {
		t.Fatal("a draft mark is personal")
	}
}

func TestOpeningARowMenuIsNotAVisit(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, _ := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Row", "x")

	got := reader.ok("GET", "/api/v1/pages/"+page+"/summary", nil)
	if got["title"] != "Row" || got["can"].(map[string]any)["page.edit"] != false {
		t.Fatalf("summary: %v", got)
	}
	if has(titlesIn(reader.ok("GET", "/api/v1/me/recent", nil), "pages"), "Row") {
		t.Fatal("a menu must not count as reading the page")
	}
}

func TestArchivedSpacesLeaveTheStartScreenList(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	owner.ok("POST", "/api/v1/spaces/"+space+"/archive", nil)
	if strings.Contains(owner.want(200, "GET", "/api/v1/spaces", nil), "ENG") {
		t.Fatal("an archived space is still listed")
	}
	if owner.ok("GET", "/api/v1/spaces/"+space, nil)["status"] != "archived" {
		t.Fatal("a direct address must keep working")
	}
}

func TestKeyCheckTellsWhetherAKeyIsFree(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	owner.space("ENG")
	if owner.ok("GET", "/api/v1/space-key-check?key=eng", nil)["free"] != false {
		t.Fatal("a taken key must not be free, whatever its case")
	}
	if owner.ok("GET", "/api/v1/space-key-check?key=NEW", nil)["free"] != true {
		t.Fatal("a free key")
	}
	reader.want(403, "GET", "/api/v1/space-key-check?key=eng", nil)
}

func TestASpaceCanPickAnAnimalByName(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	owner.ok("PATCH", "/api/v1/spaces/"+space, map[string]any{"name": "Space ENG", "icon": "crocodile"})
	if owner.ok("GET", "/api/v1/spaces/"+space, nil)["icon"] != "crocodile" {
		t.Fatal("the animal was not kept")
	}
	owner.want(400, "PATCH", "/api/v1/spaces/"+space, map[string]any{"name": "Space ENG", "icon": "much too long for an icon"})
	owner.want(400, "PATCH", "/api/v1/spaces/"+space, map[string]any{"name": "Space ENG", "icon": "<b>"})
}

func TestAComparisonCarriesTheBlocks(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, _ := owner.space("ENG")
	page := owner.page(space, nil, "Page", "the old words")
	owner.publish(page, "Page", 1, para("the new words"))
	blocks := owner.ok("GET", "/api/v1/pages/"+page+"/compare?from=1&to=2", nil)["blocks"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("blocks: %v", blocks)
	}
	block := blocks[0].(map[string]any)
	if block["op"] != "changed" || block["node"].(map[string]any)["type"] != "paragraph" || block["previous"] == nil {
		t.Fatalf("block: %v", block)
	}
}
