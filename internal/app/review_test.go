// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Document control, wave DC2 of spec 064: reviewers, rules to pass, independence, the
// reading being asked gives, the review panel and instance templates. Failure and
// permission paths come first in each test.

// reviewWorld is a control world with an instance administrator and three reviewers who view
// the space; the group holds the second and the third.
type reviewWorld struct {
	controlWorld
	root       *user
	r1, r2, r3 *user
	group      string
}

func newReviewWorld(e *env, key string) reviewWorld {
	e.t.Helper()
	k := strings.ToLower(key)
	w := reviewWorld{controlWorld: newControlWorld(e, key)}
	w.root = e.user(k+"-root@example.com", identity.PermUseProduct, identity.PermAdministrator)
	w.r1 = e.user(k+"-r1@example.com", identity.PermUseProduct)
	w.r2 = e.user(k+"-r2@example.com", identity.PermUseProduct)
	w.r3 = e.user(k+"-r3@example.com", identity.PermUseProduct)
	for _, r := range []*user{w.r1, w.r2, w.r3} {
		w.admin.grant(w.space, "user", r.id, permissions.TemplateViewer)
	}
	w.group = w.newGroup("Reviewers of "+key, w.r2, w.r3)
	return w
}

func (w reviewWorld) newGroup(name string, members ...*user) string {
	w.e.t.Helper()
	group := w.root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": name})["id"].(string)
	for _, m := range members {
		w.root.want(204, "PUT", "/api/v1/admin/groups/"+group+"/members/"+m.id, nil)
	}
	return group
}

func entry(typ, id string) map[string]any { return map[string]any{"type": typ, "id": id} }

// reviewedDef is the review flow of DC1 with reviewers on its review state: the round passes
// into approved, and a "changes" answer sends the page to rejected.
func reviewedDef(review map[string]any) map[string]any {
	d := reviewFlow()
	d["states"].([]any)[1].(map[string]any)["review"] = review
	steps := d["transitions"].([]any)
	steps[1].(map[string]any)["outcome"] = "approved"
	steps[2].(map[string]any)["outcome"] = "changes"
	return d
}

func review(rule string, quorum int, entries ...map[string]any) map[string]any {
	list := []any{}
	for _, e := range entries {
		list = append(list, e)
	}
	return map[string]any{"reviewers": list, "rule": rule, "quorum": quorum}
}

// workflowOf makes a workflow of the space and returns its id.
func (w reviewWorld) workflowOf(name string, def map[string]any) string {
	w.e.t.Helper()
	return w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": name, "definition": def})["id"].(string)
}

// reviewedPage makes a page under a workflow and sends it for review.
func (w reviewWorld) reviewedPage(workflowID, title string) string {
	w.e.t.Helper()
	page := w.author.page(w.space, w.home, title, "text of "+title)
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": workflowID})
	w.step(page, "submit")
	return page
}

func (w reviewWorld) answer(u *user, page, answer string) (int, string) {
	w.e.t.Helper()
	return u.do("POST", "/api/v1/pages/"+page+"/review/answer", map[string]any{"answer": answer, "comment": "a reason", "version": w.head(page)})
}

func (w reviewWorld) asked(page string) int {
	w.e.t.Helper()
	return w.e.count(`SELECT count(*) FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id
		WHERE r.page_id = $1 AND r.status = 'open' AND wr.stage = r.review_stage AND wr.removed_at IS NULL`, page)
}

