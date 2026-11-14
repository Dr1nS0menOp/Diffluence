// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/publishing"
)

// Draft and published spaces, wave PB2 of spec 073: releasing a branch and the whole space,
// leaving out, taking off, mirroring removals, batches, the release screen, notifications and
// the history. The failure and leak paths come first.

// runBody is the scope of a run, as the release screen sends it.
type runBody = map[string]any

// previewRun asks for the preview of a run of more than one page.
func (w pubWorld) previewRun(u *user, scope runBody) map[string]any {
	w.e.t.Helper()
	return u.ok("POST", "/api/v1/spaces/"+w.draft+"/releases/preview", scope)
}

// runAll previews a run, confirms it with a note, and lets the worker carry it out.
func (w pubWorld) runAll(u *user, scope runBody, note string) map[string]any {
	w.e.t.Helper()
	preview := w.previewRun(u, scope)
	id := preview["id"].(string)
	u.want(http.StatusAccepted, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+id+"/start", map[string]any{"note": note})
	w.e.runJobs()
	return u.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/"+id, nil)
}

func (w pubWorld) wholeSpace(note string) map[string]any {
	w.e.t.Helper()
	return w.runAll(w.admin, runBody{"scope": "space"}, note)
}

// twinOrEmpty is the twin of a draft page, or "" when it has none.
func (w pubWorld) twinOrEmpty(page string) string {
	w.e.t.Helper()
	twin, _ := w.status(w.admin, page)["twin_page_id"].(string)
	return twin
}

func (w pubWorld) publishedCount() int {
	w.e.t.Helper()
	return w.e.count("SELECT count(*) FROM pages WHERE space_id = $1 AND status = 'published'", w.published)
}

func outcomes(run map[string]any) map[string]string {
	out := map[string]string{}
	for _, p := range run["pages"].([]any) {
		page := p.(map[string]any)
		reason, _ := page["reason"].(string)
		out[page["page_id"].(string)] = fmt.Sprint(page["outcome"], ":", reason)
	}
	return out
}

func (w pubWorld) leaveOut(page string, withChildren bool, reason string) map[string]any {
	w.e.t.Helper()
	return w.admin.ok("PUT", "/api/v1/pages/"+page+"/leave-out", map[string]any{"with_children": withChildren, "reason": reason})
}

// TestAReaderOfThePublishedSpaceNeverReachesTheDraftAfterABulkRun is the leak test of PB1
// (PUB-108, PUB-620 to PUB-635) for a whole-space and a branch release: every door of the
// published space, the feed and the bell included, rendered for a reader of it, holds no id,
// key or name of the draft space, no draft page that was not released, no page left out or
// restricted in the draft, and no word that exists only in the draft.
func TestAReaderOfThePublishedSpaceNeverReachesTheDraftAfterABulkRun(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "BLEAK")
	w.reader.ok("PUT", "/api/v1/spaces/"+w.published+"/watch", map[string]any{})
	guide := w.page(w.writer, w.draftHome, "Safety guide", "wear a helmet")
	chapter := w.page(w.writer, guide, "Chapter one", "climb carefully")
	secret := w.page(w.writer, w.draftHome, "Restricted wombatfjord memo", "wombatfjord salary table")
	w.admin.restrict(secret, "view", w.admin.id, w.writer.id)
	notes := w.page(w.writer, w.draftHome, "Working notes quokkaridge", "quokkaridge thoughts")
	below := w.page(w.writer, notes, "Below the notes", "quokkaridge details")
	w.leaveOut(notes, true, "internal only")
	w.writer.comment(guide, "draft comment zanzibarion")
	w.edit(w.writer, chapter, "Chapter one", docOf(paraOf(textOf("see "), pageLinkTo(notes, ""), textOf(" and "),
		pageLinkTo(secret, ""), textOf(" and "), pageLinkTo(guide, "top"))))
	run := w.wholeSpace("First edition")
	if run["status"] != "done" {
		t.Fatalf("the run did not finish: %v", run)
	}
	w.edit(w.writer, chapter, "Chapter one", para("climb carefully, second edition"))
	w.runAll(w.admin, runBody{"scope": "branch", "root_page_id": guide}, "Second edition")
	w.edit(w.writer, chapter, "Chapter one", para("climb carefully, and the draft-only word wombatfjord"))
	twins := []string{w.twin(guide), w.twin(chapter)}
	for _, page := range []string{secret, notes, below} {
		if twin := w.twinOrEmpty(page); twin != "" {
			t.Fatalf("a page left out or restricted reached readers: %s", page)
		}
	}
	e.deliver()
	e.runJobs()
	doors := []string{
		"/api/v1/spaces/" + w.published,
		"/api/v1/spaces/" + w.published + "/tree",
		"/api/v1/spaces/" + w.published + "/publishing",
		"/api/v1/search?q=helmet",
		"/api/v1/search?q=climb",
		"/api/v1/search/quick?q=Chapter",
		"/api/v1/feed",
		"/api/v1/feed?tab=recent",
		"/api/v1/me/notifications",
		"/api/v1/spaces",
	}
	for _, twin := range twins {
		doors = append(doors, "/api/v1/pages/"+twin, "/api/v1/pages/"+twin+"/versions", "/api/v1/pages/"+twin+"/release",
			"/api/v1/pages/"+twin+"/export.md", "/api/v1/pages/"+twin+"/details", "/api/v1/pages/"+twin+"/comments")
	}
	forbidden := []string{w.draft, w.draftHome, guide, chapter, secret, notes, below, strings.ToLower(w.draftKey), w.draftName,
		"wombatfjord", "quokkaridge", "zanzibarion", "zanzibar", "internal only"}
	for _, path := range doors {
		status, out := w.reader.do("GET", path, nil)
		if status != http.StatusOK {
			t.Errorf("%s: %d %s", path, status, out)
			continue
		}
		lower := strings.ToLower(out)
		for _, word := range forbidden {
			if strings.Contains(lower, strings.ToLower(word)) {
				t.Errorf("%s shows %q to a reader of the published space: %s", path, word, out)
			}
		}
	}
	// The release screen and every route of the draft side do not exist for a reader.
	for _, path := range []string{"/api/v1/spaces/" + w.draft + "/releases", "/api/v1/spaces/" + w.draft + "/releases/history",
		"/api/v1/spaces/" + w.draft + "/releases/" + run["id"].(string)} {
		if status, out := w.reader.do("GET", path, nil); status != http.StatusNotFound {
			t.Errorf("%s answers a reader: %d %s", path, status, out)
		}
	}
	// A run in the feed is one item, with the note and the pages the reader may see.
	feed := w.reader.want(http.StatusOK, "GET", "/api/v1/feed?tab=recent", nil)
	if !strings.Contains(feed, "Second edition") {
		t.Errorf("the run is not in the feed: %s", feed)
	}
}

