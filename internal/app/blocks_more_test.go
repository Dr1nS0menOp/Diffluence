// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// rowTitles reads the titles of a block's answer.
func rowTitles(answer map[string]any) []string {
	out := []string{}
	for _, row := range answer["rows"].([]any) {
		out = append(out, row.(map[string]any)["title"].(string))
	}
	return out
}

func hasTitle(answer map[string]any, title string) bool {
	for _, got := range rowTitles(answer) {
		if got == title {
			return true
		}
	}
	return false
}

// hiddenPageFixture gives a reader who sees one page of a space and not the other.
type hiddenPageFixture struct {
	owner, reader *user
	space, home   string
	open, secret  string
}

func newHiddenPageFixture(t *testing.T, e *env) hiddenPageFixture {
	t.Helper()
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, home := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, home, "Open page", "x")
	secret := owner.page(space, home, "Secret page", "x")
	owner.publish(open, "Open page", 1, para("Words in the open"))
	owner.publish(secret, "Secret page", 1, para("Words in the secret"))
	owner.ok("POST", "/api/v1/pages/"+open+"/labels", map[string]any{"label": "shared"})
	owner.ok("POST", "/api/v1/pages/"+secret+"/labels", map[string]any{"label": "shared"})
	owner.ok("POST", "/api/v1/pages/"+secret+"/labels", map[string]any{"label": "hiddenword"})
	owner.restrict(secret, "view", owner.id)
	return hiddenPageFixture{owner: owner, reader: reader, space: space, home: home, open: open, secret: secret}
}

// The new blocks are lists like any other, and each has its own query. So each is
// asked the same question: does a word of the hidden page come out?
func TestNoNewBlockShowsAHiddenPage(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	for kind, settings := range map[string]map[string]any{
		"activity_tabs":      {"scope": "all"},
		"popular_labels":     {"this_space": true},
		"related_labels":     {"page": f.open},
		"recent_labels":      {"this_space": true},
		"rotating_cards":     {"this_space": true},
		"link_cards":         {"this_space": true},
		"contribution_table": {"page": f.home, "by": "page"},
		"decision_report":    {"page": f.home},
		"task_report":        {"this_space": true},
		"space_files":        {},
		"popular_pages":      {"this_space": true},
	} {
		got := f.reader.want(200, "POST", "/api/v1/blocks/"+kind, map[string]any{"page_id": f.home, "settings": settings})
		for _, word := range []string{"Secret page", "hiddenword", "Words in the secret"} {
			if strings.Contains(got, word) {
				t.Errorf("%s shows %q: %s", kind, word, got)
			}
		}
	}
	if !hasTitle(f.reader.block("activity_tabs", f.home, map[string]any{"scope": "all"}), "Open page") {
		t.Fatal("activity lost the open page")
	}
}

func TestLabelBlocksCountOnlyWhatAReaderMaySee(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	f.owner.ok("POST", "/api/v1/pages/"+f.open+"/labels", map[string]any{"label": "besideshared"})
	popular := f.reader.block("popular_labels", f.home, map[string]any{"this_space": true})
	for _, row := range popular["rows"].([]any) {
		label := row.(map[string]any)
		if label["title"] == "shared" && label["count"] != 1.0 {
			t.Fatalf("a hidden page counted: %v", label)
		}
	}
	if !hasTitle(popular, "shared") {
		t.Fatalf("the label went missing: %v", popular)
	}
	if owner := f.owner.block("popular_labels", f.home, map[string]any{"this_space": true}); !hasTitle(owner, "hiddenword") {
		t.Fatalf("the owner should see every label: %v", owner)
	}
	if related := f.owner.block("related_labels", f.home, map[string]any{"page": f.open}); !hasTitle(related, "hiddenword") {
		t.Fatalf("a label beside a shared label is related: %v", related)
	}
	if related := f.owner.block("related_labels", f.home, map[string]any{"page": f.open}); hasTitle(related, "shared") {
		t.Fatalf("the label of the page itself is not related to it: %v", related)
	}
	if got := f.reader.block("related_labels", f.home, map[string]any{"page": f.secret}); len(rowTitles(got)) != 0 {
		t.Fatalf("a setting pointing at a hidden page answered: %v", got)
	}
}

