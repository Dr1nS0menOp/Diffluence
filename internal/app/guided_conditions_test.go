// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// Spec 074, wave GT3: conditions, decided by the server, again on submit.

// when is a condition of tests, all or any of them.
func when(match string, tests ...map[string]any) map[string]any {
	list := []any{}
	for _, t := range tests {
		list = append(list, t)
	}
	return map[string]any{"match": match, "tests": list}
}

// check is a test of a question; a value of one item is the test's value, more are its list.
func check(field, op string, values ...string) map[string]any {
	t := map[string]any{"subject": "field", "field": field, "op": op}
	switch {
	case len(values) == 1:
		t["value"] = values[0]
	case len(values) > 1:
		t["values"] = values
	}
	return t
}

// sectionWhen is a section of text that a condition shows.
func sectionWhen(condition map[string]any, blocks ...map[string]any) map[string]any {
	content := []any{}
	for _, b := range blocks {
		content = append(content, b)
	}
	return map[string]any{"type": "conditional", "attrs": map[string]any{"condition": condition}, "content": content}
}

func paragraph(text string) map[string]any {
	return map[string]any{"type": "paragraph", "content": []any{words(text)}}
}

// setCondition gives an entry of a template its condition, or takes it away with nil.
func (u *user) setCondition(tpl, key string, condition map[string]any) (int, string) {
	u.e.t.Helper()
	return u.do("PUT", "/api/v1/templates/"+tpl+"/entries/"+key+"/condition", map[string]any{"condition": condition})
}

// madeTitles lists the titles of the pages a set made.
func madeTitles(made map[string]any) []string {
	out := []string{}
	pages, _ := made["pages"].([]any)
	for _, p := range pages {
		out = append(out, p.(map[string]any)["title"].(string))
	}
	return out
}

// Failure path first: a submit that answers a question its condition hides is treated as if
// that answer were never sent, also when it would be refused, and a hidden required question
// does not block (TPL-421, TPL-431, decided TPL-1406, TPL-1407).
func TestATamperedSubmitThatAnswersAHiddenFieldIsIgnored(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Visit",
		map[string]any{"name": "On site", "kind": "yesno"},
		map[string]any{"name": "Badge", "kind": "line", "required": true, "max_length": 5,
			"condition": when("all", check("On site", "yes"))})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Visit", "title_pattern": "Visit {Badge}",
		"document": blocksDoc(askAlone("On site"), askAlone("Badge"))})
	tampered := map[string]any{"On site": "no", "Badge": "TAMPERED-AND-FAR-TOO-LONG"}
	for _, route := range []string{"/sets/check", "/sets"} {
		status, out := editor.do("POST", "/api/v1/templates/"+tpl+route, map[string]any{"space_id": space, "answers": tampered})
		if status >= 300 {
			t.Fatalf("%s: an answer to a hidden question must be ignored, not checked: %d %s", route, status, out)
		}
		if strings.Contains(out, "TAMPERED") {
			t.Fatalf("%s: a hidden answer reached the set: %s", route, out)
		}
	}
	_, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"On site": "no", "Badge": "B-1"}})
	if text := madeText(editor, made); strings.Contains(text, "B-1") || strings.Contains(text, "templateField") {
		t.Fatalf("a hidden question is empty everywhere it is used: %s", text)
	}
	if title := made["page"].(map[string]any)["title"]; title != "Visit" {
		t.Fatalf("a hidden question is empty in a title too: %v", title)
	}
	// Shown, the question is required and checked as any other.
	for _, c := range []struct {
		answer, problem string
	}{{"", "required"}, {"B-123456", "too_long"}} {
		_, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space,
			"answers": map[string]any{"On site": "yes", "Badge": c.answer}})
		if got := refusedFields(t, out)["Badge"]; got != c.problem {
			t.Fatalf("shown, %q: %q, want %q (%s)", c.answer, got, c.problem, out)
		}
	}
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"On site": "yes", "Badge": "B-12"}})
	if status != http.StatusCreated || !strings.Contains(madeText(editor, made), "B-12") {
		t.Fatalf("shown and answered: %d %v", status, made)
	}
}

