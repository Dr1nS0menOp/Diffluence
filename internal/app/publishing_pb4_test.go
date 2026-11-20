// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Draft and published spaces, wave PB4 of spec 073: what PB2 and PB3 left for this wave. The
// draft page's history names the versions that were released (PUB-911, PUB-1223), a twin tells
// someone used to editing why Edit is missing (PUB-1231), and the tree of a draft space lists the
// pages changed since their release (PUB-1224).

// TestADraftPagesHistoryNamesWhatEachReleaseBecame is PUB-911: beside each version that was
// released, the draft page says which version of the published page it became, and when, to
// those who may see the release screen only.
func TestADraftPagesHistoryNamesWhatEachReleaseBecame(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "RELAS")
	page := w.page(w.writer, w.draftHome, "Guide", "first words")
	w.release(w.admin, page, "")
	w.edit(w.writer, page, "Guide", para("second words"))
	w.edit(w.writer, page, "Guide", para("third words"))
	w.release(w.admin, page, "")
	for _, u := range []*user{w.admin, w.rel} {
		got := fmt.Sprint(w.status(u, page)["released_as"])
		if !strings.Contains(got, "draft_version:1") || !strings.Contains(got, "draft_version:3") || strings.Contains(got, "draft_version:2") ||
			!strings.Contains(got, "twin_version:1") || !strings.Contains(got, "twin_version:2") {
			t.Fatalf("%s sees the releases of the page as %s", u.email, got)
		}
	}
	// A writer who may not release sees no release history; nor does anyone on a twin.
	if got := w.status(w.writer, page)["released_as"]; got != nil {
		t.Fatalf("a writer sees the releases: %v", got)
	}
	if got := w.status(w.reader, w.twin(page))["released_as"]; got != nil {
		t.Fatalf("a reader of the twin sees the releases: %v", got)
	}
}

// TestTheTreeOfADraftSpaceListsThePagesChangedSinceTheirRelease is PUB-1224: one more choice of
// the tree lists, flat and the longest waiting first, the released pages changed since, as far as
// the person may see them; elsewhere it lists nothing.
func TestTheTreeOfADraftSpaceListsThePagesChangedSinceTheirRelease(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "TREEF")
	first := w.page(w.writer, w.draftHome, "First changed", "a")
	second := w.page(w.writer, w.draftHome, "Second changed", "b")
	same := w.page(w.writer, w.draftHome, "In step", "c")
	never := w.page(w.writer, w.draftHome, "Never released", "d")
	secret := w.page(w.writer, w.draftHome, "Secret changed", "e")
	for _, p := range []string{first, second, same, secret} {
		w.release(w.admin, p, "")
	}
	w.edit(w.writer, first, "First changed", para("a2"))
	w.edit(w.writer, second, "Second changed", para("b2"))
	w.edit(w.writer, secret, "Secret changed", para("e2"))
	w.admin.restrict(secret, "view", w.admin.id)
	matches := func(u *user, space string) []string {
		t.Helper()
		out := []string{}
		for _, id := range u.ok("GET", "/api/v1/spaces/"+space+"/tree/find?view=changed", nil)["matches"].([]any) {
			out = append(out, id.(string))
		}
		return out
	}
	if got := matches(w.writer, w.draft); len(got) != 2 || got[0] != first || got[1] != second {
		t.Fatalf("the writer's view of what changed: %v (want %s, %s; not %s, %s, %s)", got, first, second, same, never, secret)
	}
	if got := matches(w.admin, w.draft); len(got) != 3 {
		t.Fatalf("the administrator's view of what changed: %v", got)
	}
	if got := matches(w.reader, w.published); len(got) != 0 {
		t.Fatalf("the published space lists changed pages: %v", got)
	}
	// Someone who only reads the draft space does not see the status of its pages (PUB-1221).
	if got := matches(w.both, w.draft); len(got) != 0 {
		t.Fatalf("a reader of the draft space sees what changed: %v", got)
	}
	w.writer.ok("PUT", "/api/v1/me/preferences", map[string]any{"tree_view": "changed"})
}

