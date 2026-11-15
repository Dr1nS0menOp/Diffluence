// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/permissions"
)

// Document control, wave DC5 of spec 064: delegation, absence and people who leave
// (DOC-240 to DOC-242, DOC-1220). Every way a round changes hands is refused first where it
// would let someone approve who could not before.

func (w reviewWorld) delegate(u *user, page string, body map[string]any) (int, string) {
	w.e.t.Helper()
	return u.do("POST", "/api/v1/pages/"+page+"/review/delegate", body)
}

// TestDelegationNeverWidensWhoMayApprove is DOC-240: a reviewer hands their place only to
// someone the same entry of the state names; never to a reader, a guest, an author of the
// change, or someone already asked. Only someone who looks after the workflows chooses
// another person, and even then only someone who may review. The record keeps the chain.
func TestDelegationNeverWidensWhoMayApprove(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DLA")
	r4 := e.user("dla-r4@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", r4.id, permissions.TemplateViewer)
	// The group holds r2 and r3; r1 is named alone.
	flow := w.workflowOf("Group", reviewedDef(review("any", 0, entry("group", w.group), entry("user", w.r1.id))))
	page := w.reviewedPage(flow, "Delegated")
	to := func(u *user) map[string]any { return map[string]any{"to": u.id, "reason": "away next week"} }

	if code, _ := w.delegate(w.reader, page, to(w.r2)); code != 404 && code != 403 {
		t.Fatalf("a reader delegated: %d", code)
	}
	if code, _ := w.delegate(w.author, page, to(w.r2)); code != 403 {
		t.Fatalf("an author delegated a place they do not hold: %d", code)
	}
	// r1 is named alone: nobody else is named by that entry.
	if code, out := w.delegate(w.r1, page, to(r4)); code != 409 || !strings.Contains(out, "not_a_reviewer_here") {
		t.Fatalf("a reviewer handed their place to someone the state does not name: %d %s", code, out)
	}
	if code, _ := w.delegate(w.r2, page, to(w.guest)); code != 409 {
		t.Fatalf("delegated to a guest: %d", code)
	}
	if code, _ := w.delegate(w.r2, page, to(w.author)); code != 409 {
		t.Fatalf("delegated to the author: %d", code)
	}
	if code, out := w.delegate(w.r2, page, to(w.r3)); code != 409 || !strings.Contains(out, "already_asked") {
		t.Fatalf("delegated to someone already asked: %d %s", code, out)
	}
	if code, _ := w.delegate(w.r2, page, map[string]any{"to": w.r3.id, "reason": " "}); code != 400 {
		t.Fatalf("a delegation without a reason: %d", code)
	}
	// Someone who looks after the workflows may hand r1's place to r4, who views the space.
	if code, out := w.delegate(w.admin, page, map[string]any{"from": w.r1.id, "to": w.guest.id, "reason": "covering"}); code != 409 {
		t.Fatalf("an administrator handed a place to a guest: %d %s", code, out)
	}
	if code, out := w.delegate(w.admin, page, map[string]any{"from": w.r1.id, "to": w.author.id, "reason": "covering"}); code != 409 ||
		!strings.Contains(out, "not_independent") {
		t.Fatalf("an administrator handed a place to the author of the change: %d %s", code, out)
	}
	if code, out := w.delegate(w.admin, page, map[string]any{"from": w.r1.id, "to": r4.id, "reason": "covering"}); code != 200 {
		t.Fatalf("%d %s", code, out)
	}
	// The one who handed their place on answers no more; their delegate does.
	if code, _ := w.answer(w.r1, page, "approve"); code != 403 && code != 404 {
		t.Fatalf("a delegator still answered: %d", code)
	}
	// r4 hands it on again, but only to someone who may review here: the chain stays whole.
	if code, _ := w.delegate(r4, page, to(w.reader)); code != 409 {
		t.Fatalf("a delegate widened the round: %d", code)
	}
	if code, out := w.answer(r4, page, "approve"); code != 200 {
		t.Fatalf("the delegate: %d %s", code, out)
	}
	if w.state(page) != "approved" {
		t.Fatal("the delegate's answer did not count")
	}
	if e.count(`SELECT count(*) FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id
		WHERE r.page_id = $1 AND wr.user_id = $2 AND wr.via = 'delegated' AND wr.via_from = $3 AND wr.via_reason = 'covering'`,
		page, r4.id, w.r1.id) != 1 {
		t.Fatal("the record does not say who delegated to whom and why")
	}
	if e.auditActions("page.review_delegated") != 1 {
		t.Fatal("the delegation is not audited")
	}
	e.deliver()
	if notificationCount(e, r4, notify.TypeReviewAsked) != 1 || notificationCount(e, w.r1, notify.TypeReviewerRemoved) != 1 {
		t.Fatal("both sides of the delegation are not told (DOC-608)")
	}

	t.Run("a group member hands their place to another member of the same group", func(t *testing.T) {
		g := w.newGroup("Alternates of DLA", w.r2, r4)
		all := w.workflowOf("All of a group", reviewedDef(review("all", 0, entry("group", g), entry("user", w.r1.id))))
		other := w.reviewedPage(all, "Within the group")
		if code, out := w.delegate(w.r2, other, to(w.r3)); code != 409 || !strings.Contains(out, "not_a_reviewer_here") {
			t.Fatalf("delegated to someone of another entry: %d %s", code, out)
		}
		w.answer(r4, other, "approve")
		if code, out := w.delegate(r4, other, to(w.r2)); code != 409 {
			t.Fatalf("an answered reviewer delegated: %d %s", code, out)
		}
	})
}

