// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// Draft and published spaces, wave PB3 of spec 073: approval releases. Document control runs in
// the draft space; a state marked "release when approved" releases its pages when they enter
// it, after the approval has committed, and withdrawal takes a twin off. The failure paths come
// first: an approval whose release fails, a version never approved, withdrawal, and the leak
// test of PB1 for approval releases.

// approvalWorld is a pair whose draft space has the review flow of DC1, with the approved state
// marked or not. The writer of the draft takes the steps of the workflow, so the writer is the
// approver; the writer holds no space.release.
type approvalWorld struct {
	pubWorld
	workflow string
}

// markedFlow is the review flow of DC1 with its approved state marked "release when approved".
func markedFlow(marked bool) map[string]any {
	d := reviewFlow()
	if marked {
		d["states"].([]any)[2].(map[string]any)["release_when_approved"] = true
	}
	return d
}

func newApprovalWorld(e *env, key string, marked bool) approvalWorld {
	e.t.Helper()
	w := approvalWorld{pubWorld: newPubWorld(e, key)}
	w.workflow = w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/workflows", map[string]any{"name": "Review", "definition": markedFlow(marked)})["id"].(string)
	return w
}

// controlled makes a published draft page that follows the workflow.
func (w approvalWorld) controlled(title, text string) string {
	w.e.t.Helper()
	page := w.page(w.writer, w.draftHome, title, text)
	w.writer.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflow})
	return page
}

func (w approvalWorld) head(page string) int {
	w.e.t.Helper()
	c := w.writer.ok("GET", "/api/v1/pages/"+page, nil)["control"].(map[string]any)
	return int(c["head_version"].(float64))
}

func (w approvalWorld) step(page, transition string) {
	w.e.t.Helper()
	w.writer.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": transition, "version": w.head(page)})
}

// approve sends a page through review and approves it, then lets the worker do what follows.
func (w approvalWorld) approve(page string) {
	w.e.t.Helper()
	w.step(page, "submit")
	w.step(page, "approve")
	w.settle()
}

// settle hands the events to their listeners, runs the queued work, and hands on the events
// that work made.
func (w approvalWorld) settle() {
	w.e.t.Helper()
	w.e.deliver()
	w.e.runJobs()
	w.e.deliver()
}

// publishText publishes new words on the head of a controlled page.
func (w approvalWorld) publishText(page, title, text string) {
	w.e.t.Helper()
	w.writer.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": title, "document": para(text), "base_version": w.head(page)})
}

// twinText is the text of the version readers have of a twin.
func (w approvalWorld) twinText(twin string) string {
	w.e.t.Helper()
	return w.e.scalar(`SELECT v.document::text FROM pages p JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version WHERE p.id = $1`, twin)
}

// approvalRuns counts the runs approval made for a page, by status.
func (w approvalWorld) approvalRuns(page, status string) int {
	w.e.t.Helper()
	return w.e.count(`SELECT count(*) FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id
		WHERE r.actor_type = 'approval' AND rp.draft_page_id = $1 AND r.status = $2`, page, status)
}

// audited counts audit events, sealed in the chain or still waiting for the sealer, that match a
// condition on action, actor_type, actor_id, object_id and payload.
func (e *env) audited(where string, args ...any) int {
	e.t.Helper()
	return e.count(`SELECT count(*) FROM (SELECT action, actor_type, actor_id, object_id, payload FROM audit_events
		UNION ALL SELECT action, actor_type, actor_id, object_id, payload FROM audit_pending) a WHERE `+where, args...)
}

func (w approvalWorld) settings(change map[string]any) {
	w.e.t.Helper()
	body := map[string]any{"notify_watchers": true, "mirror_removals": true, "default_note": ""}
	for k, v := range change {
		body[k] = v
	}
	w.admin.ok("PUT", "/api/v1/spaces/"+w.draft+"/publishing/settings", body)
}

