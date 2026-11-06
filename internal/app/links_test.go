// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const missingPage = "00000000-0000-4000-8000-000000000001"

// linkSetup is a page anyone in the space can read, and a page only its owner can.
type linkSetup struct {
	owner, reader *user
	space, home   string
	open, secret  string
}

func newLinkSetup(e *env) linkSetup {
	e.t.Helper()
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	reader := e.user("reader@example.com", identity.PermUseProduct)
	space, home := owner.space("ENG")
	owner.grant(space, "user", reader.id, permissions.TemplateViewer)
	open := owner.page(space, home, "Open plan", "Words of the open page")
	secret := owner.page(space, home, "Merger with Initech", "Words of the secret page")
	owner.restrict(secret, "view", owner.id)
	return linkSetup{owner: owner, reader: reader, space: space, home: home, open: open, secret: secret}
}

func inlineDoc(nodes ...any) map[string]any {
	return map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph", "content": nodes}}}
}

func pageLink(id string) map[string]any {
	return map[string]any{"type": "pageLink", "attrs": map[string]any{"page_id": id}}
}

func placeholder(title string) map[string]any {
	return map[string]any{"type": "placeholderLink", "attrs": map[string]any{"title": title}}
}

func factsOf(u *user, ids ...string) string {
	u.e.t.Helper()
	return u.want(200, "POST", "/api/v1/pages/link-facts", map[string]any{"ids": ids})
}

func TestLinkFactsLeaveOutWhatTheReaderMayNotSee(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	got := factsOf(s.reader, s.open, s.secret, missingPage)
	if !strings.Contains(got, "Open plan") {
		t.Fatalf("the open page is missing: %s", got)
	}
	if strings.Contains(got, "Merger") || strings.Contains(got, s.secret) || strings.Contains(got, missingPage) {
		t.Fatalf("a hidden page shows: %s", got)
	}
	if owner := factsOf(s.owner, s.secret); !strings.Contains(owner, "Merger with Initech") {
		t.Fatalf("the owner should see the title: %s", owner)
	}
}

// A hidden page and a page that does not exist must give the same answer.
func TestAHiddenPageAndAMissingPageLookTheSameInLinkFacts(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	if hidden, gone := factsOf(s.reader, s.secret), factsOf(s.reader, missingPage); hidden != gone {
		t.Fatalf("they can be told apart: %s and %s", hidden, gone)
	}
}

func TestLinkFactsNeedALogin(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	resp, _ := e.do(e.client(), "POST", "/api/v1/pages/link-facts", `{"ids":["`+s.open+`"]}`)
	if resp.StatusCode != 401 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestLinkFactsHaveALimit(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	ids := make([]string, 501)
	for i := range ids {
		ids[i] = missingPage
	}
	s.reader.want(400, "POST", "/api/v1/pages/link-facts", map[string]any{"ids": ids})
}

func TestLinkFactsTellEverythingACardShows(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	e.settle()
	var answer struct {
		Pages []struct {
			ID, Title, Excerpt, SpaceName, UpdatedAt string
		} `json:"pages"`
	}
	raw := strings.NewReplacer(`"space_name"`, `"SpaceName"`, `"updated_at"`, `"UpdatedAt"`).Replace(factsOf(s.reader, s.open))
	if err := json.Unmarshal([]byte(raw), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Pages) != 1 {
		t.Fatalf("got %+v", answer)
	}
	p := answer.Pages[0]
	if p.ID != s.open || p.Title != "Open plan" || p.SpaceName != "Space ENG" || p.UpdatedAt == "" {
		t.Fatalf("got %+v", p)
	}
	if !strings.Contains(p.Excerpt, "open page") {
		t.Fatalf("no excerpt in %+v", p)
	}
}

func TestALinkFollowsTheTitleOfItsPage(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.home, "Home", 1, inlineDoc(pageLink(s.open)))
	s.owner.publish(s.open, "A better name", 1, para("Words"))
	if got := factsOf(s.reader, s.open); !strings.Contains(got, "A better name") {
		t.Fatalf("got %s", got)
	}
	if got := s.owner.want(200, "GET", "/api/v1/pages/"+s.home, nil); strings.Contains(got, "A better name") {
		t.Fatalf("the title was stored in the document: %s", got)
	}
}

