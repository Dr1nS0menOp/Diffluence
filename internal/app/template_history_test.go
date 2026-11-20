// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Spec 074, wave GT5: where pages came from, what a version made, search, automation,
// restore, and turning the part off and on. The refusals come first.

// publishedSet makes a template of two pages that publishes at once, and a set of it.
func publishedSet(tt templateTeam, name string) (tpl, root, child string) {
	tt.e.t.Helper()
	tpl = tt.owner.guided(tt.space, name, "Root text")
	tt.owner.entry(tpl, map[string]any{"name": "Child", "parent": "root", "document": para("Child text")})
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	status, made := tt.owner.makeSet(tpl, map[string]any{"space_id": tt.space, "title": name + " one"})
	if status != http.StatusCreated {
		tt.e.t.Fatalf("a set: %d %v", status, made)
	}
	root = made["page"].(map[string]any)["id"].(string)
	child = pageOfEntry(tt.e.t, made, "Child")
	return tpl, root, child
}

func TestTheOriginNamesTheTemplateOnlyToWhoMaySeeIt(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl, root, child := publishedSet(tt, "Incident review")
	// A reader off the users list sees that the page was made from a template, and no name
	// (TPL-611, TPL-504).
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"use": []any{userSubject(tt.colleague)}})
	hidden := tt.viewer.ok("GET", "/api/v1/pages/"+root+"/template-origin", nil)
	if hidden["made"] != true || hidden["named"] != false || hidden["template_id"] != nil || hidden["version"] != nil ||
		strings.Contains(mustJSON(t, hidden), "Incident review") {
		t.Fatalf("a reader who may not see the template: %v", hidden)
	}
	if set := hidden["set"].([]any); len(set) != 1 || set[0].(map[string]any)["id"] != child {
		t.Fatalf("the other pages of the set: %v", hidden["set"])
	}
	// One who may see it reads its name, version and date, and may open its history only if
	// they look after it.
	seen := tt.colleague.ok("GET", "/api/v1/pages/"+root+"/template-origin", nil)
	if seen["named"] != true || seen["name"] != "Incident review" || seen["version"].(float64) != 3 || seen["history"] != false {
		t.Fatalf("a reader on the list: %v", seen)
	}
	if got := tt.owner.ok("GET", "/api/v1/pages/"+root+"/template-origin", nil); got["history"] != true {
		t.Fatalf("the manager: %v", got)
	}
	// A page the reader may not see is never listed as part of the set.
	tt.owner.restrict(child, "view", tt.owner.id)
	if set := tt.colleague.ok("GET", "/api/v1/pages/"+root+"/template-origin", nil)["set"].([]any); len(set) != 0 {
		t.Fatalf("a hidden page of the set was listed: %v", set)
	}
	// Whoever may not see the page learns nothing of its origin.
	stranger := tt.e.user("stranger@example.com", identity.PermUseProduct)
	stranger.want(http.StatusNotFound, "GET", "/api/v1/pages/"+root+"/template-origin", nil)
	// A page made without a template has none.
	plain := tt.owner.page(tt.space, nil, "By hand", "x")
	if got := tt.owner.ok("GET", "/api/v1/pages/"+plain+"/template-origin", nil); got["made"] != false {
		t.Fatalf("a page made by hand: %v", got)
	}
}

func TestANewVersionChangesNoPageAndDetailsSayItExists(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl, root, _ := publishedSet(tt, "Checklist")
	before := tt.owner.ok("GET", "/api/v1/pages/"+root, nil)
	tt.owner.ok("PATCH", "/api/v1/templates/"+tpl+"/entries/root", map[string]any{"name": "", "kind": "page", "parent": "",
		"title_pattern": "", "labels": []any{}, "restrictions": map[string]any{"view": []any{}, "edit": []any{}}, "document": para("New root text")})
	after := tt.owner.ok("GET", "/api/v1/pages/"+root, nil)
	if after["current_version"] != before["current_version"] || mustJSON(t, after["document"]) != mustJSON(t, before["document"]) {
		t.Fatal("a page followed its template (TPL-612)")
	}
	// Who may edit the page is told a newer version exists; a reader is not.
	if got := tt.owner.ok("GET", "/api/v1/pages/"+root+"/template-origin", nil); got["newer"] != true {
		t.Fatalf("an editor: %v", got)
	}
	if got := tt.viewer.ok("GET", "/api/v1/pages/"+root+"/template-origin", nil); got["newer"] != false {
		t.Fatalf("a reader: %v", got)
	}
}

