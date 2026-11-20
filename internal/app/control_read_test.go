// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/notify"
)

// Document control, wave DC5 of spec 064: read and understood (DOC-820 to DOC-824) and
// periodic review (DOC-800 to DOC-805, DOC-1223).

// readDef is the review flow whose approved state asks a group to confirm reading.
func readDef(group string) map[string]any {
	d := reviewFlow()
	d["states"].([]any)[2].(map[string]any)["read_and_understood"] = map[string]any{"groups": []any{group}}
	return d
}

// otherSession logs the same person in again, as a second browser.
func (u *user) otherSession() *user {
	u.e.t.Helper()
	c := u.e.client()
	if got := u.e.login(c, u.email, adminPassword); got != 200 {
		u.e.t.Fatalf("login %s: %d", u.email, got)
	}
	return &user{e: u.e, c: c, email: u.email, id: u.id}
}

func readConfirm(u *user, page string, version int) (int, string) {
	u.e.t.Helper()
	return u.do("POST", "/api/v1/pages/"+page+"/read-confirmation", map[string]any{"version": version})
}

// TestAReadConfirmationIsBoundToTheVersionAndToOpeningIt is DOC-821, DOC-822 and DOC-824: only
// someone asked confirms, only the version readers see, only after opening that version in
// the same session, and never through a token. A new approved version asks again, and the
// earlier confirmation stays on record for its version.
func TestAReadConfirmationIsBoundToTheVersionAndToOpeningIt(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRU")
	staff := w.newGroup("Staff of DRU", w.reader, w.r1)
	id := w.workflowOf("Read it", readDef(staff))
	page := w.author.page(w.space, w.home, "Procedure", "first text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": id})
	w.approve(page)
	version := int(w.control(w.reader, page)["approved_version"].(float64))

	if code, _ := readConfirm(w.r2, page, version); code != 403 {
		t.Fatalf("someone not asked confirmed: %d", code)
	}
	// Not opened yet in this session: the button is not offered, and the answer is no.
	fresh := w.reader.otherSession()
	if code, out := readConfirm(fresh, page, version); code != 409 || !strings.Contains(out, "not_opened") {
		t.Fatalf("a confirmation without opening the version: %d %s", code, out)
	}
	// Opened in one session does not count for another.
	if rr := w.control(w.reader, page)["read_request"].(map[string]any); rr["can_confirm"] != true {
		t.Fatalf("the band offers no confirmation after opening: %v", rr)
	}
	if code, _ := readConfirm(fresh, page, version); code != 409 {
		t.Fatalf("a confirmation in a session that did not open the version: %d", code)
	}
	if code, _ := readConfirm(w.reader, page, version-1); code != 409 {
		t.Fatalf("a confirmation of another version: %d", code)
	}
	token := w.reader.ok("POST", "/api/v1/me/tokens", map[string]any{"name": "script", "scopes": []string{"read", "write"}})["secret"].(string)
	if code, _ := e.bearer("POST", "/api/v1/pages/"+page+"/read-confirmation", token, fmt.Sprintf(`{"version":%d}`, version)); code < 400 {
		t.Fatal("a token confirmed reading")
	}
	if code, out := readConfirm(w.reader, page, version); code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	if code, _ := readConfirm(w.reader, page, version); code != 409 {
		t.Fatalf("a second confirmation of the same version: %d", code)
	}
	if e.auditActions("page.read_confirmed") != 1 {
		t.Fatal("the confirmation is not audited")
	}

	// A new approved version asks again; the first confirmation stays for its version.
	w.publishAs(w.author, page, "Procedure", "second text", false)
	w.approve(page)
	next := int(w.control(w.reader, page)["approved_version"].(float64))
	if rr := w.control(w.reader, page)["read_request"].(map[string]any); rr["confirmed"] == true || int(rr["version"].(float64)) != next {
		t.Fatalf("a new version did not ask again: %v", rr)
	}
	if e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1 AND user_id = $2 AND confirmed_at IS NOT NULL", page, w.reader.id) != 1 {
		t.Fatal("the earlier confirmation is gone or doubled")
	}
	e.deliver()
	if notificationCount(e, w.r1, notify.TypeReadRequested) != 2 {
		t.Fatal("the group was not asked for each version")
	}
}

