// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/templates"
)

// Spec 074, wave GT4: rights per template, and approval. The refusals come first.

// grantExactly gives a person exactly these permissions in a space.
func grantExactly(owner *user, space, id string, perms ...string) {
	owner.e.t.Helper()
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": id}, "permissions": perms})
}

// templateTeam is a space with an owner who runs it, a second manager of its templates, an
// editor who may propose templates, a second editor, and a viewer.
type templateTeam struct {
	e                                         *env
	owner, manager, editor, colleague, viewer *user
	space                                     string
}

func newTemplateTeam(t *testing.T) templateTeam {
	t.Helper()
	e := newEnv(t)
	owner, editor, viewer, space := e.team("ENG")
	manager := e.user("manager@example.com", identity.PermUseProduct)
	grantExactly(owner, space, manager.id, permissions.SpaceView, permissions.PageCreate, permissions.SpaceTemplates,
		permissions.TemplatePropose)
	colleague := e.user("colleague@example.com", identity.PermUseProduct)
	owner.grant(space, "user", colleague.id, permissions.TemplateEditor)
	return templateTeam{e: e, owner: owner, manager: manager, editor: editor, colleague: colleague, viewer: viewer, space: space}
}

// draft makes a template in draft through My templates, as one who may propose.
func (tt templateTeam) draft(u *user, name, text string) string {
	tt.e.t.Helper()
	return u.ok("POST", "/api/v1/spaces/"+tt.space+"/my-templates", map[string]any{"name": name, "document": para(text)})["id"].(string)
}

func versionsPath(tpl string, number int, action string) string {
	return fmt.Sprintf("/api/v1/templates/%s/versions/%d/%s", tpl, number, action)
}

func userSubject(u *user) map[string]any { return map[string]any{"type": "user", "id": u.id} }

func galleryNames(u *user, space string) []string {
	u.e.t.Helper()
	var names []string
	for _, x := range u.ok("GET", "/api/v1/spaces/"+space+"/templates", nil)["templates"].([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	return names
}

func TestAUsersListOnlyNarrowsWhoMayUseATemplate(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Dismissal procedure", "Steps")
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"use": []any{userSubject(tt.viewer), userSubject(tt.colleague)}})

	// On the list without page.create, the viewer still gets nothing (TPL-501).
	if status, _ := tt.viewer.makeSet(tpl, map[string]any{"space_id": tt.space}); status != http.StatusForbidden {
		t.Fatalf("a listed person without page.create made pages: %d", status)
	}
	// Off the list, the editor is told it does not exist, on every route (TPL-504).
	if has(galleryNames(tt.editor, tt.space), "Dismissal procedure") {
		t.Fatal("the gallery offered a template to someone off its list")
	}
	for _, r := range []struct{ method, path string }{
		{"GET", "/api/v1/templates/" + tpl},
		{"POST", "/api/v1/templates/" + tpl + "/use"},
		{"POST", "/api/v1/templates/" + tpl + "/sets"},
		{"POST", "/api/v1/templates/" + tpl + "/sets/check"},
		{"PUT", "/api/v1/templates/" + tpl + "/favourite"},
		{"GET", "/api/v1/templates/" + tpl + "/files/" + tpl},
		{"GET", "/api/v1/templates/" + tpl + "/versions"},
		{"GET", "/api/v1/templates/" + tpl + "/rights"},
	} {
		if got, out := tt.editor.do(r.method, r.path, map[string]any{"space_id": tt.space}); got != http.StatusNotFound {
			t.Errorf("%s %s off the list: %d, want 404: %s", r.method, r.path, got, out)
		}
	}
	// On the list with page.create, the colleague uses it.
	if !has(galleryNames(tt.colleague, tt.space), "Dismissal procedure") {
		t.Fatal("the gallery hid a template from someone on its list")
	}
	if status, out := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space}); status != http.StatusCreated {
		t.Fatalf("a listed creator: %d %v", status, out)
	}
	// Rights are asked again on submit: a form opened before its person left the list makes
	// nothing (TPL-503).
	opened := tt.colleague.ok("GET", "/api/v1/templates/"+tpl, nil)
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"use": []any{userSubject(tt.viewer)}})
	if status, _ := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space, "version": opened["template_version"]}); status != http.StatusNotFound {
		t.Fatalf("a form opened before its person left the list: %d", status)
	}
	// Emptied, the list limits nothing again.
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{})
	if status, _ := tt.editor.makeSet(tpl, map[string]any{"space_id": tt.space}); status != http.StatusCreated {
		t.Fatalf("an empty list: %d", status)
	}
}

