// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Spec 074, wave GT2: the kinds of field, their checks on the server, the form bound to its
// version, and the check step.

// ask is a question in a template's text.
func ask(name string) map[string]any {
	return map[string]any{"type": "templateField", "attrs": map[string]any{"name": name, "kind": "line"}}
}

// askAlone is a question that stands alone in its paragraph.
func askAlone(name string) map[string]any {
	return map[string]any{"type": "paragraph", "content": []any{ask(name)}}
}

// blocksDoc is a text of paragraphs.
func blocksDoc(blocks ...map[string]any) map[string]any {
	content := []any{}
	for _, b := range blocks {
		content = append(content, b)
	}
	return map[string]any{"type": "doc", "content": content}
}

// formTemplate makes a template of a space whose text asks each named question alone in
// its paragraph, and gives the questions their settings.
func (u *user) formTemplate(space, name string, specs ...map[string]any) string {
	u.e.t.Helper()
	blocks := []map[string]any{}
	for _, s := range specs {
		blocks = append(blocks, askAlone(s["name"].(string)))
	}
	tpl := u.ok("POST", "/api/v1/spaces/"+space+"/templates", map[string]any{"name": name, "document": blocksDoc(blocks...)})["id"].(string)
	u.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": specs, "steps": []any{}})
	return tpl
}