// TestADelegateMustHoldTheRightToSign is DOC-240 with DOC-751: when the step after the review
// is signed, the delegate must be able to sign themselves; a signature is never handed on.
func TestADelegateMustHoldTheRightToSign(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DLB")
	w.r2.enrolTOTP()
	w.r3.enrolTOTP()
	w.root.ok("PUT", "/api/v1/admin/users/"+w.r2.id+"/signing", map[string]any{"identified_by": identity.IdentifiedInPerson})
	flow := signedReviewFlow()
	flow["states"].([]any)[1].(map[string]any)["review"] = review("any", 0, entry("group", w.group))
	page := w.reviewedPage(w.workflowOf("Signed group", flow), "Signed delegation")
	// r3 may not sign: only r2 was enabled. The group names both, so r3 is not asked for a
	// signed approval they cannot give, and r2 cannot hand theirs to r3.
	out := w.r2.want(409, "POST", "/api/v1/pages/"+page+"/review/delegate", map[string]any{"to": w.r3.id, "reason": "holiday"})
	if !strings.Contains(out, "cannot_sign") && !strings.Contains(out, "already_asked") {
		t.Fatal(out)
	}
	// Only someone who looks after the workflows hands on another person's place.
	w.r3.want(403, "POST", "/api/v1/pages/"+page+"/review/delegate", map[string]any{"from": w.r2.id, "to": w.r3.id, "reason": "holiday"})
	if e.count("SELECT count(*) FROM signatures WHERE page_id = $1", page) != 0 {
		t.Fatal("a delegation made a signature")
	}
	t.Run("someone who looks after the workflows cannot hand a signed approval to one who may not sign", func(t *testing.T) {
		r4 := e.user("dlb-r4@example.com", identity.PermUseProduct)
		w.admin.grant(w.space, "user", r4.id, permissions.TemplateViewer)
		out := w.admin.want(409, "POST", "/api/v1/pages/"+page+"/review/delegate", map[string]any{"from": w.r2.id, "to": r4.id, "reason": "cover"})
		if !strings.Contains(out, "cannot_sign") {
			t.Fatal(out)
		}
	})
}

