// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Draft and published spaces, wave PB1 of spec 073. The pure rule is proved by the permission
// matrix and the rewrite by the tests of internal/publishing; these tests prove the routes,
// the trigger, a release of one page, and every door a draft could leak through.

// pubWorld is a draft space linked to a new published space, with people of each kind: the
// administrator of both, a writer of the draft, a reader of the published space only, a person
// who reads both, and a release manager who holds space.release alone.
type pubWorld struct {
	e                                *env
	admin, writer, reader, both, rel *user
	draft, draftHome, draftKey       string
	published, publishedHome, pubKey string
	draftName                        string
}

func newPubWorld(e *env, key string) pubWorld {
	e.t.Helper()
	k := strings.ToLower(key)
	w := pubWorld{e: e, draftKey: key + "DRAFT", pubKey: key + "READ", draftName: "Space " + key + "DRAFT"}
	w.admin = e.user(k+"-admin@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	w.writer = e.user(k+"-writer@example.com", identity.PermUseProduct)
	w.reader = e.user(k+"-reader@example.com", identity.PermUseProduct)
	w.both = e.user(k+"-both@example.com", identity.PermUseProduct)
	w.rel = e.user(k+"-release@example.com", identity.PermUseProduct)
	w.draft, w.draftHome = w.admin.space(w.draftKey)
	w.admin.grant(w.draft, "user", w.writer.id, permissions.TemplateEditor)
	w.admin.grant(w.draft, "user", w.both.id, permissions.TemplateViewer)
	w.admin.ok("PUT", "/api/v1/spaces/"+w.draft+"/grants", map[string]any{
		"subject":     map[string]any{"type": "user", "id": w.rel.id},
		"permissions": []string{permissions.SpaceView, permissions.SpaceRelease}})
	state := w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/publishing/link",
		map[string]any{"new_space": map[string]any{"key": w.pubKey, "name": "Readers guide " + strings.ToLower(key)}})
	w.published = state["pair"].(map[string]any)["published"].(map[string]any)["id"].(string)
	w.publishedHome = w.admin.ok("GET", "/api/v1/spaces/"+w.published, nil)["home_page_id"].(string)
	for _, u := range []*user{w.reader, w.both, w.rel} {
		w.admin.grant(w.published, "user", u.id, permissions.TemplateViewer)
	}
	return w
}

func (w pubWorld) page(u *user, parent, title, text string) string {
	w.e.t.Helper()
	return u.page(w.draft, parent, title, text)
}

func (w pubWorld) current(u *user, page string) int {
	w.e.t.Helper()
	return int(u.ok("GET", "/api/v1/pages/"+page, nil)["current_version"].(float64))
}

// edit publishes new words on a draft page, with a summary that must never reach readers.
func (w pubWorld) edit(u *user, page, title string, doc map[string]any) {
	w.e.t.Helper()
	u.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": title, "document": doc,
		"base_version": w.current(u, page), "summary": "draft summary zanzibar"})
}

func (w pubWorld) preview(u *user, page string) map[string]any {
	w.e.t.Helper()
	return u.ok("POST", "/api/v1/pages/"+page+"/release/preview", nil)
}

func (w pubWorld) release(u *user, page, note string) map[string]any {
	w.e.t.Helper()
	preview := w.preview(u, page)
	return u.ok("POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": preview["id"], "note": note})
}

func (w pubWorld) status(u *user, page string) map[string]any {
	w.e.t.Helper()
	return u.ok("GET", "/api/v1/pages/"+page+"/release", nil)
}

func (w pubWorld) twin(page string) string {
	w.e.t.Helper()
	twin, _ := w.status(w.admin, page)["twin_page_id"].(string)
	if twin == "" {
		w.e.t.Fatalf("page %s has no twin", page)
	}
	return twin
}

func (w pubWorld) aspects(page string) []string {
	w.e.t.Helper()
	s := w.status(w.admin, page)
	out := []string{}
	for _, a := range s["aspects"].([]any) {
		out = append(out, a.(string))
	}
	sort.Strings(out)
	return out
}

func docOf(blocks ...map[string]any) map[string]any {
	content := []any{}
	for _, b := range blocks {
		content = append(content, b)
	}
	return map[string]any{"type": "doc", "content": content}
}

func paraOf(inline ...map[string]any) map[string]any {
	content := []any{}
	for _, n := range inline {
		content = append(content, n)
	}
	return map[string]any{"type": "paragraph", "content": content}
}

func textOf(text string) map[string]any { return map[string]any{"type": "text", "text": text} }

func pageLinkTo(id, fragment string) map[string]any {
	attrs := map[string]any{"page_id": id}
	if fragment != "" {
		attrs["fragment"] = fragment
	}
	return map[string]any{"type": "pageLink", "attrs": attrs}
}

// TestEveryPublishingRouteIsOffUntilTurnedOnExceptEndingAPair is PUB-212 and PUB-1303: on a new
// installation the part is off, every route says so, and ending a pair stays open (PUB-1304).
func TestEveryPublishingRouteIsOffUntilTurnedOnExceptEndingAPair(t *testing.T) {
	e := newInstalledEnv(t, noSettings)
	root := admin(e)
	space, home := root.space("OFF")
	refused := []struct{ method, path string }{
		{"GET", "/api/v1/spaces/" + space + "/publishing"},
		{"POST", "/api/v1/spaces/" + space + "/publishing/link-preview"},
		{"POST", "/api/v1/spaces/" + space + "/publishing/link"},
		{"PUT", "/api/v1/spaces/" + space + "/publishing/settings"},
		{"GET", "/api/v1/pages/" + home + "/release"},
		{"POST", "/api/v1/pages/" + home + "/release/preview"},
		{"POST", "/api/v1/pages/" + home + "/release"},
	}
	for _, r := range refused {
		if out := root.want(http.StatusNotFound, r.method, r.path, map[string]any{}); !strings.Contains(out, "experimental_off") {
			t.Errorf("%s %s does not say the part is off: %s", r.method, r.path, out)
		}
	}
	if out := root.want(http.StatusConflict, "POST", "/api/v1/spaces/"+space+"/publishing/end", nil); !strings.Contains(out, "not_paired") {
		t.Fatalf("ending a pair is not open while the part is off: %s", out)
	}
	if capabilities := root.ok("GET", "/api/v1/system/capabilities", nil); capabilities["publishing"] != false {
		t.Fatalf("the screens are offered publishing while it is off: %v", capabilities)
	}
}