// refusedFields gives each refused field with its problem, from an error answer.
func refusedFields(t *testing.T, out string) map[string]string {
	t.Helper()
	var body struct {
		Error struct {
			Code   string `json:"code"`
			Fields []struct {
				Field, Problem, Message string
			} `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &body); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	got := map[string]string{}
	for _, f := range body.Error.Fields {
		if f.Message == "" {
			t.Fatalf("a refused field without words: %s", out)
		}
		got[f.Field] = f.Problem
	}
	return got
}

// madeText reads the text of the first page a set made, as its draft holds it, written
// compactly in the order of a document's own fields.
func madeText(u *user, made map[string]any) string {
	u.e.t.Helper()
	page := made["page"].(map[string]any)["id"].(string)
	doc, err := document.Parse([]byte(u.e.scalar("SELECT document::text FROM drafts WHERE page_id = $1", page)))
	if err != nil {
		u.e.t.Fatal(err)
	}
	raw, _ := json.Marshal(doc)
	return string(raw)
}

// everyKind is a field of each kind, with bounds where the kind has them.
func everyKind() []map[string]any {
	return []map[string]any{
		{"name": "Name", "kind": "line", "required": true, "max_length": 10},
		{"name": "Email", "kind": "line", "format": "email"},
		{"name": "Story", "kind": "text", "min_length": 3},
		{"name": "Team", "kind": "choice", "options": []string{"Red", "Blue"}},
		{"name": "Hours", "kind": "number", "min": 1, "max": 40, "decimals": 1},
		{"name": "Start", "kind": "date", "earliest": map[string]any{"fixed": "2027-03-01"}, "latest": map[string]any{"fixed": "2030-12-31"}},
		{"name": "Remote", "kind": "yesno"},
		{"name": "Handbook", "kind": "page"},
		{"name": "Buddy", "kind": "person"},
		{"name": "Panel", "kind": "people"},
		{"name": "Tools", "kind": "choices", "options": []string{"Laptop", "Phone", "Badge"}, "max_choices": 2},
		{"name": "Site", "kind": "link"},
		{"name": "Area", "kind": "label"},
	}
}

// Failure paths first: every value the form would stop is refused by the server as well,
// each named with its problem, and nothing is made (TPL-320, TPL-324).
func TestTheServerRefusesEveryAnswerTheFormWouldStop(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Onboarding", everyKind()...)
	before := e.traceOf(space)
	bad := map[string]any{
		"Name":     "far too long a name",
		"Email":    "not an address",
		"Story":    "ab",
		"Team":     "Green",
		"Hours":    "40.25",
		"Start":    "2027-02-29",
		"Remote":   "maybe",
		"Handbook": "home",
		"Buddy":    "someone",
		"Panel":    editor.id + "\n" + editor.id,
		"Tools":    "Laptop\nPhone\nBadge",
		"Site":     "javascript:alert(1)",
		"Area":     "a+b",
	}
	want := map[string]string{"Name": "too_long", "Email": "not_an_email", "Story": "too_short", "Team": "not_an_option",
		"Hours": "too_many_decimals", "Start": "not_a_date", "Remote": "not_yes_no", "Handbook": "not_an_id", "Buddy": "not_an_id",
		"Panel": "repeated", "Tools": "too_many_choices", "Site": "not_a_link", "Area": "not_a_label"}
	for _, route := range []string{"/sets", "/sets/check"} {
		status, out := editor.do("POST", "/api/v1/templates/"+tpl+route, map[string]any{"space_id": space, "answers": bad})
		if status != http.StatusBadRequest || !strings.Contains(out, "invalid_answers") {
			t.Fatalf("%s: %d %s", route, status, out)
		}
		if got := refusedFields(t, out); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s refused %v, want %v", route, got, want)
		}
	}
	// The bounds hold on both sides: just below and just above.
	for _, c := range []struct{ field, value, problem string }{
		{"Hours", "0.9", "below_min"}, {"Hours", "40.1", "above_max"},
		{"Start", "2027-02-28", "too_early"}, {"Start", "2031-01-01", "too_late"},
	} {
		_, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space,
			"answers": map[string]any{"Name": "Ada", c.field: c.value}})
		if got := refusedFields(t, out)[c.field]; got != c.problem {
			t.Fatalf("%s = %s: %q, want %q (%s)", c.field, c.value, got, c.problem, out)
		}
	}
	if after := e.traceOf(space); after != before {
		t.Fatalf("a refused answer left a trace: %+v, was %+v", after, before)
	}
}

func TestARequiredQuestionIsCheckedOnTheServer(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Request",
		map[string]any{"name": "Topic", "kind": "line", "required": true},
		map[string]any{"name": "Owner", "kind": "person", "required": true, "default": map[string]any{"creator": true}},
		map[string]any{"name": "Due", "kind": "date", "required": true, "default": map[string]any{"date": map[string]any{"offset": 2, "unit": "week"}}})
	for _, answers := range []map[string]any{{}, {"Topic": ""}, {"Topic": "  \n "}} {
		status, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "answers": answers})
		if got := refusedFields(t, out); status != http.StatusBadRequest || fmt.Sprint(got) != "map[Topic:required]" {
			t.Fatalf("%v: %d %v", answers, status, got)
		}
	}
	// The plain route checks alike.
	if out := editor.want(http.StatusBadRequest, "POST", "/api/v1/templates/"+tpl+"/use", map[string]any{"space_id": space}); !strings.Contains(out, `"field":"Topic"`) {
		t.Fatalf("use: %s", out)
	}
	// A default counts as the answer when the question is left out, so it never blocks; an
	// answer given empty is an answer, and a required one refuses.
	status, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space,
		"answers": map[string]any{"Topic": "Laptops", "Owner": ""}})
	if got := refusedFields(t, out); status != http.StatusBadRequest || got["Owner"] != "required" {
		t.Fatalf("an emptied default: %d %v", status, got)
	}
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"Topic": "Laptops"}})
	if status != http.StatusCreated {
		t.Fatalf("defaults: %d %v", status, made)
	}
	text := madeText(editor, made)
	if !strings.Contains(text, `"mention"`) || !strings.Contains(text, editor.id) || !strings.Contains(text, `"type":"date"`) {
		t.Fatalf("the defaults did not land: %s", text)
	}
}

// A page, a person or people the creator may not see are refused as not found, and never
// named; a guest the creator shares a space with may be chosen; a disabled person may not
// (TPL-306 to TPL-308).
func TestPagesAndPeopleAreCheckedAgainstWhatTheCreatorMaySee(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	other, _ := owner.space("HR")
	secret := owner.page(other, nil, "Salaries of everyone", "x")
	gone := owner.page(space, nil, "Old handbook", "x")
	owner.want(http.StatusNoContent, "DELETE", "/api/v1/pages/"+gone, nil)
	handbook := owner.page(space, nil, "Handbook", "x")
	elsewhere := e.user("elsewhere@example.com", identity.PermUseProduct)
	left := e.user("left@example.com", identity.PermUseProduct)
	owner.grant(space, "user", left.id, permissions.TemplateViewer)
	e.sql("UPDATE users SET status = 'disabled' WHERE id = $1", left.id)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	owner.grant(space, "user", guest.id, permissions.TemplateViewer)
	tpl := owner.formTemplate(space, "Onboarding",
		map[string]any{"name": "Handbook", "kind": "page"},
		map[string]any{"name": "Local", "kind": "page", "space_id": space},
		map[string]any{"name": "Buddy", "kind": "person"},
		map[string]any{"name": "Panel", "kind": "people"})
	try := func(answers map[string]any) map[string]string {
		t.Helper()
		_, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "answers": answers})
		if strings.Contains(out, "Salaries") {
			t.Fatalf("a page the creator may not see was named: %s", out)
		}
		return refusedFields(t, out)
	}
	for _, c := range []struct {
		answers map[string]any
		want    string
	}{
		{map[string]any{"Handbook": secret}, "map[Handbook:page_not_found]"},
		{map[string]any{"Handbook": gone}, "map[Handbook:page_not_found]"},
		{map[string]any{"Handbook": "01938f2e-7b3a-7cde-8f00-0123456789ab"}, "map[Handbook:page_not_found]"},
		{map[string]any{"Local": secret}, "map[Local:page_not_found]"},
		{map[string]any{"Buddy": elsewhere.id}, "map[Buddy:person_not_found]"},
		{map[string]any{"Buddy": left.id}, "map[Buddy:person_inactive]"},
		{map[string]any{"Panel": owner.id + "\n" + left.id}, "map[Panel:person_inactive]"},
		{map[string]any{"Panel": elsewhere.id}, "map[Panel:person_not_found]"},
	} {
		if got := fmt.Sprint(try(c.answers)); got != c.want {
			t.Fatalf("%v: %s, want %s", c.answers, got, c.want)
		}
	}
	// A page of another space, which the creator may see, is refused where the field
	// names a space.
	elsewherePage := owner.page(other, nil, "Shared", "x")
	owner.grant(other, "user", editor.id, permissions.TemplateViewer)
	if got := fmt.Sprint(try(map[string]any{"Local": elsewherePage})); got != "map[Local:page_other_space]" {
		t.Fatalf("another space: %s", got)
	}
	// A guest who shares the space can be chosen, and lands as a mention; the title uses names.
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{
		"Handbook": handbook, "Local": handbook, "Buddy": guest.id, "Panel": owner.id + "\n" + guest.id}})
	if status != http.StatusCreated {
		t.Fatalf("a guest in the space: %d %v", status, made)
	}
	text := madeText(editor, made)
	for _, want := range []string{`"pageLink"`, handbook, guest.id, owner.id} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}

// A form is bound to the version it showed: a newer version with a new required question
// and another kind does not refuse it, and the page has the older text (TPL-603).
func TestAFormBoundToAnOlderVersionMakesThatVersion(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Request", map[string]any{"name": "Size", "kind": "line"})
	first := int(editor.ok("GET", "/api/v1/templates/"+tpl, nil)["template_version"].(float64))
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Request",
		"document": blocksDoc(askAlone("Size"), askAlone("Reason"), map[string]any{"type": "paragraph", "content": []any{words("Version two")}})})
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": []any{
		map[string]any{"name": "Size", "kind": "number"}, map[string]any{"name": "Reason", "kind": "line", "required": true}}})
	form := editor.ok("GET", "/api/v1/templates/"+tpl, nil)
	if got := int(form["template_version"].(float64)); got <= first {
		t.Fatalf("no newer version: %d", got)
	}
	// Bound to the first version, "large" is a line answer and Reason is not asked.
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "version": first, "answers": map[string]any{"Size": "large"}})
	if status != http.StatusCreated {
		t.Fatalf("bound to version %d: %d %v", first, status, made)
	}
	if text := madeText(editor, made); strings.Contains(text, "Version two") || !strings.Contains(text, "large") {
		t.Fatalf("not the older text: %s", text)
	}
	// The same answers against the newest version are refused, each named.
	_, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "answers": map[string]any{"Size": "large"}})
	if got := fmt.Sprint(refusedFields(t, out)); got != "map[Reason:required Size:not_a_number]" {
		t.Fatalf("newest: %s", got)
	}
	// The plain route takes the version too, and a version that never was is not found.
	editor.ok("POST", "/api/v1/templates/"+tpl+"/use", map[string]any{"space_id": space, "version": first, "answers": map[string]any{"Size": "large"}})
	editor.want(http.StatusNotFound, "POST", "/api/v1/templates/"+tpl+"/use", map[string]any{"space_id": space, "version": 99})
}

// The settings of a question are checked before they are saved, and a default that no
// longer fits is refused (TPL-305, TPL-316).
func TestTheSettingsOfAQuestionAreCheckedBeforeTheyAreSaved(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Request", map[string]any{"name": "Team", "kind": "choice", "options": []string{"Red", "Blue"},
		"default": map[string]any{"value": "Blue"}})
	refuse := func(specs []any, want string) {
		t.Helper()
		out := owner.want(http.StatusBadRequest, "PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": specs})
		if got := fmt.Sprint(refusedFields(t, out)); got != want {
			t.Fatalf("%v: %s, want %s", specs, got, want)
		}
	}
	// The option the default names was removed.
	refuse([]any{map[string]any{"name": "Team", "kind": "choice", "options": []string{"Red"}, "default": map[string]any{"value": "Blue"}}},
		"map[Team:bad_default]")
	refuse([]any{map[string]any{"name": "Team", "kind": "yesno", "required": true}}, "map[Team:yesno_required]")
	refuse([]any{map[string]any{"name": "Nothing", "kind": "line"}}, "map[Nothing:not_asked]")
	refuse([]any{map[string]any{"name": "Team", "kind": "line", "options": []string{"x"}}}, "map[Team:setting_not_for_kind]")
	refuse([]any{map[string]any{"name": "Team", "kind": "signature"}}, "map[Team:unknown_kind]")
	// Someone who may not manage the template may not change its questions.
	if status, _ := editor.do("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": []any{}}); status != http.StatusForbidden {
		t.Fatalf("an editor changed the questions: %d", status)
	}
	// A suggested person who leaves after the save: the form shows the field empty with a
	// note, the server treats it as no answer, and the next save is refused.
	left := e.user("left@example.com", identity.PermUseProduct)
	owner.grant(space, "user", left.id, permissions.TemplateViewer)
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Request", "document": blocksDoc(askAlone("Team"), askAlone("Lead"))})
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": []any{
		map[string]any{"name": "Lead", "kind": "person", "required": true, "default": map[string]any{"value": left.id}}}})
	e.sql("UPDATE users SET status = 'disabled' WHERE id = $1", left.id)
	form := editor.ok("GET", "/api/v1/templates/"+tpl, nil)
	raw, _ := json.Marshal(form["fields"])
	if !strings.Contains(string(raw), `"default_gone":true`) || strings.Contains(string(raw), left.id) {
		t.Fatalf("the form still suggests someone who left: %s", raw)
	}
	_, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "answers": map[string]any{"Team": "Red"}})
	if got := fmt.Sprint(refusedFields(t, out)); got != "map[Lead:required]" {
		t.Fatalf("a default of someone who left: %s", got)
	}
	out = owner.want(http.StatusBadRequest, "PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Request 2"})
	if got := fmt.Sprint(refusedFields(t, out)); got != "map[Lead:default_inactive]" {
		t.Fatalf("the next save: %s", got)
	}
}

// What lands in the page depends on the kind (TPL-212), and every answer stays content the
// product makes, never markup from the person (TPL-842).
func TestEachKindLandsInThePageAsItsOwnBlock(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Onboarding",
		map[string]any{"name": "Story", "kind": "text"},
		map[string]any{"name": "Tools", "kind": "choices", "options": []string{"Laptop", "Phone"}, "show": "list"},
		map[string]any{"name": "Start", "kind": "date"},
		map[string]any{"name": "Remote", "kind": "yesno"},
		map[string]any{"name": "Site", "kind": "link"},
		map[string]any{"name": "Hours", "kind": "number", "decimals": 1})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding", "title_pattern": "{Start} for {Remote}"})
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{
		"Story": "First line\nsecond line\n\n<b>New</b> paragraph", "Tools": "Laptop\nPhone", "Start": "2028-02-29",
		"Remote": "yes", "Site": "https://example.org", "Hours": "7,5"}})
	if status != http.StatusCreated {
		t.Fatalf("%d %v", status, made)
	}
	text := madeText(editor, made)
	for _, want := range []string{`"hardBreak"`, `"text":"` + strings.NewReplacer("<", `\u003c`, ">", `\u003e`).Replace("<b>New</b> paragraph") + `"`, `"bulletList"`,
		`{"type":"date","attrs":{"value":"2028-02-29"}}`, `"text":"Yes"`, `"text":"https://example.org"`, `"text":"7,5"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
	if strings.Count(text, `"type":"paragraph"`) < 8 || strings.Contains(text, "templateField") {
		t.Fatalf("the paragraphs of the text did not land as paragraphs: %s", text)
	}
	if title := made["page"].(map[string]any)["title"]; title != "2028-02-29 for Yes" {
		t.Fatalf("title: %v", title)
	}
}

// The check step shows the pages, their labels and restrictions, whether they are drafts,
// and the titles that already exist (TPL-340).
func TestTheCheckStepListsWhatTheSetWillMake(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Onboarding", map[string]any{"name": "Name", "kind": "line", "required": true})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding", "title_pattern": "Welcome {Name}", "labels": []string{"onboarding"}})
	owner.entry(tpl, map[string]any{"name": "Manager notes", "parent": "root", "document": para("Private"), "labels": []string{"private"},
		"restrictions": map[string]any{"view": []any{map[string]any{"type": "user", "id": owner.id}}}})
	owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps")})
	owner.page(space, nil, "Plan", "already here")
	check := owner.ok("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": space, "answers": map[string]any{"Name": "Ada"}})
	raw, _ := json.Marshal(check)
	got := string(raw)
	for _, want := range []string{`"title":"Welcome Ada"`, `"labels":["onboarding"]`, `"restricted":"view"`, `"labels":["private"]`,
		`"publish":false`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	pages := check["pages"].([]any)
	if len(pages) != 3 || !pages[2].(map[string]any)["exists"].(bool) || pages[0].(map[string]any)["exists"].(bool) {
		t.Fatalf("pages: %v", pages)
	}
	// An editor without the right to restrict is refused before the check step lists anything.
	if status, _ := editor.do("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": space,
		"answers": map[string]any{"Name": "Ada"}}); status != http.StatusForbidden {
		t.Fatalf("check without the right to restrict: %d", status)
	}
}