// TestEveryReleaseRouteAsksForItsRight is PUB-413, PUB-831 and PUB-1303: each route of the
// release screen refuses whoever lacks space.release in the draft space, or view of the
// published space, and answers experimental_off while the part is off.
func TestEveryReleaseRouteAsksForItsRight(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "RIGHTS")
	page := w.page(w.writer, w.draftHome, "Policy", "words")
	base := "/api/v1/spaces/" + w.draft + "/releases"
	routes := []struct{ method, path string }{
		{"GET", base}, {"POST", base + "/preview"}, {"GET", base + "/history"}, {"GET", base + "/as-of"},
		{"PUT", "/api/v1/pages/" + page + "/leave-out"}, {"DELETE", "/api/v1/pages/" + page + "/leave-out"},
		{"GET", "/api/v1/spaces/" + w.draft + "/publishing/linking-in"},
	}
	for _, r := range routes {
		// A writer of the draft space may not release; a reader of both sees no draft side.
		for _, u := range []*user{w.writer, w.both} {
			var in any = runBody{"scope": "space"}
			if r.method != "POST" {
				in = map[string]any{}
			}
			if status, out := u.do(r.method, r.path, in); status != http.StatusForbidden && status != http.StatusNotFound {
				t.Errorf("%s %s answers someone without space.release: %d %s", r.method, r.path, status, out)
			}
		}
	}
	// The release manager may, until they lose view of the published space.
	w.rel.ok("GET", base, nil)
	w.admin.ok("PUT", "/api/v1/spaces/"+w.published+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": w.rel.id}, "permissions": []string{}})
	if out := w.rel.want(http.StatusConflict, "POST", base+"/preview", runBody{"scope": "space"}); !strings.Contains(out, "published_space_hidden") {
		t.Errorf("a release manager without the published space: %s", out)
	}
	// The export needs space.audit.
	w.rel.want(http.StatusForbidden, "GET", base+"/export", nil)
	w.admin.want(http.StatusOK, "GET", base+"/export", nil)
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"automation", "bundles", "console_restore", "databases"}})
	for _, r := range append(routes, struct{ method, path string }{"POST", base + "/check"}) {
		if out := w.admin.want(http.StatusNotFound, r.method, r.path, runBody{}); !strings.Contains(out, "experimental_off") {
			t.Errorf("%s %s while the part is off: %s", r.method, r.path, out)
		}
	}
}

// TestAWholeSpaceReleaseSendsWhatChangedAndNothingTwice is PUB-401 to PUB-403, PUB-410 and
// PUB-430: a first whole-space release makes every twin in one run, a page left out is
// skipped with the pages below it and listed, and a second run with nothing changed is
// refused as having nothing to do and makes no version.
func TestAWholeSpaceReleaseSendsWhatChangedAndNothingTwice(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "WHOLE")
	a := w.page(w.writer, w.draftHome, "Alpha", "a")
	b := w.page(w.writer, a, "Beta", "b")
	left := w.page(w.writer, w.draftHome, "Left", "l")
	under := w.page(w.writer, left, "Under left", "u")
	w.leaveOut(left, false, "not ready")
	preview := w.previewRun(w.admin, runBody{"scope": "space"})
	counts := preview["counts"].(map[string]any)
	if counts["make"] != 3.0 || counts["left_out"] != 2.0 || preview["batches"] != 0.0 {
		t.Fatalf("preview of a first release: %v", preview)
	}
	skipped := fmt.Sprint(preview["skipped"])
	if !strings.Contains(skipped, "Under left") || !strings.Contains(skipped, "below_left_out") {
		t.Fatalf("the page below one left out is not listed: %s", skipped)
	}
	run := w.runAll(w.admin, runBody{"scope": "space"}, "Edition one")
	got := outcomes(run)
	if got[a] != "released:" || got[b] != "released:" || got[w.draftHome] != "released:" || got[left] != "skipped:left_out" || got[under] != "skipped:below_left_out" {
		t.Fatalf("outcomes: %v", got)
	}
	if n := w.publishedCount(); n != 3 {
		t.Fatalf("%d pages in the published space, want the home page, Alpha and Beta", n)
	}
	if parent := e.scalar("SELECT parent_id FROM pages WHERE id = $1", w.twin(b)); parent != w.twin(a) {
		t.Fatalf("Beta's twin is under %s, not under Alpha's twin", parent)
	}
	versions := e.count("SELECT count(*) FROM page_versions v JOIN pages p ON p.id = v.page_id WHERE p.space_id = $1", w.published)
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.draft+"/releases/preview", runBody{"scope": "space"}); !strings.Contains(out, "nothing_to_release") {
		t.Fatalf("a second run with nothing changed: %s", out)
	}
	w.edit(w.writer, b, "Beta", para("b, second edition"))
	second := w.runAll(w.admin, runBody{"scope": "space"}, "Edition two")
	if got := outcomes(second); len(got) != 3 || got[b] != "released:" {
		t.Fatalf("a later run sends only what changed: %v", got)
	}
	if after := e.count("SELECT count(*) FROM page_versions v JOIN pages p ON p.id = v.page_id WHERE p.space_id = $1", w.published); after != versions+1 {
		t.Fatalf("a later run made %d versions, want one", after-versions)
	}
	if note := e.scalar("SELECT summary FROM page_versions WHERE page_id = $1 ORDER BY number DESC LIMIT 1", w.twin(b)); note != "Edition two" {
		t.Fatalf("the version's summary is %q, not the note", note)
	}
}