// TestAFailingApprovalReleaseLeavesThePageApproved is PUB-711 on its failure path: the release
// runs after the approval committed, so a release that cannot go leaves the page approved, Changed
// since release, with the reason on the page, and the approver and author are told.
func TestAFailingApprovalReleaseLeavesThePageApproved(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APFAIL", true)
	page := w.controlled("Procedure", "first words")
	w.approve(page)
	twin := w.twin(page)
	if !strings.Contains(w.twinText(twin), "first words") {
		t.Fatalf("the first approval did not release: %s", w.twinText(twin))
	}
	w.step(page, "withdraw") // a new round starts from a new version below
	w.settle()
	page2 := w.controlled("Second procedure", "second words")
	w.approve(page2)
	twin2 := w.twin(page2)
	w.publishText(page2, "Second procedure", "second words, changed")
	// The published space cannot take a release now.
	w.admin.ok("POST", "/api/v1/spaces/"+w.published+"/archive", map[string]any{})
	w.approve(page2)
	if state := w.writer.ok("GET", "/api/v1/pages/"+page2, nil)["control"].(map[string]any)["state_kind"]; state != "approved" {
		t.Fatalf("a failing release undid the approval: %v", state)
	}
	if n := w.approvalRuns(page2, "stopped"); n != 1 {
		t.Fatalf("%d stopped approval runs", n)
	}
	w.admin.ok("POST", "/api/v1/spaces/"+w.published+"/unarchive", map[string]any{})
	if text := w.twinText(twin2); !strings.Contains(text, "second words") || strings.Contains(text, "changed") {
		t.Fatalf("the twin changed although its release failed: %s", text)
	}
	s := w.status(w.admin, page2)
	if s["status"] != "changed" {
		t.Fatalf("the status after a failed approval release: %v", s)
	}
	failed, _ := s["approval_release"].(map[string]any)
	if failed == nil || failed["outcome"] != "stopped" || failed["reason"] != "space_not_active" {
		t.Fatalf("the page does not say why its approval release did not go: %v", s)
	}
	// The approver hears it did not go.
	found := false
	for _, n := range w.writer.notifications() {
		if n["type"] == "approval_release" && n["page_id"] == page2 {
			data := n["data"].(map[string]any)
			if data["outcome"] == "stopped" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the approver was not told that the release did not go")
	}
	// A release by hand of the approved version still goes.
	w.release(w.admin, page2, "after the failure")
	if text := w.twinText(twin2); !strings.Contains(text, "changed") {
		t.Fatalf("a release by hand after a failed approval release: %s", text)
	}
}

// TestANeverApprovedVersionIsNeverReleased is PUB-700 and PUB-701: a controlled page never
// approved cannot be released by hand and is skipped by every run with its reason; a page in
// review gives readers the approved version, by hand and by a run, never the version in review.
func TestANeverApprovedVersionIsNeverReleased(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APNEVER", false)
	never := w.controlled("Never approved", "unapproved words wallabyquill")
	w.step(never, "submit")
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+never+"/release/preview", nil); !strings.Contains(out, "approved") {
		t.Fatalf("a page never approved: %s", out)
	}
	if s := w.status(w.admin, never); s["status"] != "cannot_release" || s["reason"] != "not_approved" {
		t.Fatalf("the status of a page never approved: %v", s)
	}
	approved := w.controlled("Approved", "approved words")
	w.approve(approved)
	w.publishText(approved, "Approved", "words in review numbatcliff")
	w.step(approved, "submit")
	p := w.preview(w.admin, approved)
	if int(p["version"].(float64)) != 1 {
		t.Fatalf("a release by hand takes version %v of a page in review", p["version"])
	}
	run := w.wholeSpace("Edition")
	if got := outcomes(run)[never]; got != "skipped:not_approved" {
		t.Fatalf("a whole-space run did %q with a page never approved", got)
	}
	twin := w.twin(approved)
	if text := w.twinText(twin); strings.Contains(text, "numbatcliff") || !strings.Contains(text, "approved words") {
		t.Fatalf("readers got the version in review: %s", text)
	}
	if n := e.count("SELECT count(*) FROM page_versions v JOIN pages p ON p.id = v.page_id WHERE p.space_id = $1 AND v.document::text LIKE '%wallabyquill%'", w.published); n != 0 {
		t.Fatal("a version never approved reached the published space")
	}
}

// TestWithdrawalTakesTheTwinOffAtOnce is PUB-720: withdrawing a controlled page takes its twin
// off the published space, by the system with the withdrawer as the cause, whether the workflow
// releases on approval or not, and the twin goes to the trash, never purged.
func TestWithdrawalTakesTheTwinOffAtOnce(t *testing.T) {
	e := newEnv(t)
	for _, marked := range []bool{true, false} {
		key := "APWDRAWN"
		if marked {
			key = "APWDRAWM"
		}
		w := newApprovalWorld(e, key, marked)
		page := w.controlled("Old procedure", "old steps")
		w.step(page, "submit")
		w.step(page, "approve")
		w.settle()
		if !marked {
			w.release(w.admin, page, "")
		}
		twin := w.twin(page)
		w.step(page, "withdraw")
		w.settle()
		if status := e.scalar("SELECT status FROM pages WHERE id = $1", twin); status != "trashed" {
			t.Fatalf("marked %v: a withdrawn page left its twin %s", marked, status)
		}
		if status, _ := w.reader.do("GET", "/api/v1/pages/"+twin, nil); status != http.StatusNotFound {
			t.Fatalf("marked %v: a reader still reads a withdrawn procedure: %d", marked, status)
		}
		if n := e.count(`SELECT count(*) FROM release_runs WHERE actor_type = 'workflow' AND caused_by = $1 AND scope = 'take_off' AND status = 'done'`, w.writer.id); n != 1 {
			t.Fatalf("marked %v: %d take-off runs by the workflow", marked, n)
		}
		if n := e.audited(`action = 'release.run' AND actor_type = 'system' AND actor_id = 'workflow'
			AND payload::jsonb->'details'->>'caused_by' = $1`, w.writer.id); n != 1 {
			t.Fatalf("marked %v: the take-off is not audited as the workflow's, caused by the withdrawer", marked)
		}
	}
}

