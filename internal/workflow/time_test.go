// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"testing"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/conditions"
)

// timedFlow is reviewedFlow with a due period, an overdue step back to draft, a timed approved
// state that asks a group to confirm reading, and a periodic review step into review.
func timedFlow() Definition {
	d := reviewedFlow()
	d.States[1].Review.Due = &Due{WorkingDays: 5, RemindBefore: 1,
		Escalate: []Reviewer{{Type: ReviewerGroup, ID: uuid.MustParse("01900000-0000-7000-8000-000000000009")}}}
	d.States[2].Timed, d.States[2].ReviewMonths = true, 12
	d.States[2].ReadAndUnderstood = &ReadAndUnderstood{Groups: []uuid.UUID{uuid.MustParse("01900000-0000-7000-8000-000000000008")}}
	d.Transitions = append(d.Transitions,
		Transition{ID: "late", Name: "Too late", From: "review", To: "draft", Outcome: OutcomeOverdue},
		Transition{ID: "periodic", Name: "Periodic review", From: "approved", To: "review", Outcome: OutcomeReviewDue})
	return d
}

func TestATimedWorkflowPasses(t *testing.T) {
	if err := Validate(timedFlow()); err != nil {
		t.Fatal(err)
	}
}

// TestEachRuleOfTimeIsChecked is the table test of DOC-105, DOC-230, DOC-231, DOC-800, DOC-802
// and DOC-820: one broken rule per case. A time-out into approval comes first, as the rule
// that may never break.
func TestEachRuleOfTimeIsChecked(t *testing.T) {
	cases := map[string]struct {
		change  func(*Definition)
		rule    string
		subject string
	}{
		"a time-out that approves":      {func(d *Definition) { d.Transitions[4].To = "approved" }, "timeout_never_approves", "late"},
		"a time-out without a due date": {func(d *Definition) { d.States[1].Review.Due = nil }, "overdue_without_due", "late"},
		"a due period of none":          {func(d *Definition) { d.States[1].Review.Due.WorkingDays = 0 }, "bad_due", "review"},
		"a due period too long":         {func(d *Definition) { d.States[1].Review.Due.WorkingDays = MaxDueDays + 1 }, "bad_due", "review"},
		"a reminder after the date":     {func(d *Definition) { d.States[1].Review.Due.RemindBefore = 5 }, "bad_due", "review"},
		"a reminder before nothing":     {func(d *Definition) { d.States[1].Review.Due.RemindBefore = -1 }, "bad_due", "review"},
		"an escalation to nobody": {func(d *Definition) {
			d.States[1].Review.Due.Escalate = append(d.States[1].Review.Due.Escalate, Reviewer{Type: ReviewerUser})
		}, "bad_escalation", "review"},
		"an escalation named twice": {func(d *Definition) {
			d.States[1].Review.Due.Escalate = append(d.States[1].Review.Due.Escalate, d.States[1].Review.Due.Escalate[0])
		}, "bad_escalation", "review"},
		"two time-outs": {func(d *Definition) {
			d.Transitions = append(d.Transitions, Transition{ID: "late2", Name: "Late again", From: "review", To: "rejected", Outcome: OutcomeOverdue})
		}, "timed_outcomes", "review"},
		"a period on a final state":    {func(d *Definition) { d.States[2].Timed = false }, "bad_review_period", "approved"},
		"a period too long":            {func(d *Definition) { d.States[2].ReviewMonths = MaxReviewMonths + 1 }, "bad_review_period", "approved"},
		"a review date on a final one": {func(d *Definition) { d.States[2].Timed, d.States[2].ReviewMonths = false, 0 }, "review_due_not_timed", "periodic"},
		"a review date into draft":     {func(d *Definition) { d.Transitions[5].To = "draft" }, "review_due_target", "periodic"},
		"reading asked of a draft": {func(d *Definition) {
			d.States[0].ReadAndUnderstood = d.States[2].ReadAndUnderstood
		}, "acknowledge_not_approved", "draft"},
		"reading asked of nobody": {func(d *Definition) { d.States[2].ReadAndUnderstood.Groups = nil }, "bad_acknowledge", "approved"},
		"reading asked of a group twice": {func(d *Definition) {
			g := d.States[2].ReadAndUnderstood.Groups
			d.States[2].ReadAndUnderstood.Groups = append(g, g[0])
		}, "bad_acknowledge", "approved"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := timedFlow()
			c.change(&d)
			err := Validate(d)
			if err == nil {
				t.Fatal("accepted")
			}
			if got := ruleOf(t, err); got != c.rule {
				t.Fatalf("rule %q, want %q (%v)", got, c.rule, err)
			}
			if got := err.(*Problem).Subject; got != c.subject {
				t.Fatalf("subject %q, want %q", got, c.subject)
			}
		})
	}
}

func TestATimedStateWithoutAPeriodTakesTheDefault(t *testing.T) {
	if got := (State{Kind: KindApproved, Timed: true}).Months(); got != DefaultReviewMonths {
		t.Fatalf("%d months", got)
	}
	if got := (State{Kind: KindApproved, Timed: true, ReviewMonths: 3}).Months(); got != 3 {
		t.Fatalf("%d months", got)
	}
}