func TestTheSetsOfAVersionNeverShowAPageTheManagerCannotSee(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl, root, _ := publishedSet(tt, "Sprint")
	// The editor makes a set as drafts of their own, which the manager may not see.
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": false})
	if status, out := tt.editor.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Mine"}); status != http.StatusCreated {
		t.Fatalf("the editor's set: %d %v", status, out)
	}
	history := tt.owner.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)["versions"].([]any)
	counts := map[float64]float64{}
	for _, v := range history {
		row := v.(map[string]any)
		if n, ok := row["sets"].(float64); ok {
			counts[row["number"].(float64)] = n
		}
	}
	if counts[3] != 1 || counts[4] != 1 {
		t.Fatalf("the sets per version: %v", counts)
	}
	sets := tt.owner.ok("GET", "/api/v1/templates/"+tpl+"/versions/4/sets", nil)
	if sets["count"].(float64) != 1 || len(sets["sets"].([]any)) != 0 || sets["hidden"].(float64) != 1 ||
		strings.Contains(mustJSON(t, sets), "Mine") {
		t.Fatalf("a set the manager may not see: %v", sets)
	}
	listed := tt.owner.ok("GET", "/api/v1/templates/"+tpl+"/versions/3/sets", nil)["sets"].([]any)
	if len(listed) != 1 || listed[0].(map[string]any)["root"].(map[string]any)["id"] != root {
		t.Fatalf("the sets of version 3: %v", listed)
	}
	// Only who manages the template lists or counts them; one who uses it is refused, and
	// one who may not know it is told it does not exist.
	tt.editor.want(http.StatusForbidden, "GET", "/api/v1/templates/"+tpl+"/versions/3/sets", nil)
	stranger := tt.e.user("stranger@example.com", identity.PermUseProduct)
	stranger.want(http.StatusNotFound, "GET", "/api/v1/templates/"+tpl+"/versions/3/sets", nil)
	for _, v := range tt.colleague.ok("GET", "/api/v1/spaces/"+tt.space+"/templates", nil)["templates"].([]any) {
		if strings.Contains(mustJSON(t, v), `"sets"`) {
			t.Fatalf("the gallery counts sets: %v", v)
		}
	}
}