func TestANewLinkCountsAsALinkBetweenPages(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	card := map[string]any{"type": "pageCard", "attrs": map[string]any{"page_id": s.secret}}
	s.owner.publish(s.home, "Home", 1, map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "paragraph", "content": []any{pageLink(s.open)}}, card}})
	if got := e.count("SELECT count(*) FROM page_links WHERE source_id = $1", s.home); got != 2 {
		t.Fatalf("got %d links", got)
	}
}

func TestSearchDoesNotFindAPageThroughTheTitleOfALinkedPage(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(pageLink(s.secret)))
	e.settle()
	if got := s.reader.want(200, "GET", "/api/v1/search?q=Initech", nil); strings.Contains(got, s.open) {
		t.Fatalf("the linked title is in the index: %s", got)
	}
}

func TestTargetsListHeadingsAndAnchors(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, map[string]any{"type": "doc", "content": []any{
		map[string]any{"type": "heading", "attrs": map[string]any{"level": 2},
			"content": []any{map[string]any{"type": "text", "text": "Next steps"}}},
		map[string]any{"type": "paragraph", "content": []any{
			map[string]any{"type": "anchor", "attrs": map[string]any{"name": "the-list"}}}},
	}})
	got := s.reader.ok("GET", "/api/v1/pages/"+s.open+"/targets", nil)
	raw := mustJSON(t, got)
	if !strings.Contains(raw, `"next-steps"`) || !strings.Contains(raw, "Next steps") || !strings.Contains(raw, `"the-list"`) {
		t.Fatalf("got %s", raw)
	}
}

func TestTargetsOfAHiddenPageAreNotFound(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.reader.want(404, "GET", "/api/v1/pages/"+s.secret+"/targets", nil)
}

func TestFollowingAPlaceholderStartsAPageBelowIt(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes")))
	page := s.owner.ok("POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Meeting notes"})
	started := page["page_id"].(string)
	got := s.owner.ok("GET", "/api/v1/pages/"+started, nil)
	if got["title"] != "Meeting notes" || got["parent_id"] != s.open || got["status"] != "draft" {
		t.Fatalf("got %v", got)
	}
	if again := s.owner.ok("POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Meeting notes"}); again["page_id"] != started {
		t.Fatalf("a second page: %v", again)
	}
	if got := e.count("SELECT count(*) FROM pages WHERE title = 'Meeting notes'"); got != 1 {
		t.Fatalf("%d pages", got)
	}
}

func TestOnlyAPlaceholderOnThePageCanBeFollowed(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes")))
	s.owner.want(404, "POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Something else"})
}

func TestFollowingAPlaceholderNeedsTheRightToCreate(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes")))
	s.reader.want(403, "POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Meeting notes"})
	if got := e.count("SELECT count(*) FROM pages WHERE title = 'Meeting notes'"); got != 0 {
		t.Fatalf("%d pages", got)
	}
}

func TestAPlaceholderOnAHiddenPageIsNotFound(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.secret, "Merger with Initech", 1, inlineDoc(placeholder("Meeting notes")))
	s.reader.want(404, "POST", "/api/v1/pages/"+s.secret+"/placeholders", map[string]any{"title": "Meeting notes"})
	s.reader.want(404, "GET", "/api/v1/pages/"+s.secret+"/placeholders", nil)
}

func TestAPlaceholderOnlyLeadsToAPageTheReaderMaySee(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes")))
	started := s.owner.ok("POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Meeting notes"})["page_id"].(string)
	path := "/api/v1/pages/" + s.open + "/placeholders"

	if got := s.reader.want(200, "GET", path, nil); strings.Contains(got, started) {
		t.Fatalf("a draft of someone else shows: %s", got)
	}
	if got := s.owner.want(200, "GET", path, nil); !strings.Contains(got, started) {
		t.Fatalf("the author should reach it: %s", got)
	}

	s.owner.publish(started, "Meeting notes", 0, para("Notes"))
	if got := s.reader.want(200, "GET", path, nil); !strings.Contains(got, started) {
		t.Fatalf("the published page should show: %s", got)
	}
}