// TestTurningThePartOffKeepsPublishedSpacesReadOnlyAndEndingOpen is PUB-1304: an off part keeps
// what exists, so nobody edits the published space, and the pair can still be ended, after
// which its editors edit again.
func TestTurningThePartOffKeepsPublishedSpacesReadOnlyAndEndingOpen(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "TOF")
	page := w.page(w.writer, w.draftHome, "Guide", "first words")
	w.release(w.admin, page, "")
	twin := w.twin(page)
	root := admin(e)
	root.ok("PUT", "/api/v1/admin/experimental", map[string]any{"on": []string{"automation", "bundles", "console_restore", "databases"}})
	w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+twin+"/publish", map[string]any{"title": "Guide", "document": para("by hand"), "base_version": 1})
	w.admin.want(http.StatusNotFound, "POST", "/api/v1/pages/"+page+"/release/preview", nil)
	if can := w.admin.ok("GET", "/api/v1/pages/"+twin, nil)["can"].(map[string]any); can["page.edit"] != false {
		t.Fatalf("a twin offers editing while the pair stands: %v", can)
	}
	w.admin.want(http.StatusNoContent, "POST", "/api/v1/spaces/"+w.published+"/publishing/end", nil)
	// Every node reads the facts of the space again at once: its side changed (spec 071).
	if can := w.admin.ok("GET", "/api/v1/pages/"+twin, nil)["can"].(map[string]any); can["page.edit"] != true {
		t.Fatalf("after the end the page still refuses editing: %v", can)
	}
	w.admin.ok("POST", "/api/v1/pages/"+twin+"/publish", map[string]any{"title": "Guide", "document": para("by hand"), "base_version": 1})
	if n := e.auditActions("space.unpaired"); n != 1 {
		t.Fatalf("ending a pair wrote %d entries", n)
	}
}

// TestAPairIsRefusedWhereItWouldBreakTheModel is PUB-200, PUB-211 and PUB-1049 to PUB-1052:
// a second pair on either side, a chain, a pair with itself, two drafts into one, a personal
// space and a published space with content are refused with their reasons, and change nothing.
func TestAPairIsRefusedWhereItWouldBreakTheModel(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "REF")
	other, _ := w.admin.space("REFOTHER")
	full, fullHome := w.admin.space("REFFULL")
	w.admin.page(full, fullHome, "Already here", "x")
	w.admin.file(fullHome, "a.txt", "x")
	link := func(from string, to any) string {
		t.Helper()
		body := map[string]any{"published_space_id": to}
		return w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+from+"/publishing/link", body)
	}
	cases := map[string]struct {
		got, code string
	}{
		"a second published space for a draft": {link(w.draft, other), "already_paired"},
		"a second draft into one published":    {link(other, w.published), "already_published"},
		"a chain from the published space":     {link(w.published, other), "already_paired"},
		"a chain into the draft space":         {link(other, w.draft), "already_paired"},
		"a space with itself":                  {link(other, other), "self_pair"},
		"a published space with content":       {link(other, full), "published_space_not_empty"},
	}
	for name, c := range cases {
		if !strings.Contains(c.got, c.code) {
			t.Errorf("%s: %s, want %s", name, c.got, c.code)
		}
	}
	if got := cases["a published space with content"].got; !strings.Contains(got, "1 pages") || !strings.Contains(got, "1 files") {
		t.Errorf("the refusal does not say what is in the space: %s", got)
	}
	solo := e.user("ref-solo@example.com", identity.PermUseProduct, identity.PermCreateSpace, identity.PermPersonalSpace)
	personal := solo.ok("POST", "/api/v1/me/personal-space", nil)["id"].(string)
	if got := solo.want(http.StatusConflict, "POST", "/api/v1/spaces/"+personal+"/publishing/link",
		map[string]any{"new_space": map[string]any{"key": "SOLOREAD", "name": "Solo"}}); !strings.Contains(got, "personal_space") {
		t.Errorf("a personal space: %s", got)
	}
	if n := e.count("SELECT count(*) FROM space_pairs"); n != 1 {
		t.Fatalf("refused links left %d pairs", n)
	}
	// Linking needs space.administer in both spaces (PUB-011).
	w.admin.grant(other, "user", w.writer.id, permissions.TemplateEditor)
	w.writer.want(http.StatusForbidden, "POST", "/api/v1/spaces/"+other+"/publishing/link-preview", map[string]any{"new_space": map[string]any{"key": "X1", "name": "x"}})
}