// TestOnlyReviewersAnswerAndNobodyStepsAroundTheReview is DOC-013 and DOC-1424: a reader, a
// stranger and a guest cannot answer; nobody, not even an administrator, takes the step that
// follows the review by hand; a reviewer's answer takes it.
func TestOnlyReviewersAnswerAndNobodyStepsAroundTheReview(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRA")
	stranger := e.user("dra-stranger@example.com", identity.PermUseProduct)
	page := w.reviewedPage(w.workflowOf("Any one", reviewedDef(review("any", 0, entry("user", w.r1.id)))), "Procedure")

	// A page never approved does not exist for a reader (DOC-301), so the refusal is 404.
	if code, _ := w.answer(w.reader, page, "approve"); code != 404 {
		t.Fatalf("a reader answered: %d", code)
	}
	if code, _ := w.answer(stranger, page, "approve"); code != 404 {
		t.Fatalf("a stranger learned of the page: %d", code)
	}
	if code, _ := w.answer(w.guest, page, "approve"); code != 404 && code != 403 {
		t.Fatalf("a guest answered: %d", code)
	}
	// The author sees the page and is not asked: refused, not ignored.
	if code, _ := w.answer(w.author, page, "approve"); code != 403 {
		t.Fatalf("the author answered: %d", code)
	}
	for _, u := range []*user{w.author, w.admin, w.root} {
		code, out := u.do("POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "approve", "version": w.head(page)})
		if code != 409 && code != 403 && code != 404 {
			t.Fatalf("%s took the step of the reviewers: %d %s", u.email, code, out)
		}
	}
	if c := w.control(w.author, page); len(c["transitions"].([]any)) != 0 || c["in_review"] != true {
		t.Fatalf("the band offers the reviewers' steps to the author: %v", c)
	}
	if w.state(page) != "review" {
		t.Fatal("the page left review without its reviewer")
	}
	if code, out := w.answer(w.r1, page, "approve"); code != 200 {
		t.Fatalf("the reviewer: %d %s", code, out)
	}
	if w.state(page) != "approved" {
		t.Fatal("the answer did not take the step")
	}
	// The step is the system's, with its cause; the answer is the reviewer's (DOC-105).
	if e.count("SELECT count(*) FROM workflow_answers WHERE page_id = $1 AND transition_id = 'approve' AND user_id IS NULL AND cause = 'review_passed'", page) != 1 {
		t.Fatal("the system's step is not recorded as the system's")
	}
	if e.auditActions("page.review_answered") < 1 || e.auditActions("page.review_approved") < 1 {
		t.Fatal("the answer or the step is not audited")
	}
}

// TestWhoIsAskedIsFrozenLimitedAndNeverNobody is DOC-200, DOC-201, DOC-1221 and DOC-1222.
func TestWhoIsAskedIsFrozenLimitedAndNeverNobody(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRB")

	t.Run("an empty group refuses the step and names the group", func(t *testing.T) {
		empty := w.newGroup("Nobody here")
		page := w.author.page(w.space, w.home, "Empty", "x")
		w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Empty", reviewedDef(review("all", 0, entry("group", empty))))})
		out := w.author.want(409, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page)})
		if !strings.Contains(out, "reviewers_empty") || !strings.Contains(out, "Nobody here") {
			t.Fatalf("the refusal does not name the entry: %s", out)
		}
		if w.state(page) != "draft" {
			t.Fatal("a refused step moved the page")
		}
	})
	t.Run("a role nobody holds in the space refuses new rounds", func(t *testing.T) {
		role := w.root.ok("POST", "/api/v1/admin/roles", map[string]any{"scope": permissions.ScopeSpace, "name": "Quality lead", "description": "",
			"permissions": []string{permissions.SpaceView}})["id"].(string)
		page := w.author.page(w.space, w.home, "Role", "x")
		w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Role", reviewedDef(review("any", 0, entry("role", role))))})
		w.author.want(409, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page)})
		w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/role-assignments", map[string]any{
			"subject": map[string]any{"type": "user", "id": w.r1.id}, "roles": []any{role}})
		w.step(page, "submit")
		if w.asked(page) != 1 {
			t.Fatal("the role did not resolve to its holder")
		}
		// Taking the role away leaves the running round as it is (DOC-1222).
		w.admin.ok("PUT", "/api/v1/spaces/"+w.space+"/role-assignments", map[string]any{
			"subject": map[string]any{"type": "user", "id": w.r1.id}, "roles": []any{}})
		if w.asked(page) != 1 {
			t.Fatal("removing the role changed a running round")
		}
	})
	t.Run("a group changed or deleted later leaves the round as it was", func(t *testing.T) {
		group := w.newGroup("Changing", w.r2, w.r3)
		page := w.reviewedPage(w.workflowOf("Group", reviewedDef(review("all", 0, entry("group", group)))), "Frozen")
		w.root.want(204, "PUT", "/api/v1/admin/groups/"+group+"/members/"+w.r1.id, nil)
		w.root.want(204, "DELETE", "/api/v1/admin/groups/"+group+"/members/"+w.r3.id, nil)
		if w.asked(page) != 2 || e.count(`SELECT count(*) FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id
			WHERE r.page_id = $1 AND wr.user_id = $2`, page, w.r1.id) != 0 {
			t.Fatal("a later change of the group rewrote who was asked")
		}
		w.root.want(204, "DELETE", "/api/v1/admin/groups/"+group, nil)
		if w.asked(page) != 2 {
			t.Fatal("deleting the group dropped its reviewers")
		}
		if code, out := w.answer(w.r3, page, "approve"); code != 200 {
			t.Fatalf("a reviewer who left the group can no longer answer: %d %s", code, out)
		}
	})
	t.Run("more than fifty under all is refused", func(t *testing.T) {
		big := w.newGroup("Everyone")
		e.sql(`INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
			SELECT gen_random_uuid(), origin_instance_id, 'member', 'active', 'drb-bulk' || g || '@example.com', 'Bulk ' || g
			FROM users, generate_series(1, 51) g WHERE id = $1`, w.r1.id)
		e.sql(`INSERT INTO group_members (group_id, user_id) SELECT $1, id FROM users WHERE email LIKE 'drb-bulk%'`, big)
		e.sql(`INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('group', $1, 'use_product')`, big)
		w.admin.grant(w.space, "group", big, permissions.TemplateViewer)
		page := w.author.page(w.space, w.home, "Big", "x")
		w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("All of them", reviewedDef(review("all", 0, entry("group", big))))})
		out := w.author.want(409, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page)})
		if !strings.Contains(out, "too_many_reviewers") {
			t.Fatal(out)
		}
		// Under "any one", the same group is sensible.
		anyPage := w.reviewedPage(w.workflowOf("Any of them", reviewedDef(review("any", 0, entry("group", big)))), "Big any")
		if w.asked(anyPage) != 51 {
			t.Fatalf("asked %d", w.asked(anyPage))
		}
	})
	t.Run("a quorum above who can review is refused", func(t *testing.T) {
		page := w.author.page(w.space, w.home, "Quorum", "x")
		w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Three of two", reviewedDef(review("quorum", 3, entry("group", w.group))))})
		w.author.want(409, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page)})
	})
}

