// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func trashList(u *user, space, query string) []map[string]any {
	u.e.t.Helper()
	out := []map[string]any{}
	for _, it := range u.ok("GET", "/api/v1/spaces/"+space+"/trash"+query, nil)["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func fileID(t *testing.T, status int, body string) string {
	t.Helper()
	if status != 201 {
		t.Fatalf("upload: %d %s", status, body)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out.ID
}

func TestTrashNeverNamesWhatIsHiddenFromThePerson(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	secret := editor.page(space, nil, "Salary review", "x")
	status, body := editor.upload(secret, "salaries.txt", "numbers")
	file := fileID(t, status, body)
	e.hideFromAllBut(secret, editor)
	own := owner.page(space, nil, "Zebra notes", "x")
	e.hideFromAllBut(own, owner)
	editor.ok("DELETE", "/api/v1/attachments/"+file, nil)
	editor.ok("DELETE", "/api/v1/pages/"+secret, nil)
	owner.ok("DELETE", "/api/v1/pages/"+own, nil)

	raw := owner.want(200, "GET", "/api/v1/spaces/"+space+"/trash?sort=title", nil)
	if strings.Contains(raw, "Salary") || strings.Contains(raw, "salaries") {
		t.Fatalf("a hidden title reached a space administrator: %s", raw)
	}
	items := trashList(owner, space, "?sort=title")
	if len(items) != 3 {
		t.Fatalf("items: %v", items)
	}
	hidden := []string{}
	for _, it := range items {
		if it["hidden"] == true {
			hidden = append(hidden, it["id"].(string))
		}
	}
	if len(hidden) != 2 {
		t.Fatalf("a page and a file on it are restricted: %v", items)
	}
	if items[2]["title"] != "Zebra notes" || items[2]["hidden"] != false {
		t.Fatalf("what the person may see stays, and sorting tells nothing about the rest: %v", items)
	}
	if got := trashList(owner, space, "?q=salar"); len(got) != 0 {
		t.Fatalf("searching must not find a hidden title: %v", got)
	}
	if got := trashList(owner, space, "?q=zebra"); len(got) != 1 {
		t.Fatalf("search: %v", got)
	}

	for _, id := range hidden {
		owner.want(409, "DELETE", "/api/v1/trash/"+id, nil)
	}
	failed := owner.ok("POST", "/api/v1/spaces/"+space+"/trash/bulk", map[string]any{"action": "purge", "ids": hidden})
	if failed["done"].(float64) != 0 || len(failed["failed"].([]any)) != 2 {
		t.Fatalf("a bulk purge is a purge: %v", failed)
	}

	ctx := context.Background()
	if _, err := e.app.Pool().Exec(ctx, "UPDATE trash_items SET purge_after = now() - interval '1 second' WHERE id = $1", hidden[1]); err != nil {
		t.Fatal(err)
	}
	if n, err := e.app.Trash().PurgeExpired(ctx); n != 1 || err != nil {
		t.Fatalf("retention goes on regardless: %d %v", n, err)
	}
	// Newest first: the page was deleted after its file.
	owner.ok("POST", "/api/v1/trash/"+hidden[0]+"/restore", nil)
}

func TestPurgingAPageIsRefusedWhenAPageBelowItIsHidden(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	parent := editor.page(space, nil, "Open parent", "x")
	child := editor.page(space, parent, "Hidden child", "x")
	e.hideFromAllBut(child, editor)
	editor.ok("DELETE", "/api/v1/pages/"+parent, nil)
	var item string
	for _, it := range trashList(owner, space, "") {
		if it["title"] == "Open parent" {
			item = it["id"].(string)
		}
	}
	if item == "" {
		t.Fatalf("items: %v", trashList(owner, space, ""))
	}
	owner.want(409, "DELETE", "/api/v1/trash/"+item, nil)
}

func TestACopyCannotQuietlyLoseAnInheritedRestriction(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	target, _ := owner.space("DST")
	owner.grant(target, "user", editor.id, permissions.TemplateEditor)
	parent := owner.page(space, nil, "Restricted parent", "x")
	child := owner.page(space, parent, "Child", "x")
	e.hideFromAllBut(parent, editor)
	e.hideFromAllBut(parent, owner)

	beside := editor.ok("GET", "/api/v1/pages/"+child+"/copy-impact?parent_id="+parent, nil)
	if len(beside["restrictions_lost"].([]any)) != 0 || beside["needs_manage"] != false {
		t.Fatalf("beside itself nothing is lost: %v", beside)
	}
	editor.ok("POST", "/api/v1/pages/"+child+"/copy", map[string]any{"parent_id": parent})

	impact := editor.ok("GET", "/api/v1/pages/"+child+"/copy-impact", nil)
	if got := impact["restrictions_lost"].([]any); len(got) != 1 || got[0] != "Restricted parent" || impact["needs_manage"] != true {
		t.Fatalf("impact: %v", impact)
	}
	editor.want(403, "POST", "/api/v1/pages/"+child+"/copy", map[string]any{"parent_id": nil})
	editor.want(403, "POST", "/api/v1/pages/"+child+"/copy", map[string]any{"parent_id": nil, "keep_restrictions": true})
	editor.want(403, "POST", "/api/v1/pages/"+child+"/copy", map[string]any{"space_id": target})

	if got := owner.ok("GET", "/api/v1/pages/"+child+"/copy-impact?space_id="+target, nil); got["needs_manage"] != false {
		t.Fatalf("the right to manage restrictions is enough: %v", got)
	}
	copied := owner.ok("POST", "/api/v1/pages/"+child+"/copy", map[string]any{"space_id": target})
	if copied["space_id"] != target {
		t.Fatalf("copy: %v", copied)
	}
}

func TestALargeCopyRunsInSmallStepsAndLeavesHiddenPagesBehind(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	top := owner.page(space, nil, "Big branch", "x")
	for i := range 51 {
		owner.page(space, top, fmt.Sprintf("Part %d", i), "x")
	}
	hidden := editor.page(space, top, "Hidden part", "x")
	e.hideFromAllBut(hidden, editor)

	job := owner.ok("POST", "/api/v1/pages/"+top+"/copy", map[string]any{
		"parent_id": nil, "include_children": true, "prefix": "New "})["job_id"].(string)
	result := e.runJob(owner, job)
	if result["done"].(float64) != 52 || len(result["failed"].([]any)) != 0 || result["page_id"] == nil {
		t.Fatalf("report: %v", result)
	}
	got := treeTitles(owner, space)
	if !has(got, "New Big branch") || !has(got, "New Part 50") || has(got, "New Hidden part") {
		t.Fatalf("tree: %v", got)
	}

	// Each transaction leaves its own xmin, so this counts how the work was cut up.
	var steps int
	if err := e.app.Pool().QueryRow(context.Background(),
		"SELECT count(DISTINCT xmin::text) FROM pages WHERE title LIKE 'New %'").Scan(&steps); err != nil {
		t.Fatal(err)
	}
	if steps < 52 {
		t.Fatalf("a large branch must not be one transaction: %d", steps)
	}
}

func TestTemplateFilesFollowTheRuleOfTheTemplate(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	page := owner.page(space, nil, "With a picture", "x")
	status, body := owner.upload(page, "dot.png", pngHeader)
	pageFile := fileID(t, status, body)
	id := owner.ok("POST", "/api/v1/pages/"+page+"/save-as-template", map[string]any{"name": "Pictured"})["id"].(string)

	var file string
	if err := e.app.Pool().QueryRow(context.Background(), `SELECT a.id FROM attachments a
		JOIN templates t ON t.page_id = a.page_id WHERE t.id = $1`, id).Scan(&file); err != nil {
		t.Fatal(err)
	}
	address := "/api/v1/templates/" + id + "/files/"

	owner.want(404, "GET", "/api/v1/attachments/"+file+"/content", nil)
	viewer.want(200, "GET", address+file, nil)
	if got, _ := stranger.do("GET", address+file, nil); got == 200 {
		t.Fatal("a template of a space follows that space")
	}
	owner.want(404, "GET", address+pageFile, nil)
	owner.want(404, "GET", "/api/v1/templates/"+id+"/files/00000000-0000-0000-0000-000000000000", nil)
}

func TestTheApprovalRuleOfASpaceCanBeReadBack(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	editor.want(403, "GET", "/api/v1/spaces/"+space+"/approvals", nil)
	owner.ok("PUT", "/api/v1/spaces/"+space+"/approvals", map[string]any{"required": true, "approvers": []string{editor.id}})
	rule := owner.ok("GET", "/api/v1/spaces/"+space+"/approvals", nil)
	people := rule["approvers"].([]any)
	if rule["required"] != true || len(people) != 1 || people[0].(map[string]any)["id"] != editor.id {
		t.Fatalf("rule: %v", rule)
	}
}

func TestABrokenLinkFilterTreatsHiddenPagesAsGone(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	hidden := editor.page(space, nil, "Hidden target", "x")
	gone := owner.page(space, nil, "Gone target", "x")
	fine := owner.page(space, nil, "Fine target", "x")
	publishLink := func(title, target string) {
		page := owner.page(space, nil, title, "x")
		owner.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": title, "document": linkTo(target, "see"), "base_version": 1})
	}
	publishLink("Links to hidden", hidden)
	publishLink("Links to gone", gone)
	publishLink("Links to fine", fine)
	e.hideFromAllBut(hidden, editor)
	owner.ok("DELETE", "/api/v1/pages/"+gone, nil)

	got := titlesIn(owner.ok("GET", "/api/v1/spaces/"+space+"/content?broken_links=1", nil), "pages")
	if len(got) != 2 || !has(got, "Links to hidden") || !has(got, "Links to gone") {
		t.Fatalf("hidden and gone must look the same: %v", got)
	}
}