func TestAnEditorsListOnlyNarrowsWhoOfTheManagersMayChangeATemplate(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Onboarding", "Welcome")
	// The person setting a list of editors must be on it, or nobody might be left (TPL-501).
	out := tt.owner.want(http.StatusBadRequest, "PUT", "/api/v1/templates/"+tpl+"/rights",
		map[string]any{"edit": []any{userSubject(tt.manager)}})
	if !strings.Contains(out, "editors_lockout") {
		t.Fatalf("lock-out: %s", out)
	}
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"edit": []any{userSubject(tt.owner), userSubject(tt.viewer)}})
	// A manager off the list may not change it, nor set its lists; a listed viewer without
	// space.templates may not either.
	for _, u := range []*user{tt.manager, tt.viewer} {
		u.want(http.StatusForbidden, "PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Changed by " + u.email})
		u.want(http.StatusForbidden, "PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"edit": []any{userSubject(u)}})
	}
	tt.owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding 2"})
	// Whoever runs the space may always clear the lists, and nothing else (TPL-501).
	runner := tt.e.user("runner@example.com", identity.PermUseProduct)
	tt.owner.grant(tt.space, "user", runner.id, permissions.TemplateSpaceAdmin)
	runner.want(http.StatusForbidden, "PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"edit": []any{userSubject(runner)}})
	runner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{})
	if n := tt.e.auditActions("template.rights_changed"); n != 2 {
		t.Fatalf("the changes of the lists are audited: %d", n)
	}
	tt.manager.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding 3"})
}

func TestATemplateOfAllSpacesIsOfferedOnlyInItsChosenSpaces(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	other, _ := tt.owner.space("OPS")
	tpl := root.ok("POST", "/api/v1/admin/templates", map[string]any{"name": "Everywhere", "document": para("x")})["id"].(string)
	root.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"chosen": true, "spaces": []string{other}})
	if has(galleryNames(tt.editor, tt.space), "Everywhere") || !has(galleryNames(tt.owner, other), "Everywhere") {
		t.Fatal("a template of all spaces went past its chosen spaces")
	}
	if status, _ := tt.editor.makeSet(tpl, map[string]any{"space_id": tt.space}); status != http.StatusNotFound {
		t.Fatalf("used outside its spaces: %d", status)
	}
	// A space template cannot be offered elsewhere, and a deleted space drops off the list.
	space := tt.owner.guided(tt.space, "Local", "x")
	tt.owner.want(http.StatusBadRequest, "PUT", "/api/v1/templates/"+space+"/rights", map[string]any{"chosen": true, "spaces": []string{other}})
	tt.e.sql("DELETE FROM spaces WHERE id = $1", other)
	if got := root.ok("GET", "/api/v1/templates/"+tpl+"/rights", nil)["spaces"].([]any); len(got) != 0 || has(galleryNames(tt.owner, tt.space), "Everywhere") {
		t.Fatalf("an empty list of spaces offers it nowhere: %v", got)
	}
}

