// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"testing"

	"github.com/google/uuid"
)

// reviewedFlow is reviewThenApprove with reviewers on its review state: the round passes
// into approved and a "changes" answer sends the page to rejected (DOC-200, DOC-211).
func reviewedFlow() Definition {
	d := reviewThenApprove()
	d.States[1].Review = &Review{Reviewers: []Reviewer{{Type: ReviewerUser, ID: uuid.MustParse("01900000-0000-7000-8000-000000000001")}}, Rule: RuleAll}
	d.Transitions[1].Outcome = OutcomeApproved
	d.Transitions[2].Outcome = OutcomeChanges
	return d
}

func TestAReviewedWorkflowPasses(t *testing.T) {
	if err := Validate(reviewedFlow()); err != nil {
		t.Fatal(err)
	}
}

// TestEachRuleOfAReviewIsChecked is the table test of the review rules of DOC-104, DOC-200,
// DOC-210 and DOC-211: one broken rule per case, and the problem names it and its state.
func TestEachRuleOfAReviewIsChecked(t *testing.T) {
	someone := uuid.MustParse("01900000-0000-7000-8000-000000000002")
	cases := map[string]struct {
		change  func(*Definition)
		rule    string
		subject string
	}{
		"reviewers on a draft": {func(d *Definition) { d.States[0].Review = &Review{Rule: RuleAny} }, "review_not_in_review", "draft"},
		"unknown rule":         {func(d *Definition) { d.States[1].Review.Rule = "most" }, "bad_rule", "review"},
		"quorum of none":       {func(d *Definition) { d.States[1].Review.Rule = RuleQuorum }, "bad_quorum", "review"},
		"quorum above the limit": {func(d *Definition) {
			d.States[1].Review.Rule, d.States[1].Review.Quorum = RuleQuorum, MaxReviewersPerRound+1
		}, "bad_quorum", "review"},
		"a quorum on another rule": {func(d *Definition) { d.States[1].Review.Quorum = 2 }, "bad_quorum", "review"},
		"unknown kind of reviewer": {func(d *Definition) { d.States[1].Review.Reviewers[0].Type = "team" }, "bad_reviewer", "review"},
		"reviewer without id":      {func(d *Definition) { d.States[1].Review.Reviewers[0].ID = uuid.Nil }, "bad_reviewer", "review"},
		"the same reviewer twice": {func(d *Definition) {
			d.States[1].Review.Reviewers = append(d.States[1].Review.Reviewers, d.States[1].Review.Reviewers[0])
		}, "bad_reviewer", "review"},
		"too many entries": {func(d *Definition) {
			for range MaxReviewerEntries {
				d.States[1].Review.Reviewers = append(d.States[1].Review.Reviewers, Reviewer{Type: ReviewerGroup, ID: uuid.New()})
			}
		}, "too_many_reviewers", "review"},
		"unknown outcome":           {func(d *Definition) { d.Transitions[1].Outcome = "maybe" }, "bad_outcome", "approve"},
		"an outcome without review": {func(d *Definition) { d.Transitions[0].Outcome = OutcomeApproved }, "outcome_without_review", "submit"},
		"no approving outcome": {func(d *Definition) {
			d.Transitions = append(d.Transitions[:1:1], d.Transitions[2:]...)
		}, "review_outcomes", "review"},
		"no changes outcome": {func(d *Definition) { d.Transitions[2].Outcome = "" }, "review_outcomes", "review"},
		"two approving outcomes": {func(d *Definition) {
			d.Transitions = append(d.Transitions, Transition{ID: "approve2", Name: "Approve too", From: "review", To: "approved", Outcome: OutcomeApproved})
		}, "review_outcomes", "review"},
		"changes into approval": {func(d *Definition) {
			d.Transitions[2].To = "approved"
			d.Transitions[2].RequiresComment = false
		}, "changes_target", "reject"},
		"approval into rejection": {func(d *Definition) {
			d.Transitions[1].To, d.Transitions[2].To = "rejected", "draft"
			d.Transitions = append(d.Transitions, Transition{ID: "ok", Name: "Accept", From: "rejected", To: "approved"})
		}, "approval_target", "approve"},
		// An author may leave a review state, never jump from it into approval (DOC-013).
		"authors approve around the reviewers": {func(d *Definition) {
			d.Transitions = append(d.Transitions, Transition{ID: "self", Name: "Approve myself", From: "review", To: "approved"})
		}, "approval_needs_review", "self"},
		"a guest kind of reviewer": {func(d *Definition) {
			d.States[1].Review.Reviewers = []Reviewer{{Type: "guest", ID: someone}}
		}, "bad_reviewer", "review"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := reviewedFlow()
			tc.change(&d)
			err := Validate(d)
			if err == nil {
				t.Fatal("refused nothing")
			}
			if got := ruleOf(t, err); got != tc.rule {
				t.Fatalf("rule %q, want %q (%v)", got, tc.rule, err)
			}
			if p := err.(*Problem); p.Subject != tc.subject {
				t.Fatalf("subject %q, want %q", p.Subject, tc.subject)
			}
		})
	}
}