// TestTheRulesToPassAndOneChangesEndsTheRound is DOC-210 and DOC-211 under every rule.
func TestTheRulesToPassAndOneChangesEndsTheRound(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRC")
	everyone := []map[string]any{entry("user", w.r1.id), entry("user", w.r2.id), entry("user", w.r3.id)}
	cases := []struct {
		rule     string
		quorum   int
		approves int // answers before the round passes
	}{{"all", 0, 3}, {"quorum", 2, 2}, {"any", 0, 1}}
	for _, tc := range cases {
		t.Run(tc.rule+" passes", func(t *testing.T) {
			page := w.reviewedPage(w.workflowOf("Pass "+tc.rule, reviewedDef(review(tc.rule, tc.quorum, everyone...))), "Pass "+tc.rule)
			for i, r := range []*user{w.r1, w.r2, w.r3}[:tc.approves] {
				if w.state(page) != "review" {
					t.Fatalf("passed after %d of %d approvals", i, tc.approves)
				}
				w.answer(r, page, "approve")
			}
			if w.state(page) != "approved" {
				t.Fatalf("not passed after %d approvals", tc.approves)
			}
		})
		t.Run(tc.rule+" ends on changes", func(t *testing.T) {
			page := w.reviewedPage(w.workflowOf("Changes "+tc.rule, reviewedDef(review(tc.rule, tc.quorum, everyone...))), "Changes "+tc.rule)
			if tc.approves > 1 {
				w.answer(w.r1, page, "approve")
			}
			if code, out := w.answer(w.r2, page, "changes"); code != 200 {
				t.Fatalf("%d %s", code, out)
			}
			if w.state(page) != "rejected" {
				t.Fatalf("one changes answer did not end the round: %s", w.state(page))
			}
			if code, _ := w.answer(w.r3, page, "approve"); code == 200 {
				t.Fatal("an answer after the round ended was taken")
			}
			if e.count("SELECT count(*) FROM workflow_rounds WHERE page_id = $1 AND status = 'ended' AND ended_reason = 'changes_asked'", page) != 1 {
				t.Fatal("the round did not end on the request for changes")
			}
		})
	}
	t.Run("a changes answer needs a comment, and changing one's answer is audited", func(t *testing.T) {
		page := w.reviewedPage(w.workflowOf("Mind", reviewedDef(review("all", 0, everyone...))), "Mind")
		w.r1.want(400, "POST", "/api/v1/pages/"+page+"/review/answer", map[string]any{"answer": "changes", "version": w.head(page)})
		w.answer(w.r1, page, "approve")
		w.answer(w.r1, page, "changes")
		if w.state(page) != "rejected" {
			t.Fatal("changing approve to changes did not end the round")
		}
		events, err := audit.List(context.Background(), e.app.Pool(), audit.Filter{Action: "page.review_answered", Limit: 500})
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for _, ev := range events {
			if ev.ObjectID == page && ev.Details["answer"] == "changes" && ev.Details["before"] == "approve" {
				changed++
			}
		}
		if changed != 1 {
			t.Fatal("the audit lost the earlier answer")
		}
	})
}