func TestAWaitingOrWithdrawnVersionNeverMakesPages(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.draft(tt.editor, "Checklist", "First text")
	tt.editor.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	// The owner tries the draft for real, and gets drafts of their own (TPL-510).
	status, made := tt.editor.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Try"})
	if status != http.StatusCreated || made["page"].(map[string]any)["status"] == "published" {
		t.Fatalf("the owner's try: %d %v", status, made)
	}
	// Nobody else is offered a draft, nor told it exists.
	if has(galleryNames(tt.colleague, tt.space), "Checklist") {
		t.Fatal("a draft was offered to someone else")
	}
	if status, _ := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space, "version": 2}); status != http.StatusNotFound {
		t.Fatalf("someone else used a draft: %d", status)
	}
	tt.editor.ok("POST", versionsPath(tpl, 2, "propose"), nil)
	// A waiting version makes nothing, for anyone, its owner included.
	for _, u := range []*user{tt.editor, tt.owner, tt.colleague} {
		status, out := u.makeSet(tpl, map[string]any{"space_id": tt.space, "version": 2, "title": "Waiting"})
		if status == http.StatusCreated || status == http.StatusAccepted {
			t.Fatalf("%s made pages from a waiting version: %v", u.email, out)
		}
	}
	// An approver may still try it without making anything.
	if status, _ := tt.owner.do("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": tt.space, "version": 2}); status != http.StatusOK {
		t.Fatalf("an approver's dry run: %d", status)
	}
	tt.owner.ok("POST", versionsPath(tpl, 2, "approve"), map[string]any{"comment": "Fine"})
	if status, out := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space, "version": 2, "title": "Real"}); status != http.StatusCreated {
		t.Fatalf("the approved version: %d %v", status, out)
	}
	// Withdrawn, it leaves the gallery at once, and a form still open is refused (TPL-524).
	tt.owner.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Out of date"})
	if has(galleryNames(tt.colleague, tt.space), "Checklist") {
		t.Fatal("a withdrawn template stayed in the gallery")
	}
	for _, u := range []*user{tt.colleague, tt.editor} {
		status, out := u.makeSet(tpl, map[string]any{"space_id": tt.space, "version": 2, "title": "Late"})
		if code, _ := refusedWith(out); status != http.StatusConflict || code != "template_withdrawn" {
			t.Fatalf("%s submitted a form of a withdrawn template: %d %v", u.email, status, out)
		}
	}
	if status, _ := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space}); status == http.StatusCreated {
		t.Fatal("a withdrawn template made pages from its newest version")
	}
}

func TestTheApprovedVersionStaysInUseWhileANewOneWaits(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.draft(tt.editor, "Minutes", "Version one")
	tt.editor.ok("POST", versionsPath(tpl, 1, "propose"), nil)
	tt.manager.ok("POST", versionsPath(tpl, 1, "approve"), map[string]any{})
	// The owner changes it: a new draft, and the approved version is still what people get.
	tt.editor.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Minutes", "document": para("Version two")})
	opened := tt.colleague.ok("GET", "/api/v1/templates/"+tpl, nil)
	if opened["template_version"] != float64(1) || !strings.Contains(fmt.Sprint(opened["document"]), "Version one") {
		t.Fatalf("a form opened while a draft exists: %v %v", opened["template_version"], opened["document"])
	}
	tt.editor.ok("POST", versionsPath(tpl, 2, "propose"), nil)
	status, made := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Meeting"})
	if status != http.StatusCreated {
		t.Fatalf("use while a version waits: %d %v", status, made)
	}
	if d := tt.e.scalar("SELECT document::text FROM drafts WHERE page_id = $1", made["page"].(map[string]any)["id"]); !strings.Contains(d, "Version one") {
		t.Fatalf("the approved text: %s", d)
	}
	tt.manager.ok("POST", versionsPath(tpl, 2, "approve"), map[string]any{})
	// A form opened on version 1 before the approval still makes version 1, for a day (TPL-603).
	if status, _ := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space, "version": 1, "title": "Late"}); status != http.StatusCreated {
		t.Fatalf("a form opened before the approval: %d", status)
	}
	tt.e.sql("UPDATE template_versions SET superseded_at = now() - interval '25 hours' WHERE template_id = $1 AND number = 1", tpl)
	status, out := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space, "version": 1, "title": "Too late"})
	if code, _ := refusedWith(out); status != http.StatusConflict || code != "version_not_offered" {
		t.Fatalf("a form open for more than a day: %d %v", status, out)
	}
	// A withdrawal takes the version replaced within the day out of use too (TPL-524).
	tt.e.sql("UPDATE template_versions SET superseded_at = now() WHERE template_id = $1 AND number = 1", tpl)
	tt.manager.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Wrong"})
	if status, _ := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space, "version": 1, "title": "After"}); status != http.StatusConflict {
		t.Fatalf("a replaced version after a withdrawal: %d", status)
	}
}

