// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Document control, wave DC1 of spec 064. The pure rule is proved by the permission matrix;
// these tests prove the routes, the restart of review, the space and page workflows and the
// races around a transition.

// reviewFlow is draft, in review, approved, rejected and withdrawn.
func reviewFlow() map[string]any {
	state := func(id, name, kind string, start bool) map[string]any {
		return map[string]any{"id": id, "name": name, "kind": kind, "start": start}
	}
	step := func(id, name, from, to string, comment bool) map[string]any {
		return map[string]any{"id": id, "name": name, "from": from, "to": to, "requires_comment": comment}
	}
	return map[string]any{
		"states": []any{state("draft", "Draft", "draft", true), state("review", "In review", "in_review", false),
			state("approved", "Approved", "approved", false), state("rejected", "Rejected", "rejected", false),
			state("withdrawn", "Withdrawn", "obsolete", false)},
		"transitions": []any{step("submit", "Send for review", "draft", "review", false),
			step("approve", "Approve", "review", "approved", false),
			step("reject", "Reject", "review", "rejected", true),
			step("rework", "Rework", "rejected", "draft", false),
			step("withdraw", "Withdraw", "approved", "withdrawn", false)},
	}
}

// controlWorld is a space with a workflow, and people of each kind DC1 knows: the space
// administrator, an author (the editor role), a reader and a guest who may view.
type controlWorld struct {
	e                             *env
	admin, author, reader, guest  *user
	space, home, workflow, prefix string
}

func newControlWorld(e *env, key string) controlWorld {
	e.t.Helper()
	k := strings.ToLower(key)
	w := controlWorld{e: e, prefix: k}
	w.admin = e.user(k+"-admin@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	w.author = e.user(k+"-author@example.com", identity.PermUseProduct)
	w.reader = e.user(k+"-reader@example.com", identity.PermUseProduct)
	w.guest = e.userOfType(k+"-guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	w.space, w.home = w.admin.space(key)
	w.admin.grant(w.space, "user", w.author.id, permissions.TemplateEditor)
	w.admin.grant(w.space, "user", w.reader.id, permissions.TemplateViewer)
	w.admin.grant(w.space, "user", w.guest.id, permissions.TemplateViewer)
	w.workflow = w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Review", "definition": reviewFlow()})["id"].(string)
	return w
}

// controlled makes a published page and sets the workflow on it.
func (w controlWorld) controlled(title, text string) string {
	w.e.t.Helper()
	page := w.author.page(w.space, w.home, title, text)
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflow})
	return page
}

func (w controlWorld) control(u *user, page string) map[string]any {
	w.e.t.Helper()
	c, _ := u.ok("GET", "/api/v1/pages/"+page, nil)["control"].(map[string]any)
	return c
}

func (w controlWorld) head(page string) int {
	w.e.t.Helper()
	return int(w.control(w.author, page)["head_version"].(float64))
}

func (w controlWorld) step(page, transition string) {
	w.e.t.Helper()
	w.author.ok("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": transition, "version": w.head(page)})
}

func (w controlWorld) approve(page string) {
	w.e.t.Helper()
	w.step(page, "submit")
	w.step(page, "approve")
}

func (w controlWorld) state(page string) string {
	w.e.t.Helper()
	return w.control(w.author, page)["state_id"].(string)
}

// publishAs publishes new text on the head of a page.
func (w controlWorld) publishAs(u *user, page, title, text string, silent bool) {
	w.e.t.Helper()
	u.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": title, "document": para(text), "base_version": w.head(page), "silent": silent})
}

// seenBy lists, route by route, whether a person finds a page. Every route a reader has.
func (w controlWorld) seenBy(u *user, page, word string) map[string]bool {
	w.e.t.Helper()
	status, _ := u.do("GET", "/api/v1/pages/"+page, nil)
	versions, _ := u.do("GET", "/api/v1/pages/"+page+"/versions", nil)
	feed := false
	for _, item := range u.ok("GET", "/api/v1/feed?tab=recent", nil)["items"].([]any) {
		if item.(map[string]any)["page_id"] == page {
			feed = true
		}
	}
	search := false
	for _, row := range u.ok("GET", "/api/v1/search?q="+word, nil)["results"].([]any) {
		if row.(map[string]any)["page_id"] == page || row.(map[string]any)["id"] == page {
			search = true
		}
	}
	tree := false
	for _, row := range u.ok("GET", "/api/v1/spaces/"+w.space+"/tree", nil)["pages"].([]any) {
		if row.(map[string]any)["id"] == page {
			tree = true
		}
	}
	// The home page can be controlled too, and then hidden from a reader.
	child := false
	if code, out := u.do("GET", "/api/v1/pages/"+w.home+"/details", nil); code == 200 {
		child = strings.Contains(out, `"id":"`+page+`"`)
	}
	return map[string]bool{"page": status == 200, "versions": versions == 200, "feed": feed, "search": search, "tree": tree, "children": child}
}