// TestAReaderOfThePublishedSpaceNeverReachesTheDraftAfterAnApprovalRelease is the leak test of
// PB1 (PUB-108, PUB-620 to PUB-635) for approval releases: every door of the published space,
// rendered for a reader of it, holds no id, key or name of the draft space, no word only the
// draft has, and the approver's name only when the pair shows approvers (PUB-730).
func TestAReaderOfThePublishedSpaceNeverReachesTheDraftAfterAnApprovalRelease(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APLEAK", true)
	w.reader.ok("PUT", "/api/v1/spaces/"+w.published+"/watch", map[string]any{})
	page := w.controlled("Safety rules", "wear gloves")
	w.writer.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Safety rules",
		"document": docOf(paraOf(textOf("wear gloves, see "), pageLinkTo(w.draftHome, ""))), "base_version": w.head(page), "summary": "draft summary zanzibar"})
	w.writer.comment(page, "draft comment zanzibarion")
	w.approve(page)
	w.publishText(page, "Safety rules", "the draft-only word platypusmere")
	twin := w.twin(page)
	doors := []string{
		"/api/v1/spaces/" + w.published,
		"/api/v1/spaces/" + w.published + "/tree",
		"/api/v1/spaces/" + w.published + "/publishing",
		"/api/v1/search?q=gloves",
		"/api/v1/feed",
		"/api/v1/feed?tab=recent",
		"/api/v1/me/notifications",
		"/api/v1/pages/" + twin,
		"/api/v1/pages/" + twin + "/versions",
		"/api/v1/pages/" + twin + "/release",
		"/api/v1/pages/" + twin + "/export.md",
		"/api/v1/pages/" + twin + "/details",
	}
	approverName := "Test " + w.writer.email
	forbidden := []string{w.draft, w.draftHome, page, strings.ToLower(w.draftKey), w.draftName, "platypusmere", "zanzibar",
		approverName, w.writer.id, w.workflow, "In review"}
	check := func(allowed string) {
		t.Helper()
		for _, path := range doors {
			status, out := w.reader.do("GET", path, nil)
			if status != http.StatusOK {
				t.Errorf("%s: %d %s", path, status, out)
				continue
			}
			lower := strings.ToLower(out)
			for _, word := range forbidden {
				if word != allowed && strings.Contains(lower, strings.ToLower(word)) {
					t.Errorf("%s shows %q to a reader of the published space: %s", path, word, out)
				}
			}
		}
	}
	check("")
	// Readers see when it was approved and which version they read (PUB-730).
	approval, _ := w.status(w.reader, twin)["approval"].(map[string]any)
	if approval == nil || approval["approved_at"] == nil || approval["version"] != 1.0 || approval["approved_by"] != nil {
		t.Fatalf("the approved-on line of a twin: %v", approval)
	}
	w.settings(map[string]any{"show_approvers": true})
	approval, _ = w.status(w.reader, twin)["approval"].(map[string]any)
	if approval == nil || approval["approved_by"] != approverName {
		t.Fatalf("the approver is not shown although the pair shows approvers: %v", approval)
	}
	check(approverName)
}