func TestAWithdrawalWhileASetIsMadeRefusesTheSet(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Raced", "x")
	service := tt.e.app.Templates()
	defer service.SetHooks(templates.SetHooks{})
	service.SetHooks(templates.SetHooks{Step: func(_ context.Context, step string, _ int) error {
		if step == "start" {
			tt.manager.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Now"})
		}
		return nil
	}})
	status, out := tt.colleague.makeSet(tpl, map[string]any{"space_id": tt.space})
	if code, _ := refusedWith(out); status != http.StatusConflict || code != "template_withdrawn" {
		t.Fatalf("a set whose template was withdrawn while it was made: %d %v", status, out)
	}
	if n := tt.e.count("SELECT count(*) FROM template_sets WHERE template_id = $1", tpl); n != 0 {
		t.Fatalf("sets: %d", n)
	}
}

func TestOnlyTheNewestVersionIsProposed(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.draft(tt.editor, "Two drafts", "one")
	tt.editor.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Two drafts", "document": para("two")})
	out := tt.editor.want(http.StatusConflict, "POST", versionsPath(tpl, 1, "propose"), nil)
	if !strings.Contains(out, "version_superseded") {
		t.Fatalf("an older draft proposed: %s", out)
	}
	tt.editor.ok("POST", versionsPath(tpl, 2, "propose"), nil)
	tt.editor.want(http.StatusConflict, "POST", versionsPath(tpl, 2, "propose"), nil)
}

func TestNobodyApprovesTheirOwnVersionWhenFourEyesAreOn(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Incident", "Before")
	// Only whoever runs the space turns it on; a manager of templates is the one it binds.
	tt.manager.want(http.StatusForbidden, "PUT", "/api/v1/spaces/"+tt.space+"/template-review", map[string]any{"on": true})
	tt.owner.ok("PUT", "/api/v1/spaces/"+tt.space+"/template-review", map[string]any{"on": true})
	// A manager's change now waits for a second person, and the approved version stays.
	tt.owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Incident", "document": para("After")})
	if v := tt.colleague.ok("GET", "/api/v1/templates/"+tpl, nil)["template_version"]; v != float64(1) {
		t.Fatalf("a change with four eyes on was published: %v", v)
	}
	tt.owner.ok("POST", versionsPath(tpl, 2, "propose"), nil)
	out := tt.owner.want(http.StatusForbidden, "POST", versionsPath(tpl, 2, "approve"), map[string]any{})
	if !strings.Contains(out, "self_approval") {
		t.Fatalf("self-approval: %s", out)
	}
	tt.owner.want(http.StatusForbidden, "POST", versionsPath(tpl, 2, "send-back"), map[string]any{"reason": "Mine"})
	tt.manager.ok("POST", versionsPath(tpl, 2, "approve"), map[string]any{})
	if v := tt.colleague.ok("GET", "/api/v1/templates/"+tpl, nil)["template_version"]; v != float64(2) {
		t.Fatalf("the approved change: %v", v)
	}
	// Off again, managers publish their own changes directly (TPL-511).
	tt.owner.ok("PUT", "/api/v1/spaces/"+tt.space+"/template-review", map[string]any{"on": false})
	tt.owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Incident", "document": para("Third")})
	if v := tt.colleague.ok("GET", "/api/v1/templates/"+tpl, nil)["template_version"]; v != float64(3) {
		t.Fatalf("a manager's change with four eyes off: %v", v)
	}
}