func TestTheChangeListOfAVersionIsReadableAndAudited(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	tpl := tt.owner.guided(tt.space, "Onboarding", "Welcome")
	tt.owner.entry(tpl, map[string]any{"name": "Equipment", "parent": "root", "document": para("A laptop")})
	changes := tt.owner.ok("GET", "/api/v1/templates/"+tpl+"/versions/2/changes", nil)
	list := changes["list"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["code"] != "entry_added" || list[0].(map[string]any)["entry"] != "Equipment" {
		t.Fatalf("the change list: %v", changes)
	}
	events := root.want(http.StatusOK, "GET", "/api/v1/admin/audit-events?action=template.version_saved", nil)
	if !strings.Contains(events, "entry_added: Equipment") {
		t.Fatalf("the audit keeps the change list: %s", events)
	}
}

func TestARuleUsesTheApprovedVersionAndSaysWhyItMadeNothing(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Weekly report", "Version one")
	page := tt.owner.page(tt.space, nil, "Start", "x")
	rule := tt.owner.rule(tt.space, "Report", map[string]any{"trigger": map[string]any{"kind": "manual"},
		"actions": []map[string]any{{"type": "new_from_template", "target_id": tpl, "title": "From the rule"}}})
	run := func() map[string]any {
		t.Helper()
		tt.owner.want(http.StatusAccepted, "POST", "/api/v1/rules/"+rule+"/run", map[string]any{"page_id": page})
		tt.e.settle()
		return tt.owner.ok("GET", "/api/v1/rules/"+rule+"/runs", nil)["runs"].([]any)[0].(map[string]any)
	}
	// With four eyes on, a manager's change waits; the rule uses the approved version (TPL-604).
	tt.owner.want(http.StatusOK, "PUT", "/api/v1/spaces/"+tt.space+"/template-review", map[string]any{"on": true})
	tt.manager.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Weekly report", "document": para("Version two")})
	if got := run(); got["state"] != "done" {
		t.Fatalf("the run: %v", got)
	}
	made := tt.e.count("SELECT count(*) FROM page_template_origin o JOIN template_sets s ON s.id = o.set_id WHERE s.template_id = $1 AND s.version = 1", tpl)
	if made != 1 {
		t.Fatalf("the rule did not use the approved version: %d", made)
	}
	// A required question nobody answers for a rule fails the run, naming it (TPL-1105), and
	// the test run warns before (TPL-1105).
	tt.owner.want(http.StatusOK, "PUT", "/api/v1/spaces/"+tt.space+"/template-review", map[string]any{"on": false})
	tt.owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Weekly report",
		"document": textWith(map[string]any{"type": "templateField", "attrs": map[string]any{"name": "Team", "kind": "line", "required": true}})})
	test := tt.owner.ok("POST", "/api/v1/rules/"+rule+"/test", map[string]any{"page_id": page})
	if note := test["steps"].([]any)[0].(map[string]any)["note"]; note != "template_asks" {
		t.Fatalf("the test run: %v", test)
	}
	if got := run(); got["state"] != "failed" || got["report"] != "template_asks" {
		t.Fatalf("a question a rule cannot answer: %v", got)
	}
	// An owner of the rule who leaves the users list gets a run that says so (TPL-505).
	tt.owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Weekly report", "document": para("Plain again")})
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"use": []any{userSubject(tt.colleague)}})
	if got := run(); got["state"] != "failed" || got["report"] != "template_not_offered" {
		t.Fatalf("an owner off the users list: %v", got)
	}
	// A withdrawn template makes nothing for a rule either.
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{})
	tt.owner.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Out of date"})
	if got := run(); got["state"] != "failed" || got["report"] != "template_not_approved" {
		t.Fatalf("a withdrawn template: %v", got)
	}
}

// backgroundTemplate makes a template of 30 pages, so a set of it is made as background work.
func backgroundTemplate(tt templateTeam, name string) string {
	tt.e.t.Helper()
	tpl := tt.owner.guided(tt.space, name, "Root")
	for i := range 29 {
		tt.owner.entry(tpl, map[string]any{"name": fmt.Sprintf("Page %d", i+1), "parent": "root", "document": para("x")})
	}
	return tpl
}

func TestAWaitingBackgroundSetMakesNothingFromAWithdrawnVersion(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := backgroundTemplate(tt, "Big")
	before := tt.e.traceOf(tt.space)
	status, out := tt.editor.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Waiting"})
	if status != http.StatusAccepted {
		t.Fatalf("a background set: %d %v", status, out)
	}
	// A restore that brings the job back with the withdrawal, or a withdrawal while it waits:
	// the job runs, checks the version again, and makes nothing (TPL-524, TPL-854).
	tt.owner.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Wrong"})
	tt.e.settle()
	if after := tt.e.traceOf(tt.space); after.pages != before.pages || after.sets != before.sets {
		t.Fatalf("a withdrawn version made pages: %+v", after)
	}
	if n := tt.e.count("SELECT count(*) FROM jobs WHERE kind = 'template_set' AND status = 'failed'"); n != 1 {
		t.Fatalf("the job did not end: %d", n)
	}
}