// TestApprovalReleasesTheApprovedVersionThroughTheReleasePath is PUB-702, PUB-711, PUB-712,
// PUB-721 and PUB-902: approval makes a run whose actor is approval and whose cause is the
// approver, with the release path's rights, not the approver's; a version in review is Waiting
// for approval and counted on its tile; a second approval makes a second twin version; returning
// to the approved text releases nothing.
func TestApprovalReleasesTheApprovedVersionThroughTheReleasePath(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APRUN", true)
	if status, _ := w.writer.do("GET", "/api/v1/spaces/"+w.draft+"/releases", nil); status == http.StatusOK {
		t.Fatal("the writer holds space.release, so the test proves nothing")
	}
	page := w.controlled("Handbook", "version one")
	w.approve(page)
	twin := w.twin(page)
	if n := e.count(`SELECT count(*) FROM release_runs WHERE actor_type = 'approval' AND actor_id IS NULL AND caused_by = $1
		AND status = 'done' AND (options->>'round_id') IS NOT NULL`, w.writer.id); n != 1 {
		t.Fatalf("%d approval runs caused by the approver", n)
	}
	if n := e.audited(`action = 'release.page' AND actor_type = 'system' AND actor_id = 'approval'
		AND payload::jsonb->'details'->>'caused_by' = $1`, w.writer.id); n != 1 {
		t.Fatal("the release is not audited as approval's, caused by the approver")
	}
	history := w.rel.ok("GET", "/api/v1/spaces/"+w.draft+"/releases/history", nil)
	runs, _ := history["runs"].([]any)
	if len(runs) == 0 || runs[0].(map[string]any)["actor_type"] != "approval" || runs[0].(map[string]any)["caused_by_name"] != "Test "+w.writer.email {
		t.Fatalf("the history does not show the approval release: %v", history)
	}
	// A newer version in review: nothing a release would change, so Waiting for approval.
	w.publishText(page, "Handbook", "version two")
	w.step(page, "submit")
	if s := w.status(w.admin, page); s["status"] != "waiting_approval" {
		t.Fatalf("a released page with a version in review: %v", s)
	}
	screen := w.rel.ok("GET", "/api/v1/spaces/"+w.draft+"/releases", nil)
	if tiles := screen["tiles"].(map[string]any); tiles["waiting_approval"] != 1.0 || tiles["changed"] != 0.0 {
		t.Fatalf("the tiles: %v", tiles)
	}
	w.step(page, "approve")
	w.settle()
	if text := w.twinText(twin); !strings.Contains(text, "version two") {
		t.Fatalf("a second approval did not release: %s", text)
	}
	// The released version before stays in the twin's history, as any older version (PUB-721).
	if n := e.count("SELECT count(*) FROM page_versions WHERE page_id = $1", twin); n != 2 {
		t.Fatalf("the twin has %d versions", n)
	}
	// Back to the approved text without review: nothing differs, so nothing is released (PUB-712).
	before := e.count("SELECT count(*) FROM release_runs WHERE actor_type = 'approval'")
	w.publishText(page, "Handbook", "version three")
	w.publishText(page, "Handbook", "version two")
	w.settle()
	if s := w.writer.ok("GET", "/api/v1/pages/"+page, nil)["control"].(map[string]any); s["state_kind"] != "approved" {
		t.Fatalf("not back to approved: %v", s)
	}
	if after := e.count("SELECT count(*) FROM release_runs WHERE actor_type = 'approval'"); after != before {
		t.Fatalf("returning to the approved text made %d approval runs", after-before)
	}
	if n := e.count("SELECT count(*) FROM page_versions WHERE page_id = $1", twin); n != 2 {
		t.Fatalf("returning to the approved text gave the twin %d versions", n)
	}
	// Handing the same event over again releases nothing twice.
	e.sql("UPDATE outbox_events SET published_at = NULL WHERE type IN ('page.approved', 'page.version_approved')")
	w.settle()
	if after := e.count("SELECT count(*) FROM release_runs WHERE actor_type = 'approval'"); after != before {
		t.Fatal("an event handed over twice released twice")
	}
}

// TestAReleaseOfTheApprovedVersionCopiesTheFileItPinned is PUB-700 with the files of DC3: a
// new version of a file uploaded for a change in review never reaches readers by a release of
// the approved version; they get the file as the approved version pinned it.
func TestAReleaseOfTheApprovedVersionCopiesTheFileItPinned(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APPIN", false)
	page := w.controlled("Forms", "x")
	file := w.writer.file(page, "form.txt", "approved form")
	w.writer.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Forms",
		"document":     docOf(paraOf(textOf("the form")), map[string]any{"type": "file", "attrs": map[string]any{"attachment_id": file, "filename": "form.txt"}}),
		"base_version": w.head(page)})
	w.step(page, "submit")
	w.step(page, "approve")
	if status, out := w.writer.newVersion(file, "form.txt", "unapproved form"); status != http.StatusCreated {
		t.Fatalf("new version: %d %s", status, out)
	}
	w.release(w.admin, page, "")
	twin := w.twin(page)
	copies := w.reader.ok("GET", "/api/v1/pages/"+twin+"/attachments", nil)["attachments"].([]any)
	if len(copies) != 1 {
		t.Fatalf("copies: %v", copies)
	}
	copied := copies[0].(map[string]any)["id"].(string)
	if body := w.reader.want(http.StatusOK, "GET", "/api/v1/attachments/"+copied+"/content", nil); body != "approved form" {
		t.Fatalf("readers got %q from a release of the approved version", body)
	}
}