// TestAnAnswerAndItsStepShareOneTransaction is DOC-213: when the step that follows cannot be
// taken, the answer is not stored either.
func TestAnAnswerAndItsStepShareOneTransaction(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRD")
	empty := w.newGroup("Second stage, empty")
	def := reviewedDef(review("any", 0, entry("user", w.r1.id)))
	states := def["states"].([]any)
	def["states"] = append(states, map[string]any{"id": "second", "name": "Second review", "kind": "in_review",
		"review": review("any", 0, entry("group", empty))})
	steps := def["transitions"].([]any)
	steps[1].(map[string]any)["to"] = "second"
	def["transitions"] = append(steps,
		map[string]any{"id": "final", "name": "Final", "from": "second", "to": "approved", "outcome": "approved"},
		map[string]any{"id": "back", "name": "Back", "from": "second", "to": "draft", "outcome": "changes"})
	page := w.reviewedPage(w.workflowOf("Two stages", def), "Staged")
	if code, out := w.answer(w.r1, page, "approve"); code != 409 || !strings.Contains(out, "reviewers_empty") {
		t.Fatalf("%d %s", code, out)
	}
	if w.state(page) != "review" || e.count("SELECT count(*) FROM workflow_reviewers WHERE user_id = $1 AND answer IS NOT NULL", w.r1.id) != 0 {
		t.Fatal("the answer was stored without its step")
	}
}