func allAre(t *testing.T, who string, got map[string]bool, want bool) {
	t.Helper()
	for route, seen := range got {
		if seen != want {
			t.Errorf("%s on %s: seen %v, want %v", who, route, seen, want)
		}
	}
}

// TestANeverApprovedPageIsAbsentForReadersOnEveryRoute is DOC-300 and DOC-301 on the routes:
// a reader and a guest find nothing, the author and the administrator find it with its band,
// and approval shows it to all.
func TestANeverApprovedPageIsAbsentForReadersOnEveryRoute(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCA")
	page := w.controlled("Lifting procedure", "zebracode lifting")
	for _, u := range []*user{w.reader, w.guest} {
		allAre(t, u.email, w.seenBy(u, page, "zebracode"), false)
	}
	for _, u := range []*user{w.author, w.admin} {
		allAre(t, u.email, w.seenBy(u, page, "zebracode"), true)
		c := w.control(u, page)
		if c["state_id"] != "draft" || c["approved_version"] != nil || c["governed_by"] != "page" {
			t.Fatalf("band for %s: %v", u.email, c)
		}
	}
	w.approve(page)
	for _, u := range []*user{w.reader, w.guest, w.author, w.admin} {
		allAre(t, u.email+" after approval", w.seenBy(u, page, "zebracode"), true)
	}
	c := w.control(w.reader, page)
	if c["state_name"] != "Approved" || c["approved_by"] != "Test "+w.author.email || len(c["transitions"].([]any)) != 0 {
		t.Fatalf("reader band: %v", c)
	}
}

// TestReadersSeeTheApprovedVersionWhileAChangeWaits is DOC-300, DOC-303, DOC-320 and DOC-321:
// a change is the author's until approved; then the version before is obsolete, and only
// those who may see history reach it.
func TestReadersSeeTheApprovedVersionWhileAChangeWaits(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCB")
	page := w.controlled("Policy", "firstword text")
	w.approve(page)
	w.publishAs(w.author, page, "Policy changed", "secondword text", false)

	got := w.reader.ok("GET", "/api/v1/pages/"+page, nil)
	if got["title"] != "Policy" || !strings.Contains(fmt.Sprint(got["document"]), "firstword") || got["current_version"].(float64) != 1 {
		t.Fatalf("the reader sees %v %v %v", got["title"], got["current_version"], got["document"])
	}
	if c := got["control"].(map[string]any); c["shows_working"] != false || c["head_version"].(float64) != 1 {
		t.Fatalf("the reader learns of the head: %v", c)
	}
	mine := w.author.ok("GET", "/api/v1/pages/"+page, nil)
	if mine["title"] != "Policy changed" || mine["current_version"].(float64) != 2 || mine["control"].(map[string]any)["shows_working"] != true {
		t.Fatalf("the author sees %v %v %v", mine["title"], mine["current_version"], mine["control"])
	}
	if w.seenBy(w.reader, page, "secondword")["search"] || !w.seenBy(w.reader, page, "firstword")["search"] {
		t.Fatal("search shows the working text")
	}
	if n := len(w.reader.ok("GET", "/api/v1/pages/"+page+"/versions", nil)["versions"].([]any)); n != 1 {
		t.Fatalf("the reader's history has %d versions", n)
	}
	w.reader.want(404, "GET", "/api/v1/pages/"+page+"/versions/2", nil)
	w.author.ok("GET", "/api/v1/pages/"+page+"/versions/2", nil)

	w.approve(page)
	if got := w.reader.ok("GET", "/api/v1/pages/"+page, nil); got["title"] != "Policy changed" || got["current_version"].(float64) != 2 {
		t.Fatalf("after approval the reader sees %v", got["title"])
	}
	// The version approved before is obsolete: kept in history, out of a reader's reach.
	w.reader.want(404, "GET", "/api/v1/pages/"+page+"/versions/1", nil)
	w.author.ok("GET", "/api/v1/pages/"+page+"/versions/1", nil)
}