// A step a condition hides is not shown: its questions are hidden with it, their answers are
// dropped, and a required one does not block (TPL-330, TPL-421).
func TestAHiddenStepDropsTheAnswersOfItsQuestions(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Project",
		map[string]any{"name": "Pricing", "kind": "choice", "options": []string{"Fixed", "Hourly"}},
		map[string]any{"name": "Budget", "kind": "number", "required": true})
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{
		"fields": []any{map[string]any{"name": "Pricing", "kind": "choice", "options": []string{"Fixed", "Hourly"}},
			map[string]any{"name": "Budget", "kind": "number", "required": true}},
		"steps": []any{map[string]any{"title": "Kind", "fields": []string{"Pricing"}},
			map[string]any{"title": "Money", "fields": []string{"Budget"}, "condition": when("all", check("Pricing", "is", "Fixed"))}}})
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"Pricing": "Hourly", "Budget": "not a number"}})
	if status != http.StatusCreated || strings.Contains(madeText(editor, made), "not a number") {
		t.Fatalf("a hidden step: %d %v", status, made)
	}
	_, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets", map[string]any{"space_id": space, "answers": map[string]any{"Pricing": "Fixed"}})
	if got := refusedFields(t, out)["Budget"]; got != "required" {
		t.Fatalf("a shown step asks its required question: %s", out)
	}
}

// Failure path: a condition that tests a question the template does not ask is refused when
// the template is saved, whichever part holds it, and so is a change that removes a question
// a condition still tests (TPL-432).
func TestAConditionOnAQuestionThatNoLongerExistsIsRefusedAtSave(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Request",
		map[string]any{"name": "Topic", "kind": "line"},
		map[string]any{"name": "Notes", "kind": "line"})
	status, out := owner.do("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"steps": []any{}, "fields": []any{
		map[string]any{"name": "Topic", "kind": "line"},
		map[string]any{"name": "Notes", "kind": "line", "condition": when("all", check("Gone", "not_empty"))}}})
	if status != http.StatusBadRequest || !strings.Contains(out, "invalid_conditions") || refusedFields(t, out)["Notes"] != "unknown_field" {
		t.Fatalf("a question: %d %s", status, out)
	}
	plan := owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps")})
	if status, out := owner.setCondition(tpl, plan, when("all", check("Gone", "empty"))); status != http.StatusBadRequest ||
		refusedFields(t, out)["entry:"+plan] != "unknown_field" {
		t.Fatalf("an entry: %d %s", status, out)
	}
	if status, out := owner.do("PUT", "/api/v1/templates/"+tpl+"/rules", map[string]any{"label_rules": []any{
		map[string]any{"entry": "root", "label": "urgent", "when": when("any", check("Gone", "empty"))}}}); status != http.StatusBadRequest ||
		refusedFields(t, out)["rule:1"] != "unknown_field" {
		t.Fatalf("a label rule: %d %s", status, out)
	}
	if status, out := owner.do("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Request",
		"document": blocksDoc(askAlone("Topic"), sectionWhen(when("all", check("Gone", "yes")), paragraph("x")))}); status != http.StatusBadRequest ||
		refusedFields(t, out)["section:root"] != "unknown_field" {
		t.Fatalf("a section of text: %d %s", status, out)
	}
	// A condition on a question that exists is kept; removing the question is then refused,
	// so a condition never points at nothing.
	if status, out := owner.setCondition(tpl, plan, when("all", check("Notes", "not_empty"))); status != http.StatusNoContent {
		t.Fatalf("a good condition: %d %s", status, out)
	}
	status, out = owner.do("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Request", "document": blocksDoc(askAlone("Topic"))})
	if status != http.StatusBadRequest || refusedFields(t, out)["entry:"+plan] != "unknown_field" {
		t.Fatalf("removing a question a condition tests: %d %s", status, out)
	}
	versions := owner.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)["versions"].([]any)
	if len(versions) != 3 {
		t.Fatalf("a refused save makes no version: %d", len(versions))
	}
}