// TestAnApprovalReleaseLeavesARestrictedPageToAPerson is PUB-422 for approval: a draft page
// with a view restriction is not released by the system, which sees no one's restrictions; the
// approver is told why, and a person who sees it releases it by naming it.
func TestAnApprovalReleaseLeavesARestrictedPageToAPerson(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APRESTR", true)
	page := w.controlled("Salary rules", "restricted words")
	w.admin.restrict(page, "view", w.admin.id, w.writer.id)
	w.approve(page)
	if twin := w.twinOrEmpty(page); twin != "" {
		t.Fatal("approval released a page restricted in the draft")
	}
	failed, _ := w.status(w.admin, page)["approval_release"].(map[string]any)
	if failed == nil || failed["outcome"] != "skipped" || failed["reason"] != "restricted" {
		t.Fatalf("the page does not say why approval left it: %v", failed)
	}
	w.release(w.admin, page, "named by a person")
	w.twin(page)
}

// TestAReviewThatPassesReleasesAndTellsTheApproverAndTheAuthors is PUB-711 and PUB-820 for a
// state with reviewers: the reviewer whose answer passed the round is the cause, and the
// reviewer and the author are told that the page was released.
func TestAReviewThatPassesReleasesAndTellsTheApproverAndTheAuthors(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APREV", false)
	def := reviewedDef(review("all", 0, entry("user", w.both.id)))
	def["states"].([]any)[2].(map[string]any)["release_when_approved"] = true
	reviewed := w.admin.ok("POST", "/api/v1/spaces/"+w.draft+"/workflows", map[string]any{"name": "Reviewed", "definition": def})["id"].(string)
	page := w.page(w.writer, w.draftHome, "Reviewed procedure", "reviewed words")
	w.writer.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": reviewed})
	w.step(page, "submit")
	w.both.ok("POST", "/api/v1/pages/"+page+"/review/answer", map[string]any{"answer": "approve", "comment": "fine", "version": w.head(page)})
	w.settle()
	w.twin(page)
	if n := e.count(`SELECT count(*) FROM release_runs WHERE actor_type = 'approval' AND caused_by = $1 AND status = 'done'`, w.both.id); n != 1 {
		t.Fatal("the reviewer whose answer passed the round is not the cause")
	}
	for _, u := range []*user{w.both, w.writer} {
		told := false
		for _, n := range u.notifications() {
			if n["type"] == "approval_release" && n["page_id"] == page && n["data"].(map[string]any)["outcome"] == "released" {
				told = true
			}
		}
		if !told {
			t.Errorf("%s was not told that the approved page was released", u.email)
		}
	}
	for _, n := range w.reader.notifications() {
		if n["type"] == "approval_release" {
			t.Fatal("a reader of the published space heard of the draft page")
		}
	}
}

// TestTheMarkIsSetByWhoeverRunsTheSpaceAndIgnoredOutsideAPair is PUB-710: the mark needs
// space.workflows and space.administer, its change is a new version of the workflow and is
// audited, and in a space that is not a draft space it does nothing.
func TestTheMarkIsSetByWhoeverRunsTheSpaceAndIgnoredOutsideAPair(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APMARK", false)
	keeper := e.user("apmark-keeper@example.com", identity.PermUseProduct)
	w.admin.ok("PUT", "/api/v1/spaces/"+w.draft+"/grants", map[string]any{
		"subject":     map[string]any{"type": "user", "id": keeper.id},
		"permissions": []string{permissions.SpaceView, permissions.SpaceWorkflows}})
	wf := w.admin.ok("GET", "/api/v1/workflows/"+w.workflow, nil)
	save := func(u *user, marked bool) (int, string) {
		current := w.admin.ok("GET", "/api/v1/workflows/"+w.workflow, nil)
		return u.do("PUT", "/api/v1/workflows/"+w.workflow, map[string]any{"name": wf["name"], "description": "",
			"definition": markedFlow(marked), "base_version": current["version"]})
	}
	if status, out := save(keeper, true); status != http.StatusForbidden {
		t.Fatalf("someone who keeps workflows but does not run the space set the mark: %d %s", status, out)
	}
	if status, out := save(keeper, false); status != http.StatusOK {
		t.Fatalf("a change without the mark: %d %s", status, out)
	}
	if status, out := save(w.admin, true); status != http.StatusOK {
		t.Fatalf("the administrator set the mark: %d %s", status, out)
	}
	if n := e.count(`SELECT count(*) FROM workflow_versions WHERE workflow_id = $1 AND definition::text LIKE '%release_when_approved%'`, w.workflow); n != 1 {
		t.Fatalf("%d versions hold the mark", n)
	}
	if n := e.audited(`action = 'workflow.saved' AND object_id = $1`, w.workflow); n != 2 {
		t.Fatalf("%d saves audited", n)
	}
	// A space in no pair: the mark is kept and does nothing.
	cw := newControlWorld(e, "APLONE")
	lone := cw.admin.ok("POST", "/api/v1/spaces/"+cw.space+"/workflows", map[string]any{"name": "Marked", "definition": markedFlow(true)})["id"].(string)
	page := cw.author.page(cw.space, cw.home, "Alone", "words")
	cw.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": lone})
	cw.approve(page)
	e.deliver()
	e.runJobs()
	if n := e.count("SELECT count(*) FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id WHERE rp.draft_page_id = $1", page); n != 0 {
		t.Fatalf("a space in no pair released on approval: %d runs", n)
	}
}