// TestWhatRestartsReview is the table of DOC-310 to DOC-312 and DOC-1410: every published
// change starts review again, silent or restored; what is not content does not.
func TestWhatRestartsReview(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCC")
	other := w.author.page(w.space, w.home, "Elsewhere", "x")
	cases := []struct {
		name    string
		change  func(page string)
		restart bool
	}{
		{"a publish", func(p string) { w.publishAs(w.author, p, "T", "changed text", false) }, true},
		{"a silent publish", func(p string) { w.publishAs(w.author, p, "T", "quietly changed", true) }, true},
		{"a rename", func(p string) { w.author.ok("PATCH", "/api/v1/pages/"+p, map[string]any{"title": "Renamed"}) }, true},
		{"a label", func(p string) { w.author.ok("POST", "/api/v1/pages/"+p+"/labels", map[string]any{"label": "quality"}) }, false},
		{"a move in the space", func(p string) { w.author.ok("POST", "/api/v1/pages/"+p+"/move", map[string]any{"parent_id": other}) }, false},
		{"a comment", func(p string) {
			w.reader.ok("POST", "/api/v1/pages/"+p+"/comments", map[string]any{"body": para("a remark")})
		}, false},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page := w.controlled(fmt.Sprintf("T%d", i), "approved text")
			w.approve(page)
			tc.change(page)
			want := "approved"
			if tc.restart {
				want = "draft"
			}
			if got := w.state(page); got != want {
				t.Fatalf("state %q, want %q", got, want)
			}
		})
	}
	t.Run("restoring an older version", func(t *testing.T) {
		page := w.controlled("Restored", "one")
		w.approve(page)
		w.publishAs(w.author, page, "Restored", "two", false)
		w.approve(page)
		w.author.ok("POST", "/api/v1/pages/"+page+"/versions/1/restore", nil)
		if got := w.state(page); got != "draft" {
			t.Fatalf("a restore went around review: %q", got)
		}
		if got := w.reader.ok("GET", "/api/v1/pages/"+page, nil); !strings.Contains(fmt.Sprint(got["document"]), "two") {
			t.Fatal("a restore reached readers without review")
		}
	})
	t.Run("restoring the approved text", func(t *testing.T) {
		page := w.controlled("Back", "kept")
		w.approve(page)
		w.publishAs(w.author, page, "Back", "tried", false)
		w.author.ok("POST", "/api/v1/pages/"+page+"/versions/1/restore", nil)
		c := w.control(w.author, page)
		if c["state_id"] != "approved" || c["shows_working"] != false {
			t.Fatalf("equal to the approved text, yet waiting: %v", c)
		}
	})
}

// TestAPublishSupersedesTheRoundAndItsAnswers is DOC-330: an answer about a replaced text
// does not count, and the round keeps its record.
func TestAPublishSupersedesTheRoundAndItsAnswers(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCD")
	page := w.controlled("Guide", "v1")
	w.approve(page)
	w.publishAs(w.author, page, "Guide", "v2", false)
	w.step(page, "submit")
	looked := w.head(page)
	w.publishAs(w.author, page, "Guide", "v3", false)
	if w.state(page) != "draft" {
		t.Fatal("a publish during review did not start review again")
	}
	w.step(page, "submit")
	out := w.author.want(409, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": looked})
	if !strings.Contains(out, "superseded") {
		t.Fatalf("an approval of a replaced text: %s", out)
	}
	if n := e.count("SELECT count(*) FROM workflow_rounds WHERE page_id = $1 AND status = 'superseded'", page); n < 1 {
		t.Fatal("the round on v2 left no record")
	}
	if n := e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND transition_id = 'submit' AND version_number = $2", page, looked); n != 1 {
		t.Fatalf("the answer on v2 is gone: %d", n)
	}
	// The restart is the system's step, never put on a person (DOC-105).
	if e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND transition_id = '_system' AND user_id IS NOT NULL", page) != 0 ||
		e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND transition_id = '_system'", page) == 0 {
		t.Fatal("a system step names a person, or is not recorded")
	}
	// A step that needs a comment is refused without one.
	w.author.want(400, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "reject", "version": w.head(page)})
}