// TestAReviewStateWithNobodyNamedIsValid: the workflow may be saved before its reviewers are
// known, as the examples are; a round that would start with nobody is refused (DOC-200).
func TestAReviewStateWithNobodyNamedIsValid(t *testing.T) {
	d := reviewedFlow()
	d.States[1].Review.Reviewers = nil
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
}

// TestBothExamplesPassTheValidator is DOC-131.
func TestBothExamplesPassTheValidator(t *testing.T) {
	list := Examples()
	if len(list) != 2 {
		t.Fatalf("%d examples", len(list))
	}
	for _, ex := range list {
		if err := Validate(ex.Definition); err != nil {
			t.Fatalf("%s: %v", ex.Key, err)
		}
		reviewed := 0
		for _, s := range ex.Definition.States {
			if s.Review != nil {
				reviewed++
				if s.Review.AuthorsMayReview {
					t.Fatalf("%s: %s is not independent", ex.Key, s.ID)
				}
			}
		}
		if reviewed == 0 {
			t.Fatalf("%s has no review state", ex.Key)
		}
	}
	if Examples()[0].Definition.States[1].Review == list[0].Definition.States[1].Review {
		t.Fatal("examples share their reviews, so a change to one copy changes the other")
	}
}

// TestPassingCountsByTheRule is DOC-210 and DOC-211 on the pure count.
func TestPassingCountsByTheRule(t *testing.T) {
	cases := []struct {
		name                     string
		review                   Review
		asked, approved, changes int
		want                     Outcome
	}{
		{"all, one missing", Review{Rule: RuleAll}, 3, 2, 0, OutcomePending},
		{"all, every one", Review{Rule: RuleAll}, 3, 3, 0, OutcomeApproved},
		{"quorum reached", Review{Rule: RuleQuorum, Quorum: 2}, 3, 2, 0, OutcomeApproved},
		{"quorum short", Review{Rule: RuleQuorum, Quorum: 2}, 3, 1, 0, OutcomePending},
		{"any, one", Review{Rule: RuleAny}, 5, 1, 0, OutcomeApproved},
		{"any, none", Review{Rule: RuleAny}, 5, 0, 0, OutcomePending},
		{"all, nobody asked", Review{Rule: RuleAll}, 0, 0, 0, OutcomePending},
		// One "changes" ends the round under every rule (DOC-1402).
		{"all with changes", Review{Rule: RuleAll}, 3, 2, 1, OutcomeChanges},
		{"quorum reached and changes", Review{Rule: RuleQuorum, Quorum: 2}, 4, 3, 1, OutcomeChanges},
		{"any with changes", Review{Rule: RuleAny}, 3, 1, 1, OutcomeChanges},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.review.Count(tc.asked, tc.approved, tc.changes); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNeededSaysHowManyMustApprove is the count the panel shows ("2 of 3 needed").
func TestNeededSaysHowManyMustApprove(t *testing.T) {
	for _, tc := range []struct {
		review Review
		asked  int
		want   int
	}{{Review{Rule: RuleAll}, 4, 4}, {Review{Rule: RuleQuorum, Quorum: 3}, 5, 3}, {Review{Rule: RuleAny}, 9, 1}} {
		if got := tc.review.Needed(tc.asked); got != tc.want {
			t.Fatalf("%s: %d, want %d", tc.review.Rule, got, tc.want)
		}
	}
}

// TestAChangeOfReviewersIsNamedInTheHistory is DOC-112 and DOC-221: switching independence
// off is a change of the workflow, listed in its history.
func TestAChangeOfReviewersIsNamedInTheHistory(t *testing.T) {
	before := reviewedFlow()
	after := reviewedFlow()
	after.States[1].Review.AuthorsMayReview = true
	changes := Diff(before, after)
	if len(changes) != 1 || changes[0].Kind != ReviewChanged || changes[0].ID != "review" {
		t.Fatalf("changes: %+v", changes)
	}
	if got := Diff(reviewedFlow(), reviewedFlow()); len(got) != 0 {
		t.Fatalf("an equal workflow changed: %+v", got)
	}
}