func TestFilterListFindsByTextPeopleAndDate(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	cases := []struct {
		name     string
		settings map[string]any
		want     bool
	}{
		{"text in the page", map[string]any{"text": "open"}, true},
		{"text elsewhere", map[string]any{"text": "nowhereword"}, false},
		{"creator", map[string]any{"creator": "owner@example.com"}, true},
		{"another creator", map[string]any{"creator": "reader@example.com"}, false},
		{"contributor", map[string]any{"contributor": "owner@example.com"}, true},
		{"another contributor", map[string]any{"contributor": "reader@example.com"}, false},
		{"created before", map[string]any{"created_after": "2000-01-01"}, true},
		{"created later", map[string]any{"created_after": time.Now().AddDate(1, 0, 0).Format(time.DateOnly)}, false},
		{"changed later", map[string]any{"changed_after": time.Now().AddDate(1, 0, 0).Format(time.DateOnly)}, false},
		{"a post only", map[string]any{"content_type": "post"}, false},
		{"this space by key", map[string]any{"spaces": []any{"ENG"}}, true},
		{"another space by key", map[string]any{"spaces": []any{"OTHER"}}, false},
	}
	for _, c := range cases {
		got := hasTitle(f.reader.block("filter_list", f.home, c.settings), "Open page")
		if got != c.want {
			t.Errorf("%s: found %v, want %v", c.name, got, c.want)
		}
	}
	if got := f.reader.block("filter_list", f.home, map[string]any{"text": "secret"}); len(rowTitles(got)) != 0 {
		t.Fatalf("searching a hidden page's words found it: %v", got)
	}
}

func TestReportTableShowsTheChosenColumns(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	answer := f.reader.block("report_table", f.home, map[string]any{"this_space": true, "labels": []any{"shared"},
		"columns": []any{"labels", "comments", "nonsense"}})
	keys, _ := answer["column_keys"].([]any)
	if len(keys) != 2 || keys[0] != "labels" || keys[1] != "comments" {
		t.Fatalf("columns: %v", keys)
	}
	row := answer["rows"].([]any)[0].(map[string]any)
	cells := row["cells"].([]any)
	if cells[0] != "shared" || cells[1] != "0" {
		t.Fatalf("cells: %v", cells)
	}
	plain := f.reader.block("report_table", f.home, map[string]any{"this_space": true})
	if keys, _ := plain["column_keys"].([]any); len(keys) != 3 {
		t.Fatalf("the default columns: %v", keys)
	}
}

func TestWatchersShowOnlyToThoseWhoMaySeeThem(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	watcher := e.user("watcher@example.com", identity.PermUseProduct)
	f.owner.grant(f.space, "user", watcher.id, permissions.TemplateViewer)
	watcher.ok("PUT", "/api/v1/pages/"+f.open+"/watch", nil)
	settings := map[string]any{"page": f.open, "kinds": []any{"watchers"}}
	if got := f.reader.want(200, "POST", "/api/v1/blocks/contributors", map[string]any{"page_id": f.home, "settings": settings}); strings.Contains(got, "watcher") {
		t.Fatalf("a reader saw who watches: %s", got)
	}
	if got := f.owner.want(200, "POST", "/api/v1/blocks/contributors", map[string]any{"page_id": f.home, "settings": settings}); !strings.Contains(got, "watcher") {
		t.Fatalf("a space administrator should see who watches: %s", got)
	}
}

func TestCommentsForEditorsDoNotCountForReaders(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	note := f.owner.ok("POST", "/api/v1/pages/"+f.open+"/comments", map[string]any{"body": commentBody("Only for editors")})
	if _, err := e.app.Pool().Exec(context.Background(), "UPDATE comments SET editors_only = true WHERE id = $1", note["id"]); err != nil {
		t.Fatal(err)
	}
	got := f.reader.block("contributors", f.home, map[string]any{"page": f.open, "kinds": []any{"comments"}})
	if len(rowTitles(got)) != 0 {
		t.Fatalf("a comment for editors counted: %v", got)
	}
	table := f.reader.block("contribution_table", f.home, map[string]any{"page": f.open, "by": "person"})
	for _, row := range table["rows"].([]any) {
		if cells := row.(map[string]any)["cells"].([]any); cells[1] != "0" {
			t.Fatalf("a comment for editors counted in the table: %v", cells)
		}
	}
}