// TestRemovingControlShowsNoUnapprovedText is DOC-141 and DOC-1407: readers keep the
// approved version, and the change that waited stays out of their history until the next
// publish. The preview counts the pages readers did not see.
func TestRemovingControlShowsNoUnapprovedText(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCP")
	page := w.controlled("Kept", "approved words")
	w.approve(page)
	w.publishAs(w.author, page, "Kept", "waiting words", false)
	never := w.controlled("Never", "draft words")
	preview := w.author.ok("PUT", "/api/v1/pages/"+never+"/workflow", map[string]any{"workflow_id": nil, "preview": true})
	if preview["revealed"].(float64) != 1 {
		t.Fatalf("preview: %v", preview)
	}
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": nil})
	if got := fmt.Sprint(w.reader.ok("GET", "/api/v1/pages/"+page, nil)["document"]); !strings.Contains(got, "approved words") {
		t.Fatalf("after removal the reader sees %s", got)
	}
	w.reader.want(404, "GET", "/api/v1/pages/"+page+"/versions/2", nil)
	if n := len(w.reader.ok("GET", "/api/v1/pages/"+page+"/versions", nil)["versions"].([]any)); n != 1 {
		t.Fatalf("the reader's history holds %d versions", n)
	}
	w.author.ok("GET", "/api/v1/pages/"+page+"/versions/2", nil)
	if n := e.count("SELECT count(*) FROM workflow_rounds WHERE page_id = $1", page); n == 0 {
		t.Fatal("removing control dropped the record of its rounds")
	}
	// The next publish is an ordinary one again.
	w.author.ok("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Kept", "document": para("public words"), "base_version": 1})
	if got := w.reader.ok("GET", "/api/v1/pages/"+page, nil); got["current_version"].(float64) != 3 {
		t.Fatalf("the next publish became version %v", got["current_version"])
	}
}

// TestAWithdrawnPageDisappearsForReaders is DOC-322.
func TestAWithdrawnPageDisappearsForReaders(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCE")
	page := w.controlled("Old procedure", "yakword")
	w.approve(page)
	w.step(page, "withdraw")
	allAre(t, "reader", w.seenBy(w.reader, page, "yakword"), false)
	allAre(t, "author", w.seenBy(w.author, page, "yakword"), true)
}

// TestTheSpaceWorkflowOverridesPageWorkflows is DOC-400, DOC-401, DOC-420 and DOC-421.
func TestTheSpaceWorkflowOverridesPageWorkflows(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCF")
	own := w.controlled("Own workflow", "a")
	w.step(own, "submit")
	plain := w.author.page(w.space, w.home, "Plain page", "b")
	body := map[string]any{"workflow_id": w.workflow, "kinds": []string{"page"}, "preview": true}
	preview := w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", body)
	if len(preview["replaced_workflows"].([]any)) != 1 || preview["start"].(float64) < 2 {
		t.Fatalf("preview: %v", preview)
	}
	if e.count("SELECT count(*) FROM space_workflows") != 0 || e.count("SELECT count(*) FROM page_workflows") != 1 {
		t.Fatal("the preview changed something")
	}
	w.author.want(403, "PUT", "/api/v1/spaces/"+w.space+"/workflow", map[string]any{"workflow_id": w.workflow, "kinds": []string{"page"}})
	body["preview"] = false
	w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", body)
	if e.count("SELECT count(*) FROM page_workflows") != 0 {
		t.Fatal("a page workflow survived the space workflow")
	}
	if n := e.count("SELECT count(*) FROM workflow_rounds WHERE page_id = $1 AND ended_reason = 'space_workflow'", own); n != 1 {
		t.Fatalf("the round under the page workflow did not end as superseded by the space: %d", n)
	}
	if c := w.control(w.author, plain); c["governed_by"] != "space" || c["state_id"] != "draft" {
		t.Fatalf("plain page: %v", c)
	}
	fresh := w.author.page(w.space, w.home, "Fresh", "c")
	if w.state(fresh) != "draft" || w.seenBy(w.reader, fresh, "Fresh")["page"] {
		t.Fatal("a new page in a controlled space did not start in the start state")
	}
	w.author.want(409, "PUT", "/api/v1/pages/"+plain+"/workflow", map[string]any{"workflow_id": w.workflow})

	w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", map[string]any{"workflow_id": nil})
	if e.count("SELECT count(*) FROM page_control") != 0 || e.count("SELECT count(*) FROM page_workflows") != 0 {
		t.Fatal("turning the space workflow off brought control back")
	}
	if !w.seenBy(w.reader, plain, "Plain")["page"] {
		t.Fatal("an ordinary page again is not seen")
	}
}