// TestTheLinkPreviewCountsWhoLosesEditingAndWarnsAboutOpenDrafts is PUB-210, PUB-610 and
// PUB-611: the preview counts those who will no longer edit, names the public pages of the
// draft space, and warns when everyone can read the drafts.
func TestTheLinkPreviewCountsWhoLosesEditingAndWarnsAboutOpenDrafts(t *testing.T) {
	e := newEnv(t)
	owner := e.user("lp-owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	editor := e.user("lp-editor@example.com", identity.PermUseProduct)
	draft, _ := owner.space("LPDRAFT")
	target, _ := owner.space("LPTARGET")
	owner.grant(target, "user", editor.id, permissions.TemplateEditor)
	owner.grant(target, "all_members", "", permissions.TemplateViewer)
	preview := owner.ok("POST", "/api/v1/spaces/"+draft+"/publishing/link-preview", map[string]any{"published_space_id": target})
	losing := preview["losing_editors"].(map[string]any)
	if losing["people"] != 2.0 || losing["groups"] != 0.0 || preview["drafts_readable"] != false {
		t.Fatalf("preview: %v", preview)
	}
	if names := fmt.Sprint(losing["names"]); !strings.Contains(names, "lp-editor") || !strings.Contains(names, "lp-owner") {
		t.Fatalf("the preview does not name who loses editing: %v", names)
	}
	owner.grant(draft, "all_members", "", permissions.TemplateViewer)
	if again := owner.ok("POST", "/api/v1/spaces/"+draft+"/publishing/link-preview", map[string]any{"published_space_id": target}); again["drafts_readable"] != true {
		t.Fatalf("drafts open to all members are not warned about: %v", again)
	}
	if n := e.count("SELECT count(*) FROM space_pairs"); n != 0 {
		t.Fatalf("a preview made a pair")
	}
}

// TestLinkingMakesAnEmptyManagedSpaceThatNoReaderSees is PUB-211, PUB-213, PUB-214: a new
// published space has no reader but its maker, linking releases nothing, and the two home
// pages are paired but not released.
func TestLinkingMakesAnEmptyManagedSpaceThatNoReaderSees(t *testing.T) {
	e := newEnv(t)
	owner := e.user("lk-owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	member := e.user("lk-member@example.com", identity.PermUseProduct)
	draft, home := owner.space("LKDRAFT")
	owner.page(draft, home, "Secret plan", "only drafts")
	state := owner.ok("POST", "/api/v1/spaces/"+draft+"/publishing/link", map[string]any{"new_space": map[string]any{"key": "LKREAD", "name": "Readers"}})
	if state["side"] != "draft" {
		t.Fatalf("state: %v", state)
	}
	published := state["pair"].(map[string]any)["published"].(map[string]any)["id"].(string)
	member.want(http.StatusNotFound, "GET", "/api/v1/spaces/"+published, nil)
	if n := e.count("SELECT count(*) FROM pages WHERE space_id = $1", published); n != 1 {
		t.Fatalf("linking released %d pages", n-1)
	}
	if got := owner.ok("GET", "/api/v1/pages/"+home+"/release", nil); got["status"] != "not_released" {
		t.Fatalf("the draft home page after linking: %v", got)
	}
	pubState := owner.ok("GET", "/api/v1/spaces/"+published+"/publishing", nil)
	if pubState["side"] != "published" || e.auditActions("space.paired") != 1 || e.auditActions("space.managed") != 1 {
		t.Fatalf("published side: %v", pubState)
	}
}

// TestNobodyWritesAPublishedSpaceByHand is PUB-220, PUB-221 and PUB-602: every write of content
// is refused there, for its administrator who is also an instance administrator too, with
// managed_space; what concerns readers stays allowed.
func TestNobodyWritesAPublishedSpaceByHand(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "MAN")
	page := w.page(w.writer, w.draftHome, "Procedure", "steps")
	w.release(w.admin, page, "")
	twin := w.twin(page)
	root := admin(e)
	w.admin.grant(w.published, "user", root.id, permissions.TemplateSpaceAdmin)
	w.admin.grant(w.published, "user", w.writer.id, permissions.TemplateEditor)
	for _, u := range []*user{w.admin, root, w.writer} {
		refused := []struct {
			method, path string
			body         any
		}{
			{"POST", "/api/v1/pages", map[string]any{"space_id": w.published, "parent_id": w.publishedHome, "title": "By hand"}},
			{"POST", "/api/v1/pages/" + twin + "/publish", map[string]any{"title": "x", "document": para("by hand"), "base_version": 1}},
			{"PATCH", "/api/v1/pages/" + twin + "/look", map[string]any{"emoji": "🚀", "title_align": "start", "header_position": 50}},
			{"POST", "/api/v1/pages/" + twin + "/labels", map[string]any{"label": "by-hand"}},
			{"POST", "/api/v1/pages/" + twin + "/archive", map[string]any{}},
			{"POST", "/api/v1/pages/" + twin + "/versions/1/restore", map[string]any{}},
			{"POST", "/api/v1/pages/" + twin + "/move", map[string]any{"space_id": w.draft, "parent_id": w.draftHome}},
			{"PUT", "/api/v1/pages/" + twin + "/stage", map[string]any{"text": "Done", "color": "green"}},
		}
		for _, r := range refused {
			if out := u.want(http.StatusConflict, r.method, r.path, r.body); !strings.Contains(out, "managed_space") {
				t.Errorf("%s %s %s: %s", u.email, r.method, r.path, out)
			}
		}
		if status, out := u.upload(twin, "by-hand.txt", "x"); status != http.StatusConflict || !strings.Contains(out, "managed_space") {
			t.Errorf("%s uploads into a published page: %d %s", u.email, status, out)
		}
	}
	for _, u := range []*user{w.admin, root} {
		if out := u.want(http.StatusConflict, "DELETE", "/api/v1/pages/"+twin, nil); !strings.Contains(out, "managed_space") {
			t.Errorf("%s deletes a twin: %s", u.email, out)
		}
	}
	// Moving a draft page into the published space is refused too (PUB-1004).
	other := w.page(w.admin, w.draftHome, "Other", "x")
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+other+"/move", map[string]any{"space_id": w.published, "parent_id": w.publishedHome}); !strings.Contains(out, "managed_space") {
		t.Fatalf("a move into the published space: %s", out)
	}
	// Readers' things stay: comments, reactions, restrictions (PUB-221).
	w.reader.comment(twin, "A question from a reader")
	w.reader.want(http.StatusNoContent, "PUT", "/api/v1/pages/"+twin+"/reactions/"+url.PathEscape("👍"), nil)
	w.admin.restrict(twin, "view", w.admin.id, w.reader.id)
	if can := w.admin.ok("GET", "/api/v1/pages/"+twin, nil)["can"].(map[string]any); can["page.edit"] != false || can["attachment.add"] != false {
		t.Fatalf("a twin offers editing: %v", can)
	}
}