// TestRestrictedDraftPagesStayOutOfBulkRunsUnlessNamed is PUB-422 and PUB-413: a page with a
// view restriction in the draft is left out of a branch and a whole-space run unless named,
// and one the releaser cannot see is counted without its title.
func TestRestrictedDraftPagesStayOutOfBulkRunsUnlessNamed(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "RESTR")
	open := w.page(w.writer, w.draftHome, "Open", "o")
	hr := w.page(w.writer, w.draftHome, "HR handbook", "h")
	w.admin.restrict(hr, "view", w.admin.id, w.writer.id, w.rel.id)
	hidden := w.page(w.writer, w.draftHome, "Board minutes", "m")
	w.admin.restrict(hidden, "view", w.admin.id, w.writer.id)
	preview := w.previewRun(w.rel, runBody{"scope": "space"})
	counts := preview["counts"].(map[string]any)
	if counts["restricted"] != 1.0 || counts["hidden"] != 1.0 || strings.Contains(fmt.Sprint(preview), "Board minutes") {
		t.Fatalf("restricted pages in a preview: %v", preview)
	}
	run := w.runAll(w.rel, runBody{"scope": "space"}, "")
	if got := outcomes(run); got[open] != "released:" || got[hr] != "skipped:restricted" {
		t.Fatalf("a restricted page went in a bulk run: %v", got)
	}
	if strings.Contains(fmt.Sprint(run), hidden) {
		t.Fatalf("the report names a page hidden from the releaser")
	}
	named := w.runAll(w.rel, runBody{"scope": "space", "named": []string{hr}}, "")
	if got := outcomes(named); got[hr] != "released:" {
		t.Fatalf("a named restricted page: %v", got)
	}
	if w.twinOrEmpty(hidden) != "" {
		t.Fatalf("a page hidden from the releaser was released")
	}
}

// TestLeavingOutFollowsTheTreeAndCanKeepOrTakeOffATwin is PUB-420, PUB-421 and PUB-423.
func TestLeavingOutFollowsTheTreeAndCanKeepOrTakeOffATwin(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "LEAVE")
	section := w.page(w.writer, w.draftHome, "Section", "s")
	w.leaveOut(section, true, "draft section")
	child := w.page(w.writer, section, "New child", "c")
	if s := w.status(w.admin, child); s["status"] != "left_out" {
		t.Fatalf("a new child of a branch left out: %v", s)
	}
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+child+"/release/preview", nil); !strings.Contains(out, "left_out") {
		t.Fatalf("a page left out released alone: %s", out)
	}
	if out := w.admin.want(http.StatusConflict, "DELETE", "/api/v1/pages/"+child+"/leave-out", nil); !strings.Contains(out, "left_out_above") {
		t.Fatalf("including a page left out by its parent: %s", out)
	}
	w.writer.ok("POST", "/api/v1/pages/"+child+"/move", map[string]any{"parent_id": w.draftHome})
	if s := w.status(w.admin, child); s["status"] != "not_released" {
		t.Fatalf("a page moved out of a branch left out: %v", s)
	}
	if out := w.admin.want(http.StatusConflict, "PUT", "/api/v1/pages/"+w.draftHome+"/leave-out", map[string]any{}); !strings.Contains(out, "home_page") {
		t.Fatalf("leaving out the home page: %s", out)
	}
	// A released page left out: kept, its twin no longer changes.
	kept := w.page(w.writer, w.draftHome, "Kept", "v1")
	w.release(w.admin, kept, "")
	w.leaveOut(kept, false, "frozen")
	w.edit(w.writer, kept, "Kept", para("v2 in the draft"))
	if s := w.status(w.admin, kept); s["status"] != "left_out_released" {
		t.Fatalf("a released page left out: %v", s)
	}
	w.wholeSpace("")
	if doc := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+w.twin(kept), nil); !strings.Contains(doc, "v1") || strings.Contains(doc, "v2") {
		t.Fatalf("a twin left out changed: %s", doc)
	}
	// Including it again makes it Changed since release.
	w.admin.ok("DELETE", "/api/v1/pages/"+kept+"/leave-out", nil)
	if s := w.status(w.admin, kept); s["status"] != "changed" {
		t.Fatalf("included again: %v", s)
	}
	// The other answer: take it off now.
	gone := w.page(w.writer, w.draftHome, "Retired", "r")
	w.release(w.admin, gone, "")
	twin := w.twin(gone)
	w.leaveOut(gone, false, "retired")
	w.runAll(w.admin, runBody{"scope": "take_off", "page_ids": []string{gone}}, "Retired")
	if status := e.scalar("SELECT status FROM pages WHERE id = $1", twin); status != "trashed" {
		t.Fatalf("taken off, the twin is %s", status)
	}
	if n := e.auditActions("release.left_out") + e.auditActions("release.included"); n != 4 {
		t.Fatalf("leaving out and including wrote %d entries", n)
	}
}

// TestTakingOffSendsTwinsToTheTrashAndNeverPurges is PUB-440 to PUB-442 and PUB-1015.
func TestTakingOffSendsTwinsToTheTrashAndNeverPurges(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "TAKE")
	parent := w.page(w.writer, w.draftHome, "Parent", "p")
	child := w.page(w.writer, parent, "Child", "c")
	w.wholeSpace("")
	parentTwin, childTwin := w.twin(parent), w.twin(child)
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.draft+"/releases/preview",
		runBody{"scope": "take_off", "page_ids": []string{w.draftHome}}); !strings.Contains(out, "home_page") {
		t.Fatalf("taking off the home page: %s", out)
	}
	preview := w.previewRun(w.admin, runBody{"scope": "take_off", "page_ids": []string{parent}})
	if kept := fmt.Sprint(preview["kept_below"]); !strings.Contains(kept, childTwin) {
		t.Fatalf("the preview does not list the twin that stays: %v", preview)
	}
	w.runAll(w.admin, runBody{"scope": "take_off", "page_ids": []string{parent}}, "")
	if status := e.scalar("SELECT status FROM pages WHERE id = $1", parentTwin); status != "trashed" {
		t.Fatalf("the twin taken off is %s", status)
	}
	if up := e.scalar("SELECT parent_id FROM pages WHERE id = $1", childTwin); up != w.publishedHome {
		t.Fatalf("the child kept moved under %s, not the published home page", up)
	}
	w.reader.want(http.StatusNotFound, "GET", "/api/v1/pages/"+parentTwin, nil)
	if s := w.status(w.admin, parent); s["status"] != "not_released" {
		t.Fatalf("after a take-off the draft page is %v", s)
	}
	// The twin stays in the trash, where nobody restores it while the pair stands.
	item := e.scalar("SELECT trash_item_id FROM pages WHERE id = $1", parentTwin)
	if status, out := w.admin.do("POST", "/api/v1/trash/"+item+"/restore", nil); status < 400 {
		t.Fatalf("a twin was restored from the trash while paired: %d %s", status, out)
	}
	if e.count("SELECT count(*) FROM pages WHERE id = $1", parentTwin) != 1 {
		t.Fatalf("a take-off purged the twin")
	}
	// Releasing it again makes a new twin.
	w.release(w.admin, parent, "")
	if again := w.twin(parent); again == parentTwin {
		t.Fatalf("a page released after its take-off got its old twin back")
	}
	// From the tab Removed in the draft, a twin is taken off by its own id.
	w.admin.ok("DELETE", "/api/v1/pages/"+child, nil)
	w.runAll(w.admin, runBody{"scope": "take_off", "page_ids": []string{childTwin}}, "")
	if status := e.scalar("SELECT status FROM pages WHERE id = $1", childTwin); status != "trashed" {
		t.Fatalf("a twin taken off from the removed tab is %s", status)
	}
}

