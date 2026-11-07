// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/permissions"
)

func searchTitles(t *testing.T, u *user, query string) []string {
	t.Helper()
	return titlesIn(u.ok("GET", "/api/v1/search?"+query, nil), "results")
}

// newerPages adds pages faster than the API could, all newer than anything so far, so an
// older page ends up behind them.
func (e *env) newerPages(like string, count int) {
	e.t.Helper()
	e.sql(`INSERT INTO pages (id, origin_instance_id, space_id, parent_id, position, title, status, current_version,
			created_by, updated_by, updated_at, kind)
		SELECT gen_random_uuid(), p.origin_instance_id, p.space_id, p.parent_id, 1000 + n, 'Filler ' || n, 'published', 1,
			p.created_by, p.updated_by, now() + interval '1 hour', 'page'
		FROM pages p, generate_series(1, $2) n WHERE p.id = $1`, like, count)
	e.sql(`INSERT INTO search_documents (page_id, title, body)
		SELECT p.id, p.title, 'filler words only' FROM pages p WHERE p.title LIKE 'Filler %'
		ON CONFLICT DO NOTHING`)
}

func TestARestrictionCountsAtOnceInSearch(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "Merger notes", "quartermaster")
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 1 {
		t.Fatalf("before the restriction: %v", got)
	}
	owner.restrict(page, "view", owner.id)
	out := viewer.ok("GET", "/api/v1/search?q=quartermaster", nil)
	if len(out["results"].([]any)) != 0 || out["total"].(float64) != 0 {
		t.Fatalf("a restricted page still shows or counts: %v", out)
	}
	if strings.Contains(viewer.want(200, "GET", "/api/v1/search/quick?q=quartermaster", nil), "Merger") {
		t.Fatal("quick search shows a restricted page")
	}
	owner.restrict(page, "view")
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 1 {
		t.Fatalf("after lifting it: %v", got)
	}
}

func TestLosingASpaceCountsAtOnceInSearch(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.page(space, nil, "Open notes", "quartermaster")
	owner.ok("PUT", "/api/v1/spaces/"+space+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": viewer.id}, "permissions": []string{}})
	out := viewer.ok("GET", "/api/v1/search?q=quartermaster", nil)
	if len(out["results"].([]any)) != 0 || out["total"].(float64) != 0 {
		t.Fatalf("a person without the space finds or counts: %v", out)
	}
}

// The rule on the rows shown is what counts: a row that says the wrong thing gives nothing away.
func TestAWrongRowOfTheSearchTableShowsNothing(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, open := e.team("ENG")
	closed, _ := owner.space("SECRET")
	page := owner.page(closed, nil, "Hidden plan", "quartermaster")
	e.sql("UPDATE search_pages SET space_id = $1 WHERE page_id = $2", open, page)
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 0 {
		t.Fatalf("the search table outranked the permissions: %v", got)
	}
}

func TestTheSearchTableFollowsThePage(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "Findable", "quartermaster")
	owner.ok("POST", "/api/v1/pages/"+page+"/archive", map[string]any{})
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 0 {
		t.Fatalf("an archived page stays out unless asked for: %v", got)
	}
	if got := searchTitles(t, viewer, "q=quartermaster&archived=1"); len(got) != 1 {
		t.Fatalf("asked for: %v", got)
	}
	owner.ok("POST", "/api/v1/pages/"+page+"/unarchive", map[string]any{})
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 1 {
		t.Fatalf("back from the archive: %v", got)
	}
	owner.ok("DELETE", "/api/v1/pages/"+page, nil)
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 0 {
		t.Fatalf("a page in the trash: %v", got)
	}
}

func TestATitleGoesBeforeTextAndNewPagesBeforeOld(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.page(space, nil, "Older note", "harbour works")
	owner.page(space, nil, "Harbour plan old", "x")
	owner.page(space, nil, "Harbour", "x")
	owner.page(space, nil, "Harbour plan new", "x")
	owner.page(space, nil, "Newer note", "harbour works")
	got := searchTitles(t, viewer, "q=harbour")
	want := []string{"Harbour", "Harbour plan new", "Harbour plan old", "Newer note", "Older note"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("order: %v", got)
	}
	got = searchTitles(t, viewer, "q=harbour&sort=date")
	if got[0] != "Newer note" {
		t.Fatalf("by date: %v", got)
	}
}