// TestABranchWorkflowFollowsMovesInAndOut is DOC-410 and DOC-411.
func TestABranchWorkflowFollowsMovesInAndOut(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCG")
	parent := w.author.page(w.space, w.home, "Handbook", "h")
	child := w.author.page(w.space, parent, "Chapter", "c")
	w.author.ok("PUT", "/api/v1/pages/"+child+"/workflow", map[string]any{"workflow_id": w.workflow})
	w.approve(child)
	branch := map[string]any{"workflow_id": w.workflow, "branch": true}
	out := w.author.want(409, "PUT", "/api/v1/pages/"+parent+"/workflow", branch)
	if !strings.Contains(out, "children_have_workflows") {
		t.Fatalf("a branch over a page workflow: %s", out)
	}
	branch["preview"] = true
	if got := w.author.ok("PUT", "/api/v1/pages/"+parent+"/workflow", branch); len(got["replaced_workflows"].([]any)) != 1 {
		t.Fatalf("preview: %v", got)
	}
	branch["preview"], branch["replace_children"] = false, true
	w.author.ok("PUT", "/api/v1/pages/"+parent+"/workflow", branch)
	if c := w.control(w.author, child); c["governed_by"] != "branch" || c["state_id"] != "approved" {
		t.Fatalf("the approved chapter under the branch: %v", c)
	}
	inherited := w.author.page(w.space, parent, "New chapter", "n")
	if c := w.control(w.author, inherited); c["governed_by"] != "branch" || c["state_id"] != "draft" {
		t.Fatalf("a new child did not inherit: %v", c)
	}
	w.author.want(409, "PUT", "/api/v1/pages/"+inherited+"/workflow", map[string]any{"workflow_id": w.workflow})

	outside := w.author.page(w.space, w.home, "Loose page", "l")
	w.author.ok("POST", "/api/v1/pages/"+outside+"/move", map[string]any{"parent_id": parent})
	if c := w.control(w.author, outside); c == nil || c["state_id"] != "draft" || w.seenBy(w.reader, outside, "Loose")["page"] {
		t.Fatalf("a page moved in did not join in the start state: %v", c)
	}
	w.author.ok("POST", "/api/v1/pages/"+child+"/move", map[string]any{"parent_id": w.home})
	if c := w.control(w.author, child); c != nil {
		t.Fatalf("a page moved out kept the workflow: %v", c)
	}
	if !w.seenBy(w.reader, child, "Chapter")["page"] {
		t.Fatal("a page moved out is not an ordinary page at its approved version")
	}
}

// TestAssignmentLandsApprovedPagesAndAFailureLeavesNothing is DOC-140 and DOC-401: a page
// approved under today's approval rounds lands approved, the rest in the start state, and
// an assignment that fails half-way leaves no page half-assigned.
func TestAssignmentLandsApprovedPagesAndAFailureLeavesNothing(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCH")
	approved := w.author.page(w.space, w.home, "Approved today", "a")
	w.author.page(w.space, w.home, "Not approved", "b")
	e.sql(`INSERT INTO approval_rounds (id, page_id, asked_by, title, document, base_version, approved_version, status)
		SELECT gen_random_uuid(), p.id, p.created_by, p.title, v.document, p.current_version, p.current_version, 'approved'
		FROM pages p JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version WHERE p.id = $1`, approved)
	body := map[string]any{"workflow_id": w.workflow, "kinds": []string{"page"}, "preview": true}
	preview := w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", body)
	if preview["approved"].(float64) != 1 || preview["start"].(float64) != preview["pages"].(float64)-1 {
		t.Fatalf("preview: %v", preview)
	}

	// A fault on the last page: the whole assignment rolls back.
	e.sql(`CREATE FUNCTION dc_fail() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.page_id = '` + approved + `' THEN RAISE EXCEPTION 'injected'; END IF; RETURN NEW; END $$`)
	e.sql("CREATE TRIGGER dc_fail BEFORE INSERT ON page_control FOR EACH ROW EXECUTE FUNCTION dc_fail()")
	body["preview"] = false
	if status, _ := w.admin.do("PUT", "/api/v1/spaces/"+w.space+"/workflow", body); status != 500 {
		t.Fatalf("the injected fault answered %d", status)
	}
	if e.count("SELECT count(*) FROM page_control")+e.count("SELECT count(*) FROM space_workflows")+e.count("SELECT count(*) FROM workflow_rounds") != 0 {
		t.Fatal("a failed assignment left pages half-assigned")
	}
	e.sql("DROP TRIGGER dc_fail ON page_control")
	got := w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/workflow", body)
	if got["approved"] != preview["approved"] || got["start"] != preview["start"] {
		t.Fatalf("the change landed %v, the preview said %v", got, preview)
	}
	if !w.seenBy(w.reader, approved, "Approved")["page"] {
		t.Fatal("a page approved today disappeared")
	}
}

