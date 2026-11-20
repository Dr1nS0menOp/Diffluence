// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Spec 074, wave GT5: export and import (TPL-852, TPL-1106). A file never carries who has a
// template, nor a person of the installation it came from. The refusals come first.

func groupSubject(id string) map[string]any { return map[string]any{"type": "group", "id": id} }

// richTemplate makes a template that names people and groups everywhere it can: a users and
// an editors list, a restricted page, a suggested person and a question that tests a group.
func richTemplate(tt templateTeam, group string) string {
	tt.e.t.Helper()
	tpl := tt.owner.guided(tt.space, "Onboarding", "Welcome")
	tt.owner.entry(tpl, map[string]any{"name": "Manager notes", "parent": "root", "document": para("Private"),
		"restrictions": map[string]any{"view": []any{userSubject(tt.manager), groupSubject(group)}, "edit": []any{userSubject(tt.manager)}}})
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{
		"use": []any{userSubject(tt.colleague), groupSubject(group)}, "edit": []any{userSubject(tt.owner)}})
	return tpl
}

func TestAnExportCarriesNoRightsNoStateAndNoPersonOfItsInstallation(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Managers"})["id"].(string)
	tpl := richTemplate(tt, group)
	file := tt.owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/export", nil)
	// No person of this installation travels: not the owner, an approver, a listed person, nor
	// one on a restriction (TPL-852).
	for _, u := range []*user{tt.owner, tt.manager, tt.colleague, root} {
		if strings.Contains(file, u.id) {
			t.Fatalf("the file names %s: %s", u.email, file)
		}
	}
	for _, word := range []string{`"use"`, `"edit":[{"type":"user"`, `"owner`, `"state"`, `"approved`, `"decisions"`, `"page_id"`, `"workflow_id"`} {
		if strings.Contains(file, word) {
			t.Fatalf("the file holds %s: %s", word, file)
		}
	}
	// A group travels by id and name only, to be chosen again, and the people left out are
	// counted so the import asks for them.
	if !strings.Contains(file, `"format":2`) || !strings.Contains(file, "Managers") || !strings.Contains(file, `"view_people":1`) {
		t.Fatalf("the file: %s", file)
	}
	// Only who manages it exports it: one who uses it is refused, a stranger told it is not there.
	tt.colleague.want(http.StatusForbidden, "GET", "/api/v1/templates/"+tpl+"/export", nil)
	tt.e.user("stranger@example.com", identity.PermUseProduct).want(http.StatusNotFound, "GET", "/api/v1/templates/"+tpl+"/export", nil)
}