// TestRemovalsInTheDraftAreMirroredByAWholeSpaceRelease is PUB-331, PUB-1001, PUB-1010 to
// PUB-1014: a deleted, a moved away, a purged and an archived draft page show as Removed in the
// draft with what happened, and a whole-space release takes off or archives their twins;
// restored and unarchived pages come back.
func TestRemovalsInTheDraftAreMirroredByAWholeSpaceRelease(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "MIRROR")
	deleted := w.page(w.writer, w.draftHome, "Deleted", "d")
	moved := w.page(w.writer, w.draftHome, "Moved", "m")
	purged := w.page(w.writer, w.draftHome, "Purged", "p")
	archived := w.page(w.writer, w.draftHome, "Archived", "a")
	restored := w.page(w.writer, w.draftHome, "Restored", "r")
	w.wholeSpace("")
	twins := map[string]string{}
	for _, p := range []string{deleted, moved, purged, archived, restored} {
		twins[p] = w.twin(p)
	}
	other, _ := w.admin.space("MIRROROTHER")
	w.admin.ok("DELETE", "/api/v1/pages/"+deleted, nil)
	w.admin.ok("POST", "/api/v1/pages/"+moved+"/move", map[string]any{"space_id": other})
	w.admin.ok("DELETE", "/api/v1/pages/"+purged, nil)
	w.admin.ok("DELETE", "/api/v1/trash/"+e.scalar("SELECT trash_item_id FROM pages WHERE id = $1", purged), nil)
	w.admin.ok("POST", "/api/v1/pages/"+archived+"/archive", map[string]any{})
	w.admin.ok("DELETE", "/api/v1/pages/"+restored, nil)
	w.admin.ok("POST", "/api/v1/trash/"+e.scalar("SELECT trash_item_id FROM pages WHERE id = $1", restored)+"/restore", nil)
	screen := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases?tab=removed", nil)
	causes := map[string]string{}
	for _, r := range screen["rows"].([]any) {
		row := r.(map[string]any)
		causes[row["title"].(string)] = row["reason"].(string)
	}
	want := map[string]string{"Deleted": "deleted", "Moved": "moved", "Purged": "purged", "Archived": "archived"}
	if fmt.Sprint(causes) != fmt.Sprint(want) {
		t.Fatalf("removed in the draft: %v, want %v", causes, want)
	}
	// A page moved away and back again lost its pair: it is not released yet (PUB-1001).
	back := w.page(w.writer, w.draftHome, "Away and back", "b")
	w.release(w.admin, back, "")
	w.admin.ok("POST", "/api/v1/pages/"+back+"/move", map[string]any{"space_id": other})
	w.admin.ok("POST", "/api/v1/pages/"+back+"/move", map[string]any{"space_id": w.draft, "parent_id": w.draftHome})
	if s := w.status(w.admin, back); s["status"] != "not_released" {
		t.Fatalf("a page moved away and back: %v", s)
	}
	if s := w.status(w.admin, restored); s["status"] != "released" {
		t.Fatalf("a page restored from the trash: %v", s)
	}
	w.wholeSpace("Removals")
	for _, p := range []string{deleted, moved, purged} {
		if status := e.scalar("SELECT status FROM pages WHERE id = $1", twins[p]); status != "trashed" {
			t.Errorf("the twin of a removed page is %s", status)
		}
	}
	if at := e.scalar("SELECT COALESCE(archived_at::text, '') FROM pages WHERE id = $1", twins[archived]); at == "" {
		t.Fatalf("an archived draft page left its twin out of the archive")
	}
	w.admin.ok("POST", "/api/v1/pages/"+archived+"/unarchive", map[string]any{})
	if aspects := w.aspects(archived); !slices.Contains(aspects, "archive") {
		t.Fatalf("unarchived: %v", aspects)
	}
	w.wholeSpace("")
	if at := e.scalar("SELECT COALESCE(archived_at::text, '') FROM pages WHERE id = $1", twins[archived]); at != "" {
		t.Fatalf("an unarchived draft page left its twin archived")
	}
}