// TestTwoFinalAnswersAtOnceGiveOneApproval is DOC-1230.
func TestTwoFinalAnswersAtOnceGiveOneApproval(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCI")
	page := w.controlled("Race", "x")
	w.step(page, "submit")
	head := w.head(page)
	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i, u := range []*user{w.author, w.admin} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses[i], _ = u.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": head})
		}()
	}
	wg.Wait()
	if !(statuses[0] == 200 && statuses[1] == 409) && !(statuses[0] == 409 && statuses[1] == 200) {
		t.Fatalf("two final answers: %v", statuses)
	}
	if n := e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND transition_id = 'approve'", page); n != 1 {
		t.Fatalf("%d approvals recorded", n)
	}
}

// TestAPublishAndAFinalApprovalAtOnceNeverApproveTheWrongText is DOC-1231: whichever comes
// first, what readers see is a text someone approved.
func TestAPublishAndAFinalApprovalAtOnceNeverApproveTheWrongText(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCJ")
	for round := range 5 {
		page := w.controlled(fmt.Sprintf("Race %d", round), "approved one")
		w.approve(page)
		w.publishAs(w.author, page, "Race", "under review", false)
		w.step(page, "submit")
		head := w.head(page)
		var wg sync.WaitGroup
		var approval int
		wg.Add(2)
		go func() {
			defer wg.Done()
			approval, _ = w.admin.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": head})
		}()
		go func() {
			defer wg.Done()
			w.author.do("POST", "/api/v1/pages/"+page+"/publish", map[string]any{"title": "Race", "document": para("newest"), "base_version": head})
		}()
		wg.Wait()
		seen := fmt.Sprint(w.reader.ok("GET", "/api/v1/pages/"+page, nil)["document"])
		switch {
		case strings.Contains(seen, "newest"):
			t.Fatal("a text nobody approved reached readers")
		case approval == 200 && !strings.Contains(seen, "under review"):
			t.Fatal("an approval went missing")
		case approval != 200 && !strings.Contains(seen, "approved one"):
			t.Fatalf("a refused approval changed what readers see: %s", seen)
		}
	}
}

// TestReadingDuringATransitionSeesOneWholeVersion is DOC-1233: a reader gets the old text or
// the new one, never a mix of title and text.
func TestReadingDuringATransitionSeesOneWholeVersion(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCK")
	page := w.controlled("Old title", "old body")
	w.approve(page)
	w.publishAs(w.author, page, "New title", "new body", false)
	w.step(page, "submit")
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	mixed := 0
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			got := w.reader.ok("GET", "/api/v1/pages/"+page, nil)
			doc := fmt.Sprint(got["document"])
			if (got["title"] == "Old title") != strings.Contains(doc, "old body") {
				mixed++
			}
		}
	}()
	w.step(page, "approve")
	close(done)
	wg.Wait()
	if mixed > 0 {
		t.Fatalf("%d reads mixed two versions", mixed)
	}
}

// TestAFaultDuringATransitionChangesNothing is DOC-1250: a failure while the approved text
// is made current rolls the whole step back.
func TestAFaultDuringATransitionChangesNothing(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCL")
	page := w.controlled("Fragile", "one")
	w.approve(page)
	w.publishAs(w.author, page, "Fragile", "two", false)
	w.step(page, "submit")
	e.sql(`CREATE FUNCTION dc_fail_search() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$`)
	e.sql("CREATE TRIGGER dc_fail_search BEFORE INSERT OR UPDATE ON search_documents FOR EACH ROW EXECUTE FUNCTION dc_fail_search()")
	if status, _ := w.author.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page)}); status != 500 {
		t.Fatalf("the injected fault answered %d", status)
	}
	e.sql("DROP TRIGGER dc_fail_search ON search_documents")
	if w.state(page) != "review" {
		t.Fatal("the state moved on")
	}
	if got := w.reader.ok("GET", "/api/v1/pages/"+page, nil); got["current_version"].(float64) != 1 {
		t.Fatal("readers see a version nobody approved")
	}
	if n := e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND transition_id = 'approve'", page); n != 1 {
		t.Fatalf("the failed step left an answer: %d approvals", n)
	}
	w.step(page, "approve")
	if got := w.reader.ok("GET", "/api/v1/pages/"+page, nil); got["current_version"].(float64) != 2 {
		t.Fatal("the step did not work once the fault was gone")
	}
}