// TestAStandInIsAskedOnlyWhenTheyCouldReview is DOC-241: someone absent is replaced by their
// stand-in only when the same entry names the stand-in; a chain of stand-ins is followed for
// at most three steps, then the escalation target is asked. A stand-in from outside never
// gets the round, and setting an absence for someone else is refused.
func TestAStandInIsAskedOnlyWhenTheyCouldReview(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DLC")
	absent := func(u, standIn *user) {
		u.ok("PUT", "/api/v1/me/absence", map[string]any{"starts_on": "2000-01-01", "ends_on": "2999-12-31", "stand_in": standIn.id})
	}
	w.r1.want(400, "PUT", "/api/v1/me/absence", map[string]any{"starts_on": "2026-02-01", "ends_on": "2026-01-01", "stand_in": w.r2.id})
	w.r1.want(400, "PUT", "/api/v1/me/absence", map[string]any{"starts_on": "2026-01-01", "ends_on": "2026-02-01", "stand_in": w.r1.id})

	// r1 is named alone; their stand-in, the reader, is named by no entry, so r1 stays asked.
	absent(w.r1, w.reader)
	alone := w.reviewedPage(w.workflowOf("Alone", reviewedDef(review("any", 0, entry("user", w.r1.id)))), "Alone")
	if !w.isAsked(alone, w.r1) || w.isAsked(alone, w.reader) {
		t.Fatal("a stand-in from outside the state took the round")
	}

	// In the group of r2 and r3, absent r2 stands in by r3 who is asked anyway.
	g := w.newGroup("Chain of DLC", w.r2, w.r3, w.admin)
	absent(w.r2, w.r3)
	absent(w.r3, w.admin)
	page := w.reviewedPage(w.workflowOf("Group", reviewedDef(review("any", 0, entry("group", g)))), "Chained")
	if w.isAsked(page, w.r2) || w.isAsked(page, w.r3) || !w.isAsked(page, w.admin) {
		t.Fatal("the chain of stand-ins was not followed")
	}
	// The administrator is named by the group too, so they are asked once; the record says
	// whom they stand in for.
	if e.auditActions("page.review_stand_in") != 2 {
		t.Fatal("the round does not record who stood in for whom")
	}

	t.Run("after three steps the escalation target is asked", func(t *testing.T) {
		r4 := e.user("dlc-r4@example.com", identity.PermUseProduct)
		r5 := e.user("dlc-r5@example.com", identity.PermUseProduct)
		r6 := e.user("dlc-r6@example.com", identity.PermUseProduct)
		target := e.user("dlc-target@example.com", identity.PermUseProduct)
		for _, u := range []*user{r4, r5, r6, target} {
			w.admin.grant(w.space, "user", u.id, permissions.TemplateViewer)
		}
		// r1 reaches someone present, r6, only in four steps; the others in three or fewer.
		long := w.newGroup("Long chain of DLC", w.r1, w.r2, r4, r5, r6)
		absent(w.r1, w.r2)
		absent(w.r2, r4)
		absent(r4, r5)
		absent(r5, r6)
		def := dueDef(review("any", 0, entry("user", w.r1.id)), 3, 0, "", entry("user", target.id))
		def["states"].([]any)[1].(map[string]any)["review"].(map[string]any)["reviewers"] = []any{entry("group", long)}
		p := w.reviewedPage(w.workflowOf("Long", def), "Long chain")
		if !w.isAsked(p, target) {
			t.Fatal("the escalation target was not asked at the end of the chain")
		}
		if w.isAsked(p, r5) {
			t.Fatal("the chain went past three steps")
		}
	})
}

func (w reviewWorld) isAsked(page string, u *user) bool {
	w.e.t.Helper()
	return w.e.count(`SELECT count(*) FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id
		WHERE r.page_id = $1 AND r.status = 'open' AND wr.stage = r.review_stage AND wr.user_id = $2 AND wr.removed_at IS NULL`, page, u.id) == 1
}

// TestADisabledReviewerIsDroppedAndTheRoundEscalates is DOC-242 and DOC-1220: a reviewer who
// is disabled cannot answer, is dropped with an audit entry, and a round that can no longer
// pass escalates to its target. A removal by hand that leaves the round unable to pass
// escalates too, where the state names a target (DC2-10).
func TestADisabledReviewerIsDroppedAndTheRoundEscalates(t *testing.T) {
	e := newEnv(t)
	w := newReviewWorld(e, "DLD")
	target := e.user("dld-target@example.com", identity.PermUseProduct)
	w.admin.grant(w.space, "user", target.id, permissions.TemplateViewer)
	def := dueDef(review("quorum", 2, entry("user", w.r1.id), entry("user", w.r2.id)), 5, 0, "", entry("user", target.id))
	page := w.reviewedPage(w.workflowOf("Two of two", def), "Leaving")
	w.root.ok("POST", "/api/v1/admin/users/"+w.r1.id+"/disable", nil)
	if code, _ := w.answer(w.r1, page, "approve"); code == 200 {
		t.Fatal("a disabled reviewer answered")
	}
	e.dueJob()
	if w.isAsked(page, w.r1) {
		t.Fatal("the disabled reviewer is still asked")
	}
	if !w.isAsked(page, target) {
		t.Fatal("a round that could no longer pass did not escalate")
	}
	if e.auditActions("page.reviewer_dropped") != 1 {
		t.Fatal("dropping the reviewer is not audited")
	}

	t.Run("a removal that leaves too few escalates where the state names a target", func(t *testing.T) {
		p := w.reviewedPage(w.workflowOf("Two again", dueDef(review("quorum", 2, entry("user", w.r2.id), entry("user", w.r3.id)), 5, 0, "",
			entry("user", target.id))), "Removal escalates")
		w.admin.ok("POST", "/api/v1/pages/"+p+"/reviewers/"+w.r3.id+"/remove", map[string]any{"reason": "left the team"})
		if !w.isAsked(p, target) || w.isAsked(p, w.r3) {
			t.Fatal("the removal did not escalate")
		}
	})
}