func TestSomeoneWhoCannotSeeTheStartedPageGetsAConflict(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	writer := e.user("writer@example.com", identity.PermUseProduct)
	s.owner.grant(s.space, "user", writer.id, permissions.TemplateEditor)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes")))
	started := s.owner.ok("POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Meeting notes"})["page_id"].(string)
	got := writer.want(409, "POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Meeting notes"})
	if strings.Contains(got, started) {
		t.Fatalf("the answer names the page: %s", got)
	}
}

func TestRemovingAPlaceholderFromThePageForgetsIt(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes")))
	s.owner.publish(s.open, "Open plan", 2, para("No link any more"))
	if got := e.count("SELECT count(*) FROM page_placeholders"); got != 0 {
		t.Fatalf("%d rows", got)
	}
}

func TestPublishingAgainKeepsWhatAPlaceholderBecame(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes")))
	started := s.owner.ok("POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Meeting notes"})["page_id"].(string)
	s.owner.publish(s.open, "Open plan", 2, inlineDoc(placeholder("Meeting notes")))
	if got := s.owner.want(200, "GET", "/api/v1/pages/"+s.open+"/placeholders", nil); !strings.Contains(got, started) {
		t.Fatalf("got %s", got)
	}
}

func TestSpaceSettingsListOpenPlaceholders(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(placeholder("Meeting notes"), placeholder("Started already")))
	s.owner.ok("POST", "/api/v1/pages/"+s.open+"/placeholders", map[string]any{"title": "Started already"})
	got := s.owner.want(200, "GET", "/api/v1/spaces/"+s.space+"/placeholders", nil)
	if !strings.Contains(got, "Meeting notes") || !strings.Contains(got, "Open plan") {
		t.Fatalf("got %s", got)
	}
	if strings.Contains(got, "Started already") {
		t.Fatalf("a placeholder that became a page shows: %s", got)
	}
}

func TestOnlyASpaceAdministratorListsPlaceholders(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.reader.want(403, "GET", "/api/v1/spaces/"+s.space+"/placeholders", nil)
}

func TestThePlaceholderListSkipsAPageTheAdministratorMayNotSee(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	manager := e.user("manager@example.com", identity.PermUseProduct)
	s.owner.grant(s.space, "user", manager.id, permissions.TemplateSpaceAdmin)
	s.owner.publish(s.secret, "Merger with Initech", 1, inlineDoc(placeholder("Meeting notes")))
	got := manager.want(200, "GET", "/api/v1/spaces/"+s.space+"/placeholders", nil)
	if strings.Contains(got, "Merger") || strings.Contains(got, "Meeting notes") {
		t.Fatalf("a hidden page shows: %s", got)
	}
}

func exportOf(t *testing.T, u *user, page string) string {
	t.Helper()
	return u.want(200, "GET", "/api/v1/pages/"+page+"/export.md", nil)
}

func TestAnExportNamesOnlyWhatTheExporterMaySee(t *testing.T) {
	e := newEnv(t)
	s := newLinkSetup(e)
	s.owner.publish(s.open, "Open plan", 1, inlineDoc(pageLink(s.secret), placeholder("Meeting notes")))
	if got := exportOf(t, s.owner, s.open); !strings.Contains(got, "Merger with Initech") {
		t.Fatalf("the owner should get the title: %s", got)
	}
	got := exportOf(t, s.reader, s.open)
	if strings.Contains(got, "Merger") || strings.Contains(got, s.secret) {
		t.Fatalf("a hidden page shows: %s", got)
	}
	if !strings.Contains(got, "Linked page") || !strings.Contains(got, "Meeting notes") {
		t.Fatalf("got %s", got)
	}
}