// TestPeopleWhoJoinTheGroupLaterAreAsked is DOC-820 and DOC-1418, and the list of DOC-823 for
// space.audit only, with CSV.
func TestPeopleWhoJoinTheGroupLaterAreAsked(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRV")
	staff := w.newGroup("Staff of DRV", w.reader)
	page := w.author.page(w.space, w.home, "Policy", "text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Read it", readDef(staff))})
	w.approve(page)
	if e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1", page) != 1 {
		t.Fatal("the group was not resolved at approval")
	}
	w.root.want(204, "PUT", "/api/v1/admin/groups/"+staff+"/members/"+w.r3.id, nil)
	// A guest who joins is never asked: guests are outside the organisation.
	w.root.want(204, "PUT", "/api/v1/admin/groups/"+staff+"/members/"+w.guest.id, nil)
	e.dueJob()
	if e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1 AND user_id = $2 AND late", page, w.r3.id) != 1 {
		t.Fatal("a late joiner was not asked")
	}
	if e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1 AND user_id = $2", page, w.guest.id) != 0 {
		t.Fatal("a guest was asked")
	}
	path := "/api/v1/pages/" + page + "/read-confirmations"
	if code, _ := w.author.do("GET", path, nil); code != 403 {
		t.Fatalf("an author without the audit right read the list: %d", code)
	}
	if code, _ := w.reader.do("GET", path, nil); code != 403 && code != 404 {
		t.Fatalf("a reader read the list: %d", code)
	}
	list := w.admin.ok("GET", path, nil)
	if people := list["people"].([]any); len(people) != 2 {
		t.Fatalf("the list holds %d people", len(people))
	}
	if code, out := w.admin.do("GET", path+"?format=csv", nil); code != 200 || !strings.Contains(out, "dru") && !strings.Contains(out, "drv-reader") {
		t.Fatalf("no CSV: %d %s", code, out)
	}
}

// TestOnlyPeopleWhoMayReadThePageAreAskedToConfirmIt is DOC-820 and DOC-823 under page
// restrictions: the groups are resolved by who may read the page, at approval and for people
// who join later, so the list names nobody a restriction keeps out.
func TestOnlyPeopleWhoMayReadThePageAreAskedToConfirmIt(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRP")
	staff := w.newGroup("Staff of DRP", w.reader, w.r2)
	page := w.author.page(w.space, w.home, "Restricted policy", "text")
	w.admin.restrict(page, "view", w.author.id, w.reader.id, w.r3.id)
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Read it", readDef(staff))})
	w.approve(page)
	asked := func(u *user) bool {
		t.Helper()
		return e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1 AND user_id = $2", page, u.id) == 1
	}
	if !asked(w.reader) || asked(w.r2) {
		t.Fatalf("at approval: reader asked %v, someone kept out asked %v", asked(w.reader), asked(w.r2))
	}
	for _, u := range []*user{w.r3, w.r1} {
		w.root.want(204, "PUT", "/api/v1/admin/groups/"+staff+"/members/"+u.id, nil)
	}
	e.dueJob()
	if !asked(w.r3) || asked(w.r1) {
		t.Fatalf("later: a reader who joined asked %v, someone kept out asked %v", asked(w.r3), asked(w.r1))
	}
	if people := w.admin.ok("GET", "/api/v1/pages/"+page+"/read-confirmations", nil)["people"].([]any); len(people) != 2 {
		t.Fatalf("the list holds %d people, want the two who may read the page", len(people))
	}
}