// TestSiblingsAreOrderedAsInTheDraftAndOnlyAMovedPageIsMarked is PUB-302 and PUB-412.
func TestSiblingsAreOrderedAsInTheDraftAndOnlyAMovedPageIsMarked(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "ORDER")
	a := w.page(w.writer, w.draftHome, "A", "a")
	b := w.page(w.writer, w.draftHome, "B", "b")
	c := w.page(w.writer, w.draftHome, "C", "c")
	d := w.page(w.writer, w.draftHome, "D", "d")
	// Released and unreleased siblings: C and A first, one by one, out of order.
	w.release(w.admin, c, "")
	w.release(w.admin, a, "")
	order := func() string {
		return e.scalar(`SELECT string_agg(title, '' ORDER BY position, id) FROM pages WHERE space_id = $1 AND parent_id = $2 AND status = 'published'`,
			w.published, w.publishedHome)
	}
	if got := order(); got != "AC" {
		t.Fatalf("twins of siblings released one by one: %s", got)
	}
	w.wholeSpace("")
	if got := order(); got != "ABCD" {
		t.Fatalf("after the whole space: %s", got)
	}
	for _, p := range []string{a, b, c, d} {
		if s := w.status(w.admin, p); s["status"] != "released" {
			t.Fatalf("%s after the release: %v", p, s)
		}
	}
	// Moving D before A rewrites every position, and marks D alone.
	w.writer.ok("POST", "/api/v1/pages/"+d+"/move", map[string]any{"parent_id": w.draftHome, "before_id": a})
	for _, p := range []string{a, b, c} {
		if s := w.status(w.admin, p); s["status"] != "released" {
			t.Errorf("a sibling of the moved page is marked: %v", s)
		}
	}
	if aspects := w.aspects(d); fmt.Sprint(aspects) != "[place]" {
		t.Fatalf("the moved page: %v", aspects)
	}
	w.release(w.admin, d, "")
	if got := order(); got != "DABC" {
		t.Fatalf("after releasing the moved page: %s", got)
	}
	// Moving it back undoes it.
	w.writer.ok("POST", "/api/v1/pages/"+d+"/move", map[string]any{"parent_id": w.draftHome})
	w.writer.ok("POST", "/api/v1/pages/"+d+"/move", map[string]any{"parent_id": w.draftHome, "before_id": a})
	if s := w.status(w.admin, d); s["status"] != "released" {
		t.Fatalf("moved and moved back: %v", s)
	}
}

// TestLinksInOneRunResolveBothWaysAndWaitingLinksMarkTheirPage is PUB-306, PUB-512 to PUB-514.
func TestLinksInOneRunResolveBothWaysAndWaitingLinksMarkTheirPage(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "LINKS")
	first := w.page(w.writer, w.draftHome, "First", "x")
	second := w.page(w.writer, w.draftHome, "Second", "x")
	w.edit(w.writer, first, "First", docOf(paraOf(textOf("to "), pageLinkTo(second, ""))))
	w.edit(w.writer, second, "Second", docOf(paraOf(textOf("to "), pageLinkTo(first, ""))))
	later := w.page(w.writer, w.draftHome, "Later", "x")
	w.leaveOut(later, false, "")
	third := w.page(w.writer, w.draftHome, "Third", "x")
	w.edit(w.writer, third, "Third", docOf(paraOf(textOf("to "), pageLinkTo(later, ""))))
	w.wholeSpace("")
	for page, target := range map[string]string{first: second, second: first} {
		if doc := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+w.twin(page), nil); !strings.Contains(doc, w.twin(target)) {
			t.Fatalf("a link within one run does not lead to the twin: %s", doc)
		}
	}
	thirdTwin := w.twin(third)
	before := e.scalar("SELECT current_version FROM pages WHERE id = $1", thirdTwin)
	w.admin.ok("DELETE", "/api/v1/pages/"+later+"/leave-out", nil)
	w.release(w.admin, later, "")
	if aspects := w.aspects(third); !slices.Contains(aspects, "links") {
		t.Fatalf("a page whose link can lead somewhere now: %v", aspects)
	}
	if after := e.scalar("SELECT current_version FROM pages WHERE id = $1", thirdTwin); after != before {
		t.Fatalf("a waiting link changed its twin silently")
	}
	w.release(w.admin, third, "")
	if doc := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+thirdTwin, nil); !strings.Contains(doc, w.twin(later)) {
		t.Fatalf("the waiting link was not made a link by its release: %s", doc)
	}
}

// smallBatches lowers the limits of runs, so a run of a few pages goes in batches of two.
func smallBatches(e *env) {
	l := publishing.DefaultLimits()
	l.OneTransaction, l.BatchSize, l.Pause = 3, 2, 10*time.Millisecond
	e.app.PublishingForTest().SetLimits(l)
}

// TestABatchedRunStopsInsideABatchAndResumesWithTheSameVersions is PUB-451, PUB-514 and
// PUB-1061: a run past the limit goes in batches, its twins made first; a batch that fails
// leaves nothing of itself and the run stopped; resuming goes on with the previewed versions.
func TestABatchedRunStopsInsideABatchAndResumesWithTheSameVersions(t *testing.T) {
	e := newEnv(t)
	smallBatches(e)
	w := newPubWorld(e, "BATCH")
	var list []string
	for i := range 5 {
		list = append(list, w.page(w.writer, w.draftHome, fmt.Sprintf("Page %d", i), fmt.Sprintf("edition one %d", i)))
	}
	w.edit(w.writer, list[0], "Page 0", docOf(paraOf(textOf("to "), pageLinkTo(list[4], ""))))
	preview := w.previewRun(w.admin, runBody{"scope": "space"})
	if preview["batches"] != 3.0 {
		t.Fatalf("six pages in batches of two: %v", preview)
	}
	// The fourth page fails inside the second batch, as a node that dies would.
	e.sql(`CREATE FUNCTION pb2_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
		IF NEW.title = 'Page 2' THEN RAISE EXCEPTION 'injected'; END IF; RETURN NEW; END $$`)
	e.sql(`CREATE TRIGGER pb2_fail BEFORE INSERT ON page_versions FOR EACH ROW EXECUTE FUNCTION pb2_fail()`)
	id := preview["id"].(string)
	w.admin.want(http.StatusAccepted, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+id+"/start", map[string]any{"note": "batched"})
	e.runJobs()
	run := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/"+id, nil)
	if run["status"] != "stopped" || run["batches_done"] != 1.0 {
		t.Fatalf("a run that failed inside its second batch: %v", run)
	}
	// The first batch stays; nothing of the second landed; every twin was made first, unseen.
	if n := w.publishedCount(); n != 2 {
		t.Fatalf("%d published pages after one batch, want the home page and Page 0", n)
	}
	if doc := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+w.twin(list[0]), nil); !strings.Contains(doc, e.scalar(
		"SELECT twin_page_id FROM release_run_pages WHERE run_id = $1 AND draft_page_id = $2", id, list[4])) {
		t.Fatalf("a link of an early batch does not lead to the twin of a later one: %s", doc)
	}
	later := e.scalar("SELECT twin_page_id FROM release_run_pages WHERE run_id = $1 AND draft_page_id = $2", id, list[4])
	w.reader.want(http.StatusNotFound, "GET", "/api/v1/pages/"+later, nil)
	// The draft changes meanwhile; resuming takes the versions of the preview.
	w.edit(w.writer, list[3], "Page 3", para("edition two"))
	e.sql("DROP TRIGGER pb2_fail ON page_versions")
	w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/releases/"+id+"/resume", nil)
	e.runJobs()
	run = w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/"+id, nil)
	if run["status"] != "done" {
		t.Fatalf("resumed: %v", run)
	}
	if doc := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+w.twin(list[3]), nil); !strings.Contains(doc, "edition one 3") {
		t.Fatalf("a resumed run took a newer version: %s", doc)
	}
	if s := w.status(w.admin, list[3]); s["status"] != "changed" {
		t.Fatalf("a page changed after the preview: %v", s)
	}
}