// TestIndependence is DOC-220 and DOC-221: no author of the change counts as a reviewer, one
// author or several, unless the workflow itself says so.
func TestIndependence(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRE")
	coauthor := e.user("dre-coauthor@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", coauthor.id, permissions.TemplateEditor)
	writers := w.newGroup("Writers", w.author, coauthor, w.r1)

	t.Run("one author", func(t *testing.T) {
		page := w.reviewedPage(w.workflowOf("Four eyes", reviewedDef(review("any", 0, entry("group", writers)))), "One author")
		if code, _ := w.answer(w.author, page, "approve"); code != 403 {
			t.Fatal("the author approved their own change")
		}
		if w.asked(page) != 2 {
			t.Fatalf("asked %d", w.asked(page))
		}
	})
	t.Run("several authors", func(t *testing.T) {
		page := w.author.page(w.space, w.home, "Two authors", "x")
		w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Four eyes too", reviewedDef(review("any", 0, entry("group", writers))))})
		w.publishAs(coauthor, page, "Two authors", "a colleague's part", false)
		w.step(page, "submit")
		for _, u := range []*user{w.author, coauthor} {
			if code, _ := w.answer(u, page, "approve"); code != 403 {
				t.Fatalf("%s approved a change they wrote part of", u.email)
			}
		}
		if code, _ := w.answer(w.r1, page, "approve"); code != 200 {
			t.Fatal("the independent reviewer could not answer")
		}
	})
	t.Run("only authors named", func(t *testing.T) {
		page := w.author.page(w.space, w.home, "Alone", "x")
		w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Self", reviewedDef(review("any", 0, entry("user", w.author.id))))})
		out := w.author.want(409, "POST", "/api/v1/pages/"+page+"/transitions", map[string]any{"transition": "submit", "version": w.head(page)})
		if !strings.Contains(out, "no_independent_reviewer") {
			t.Fatal(out)
		}
	})
	t.Run("only the workflow switches it off", func(t *testing.T) {
		rv := review("any", 0, entry("group", writers))
		rv["authors_may_review"] = true
		page := w.reviewedPage(w.workflowOf("Self review", reviewedDef(rv)), "Allowed")
		if w.asked(page) != 3 {
			t.Fatalf("asked %d", w.asked(page))
		}
		// No route turns it off for one round: an answer that asks for it is refused.
		other := w.reviewedPage(w.workflowOf("Strict", reviewedDef(review("any", 0, entry("group", writers)))), "Strict")
		w.author.want(400, "POST", "/api/v1/pages/"+other+"/review/answer", map[string]any{"answer": "approve", "version": w.head(other), "authors_may_review": true})
		w.author.want(403, "POST", "/api/v1/pages/"+other+"/review/answer", map[string]any{"answer": "approve", "version": w.head(other)})
	})
}

// TestRemovalNeedsARightAndAReason is DOC-202 and DOC-608.
func TestRemovalNeedsARightAndAReason(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRF")
	all := reviewedDef(review("all", 0, entry("user", w.r1.id), entry("user", w.r2.id), entry("user", w.r3.id)))
	page := w.reviewedPage(w.workflowOf("All three", all), "Removal")
	remove := func(u, who *user, reason string) (int, string) {
		return u.do("POST", "/api/v1/pages/"+page+"/reviewers/"+who.id+"/remove", map[string]any{"reason": reason})
	}
	if code, _ := remove(w.reader, w.r1, "not needed"); code != 404 {
		t.Fatalf("a reader removed a reviewer: %d", code)
	}
	if code, _ := remove(w.author, w.r1, "they object"); code != 403 {
		t.Fatalf("the author removed a reviewer: %d", code)
	}
	if code, _ := remove(w.admin, w.r1, " "); code != 400 {
		t.Fatalf("a removal without a reason: %d", code)
	}
	w.answer(w.r1, page, "approve")
	w.answer(w.r2, page, "approve")
	// The one still to answer cannot review: taking them off passes the round at once.
	if code, out := remove(w.r3, w.r3, "I cannot review this"); code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	if w.state(page) != "approved" {
		t.Fatal("a removal that leaves only approvals did not pass the round")
	}
	e.deliver()
	if e.auditActions("page.reviewer_removed") != 1 {
		t.Fatal("the removal is not audited")
	}
	if notificationCount(e, w.author, notify.TypeReviewerRemoved) != 1 {
		t.Fatal("the author was not told of the removal")
	}

	t.Run("a removal that leaves too few is refused", func(t *testing.T) {
		quorum := w.reviewedPage(w.workflowOf("Two of three", reviewedDef(review("quorum", 2, entry("user", w.r1.id), entry("user", w.r2.id)))), "Few")
		out := w.admin.want(409, "POST", "/api/v1/pages/"+quorum+"/reviewers/"+w.r1.id+"/remove", map[string]any{"reason": "away"})
		if !strings.Contains(out, "round_impossible") {
			t.Fatal(out)
		}
	})
	t.Run("a changes answer survives its reviewer", func(t *testing.T) {
		other := w.reviewedPage(w.workflowOf("All again", all), "Objection")
		w.answer(w.r1, other, "changes")
		w.admin.want(409, "POST", "/api/v1/pages/"+other+"/reviewers/"+w.r1.id+"/remove", map[string]any{"reason": "objected"})
		if w.state(other) != "rejected" {
			t.Fatal("the objection was undone")
		}
	})
}