// TestTheJobAsksEachLaterJoinerOnceAndOnlyAfterTheyJoin is DOC-1418 on the incremental pass: a
// person who joins after the approval is asked at the next pass, once; a pass with nobody new
// asks nobody; someone who joined and left before the pass is not asked; and a person who
// joins a second group of the same request is not asked twice.
func TestTheJobAsksEachLaterJoinerOnceAndOnlyAfterTheyJoin(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRJ")
	staff := w.newGroup("Staff of DRJ", w.reader)
	d := reviewFlow()
	d["states"].([]any)[2].(map[string]any)["read_and_understood"] = map[string]any{"groups": []any{staff, w.group}}
	page := w.author.page(w.space, w.home, "Joined policy", "text")
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Read it", d)})
	w.approve(page)
	late := func(u *user) int {
		t.Helper()
		return e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1 AND user_id = $2 AND late", page, u.id)
	}
	e.dueJob()
	if n := e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1 AND late", page); n != 0 {
		t.Fatalf("a pass with nobody new asked %d people", n)
	}
	w.root.want(204, "PUT", "/api/v1/admin/groups/"+staff+"/members/"+w.r1.id, nil)
	w.root.want(204, "PUT", "/api/v1/admin/groups/"+staff+"/members/"+w.r2.id, nil)
	w.root.want(204, "DELETE", "/api/v1/admin/groups/"+staff+"/members/"+w.r1.id, nil)
	e.dueJob()
	if late(w.r1) != 0 {
		t.Fatal("someone who left the group before the pass was asked")
	}
	if n := e.count("SELECT count(*) FROM read_confirmations WHERE page_id = $1 AND user_id = $2", page, w.r2.id); n != 1 {
		t.Fatalf("someone in both groups was asked %d times", n)
	}
	w.root.want(204, "PUT", "/api/v1/admin/groups/"+staff+"/members/"+w.r1.id, nil)
	e.dueJob()
	e.dueJob()
	if late(w.r1) != 1 {
		t.Fatalf("a person who joined later was asked %d times", late(w.r1))
	}
	e.deliver()
	if notificationCount(e, w.r1, notify.TypeReadRequested) != 1 {
		t.Fatal("the person who joined later was not told once")
	}
}

// periodicDef is the review flow with reviewers whose approved state is timed, with a
// periodic review step into the review state.
func periodicDef(rv map[string]any, months int) map[string]any {
	d := reviewedDef(rv)
	approved := d["states"].([]any)[2].(map[string]any)
	approved["timed"], approved["review_months"] = true, months
	d["transitions"] = append(d["transitions"].([]any),
		map[string]any{"id": "periodic", "name": "Periodic review", "from": "approved", "to": "review", "outcome": "review_due"})
	return d
}

// TestAPeriodicReviewConfirmsWithoutANewVersion is DOC-800 to DOC-803: the approval sets the
// next review date, the page shows it to readers, a page may only shorten its period, the
// date starts a round on the same text while readers keep the approved version, and the
// round passing keeps the version and moves the date on.
func TestAPeriodicReviewConfirmsWithoutANewVersion(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DPR")
	id := w.workflowOf("Yearly", periodicDef(review("any", 0, entry("user", w.r1.id)), 12))
	page := w.reviewedPage(id, "Yearly procedure")
	w.answer(w.r1, page, "approve")
	approved := w.control(w.reader, page)
	if approved["next_review_at"] == nil {
		t.Fatal("readers are not shown the next review date")
	}
	version := approved["approved_version"]
	// A page's own period: only shorter, and only with the right to set workflows.
	if code, _ := w.reader.do("PUT", "/api/v1/pages/"+page+"/review-period", map[string]any{"months": 6}); code != 403 && code != 404 {
		t.Fatalf("a reader set the period: %d", code)
	}
	if code, out := w.admin.do("PUT", "/api/v1/pages/"+page+"/review-period", map[string]any{"months": 24}); code != 409 {
		t.Fatalf("a page period longer than its state's: %d %s", code, out)
	}
	w.admin.ok("PUT", "/api/v1/pages/"+page+"/review-period", map[string]any{"months": 6})
	if e.count("SELECT count(*) FROM periodic_reviews WHERE page_id = $1 AND due_at < now() + interval '7 months'", page) != 1 {
		t.Fatal("the shorter period did not move the date")
	}
	// The date comes: a round starts on the same text, and readers keep the approved version.
	e.sql("UPDATE periodic_reviews SET due_at = now() - interval '1 minute', next_at = now() - interval '1 minute' WHERE page_id = $1", page)
	e.dueJob()
	if w.state(page) != "review" {
		t.Fatalf("the review date did not start a review: %s", w.state(page))
	}
	if c := w.control(w.reader, page); c["approved_version"] != version {
		t.Fatal("readers lost the approved version during the periodic review")
	}
	w.answer(w.r1, page, "approve")
	after := w.control(w.reader, page)
	if after["approved_version"] != version {
		t.Fatalf("a periodic review made a new version: %v", after["approved_version"])
	}
	if after["next_review_at"] == approved["next_review_at"] {
		t.Fatal("the periodic review did not move the date on")
	}
	if e.auditActions("page.review_confirmed") != 1 {
		t.Fatal("the confirmation is not on record")
	}
}