func TestPagesContinueAcrossTheTiers(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.page(space, nil, "Harbour title", "x")
	owner.page(space, nil, "Text one", "harbour")
	owner.page(space, nil, "Text two", "harbour")
	first := searchTitles(t, viewer, "q=harbour&limit=2")
	second := searchTitles(t, viewer, "q=harbour&limit=2&offset=2")
	if strings.Join(first, "|") != "Harbour title|Text two" || strings.Join(second, "|") != "Text one" {
		t.Fatalf("pages: %v then %v", first, second)
	}
}

// The word stands far past the newest pages. It is what someone looks for during an audit.
func TestAnOldPageIsFoundBehindAWholeWindowOfNewerPages(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	old := owner.page(space, nil, "Nine years old", "quartermaster")
	e.newerPages(old, 2100)
	out := viewer.ok("GET", "/api/v1/search?q=quartermaster", nil)
	if got := titlesIn(out, "results"); len(got) != 1 || got[0] != "Nine years old" {
		t.Fatalf("the old page is gone: %v", got)
	}
	if got := searchTitles(t, viewer, "q=nine"); len(got) != 1 {
		t.Fatalf("by its title: %v", got)
	}
	got := viewer.ok("GET", "/api/v1/search?q=filler", nil)
	if got["capped"] != true || got["total"].(float64) != 200 {
		t.Fatalf("a common word: %v", got)
	}
	if rows := searchTitles(t, viewer, "q=filler&limit=25&offset=2000"); len(rows) != 25 {
		t.Fatalf("the results stop at a total: %d rows far along", len(rows))
	}
}

func TestLabelsAndFileNamesAreFoundWithoutAField(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	page := owner.page(space, nil, "Plain page", "x")
	owner.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "quartermaster"})
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 1 {
		t.Fatalf("by label: %v", got)
	}
	owner.want(201, "POST", "/api/v1/pages/"+page+"/attachments?filename=harbourplan.txt", "words")
	if got := searchTitles(t, viewer, "q=harbourplan"); len(got) != 1 {
		t.Fatalf("by file: %v", got)
	}
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 1 {
		t.Fatalf("a file wiped the label: %v", got)
	}
	owner.ok("DELETE", "/api/v1/pages/"+page+"/labels/quartermaster", nil)
	if got := searchTitles(t, viewer, "q=quartermaster"); len(got) != 0 {
		t.Fatalf("a removed label: %v", got)
	}
	if got := searchTitles(t, viewer, "q=harbourplan"); len(got) != 1 {
		t.Fatalf("a label wiped the file: %v", got)
	}
}

func TestQuickSearchHoldsNoSnippet(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.page(space, nil, "Findable", "the quartermaster words")
	if got := viewer.want(200, "GET", "/api/v1/search/quick?q=findable", nil); strings.Contains(got, "quartermaster") {
		t.Fatalf("a snippet costs time the box does not use: %s", got)
	}
}

// Nothing found and nothing known are not the same: an audit must see the difference.
func TestASearchThatRunsOutOfTimeSaysSo(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, space := e.team("ENG")
	owner.page(space, nil, "Findable", "quartermaster")
	ctx := context.Background()
	tx, err := e.app.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "LOCK TABLE search_pages IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	got := viewer.ok("GET", "/api/v1/search?q=quartermaster", nil)
	if got["timed_out"] != true {
		t.Fatalf("after %s: %v", time.Since(started), got)
	}
}

func TestAViewerInASmallSpaceFindsACommonWord(t *testing.T) {
	e := newEnv(t)
	owner, _, viewer, big := e.team("BIG")
	small, _ := owner.space("SMALL")
	owner.grant(small, "user", viewer.id, permissions.TemplateViewer)
	first := owner.page(big, nil, "Busy", "filler words only")
	e.newerPages(first, 2100)
	owner.page(small, nil, "Mine", "filler")
	if got := searchTitles(t, viewer, "q=filler&space_id="+small); len(got) != 1 {
		t.Fatalf("within the space: %v", got)
	}
}