// TestEachConditionKindDecidesAndFailsClosed is DOC-120: a table per kind, and a label that no
// longer exists is false for both of its tests, with its own reason.
func TestEachConditionKindDecidesAndFailsClosed(t *testing.T) {
	sop := "sop"
	step := func(match conditions.Match, tests ...Test) Transition {
		return Transition{ID: "x", Name: "X", Conditions: &Conditions{Match: match, Tests: tests}}
	}
	labelled := Facts{Labels: map[string]bool{sop: true}, Known: map[string]bool{sop: true}, Changed: map[string]bool{PartBody: true}, Attachments: 1}
	plain := Facts{Labels: map[string]bool{}, Known: map[string]bool{sop: true}, Changed: map[string]bool{}}
	gone := Facts{Labels: map[string]bool{}, Known: map[string]bool{}, Changed: map[string]bool{}}
	cases := map[string]struct {
		step   Transition
		facts  Facts
		ok     bool
		reason string
	}{
		"a removed label fails present":       {step(conditions.All, Test{Kind: TestLabelPresent, Label: sop}), gone, false, ReasonLabelGone},
		"a removed label fails absent too":    {step(conditions.All, Test{Kind: TestLabelAbsent, Label: sop}), gone, false, ReasonLabelGone},
		"a missing label":                     {step(conditions.All, Test{Kind: TestLabelPresent, Label: sop}), plain, false, ReasonLabelMissing},
		"a label that must be absent":         {step(conditions.All, Test{Kind: TestLabelAbsent, Label: sop}), labelled, false, ReasonLabelPresent},
		"an unchanged title":                  {step(conditions.All, Test{Kind: TestChanged, Part: PartTitle}), labelled, false, ReasonUnchanged},
		"no file":                             {step(conditions.All, Test{Kind: TestHasAttachment}), plain, false, ReasonNoAttachment},
		"none of any":                         {step(conditions.Any, Test{Kind: TestHasAttachment}, Test{Kind: TestChanged, Part: PartFiles}), plain, false, ReasonNoneHolds},
		"an unknown test":                     {step(conditions.All, Test{Kind: "weather"}), labelled, false, ReasonBadConditions},
		"no test at all":                      {step(conditions.All), labelled, false, ReasonBadConditions},
		"an unknown match":                    {step("most", Test{Kind: TestHasAttachment}), labelled, false, ReasonBadConditions},
		"a present label":                     {step(conditions.All, Test{Kind: TestLabelPresent, Label: sop}), labelled, true, ""},
		"an absent label":                     {step(conditions.All, Test{Kind: TestLabelAbsent, Label: sop}), plain, true, ""},
		"a changed body and a file":           {step(conditions.All, Test{Kind: TestChanged, Part: PartBody}, Test{Kind: TestHasAttachment}), labelled, true, ""},
		"one of any":                          {step(conditions.Any, Test{Kind: TestChanged, Part: PartTitle}, Test{Kind: TestHasAttachment}), labelled, true, ""},
		"a step without conditions is always": {Transition{ID: "y", Name: "Y"}, gone, true, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ok, reason := c.step.Allows(c.facts)
			if ok != c.ok || reason != c.reason {
				t.Fatalf("got %v %q, want %v %q", ok, reason, c.ok, c.reason)
			}
		})
	}
}

func TestConditionsAreCheckedWhenSaved(t *testing.T) {
	cases := map[string]*Conditions{
		"no test":                {Match: conditions.All},
		"too many":               {Match: conditions.Any, Tests: make([]Test, conditions.MaxTests+1)},
		"unknown match":          {Match: "most", Tests: []Test{{Kind: TestHasAttachment}}},
		"a label without its id": {Match: conditions.All, Tests: []Test{{Kind: TestLabelPresent}}},
		"an unknown part":        {Match: conditions.All, Tests: []Test{{Kind: TestChanged, Part: "colour"}}},
		"an unknown kind":        {Match: conditions.All, Tests: []Test{{Kind: "weather"}}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			d := reviewThenApprove()
			d.Transitions[0].Conditions = c
			if got := ruleOf(t, Validate(d)); got != "bad_conditions" {
				t.Fatal(got)
			}
		})
	}
	t.Run("a step of the system has none", func(t *testing.T) {
		d := reviewedFlow()
		d.Transitions[1].Conditions = &Conditions{Match: conditions.All, Tests: []Test{{Kind: TestHasAttachment}}}
		if got := ruleOf(t, Validate(d)); got != "conditions_on_outcome" {
			t.Fatal(got)
		}
	})
	t.Run("a good one passes and shows in the history", func(t *testing.T) {
		d := reviewThenApprove()
		d.Transitions[0].Conditions = &Conditions{Match: conditions.All, Tests: []Test{{Kind: TestChanged, Part: PartBody}}}
		if err := Validate(d); err != nil {
			t.Fatal(err)
		}
		changes := Diff(reviewThenApprove(), d)
		if len(changes) != 1 || changes[0].Kind != TransitionChanged {
			t.Fatalf("%v", changes)
		}
	})
}