// TestAWholeSpaceReleaseOfAControlledSpaceTakesApprovedVersions is PUB-713: a whole-space release
// at a product date takes each controlled page at its approved version, and lists those never
// approved as skipped.
func TestAWholeSpaceReleaseOfAControlledSpaceTakesApprovedVersions(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APSPACE", false)
	w.admin.ok("PUT", "/api/v1/spaces/"+w.draft+"/workflow", map[string]any{"workflow_id": w.workflow, "kinds": []string{"page"}})
	// The space workflow governs every new page.
	a := w.page(w.writer, w.draftHome, "Chapter A", "a approved")
	b := w.page(w.writer, w.draftHome, "Chapter B", "b approved")
	c := w.page(w.writer, w.draftHome, "Chapter C", "c never approved")
	for _, p := range []string{a, b} {
		w.step(p, "submit")
		w.step(p, "approve")
	}
	w.publishText(b, "Chapter B", "b in review")
	w.step(b, "submit")
	run := w.wholeSpace("Product date")
	got := outcomes(run)
	if got[a] != "released:" || got[b] != "released:" || got[c] != "skipped:not_approved" {
		t.Fatalf("the run: %v", got)
	}
	if text := w.twinText(w.twin(b)); !strings.Contains(text, "b approved") {
		t.Fatalf("a page in review went at its version in review: %s", text)
	}
}

// TestOnlyApprovalReleasesControlledPages is PUB-714: with "only approval releases", controlled
// pages are refused by hand and skipped by runs, ordinary pages go as usual, and taking off stays.
func TestOnlyApprovalReleasesControlledPages(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APONLY", false)
	controlled := w.controlled("Controlled", "approved words")
	w.step(controlled, "submit")
	w.step(controlled, "approve")
	w.release(w.admin, controlled, "before the setting")
	w.publishText(controlled, "Controlled", "approved again")
	w.step(controlled, "submit")
	w.step(controlled, "approve")
	ordinary := w.page(w.writer, w.draftHome, "Ordinary", "plain words")
	w.settings(map[string]any{"only_approval_releases": true})
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/pages/"+controlled+"/release/preview", nil); !strings.Contains(out, "approval_only") {
		t.Fatalf("a controlled page released by hand: %s", out)
	}
	w.release(w.admin, ordinary, "")
	w.twin(ordinary)
	w.edit(w.writer, ordinary, "Ordinary", para("plain words again"))
	run := w.wholeSpace("Everything")
	got := outcomes(run)
	if got[controlled] != "skipped:approval_only" || got[ordinary] != "released:" {
		t.Fatalf("a run under only approval releases: %v", got)
	}
	twin := w.twin(controlled)
	w.runAll(w.admin, runBody{"scope": "take_off", "page_ids": []string{controlled}}, "")
	if status := e.scalar("SELECT status FROM pages WHERE id = $1", twin); status != "trashed" {
		t.Fatalf("taking off by hand under only approval releases: %s", status)
	}
}

// TestWorkflowsAreRefusedInThePublishedSpace is PUB-703: control happens in the draft space,
// and the published space takes no workflow of its own, on the space or on a page.
func TestWorkflowsAreRefusedInThePublishedSpace(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APPUBWF", false)
	page := w.page(w.writer, w.draftHome, "Page", "words")
	w.release(w.admin, page, "")
	twin := w.twin(page)
	if out := w.admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+w.published+"/workflows",
		map[string]any{"name": "Review", "definition": reviewFlow()}); !strings.Contains(out, "managed_space") {
		t.Fatalf("a workflow made in the published space: %s", out)
	}
	if status, out := w.admin.do("PUT", "/api/v1/pages/"+twin+"/workflow", map[string]any{"workflow_id": w.workflow}); status < 400 {
		t.Fatalf("a workflow set on a twin: %d %s", status, out)
	}
}