// TestTheDatabaseRefusesAWriteIntoAPublishedSpaceWithoutTheReleaseMark is PUB-224 and
// PUB-1306: what a node of the previous release would write, which knows no managed rule, is
// refused by the trigger with its message; a transaction marked as a release goes through.
func TestTheDatabaseRefusesAWriteIntoAPublishedSpaceWithoutTheReleaseMark(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "TRG")
	page := w.page(w.writer, w.draftHome, "Policy", "words")
	w.release(w.admin, page, "")
	twin := w.twin(page)
	ctx := context.Background()
	pool := e.app.Pool()
	writes := map[string]string{
		"a new version":   `INSERT INTO page_versions (id, origin_instance_id, page_id, number, title, document, schema_version, author_id) SELECT gen_random_uuid(), origin_instance_id, id, 9, 'x', '{"type":"doc"}', 1, created_by FROM pages WHERE id = '` + twin + `'`,
		"a rename":        `UPDATE pages SET title = 'edited by an old node' WHERE id = '` + twin + `'`,
		"a new page":      `INSERT INTO pages (id, origin_instance_id, space_id, title, status, created_by, updated_by) SELECT gen_random_uuid(), origin_instance_id, space_id, 'x', 'draft', created_by, created_by FROM pages WHERE id = '` + twin + `'`,
		"a file":          `INSERT INTO attachments (id, origin_instance_id, space_id, page_id, filename, status, created_by) SELECT gen_random_uuid(), origin_instance_id, space_id, id, 'x.txt', 'active', created_by FROM pages WHERE id = '` + twin + `'`,
		"a move away":     `UPDATE pages SET space_id = '` + w.draft + `' WHERE id = '` + twin + `'`,
		"to the trash":    `UPDATE pages SET status = 'trashed' WHERE id = '` + twin + `'`,
		"a look":          `UPDATE pages SET emoji = 'x' WHERE id = '` + twin + `'`,
		"a draft page in": `UPDATE pages SET space_id = '` + w.published + `' WHERE id = '` + page + `'`,
	}
	for name, sql := range writes {
		_, err := pool.Exec(ctx, sql)
		if !db.IsManagedSpace(err) || !strings.Contains(err.Error(), "changed only by releases") {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Not content: an owner, closing comments and review reminders stay free (PUB-221).
	if _, err := pool.Exec(ctx, `UPDATE pages SET comments_closed = true, review_reminded_on = now()::date WHERE id = $1`, twin); err != nil {
		t.Fatalf("a write that is not content was refused: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT set_config('diffluence.release', 'on', true)"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, writes["a rename"]); err != nil {
		t.Fatalf("a release was refused: %v", err)
	}
}

// TestReleasingOnePageMakesATwinReadersSee is PUB-400, PUB-403, PUB-410, PUB-332 and PUB-1408:
// the first release makes the twin under the published home, with the releaser as author and
// the note as summary; a second release makes a version; an unchanged one makes none.
func TestReleasingOnePageMakesATwinReadersSee(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "REL")
	page := w.page(w.writer, w.draftHome, "Onboarding", "welcome aboard")
	w.writer.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "start-here"})
	w.writer.ok("PATCH", "/api/v1/pages/"+page+"/look", map[string]any{"emoji": "🚀", "title_align": "start", "header_position": 50, "full_width": true})
	if got := w.status(w.writer, page); got["status"] != "not_released" || got["can_release"] != false {
		t.Fatalf("before the first release, as a writer: %v", got)
	}
	report := w.release(w.rel, page, "First edition for readers")
	if report["action"] != "make" || report["twin_version"] != 1.0 {
		t.Fatalf("first release: %v", report)
	}
	twin := w.twin(page)
	got := w.reader.ok("GET", "/api/v1/pages/"+twin, nil)
	if got["title"] != "Onboarding" || got["parent_id"] != w.publishedHome || got["emoji"] != "🚀" || got["full_width"] != true ||
		!strings.Contains(fmt.Sprint(got["document"]), "welcome aboard") || fmt.Sprint(got["labels"]) != "[start-here]" {
		t.Fatalf("the twin as a reader: %v", got)
	}
	versions := w.reader.ok("GET", "/api/v1/pages/"+twin+"/versions", nil)["versions"].([]any)
	first := versions[0].(map[string]any)
	if len(versions) != 1 || first["summary"] != "First edition for readers" || first["author_id"] != w.rel.id {
		t.Fatalf("the twin's history: %v", versions)
	}
	if s := w.status(w.writer, page); s["status"] != "released" || len(s["aspects"].([]any)) != 0 {
		t.Fatalf("after the release: %v", s)
	}

	w.edit(w.writer, page, "Onboarding", para("welcome aboard, second edition"))
	if s := w.status(w.writer, page); s["status"] != "changed" || fmt.Sprint(s["aspects"]) != "[text]" {
		t.Fatalf("after an edit: %v", s)
	}
	if again := w.release(w.rel, page, "Second"); again["action"] != "change" || again["twin_version"] != 2.0 {
		t.Fatalf("second release: %v", again)
	}
	if unchanged := w.release(w.rel, page, "Nothing new"); unchanged["action"] != "none" {
		t.Fatalf("an unchanged release: %v", unchanged)
	}
	if n := len(w.reader.ok("GET", "/api/v1/pages/"+twin+"/versions", nil)["versions"].([]any)); n != 2 {
		t.Fatalf("an unchanged release made a version: %d versions", n)
	}
	if out := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+twin+"/versions", nil); strings.Contains(out, "zanzibar") {
		t.Fatalf("a draft summary reached readers: %s", out)
	}
}

// TestAPageWhoseParentIsNotReleasedGoesUnderItsNearestReleasedAncestor is PUB-400 and PUB-1426,
// and a move of the draft is released as a move of the twin without a version (PUB-411,
// PUB-1000).
func TestAPageWhoseParentIsNotReleasedGoesUnderItsNearestReleasedAncestor(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "PLC")
	chapter := w.page(w.writer, w.draftHome, "Chapter", "c")
	section := w.page(w.writer, chapter, "Section", "s")
	if preview := w.preview(w.admin, section); preview["placed_higher"] != true {
		t.Fatalf("preview: %v", preview)
	}
	w.release(w.admin, section, "")
	if got := w.reader.ok("GET", "/api/v1/pages/"+w.twin(section), nil); got["parent_id"] != w.publishedHome {
		t.Fatalf("placed under %v", got["parent_id"])
	}
	w.release(w.admin, chapter, "")
	w.writer.ok("POST", "/api/v1/pages/"+section+"/move", map[string]any{"parent_id": chapter})
	if got := w.aspects(section); fmt.Sprint(got) != "[]" {
		t.Fatalf("a page moved back under its parent: %v", got)
	}
	other := w.page(w.writer, w.draftHome, "Appendix", "a")
	w.release(w.admin, other, "")
	w.writer.ok("POST", "/api/v1/pages/"+section+"/move", map[string]any{"parent_id": other})
	if got := w.aspects(section); fmt.Sprint(got) != "[place]" {
		t.Fatalf("a moved page: %v", got)
	}
	if report := w.release(w.admin, section, ""); report["action"] != "update" {
		t.Fatalf("releasing a move: %v", report)
	}
	twin := w.twin(section)
	if got := w.reader.ok("GET", "/api/v1/pages/"+twin, nil); got["parent_id"] != w.twin(other) || got["current_version"] != 1.0 {
		t.Fatalf("the moved twin: parent %v, version %v", got["parent_id"], got["current_version"])
	}
}