func TestTurningThePartOffAndOnKeepsEverything(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	tpl, page, _ := publishedSet(tt, "Guided one")
	tt.owner.guided(tt.space, "Plain one", "x")
	snapshot := func() string {
		return fmt.Sprint(
			tt.e.count("SELECT count(*) FROM template_versions WHERE template_id = $1", tpl),
			tt.e.count("SELECT count(*) FROM template_versions WHERE template_id = $1 AND state = 'approved'", tpl),
			tt.e.count("SELECT count(*) FROM template_entry_pages WHERE template_id = $1", tpl),
			tt.e.count("SELECT count(*) FROM page_template_origin WHERE template_id = $1", tpl),
			tt.e.count("SELECT count(*) FROM template_sets WHERE template_id = $1", tpl))
	}
	before := snapshot()
	partOff(root)
	// Guided templates leave the gallery, plain ones stay, and nothing is lost (TPL-1107).
	names := galleryNames(tt.editor, tt.space)
	if has(names, "Guided one") || !has(names, "Plain one") {
		t.Fatalf("the gallery while off: %v", names)
	}
	if got := snapshot(); got != before {
		t.Fatalf("turning the part off changed %s into %s", before, got)
	}
	tt.owner.want(http.StatusNotFound, "GET", "/api/v1/pages/"+page+"/template-origin", nil)
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"automation", "bundles",
		"console_restore", "databases", "guided_templates"}})
	if names := galleryNames(tt.editor, tt.space); !has(names, "Guided one") || !has(names, "Plain one") {
		t.Fatalf("the gallery on again: %v", names)
	}
	if got := snapshot(); got != before {
		t.Fatalf("turning the part on again changed %s into %s", before, got)
	}
	if got := tt.owner.ok("GET", "/api/v1/pages/"+page+"/template-origin", nil); got["name"] != "Guided one" {
		t.Fatalf("the origin on again: %v", got)
	}
}

func TestTheSpaceAuditShowsTheTemplateEventsOfItsSpaceOnly(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	auditor := tt.e.user("auditor@example.com", identity.PermUseProduct)
	grantExactly(tt.owner, tt.space, auditor.id, permissions.SpaceView, permissions.SpaceAudit)
	tpl := tt.owner.guided(tt.space, "Audited", "x")
	tt.owner.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Done"})
	everywhere := root.ok("POST", "/api/v1/admin/templates", map[string]any{"name": "Everywhere", "document": para("x")})["id"].(string)
	out := auditor.want(http.StatusOK, "GET", "/api/v1/spaces/"+tt.space+"/audit", nil)
	for _, action := range []string{"template.created", "template.version_saved", "template.withdrawn"} {
		if !strings.Contains(out, action) {
			t.Fatalf("the space audit lacks %s: %s", action, out)
		}
	}
	if strings.Contains(out, everywhere) {
		t.Fatalf("the space audit shows a template of all spaces: %s", out)
	}
	if all := root.want(http.StatusOK, "GET", "/api/v1/admin/audit-events?action=template.created", nil); !strings.Contains(all, everywhere) {
		t.Fatalf("the instance audit lacks the template of all spaces: %s", all)
	}
	tt.editor.want(http.StatusForbidden, "GET", "/api/v1/spaces/"+tt.space+"/audit", nil)
}

func TestTheStartersStayApprovedAndOfferedAfterTheMigration(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	galleryNames(tt.editor, tt.space)
	starters := tt.e.count("SELECT count(*) FROM templates WHERE starter_key IS NOT NULL")
	if starters == 0 {
		t.Fatal("no starters")
	}
	approved := tt.e.count(`SELECT count(*) FROM templates t JOIN template_versions v ON v.template_id = t.id AND v.number = t.approved_version
		WHERE t.starter_key IS NOT NULL AND t.state = 'approved' AND v.state = 'approved'`)
	if approved != starters {
		t.Fatalf("%d of %d starters are approved (TPL-531)", approved, starters)
	}
	on := len(galleryNames(tt.editor, tt.space))
	partOff(root)
	if off := len(galleryNames(tt.editor, tt.space)); off != on {
		t.Fatalf("the starters on %d, off %d", on, off)
	}
}

func TestTheNewRoutesOfGT5AreOffWhileThePartIsOff(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	tpl, page, _ := publishedSet(tt, "Set")
	partOff(root)
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/pages/" + page + "/template-origin"},
		{"GET", "/api/v1/templates/" + tpl + "/versions/3/sets"},
		{"GET", "/api/v1/spaces/" + tt.space + "/templates?q=Set"},
	} {
		if out := tt.owner.want(http.StatusNotFound, r.method, r.path, nil); !strings.Contains(out, "experimental_off") {
			t.Errorf("%s %s: %s", r.method, r.path, out)
		}
	}
}