// TestASpaceThatFollowsAWorkflowIsNotLinkedAsThePublishedSpace is PUB-703 at linking: a space
// that follows a workflow, on the space or on a page, becomes no published space, also when the
// same two spaces are linked again; once its workflows are off it links.
func TestASpaceThatFollowsAWorkflowIsNotLinkedAsThePublishedSpace(t *testing.T) {
	e := newEnv(t)
	admin := e.user("plw-admin@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	draft, _ := admin.space("PLWDRAFT")
	target, targetHome := admin.space("PLWREAD")
	flow := admin.ok("POST", "/api/v1/spaces/"+target+"/workflows", map[string]any{"name": "Review", "definition": reviewFlow()})["id"].(string)
	link := map[string]any{"published_space_id": target}
	refused := func(why string) {
		t.Helper()
		for _, path := range []string{"/publishing/link-preview", "/publishing/link"} {
			if out := admin.want(http.StatusConflict, "POST", "/api/v1/spaces/"+draft+path, link); !strings.Contains(out, "published_space_controlled") {
				t.Fatalf("%s: %s linked: %s", why, path, out)
			}
		}
		if n := e.count("SELECT count(*) FROM space_pairs WHERE ended_at IS NULL"); n != 0 {
			t.Fatalf("%s: a refused link left a pair", why)
		}
	}
	admin.ok("PUT", "/api/v1/spaces/"+target+"/workflow", map[string]any{"workflow_id": flow, "kinds": []string{"page"},
		"exempt_labels": []string{"working-notes"}})
	refused("a space workflow")
	admin.ok("PUT", "/api/v1/spaces/"+target+"/workflow", map[string]any{"workflow_id": nil})
	admin.ok("PUT", "/api/v1/pages/"+targetHome+"/workflow", map[string]any{"workflow_id": flow})
	refused("a page workflow")
	admin.ok("PUT", "/api/v1/pages/"+targetHome+"/workflow", map[string]any{"workflow_id": nil})
	admin.ok("POST", "/api/v1/spaces/"+draft+"/publishing/link", link)

	// Ended, given a workflow, and linked again.
	admin.want(http.StatusNoContent, "POST", "/api/v1/spaces/"+target+"/publishing/end", nil)
	admin.ok("PUT", "/api/v1/spaces/"+target+"/workflow", map[string]any{"workflow_id": flow, "kinds": []string{"page"}})
	refused("linked again")
}

// TestAReleaseCopiesLabelsUnderTheRuleForExemptLabels is DOC-402 with PUB-304: a release puts
// labels on a twin and takes them off under the same rule for exempt labels as a person's
// change, in the twin's space. A published space that follows a workflow, as one linked
// before the check at linking could, exempts a label that no release may then set or remove.
func TestAReleaseCopiesLabelsUnderTheRuleForExemptLabels(t *testing.T) {
	e := newEnv(t)
	w := newPubWorld(e, "RXL")
	other, _ := w.admin.space("RXLOTHER")
	flow := w.admin.ok("POST", "/api/v1/spaces/"+other+"/workflows", map[string]any{"name": "Review", "definition": reviewFlow()})["id"].(string)
	page := w.page(w.writer, w.draftHome, "Guide", "words")
	w.writer.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "working-notes"})
	w.release(w.admin, page, "")
	twin := w.twin(page)
	e.sql("UPDATE workflows SET space_id = $1 WHERE id = $2", w.published, flow)
	e.sql(`INSERT INTO space_workflows (space_id, workflow_id, kinds, set_by, exempt_labels) VALUES ($1, $2, '{page}', $3, '{working-notes}')`,
		w.published, flow, w.admin.id)
	labelsOf := func() string {
		t.Helper()
		return e.scalar("SELECT COALESCE(string_agg(label, ',' ORDER BY label), '') FROM page_labels WHERE page_id = $1", twin)
	}

	w.writer.ok("DELETE", "/api/v1/pages/"+page+"/labels/working-notes", nil)
	preview := w.preview(w.admin, page)
	if code, out := w.admin.do("POST", "/api/v1/pages/"+page+"/release", map[string]any{"preview_id": preview["id"]}); code != http.StatusForbidden {
		t.Fatalf("a release took an exempt label off: %d %s", code, out)
	}
	if got := labelsOf(); got != "working-notes" {
		t.Fatalf("the twin's labels after a refused release: %q", got)
	}
	w.writer.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "working-notes"})
	w.writer.ok("POST", "/api/v1/pages/"+page+"/labels", map[string]any{"label": "reference"})
	w.release(w.admin, page, "")
	if got := labelsOf(); got != "reference,working-notes" {
		t.Fatalf("an ordinary label beside a kept exempt one: %q", got)
	}

	fresh := w.page(w.writer, w.draftHome, "New guide", "words")
	w.writer.ok("POST", "/api/v1/pages/"+fresh+"/labels", map[string]any{"label": "working-notes"})
	preview = w.preview(w.admin, fresh)
	if code, out := w.admin.do("POST", "/api/v1/pages/"+fresh+"/release", map[string]any{"preview_id": preview["id"]}); code != http.StatusForbidden {
		t.Fatalf("a release put an exempt label on: %d %s", code, out)
	}
	if n := e.auditActions("page.exempted") + e.auditActions("page.exemption_ended"); n != 0 {
		t.Fatalf("a release changed an exemption: %d", n)
	}
}