// TestAReleaseNeedsEachOfItsRights is PUB-413: a page hidden from the releaser is not found,
// space.release is needed in the draft, and view of the published space.
func TestAReleaseNeedsEachOfItsRights(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "RGT")
	page := w.page(w.admin, w.draftHome, "Board minutes", "secret")
	w.admin.restrict(page, "view", w.admin.id)
	w.rel.want(http.StatusNotFound, "POST", "/api/v1/pages/"+page+"/release/preview", nil)
	open := w.page(w.writer, w.draftHome, "Open", "x")
	w.writer.want(http.StatusForbidden, "POST", "/api/v1/pages/"+open+"/release/preview", nil)
	w.admin.ok("PUT", "/api/v1/spaces/"+w.published+"/grants", map[string]any{
		"subject": map[string]any{"type": "user", "id": w.rel.id}, "permissions": []string{}})
	if out := w.rel.want(http.StatusConflict, "POST", "/api/v1/pages/"+open+"/release/preview", nil); !strings.Contains(out, "published_space_hidden") {
		t.Fatalf("a release without view of the published space: %s", out)
	}
	post := w.admin.ok("POST", "/api/v1/pages", map[string]any{"space_id": w.draft, "title": "News", "kind": "post"})["id"].(string)
	w.admin.ok("POST", "/api/v1/pages/"+post+"/publish", map[string]any{"title": "News", "document": para("n"), "base_version": 0})
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+post+"/release/preview", nil); !strings.Contains(out, "cannot_release") {
		t.Fatalf("a blog post was offered for release: %s", out)
	}
	if n := e.count("SELECT count(*) FROM pages WHERE space_id = $1", w.published); n != 1 {
		t.Fatalf("refused releases made %d pages", n-1)
	}
}

// TestOutOfStepFollowsEachAspectAndComesBackWhenUndone is PUB-300 to PUB-304, PUB-310 and
// PUB-342: each aspect marks the page, what never counts does not, and undoing a change
// makes the page released again.
func TestOutOfStepFollowsEachAspectAndComesBackWhenUndone(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "OOS")
	page := w.page(w.writer, w.draftHome, "Handbook", "rules")
	w.release(w.admin, page, "")
	released := w.current(w.writer, page)
	steps := []struct {
		name string
		do   func()
		want string
	}{
		{"a comment", func() { w.both.comment(page, "draft discussion") }, "[]"},
		{"a status", func() {
			w.writer.ok("PUT", "/api/v1/pages/"+page+"/stage", map[string]any{"text": "Needs review", "color": "purple"})
		}, "[]"},
		{"a label", func() { w.writer.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "hr"}) }, "[labels]"},
		{"the label taken away", func() { w.writer.want(http.StatusOK, "DELETE", "/api/v1/pages/"+page+"/labels/hr", nil) }, "[]"},
		{"an emoji", func() {
			w.writer.ok("PATCH", "/api/v1/pages/"+page+"/look", map[string]any{"emoji": "📘", "title_align": "start", "header_position": 50})
		}, "[look]"},
		{"the emoji taken away", func() {
			w.writer.ok("PATCH", "/api/v1/pages/"+page+"/look", map[string]any{"emoji": "", "title_align": "start", "header_position": 50})
		}, "[]"},
		{"new words", func() { w.edit(w.writer, page, "Handbook", para("new rules")) }, "[text]"},
		{"the released version restored", func() {
			w.writer.ok("POST", fmt.Sprintf("/api/v1/pages/%s/versions/%d/restore", page, released), map[string]any{})
		}, "[]"},
		{"a new title", func() { w.edit(w.writer, page, "Staff handbook", para("rules")) }, "[title]"},
	}
	for _, s := range steps {
		s.do()
		if got := w.aspects(page); fmt.Sprint(got) != s.want {
			t.Fatalf("after %s: %v, want %s", s.name, got, s.want)
		}
	}
	// A draft that nobody published yet changes nothing (PUB-311).
	w.writer.saveDraft(page, "Staff handbook", w.current(w.writer, page), "unsaved words")
	if got := w.aspects(page); fmt.Sprint(got) != "[title]" {
		t.Fatalf("an unsaved draft: %v", got)
	}
}

// TestFilesAreCopiedOnlyWhenTheyChangedAndReadersGetTheCopy is PUB-320, PUB-321 and PUB-624.
func TestFilesAreCopiedOnlyWhenTheyChangedAndReadersGetTheCopy(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "FIL")
	page := w.page(w.writer, w.draftHome, "Forms", "x")
	file := w.writer.file(page, "form.txt", "draft form one")
	fileBlock := map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": file, "filename": "form.txt"}}
	w.edit(w.writer, page, "Forms", docOf(paraOf(textOf("the form")), fileBlock))
	if report := w.release(w.admin, page, ""); report["files_copied"] != 1.0 {
		t.Fatalf("first release: %v", report)
	}
	twin := w.twin(page)
	twinDoc := fmt.Sprint(w.reader.ok("GET", "/api/v1/pages/"+twin, nil)["document"])
	if strings.Contains(twinDoc, file) {
		t.Fatalf("the twin shows the draft file: %s", twinDoc)
	}
	w.reader.want(http.StatusNotFound, "GET", "/api/v1/attachments/"+file+"/content", nil)
	copies := w.reader.ok("GET", "/api/v1/pages/"+twin+"/attachments", nil)["attachments"].([]any)
	if len(copies) != 1 {
		t.Fatalf("copies: %v", copies)
	}
	copied := copies[0].(map[string]any)["id"].(string)
	if body := w.reader.want(http.StatusOK, "GET", "/api/v1/attachments/"+copied+"/content", nil); body != "draft form one" {
		t.Fatalf("the copy holds %q", body)
	}
	// Unchanged: not copied again. A new version: copied as the copy's next version.
	w.edit(w.writer, page, "Forms", docOf(paraOf(textOf("the form, again")), fileBlock))
	if report := w.release(w.admin, page, ""); report["files_copied"] != 0.0 {
		t.Fatalf("an unchanged file was copied: %v", report)
	}
	if status, out := w.writer.newVersion(file, "form.txt", "draft form two"); status != http.StatusCreated {
		t.Fatalf("new version: %d %s", status, out)
	}
	if report := w.release(w.admin, page, ""); report["files_copied"] != 1.0 || report["action"] != "update" {
		t.Fatalf("a new version of a file: %v", report)
	}
	if body := w.reader.want(http.StatusOK, "GET", "/api/v1/attachments/"+copied+"/content", nil); body != "draft form two" {
		t.Fatalf("the copy holds %q after a new version", body)
	}
	// Removed from the draft: the copy goes to the published space's trash.
	w.edit(w.writer, page, "Forms", para("no form any more"))
	w.release(w.admin, page, "")
	if copies := w.reader.ok("GET", "/api/v1/pages/"+twin+"/attachments", nil)["attachments"].([]any); len(copies) != 0 {
		t.Fatalf("a removed file stayed: %v", copies)
	}
	// A file that hangs on a comment of the draft never crosses (PUB-1406).
	secret := w.writer.file(page, "comment-only.txt", "comment secret")
	w.writer.comment(page, "see "+secret)
	w.edit(w.writer, page, "Forms", para("words"))
	w.release(w.admin, page, "")
	if out := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+twin+"/attachments", nil); strings.Contains(out, "comment-only") {
		t.Fatalf("a file of a comment crossed: %s", out)
	}
}