// Failure path: a question may only be shown by questions asked before it, so a condition on
// itself or on a later question, and so any cycle, is refused at save (TPL-421).
func TestACycleOrASelfReferenceIsRefusedAtSave(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Request",
		map[string]any{"name": "A", "kind": "line"}, map[string]any{"name": "B", "kind": "line"}, map[string]any{"name": "C", "kind": "line"})
	put := func(fields []any, steps []any) (int, map[string]string) {
		t.Helper()
		status, out := owner.do("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"fields": fields, "steps": steps})
		if status == http.StatusOK {
			return status, nil
		}
		return status, refusedFields(t, out)
	}
	line := func(name string, condition map[string]any) map[string]any {
		f := map[string]any{"name": name, "kind": "line"}
		if condition != nil {
			f["condition"] = condition
		}
		return f
	}
	if _, got := put([]any{line("A", when("all", check("A", "empty"))), line("B", nil), line("C", nil)}, []any{}); got["A"] != "later_field" {
		t.Fatalf("a question shown by itself: %v", got)
	}
	// A shown by B and B by A: A tests a later question, which refuses the cycle.
	if _, got := put([]any{line("A", when("all", check("B", "empty"))), line("B", when("all", check("A", "empty"))), line("C", nil)}, []any{}); got["A"] != "later_field" {
		t.Fatalf("a cycle: %v", got)
	}
	// A step may only test the questions of the steps before it.
	steps := []any{map[string]any{"title": "One", "fields": []string{"A", "B"}, "condition": when("all", check("A", "not_empty"))},
		map[string]any{"title": "Two", "fields": []string{"C"}}}
	if _, got := put([]any{line("A", nil), line("B", nil), line("C", nil)}, steps); got["step:1"] != "later_field" {
		t.Fatalf("a step shown by its own question: %v", got)
	}
	// The order of the form decides: C in the first step is asked before A and B of the second.
	steps = []any{map[string]any{"title": "One", "fields": []string{"C"}}, map[string]any{"title": "Two", "fields": []string{"A", "B"},
		"condition": when("all", check("C", "not_empty"))}}
	if status, got := put([]any{line("A", when("all", check("C", "is", "x"))), line("B", when("any", check("A", "empty"), check("C", "empty"))), line("C", nil)}, steps); status != http.StatusOK {
		t.Fatalf("earlier questions only: %d %v", status, got)
	}
}

// Failure path: a condition has at most ten tests, on one level, each with an operator its
// kind has, an option that exists and a group that exists (TPL-400, TPL-401, TPL-432).
func TestTheValidatorHoldsAConditionToItsShape(t *testing.T) {
	e := newEnv(t)
	owner, _, _, space := e.team("ENG")
	root := admin(e)
	tpl := owner.formTemplate(space, "Request",
		map[string]any{"name": "Team", "kind": "choice", "options": []string{"Red", "Blue"}},
		map[string]any{"name": "Hours", "kind": "number"})
	plan := owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps")})
	ten := []map[string]any{}
	for range 10 {
		ten = append(ten, check("Hours", "gt", "1"))
	}
	if status, out := owner.setCondition(tpl, plan, when("all", ten...)); status != http.StatusNoContent {
		t.Fatalf("ten tests: %d %s", status, out)
	}
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Contractors"})["id"].(string)
	gone := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Gone"})["id"].(string)
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/groups/"+gone, nil)
	for name, c := range map[string]struct {
		condition map[string]any
		problem   string
	}{
		"an eleventh test":            {when("all", append(ten, check("Hours", "gt", "1"))...), "too_many_tests"},
		"a group of tests in a test":  {when("all", when("any", check("Hours", "gt", "1"))), "bad_body"},
		"an operator of words":        {when("all", check("Hours", "contains", "4")), "bad_op"},
		"an expression as a value":    {when("all", check("Hours", "gt", "Hours * 2")), "bad_value"},
		"an option that is not there": {when("all", check("Team", "is", "Green")), "not_an_option"},
		"a deleted group":             {when("all", map[string]any{"subject": "groups", "op": "member", "value": gone}), "group_not_found"},
		"the space, in a space":       {when("all", map[string]any{"subject": "space", "op": "one_of", "values": []string{space}}), "space_not_offered"},
		"no way to combine":           {map[string]any{"tests": []any{check("Hours", "gt", "1")}}, "bad_match"},
	} {
		status, out := owner.setCondition(tpl, plan, c.condition)
		// A test holds no tests: the shape of a condition has no place for nesting at all.
		if c.problem == "bad_body" {
			if status != http.StatusBadRequest || strings.Contains(out, "invalid_conditions") {
				t.Errorf("%s: %d %s", name, status, out)
			}
			continue
		}
		if status != http.StatusBadRequest || refusedFields(t, out)["entry:"+plan] != c.problem {
			t.Errorf("%s: %d %s", name, status, out)
		}
	}
	if status, out := owner.setCondition(tpl, plan, when("all", map[string]any{"subject": "groups", "op": "member", "value": group})); status != http.StatusNoContent {
		t.Fatalf("a group that exists: %d %s", status, out)
	}
	// The first page is always made (TPL-420).
	if status, out := owner.setCondition(tpl, "root", when("all", check("Hours", "gt", "1"))); status != http.StatusBadRequest ||
		refusedFields(t, out)["entry:root"] != "root_condition" {
		t.Fatalf("the first page: %d %s", status, out)
	}
}