// TestARunStopsWhenItsReleaserLosesTheRightAndCanBeCancelled is PUB-454 and PUB-455.
func TestARunStopsWhenItsReleaserLosesTheRightAndCanBeCancelled(t *testing.T) {
	e := newEnv(t)
	smallBatches(e)
	w := newPubWorld(e, "LOST")
	for i := range 5 {
		w.page(w.writer, w.draftHome, fmt.Sprintf("Page %d", i), "x")
	}
	preview := w.previewRun(w.rel, runBody{"scope": "space"})
	id := preview["id"].(string)
	w.rel.want(http.StatusAccepted, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+id+"/start", map[string]any{})
	w.admin.ok("PUT", "/api/v1/spaces/"+w.draft+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": w.rel.id}, "permissions": []string{permissions.SpaceView}})
	e.runJobs()
	run := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/"+id, nil)
	if run["status"] != "stopped" || run["stop_reason"] != "rights" {
		t.Fatalf("a run of someone who lost space.release: %v", run)
	}
	if n := e.count("SELECT count(*) FROM release_run_pages WHERE run_id = $1 AND outcome = 'released'", id); n != 0 {
		t.Fatalf("a run without rights released %d pages", n)
	}
	// Cancelled, the twins it made go, and it cannot be resumed.
	w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/releases/"+id+"/cancel", nil)
	if n := e.count("SELECT count(*) FROM pages WHERE space_id = $1 AND current_version = 0", w.published); n != 0 {
		t.Fatalf("a cancelled run left %d unseen twins", n)
	}
	w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+id+"/resume", nil)
	// A single release waits while a run goes.
	page := w.page(w.writer, w.draftHome, "Single", "x")
	queued := w.previewRun(w.admin, runBody{"scope": "space"})
	w.admin.want(http.StatusAccepted, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+queued["id"].(string)+"/start", map[string]any{})
	single := w.preview(w.admin, page)
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": single["id"]}); !strings.Contains(out, "run_going") {
		t.Fatalf("a single release during a run: %s", out)
	}
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.published+"/publishing/end", nil); !strings.Contains(out, "run_going") {
		t.Fatalf("ending a pair during a run: %s", out)
	}
	e.runJobs()
}

// TestARunMeetsTheLimitsOfStorageAndSizeAndAMissingFile is PUB-1054, PUB-1055 and PUB-1065.
func TestARunMeetsTheLimitsOfStorageAndSizeAndAMissingFile(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "LIMIT")
	parent := w.page(w.writer, w.draftHome, "With a file", "x")
	child := w.page(w.writer, parent, "Below the file", "x")
	other := w.page(w.writer, w.draftHome, "Plain", "x")
	file := w.writer.file(parent, "big.txt", strings.Repeat("z", 3<<20))
	w.edit(w.writer, parent, "With a file", docOf(map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": file, "filename": "big.txt"}}))
	root := admin(e)
	root.setLimit(w.published, 1)
	preview := w.previewRun(w.admin, runBody{"scope": "space"})
	if preview["fits"] != false || preview["bytes"].(float64) < 3<<20 {
		t.Fatalf("a preview past the room: %v", preview)
	}
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+preview["id"].(string)+"/start", map[string]any{}); !strings.Contains(out, "no_room") {
		t.Fatalf("a run past the room started: %s", out)
	}
	root.setLimit(w.published, 0)
	// The stored object goes missing: that page stops with its children, the others go on.
	e.sql("UPDATE attachment_versions SET object_key = 'attachments/pb2-missing' WHERE attachment_id = $1", file)
	run := w.wholeSpace("")
	got := outcomes(run)
	if got[parent] != "skipped:file_missing" || got[child] != "skipped:below_failed" || got[other] != "released:" {
		t.Fatalf("a missing file: %v", got)
	}
	// A run past the size limit is refused.
	l := publishing.DefaultLimits()
	l.MaxPages = 1
	e.app.PublishingForTest().SetLimits(l)
	w.edit(w.writer, other, "Plain", para("changed"))
	w.page(w.writer, w.draftHome, "Another", "x")
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.draft+"/releases/preview", runBody{"scope": "space"}); !strings.Contains(out, "run_too_large") {
		t.Fatalf("a run past the size limit: %s", out)
	}
}

// TestTheConsistencyCheckFindsAndFixesWhatDrifted is PUB-343 and PUB-921.
func TestTheConsistencyCheckFindsAndFixesWhatDrifted(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "CHECK")
	page := w.page(w.writer, w.draftHome, "Checked", "x")
	w.release(w.admin, page, "")
	base := "/api/v1/spaces/" + w.draft + "/releases/check"
	if res := w.admin.ok("POST", base, map[string]any{}); len(res["found"].([]any)) != 0 {
		t.Fatalf("a clean pair: %v", res)
	}
	e.sql("UPDATE page_pairs SET released_title = 'Corrupted', changed = '{title}' WHERE draft_page_id = $1", page)
	twin := w.twin(page)
	e.sql(`DO $$ BEGIN PERFORM set_config('diffluence.release', 'on', true);
		INSERT INTO page_versions (id, origin_instance_id, page_id, number, title, document, schema_version, author_id, summary)
		SELECT gen_random_uuid(), origin_instance_id, page_id, 2, title, document, schema_version, author_id, 'by hand'
		FROM page_versions WHERE page_id = '` + twin + `' AND number = 1; END $$`)
	found := fmt.Sprint(w.admin.ok("POST", base, map[string]any{})["found"])
	for _, want := range []string{"facts", "status", "unreleased_version"} {
		if !strings.Contains(found, want) {
			t.Fatalf("the check missed %s: %s", want, found)
		}
	}
	if title := e.scalar("SELECT released_title FROM page_pairs WHERE draft_page_id = $1", page); title != "Corrupted" {
		t.Fatalf("a check that only looks changed the facts")
	}
	w.admin.ok("POST", base, map[string]any{"fix": true})
	if s := w.status(w.admin, page); s["status"] != "released" {
		t.Fatalf("after the fix: %v", s)
	}
	w.rel.want(http.StatusForbidden, "POST", base, map[string]any{})
}