// TestAddingAReviewer is DOC-202 and DOC-1224: authors add only where the state lets them,
// never a guest, and never an author under independence.
func TestAddingAReviewer(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRG")
	closed := w.reviewedPage(w.workflowOf("Closed", reviewedDef(review("all", 0, entry("user", w.r1.id)))), "Closed")
	add := func(u *user, page, who string) (int, string) {
		return u.do("POST", "/api/v1/pages/"+page+"/reviewers", map[string]any{"user_id": who})
	}
	if code, _ := add(w.author, closed, w.r2.id); code != 403 {
		t.Fatalf("the author added a reviewer where the state does not allow it: %d", code)
	}
	if code, _ := add(w.reader, closed, w.r2.id); code != 404 {
		t.Fatalf("a reader added a reviewer: %d", code)
	}
	rv := review("all", 0, entry("user", w.r1.id))
	rv["authors_may_add"] = true
	open := w.reviewedPage(w.workflowOf("Open", reviewedDef(rv)), "Open")
	if code, out := add(w.author, open, w.guest.id); code != 409 || !strings.Contains(out, "cannot_review") {
		t.Fatalf("a guest was added: %d %s", code, out)
	}
	if code, out := add(w.author, open, w.author.id); code != 409 || !strings.Contains(out, "not_independent") {
		t.Fatalf("the author added themselves: %d %s", code, out)
	}
	if code, out := add(w.author, open, w.r2.id); code != 204 {
		t.Fatalf("%d %s", code, out)
	}
	if w.asked(open) != 2 || e.auditActions("page.reviewer_added") != 1 {
		t.Fatal("the addition is missing or not audited")
	}
	// Whoever looks after the workflows may step in anywhere (DOC-013).
	if code, out := add(w.admin, closed, w.r3.id); code != 204 {
		t.Fatalf("%d %s", code, out)
	}
	// A guest is refused in the workflow itself too (DOC-1224).
	out := w.admin.want(409, "POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": "Guests",
		"definition": reviewedDef(review("any", 0, entry("user", w.guest.id)))})
	if !strings.Contains(out, "guest_reviewer") {
		t.Fatal(out)
	}
}

// TestARestrictedPageOpensToItsReviewersForTheRoundOnly is DOC-203 and DOC-300: a reviewer
// outside a restriction reads the working version while the round runs, and nothing around
// the page; afterwards only the record of the round.
func TestARestrictedPageOpensToItsReviewersForTheRoundOnly(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRH")
	page := w.author.page(w.space, w.home, "Secret procedure", "hidden words")
	w.admin.restrict(page, "view", w.author.id)
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("One", reviewedDef(review("any", 0, entry("user", w.r1.id))))})
	w.r1.want(404, "GET", "/api/v1/pages/"+page, nil)
	w.r1.want(404, "GET", "/api/v1/pages/"+page+"/review", nil)
	w.step(page, "submit")

	got := w.r1.ok("GET", "/api/v1/pages/"+page, nil)
	if got["review_only"] != true || len(got["breadcrumbs"].([]any)) != 0 || got["control"].(map[string]any)["reviewing"] != true {
		t.Fatalf("the reviewer's page: %v %v %v", got["review_only"], got["breadcrumbs"], got["control"])
	}
	panel := w.r1.ok("GET", "/api/v1/pages/"+page+"/review", nil)
	if panel["head"] == nil || panel["can"].(map[string]any)["answer"] != true {
		t.Fatalf("the panel: %v", panel)
	}
	// Being asked opens the page, not the lists or the pages around it.
	seen := w.seenBy(w.r1, page, "hidden")
	if seen["tree"] || seen["search"] || seen["children"] {
		t.Fatalf("the reading spread past the page: %v", seen)
	}
	w.reader.want(404, "GET", "/api/v1/pages/"+page, nil)
	if code, _ := w.answer(w.r1, page, "approve"); code != 200 {
		t.Fatal("the reviewer could not answer")
	}
	w.r1.want(404, "GET", "/api/v1/pages/"+page, nil)
	record := w.r1.ok("GET", "/api/v1/pages/"+page+"/review", nil)
	if record["head"] != nil || record["base"] != nil || len(record["reviewers"].([]any)) != 1 {
		t.Fatalf("after the round the former reviewer keeps more than the record: %v", record)
	}
}