// An entry whose condition does not hold is not made, nor anything below it, and a link to it
// becomes the title it would have had, without a link (TPL-110, TPL-420).
func TestAnEntryLeftOutTakesItsChildrenAndItsLinksBecomeText(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Incident", map[string]any{"name": "Severity", "kind": "choice", "options": []string{"Low", "High"}})
	statement := owner.entry(tpl, map[string]any{"name": "Customer statement", "parent": "root", "document": para("Say sorry"),
		"title_pattern": "Statement for {Severity}"})
	owner.entry(tpl, map[string]any{"name": "Draft", "parent": statement, "document": para("Words")})
	owner.entry(tpl, map[string]any{"name": "Timeline", "parent": "root", "document": para("When")})
	if status, out := owner.setCondition(tpl, statement, when("all", check("Severity", "is", "High"))); status != http.StatusNoContent {
		t.Fatalf("condition: %d %s", status, out)
	}
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Incident", "document": blocksDoc(askAlone("Severity"),
		map[string]any{"type": "paragraph", "content": []any{words("See "), valueOf("entry:" + statement)}})})
	before := e.traceOf(space)
	checked, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": space, "title": "Incident",
		"answers": map[string]any{"Severity": "Low"}})
	if checked != http.StatusOK || strings.Contains(out, "Statement") || strings.Contains(out, `"Draft"`) {
		t.Fatalf("the check step lists only what is made: %d %s", checked, out)
	}
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "title": "Incident", "answers": map[string]any{"Severity": "Low"}})
	if status != http.StatusCreated || fmt.Sprint(madeTitles(made)) != "[Incident Timeline]" {
		t.Fatalf("left out: %d %v", status, madeTitles(made))
	}
	if after := e.traceOf(space); after.pages != before.pages+2 || after.origins != before.origins+2 {
		t.Fatalf("pages made: %+v, was %+v", after, before)
	}
	text := madeText(editor, made)
	if !strings.Contains(text, `"text":"Statement for Low"`) || strings.Contains(text, "pageLink") {
		t.Fatalf("a link to a page left out is its title as text: %s", text)
	}
	status, made = editor.makeSet(tpl, map[string]any{"space_id": space, "title": "Incident", "answers": map[string]any{"Severity": "High"}})
	if status != http.StatusCreated || fmt.Sprint(madeTitles(made)) != "[Incident Statement for High Draft Timeline]" ||
		!strings.Contains(madeText(editor, made), "pageLink") {
		t.Fatalf("made: %d %v", status, madeTitles(made))
	}
	// Taking the condition away makes the page always.
	if status, out := owner.setCondition(tpl, statement, nil); status != http.StatusNoContent {
		t.Fatalf("no condition: %d %s", status, out)
	}
	if _, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"Severity": "Low"}}); len(madeTitles(made)) != 4 {
		t.Fatalf("without a condition: %v", madeTitles(made))
	}
}