func TestAProposerNeitherApprovesNorPublishes(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.draft(tt.editor, "Proposal", "x")
	tt.editor.ok("POST", versionsPath(tpl, 1, "propose"), nil)
	tt.editor.want(http.StatusForbidden, "POST", versionsPath(tpl, 1, "approve"), map[string]any{})
	tt.colleague.want(http.StatusNotFound, "POST", versionsPath(tpl, 1, "approve"), map[string]any{})
	tt.manager.ok("POST", versionsPath(tpl, 1, "approve"), map[string]any{})
	// Their next change is a draft again, and they cannot hide, promote or withdraw it.
	tt.editor.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Proposal", "hidden": true, "document": para("y")})
	if state := tt.e.scalar("SELECT state FROM template_versions WHERE template_id = $1 AND number = 2", tpl); state != "draft" {
		t.Fatalf("a proposer's change: %s", state)
	}
	if tt.e.scalar("SELECT hidden::text FROM templates WHERE id = $1", tpl) != "false" {
		t.Fatal("a proposer hid a template")
	}
	tt.editor.want(http.StatusForbidden, "POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "x"})
	// A template of all spaces is not theirs to make (TPL-512).
	tt.editor.want(http.StatusForbidden, "POST", "/api/v1/admin/templates", map[string]any{"name": "Everywhere"})
	// A person without template.propose makes no draft.
	tt.viewer.want(http.StatusForbidden, "POST", "/api/v1/spaces/"+tt.space+"/my-templates", map[string]any{"name": "No"})
}

func TestTwoApproversAtOnceMakeOneDecision(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.draft(tt.editor, "Race", "x")
	tt.editor.ok("POST", versionsPath(tpl, 1, "propose"), nil)
	var wg sync.WaitGroup
	results := make([]int, 2)
	for i, u := range []*user{tt.owner, tt.manager} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _ = u.do("POST", versionsPath(tpl, 1, "approve"), map[string]any{})
		}()
	}
	wg.Wait()
	if !((results[0] == 200 && results[1] == 409) || (results[0] == 409 && results[1] == 200)) {
		t.Fatalf("two approvals: %v", results)
	}
	if n := tt.e.count("SELECT count(*) FROM template_reviews WHERE template_id = $1 AND decision = 'approved'", tpl); n != 1 {
		t.Fatalf("decisions: %d", n)
	}
	out := tt.owner.want(http.StatusConflict, "POST", versionsPath(tpl, 1, "send-back"), map[string]any{"reason": "Late"})
	if !strings.Contains(out, "already_decided") {
		t.Fatalf("the second: %s", out)
	}
	// A proposal that was changed after it was made is over: its version cannot be decided on.
	tt.editor.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Race", "document": para("y")})
	tt.editor.ok("POST", versionsPath(tpl, 2, "propose"), nil)
	tt.editor.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Race", "document": para("z")})
	tt.manager.want(http.StatusConflict, "POST", versionsPath(tpl, 2, "approve"), map[string]any{})
	if state := tt.e.scalar("SELECT state FROM template_versions WHERE template_id = $1 AND number = 2", tpl); state != "superseded" {
		t.Fatalf("a changed proposal: %s", state)
	}
}

func TestApprovalChecksTheTemplateAgain(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Site staff"})["id"].(string)
	tpl := tt.draft(tt.editor, "Site visit", "x")
	key := tt.editor.entry(tpl, map[string]any{"name": "On site", "parent": "root", "document": para("Boots")})
	if status, out := tt.editor.setCondition(tpl, key, when("all", map[string]any{"subject": "groups", "op": "member", "value": group})); status != http.StatusNoContent {
		t.Fatalf("a condition on a group: %d %s", status, out)
	}
	number := int(tt.editor.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)["versions"].([]any)[0].(map[string]any)["number"].(float64))
	tt.editor.ok("POST", versionsPath(tpl, number, "propose"), nil)
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/groups/"+group, nil)
	out := tt.manager.want(http.StatusBadRequest, "POST", versionsPath(tpl, number, "approve"), map[string]any{})
	if !strings.Contains(out, "invalid_conditions") {
		t.Fatalf("approval of a template naming a deleted group: %s", out)
	}
	// Sending it back needs a reason, which the author reads.
	tt.manager.want(http.StatusBadRequest, "POST", versionsPath(tpl, number, "send-back"), map[string]any{"reason": " "})
	tt.manager.ok("POST", versionsPath(tpl, number, "send-back"), map[string]any{"reason": "The group is gone"})
	history := tt.editor.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)
	if !strings.Contains(fmt.Sprint(history), "The group is gone") || history["state"] != "sent_back" {
		t.Fatalf("the author reads why: %v", history)
	}
}

