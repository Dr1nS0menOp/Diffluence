// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func fieldDocument(name string, required bool) map[string]any {
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": []any{
		map[string]any{"type": "text", "text": "Topic: "},
		map[string]any{"type": "templateField", "attrs": map[string]any{"name": name, "kind": "line", "required": required}},
		map[string]any{"type": "instruction", "attrs": map[string]any{"text": "Say what it is about"}}}}}}
}

func TestStarterTemplatesAreThereAndCanBeHidden(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	viewer.want(403, "GET", "/api/v1/spaces/"+space+"/templates", nil)
	list := editor.ok("GET", "/api/v1/spaces/"+space+"/templates", nil)["templates"].([]any)
	if len(list) != 10 {
		t.Fatalf("templates: %d", len(list))
	}
	if !has(titlesIn(map[string]any{"x": list}, "x"), "Meeting notes") {
		t.Fatalf("names: %v", list)
	}
	for _, tpl := range list {
		id := tpl.(map[string]any)["id"].(string)
		if got := editor.ok("GET", "/api/v1/templates/"+id, nil); got["document"] == nil {
			t.Fatalf("no text: %v", got)
		}
	}
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	owner.want(403, "GET", "/api/v1/admin/templates", nil)
	all := admin.ok("GET", "/api/v1/admin/templates", nil)["templates"].([]any)
	first := all[0].(map[string]any)
	id := first["id"].(string)
	update := map[string]any{"name": first["name"], "hidden": true}
	owner.want(403, "PATCH", "/api/v1/templates/"+id, update)
	admin.ok("PATCH", "/api/v1/templates/"+id, update)
	if got := editor.ok("GET", "/api/v1/spaces/"+space+"/templates", nil)["templates"].([]any); len(got) != 9 {
		t.Fatalf("hidden template still offered: %d", len(got))
	}
	// A template is never a page, and never found.
	if has(treeTitles(owner, space), "Meeting notes") {
		t.Fatal("a template in the tree")
	}
	var page string
	if err := e.app.Pool().QueryRow(context.Background(), "SELECT page_id::text FROM templates WHERE id = $1", id).Scan(&page); err != nil {
		t.Fatal(err)
	}
	admin.want(404, "GET", "/api/v1/pages/"+page, nil)
	if len(admin.ok("GET", "/api/v1/search?q=meeting", nil)["results"].([]any)) != 0 {
		t.Fatal("a template was found")
	}
	if strings.Contains(admin.want(200, "GET", "/api/v1/admin/spaces", nil), "__templates") {
		t.Fatal("the system space shows")
	}
}