// TestTheCompareBaseIsTheApprovedVersionAndReviewCommentsStayWithTheRound is DOC-250, DOC-251
// and DOC-501.
func TestTheCompareBaseIsTheApprovedVersionAndReviewCommentsStayWithTheRound(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRI")
	page := w.reviewedPage(w.workflowOf("One", reviewedDef(review("any", 0, entry("user", w.r1.id)))), "Compared")
	w.answer(w.r1, page, "approve")
	w.publishAs(w.author, page, "Compared", "second", false)
	w.publishAs(w.author, page, "Compared", "third", false)
	w.step(page, "submit")

	w.reader.want(403, "GET", "/api/v1/pages/"+page+"/review", nil)
	w.reader.want(403, "POST", "/api/v1/pages/"+page+"/review/comments", map[string]any{"body": "my view"})
	w.r1.ok("POST", "/api/v1/pages/"+page+"/review/comments", map[string]any{"body": "reviewnote in the round"})
	panel := w.author.ok("GET", "/api/v1/pages/"+page+"/review", nil)
	base, head := panel["base"].(map[string]any), panel["head"].(map[string]any)
	if base["version"].(float64) != 1 || head["version"].(float64) != 3 {
		t.Fatalf("compared %v with %v, want the approved version 1 with 3", base["version"], head["version"])
	}
	if changes, _ := panel["changes"].([]any); len(changes) == 0 {
		t.Fatal("the panel does not compare the change with the approved version")
	}
	if len(panel["comments"].([]any)) != 1 || panel["needed"].(float64) != 1 || panel["asked"].(float64) != 1 {
		t.Fatalf("the panel: %v", panel)
	}
	w.answer(w.r1, page, "approve")
	w.reader.want(403, "GET", "/api/v1/pages/"+page+"/review", nil)
	if _, out := w.reader.do("GET", "/api/v1/pages/"+page, nil); strings.Contains(out, "reviewnote") {
		t.Fatal("a review comment reached a reader")
	}
	if _, out := w.reader.do("GET", "/api/v1/pages/"+page+"/comments", nil); strings.Contains(out, "reviewnote") {
		t.Fatal("a review comment is among the page's comments")
	}
}

// TestReviewersAndAuthorsAreTold is DOC-600 and DOC-602: the reviewer is asked, also on a page
// they cannot otherwise see; the author hears each answer and the end; a reader hears nothing.
func TestReviewersAndAuthorsAreTold(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRJ")
	page := w.author.page(w.space, w.home, "Told", "x")
	w.admin.restrict(page, "view", w.author.id, w.reader.id)
	w.author.ok("PUT", "/api/v1/pages/"+page+"/workflow", map[string]any{"workflow_id": w.workflowOf("Two", reviewedDef(review("all", 0, entry("user", w.r1.id), entry("user", w.r2.id))))})
	w.step(page, "submit")
	e.deliver()
	for _, r := range []*user{w.r1, w.r2} {
		if notificationCount(e, r, notify.TypeReviewAsked) != 1 {
			t.Fatalf("%s was not asked", r.email)
		}
	}
	w.answer(w.r1, page, "approve")
	w.answer(w.r2, page, "approve")
	e.deliver()
	if notificationCount(e, w.author, notify.TypeReviewAnswered) < 1 || notificationCount(e, w.author, notify.TypeReviewEnded) != 1 {
		t.Fatal("the author was not told")
	}
	for _, kind := range []string{notify.TypeReviewAsked, notify.TypeReviewAnswered, notify.TypeReviewEnded} {
		if notificationCount(e, w.reader, kind) != 0 {
			t.Fatalf("a reader heard %s", kind)
		}
	}
}