// TestARunTakesThePreviewedVersionAndRefusesAnOldPreview is PUB-431, PUB-433 and PUB-1062.
func TestARunTakesThePreviewedVersionAndRefusesAnOldPreview(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "PRV")
	page := w.page(w.writer, w.draftHome, "Rates", "rate one")
	preview := w.preview(w.admin, page)
	w.edit(w.writer, page, "Rates", para("rate two, not reviewed"))
	w.admin.ok("POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": preview["id"], "note": ""})
	if doc := fmt.Sprint(w.reader.ok("GET", "/api/v1/pages/"+w.twin(page), nil)["document"]); !strings.Contains(doc, "rate one") {
		t.Fatalf("the run took a version it did not preview: %s", doc)
	}
	if s := w.status(w.admin, page); s["status"] != "changed" {
		t.Fatalf("a page changed after its preview: %v", s)
	}
	w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": preview["id"]})
	old := w.preview(w.admin, page)
	e.sql("UPDATE release_runs SET previewed_at = now() - interval '61 minutes' WHERE id = $1", old["id"])
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": old["id"]}); !strings.Contains(out, "preview_expired") {
		t.Fatalf("an old preview: %s", out)
	}
	w.rel.want(http.StatusConflict, "POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": w.preview(w.admin, page)["id"]})
	long := strings.Repeat("n", 2001)
	w.admin.want(http.StatusBadRequest, "POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": w.preview(w.admin, page)["id"], "note": long})
}

// TestTwoRunsAtOnceMakeOneVersionAndAPublishGoesOn is PUB-1060 and PUB-1062: two runs of one
// pair take turns on the pair row, so the second finds the page in step; a publish in the
// draft at the same moment makes its own newer version.
func TestTwoRunsAtOnceMakeOneVersionAndAPublishGoesOn(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "CON")
	page := w.page(w.writer, w.draftHome, "Prices", "one")
	first, second := w.preview(w.admin, page), w.preview(w.rel, page)
	var wg sync.WaitGroup
	results := make([]string, 3)
	run := func(i int, u *user, preview map[string]any) {
		defer wg.Done()
		_, out := u.do("POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": preview["id"]})
		results[i] = out
	}
	wg.Add(3)
	go run(0, w.admin, first)
	go run(1, w.rel, second)
	go func() {
		defer wg.Done()
		status, out := w.writer.do("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Prices", "document": para("two"), "base_version": 1})
		results[2] = fmt.Sprint(status, out)
	}()
	wg.Wait()
	actions := []string{}
	for _, out := range results[:2] {
		var r map[string]any
		_ = json.Unmarshal([]byte(out), &r)
		actions = append(actions, fmt.Sprint(r["action"]))
	}
	sort.Strings(actions)
	if fmt.Sprint(actions) != "[make none]" || !strings.HasPrefix(results[2], "200") {
		t.Fatalf("two runs and a publish: %v / %s", actions, results[2])
	}
	if n := e.count("SELECT count(*) FROM pages WHERE space_id = $1", w.published); n != 2 {
		t.Fatalf("two runs made %d twins", n-1)
	}
}

// TestAFailedReleaseLeavesNothing is PUB-450: a run is one transaction, so a run that fails
// half way leaves no twin, no file and no fact behind.
func TestAFailedReleaseLeavesNothing(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "ATM")
	page := w.page(w.writer, w.draftHome, "Twins", "x")
	other := w.page(w.writer, w.draftHome, "Other", "x")
	a := w.writer.file(page, "same.txt", "a")
	b := w.writer.file(other, "same.txt", "b")
	block := func(id string) map[string]any {
		return map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": id, "filename": "same.txt"}}
	}
	w.edit(w.writer, page, "Twins", docOf(block(a), block(b)))
	preview := w.preview(w.admin, page)
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": preview["id"]}); !strings.Contains(out, "file_name_taken") {
		t.Fatalf("a release with two files of one name: %s", out)
	}
	if n := e.count("SELECT count(*) FROM pages WHERE space_id = $1", w.published) +
		e.count("SELECT count(*) FROM attachments WHERE space_id = $1", w.published) +
		e.count("SELECT count(*) FROM page_pairs WHERE draft_page_id = $1", page); n != 1 {
		t.Fatalf("a failed release left %d rows behind", n-1)
	}
}

// TestLinksInAReleasedPageLeadToTwinsAndNeverIntoTheDrafts is PUB-500, PUB-510, PUB-512 and
// PUB-530 on the routes.
func TestLinksInAReleasedPageLeadToTwinsAndNeverIntoTheDrafts(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "LNK")
	target := w.page(w.writer, w.draftHome, "Glossary", "terms")
	hidden := w.page(w.writer, w.draftHome, "Unfinished chapter", "draft only")
	w.release(w.admin, target, "")
	page := w.page(w.writer, w.draftHome, "Guide", "x")
	w.edit(w.writer, page, "Guide", docOf(paraOf(textOf("see "), pageLinkTo(target, "terms"), textOf(" and "), pageLinkTo(hidden, ""),
		textOf(" and "), pageLinkTo(page, "top"))))
	if preview := w.preview(w.admin, page); preview["links_as_text"] != 1.0 {
		t.Fatalf("preview: %v", preview)
	}
	w.release(w.admin, page, "")
	twin := w.twin(page)
	out := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+twin, nil)
	for _, id := range []string{target, hidden, page, w.draft} {
		if strings.Contains(out, id) {
			t.Fatalf("draft id %s in the twin: %s", id, out)
		}
	}
	if !strings.Contains(out, w.twin(target)) || !strings.Contains(out, "Unfinished chapter") || !strings.Contains(out, twin) {
		t.Fatalf("links did not become twins and words: %s", out)
	}
	// Renaming the target changes no reader's link until it is released (PUB-530).
	w.edit(w.writer, target, "Glossary of draft words", para("terms"))
	if out := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+w.twin(target), nil); strings.Contains(out, "Glossary of draft words") {
		t.Fatalf("a rename reached readers before its release")
	}
}