func TestAnImportIsADraftOfTheImporterThatOnlyApprovalPutsInUse(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Managers"})["id"].(string)
	tpl := richTemplate(tt, group)
	var file map[string]any
	if err := json.Unmarshal([]byte(tt.owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/export", nil)), &file); err != nil {
		t.Fatal(err)
	}
	other, _ := tt.owner.space("OPS")
	grantExactly(tt.owner, other, tt.manager.id, permissions.SpaceView, permissions.PageCreate, permissions.SpaceTemplates,
		permissions.TemplatePropose)
	// Whoever may not manage templates there may not import, and learns nothing.
	if status, _ := tt.colleague.do("POST", "/api/v1/spaces/"+other+"/templates/import", file); status != http.StatusForbidden &&
		status != http.StatusNotFound {
		t.Fatalf("an import by someone who may not: %d", status)
	}
	made := tt.manager.ok("POST", "/api/v1/spaces/"+other+"/templates/import", file)
	id := made["id"].(string)
	// A draft of the importer, even with four eyes off and an importer who manages templates:
	// an imported template has not been checked here (TPL-852).
	history := tt.manager.ok("GET", "/api/v1/templates/"+id+"/versions", nil)
	if made["state"] != "draft" || history["approved"].(float64) != 0 || made["mine"] != true {
		t.Fatalf("the import: %v", made)
	}
	if n := tt.e.count("SELECT count(*) FROM template_rights WHERE template_id = $1", id); n != 0 {
		t.Fatalf("the import carried %d entries of rights lists", n)
	}
	if n := tt.e.count(`SELECT count(*) FROM template_reviews WHERE template_id = $1`, id); n != 0 {
		t.Fatalf("the import carried %d decisions", n)
	}
	notes := made["import_notes"].(map[string]any)
	if notes["people_left_out"].(float64) != 2 {
		t.Fatalf("the notes: %v", notes)
	}
	// Nobody but the importer is offered it, and nothing is made from it for anyone else.
	if has(galleryNames(tt.owner, other), "Onboarding") {
		t.Fatal("an imported draft was offered to someone else")
	}
	// The restricted page stays restricted, to the importer instead of the people left out,
	// and asks for them to be chosen again before approval.
	entries := tt.manager.ok("GET", "/api/v1/templates/"+id+"/entries", nil)["entries"].([]any)
	notesEntry := entries[1].(map[string]any)
	restrictions := notesEntry["restrictions"].(map[string]any)
	if restrictions["choose_again"] != true || !strings.Contains(mustJSON(t, restrictions), tt.manager.id) ||
		strings.Contains(mustJSON(t, restrictions), tt.owner.id) {
		t.Fatalf("the restriction of the imported page: %v", restrictions)
	}
	tt.manager.ok("POST", versionsPath(id, 1, "propose"), nil)
	out := tt.manager.want(http.StatusConflict, "POST", versionsPath(id, 1, "approve"), map[string]any{})
	if !strings.Contains(out, "choose_again") || !strings.Contains(out, "Manager notes") {
		t.Fatalf("approval before choosing again: %s", out)
	}
	// Sending the same lists back does not clear the mark; choosing them again does, and a
	// manager without four eyes then publishes directly, as with any change of theirs (TPL-1411).
	change := func(view []any) map[string]any {
		return map[string]any{"name": notesEntry["name"], "kind": "page", "parent": "root", "title_pattern": "", "labels": []any{},
			"restrictions": map[string]any{"view": view, "edit": restrictions["edit"], "choose_again": false}}
	}
	if out := tt.manager.want(http.StatusConflict, "PATCH", "/api/v1/templates/"+id+"/entries/"+notesEntry["key"].(string),
		change(restrictions["view"].([]any))); !strings.Contains(out, "choose_again") {
		t.Fatalf("the mark was cleared by sending it back: %s", out)
	}
	tt.manager.ok("PATCH", "/api/v1/templates/"+id+"/entries/"+notesEntry["key"].(string), change([]any{groupSubject(group), userSubject(tt.colleague)}))
	if !has(galleryNames(tt.owner, other), "Onboarding") {
		t.Fatal("the import is not offered once its lists were chosen again")
	}
}

func TestAnImportNamingAGroupThatIsNotHereKeepsTheRestrictionAndWaits(t *testing.T) {
	tt := newTemplateTeam(t)
	root := admin(tt.e)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Managers"})["id"].(string)
	tpl := tt.owner.guided(tt.space, "Review", "Text")
	tt.owner.entry(tpl, map[string]any{"name": "Board notes", "parent": "root", "document": para("Private"),
		"restrictions": map[string]any{"view": []any{groupSubject(group)}}})
	file := tt.owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/export", nil)
	// The other installation has no such group.
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/groups/"+group, nil)
	made := tt.owner.ok("POST", "/api/v1/spaces/"+tt.space+"/templates/import", json.RawMessage(file))
	id := made["id"].(string)
	notes := made["import_notes"].(map[string]any)
	if got := notes["groups_to_choose"].([]any); len(got) != 1 || got[0] != "Managers" {
		t.Fatalf("the groups to choose: %v", notes)
	}
	// The page says it is to be chosen again, so its editor shows it (TPL-852).
	entries := tt.owner.ok("GET", "/api/v1/templates/"+id+"/entries", nil)["entries"].([]any)
	if r := entries[1].(map[string]any)["restrictions"].(map[string]any); r["choose_again"] != true {
		t.Fatalf("a restriction naming a group that is not here: %v", r)
	}
	// The page keeps its restriction, so it never opens to everyone (TPL-1417), and the owner's
	// try is refused rather than made without it.
	if status, out := tt.owner.makeSet(id, map[string]any{"space_id": tt.space}); status != http.StatusConflict {
		t.Fatalf("a try with a group that is not here: %d %v", status, out)
	}
	tt.owner.ok("POST", versionsPath(id, 1, "propose"), nil)
	if out := tt.owner.want(http.StatusConflict, "POST", versionsPath(id, 1, "approve"), map[string]any{}); !strings.Contains(out, "choose_again") {
		t.Fatalf("approval with a missing group: %s", out)
	}
}

func TestAnImportNeverPutsAPersonOfTheFileOnARestriction(t *testing.T) {
	tt := newTemplateTeam(t)
	// A file written by hand names a person of this installation as a group, and as a person.
	crafted := map[string]any{"format": 2, "name": "Crafted", "description": "", "category": "", "labels": []string{},
		"title_pattern": "", "steps": []any{}, "document": para("x"),
		"entries": []any{map[string]any{"key": "k", "name": "Private", "kind": "page", "parent": "root", "title_pattern": "",
			"labels": []string{}, "document": para("y"),
			"restrictions": map[string]any{"view": []any{map[string]any{"type": "user", "id": tt.colleague.id, "name": "x"}},
				"edit": []any{map[string]any{"type": "group", "id": tt.viewer.id, "name": "y"}}}}}}
	made := tt.owner.ok("POST", "/api/v1/spaces/"+tt.space+"/templates/import", crafted)
	entries := tt.owner.ok("GET", "/api/v1/templates/"+made["id"].(string)+"/entries", nil)["entries"].([]any)
	restrictions := mustJSON(t, entries[1].(map[string]any)["restrictions"])
	if strings.Contains(restrictions, tt.colleague.id) || !strings.Contains(restrictions, tt.owner.id) ||
		!strings.Contains(restrictions, `"choose_again":true`) {
		t.Fatalf("the restriction of a crafted file: %s", restrictions)
	}
	// A person's id given as a group is a group that is not here: it matches nobody.
	if made["import_notes"].(map[string]any)["people_left_out"].(float64) != 1 {
		t.Fatalf("the notes: %v", made["import_notes"])
	}
}

func TestAFileOfAnOlderReleaseImportsAsOnePageAndAnUnknownFormatIsRefused(t *testing.T) {
	tt := newTemplateTeam(t)
	// A file from before guided templates: no format, one page (TPL-1106).
	old := map[string]any{"name": "Old note", "description": "", "category": "", "labels": []string{"x"}, "title_pattern": "",
		"steps": []any{}, "document": para("Old words")}
	made := tt.owner.ok("POST", "/api/v1/spaces/"+tt.space+"/templates/import", old)
	if made["pages"].(float64) != 1 || made["state"] != "draft" {
		t.Fatalf("an old file: %v", made)
	}
	if n := tt.e.count("SELECT count(*) FROM template_versions WHERE template_id = $1", made["id"]); n != 1 {
		t.Fatalf("an old file makes version 1 only: %d", n)
	}
	// A file without a number that holds what only numbered files hold is refused whole.
	old["entries"] = []any{map[string]any{"key": "k", "name": "More", "kind": "page", "parent": "root", "document": para("x")}}
	tt.owner.want(http.StatusBadRequest, "POST", "/api/v1/spaces/"+tt.space+"/templates/import", old)
	delete(old, "entries")
	old["format"] = 3
	if out := tt.owner.want(http.StatusBadRequest, "POST", "/api/v1/spaces/"+tt.space+"/templates/import", old); !strings.Contains(out, "newer release") {
		t.Fatalf("a newer format: %s", out)
	}
	// A rights list in a file is no field of it: the file is refused, never read in part.
	old["format"] = 2
	old["use"] = []any{userSubject(tt.colleague)}
	tt.owner.want(http.StatusBadRequest, "POST", "/api/v1/spaces/"+tt.space+"/templates/import", old)
}

func TestAnOlderReleaseRefusesAFileOfGuidedTemplatesWhole(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Set", "Root")
	tt.owner.entry(tpl, map[string]any{"name": "Child", "parent": "root", "document": para("x")})
	file := tt.owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/export", nil)
	// The previous release reads a file into these fields alone and refuses any other (TPL-1106).
	var previous struct {
		Name         string          `json:"name"`
		Description  string          `json:"description"`
		Category     string          `json:"category"`
		Labels       []string        `json:"labels"`
		TitlePattern string          `json:"title_pattern"`
		Steps        []any           `json:"steps"`
		Document     json.RawMessage `json:"document"`
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(file)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&previous); err == nil {
		t.Fatalf("the previous release would import part of a file: %s", file)
	}
}

func TestAnExportImportRoundTripMakesTheSameSet(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Project", "Start {Team}")
	key := tt.owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": textWith(words("Plan of "), valueOf("entry:root"))})
	tt.owner.entry(tpl, map[string]any{"name": "Risks", "parent": key, "document": para("Risks")})
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	file := tt.owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/export", nil)
	other, _ := tt.owner.space("OPS")
	made := tt.owner.ok("POST", "/api/v1/spaces/"+other+"/templates/import", json.RawMessage(file))
	id := made["id"].(string)
	tt.owner.ok("POST", versionsPath(id, 1, "propose"), nil)
	tt.owner.ok("POST", versionsPath(id, 1, "approve"), map[string]any{})
	status, set := tt.owner.makeSet(id, map[string]any{"space_id": other, "title": "Moon"})
	if status != http.StatusCreated || len(set["pages"].([]any)) != 3 {
		t.Fatalf("the imported set: %d %v", status, set)
	}
	if set["page"].(map[string]any)["status"] != "published" {
		t.Fatalf("publishing at once travelled: %v", set["page"])
	}
}

func TestAnImportWhileThePartIsOffKeepsTodaysTemplatesAndRefusesGuidedOnes(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	root := admin(e)
	tpl := owner.guided(space, "Set", "Root")
	owner.entry(tpl, map[string]any{"name": "Child", "parent": "root", "document": para("x")})
	file := owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/export", nil)
	partOff(root)
	if out := owner.want(http.StatusNotFound, "POST", "/api/v1/spaces/"+space+"/templates/import", json.RawMessage(file)); !strings.Contains(out, "experimental_off") {
		t.Fatalf("a guided file while off: %s", out)
	}
	// A plain file is imported as before: in use at once for whoever manages templates.
	plain := owner.ok("POST", "/api/v1/spaces/"+space+"/templates/import", map[string]any{"name": "Plain", "document": para("x")})
	if plain["state"] != "approved" {
		t.Fatalf("a plain import while off: %v", plain)
	}
}