// A section whose condition holds is unwrapped and one whose condition does not is removed,
// nested ones too, and a file only a removed section shows is not copied. A page holding a
// section is refused (TPL-422).
func TestSectionsOfTextAreKeptOrRemovedWithTheirFiles(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	source := owner.page(space, nil, "Visit", "x")
	owner.upload(source, "shoes.txt", "safety shoes")
	owner.upload(source, "home.txt", "a desk at home")
	tpl := owner.ok("POST", "/api/v1/pages/"+source+"/save-as-template", map[string]any{"name": "Visit"})["id"].(string)
	fileOf := func(name string) map[string]any {
		id := e.scalar("SELECT a.id::text FROM attachments a JOIN templates t ON t.page_id = a.page_id WHERE t.id = $1 AND a.filename = $2", tpl, name)
		return map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": id, "filename": name}}
	}
	onSite, remote := when("all", check("On site", "yes")), when("all", check("Remote", "yes"))
	// The questions are asked and made yes/no first: a test of yes on a line is refused.
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Visit", "document": blocksDoc(askAlone("On site"), askAlone("Remote"))})
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{"steps": []any{}, "fields": []any{
		map[string]any{"name": "On site", "kind": "yesno"}, map[string]any{"name": "Remote", "kind": "yesno"}}})
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Visit", "document": blocksDoc(askAlone("On site"), askAlone("Remote"),
		sectionWhen(onSite, paragraph("Bring safety shoes"), fileOf("shoes.txt"), sectionWhen(remote, paragraph("And a laptop"))),
		sectionWhen(when("all", check("On site", "no")), paragraph("Work from home"), fileOf("home.txt")),
		paragraph("The end"))})
	filesOf := func(made map[string]any) []string {
		page := made["page"].(map[string]any)["id"].(string)
		out := []string{}
		for _, f := range editor.ok("GET", "/api/v1/pages/"+page+"/attachments", nil)["attachments"].([]any) {
			out = append(out, f.(map[string]any)["filename"].(string))
		}
		slices.Sort(out)
		return out
	}
	for _, c := range []struct {
		onSite, remote string
		has, hasNot    []string
		files          string
	}{
		{"yes", "no", []string{"Bring safety shoes", "The end"}, []string{"And a laptop", "Work from home"}, "[shoes.txt]"},
		{"yes", "yes", []string{"Bring safety shoes", "And a laptop"}, []string{"Work from home"}, "[shoes.txt]"},
		{"no", "yes", []string{"Work from home", "The end"}, []string{"Bring safety shoes", "And a laptop"}, "[home.txt]"},
	} {
		status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"On site": c.onSite, "Remote": c.remote}})
		if status != http.StatusCreated {
			t.Fatalf("%v: %d %v", c, status, made)
		}
		text := madeText(editor, made)
		for _, want := range c.has {
			if !strings.Contains(text, want) {
				t.Errorf("on site %s, remote %s: %q missing in %s", c.onSite, c.remote, want, text)
			}
		}
		for _, unwanted := range append(c.hasNot, "conditional") {
			if strings.Contains(text, unwanted) {
				t.Errorf("on site %s, remote %s: %q kept in %s", c.onSite, c.remote, unwanted, text)
			}
		}
		if got := fmt.Sprint(filesOf(made)); got != c.files {
			t.Errorf("on site %s: files %s, want %s", c.onSite, got, c.files)
		}
	}
	// A section never reaches a page: not in a draft, not in a published page.
	page := owner.ok("POST", "/api/v1/pages", map[string]any{"space_id": space, "title": "Plain"})["id"].(string)
	section := blocksDoc(sectionWhen(onSite, paragraph("x")))
	if status, out := owner.do("PUT", "/api/v1/pages/"+page+"/draft", map[string]any{"title": "Plain", "document": section, "base_version": 0}); status != http.StatusBadRequest {
		t.Fatalf("a draft with a section: %d %s", status, out)
	}
	if status, out := owner.do("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Plain", "document": section, "base_version": 0}); status != http.StatusBadRequest {
		t.Fatalf("a published page with a section: %d %s", status, out)
	}
}