// TestAReaderOfThePublishedSpaceNeverReachesTheDraft is PUB-108 and PUB-620 to PUB-635: every
// door of the published space, rendered for a reader of it, holds no id, key or name of the
// draft space, no draft page and no word that exists only in the draft.
func TestAReaderOfThePublishedSpaceNeverReachesTheDraft(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "LEAK")
	page := w.page(w.writer, w.draftHome, "Safety rules", "wear a helmet")
	w.writer.comment(page, "draft comment quokka")
	w.release(w.admin, page, "Rules for everyone")
	twin := w.twin(page)
	w.reader.ok("PUT", "/api/v1/pages/"+twin+"/watch", map[string]any{})
	w.edit(w.writer, page, "Safety rules", para("wear a helmet, second edition"))
	w.release(w.admin, page, "Second edition")
	w.edit(w.writer, page, "Safety rules", para("wear a helmet and the secret word wombatfjord"))
	secretPage := w.page(w.writer, w.draftHome, "Unreleased wombatfjord plan", "wombatfjord")
	e.deliver()
	e.runJobs()

	doors := []string{
		"/api/v1/pages/" + twin,
		"/api/v1/pages/" + twin + "/versions",
		"/api/v1/pages/" + twin + "/versions/1",
		"/api/v1/pages/" + twin + "/details",
		"/api/v1/pages/" + twin + "/summary",
		"/api/v1/pages/" + twin + "/attachments",
		"/api/v1/pages/" + twin + "/comments",
		"/api/v1/pages/" + twin + "/labels",
		"/api/v1/pages/" + twin + "/export.md",
		"/api/v1/pages/" + twin + "/release",
		"/api/v1/pages/" + twin + "/compare?from=1&to=2",
		"/api/v1/spaces/" + w.published,
		"/api/v1/spaces/" + w.published + "/tree",
		"/api/v1/spaces/" + w.published + "/tree/branches",
		"/api/v1/spaces/" + w.published + "/publishing",
		"/api/v1/search?q=wombatfjord",
		"/api/v1/search?q=helmet",
		"/api/v1/search/quick?q=Safety",
		"/api/v1/feed",
		"/api/v1/me/notifications",
		"/api/v1/spaces",
	}
	forbidden := []string{w.draft, w.draftHome, page, secretPage, strings.ToLower(w.draftKey), w.draftName, "wombatfjord", "quokka", "zanzibar"}
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
	if !strings.Contains(w.reader.want(http.StatusOK, "GET", "/api/v1/me/notifications", nil), twin) {
		t.Errorf("a watcher of the twin was not told of the release")
	}
	// The drafts themselves are not found at all.
	for _, path := range []string{"/api/v1/pages/" + page, "/api/v1/spaces/" + w.draft, "/api/v1/pages/" + page + "/release"} {
		w.reader.want(http.StatusNotFound, "GET", path, nil)
	}
	// A person who reads both sees which page it came from (PUB-603).
	if s := w.status(w.both, twin); s["draft_page_id"] != page || s["side"] != "published" {
		t.Errorf("a person in both: %v", s)
	}
	// A reader of the draft space who does not write sees nothing of releases (PUB-1221).
	if s := w.status(w.both, page); s["side"] != "" || s["status"] != nil {
		t.Errorf("a reader of the draft space: %v", s)
	}
	// A reader is told it was released, and nothing of the draft side.
	if s := w.status(w.reader, twin); s["released_at"] == nil || s["draft_page_id"] != nil || s["draft_space"] != nil {
		t.Errorf("the twin as a reader: %v", s)
	}
}

// TestTheTwinsComeOnlyFromThePublishedSpaceOnTheWeb is PUB-610 and PUB-626: a draft space
// cannot publish to the web.
func TestADraftSpaceCannotGoOnTheWeb(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "WEB")
	if can := w.admin.ok("GET", "/api/v1/spaces/"+w.draft, nil)["can"].(map[string]any); can["site.publish"] != false {
		t.Fatalf("a draft space offers a public site: %v", can)
	}
	page := w.page(w.admin, w.draftHome, "Public?", "x")
	w.admin.want(http.StatusForbidden, "PUT", "/api/v1/pages/"+page+"/public-link", map[string]any{})
	if can := w.admin.ok("GET", "/api/v1/spaces/"+w.published, nil)["can"].(map[string]any); can["site.publish"] != true {
		t.Fatalf("the published space should still be able to go on the web: %v", can)
	}
}

// TestReleasesAreAuditedOnBothSides is PUB-900 to PUB-903, PUB-922 and PUB-923: the draft space
// holds each release with both sides and the content hashes, the published space the twin
// only, and no entry holds page text.
func TestReleasesAreAuditedOnBothSides(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "AUD")
	page := w.page(w.writer, w.draftHome, "Audit me", "text that is not for the audit log")
	w.release(w.admin, page, "Note for readers here")
	twin := w.twin(page)
	draftAudit := w.admin.want(http.StatusOK, "GET", "/api/v1/spaces/"+w.draft+"/audit?limit=200", nil)
	for _, want := range []string{"release.previewed", "release.page", "content_hash", "twin_hash", "Note for readers here", twin} {
		if !strings.Contains(draftAudit, want) {
			t.Errorf("the draft space's audit lacks %s", want)
		}
	}
	pubAudit := w.admin.want(http.StatusOK, "GET", "/api/v1/spaces/"+w.published+"/audit?limit=200", nil)
	if !strings.Contains(pubAudit, "page.released") || !strings.Contains(pubAudit, "space.managed") {
		t.Errorf("the published space's audit lacks its releases: %s", pubAudit)
	}
	for _, word := range []string{w.draft, page, strings.ToLower(w.draftKey), w.draftName} {
		if strings.Contains(strings.ToLower(pubAudit), strings.ToLower(word)) {
			t.Errorf("the published space's audit names the draft side: %q", word)
		}
	}
	all := w.admin.want(http.StatusOK, "GET", "/api/v1/spaces/"+w.draft+"/audit?limit=200", nil) + pubAudit
	if strings.Contains(all, "not for the audit log") {
		t.Errorf("page text reached the audit log")
	}
}