func TestWhoSeesATemplateInEachState(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.draft(tt.editor, "States", "x")
	sees := func() map[string]int {
		out := map[string]int{}
		for name, u := range map[string]*user{"owner": tt.editor, "manager": tt.manager, "member": tt.colleague, "viewer": tt.viewer} {
			out[name], _ = u.do("GET", "/api/v1/templates/"+tpl, nil)
		}
		return out
	}
	check := func(state string, want map[string]int) {
		t.Helper()
		if got := sees(); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: got %v, want %v", state, got, want)
		}
	}
	check("draft", map[string]int{"owner": 200, "manager": 404, "member": 404, "viewer": 404})
	tt.editor.ok("POST", versionsPath(tpl, 1, "propose"), nil)
	check("waiting", map[string]int{"owner": 200, "manager": 200, "member": 404, "viewer": 404})
	tt.manager.ok("POST", versionsPath(tpl, 1, "approve"), map[string]any{})
	check("approved", map[string]int{"owner": 200, "manager": 200, "member": 200, "viewer": 200})
	tt.manager.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Old"})
	check("withdrawn", map[string]int{"owner": 200, "manager": 200, "member": 404, "viewer": 404})
	// A draft never proposed is its owner's own, until the owner leaves; then the managers see
	// it and may delete it (TPL-853).
	left := tt.draft(tt.colleague, "Left behind", "x")
	tt.manager.want(http.StatusNotFound, "GET", "/api/v1/templates/"+left, nil)
	tt.e.sql("UPDATE users SET status = 'disabled' WHERE id = $1", tt.colleague.id)
	tt.manager.ok("GET", "/api/v1/templates/"+left, nil)
	tt.manager.want(http.StatusNoContent, "DELETE", "/api/v1/templates/"+left, nil)
	// An owner who may no longer read the space reads nothing of their template.
	grantExactly(tt.owner, tt.space, tt.editor.id)
	tt.editor.want(http.StatusNotFound, "GET", "/api/v1/templates/"+tpl+"/newest", nil)
	tt.editor.want(http.StatusNotFound, "GET", "/api/v1/templates/"+tpl+"/versions", nil)
}