// A label rule adds its label when its condition holds, or the label chosen in a question;
// a label off the space's fixed list is left out and reported (TPL-423).
func TestLabelRulesAddLabelsAndFollowTheFixedList(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Project",
		map[string]any{"name": "Pricing", "kind": "choice", "options": []string{"Fixed", "Hourly"}},
		map[string]any{"name": "Area", "kind": "label"})
	plan := owner.entry(tpl, map[string]any{"name": "Plan", "parent": "root", "document": para("Steps")})
	for name, c := range map[string]struct {
		rule    map[string]any
		problem string
	}{
		"no label":            {map[string]any{"entry": "root"}, "bad_rule"},
		"both":                {map[string]any{"entry": "root", "label": "a", "field": "Area"}, "bad_rule"},
		"a page not there":    {map[string]any{"entry": "gone", "label": "a"}, "rule_entry"},
		"a question of words": {map[string]any{"entry": "root", "field": "Pricing"}, "rule_field"},
		"a bad label":         {map[string]any{"entry": "root", "label": "two words!"}, "bad_rule"},
	} {
		status, out := owner.do("PUT", "/api/v1/templates/"+tpl+"/rules", map[string]any{"label_rules": []any{c.rule}})
		if status != http.StatusBadRequest || refusedFields(t, out)["rule:1"] != c.problem {
			t.Errorf("%s: %d %s", name, status, out)
		}
	}
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/rules", map[string]any{"label_rules": []any{
		map[string]any{"entry": "root", "label": "fixed-price", "when": when("all", check("Pricing", "is", "Fixed"))},
		map[string]any{"entry": plan, "field": "Area"},
		map[string]any{"entry": plan, "label": "planned"}}})
	if listed := owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/entries", nil); !strings.Contains(listed, `"label_rules":[{"entry":"root","label":"fixed-price"`) {
		t.Fatalf("the Rules section reads the rules: %s", listed)
	}
	labelsOf := func(page string) string {
		got := []string{}
		for _, l := range editor.ok("GET", "/api/v1/pages/"+page, nil)["labels"].([]any) {
			got = append(got, l.(string))
		}
		slices.Sort(got)
		return fmt.Sprint(got)
	}
	pageOf := func(made map[string]any, i int) string {
		return made["pages"].([]any)[i].(map[string]any)["id"].(string)
	}
	checked, out := editor.do("POST", "/api/v1/templates/"+tpl+"/sets/check", map[string]any{"space_id": space,
		"answers": map[string]any{"Pricing": "Fixed", "Area": "Team Alpha"}})
	if checked != http.StatusOK || !strings.Contains(out, "fixed-price") || !strings.Contains(out, "team-alpha") {
		t.Fatalf("the check step shows the labels of the rules: %s", out)
	}
	_, made := editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"Pricing": "Fixed", "Area": "Team Alpha"}})
	if got := labelsOf(pageOf(made, 0)); got != "[fixed-price]" {
		t.Fatalf("the first page: %s", got)
	}
	if got := labelsOf(pageOf(made, 1)); got != "[planned team-alpha]" {
		t.Fatalf("the plan: %s", got)
	}
	_, made = editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"Pricing": "Hourly"}})
	if labelsOf(pageOf(made, 0)) != "[]" || labelsOf(pageOf(made, 1)) != "[planned]" {
		t.Fatalf("hourly, no area: %s %s", labelsOf(pageOf(made, 0)), labelsOf(pageOf(made, 1)))
	}
	// A fixed list of the space keeps a rule's label out, and says so.
	owner.want(http.StatusNoContent, "PUT", "/api/v1/spaces/"+space+"/labels", map[string]any{"fixed": true, "labels": []string{"team-alpha"}})
	_, made = editor.makeSet(tpl, map[string]any{"space_id": space, "answers": map[string]any{"Pricing": "Fixed", "Area": "Team Alpha"}})
	if labelsOf(pageOf(made, 0)) != "[]" || fmt.Sprint(made["labels_left_out"]) != "[fixed-price planned]" {
		t.Fatalf("a fixed list: %s %v", labelsOf(pageOf(made, 0)), made["labels_left_out"])
	}
}

// A test of the creator's groups: a member gets the text, a non-member does not, and a group
// deleted after the save makes its tests false (TPL-402).
func TestConditionsOnTheGroupsOfTheCreator(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	root := admin(e)
	group := root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Contractors"})["id"].(string)
	root.want(http.StatusNoContent, "PUT", "/api/v1/admin/groups/"+group+"/members/"+editor.id, nil)
	member := map[string]any{"subject": "groups", "op": "member", "value": group}
	notMember := map[string]any{"subject": "groups", "op": "not_member", "value": group}
	tpl := owner.guided(space, "Onboarding", "Welcome")
	owner.ok("PATCH", "/api/v1/templates/"+tpl, map[string]any{"name": "Onboarding", "document": blocksDoc(paragraph("Welcome"),
		sectionWhen(when("all", member), paragraph("Your contract")), sectionWhen(when("all", notMember), paragraph("Your employment")))})
	// The editor names the group in the condition's sentence.
	if listed := owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/entries", nil); !strings.Contains(listed, `"`+group+`":"Contractors"`) {
		t.Fatalf("the names of tested groups: %s", listed)
	}
	textFor := func(u *user) string {
		t.Helper()
		status, made := u.makeSet(tpl, map[string]any{"space_id": space})
		if status != http.StatusCreated {
			t.Fatalf("make: %d %v", status, made)
		}
		return madeText(u, made)
	}
	if text := textFor(editor); !strings.Contains(text, "Your contract") || strings.Contains(text, "Your employment") {
		t.Fatalf("a member: %s", text)
	}
	if text := textFor(owner); strings.Contains(text, "Your contract") || !strings.Contains(text, "Your employment") {
		t.Fatalf("a non-member: %s", text)
	}
	root.want(http.StatusNoContent, "DELETE", "/api/v1/admin/groups/"+group, nil)
	for _, u := range []*user{editor, owner} {
		if text := textFor(u); strings.Contains(text, "Your contract") || strings.Contains(text, "Your employment") {
			t.Fatalf("a deleted group decides nothing: %s", text)
		}
	}
}