// TestTemplatesAreCopiedNotLinked is DOC-130 and DOC-131.
func TestTemplatesAreCopiedNotLinked(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DRK")
	def := reviewedDef(review("any", 0, entry("user", w.r1.id)))
	w.admin.want(403, "POST", "/api/v1/admin/workflow-templates", map[string]any{"name": "Company review", "definition": def})
	tpl := w.root.ok("POST", "/api/v1/admin/workflow-templates", map[string]any{"name": "Company review", "definition": def})
	tplID := tpl["id"].(string)
	w.reader.want(403, "POST", "/api/v1/spaces/"+w.space+"/workflows/from-template", map[string]any{"template_id": tplID})
	copied := w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows/from-template", map[string]any{"template_id": tplID})
	if names, _ := w.admin.ok("GET", "/api/v1/workflows/"+copied["id"].(string), nil)["names"].(map[string]any); names[w.r1.id] != "Test "+w.r1.email {
		t.Fatalf("the editor gets no name for a reviewer: %v", names)
	}
	if _, named := w.reader.ok("GET", "/api/v1/workflows/"+copied["id"].(string), nil)["names"]; named {
		t.Fatal("a reader learns the names of reviewers")
	}
	if copied["template_id"] != tplID || copied["template_version"].(float64) != 1 {
		t.Fatalf("the copy does not say where it came from: %v", copied)
	}
	changed := reviewedDef(review("all", 0, entry("user", w.r1.id), entry("user", w.r2.id)))
	w.root.ok("PUT", "/api/v1/admin/workflow-templates/"+tplID, map[string]any{"name": "Company review", "definition": changed, "base_version": 1})
	again := w.admin.ok("GET", "/api/v1/workflows/"+copied["id"].(string), nil)
	if again["version"].(float64) != 1 || again["template_latest"].(float64) != 2 || fmt.Sprint(again["definition"]) != fmt.Sprint(copied["definition"]) {
		t.Fatalf("a template change moved its copy: %v", again)
	}
	diff := w.admin.ok("GET", "/api/v1/workflows/"+copied["id"].(string)+"/template-changes", nil)
	if len(diff["changes"].([]any)) != 1 {
		t.Fatalf("the comparison: %v", diff)
	}
	w.reader.want(403, "POST", "/api/v1/workflows/"+copied["id"].(string)+"/take-template", map[string]any{"base_version": 1})
	taken := w.admin.ok("POST", "/api/v1/workflows/"+copied["id"].(string)+"/take-template", map[string]any{"base_version": 1})
	if taken["version"].(float64) != 2 || taken["template_version"].(float64) != 2 {
		t.Fatalf("taking the new version: %v", taken)
	}
	list := w.root.ok("GET", "/api/v1/admin/workflow-templates", nil)["templates"].([]any)
	if len(list) != 1 || len(list[0].(map[string]any)["copies"].([]any)) != 1 {
		t.Fatalf("the administrator does not see the copy: %v", list)
	}
	starts := w.reader.ok("GET", "/api/v1/workflow-templates", nil)
	if len(starts["examples"].([]any)) != 2 {
		t.Fatal("the two examples are not offered")
	}
	// Both examples can be saved as they are (DOC-131).
	for _, ex := range starts["examples"].([]any) {
		x := ex.(map[string]any)
		w.admin.ok("POST", "/api/v1/spaces/"+w.space+"/workflows", map[string]any{"name": x["name"], "definition": x["definition"]})
	}
}