// TestATwinVerifiesAgainstItsDraftVersion is PUB-732: verification maps the links of the
// release back and compares the content hash, so a twin verifies and a twin changed in the
// database does not; only those who see the draft space's audit may run it.
func TestATwinVerifiesAgainstItsDraftVersion(t *testing.T) {
	e := newEnv(t)
	w := newApprovalWorld(e, "APVERIFY", true)
	target := w.page(w.writer, w.draftHome, "Target", "the target")
	w.release(w.admin, target, "")
	unreleased := w.page(w.writer, w.draftHome, "Not released", "x")
	page := w.controlled("Linked procedure", "x")
	w.writer.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Linked procedure",
		"document": docOf(paraOf(textOf("see "), pageLinkTo(target, "top"), textOf(" and "), pageLinkTo(unreleased, ""))), "base_version": w.head(page)})
	w.approve(page)
	twin := w.twin(page)
	verify := func(u *user) map[string]any {
		t.Helper()
		return u.ok("POST", "/api/v1/pages/"+page+"/release/verify", nil)
	}
	if out := verify(w.admin); out["outcome"] != "verified" {
		t.Fatalf("a twin as released: %v", out)
	}
	for _, u := range []*user{w.writer, w.rel, w.reader} {
		if status, out := u.do("POST", "/api/v1/pages/"+page+"/release/verify", nil); status != http.StatusForbidden && status != http.StatusNotFound {
			t.Fatalf("%s verified without space.audit: %d %s", u.email, status, out)
		}
	}
	// A changed twin, as only someone who owns the database could change it: past the trigger that
	// keeps versions as they were written.
	ctx := context.Background()
	tx, err := e.app.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"ALTER TABLE page_versions DISABLE TRIGGER page_versions_no_update",
		`UPDATE page_versions SET document = jsonb_set(document, '{content,0,content,0,text}', '"see quietly "')
			WHERE page_id = '` + twin + `' AND number = (SELECT current_version FROM pages WHERE id = '` + twin + `')`,
		"ALTER TABLE page_versions ENABLE TRIGGER page_versions_no_update"} {
		if _, err := tx.Exec(ctx, sql); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("%s: %v", sql, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if out := verify(w.admin); out["outcome"] != "differs" {
		t.Fatalf("a twin changed in the database: %v", out)
	}
	if n := e.audited("action = 'release.verified' AND object_id = $1", page); n != 2 {
		t.Fatalf("%d verifications audited", n)
	}
}

// TestAnApprovalReleaseFitsItsBudget measures what an approval that releases adds: the request
// of the approval itself, which only enqueues an event, and the run in the worker.
func TestAnApprovalReleaseFitsItsBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("a measurement")
	}
	e := newEnv(t)
	timed := func(marked bool, key string) (approval, run time.Duration) {
		w := newApprovalWorld(e, key, marked)
		var approvals, runs []time.Duration
		for i := range 5 {
			page := w.controlled("Measured "+key+string(rune('a'+i)), "measured words")
			w.step(page, "submit")
			start := time.Now()
			w.step(page, "approve")
			approvals = append(approvals, time.Since(start))
			start = time.Now()
			w.settle()
			runs = append(runs, time.Since(start))
		}
		return minDuration(approvals), minDuration(runs)
	}
	plainApproval, plainAfter := timed(false, "APCOSTP")
	markedApproval, markedAfter := timed(true, "APCOSTM")
	t.Logf("an approval: %v without the mark, %v with it; the work after it: %v without, %v with", plainApproval, markedApproval, plainAfter, markedAfter)
	if markedApproval > plainApproval+250*time.Millisecond {
		t.Fatalf("the mark made the approval itself slower by %v", markedApproval-plainApproval)
	}
	if markedAfter > 5*time.Second {
		t.Fatalf("an approval release took %v", markedAfter)
	}
}