// TestDeletingASpaceOfAPairTellsTheOtherSidesAdministrators is PUB-1046 and PUB-1047: deleting
// either space ends the pair, and the administrators of the space that stays are told, on its
// home page; the person who deleted it is not told of their own act.
func TestDeletingASpaceOfAPairTellsTheOtherSidesAdministrators(t *testing.T) {
	for _, deleted := range []string{"published", "draft"} {
		t.Run(deleted, func(t *testing.T) {
			e := newEnv(t)
			w := newPubWorld(e, "DEL"+strings.ToUpper(deleted[:3]))
			other, gone, home := w.draft, w.published, w.draftHome
			if deleted == "draft" {
				other, gone, home = w.published, w.draft, w.publishedHome
			}
			keeper := e.user("keeper-"+deleted+"@example.com", identity.PermUseProduct)
			w.admin.grant(other, "user", keeper.id, permissions.TemplateSpaceAdmin)
			// Even someone who asked to hear of their own acts is not told of deleting a space.
			settings := w.admin.ok("GET", "/api/v1/me/notification-settings", nil)
			settings["notify_own"] = true
			w.admin.ok("PUT", "/api/v1/me/notification-settings", settings)
			w.admin.want(http.StatusNoContent, "DELETE", "/api/v1/spaces/"+gone, nil)
			e.deliver()
			if n := e.count("SELECT count(*) FROM space_pairs WHERE ended_at IS NULL"); n != 0 {
				t.Fatalf("deleting the %s space left the pair", deleted)
			}
			told := func(u *user) bool {
				t.Helper()
				out := u.want(http.StatusOK, "GET", "/api/v1/me/notifications", nil)
				return strings.Contains(out, `"type":"pair_ended"`) && strings.Contains(out, home)
			}
			if !told(keeper) {
				t.Fatalf("the administrator of the space that stays was not told")
			}
			if told(w.admin) {
				t.Fatalf("the person who deleted the space was told of it")
			}
			if told(w.reader) || told(w.writer) {
				t.Fatalf("someone who does not run the space was told")
			}
		})
	}
}

// TestAStoppedApprovalReleaseIsResumedFromTheReleaseScreen is PUB-1061 and D27 for a run of
// approval: the release screen shows it stopped, with its reason, and someone with space.release
// resumes it with its version, as theirs; a writer cannot.
func TestAStoppedApprovalReleaseIsResumedFromTheReleaseScreen(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APRES", true)
	page := w.controlled("Procedure", "first words")
	w.approve(page)
	twin := w.twin(page)
	w.publishText(page, "Procedure", "second words")
	w.admin.ok("POST", "/api/v1/spaces/"+w.published+"/archive", map[string]any{})
	w.approve(page)
	w.admin.ok("POST", "/api/v1/spaces/"+w.published+"/unarchive", map[string]any{})
	going, _ := w.admin.ok("GET", "/api/v1/spaces/"+w.draft+"/releases", nil)["going"].(map[string]any)
	if going == nil || going["status"] != "stopped" || going["actor_type"] != "approval" || going["stop_reason"] != "space_not_active" {
		t.Fatalf("the screen does not show the stopped approval release: %v", going)
	}
	run := going["id"].(string)
	w.writer.want(http.StatusForbidden, "POST", "/api/v1/spaces/"+w.draft+"/releases/"+run+"/resume", nil)
	resumed := w.rel.ok("POST", "/api/v1/spaces/"+w.draft+"/releases/"+run+"/resume", nil)
	if resumed["status"] != "queued" || resumed["actor_type"] != "user" {
		t.Fatalf("the resumed run: %v", resumed)
	}
	w.settle()
	if text := w.twinText(twin); !strings.Contains(text, "second words") {
		t.Fatalf("the resumed approval release did not release the approved version: %s", text)
	}
	if got := w.rel.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/"+run, nil); got["status"] != "done" {
		t.Fatalf("the resumed run did not finish: %v", got)
	}
}