// TestARunIsOneFeedItemAndOneNotificationPerPerson is PUB-810, PUB-813, PUB-815, PUB-817,
// PUB-818, PUB-821 and PUB-822.
func TestARunIsOneFeedItemAndOneNotificationPerPerson(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "TELL")
	w.reader.ok("PUT", "/api/v1/spaces/"+w.published+"/watch", map[string]any{})
	var list []string
	for i := range 12 {
		list = append(list, w.page(w.writer, w.draftHome, fmt.Sprintf("Note %d", i), "x"))
	}
	w.edit(w.writer, list[0], "Note 0", docOf(map[string]any{"type": "taskList", "content": []any{map[string]any{"type": "taskItem",
		"attrs": map[string]any{"checked": false}, "content": []any{paraOf(textOf("a task in a twin"))}}}}))
	w.wholeSpace("Twelve notes")
	e.deliver()
	e.runJobs()
	e.deliver()
	runs := 0
	for _, n := range w.reader.notifications() {
		if n["type"] == "pages_released" {
			runs++
		}
		if n["type"] == "page_changed" && n["page_id"] != w.publishedHome {
			t.Errorf("a watcher was told of a page of a run: %v", n)
		}
	}
	if runs != 1 {
		t.Fatalf("a watcher got %d notifications of one run", runs)
	}
	told := 0
	for _, n := range w.admin.notifications() {
		if n["type"] == "release_finished" {
			told++
		}
	}
	if told != 1 {
		t.Fatalf("the releaser was told %d times", told)
	}
	feed := w.reader.ok("GET", "/api/v1/feed?tab=recent", nil)
	items := 0
	for _, it := range feed["items"].([]any) {
		item := it.(map[string]any)
		if item["kind"] == "released" {
			items++
			if item["pages"] != 13.0 {
				t.Errorf("the feed item counts %v pages", item["pages"])
			}
		}
		if strings.HasPrefix(fmt.Sprint(item["title"]), "Note ") {
			t.Errorf("the feed shows a page of a run on its own: %v", item)
		}
	}
	if items != 1 {
		t.Fatalf("the feed shows %d items for one run", items)
	}
	if n := e.count("SELECT count(*) FROM page_tasks t JOIN pages p ON p.id = t.page_id WHERE p.space_id = $1", w.published); n != 0 {
		t.Fatalf("a released task list made %d task rows", n)
	}
	// A silent run tells no watcher; the feed still shows it. Rows are counted in the table,
	// since the list puts the rows of one page and kind on one line.
	runRows := func() int {
		return e.count(`SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'pages_released'`, w.reader.id)
	}
	if n := runRows(); n != 1 {
		t.Fatalf("a watcher has %d rows of one run", n)
	}
	before := runRows()
	for _, p := range list {
		w.edit(w.writer, p, "Note", para("again"))
	}
	quiet := w.previewRun(w.admin, runBody{"scope": "space"})
	w.admin.want(http.StatusAccepted, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+quiet["id"].(string)+"/start",
		map[string]any{"note": "Quiet", "silent": true})
	e.runJobs()
	e.deliver()
	if after := runRows(); after != before {
		t.Fatalf("a silent run told a watcher: %d notifications, before %d", after, before)
	}
	if feed := w.reader.want(http.StatusOK, "GET", "/api/v1/feed?tab=recent", nil); !strings.Contains(feed, "Quiet") {
		t.Fatalf("a silent run is not in the feed: %s", feed)
	}
}

// TestTheHistoryTellsWhatReadersHadOnADate is PUB-910, PUB-912 and PUB-913.
func TestTheHistoryTellsWhatReadersHadOnADate(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "HIST")
	page := w.page(w.writer, w.draftHome, "Rules", "one")
	moments := []time.Time{time.Now().UTC()}
	for i, text := range []string{"one", "two", "three"} {
		if i > 0 {
			w.edit(w.writer, page, "Rules", para(text))
		}
		time.Sleep(20 * time.Millisecond)
		w.wholeSpace(fmt.Sprintf("Run %d", i+1))
		time.Sleep(20 * time.Millisecond)
		moments = append(moments, time.Now().UTC())
	}
	history := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/history", nil)["runs"].([]any)
	if len(history) != 3 || history[0].(map[string]any)["note"] != "Run 3" {
		t.Fatalf("the history, newest first: %v", history)
	}
	for i, at := range moments {
		out := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/as-of?page_id="+page+"&at="+at.Format(time.RFC3339Nano), nil)
		got := out["pages"].([]any)
		if i == 0 {
			if len(got) != 0 {
				t.Fatalf("before the first run readers had %v", got)
			}
			continue
		}
		if len(got) != 1 || got[0].(map[string]any)["twin_version"] != float64(i) {
			t.Fatalf("at moment %d readers had %v", i, got)
		}
	}
	csv := w.admin.want(http.StatusOK, "GET", "/api/v1/spaces/"+w.draft+"/releases/export?format=csv", nil)
	if strings.Count(csv, page) != 3 || !strings.Contains(csv, "content hash") {
		t.Fatalf("the export: %s", csv)
	}
	lines := w.admin.want(http.StatusOK, "GET", "/api/v1/spaces/"+w.draft+"/releases/export?format=jsonl", nil)
	if strings.Count(lines, "\n") != 4 { // the home page in the first run, and the page in three
		t.Fatalf("the export as JSON Lines: %s", lines)
	}
	if e.auditActions("release.history_exported") != 2 {
		t.Fatalf("an export was not audited")
	}
}