// The dry run of the check step and the real run give the same pages, or the same refusal,
// for 200 random answer sets (TPL-341).
func TestTheDryRunAndTheRealRunAgree(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Project",
		map[string]any{"name": "Project", "kind": "line", "required": true, "max_length": 12},
		map[string]any{"name": "Size", "kind": "number", "min": 1, "max": 9},
		map[string]any{"name": "Kick-off", "kind": "date", "default": map[string]any{"date": map[string]any{"offset": 1, "unit": "week"}}},
		map[string]any{"name": "Kind", "kind": "choice", "options": []string{"Internal", "Client"}})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Project", "title_pattern": "{Project} ({Kind})", "labels": []string{"project"}})
	owner.entry(tpl, map[string]any{"name": "Risks", "parent": "root", "document": blocksDoc(askAlone("Project")), "title_pattern": "Risks of {Project}"})
	owner.entry(tpl, map[string]any{"name": "Folder", "parent": "root", "kind": "folder", "title_pattern": "{Size} people"})
	rng := rand.New(rand.NewPCG(74, 341))
	pick := func(options ...string) string { return options[rng.IntN(len(options))] }
	made := 0
	for i := range 200 {
		answers := map[string]any{}
		if rng.IntN(5) > 0 {
			answers["Project"] = pick("Apollo", "Gemini", "", "A name longer than twelve", "Mercury "+fmt.Sprint(i))
		}
		if rng.IntN(2) == 0 {
			answers["Size"] = pick("1", "9", "10", "0", "3,5", "x", "")
		}
		if rng.IntN(3) == 0 {
			answers["Kick-off"] = pick("2027-02-29", "2028-02-29", "", "soon")
		}
		answers["Kind"] = pick("Internal", "Client", "Other", "")
		in := map[string]any{"space_id": space, "answers": answers, "request_key": fmt.Sprintf("run-%d", i)}
		checkStatus, check := owner.do("POST", "/api/v1/templates/"+tpl+"/sets/check", in)
		realStatus, real := owner.do("POST", "/api/v1/templates/"+tpl+"/sets", in)
		if checkStatus != http.StatusOK {
			if realStatus != checkStatus || fmt.Sprint(refusedFields(t, check)) != fmt.Sprint(refusedFields(t, real)) {
				t.Fatalf("answers %v: the dry run refused %d %s, the real run %d %s", answers, checkStatus, check, realStatus, real)
			}
			continue
		}
		if realStatus != http.StatusCreated {
			t.Fatalf("answers %v: the dry run passed, the real run %d %s", answers, realStatus, real)
		}
		made++
		var dry, wet struct {
			Pages []struct {
				Entry, Title string
				ParentEntry  string `json:"parent_entry"`
				Labels       []string
				Restricted   string
			} `json:"pages"`
		}
		_ = json.Unmarshal([]byte(check), &dry)
		_ = json.Unmarshal([]byte(real), &wet)
		if fmt.Sprintf("%+v", dry.Pages) != fmt.Sprintf("%+v", wet.Pages) || len(dry.Pages) != 3 {
			t.Fatalf("answers %v: dry %+v, real %+v", answers, dry.Pages, wet.Pages)
		}
		// The pages that were made are the ones the dry run listed, by title.
		var root struct {
			Page struct{ ID, Title string }
		}
		_ = json.Unmarshal([]byte(real), &root)
		if root.Page.Title != dry.Pages[0].Title {
			t.Fatalf("made %q, listed %q", root.Page.Title, dry.Pages[0].Title)
		}
	}
	if made < 20 || made > 180 {
		t.Fatalf("the answers did not mix valid and invalid sets well: %d made", made)
	}
}