func TestAPersonCardShowsWhatTheDirectoryShows(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	stranger := e.user("stranger@example.com", identity.PermUseProduct)
	stranger.ok("PUT", "/api/v1/me/profile", map[string]any{"job_title": "Unseen"})
	if got := f.reader.want(200, "POST", "/api/v1/blocks/person_card", map[string]any{"page_id": f.home, "settings": map[string]any{"person": "stranger@example.com"}}); strings.Contains(got, "Unseen") || strings.Contains(got, "stranger") {
		t.Fatalf("someone the reader may not see has a card: %s", got)
	}
	person := e.user("person@example.com", identity.PermUseProduct)
	f.owner.grant(f.space, "user", person.id, permissions.TemplateViewer)
	person.ok("PUT", "/api/v1/me/profile", map[string]any{"job_title": "Keeper", "location": "Harbour",
		"visibility": map[string]string{"location": "me", "email": "me"}})
	got := f.reader.want(200, "POST", "/api/v1/blocks/person_card", map[string]any{"page_id": f.home, "settings": map[string]any{"person": "person@example.com"}})
	if !strings.Contains(got, "Keeper") {
		t.Fatalf("an open field went missing: %s", got)
	}
	if strings.Contains(got, "Harbour") || strings.Contains(got, `"email"`) {
		t.Fatalf("a field for the person alone came out: %s", got)
	}
	if got := f.reader.want(200, "POST", "/api/v1/blocks/person_card", map[string]any{"page_id": f.home, "settings": map[string]any{"person": "nobody@example.com"}}); strings.Contains(got, "nobody") {
		t.Fatalf("nobody has a card: %s", got)
	}
}

func TestThePeopleListKeepsToTheDirectory(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	var answer struct {
		People []struct {
			Name string `json:"name"`
		} `json:"people"`
	}
	raw := f.reader.want(200, "POST", "/api/v1/blocks/people_list", map[string]any{"page_id": f.home,
		"settings": map[string]any{"people": []any{"owner@example.com", "nobody@example.com"}}})
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.People) != 1 {
		t.Fatalf("people: %s", raw)
	}
}

func TestSpaceFilesWalkPastARestriction(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	f.owner.upload(f.open, "open-file.txt", "words")
	f.owner.upload(f.secret, "secret-file.txt", "words")
	answer := f.reader.block("space_files", f.home, nil)
	if !hasTitle(answer, "open-file.txt") {
		t.Fatalf("the open file went missing: %v", answer)
	}
	if hasTitle(answer, "secret-file.txt") {
		t.Fatalf("a file of a hidden page came out: %v", answer)
	}
	if got := f.reader.block("space_files", f.home, map[string]any{"file_type": "pdf"}); hasTitle(got, "open-file.txt") {
		t.Fatalf("the type filter let a text file through: %v", got)
	}
}

func TestDecisionsAndTasksFilterByPageAndPerson(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	decision := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "decision",
		"content": []any{map[string]any{"type": "text", "text": "We chose the harbour"}}}}}
	f.owner.publish(f.open, "Open page", 2, decision)
	if got := f.reader.block("decision_report", f.home, map[string]any{"page": f.open}); !hasTitle(got, "We chose the harbour") {
		t.Fatalf("by page: %v", got)
	}
	if got := f.reader.block("decision_report", f.home, map[string]any{"page": f.secret}); len(rowTitles(got)) != 0 {
		t.Fatalf("another page: %v", got)
	}
	if got := f.reader.block("decision_report", f.home, map[string]any{"person": "owner@example.com"}); !hasTitle(got, "We chose the harbour") {
		t.Fatalf("the creator of the page: %v", got)
	}
	if got := f.reader.block("decision_report", f.home, map[string]any{"person": "reader@example.com"}); len(rowTitles(got)) != 0 {
		t.Fatalf("a person with nothing to do with it: %v", got)
	}
}

func TestPopularPagesStayQuietWhileAnalyticsIsOff(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	if _, err := e.app.Pool().Exec(context.Background(), `INSERT INTO page_view_days (page_id, day, views) VALUES ($1, CURRENT_DATE, 9), ($2, CURRENT_DATE, 99)`, f.open, f.secret); err != nil {
		t.Fatal(err)
	}
	if got := f.reader.block("popular_pages", f.home, nil); len(rowTitles(got)) != 0 {
		t.Fatalf("analytics is off: %v", got)
	}
	f.setAnalytics(t, true)
	got := f.reader.block("popular_pages", f.home, nil)
	if !hasTitle(got, "Open page") || hasTitle(got, "Secret page") {
		t.Fatalf("popular pages: %v", got)
	}
}