// TestAControlledReadCostsAtMostOneQueryMore is DOC-1300.
func TestAControlledReadCostsAtMostOneQueryMore(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCM")
	plain := w.author.page(w.space, w.home, "Plain", "x")
	page := w.controlled("Controlled", "x")
	w.approve(page)
	w.publishAs(w.author, page, "Controlled", "y", false)
	for _, u := range []*user{w.reader, w.author} {
		base := queriesOf(t, e, u, "/api/v1/pages/"+plain)
		if got := queriesOf(t, e, u, "/api/v1/pages/"+page); got > base+1 {
			t.Errorf("%s: a controlled page costs %d queries, an ordinary one %d", u.email, got, base)
		}
		if base > queryBudgets["open a page"] {
			t.Errorf("%s: an ordinary page costs %d queries, over its budget", u.email, base)
		}
	}
}

// TestTheFeedNeverShowsAVersionWaitingForReview: the feed of a reader holds only versions
// readers may see (DOC-300).
func TestTheFeedNeverShowsAVersionWaitingForReview(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCN")
	page := w.controlled("Feed page", "x")
	w.approve(page)
	before := e.scalar("SELECT max(created_at)::text FROM page_versions WHERE page_id = $1", page)
	w.publishAs(w.author, page, "Feed page", "working", false)
	for _, item := range w.reader.ok("GET", "/api/v1/feed?tab=recent", nil)["items"].([]any) {
		row := item.(map[string]any)
		if row["page_id"] == page && row["kind"] != "created" {
			t.Fatalf("the feed shows the change under review: %v (approved at %s)", row, before)
		}
	}
}

// TestAWorkflowWithoutAWayToApprovalIsRefusedAndAnOccupiedStateStays is DOC-102 and DOC-110
// through the API.
func TestAWorkflowWithoutAWayToApprovalIsRefusedAndAnOccupiedStateStays(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "DCO")
	broken := reviewFlow()
	steps := broken["transitions"].([]any)
	broken["transitions"] = []any{steps[0], steps[1], steps[2], steps[4]} // rejected leads nowhere
	out := w.admin.want(409, "POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Broken", "definition": broken})
	if !strings.Contains(out, "workflow_no_way_to_approval") {
		t.Fatalf("refusal: %s", out)
	}
	w.author.want(403, "POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Mine", "definition": reviewFlow()})

	page := w.controlled("Occupied", "x")
	w.step(page, "submit")
	smaller := reviewFlow()
	smaller["states"] = []any{smaller["states"].([]any)[0], smaller["states"].([]any)[2]}
	smaller["transitions"] = []any{map[string]any{"id": "approve", "name": "Approve", "from": "draft", "to": "approved"}}
	out = w.admin.want(409, "PUT", "/api/v1/workflows/"+w.workflow, map[string]any{"name": "Review", "definition": smaller, "base_version": 1})
	if !strings.Contains(out, "state_occupied") {
		t.Fatalf("removing an occupied state: %s", out)
	}
	renamed := reviewFlow()
	renamed["states"].([]any)[1].(map[string]any)["name"] = "Under review"
	saved := w.admin.ok("PUT", "/api/v1/workflows/"+w.workflow, map[string]any{"name": "Review", "definition": renamed, "base_version": 1})
	if saved["version"].(float64) != 2 {
		t.Fatalf("saved: %v", saved)
	}
	if c := w.control(w.author, page); c["workflow_version"].(float64) != 1 || c["state_name"] != "In review" {
		t.Fatalf("a running round left its version: %v", c)
	}
	history := w.admin.ok("GET", "/api/v1/workflows/"+w.workflow+"/history", nil)["versions"].([]any)
	if len(history) != 2 || !strings.Contains(fmt.Sprint(history[0]), "state_renamed") {
		t.Fatalf("history: %v", history)
	}
}