// Steps are written on the template screen (ORG-061, TPL-330): at most ten, each with a
// title, each question in one step.
func TestTheStepsOfAFormFollowTheirRules(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.guided(space, "Request", "x")
	base := map[string]any{"name": "Request"}
	with := func(steps []any) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		out["steps"] = steps
		return out
	}
	eleven := []any{}
	for i := range 11 {
		eleven = append(eleven, map[string]any{"title": fmt.Sprintf("Step %d", i), "fields": []string{}})
	}
	for _, steps := range [][]any{
		eleven,
		{map[string]any{"title": " ", "fields": []string{}}},
		{map[string]any{"title": strings.Repeat("s", 101), "fields": []string{}}},
		{map[string]any{"title": "One", "fields": []string{"Topic"}}, map[string]any{"title": "Two", "fields": []string{"Topic"}}},
	} {
		owner.want(http.StatusBadRequest, "PATCH", "/api/v1/templates/"+tpl, with(steps))
	}
	ten := eleven[:10]
	got := owner.ok("PATCH", "/api/v1/templates/"+tpl, with(ten))["steps"].([]any)
	if len(got) != 10 {
		t.Fatalf("steps: %v", got)
	}
}

// The versions of every template can be listed and compared, also while guided templates
// are off (ORG-060).
func TestTheVersionsOfATemplateCanBeComparedPageByPage(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Minutes", "Agenda")
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Minutes", "document": para("Agenda and actions")})
	list := owner.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)["versions"].([]any)
	newest := list[0].(map[string]any)
	if len(list) != 2 || newest["author_name"] != "Test owner@example.com" || newest["pages"].(float64) != 1 {
		t.Fatalf("versions: %v", list)
	}
	changes := owner.ok("GET", "/api/v1/templates/"+tpl+"/versions/2/changes", nil)
	raw, _ := json.Marshal(changes)
	if !strings.Contains(string(raw), `"change":"changed"`) || !strings.Contains(string(raw), "actions") || changes["previous"].(float64) != 1 {
		t.Fatalf("changes: %s", raw)
	}
	first := owner.ok("GET", "/api/v1/templates/"+tpl+"/versions/1/changes", nil)
	if raw, _ := json.Marshal(first); !strings.Contains(string(raw), `"change":"added"`) {
		t.Fatalf("the first version: %s", raw)
	}
	owner.want(http.StatusNotFound, "GET", "/api/v1/templates/"+tpl+"/versions/3/changes", nil)
	owner.want(http.StatusNotFound, "GET", "/api/v1/templates/"+tpl+"/versions/0/changes", nil)
	// Only its managers see the history.
	if status, _ := editor.do("GET", "/api/v1/templates/"+tpl+"/versions/2/changes", nil); status != http.StatusForbidden {
		t.Fatalf("an editor read the history: %d", status)
	}
	if !slices.Contains([]int{http.StatusForbidden, http.StatusNotFound}, func() int {
		s, _ := editor.do("GET", "/api/v1/templates/"+tpl+"/versions", nil)
		return s
	}()) {
		t.Fatal("an editor listed the versions")
	}
}