func TestAPageFromATemplateGetsTextLabelsAndAnswers(t *testing.T) {
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	viewer.want(403, "POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": "x"})
	tpl := owner.ok("POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": "Note", "category": "Notes", "labels": []string{"from-template", "Not Fixed"},
		"title_pattern": "Note on {Topic}, {space}", "document": fieldDocument("Topic", true)})
	id := tpl["id"].(string)
	if fields := tpl["fields"].([]any); len(fields) != 1 || fields[0].(map[string]any)["required"] != true {
		t.Fatalf("fields: %v", tpl)
	}

	use := func(answers map[string]string) (int, string) {
		return editor.do("POST", "/api/v1/templates/"+id+"/use", map[string]any{"space_id": space, "answers": answers})
	}
	if status, out := use(map[string]string{}); status != 400 {
		t.Fatalf("a required field: %d %s", status, out)
	}
	if status, out := use(map[string]string{"Topic": strings.Repeat("x", 2001)}); status != 400 {
		t.Fatalf("a long answer: %d %s", status, out)
	}
	owner.ok("PUT", "/api/v1/spaces/"+space+"/labels", map[string]any{"fixed": true, "labels": []string{"from-template"}})
	status, out := use(map[string]string{"Topic": "<b>bold</b> [x](javascript:alert(1))"})
	if status != 201 {
		t.Fatalf("use: %d %s", status, out)
	}
	var made map[string]any
	if err := json.Unmarshal([]byte(out), &made); err != nil {
		t.Fatal(err)
	}
	if got := strs(made["labels_left_out"]); len(got) != 1 || got[0] != "not-fixed" {
		t.Fatalf("a label off the fixed list is reported: %v", made)
	}
	page := made["page"].(map[string]any)
	pageID := page["id"].(string)
	if page["status"] != "draft" || !strings.Contains(page["title"].(string), "Note on <b>bold</b>") || !strings.Contains(page["title"].(string), "Space ENG") {
		t.Fatalf("page: %v", page)
	}
	draft := editor.ok("GET", "/api/v1/pages/"+pageID+"/draft", nil)
	doc := mustJSON(t, draft["document"])
	if strings.Contains(doc, "templateField") || !strings.Contains(doc, "bold") {
		t.Fatalf("an answer is plain text: %s", doc)
	}
	if strings.Contains(doc, `"link"`) {
		t.Fatalf("an answer can carry no link: %s", doc)
	}
	if !strings.Contains(doc, "Say what it is about") {
		t.Fatalf("the instruction stays while writing: %s", doc)
	}
	if got := strs(editor.ok("GET", "/api/v1/pages/"+pageID, nil)["labels"]); len(got) != 1 {
		t.Fatalf("labels: %v", got)
	}

	// Later changes to the template change nothing.
	owner.ok("PATCH", "/api/v1/templates/"+id, map[string]any{"name": "Note", "document": para("Something else")})
	if got := mustJSON(t, editor.ok("GET", "/api/v1/pages/"+pageID+"/draft", nil)["document"]); strings.Contains(got, "Something else") {
		t.Fatal("a page followed its template")
	}
	if got := owner.ok("GET", "/api/v1/templates/"+id, nil)["version"].(float64); got != 2 {
		t.Fatalf("each save is a version: %v", got)
	}

	other, _ := owner.space("OTHER")
	owner.want(404, "POST", "/api/v1/templates/"+id+"/use", map[string]any{"space_id": other})
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	stranger.want(404, "GET", "/api/v1/templates/"+id, nil)
}

func TestAnImportWithAnUnknownBlockIsRefusedWhole(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	id := owner.ok("POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": "Exported", "document": para("words")})["id"].(string)
	bundle := owner.ok("GET", "/api/v1/templates/"+id+"/export", nil)
	if bundle["name"] != "Exported" {
		t.Fatalf("bundle: %v", bundle)
	}
	owner.ok("POST", "/api/v1/spaces/"+space+"/templates/import", bundle)
	bad := map[string]any{"name": "Bad", "document": map[string]any{"type": "doc", "content": []any{map[string]any{"type": "script"}}}}
	owner.want(400, "POST", "/api/v1/spaces/"+space+"/templates/import", bad)
	if got := owner.ok("GET", "/api/v1/spaces/"+space+"/manage-templates", nil)["templates"].([]any); len(got) != 2 {
		t.Fatalf("templates: %d", len(got))
	}
}

func TestSaveAPageAsATemplateAndFavouriteIt(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	page := owner.page(space, nil, "Good page", "worth repeating")
	editor.want(403, "POST", "/api/v1/pages/"+page+"/save-as-template", map[string]any{"name": "x"})
	tpl := owner.ok("POST", "/api/v1/pages/"+page+"/save-as-template", map[string]any{"name": "From a page"})
	id := tpl["id"].(string)
	editor.ok("PUT", "/api/v1/templates/"+id+"/favourite", nil)
	first := editor.ok("GET", "/api/v1/spaces/"+space+"/templates", nil)["templates"].([]any)[0].(map[string]any)
	if first["name"] != "From a page" || first["favourite"] != true {
		t.Fatalf("favourites come first: %v", first)
	}
	owner.ok("DELETE", "/api/v1/templates/"+id, nil)
	editor.want(404, "GET", "/api/v1/templates/"+id, nil)
}