func (f hiddenPageFixture) setAnalytics(t *testing.T, on bool) {
	t.Helper()
	if _, err := f.owner.e.app.Pool().Exec(context.Background(),
		`INSERT INTO settings (key, value) VALUES ('analytics', jsonb_build_object('enabled', $1::boolean, 'mode', 'counts', 'retention_days', 30))
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, on); err != nil {
		t.Fatal(err)
	}
}

func TestSnippetValuesFillTextAndNothingElse(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	source := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "excerpt",
		"attrs": map[string]any{"name": "greeting"}, "content": []any{para2("Version {{version}} of {{name}}, and {{left}}")}}}}
	f.owner.publish(f.open, "Open page", 2, source)
	got := f.reader.want(200, "POST", "/api/v1/blocks/show_excerpt", map[string]any{"page_id": f.home,
		"settings": map[string]any{"page": f.open, "name": "greeting", "values": []any{"version=2.1", "name=<b>Tool</b>", "broken"}}})
	if !strings.Contains(got, "Version 2.1 of") {
		t.Fatalf("a value was not filled in: %s", got)
	}
	if !strings.Contains(got, "{{left}}") {
		t.Fatalf("a blank without a value should stay: %s", got)
	}
	var answer struct {
		Document struct {
			Content []struct {
				Type    string `json:"type"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"content"`
		} `json:"document"`
	}
	if err := json.Unmarshal([]byte(got), &answer); err != nil {
		t.Fatal(err)
	}
	if text := answer.Document.Content[0].Content[0]; text.Type != "text" || !strings.Contains(text.Text, "<b>Tool</b>") {
		t.Fatalf("a value must stay plain text: %+v", text)
	}
}

func TestASyncedBlockSaysWhetherTheReaderMayEditItsSource(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	source := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "excerpt",
		"attrs": map[string]any{"name": "shared", "synced": true}, "content": []any{para2("Kept in one place")}}}}
	f.owner.publish(f.open, "Open page", 2, source)
	settings := map[string]any{"page": f.open, "name": "shared"}
	for who, want := range map[*user]bool{f.reader: false, f.owner: true} {
		answer := who.block("synced_block", f.home, settings)
		got, _ := answer["source"].(map[string]any)["can_edit"].(bool)
		if got != want {
			t.Errorf("%s may edit: %v, want %v", who.email, got, want)
		}
	}
	hidden := map[string]any{"page": f.secret, "name": "shared"}
	if got := f.reader.want(200, "POST", "/api/v1/blocks/synced_block", map[string]any{"page_id": f.home, "settings": hidden}); strings.Contains(got, "Secret") {
		t.Fatalf("a synced block on a hidden page answered: %s", got)
	}
}

func TestATemplateCarriesPageFactsIntoTheReport(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	made := f.owner.ok("POST", "/api/v1/spaces/"+f.space+"/templates", map[string]any{"name": "Fact sheet",
		"document": facts([2]string{"Owner", "Harbour team"}), "labels": []any{"sheet"}})
	used := f.owner.ok("POST", "/api/v1/templates/"+made["id"].(string)+"/use", map[string]any{"space_id": f.space, "parent_id": f.home, "title": "From the template"})
	id := used["page"].(map[string]any)["id"].(string)
	f.owner.publish(id, "From the template", 0, facts([2]string{"Owner", "Harbour team"}))
	report := f.owner.block("facts_report", f.home, map[string]any{"labels": []any{"sheet"}})
	if !hasTitle(report, "From the template") {
		t.Fatalf("the report missed the page: %v", report)
	}
}

func TestTasksFilterByCreatorAndDate(t *testing.T) {
	e := newEnv(t)
	f := newHiddenPageFixture(t, e)
	task := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "taskList", "content": []any{
		map[string]any{"type": "taskItem", "attrs": map[string]any{"checked": false}, "content": []any{para2("Paint the boat")}}}}}}
	f.owner.publish(f.open, "Open page", 2, task)
	if got := f.reader.block("task_report", f.home, map[string]any{"created_by": "owner@example.com"}); !hasTitle(got, "Paint the boat") {
		t.Fatalf("by creator: %v", got)
	}
	if got := f.reader.block("task_report", f.home, map[string]any{"created_by": "reader@example.com"}); len(rowTitles(got)) != 0 {
		t.Fatalf("by someone else: %v", got)
	}
	later := time.Now().AddDate(1, 0, 0).Format(time.DateOnly)
	if got := f.reader.block("task_report", f.home, map[string]any{"created_after": later}); len(rowTitles(got)) != 0 {
		t.Fatalf("after a later date: %v", got)
	}
}