// Renaming a question renames it in the texts of every entry, in the title patterns, in its
// settings and steps, and in every condition, as one change and one new version (TPL-230).
func TestARenameOfAQuestionIsCarriedIntoItsConditions(t *testing.T) {
	e := newEnv(t)
	owner, editor, _, space := e.team("ENG")
	tpl := owner.formTemplate(space, "Visit", map[string]any{"name": "On site", "kind": "yesno"}, map[string]any{"name": "Badge", "kind": "line"})
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/fields", map[string]any{
		"fields": []any{map[string]any{"name": "On site", "kind": "yesno"},
			map[string]any{"name": "Badge", "kind": "line", "condition": when("all", check("On site", "yes"))}},
		"steps": []any{map[string]any{"title": "Where", "fields": []string{"On site"}},
			map[string]any{"title": "Badge", "fields": []string{"Badge"}, "condition": when("all", check("On site", "yes"))}}})
	shoes := owner.entry(tpl, map[string]any{"name": "Shoes", "parent": "root", "title_pattern": "Shoes {On site}",
		"document": blocksDoc(askAlone("On site"), sectionWhen(when("all", check("On site", "yes")), paragraph("Bring shoes")))})
	desk := owner.entry(tpl, map[string]any{"name": "Desk", "parent": "root", "document": blocksDoc(askAlone("On site"))})
	owner.setCondition(tpl, desk, when("all", check("On site", "no")))
	owner.ok("PUT", "/api/v1/templates/"+tpl+"/rules", map[string]any{"label_rules": []any{
		map[string]any{"entry": shoes, "label": "visit", "when": when("all", check("On site", "yes"))}}})
	versions := len(owner.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)["versions"].([]any))
	// Failure paths first: a name in use, a reserved name, a question that is not asked.
	for _, c := range []struct{ from, to, code string }{{"On site", "badge", "name_taken"}, {"On site", "Date", "reserved_name"},
		{"Gone", "New", "not_asked"}, {"On site", "", "bad_name"}} {
		if status, out := owner.do("POST", "/api/v1/templates/"+tpl+"/fields/rename", map[string]any{"from": c.from, "to": c.to}); status != http.StatusBadRequest ||
			!strings.Contains(out, c.code) {
			t.Errorf("%s to %q: %d %s", c.from, c.to, status, out)
		}
	}
	owner.ok("POST", "/api/v1/templates/"+tpl+"/fields/rename", map[string]any{"from": "On site", "to": "At the office"})
	if after := len(owner.ok("GET", "/api/v1/templates/"+tpl+"/versions", nil)["versions"].([]any)); after != versions+1 {
		t.Fatalf("one new version: %d, was %d", after, versions)
	}
	got := owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl, nil)
	if strings.Contains(got, "On site") || !strings.Contains(got, "At the office") {
		t.Fatalf("the form: %s", got)
	}
	for _, key := range []string{"root", shoes, desk} {
		if entry := owner.want(http.StatusOK, "GET", "/api/v1/templates/"+tpl+"/entries/"+key, nil); strings.Contains(entry, "On site") {
			t.Fatalf("entry %s still asks the old name: %s", key, entry)
		}
	}
	status, made := editor.makeSet(tpl, map[string]any{"space_id": space, "title": "Visit", "answers": map[string]any{"At the office": "yes", "Badge": "B-7"}})
	if status != http.StatusCreated || fmt.Sprint(madeTitles(made)) != "[Visit Shoes Yes]" || !strings.Contains(madeText(editor, made), "B-7") {
		t.Fatalf("made after the rename: %d %v", status, madeTitles(made))
	}
	shoesPage := made["pages"].([]any)[1].(map[string]any)["id"].(string)
	if labels := fmt.Sprint(editor.ok("GET", "/api/v1/pages/"+shoesPage, nil)["labels"]); labels != "[visit]" {
		t.Fatalf("the rule follows the rename: %s", labels)
	}
}