func TestApprovalNotificationsTellWhoMayDecideAndTheAuthor(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.draft(tt.editor, "Told", "x")
	tt.editor.ok("POST", versionsPath(tpl, 1, "propose"), nil)
	tt.e.deliver()
	rows := func(u *user, kind string) int {
		return tt.e.count("SELECT count(*) FROM notifications WHERE user_id = $1 AND type = $2", u.id, kind)
	}
	if rows(tt.owner, "template_proposed") != 1 || rows(tt.manager, "template_proposed") != 1 || rows(tt.editor, "template_proposed") != 0 {
		t.Fatal("a proposal tells each who may decide, once, and not its author")
	}
	if !strings.Contains(tt.manager.want(200, "GET", "/api/v1/me/notifications", nil), "template_proposed") {
		t.Fatal("the bell shows the proposal to an approver")
	}
	if strings.Contains(tt.colleague.want(200, "GET", "/api/v1/me/notifications", nil), "template_") {
		t.Fatal("a member was told of a template")
	}
	tt.manager.ok("POST", versionsPath(tpl, 1, "send-back"), map[string]any{"reason": "Needs a title"})
	tt.e.deliver()
	if rows(tt.editor, "template_decided") != 1 {
		t.Fatal("a decision tells the author")
	}
	if n := tt.e.count("SELECT count(*) FROM notifications WHERE type = 'template_proposed' AND read_at IS NULL"); n != 0 {
		t.Fatalf("the first decision closes the proposal for all: %d unread", n)
	}
	if n := tt.e.count("SELECT count(*) FROM notifications WHERE type LIKE 'template_%' AND data::text LIKE '%Needs a title%'"); n != 0 {
		t.Fatal("a reason travelled in a notification")
	}
	tt.editor.ok("POST", versionsPath(tpl, 1, "propose"), nil)
	tt.manager.ok("POST", versionsPath(tpl, 1, "approve"), map[string]any{})
	tt.owner.ok("POST", "/api/v1/templates/"+tpl+"/withdraw", map[string]any{"reason": "Done"})
	tt.e.deliver()
	if rows(tt.editor, "template_decided") != 3 || rows(tt.manager, "template_decided") != 1 {
		t.Fatal("a withdrawal tells the author and the managers")
	}
	for _, action := range []string{"template.proposed", "template.approved", "template.sent_back", "template.withdrawn"} {
		if tt.e.auditActions(action) == 0 {
			t.Errorf("no audit event %s", action)
		}
	}
}

func TestTheNewRoutesOfGT4AreOffWhileThePartIsOff(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	tpl := tt.owner.guided(tt.space, "Off", "x")
	partOff(root)
	for _, r := range []struct {
		u            *user
		method, path string
	}{
		{tt.owner, "GET", "/api/v1/templates/" + tpl + "/rights"},
		{tt.owner, "PUT", "/api/v1/templates/" + tpl + "/rights"},
		{tt.owner, "POST", versionsPath(tpl, 1, "propose")},
		{tt.owner, "POST", versionsPath(tpl, 1, "approve")},
		{tt.owner, "POST", versionsPath(tpl, 1, "send-back")},
		{tt.owner, "POST", "/api/v1/templates/" + tpl + "/withdraw"},
		{tt.editor, "GET", "/api/v1/spaces/" + tt.space + "/my-templates"},
		{tt.editor, "POST", "/api/v1/spaces/" + tt.space + "/my-templates"},
		{tt.owner, "GET", "/api/v1/spaces/" + tt.space + "/template-review"},
		{tt.owner, "PUT", "/api/v1/spaces/" + tt.space + "/template-review"},
		{root, "GET", "/api/v1/admin/template-review"},
		{root, "PUT", "/api/v1/admin/template-review"},
	} {
		if out := r.u.want(http.StatusNotFound, r.method, r.path, map[string]any{}); !strings.Contains(out, "experimental_off") {
			t.Errorf("%s %s: %s", r.method, r.path, out)
		}
	}
	// The newest version answers while the part is off: every template's editor reads it.
	tt.owner.ok("GET", "/api/v1/templates/"+tpl+"/newest", nil)
}

func TestListsDraftsAndFourEyesWaitForTheOlderReleaseToStop(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Held", "x")
	tt.e.sql(`INSERT INTO nodes (id, role, version, max_migration, last_seen_at) VALUES (gen_random_uuid(), 'all', 'older', 69, now())`)
	for _, r := range []struct {
		u            *user
		method, path string
		body         map[string]any
	}{
		{tt.owner, "PUT", "/api/v1/templates/" + tpl + "/rights", map[string]any{"use": []any{userSubject(tt.editor)}}},
		{tt.editor, "POST", "/api/v1/spaces/" + tt.space + "/my-templates", map[string]any{"name": "Draft"}},
		{tt.owner, "PUT", "/api/v1/spaces/" + tt.space + "/template-review", map[string]any{"on": true}},
	} {
		if out := r.u.want(http.StatusConflict, r.method, r.path, r.body); !strings.Contains(out, "older_release_running") {
			t.Errorf("%s %s: %s", r.method, r.path, out)
		}
	}
	// What the older release knows still works.
	tt.owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Held 2"})
}