// TestABundleOfADraftSpaceCarriesItsLeftOutMarksAndNotThePair is PUB-1042 and PUB-1043: a bundle of
// the draft space carries which pages are left out, with their reasons, and no pair; a bundle of
// the published space makes an ordinary space that its editors can change.
func TestABundleOfADraftSpaceCarriesItsLeftOutMarksAndNotThePair(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "BUNDLE")
	notes := w.page(w.writer, w.draftHome, "Working notes", "n")
	w.page(w.writer, notes, "Below the notes", "b")
	guide := w.page(w.writer, w.draftHome, "Guide", "g")
	w.release(w.admin, guide, "")
	w.leaveOut(notes, true, "internal only")
	root := e.user("bundle-root@example.com", identity.AllGlobalPermissions...)
	carry := func(space, key string) string {
		t.Helper()
		data := bundleOf(t, w.admin, space)
		status, got := root.post("/api/v1/admin/bundles?filename=b.zip", data)
		if status != http.StatusAccepted {
			t.Fatalf("%d %v", status, got)
		}
		e.runJobs()
		id := got["id"].(string)
		root.ok("PUT", "/api/v1/imports/"+id+"/mapping", map[string]any{"space_key": key})
		if done := root.carryOut(id); done["state"] != "done" {
			t.Fatalf("%+v", done)
		}
		return e.scalar("SELECT id::text FROM spaces WHERE key = $1", key)
	}
	copied := carry(w.draft, "BUNDLECOPY")
	mark := e.count(`SELECT count(*) FROM release_exclusions x JOIN pages p ON p.id = x.draft_page_id
		WHERE p.space_id = $1 AND p.title = 'Working notes' AND x.with_children AND x.reason = 'internal only'`, copied)
	if mark != 1 {
		t.Fatalf("the left-out mark did not travel: %d", mark)
	}
	if n := e.count("SELECT count(*) FROM release_exclusions x JOIN pages p ON p.id = x.draft_page_id WHERE p.space_id = $1", copied); n != 1 {
		t.Fatalf("%d marks arrived, want the one", n)
	}
	if role := e.scalar("SELECT COALESCE(pair_role, 'none') FROM spaces WHERE id = $1", copied); role != "none" {
		t.Fatalf("a bundle of the draft space made a space in a pair: %s", role)
	}
	published := carry(w.published, "BUNDLEOUT")
	if role := e.scalar("SELECT COALESCE(pair_role, 'none') FROM spaces WHERE id = $1", published); role != "none" {
		t.Fatalf("a bundle of the published space made a managed space: %s", role)
	}
	page := e.scalar("SELECT id::text FROM pages WHERE space_id = $1 AND title = 'Guide'", published)
	root.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Guide", "document": para("by hand"),
		"base_version": w.current(root, page)})
}

// TestAnExportOfATwinCarriesTheControlBlockOfItsApproval is PUB-731, the part DC4 left (DC4-21):
// a file of a published page of a controlled page says that it is a controlled document, its
// version and when it was approved, and who approved it only when the pair shows approvers; it
// names nothing of the draft space.
func TestAnExportOfATwinCarriesTheControlBlockOfItsApproval(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "TWEXP", true)
	page := w.controlled("Procedure", "approved words")
	w.approve(page)
	twin := w.twin(page)
	exportText := func() string {
		t.Helper()
		run := w.reader.export(map[string]any{"space_id": w.published, "format": "text", "page_ids": []string{twin}, "options": map[string]any{}})
		if run["state"] != "done" {
			t.Fatalf("the export of a twin: %v", run)
		}
		return string(w.reader.exportFile(run["id"].(string)))
	}
	got := exportText()
	if !strings.Contains(got, "Controlled document. State: Approved. Version 1, approved on") || !strings.Contains(got, "approved words") {
		t.Fatalf("the file of a twin has no control block: %s", got)
	}
	approver := "Test " + w.writer.email
	for _, word := range []string{approver, w.draft, strings.ToLower(w.draftKey), w.draftName} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(word)) {
			t.Fatalf("the file of a twin names %q: %s", word, got)
		}
	}
	w.settings(map[string]any{"show_approvers": true})
	if got := exportText(); !strings.Contains(got, "by "+approver) {
		t.Fatalf("the approver is not named once the pair shows approvers: %s", got)
	}
}

// TestATwinTellsWouldBeEditorsWhyEditIsMissing is PUB-1231: someone who holds page.edit in the
// published space learns that releases change the page, with the draft space's name only when
// they may view it; a reader learns nothing of it.
func TestATwinTellsWouldBeEditorsWhyEditIsMissing(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "NOTE")
	page := w.page(w.writer, w.draftHome, "Guide", "first words")
	w.release(w.admin, page, "")
	twin := w.twin(page)
	editor := e.user("note-editor@example.com", identity.PermUseProduct)
	w.admin.grant(w.published, "user", editor.id, permissions.TemplateEditor)
	w.admin.grant(w.published, "user", w.both.id, permissions.TemplateEditor)
	if got := w.status(editor, twin); got["would_edit"] != true || got["draft_space"] != nil {
		t.Fatalf("an editor of the published space only: %v", got)
	}
	if got := w.status(w.both, twin); got["would_edit"] != true || got["draft_space"] == nil {
		t.Fatalf("an editor who views the draft space: %v", got)
	}
	if got := w.status(w.reader, twin); got["would_edit"] == true {
		t.Fatalf("a reader is told of editing: %v", got)
	}
	if out := editor.want(http.StatusOK, "GET", "/api/v1/pages/"+twin+"/release", nil); strings.Contains(out, w.draft) ||
		strings.Contains(strings.ToLower(out), strings.ToLower(w.draftKey)) {
		t.Fatalf("the note names the draft space to someone who may not view it: %s", out)
	}
}