// TestEndingRelinkingAndDeletingSpacesOfAPair is PUB-243 to PUB-246, PUB-1034 and PUB-1046 to
// PUB-1048.
func TestEndingRelinkingAndDeletingSpacesOfAPair(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "ENDS")
	page := w.page(w.writer, w.draftHome, "Kept", "one")
	edited := w.page(w.writer, w.draftHome, "Edited", "one")
	w.wholeSpace("")
	w.edit(w.writer, page, "Kept", para("two"))
	if preview := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/publishing/end-preview", nil); preview["out_of_step"] != 1.0 {
		t.Fatalf("the preview of the end: %v", preview)
	}
	w.admin.want(http.StatusNoContent, "POST", "/api/v1/spaces/"+w.draft+"/publishing/end", nil)
	// Edits on both sides while it is ended.
	twin := w.twin2(edited)
	w.admin.ok("POST", "/api/v1/pages/"+twin+"/publish", map[string]any{"title": "Edited", "document": para("by hand"), "base_version": 1})
	preview := w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/publishing/link-preview", map[string]any{"published_space_id": w.published})
	if preview["restores"] != 3.0 || preview["edited_by_hand"] != 1.0 {
		t.Fatalf("the preview of linking again: %v", preview)
	}
	w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/publishing/link", map[string]any{"published_space_id": w.published})
	if s := w.status(w.admin, page); s["status"] != "changed" {
		t.Fatalf("a page changed while the pair was ended: %v", s)
	}
	if s := w.status(w.admin, edited); s["status"] != "changed" || w.twin(edited) != twin {
		t.Fatalf("a twin edited by hand while the pair was ended: %v", s)
	}
	w.release(w.admin, edited, "")
	if doc := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+twin, nil); strings.Contains(doc, "by hand") {
		t.Fatalf("a release did not overwrite the edit by hand")
	}
	// Archiving either space stops runs and keeps the pair.
	w.admin.ok("POST", "/api/v1/spaces/"+w.published+"/archive", map[string]any{})
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.draft+"/releases/preview", runBody{"scope": "space"}); !strings.Contains(out, "space_not_active") {
		t.Fatalf("a run into an archived space: %s", out)
	}
	w.admin.ok("POST", "/api/v1/spaces/"+w.published+"/unarchive", map[string]any{})
	// Deleting the published space ends the pair; the draft space keeps everything.
	w.admin.ok("DELETE", "/api/v1/spaces/"+w.published, nil)
	if state := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/publishing", nil); state["side"] != "" {
		t.Fatalf("after the published space was deleted: %v", state)
	}
	w.edit(w.writer, page, "Kept", para("three"))
}

// twin2 reads the twin of a draft page from the table, while its pair is ended.
func (w pubWorld) twin2(page string) string {
	w.e.t.Helper()
	return w.e.scalar("SELECT twin_page_id FROM page_pairs WHERE draft_page_id = $1", page)
}

// TestTheReleaseScreenListsThePagesThatWaitedLongestFirst is PUB-1103, PUB-1210 and PUB-1220.
func TestTheReleaseScreenListsThePagesThatWaitedLongestFirst(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "SCREEN")
	old := w.page(w.writer, w.draftHome, "Old change", "x")
	recent := w.page(w.writer, w.draftHome, "Recent change", "x")
	w.wholeSpace("")
	w.edit(w.writer, recent, "Recent change", para("y"))
	time.Sleep(10 * time.Millisecond)
	e.sql("UPDATE page_pairs SET changed_since = now() - interval '3 days' WHERE draft_page_id = $1", old)
	w.edit(w.writer, old, "Old change", para("y"))
	fresh := w.page(w.writer, w.draftHome, "Never released", "x")
	db := w.page(w.writer, w.draftHome, "Ledger", "x")
	e.sql("UPDATE pages SET kind = 'database' WHERE id = $1", db)
	screen := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases", nil)
	tiles := screen["tiles"].(map[string]any)
	if tiles["changed"] != 2.0 || tiles["not_released"] != 1.0 || tiles["cannot_release"] != 1.0 {
		t.Fatalf("tiles: %v", tiles)
	}
	var titles []string
	for _, r := range screen["rows"].([]any) {
		titles = append(titles, r.(map[string]any)["title"].(string))
	}
	if titles[0] != "Old change" {
		t.Fatalf("the oldest change is not first: %v", titles)
	}
	if !slices.Contains(titles, "Never released") || !slices.Contains(titles, "Ledger") {
		t.Fatalf("rows: %v", titles)
	}
	if titles[len(titles)-1] == "Old change" || fresh == "" {
		t.Fatalf("rows: %v", titles)
	}
	w.admin.want(http.StatusBadRequest, "GET", "/api/v1/spaces/"+w.draft+"/releases?tab=nonsense", nil)
}

// TestANewVersionOfAFileMarksItsPageAndItsReleaseCopiesIt is PUB-305 and PUB-307: a file the
// twin holds a copy of gets a newer version in the draft, so its page shows files changed, in
// words, and the next release copies it without a new version of the page.
func TestANewVersionOfAFileMarksItsPageAndItsReleaseCopiesIt(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "FILES")
	page := w.page(w.writer, w.draftHome, "Forms", "x")
	file := w.writer.file(page, "form.txt", "first form")
	w.edit(w.writer, page, "Forms", docOf(map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": file, "filename": "form.txt"}}))
	w.wholeSpace("")
	if s := w.status(w.admin, page); s["status"] != "released" {
		t.Fatalf("after the release: %v", s)
	}
	if status, out := w.writer.newVersion(file, "form.txt", "second form"); status >= 300 {
		t.Fatalf("a new version of the file: %d %s", status, out)
	}
	if aspects := w.aspects(page); fmt.Sprint(aspects) != "[files]" {
		t.Fatalf("a new version of a file shown on a released page: %v", aspects)
	}
	twin := w.twin(page)
	before := e.scalar("SELECT current_version FROM pages WHERE id = $1", twin)
	w.wholeSpace("")
	if after := e.scalar("SELECT current_version FROM pages WHERE id = $1", twin); after != before {
		t.Fatalf("copying a new file version made a version of the page")
	}
	if s := w.status(w.admin, page); s["status"] != "released" {
		t.Fatalf("after the second release: %v", s)
	}
}