// TestRenamingEitherSpaceChangesNothingOfThePair is PUB-1021 and PUB-1022.
func TestRenamingEitherSpaceChangesNothingOfThePair(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "RNM")
	page := w.page(w.writer, w.draftHome, "Stable", "x")
	w.release(w.admin, page, "")
	w.admin.ok("PATCH", "/api/v1/spaces/"+w.draft, map[string]any{"name": "Renamed drafts"})
	w.admin.ok("PATCH", "/api/v1/spaces/"+w.published, map[string]any{"name": "Renamed readers"})
	if s := w.status(w.admin, page); s["status"] != "released" {
		t.Fatalf("after renames: %v", s)
	}
	if out := w.reader.want(http.StatusOK, "GET", "/api/v1/pages/"+w.twin(page), nil); strings.Contains(out, "Renamed drafts") {
		t.Fatalf("the draft's new name reached a reader")
	}
}

// TestNobodyEditsATwinTogether is PUB-630: a twin has no session for editing together.
func TestNobodyEditsATwinTogether(t *testing.T) {
	c := newCollabTeam(t)
	state := c.editor.ok("POST", "/api/v1/spaces/"+c.space+"/publishing/link", map[string]any{"new_space": map[string]any{"key": "COLREAD", "name": "Readers"}})
	if state["side"] != "draft" {
		t.Fatalf("link: %v", state)
	}
	preview := c.editor.ok("POST", "/api/v1/pages/"+c.page+"/release/preview", nil)
	c.editor.ok("POST", "/api/v1/pages/"+c.page+"/release", map[string]any{"preview_id": preview["id"]})
	twin := c.editor.ok("GET", "/api/v1/pages/"+c.page+"/release", nil)["twin_page_id"].(string)
	if status, reached := c.connect(c.editor.c, twin); reached != nil {
		t.Fatalf("a socket on a twin was passed on: %d", status)
	}
}

// TestReadingATwinCostsWhatAnyPageCosts is PUB-1100 and PUB-1102: a twin is an ordinary page on
// the read path, and publishing a released draft page sends no more statements than publishing
// in an ordinary space (the facts are kept by a trigger in the same statement).
func TestReadingATwinCostsWhatAnyPageCosts(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "QRY")
	page := w.page(w.writer, w.draftHome, "Costs", "the release plan")
	w.release(w.admin, page, "")
	twin := w.twin(page)
	ordinary, home := w.admin.space("QRYPLAIN")
	plain := w.admin.page(ordinary, home, "Costs", "the release plan")
	w.admin.grant(ordinary, "user", w.reader.id, permissions.TemplateViewer)
	if a, b := queriesOf(t, e, w.reader, "/api/v1/pages/"+twin), queriesOf(t, e, w.reader, "/api/v1/pages/"+plain); a != b {
		t.Fatalf("opening a twin costs %d queries, an ordinary page %d", a, b)
	}
	publish := func(u *user, id string) int64 {
		t.Helper()
		lowest := int64(-1)
		for range 3 {
			body := map[string]any{"title": "Costs", "document": para(fmt.Sprint(time.Now().UnixNano())), "base_version": w.current(u, id)}
			before := db.Queries(e.app.Pool())
			u.ok("POST", "/api/v1/pages/"+id+"/publish", body)
			if n := db.Queries(e.app.Pool()) - before; lowest < 0 || n < lowest {
				lowest = n
			}
		}
		return lowest
	}
	w.admin.grant(ordinary, "user", w.writer.id, permissions.TemplateEditor)
	drafted, plainCost := publish(w.writer, page), publish(w.writer, plain)
	if drafted > plainCost {
		t.Fatalf("publishing a released draft page sends %d statements, in an ordinary space %d", drafted, plainCost)
	}
	t.Logf("publish: %d statements in a draft space, %d in an ordinary one", drafted, plainCost)
}

// TestASinglePageReleaseFitsItsBudget measures PUB-1105: a single-page release without files
// within 150 ms at the 95th percentile. On a loaded test machine the bound checked here is
// wider; the measured numbers go in the report.
func TestASinglePageReleaseFitsItsBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("a measurement")
	}
	e := newEnv(t)
	w := newPubWorld(e, "BUD")
	page := w.page(w.writer, w.draftHome, "Measured", "x")
	var runs []time.Duration
	for i := range 30 {
		w.edit(w.writer, page, "Measured", para(fmt.Sprintf("edition %d", i)))
		preview := w.preview(w.admin, page)
		start := time.Now()
		w.admin.ok("POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": preview["id"]})
		runs = append(runs, time.Since(start))
	}
	slices.Sort(runs)
	p50, p95 := runs[len(runs)/2], runs[len(runs)*95/100]
	t.Logf("single-page release over HTTP: p50 %v, p95 %v, max %v", p50, p95, runs[len(runs)-1])
	if p95 > time.Second {
		t.Fatalf("a single-page release took %v at the 95th percentile", p95)
	}
}

// TestLinkingTakesTheSiteAndLinksOfTheDraftSpaceDown is PUB-610: a draft space's public site and
// public links are withdrawn when it is linked, and it cannot make new ones.
func TestLinkingTakesTheSiteAndLinksOfTheDraftSpaceDown(t *testing.T) {
	s := siteSetup(t)
	s.choose(s.home)
	s.publish()
	if !strings.Contains(s.live("welcome.html"), "welcome words") {
		t.Fatal("the site was not built")
	}
	s.owner.ok("PUT", "/api/v1/pages/"+s.home+"/public-link", map[string]any{})
	preview := s.owner.ok("POST", "/api/v1/spaces/"+s.space+"/publishing/link-preview", map[string]any{"new_space": map[string]any{"key": "PUBREAD", "name": "Readers"}})
	if preview["sites"] != 1.0 || preview["links"] != 1.0 {
		t.Fatalf("the preview does not name the site and the link: %v", preview)
	}
	s.owner.ok("POST", "/api/v1/spaces/"+s.space+"/publishing/link", map[string]any{"new_space": map[string]any{"key": "PUBREAD", "name": "Readers"}})
	s.deliver()
	if n := s.count("SELECT count(*) FROM public_sites WHERE space_id = $1 AND withdrawn_at IS NULL AND live_build > 0", s.space); n != 0 {
		t.Fatalf("%d sites of a draft space stayed live", n)
	}
	if n := s.count("SELECT count(*) FROM public_links WHERE page_id = $1 AND withdrawn_at IS NULL", s.home); n != 0 {
		t.Fatalf("%d public links of a draft space stayed", n)
	}
	s.owner.want(http.StatusForbidden, "POST", "/api/v1/spaces/"+s.space+"/sites", map[string]any{"name": "Again"})
}