func TestSpaceTemplatesStartAWorkingSpace(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	list := owner.ok("GET", "/api/v1/space-templates", nil)["templates"].([]any)
	if len(list) != 2 {
		t.Fatalf("starters: %v", list)
	}
	id := list[0].(map[string]any)["id"].(string)
	sp := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "STARTED", "name": "Started", "template_id": id})
	if got := treeTitles(owner, sp["id"].(string)); len(got) != 4 {
		t.Fatalf("pages: %v", got)
	}

	source, _ := owner.space("SRC")
	owner.page(source, nil, "Kept page", "x")
	owner.grant(source, "user", admin.id, permissions.TemplateViewer)
	owner.want(403, "POST", "/api/v1/admin/space-templates", map[string]any{"space_id": source, "name": "Mine"})
	saved := admin.ok("POST", "/api/v1/admin/space-templates", map[string]any{"space_id": source, "name": "Mine"})
	made := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "COPY", "name": "Copy", "template_id": saved["id"]})
	if !has(treeTitles(owner, made["id"].(string)), "Kept page") {
		t.Fatal("a saved space starts a new one")
	}
}

func TestNewSpacesFollowTheChosenPermissionSet(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	member := e.user("member@example.com", identity.PermUseProduct)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	private := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "PRIV", "name": "Private"})["id"].(string)
	member.want(404, "GET", "/api/v1/spaces/"+private, nil)
	open := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "OPEN", "name": "Open", "preset": "team_space"})["id"].(string)
	member.ok("GET", "/api/v1/spaces/"+open, nil)

	settings := admin.ok("GET", "/api/v1/admin/organising", nil)
	settings["default_preset"] = "read_only_kb"
	admin.ok("PUT", "/api/v1/admin/organising", settings)
	byDefault := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "DEF", "name": "Default"})["id"].(string)
	member.ok("GET", "/api/v1/spaces/"+byDefault, nil)
	stillPrivate := owner.ok("POST", "/api/v1/spaces", map[string]any{"key": "OWN", "name": "Own", "preset": "private"})["id"].(string)
	member.want(404, "GET", "/api/v1/spaces/"+stillPrivate, nil)
}

func TestCompanyHomeIsForThoseWhoMaySeeIt(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	admin := e.user("admin@example.com", identity.AllGlobalPermissions...)
	page := owner.page(space, nil, "Welcome", "x")
	if viewer.ok("GET", "/api/v1/start", nil)["page_id"] != nil {
		t.Fatal("nothing picked yet")
	}
	settings := admin.ok("GET", "/api/v1/admin/organising", nil)
	settings["company_home"] = page
	admin.ok("PUT", "/api/v1/admin/organising", settings)
	if viewer.ok("GET", "/api/v1/start", nil)["page_id"] != page {
		t.Fatal("no company home")
	}
	if stranger.ok("GET", "/api/v1/start", nil)["page_id"] != nil {
		t.Fatal("someone who may not see it gets the personal screen")
	}
}

func TestTrashCanBeSearchedAndWorkedOffTogether(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	parent := owner.page(space, nil, "Removed parent", "x")
	child := owner.page(space, parent, "Removed child", "x")
	other := editor.page(space, nil, "Another", "x")
	owner.ok("DELETE", "/api/v1/pages/"+child, nil)
	owner.ok("DELETE", "/api/v1/pages/"+parent, nil)
	editor.ok("DELETE", "/api/v1/pages/"+other, nil)

	items := owner.ok("GET", "/api/v1/spaces/"+space+"/trash?q=removed&sort=title", nil)["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["title"] != "Removed child" {
		t.Fatalf("items: %v", items)
	}
	if got := owner.ok("GET", "/api/v1/spaces/"+space+"/trash?q=another", nil)["items"].([]any)[0].(map[string]any)["deleted_by_name"]; got != "Test editor@example.com" {
		t.Fatalf("who: %v", got)
	}
	ids := []string{}
	for _, it := range items {
		ids = append(ids, it.(map[string]any)["id"].(string))
	}
	editor.want(403, "POST", "/api/v1/spaces/"+space+"/trash/bulk", map[string]any{"action": "restore", "ids": ids})
	got := owner.ok("POST", "/api/v1/spaces/"+space+"/trash/bulk", map[string]any{"action": "restore", "ids": ids})
	if got["done"].(float64) != 1 || len(got["failed"].([]any)) != 1 {
		t.Fatalf("a child waits for its parent, and the rest goes on: %v", got)
	}
}