// Saving the Form section with nothing beyond what the text says keeps a template plain,
// so it stays offered while guided templates are off (TPL-050).
func TestSavingPlainQuestionsKeepsATemplatePlain(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Minutes", "x")
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Minutes", "document": blocksDoc(askAlone("Topic"))})
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": []any{map[string]any{"name": "Topic", "kind": "line", "required": false}},
		"steps": []any{map[string]any{"title": "About", "fields": []string{"Topic"}}}})
	if got := editor.ok("GET", "/api/v1/templates/"+tpl, nil); got["guided"] != false || len(got["steps"].([]any)) != 1 {
		t.Fatalf("plain questions made the template guided: %v", got)
	}
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": []any{map[string]any{"name": "Topic", "kind": "line", "help": "What it is about"}}})
	if got := editor.ok("GET", "/api/v1/templates/"+tpl, nil); got["guided"] != true {
		t.Fatalf("help text is a part of guided templates: %v", got)
	}
}

// The Pages section moves entries with buttons instead of dragging (TPL-1011).
func TestEntriesMoveUpDownInAndOut(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.guided(space, "Project", "x")
	a := owner.entry(tpl, map[string]any{"name": "A", "parent": "root"})
	b := owner.entry(tpl, map[string]any{"name": "B", "parent": "root"})
	order := func() string {
		t.Helper()
		out := []string{}
		for _, raw := range owner.ok("GET", "/api/v1/templates/"+tpl+"/entries", nil)["entries"].([]any) {
			en := raw.(map[string]any)
			parent := ""
			if en["parent"] == a {
				parent = "A>"
			}
			out = append(out, parent+fmt.Sprint(en["name"]))
		}
		return strings.Join(out, " ")
	}
	move := func(key, to string, status int) {
		t.Helper()
		owner.want(status, "POST", "/api/v1/templates/"+tpl+"/entries/"+key+"/move", map[string]any{"to": to})
	}
	move(b, "up", http.StatusNoContent)
	if got := order(); got != " B A" {
		t.Fatalf("up: %q", got)
	}
	move(b, "up", http.StatusConflict)
	move(b, "down", http.StatusNoContent)
	move(b, "in", http.StatusNoContent)
	if got := order(); got != " A A>B" {
		t.Fatalf("in: %q", got)
	}
	move(a, "in", http.StatusConflict)
	move(b, "out", http.StatusNoContent)
	if got := order(); got != " A B" {
		t.Fatalf("out: %q", got)
	}
	move(b, "out", http.StatusConflict)
	move("root", "down", http.StatusConflict)
	move(b, "sideways", http.StatusBadRequest)
	if status, _ := editor.do("POST", "/api/v1/templates/"+tpl+"/entries/"+b+"/move", map[string]any{"to": "up"}); status != http.StatusForbidden {
		t.Fatalf("an editor moved an entry: %d", status)
	}
}

// A label field takes a label of the space's fixed list, or any label where there is none
// (TPL-311).
func TestALabelFieldFollowsTheFixedListOfTheSpace(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Request", map[string]any{"name": "Area", "kind": "label"})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Request", "title_pattern": "Area {Area}",
		"document": blocksDoc(askAlone("Area"))})
	if status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"Area": "Team Alpha"}}); status != http.StatusCreated ||
		made["page"].(map[string]any)["title"] != "Area team-alpha" {
		t.Fatalf("any label without a fixed list: %d %v", status, made)
	}
	owner.want(http.StatusNoContent, "PUT", "/api/v1/spaces/"+space+"/labels", map[string]any{"fixed": true, "labels": []string{"platform"}})
	_, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "answers": map[string]any{"Area": "Team Alpha"}})
	if got := fmt.Sprint(refusedFields(t, out)); got != "map[Area:label_not_allowed]" {
		t.Fatalf("a label off the fixed list: %s", got)
	}
	editor.ok("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "answers": map[string]any{"Area": "Platform"}})
}