// TestTheOwnerIsRemindedAndTheSpaceOwnersWhenTheOwnerLeft is DOC-802, DOC-805 and DOC-1223,
// with the report of DOC-804 for space.audit only.
func TestTheOwnerIsRemindedAndTheSpaceOwnersWhenTheOwnerLeft(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DPS")
	// A timed state without a periodic step keeps the page approved and keeps reminding.
	d := reviewedDef(review("any", 0, entry("user", w.r1.id)))
	d["states"].([]any)[2].(map[string]any)["timed"] = true
	page := w.reviewedPage(w.workflowOf("Timed", d), "Owned")
	w.answer(w.r1, page, "approve")
	e.sql("UPDATE periodic_reviews SET next_at = now() - interval '1 minute' WHERE page_id = $1", page)
	e.dueJob()
	e.deliver()
	if notificationCount(e, w.author, notify.TypePeriodicReview) != 1 {
		t.Fatal("the page owner was not reminded")
	}
	w.root.ok("POST", "/api/v1/admin/users/"+w.author.id+"/disable", nil)
	e.sql("UPDATE periodic_reviews SET due_at = now() - interval '1 minute', next_at = now() - interval '1 minute' WHERE page_id = $1", page)
	e.dueJob()
	e.deliver()
	if notificationCount(e, w.admin, notify.TypePeriodicReview) != 1 {
		t.Fatal("the space owners were not reminded when the owner left")
	}
	if c := w.control(w.admin, page); c["state_id"] != "approved" {
		t.Fatal("a timed state without a periodic step left approval")
	}
	reportPath := "/api/v1/spaces/" + w.space + "/control-reports/periodic"
	if code, _ := w.reader.do("GET", reportPath, nil); code != 403 && code != 404 {
		t.Fatalf("a reader read the report: %d", code)
	}
	rows := w.admin.ok("GET", reportPath, nil)["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("%d rows", len(rows))
	}
	row := rows[0].(map[string]any)
	if row["status"] != "overdue" || row["owner_gone"] != true {
		t.Fatalf("the report row: %v", row)
	}
}

// TestReportsShowOnlySpacesWithTheAuditRight is DOC-1030 and DOC-1031: every report kind for
// a space and across spaces, as JSON and CSV, never naming a space the person may not audit.
func TestReportsShowOnlySpacesWithTheAuditRight(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRT")
	other := newReviewWorld(e, "DRX")
	page := w.reviewedPage(w.workflowOf("Due", dueDef(review("any", 0, entry("user", w.r1.id)), 2, 0, "")), "Late one")
	passDue(e, page)
	hidden := other.reviewedPage(other.workflowOf("Due", dueDef(review("any", 0, entry("user", other.r1.id)), 2, 0, "")), "Elsewhere")
	passDue(e, hidden)
	for _, kind := range []string{"states", "overdue", "periodic", "read", "approvals"} {
		if code, _ := w.author.do("GET", "/api/v1/spaces/"+w.space+"/control-reports/"+kind, nil); code != 403 {
			t.Fatalf("%s: an author without the audit right: %d", kind, code)
		}
		w.admin.ok("GET", "/api/v1/spaces/"+w.space+"/control-reports/"+kind, nil)
		if code, out := w.admin.do("GET", "/api/v1/control-reports/"+kind+"?format=csv", nil); code != 200 || strings.Contains(out, "Elsewhere") {
			t.Fatalf("%s across spaces: %d %s", kind, code, out)
		}
	}
	w.admin.want(404, "GET", "/api/v1/spaces/"+w.space+"/control-reports/gossip", nil)
	overdue := w.admin.ok("GET", "/api/v1/control-reports/overdue", nil)["rows"].([]any)
	if len(overdue) != 1 || overdue[0].(map[string]any)["page_id"] != page {
		t.Fatalf("overdue across spaces: %v", overdue)
	}
}
