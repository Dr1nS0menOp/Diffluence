// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Draft and published spaces, wave PB4 of spec 073: a draft space started from a space that
// already has content (PUB-1302 to PUB-1311), and the space-wide verification (PUB-920). The
// refusals and the leak paths come first.

// startWorld is a space with content and its people, before a draft space is started from it: an
// administrator who may make spaces, two editors, a reader, a restricted page that one editor and
// the reader may not see, a page with a file and a label, a page that links to another, and a
// blog post.
type startWorld struct {
	e                                *env
	admin, editor, editor2, reader   *user
	space, home, key                 string
	guide, chapter, secret, post     string
	file                             string
	draft, draftKey, draftName       string
	draftHome, draftGuide, draftChap string
}

func newStartWorld(e *env, key string) startWorld {
	e.t.Helper()
	k := strings.ToLower(key)
	s := startWorld{e: e, key: key}
	s.admin = e.user(k+"-admin@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	s.editor = e.user(k+"-editor@example.com", identity.PermUseProduct)
	s.editor2 = e.user(k+"-editor2@example.com", identity.PermUseProduct)
	s.reader = e.user(k+"-reader@example.com", identity.PermUseProduct)
	s.space, s.home = s.admin.space(key)
	s.admin.grant(s.space, "user", s.editor.id, permissions.TemplateEditor)
	s.admin.grant(s.space, "user", s.editor2.id, permissions.TemplateEditor)
	s.admin.grant(s.space, "user", s.reader.id, permissions.TemplateViewer)
	s.guide = s.admin.page(s.space, s.home, "Safety guide", "wear a helmet")
	s.file = s.admin.file(s.guide, "form.txt", "the form")
	s.admin.ok("POST", "/api/v1/pages/"+s.guide+"/publish", map[string]any{"title": "Safety guide", "base_version": 1,
		"document": docOf(paraOf(textOf("wear a helmet")),
			map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": s.file, "filename": "form.txt"}})})
	s.admin.ok("POST", "/api/v1/pages/"+s.guide+"/labels", map[string]any{"label": "safety"})
	s.chapter = s.admin.page(s.space, s.guide, "Chapter one", "climb")
	s.admin.ok("POST", "/api/v1/pages/"+s.chapter+"/publish", map[string]any{"title": "Chapter one", "base_version": 1,
		"document": docOf(paraOf(textOf("see "), pageLinkTo(s.guide, "top"), textOf(" and "),
			map[string]any{"type": "text", "text": "the space", "marks": []any{map[string]any{"type": "link",
				"attrs": map[string]any{"href": "/spaces/" + key + "/pages"}}}}))})
	s.secret = s.admin.page(s.space, s.home, "Salary table", "secret numbers")
	s.admin.restrict(s.secret, "view", s.admin.id, s.editor.id)
	s.post = s.admin.ok("POST", "/api/v1/pages", map[string]any{"space_id": s.space, "kind": "post", "title": "News"})["id"].(string)
	s.admin.ok("POST", "/api/v1/pages/"+s.post+"/publish", map[string]any{"title": "News", "document": para("old news"), "base_version": 0})
	return s
}

func (s startWorld) startBody() map[string]any {
	return map[string]any{"key": s.key + "DRAFT", "name": "Drafts of " + strings.ToLower(s.key)}
}

// start starts a draft space from the world's space and lets the worker carry it out.
func (s *startWorld) start() map[string]any {
	s.e.t.Helper()
	job := s.admin.ok("POST", "/api/v1/spaces/"+s.space+"/publishing/start", s.startBody())["job_id"].(string)
	s.e.runJobs()
	got := s.admin.ok("GET", "/api/v1/jobs/"+job, nil)
	if got["status"] != "done" {
		s.e.t.Fatalf("the start did not finish: %v", got)
	}
	result := got["result"].(map[string]any)
	s.draft, s.draftKey, s.draftName = result["draft_space_id"].(string), result["draft_key"].(string), s.startBody()["name"].(string)
	s.draftHome = s.admin.ok("GET", "/api/v1/spaces/"+s.draft, nil)["home_page_id"].(string)
	s.draftGuide, s.draftChap = s.copyOf(s.guide), s.copyOf(s.chapter)
	return result
}

// copyOf is the page of the draft space paired with a page of the old space.
func (s startWorld) copyOf(page string) string {
	s.e.t.Helper()
	return s.e.scalar("SELECT draft_page_id::text FROM page_pairs WHERE twin_page_id = $1", page)
}

// TestAStartIsRefusedWhereItWouldBreakTheModel is PUB-1302 and PUB-200: a start needs the right
// to run the space and to make spaces, a space in no pair and not personal, a free key and no
// page under a workflow; a refused start makes nothing.
func TestAStartIsRefusedWhereItWouldBreakTheModel(t *testing.T) {
	e := newEnv(t)
	s := newStartWorld(e, "SREF")
	body := s.startBody()
	for _, path := range []string{"/start-preview", "/start"} {
		if status, out := s.editor.do("POST", "/api/v1/spaces/"+s.space+"/publishing"+path, body); status != http.StatusForbidden {
			t.Errorf("an editor %s: %d %s", path, status, out)
		}
		if status, out := s.reader.do("POST", "/api/v1/spaces/"+s.space+"/publishing"+path, body); status != http.StatusForbidden {
			t.Errorf("a reader %s: %d %s", path, status, out)
		}
	}
	// Running the space is not enough: the start makes a space.
	runner := e.user("sref-runner@example.com", identity.PermUseProduct)
	s.admin.grant(s.space, "user", runner.id, permissions.TemplateSpaceAdmin)
	runner.want(http.StatusForbidden, "POST", "/api/v1/spaces/"+s.space+"/publishing/start-preview", body)
	if got := s.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+s.space+"/publishing/start",
		map[string]any{"key": s.key, "name": "Same key"}); !strings.Contains(got, "key_taken") {
		t.Errorf("a key in use: %s", got)
	}
	s.admin.want(http.StatusBadRequest, "POST", "/api/v1/spaces/"+s.space+"/publishing/start", map[string]any{"key": "x", "name": "Bad key"})
	w := newPubWorld(e, "SREFPAIR")
	for _, side := range []string{w.draft, w.published} {
		if got := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+side+"/publishing/start",
			map[string]any{"key": "SREFNEW", "name": "New"}); !strings.Contains(got, "already_paired") {
			t.Errorf("a space in a pair: %s", got)
		}
	}
	solo := e.user("sref-solo@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermPersonalSpace)
	personal := solo.ok("POST", "/api/v1/me/personal-space", nil)["id"].(string)
	solo.page(personal, nil, "Mine", "x")
	if got := solo.want(http.StatusConflict, "POST", "/api/v1/spaces/"+personal+"/publishing/start",
		map[string]any{"key": "SOLODRAFT", "name": "Solo"}); !strings.Contains(got, "personal_space") {
		t.Errorf("a personal space: %s", got)
	}
	// A published space holds no workflow (PUB-703).
	flow := s.admin.ok("POST", "/api/v1/spaces/"+s.space+"/workflows", map[string]any{"name": "Review", "definition": reviewFlow()})["id"].(string)
	s.admin.ok("PUT", "/api/v1/pages/"+s.chapter+"/workflow", map[string]any{"workflow_id": flow})
	if got := s.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+s.space+"/publishing/start", body); !strings.Contains(got, "controlled_pages") {
		t.Errorf("a space with a page under a workflow: %s", got)
	}
	if n := e.count("SELECT count(*) FROM space_pairs"); n != 1 {
		t.Fatalf("refused starts left %d pairs", n)
	}
	if n := e.count("SELECT count(*) FROM jobs WHERE kind = 'publishing.start'"); n != 0 {
		t.Fatalf("refused starts queued %d jobs", n)
	}
}

// TestAReaderOfTheOldSpaceNeverReachesTheDraftsAfterAStart is the leak test of PB1 (PUB-108,
// PUB-620 to PUB-635) for a start: the old space is the published space now, and every door of
// it, rendered for its reader, holds no id, key or name of the new draft space and no word only
// the drafts have. The draft space has only the writers' grants, so its routes do not exist for
// the reader; a restricted page stays restricted on both sides (PUB-1302).
func TestAReaderOfTheOldSpaceNeverReachesTheDraftsAfterAStart(t *testing.T) {
	e := newEnv(t)
	s := newStartWorld(e, "SLEAK")
	s.reader.ok("PUT", "/api/v1/spaces/"+s.space+"/watch", map[string]any{})
	s.start()
	copies := []string{s.draftHome, s.draftGuide, s.draftChap, s.copyOf(s.secret)}
	// Writers move on in the draft space, with words readers have not had.
	s.editor.ok("POST", "/api/v1/pages/"+s.draftChap+"/publish", map[string]any{"title": "Chapter one", "document": para("climb, wombatfjord edition"),
		"base_version": 1, "summary": "draft summary zanzibar"})
	unreleased := s.editor.page(s.draft, s.draftHome, "Unreleased quokkaridge", "quokkaridge thoughts")
	e.deliver()
	e.runJobs()
	doors := []string{
		"/api/v1/spaces/" + s.space,
		"/api/v1/spaces/" + s.space + "/tree",
		"/api/v1/spaces/" + s.space + "/publishing",
		"/api/v1/search?q=helmet",
		"/api/v1/search?q=climb",
		"/api/v1/search/quick?q=Chapter",
		"/api/v1/feed",
		"/api/v1/feed?tab=recent",
		"/api/v1/me/notifications",
		"/api/v1/spaces",
	}
	for _, twin := range []string{s.home, s.guide, s.chapter} {
		doors = append(doors, "/api/v1/pages/"+twin, "/api/v1/pages/"+twin+"/versions", "/api/v1/pages/"+twin+"/release",
			"/api/v1/pages/"+twin+"/export.md", "/api/v1/pages/"+twin+"/details", "/api/v1/pages/"+twin+"/comments")
	}
	forbidden := append([]string{s.draft, unreleased, strings.ToLower(s.draftKey), s.draftName, "wombatfjord", "quokkaridge", "zanzibar"}, copies...)
	for _, path := range doors {
		status, out := s.reader.do("GET", path, nil)
		if status != http.StatusOK {
			t.Errorf("%s: %d %s", path, status, out)
			continue
		}
		lower := strings.ToLower(out)
		for _, word := range forbidden {
			if strings.Contains(lower, strings.ToLower(word)) {
				t.Errorf("%s shows %q to a reader of the old space: %s", path, word, out)
			}
		}
	}
	// The draft space and every route of the draft side do not exist for the reader.
	for _, path := range append([]string{"/api/v1/spaces/" + s.draft, "/api/v1/spaces/" + s.draft + "/tree",
		"/api/v1/spaces/" + s.draft + "/releases", "/api/v1/spaces/" + s.draft + "/releases/history"}, pagePaths(copies)...) {
		if status, out := s.reader.do("GET", path, nil); status != http.StatusNotFound && status != http.StatusForbidden {
			t.Errorf("%s answers the reader: %d %s", path, status, out)
		}
	}
	for _, path := range []string{"/releases/verify"} {
		if status, out := s.reader.do("POST", "/api/v1/spaces/"+s.draft+path, nil); status != http.StatusNotFound && status != http.StatusForbidden {
			t.Errorf("%s answers the reader: %d %s", path, status, out)
		}
	}
	// Only writers came along: the reader's grant stayed behind (PUB-1302).
	grants := s.admin.want(http.StatusOK, "GET", "/api/v1/spaces/"+s.draft+"/grants", nil)
	if strings.Contains(grants, s.reader.id) || !strings.Contains(grants, s.editor.id) || !strings.Contains(grants, s.editor2.id) {
		t.Errorf("the draft space's grants: %s", grants)
	}
	if n := e.count(`SELECT count(*) FROM space_grants WHERE space_id = $1 AND subject_id = $2`, s.draft, s.reader.id); n != 0 {
		t.Errorf("the reader holds %d grants in the draft space", n)
	}
	// The restricted page is restricted on both sides: the editor outside it sees neither.
	secretCopy := s.copyOf(s.secret)
	for _, page := range []string{s.secret, secretCopy} {
		if status, _ := s.editor2.do("GET", "/api/v1/pages/"+page, nil); status != http.StatusNotFound && status != http.StatusForbidden {
			t.Errorf("a restricted page %s reached someone outside its restriction: %d", page, status)
		}
		s.editor.ok("GET", "/api/v1/pages/"+page, nil)
	}
}

func pagePaths(list []string) []string {
	out := []string{}
	for _, p := range list {
		out = append(out, "/api/v1/pages/"+p, "/api/v1/pages/"+p+"/versions")
	}
	return out
}

// TestAStartPairsEveryPageInStepAndItsLinksLeadToTheCopies is PUB-1302, PUB-1309 and PUB-1310:
// the preview says what changes for whom; every page, folder and link is copied with its files,
// labels and look and paired in step; posts stay; the copies' links lead to the copies; and a
// release maps them back.
func TestAStartPairsEveryPageInStepAndItsLinksLeadToTheCopies(t *testing.T) {
	e := newEnv(t)
	s := newStartWorld(e, "SSTEP")
	s.admin.ok("PATCH", "/api/v1/pages/"+s.guide+"/look", map[string]any{"emoji": "🚀", "full_width": true, "title_align": "start"})
	preview := s.admin.ok("POST", "/api/v1/spaces/"+s.space+"/publishing/start-preview", s.startBody())
	for key, want := range map[string]float64{"pages": 4, "hidden": 0, "restricted": 1, "files": 1, "posts": 1, "databases": 0} {
		if preview[key] != want {
			t.Errorf("preview %s = %v, want %v: %v", key, preview[key], want, preview)
		}
	}
	moving := preview["move_to_draft"].(map[string]any)
	if moving["people"] != 3.0 {
		t.Errorf("those who will edit the draft space instead: %v", moving)
	}
	if n := e.count("SELECT count(*) FROM spaces WHERE key = $1", s.key+"DRAFT"); n != 0 {
		t.Fatalf("a preview made a space")
	}
	s.start()
	// Every copy is in step, and the release screen has nothing to release.
	for _, copy := range []string{s.draftHome, s.draftGuide, s.draftChap, s.copyOf(s.secret)} {
		st := s.admin.ok("GET", "/api/v1/pages/"+copy+"/release", nil)
		if st["status"] != "released" || len(st["aspects"].([]any)) != 0 {
			t.Errorf("copy %s is not paired in step: %v", copy, st)
		}
	}
	screen := s.admin.ok("GET", "/api/v1/spaces/"+s.draft+"/releases", nil)
	if tiles := screen["tiles"].(map[string]any); tiles["changed"] != 0.0 || tiles["not_released"] != 0.0 {
		t.Errorf("after a start the screen has work: %v", tiles)
	}
	if n := e.count("SELECT count(*) FROM pages WHERE space_id = $1 AND kind = 'post'", s.draft); n != 0 {
		t.Errorf("a post was copied")
	}
	// The copy's links lead to the copies, and to the draft space (PUB-1310).
	chapter := s.editor.want(http.StatusOK, "GET", "/api/v1/pages/"+s.draftChap, nil)
	if !strings.Contains(chapter, s.draftGuide) || strings.Contains(chapter, s.guide) || !strings.Contains(chapter, "/spaces/"+s.draftKey+"/pages") {
		t.Errorf("the copy's links do not lead to the copies: %s", chapter)
	}
	guide := s.editor.ok("GET", "/api/v1/pages/"+s.draftGuide, nil)
	if guide["emoji"] != "🚀" || guide["full_width"] != true || strings.Contains(e.scalar("SELECT document::text FROM page_versions WHERE page_id = $1", s.draftGuide), s.file) {
		t.Errorf("the copy's look or file: %v", guide)
	}
	if n := e.count("SELECT count(*) FROM page_labels WHERE page_id = $1 AND label = 'safety'", s.draftGuide); n != 1 {
		t.Errorf("the copy's label is missing")
	}
	if n := e.count("SELECT count(*) FROM attachments WHERE page_id = $1 AND status = 'active'", s.draftGuide); n != 1 {
		t.Errorf("the copy has %d files", n)
	}
	// Editing moved to the draft space: the old space is changed only by releases.
	s.editor.want(http.StatusConflict, "POST", "/api/v1/pages/"+s.chapter+"/publish", map[string]any{"title": "Chapter one",
		"document": para("by hand"), "base_version": 2})
	s.editor.ok("POST", "/api/v1/pages/"+s.draftChap+"/publish", map[string]any{"title": "Chapter one",
		"document": docOf(paraOf(textOf("see, again, "), pageLinkTo(s.draftGuide, "top"))), "base_version": 1})
	if st := s.admin.ok("GET", "/api/v1/pages/"+s.draftChap+"/release", nil); st["status"] != "changed" {
		t.Fatalf("an edit of a copy is not out of step: %v", st)
	}
	preview1 := s.admin.ok("POST", "/api/v1/pages/"+s.draftChap+"/release/preview", nil)
	s.admin.ok("POST", "/api/v1/pages/"+s.draftChap+"/release", map[string]any{"preview_id": preview1["id"]})
	twin := e.scalar(`SELECT v.document::text FROM pages p JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version WHERE p.id = $1`, s.chapter)
	if !strings.Contains(twin, s.guide) || strings.Contains(twin, s.draftGuide) || !strings.Contains(twin, "see, again") {
		t.Errorf("the release did not map the links back: %s", twin)
	}
	// The history shows the start, and the audit has it once.
	history := s.admin.want(http.StatusOK, "GET", "/api/v1/spaces/"+s.draft+"/releases/history", nil)
	if !strings.Contains(history, `"scope":"start"`) {
		t.Errorf("the history does not show the start: %s", history)
	}
	if n := e.auditActions("space.draft_started"); n != 1 {
		t.Errorf("the start was audited %d times", n)
	}
}

// TestAStartIsUndoneUntilTheFirstRelease is PUB-1311: before the first release, undoing the start
// ends the pair and sends the draft space to the trash, and the old space is ordinary again;
// after a release it is refused, and ending the pair follows PUB-243.
func TestAStartIsUndoneUntilTheFirstRelease(t *testing.T) {
	e := newEnv(t)
	s := newStartWorld(e, "SUNDO")
	s.start()
	if end := s.admin.ok("GET", "/api/v1/spaces/"+s.draft+"/publishing/end-preview", nil); end["undo"] != true {
		t.Fatalf("a fresh start cannot be undone: %v", end)
	}
	s.editor.want(http.StatusForbidden, "POST", "/api/v1/spaces/"+s.space+"/publishing/undo-start", nil)
	s.admin.want(http.StatusNoContent, "POST", "/api/v1/spaces/"+s.space+"/publishing/undo-start", nil)
	if status := e.scalar("SELECT status FROM spaces WHERE id = $1", s.draft); status != "trashed" {
		t.Errorf("the draft space is %s after undoing", status)
	}
	s.editor.ok("POST", "/api/v1/pages/"+s.chapter+"/publish", map[string]any{"title": "Chapter one", "document": para("by hand again"), "base_version": 2})
	if n := e.auditActions("space.draft_start_undone"); n != 1 {
		t.Errorf("undoing was audited %d times", n)
	}
	// A second start, then a release: there is no going back by undoing.
	s.key = "SUNDO2"
	s.start()
	s.editor.ok("POST", "/api/v1/pages/"+s.draftChap+"/publish", map[string]any{"title": "Chapter one", "document": para("new"), "base_version": 1})
	preview := s.admin.ok("POST", "/api/v1/pages/"+s.draftChap+"/release/preview", nil)
	s.admin.ok("POST", "/api/v1/pages/"+s.draftChap+"/release", map[string]any{"preview_id": preview["id"]})
	if end := s.admin.ok("GET", "/api/v1/spaces/"+s.draft+"/publishing/end-preview", nil); end["undo"] != false {
		t.Fatalf("a start can be undone after a release: %v", end)
	}
	if got := s.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+s.draft+"/publishing/undo-start", nil); !strings.Contains(got, "released_since_start") {
		t.Fatalf("undoing after a release: %s", got)
	}
	s.admin.want(http.StatusNoContent, "POST", "/api/v1/spaces/"+s.draft+"/publishing/end", nil)
	if status := e.scalar("SELECT status FROM spaces WHERE id = $1", s.draft); status != "active" {
		t.Errorf("ending deleted the draft space: %s", status)
	}
}

// tamper changes a stored version past the trigger that keeps versions as they were written, as
// only someone who owns the database could.
func tamper(e *env, sql string, args ...any) {
	e.t.Helper()
	ctx := context.Background()
	tx, err := e.app.Pool().Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, step := range []string{"ALTER TABLE page_versions DISABLE TRIGGER page_versions_no_update", sql,
		"ALTER TABLE page_versions ENABLE TRIGGER page_versions_no_update"} {
		var err error
		if step == sql {
			_, err = tx.Exec(ctx, step, args...)
		} else {
			_, err = tx.Exec(ctx, step)
		}
		if err != nil {
			e.t.Fatalf("%s: %v", step, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		e.t.Fatal(err)
	}
}

// TestTheSpaceWideVerificationReportsEachOutcome is PUB-920: every twin is checked in the
// background, as one is, and the report says per page whether it is the same as its draft
// version, differs, has a draft version that changed, cannot be verified, or has no draft left.
// A page paired by a start verifies against the copy the start made.
func TestTheSpaceWideVerificationReportsEachOutcome(t *testing.T) {
	e := newEnv(t)
	s := newStartWorld(e, "SVER")
	extra := s.admin.page(s.space, s.home, "Glossary", "words")
	gone := s.admin.page(s.space, s.home, "Old memo", "memo")
	changed := s.admin.page(s.space, s.home, "Draft changed", "first")
	cannot := s.admin.page(s.space, s.home, "Cannot verify", "first")
	s.start()
	verify := func(page string) string {
		t.Helper()
		return s.admin.ok("POST", "/api/v1/pages/"+page+"/release/verify", nil)["outcome"].(string)
	}
	if got := verify(s.draftGuide); got != "verified" {
		t.Fatalf("a page paired by the start: %s", got)
	}
	// A page released since verifies as released.
	s.editor.ok("POST", "/api/v1/pages/"+s.draftChap+"/publish", map[string]any{"title": "Chapter one", "document": para("new words"), "base_version": 1})
	preview := s.admin.ok("POST", "/api/v1/pages/"+s.draftChap+"/release/preview", nil)
	s.admin.ok("POST", "/api/v1/pages/"+s.draftChap+"/release", map[string]any{"preview_id": preview["id"]})
	// A twin changed in the database, a draft version changed in the database, a map lost, and a
	// draft page purged.
	tamper(e, `UPDATE page_versions SET document = jsonb_set(document, '{content,0,content,0,text}', '"by hand"')
		WHERE page_id = $1 AND number = (SELECT current_version FROM pages WHERE id = $1)`, extra)
	draftChanged := s.copyOf(changed)
	tamper(e, `UPDATE page_versions SET document = jsonb_set(document, '{content,0,content,0,text}', '"edited"')
		WHERE page_id = $1 AND number = (SELECT current_version FROM pages WHERE id = $1)`, draftChanged)
	if _, err := e.app.Pool().Exec(context.Background(), `UPDATE release_run_pages SET link_map = '{}' WHERE draft_page_id = $1`, s.copyOf(cannot)); err != nil {
		t.Fatal(err)
	}
	goneCopy := s.copyOf(gone)
	s.admin.ok("DELETE", "/api/v1/pages/"+goneCopy, nil)
	s.admin.ok("DELETE", "/api/v1/trash/"+e.scalar("SELECT trash_item_id FROM pages WHERE id = $1", goneCopy), nil)
	for _, u := range []*user{s.editor, s.reader} {
		if status, out := u.do("POST", "/api/v1/spaces/"+s.draft+"/releases/verify", nil); status != http.StatusForbidden && status != http.StatusNotFound {
			t.Errorf("%s verified the space without space.audit: %d %s", u.email, status, out)
		}
	}
	job := s.admin.want(http.StatusAccepted, "POST", "/api/v1/spaces/"+s.draft+"/releases/verify", nil)
	id := strings.Split(strings.Split(job, `"job_id":"`)[1], `"`)[0]
	e.runJobs()
	report := s.admin.ok("GET", "/api/v1/jobs/"+id, nil)
	if report["status"] != "done" {
		t.Fatalf("the verification did not finish: %v", report)
	}
	result := report["result"].(map[string]any)
	counts := result["counts"].(map[string]any)
	for outcome, want := range map[string]float64{"verified": 4, "differs": 1, "draft_changed": 1, "cannot_verify": 1, "draft_purged": 1} {
		if counts[outcome] != want {
			t.Errorf("%s: %v, want %v (%v)", outcome, counts[outcome], want, counts)
		}
	}
	byTwin := map[string]string{}
	for _, p := range result["pages"].([]any) {
		page := p.(map[string]any)
		byTwin[page["twin_page_id"].(string)] = page["outcome"].(string)
	}
	for twin, want := range map[string]string{extra: "differs", changed: "draft_changed", cannot: "cannot_verify", gone: "draft_purged",
		s.chapter: "verified", s.guide: "verified"} {
		if byTwin[twin] != want {
			t.Errorf("twin %s: %q, want %q", twin, byTwin[twin], want)
		}
	}
	if n := e.auditActions("release.space_verified"); n != 1 {
		t.Errorf("the verification was audited %d times", n)
	}
}
