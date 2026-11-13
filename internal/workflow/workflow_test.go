// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
)

// reviewThenApprove is the smallest useful workflow: draft, in review, approved, and back.
func reviewThenApprove() Definition {
	return Definition{
		States: []State{
			{ID: "draft", Name: "Draft", Kind: KindDraft, Start: true},
			{ID: "review", Name: "In review", Kind: KindInReview},
			{ID: "approved", Name: "Approved", Kind: KindApproved},
			{ID: "rejected", Name: "Rejected", Kind: KindRejected},
		},
		Transitions: []Transition{
			{ID: "submit", Name: "Send for review", From: "draft", To: "review"},
			{ID: "approve", Name: "Approve", From: "review", To: "approved"},
			{ID: "reject", Name: "Reject", From: "review", To: "rejected", RequiresComment: true},
			{ID: "rework", Name: "Rework", From: "rejected", To: "draft"},
		},
	}
}

func ruleOf(t *testing.T, err error) string {
	t.Helper()
	var p *Problem
	if !errors.As(err, &p) {
		t.Fatalf("not a problem: %v", err)
	}
	return p.Rule
}

func TestAValidWorkflowPasses(t *testing.T) {
	if err := Validate(reviewThenApprove()); err != nil {
		t.Fatal(err)
	}
}

// TestEachRuleOfAWorkflowIsChecked is the table test of DOC-100 to DOC-104: one broken
// rule per case, and the problem names that rule.
func TestEachRuleOfAWorkflowIsChecked(t *testing.T) {
	cases := map[string]struct {
		change func(*Definition)
		rule   string
	}{
		"no states":            {func(d *Definition) { d.States = nil; d.Transitions = nil }, "no_states"},
		"no start":             {func(d *Definition) { d.States[0].Start = false }, "one_start"},
		"two starts":           {func(d *Definition) { d.States[3].Start = true; d.States[3].Kind = KindDraft }, "one_start"},
		"start not draft":      {func(d *Definition) { d.States[0].Kind = KindInReview }, "start_not_draft"},
		"no approved state":    {func(d *Definition) { d.States[2].Kind = KindRejected }, "no_approved"},
		"duplicate state id":   {func(d *Definition) { d.States[1].ID = "draft" }, "duplicate_state"},
		"bad state id":         {func(d *Definition) { d.States[1].ID = "In Review" }, "bad_id"},
		"state without name":   {func(d *Definition) { d.States[1].Name = "  " }, "name_missing"},
		"same state name":      {func(d *Definition) { d.States[1].Name = "draft" }, "duplicate_name"},
		"unknown kind":         {func(d *Definition) { d.States[1].Kind = "done" }, "bad_kind"},
		"timed draft":          {func(d *Definition) { d.States[1].Timed = true }, "timed_not_approved"},
		"transition to self":   {func(d *Definition) { d.Transitions[0].To = "draft" }, "same_state"},
		"unknown target":       {func(d *Definition) { d.Transitions[0].To = "nowhere" }, "unknown_state"},
		"unknown source":       {func(d *Definition) { d.Transitions[0].From = "nowhere" }, "unknown_state"},
		"duplicate transition": {func(d *Definition) { d.Transitions[1].ID = "submit" }, "duplicate_transition"},
		"transition no name":   {func(d *Definition) { d.Transitions[1].Name = "" }, "name_missing"},
		"two same-named steps": {func(d *Definition) {
			d.Transitions = append(d.Transitions, Transition{ID: "approve2", Name: "approve", From: "review", To: "approved"})
		}, "duplicate_name"},
		"unreachable state": {func(d *Definition) {
			d.States = append(d.States, State{ID: "island", Name: "Island", Kind: KindApproved})
		}, "unreachable"},
		"rejected is a dead end": {func(d *Definition) { d.Transitions = d.Transitions[:3] }, "no_way_to_approval"},
		"too long a name":        {func(d *Definition) { d.States[1].Name = strings.Repeat("x", 81) }, "text_too_long"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := reviewThenApprove()
			tc.change(&d)
			err := Validate(d)
			if err == nil {
				t.Fatal("refused nothing")
			}
			if got := ruleOf(t, err); got != tc.rule {
				t.Fatalf("rule %q, want %q (%v)", got, tc.rule, err)
			}
		})
	}
}

// TestTwoTransitionsBetweenTheSameStatesMayDifferByName: DOC-104 allows "approve" and
// "approve with signature" side by side.
func TestTwoTransitionsBetweenTheSameStatesMayDifferByName(t *testing.T) {
	d := reviewThenApprove()
	d.Transitions = append(d.Transitions, Transition{ID: "approve-signed", Name: "Approve with signature", From: "review", To: "approved"})
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
}

// TestAnObsoleteStateMayBeADeadEnd and a cycle back to draft is allowed (DOC-102).
func TestAnObsoleteStateMayBeADeadEnd(t *testing.T) {
	d := reviewThenApprove()
	d.States = append(d.States, State{ID: "withdrawn", Name: "Withdrawn", Kind: KindObsolete})
	d.Transitions = append(d.Transitions, Transition{ID: "withdraw", Name: "Withdraw", From: "approved", To: "withdrawn"},
		Transition{ID: "change", Name: "Change", From: "approved", To: "draft"})
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
}

// pathToApproval is written apart from Validate, as plain repeated relaxation, so the
// fuzz test does not check the validator against itself.
func pathToApproval(d Definition) bool {
	kind := map[string]Kind{}
	for _, s := range d.States {
		kind[s.ID] = s.Kind
	}
	good := map[string]bool{}
	for id, k := range kind {
		if k == KindApproved {
			good[id] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, tr := range d.Transitions {
			if good[tr.To] && !good[tr.From] {
				good[tr.From] = true
				changed = true
			}
		}
	}
	for id, k := range kind {
		if k != KindApproved && k != KindObsolete && !good[id] {
			return false
		}
	}
	return good[d.StartState().ID]
}

func randomDefinition(r *rand.Rand) Definition {
	n := 1 + r.IntN(7)
	var d Definition
	for i := range n {
		s := State{ID: fmt.Sprintf("s%d", i), Name: fmt.Sprintf("State %d", i), Kind: Kinds[r.IntN(len(Kinds))]}
		if i == 0 {
			s.Start, s.Kind = true, KindDraft
		}
		d.States = append(d.States, s)
	}
	for i := range r.IntN(12) {
		from, to := r.IntN(n), r.IntN(n)
		d.Transitions = append(d.Transitions, Transition{ID: fmt.Sprintf("t%d", i), Name: fmt.Sprintf("Step %d", i),
			From: fmt.Sprintf("s%d", from), To: fmt.Sprintf("s%d", to)})
	}
	return d
}

// TestNoAcceptedWorkflowLacksAWayToApproval is the property of DOC-102 over random graphs:
// whatever Validate accepts can reach approval from the start and from every open state.
func TestNoAcceptedWorkflowLacksAWayToApproval(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	accepted := 0
	for range 20000 {
		d := randomDefinition(r)
		if Validate(d) != nil {
			continue
		}
		accepted++
		if !pathToApproval(d) {
			t.Fatalf("accepted a workflow with no way to approval: %+v", d)
		}
	}
	if accepted < 100 {
		t.Fatalf("only %d random workflows were accepted: the generator says too little", accepted)
	}
}

func FuzzValidateNeverAcceptsAWorkflowWithoutAWayToApproval(f *testing.F) {
	f.Add(uint64(1), uint64(2))
	f.Add(uint64(42), uint64(7))
	f.Fuzz(func(t *testing.T, a, b uint64) {
		d := randomDefinition(rand.New(rand.NewPCG(a, b)))
		if Validate(d) == nil && !pathToApproval(d) {
			t.Fatalf("accepted a workflow with no way to approval: %+v", d)
		}
	})
}

// TestDiffNamesEachKindOfChange is the change list of DOC-112.
func TestDiffNamesEachKindOfChange(t *testing.T) {
	before := reviewThenApprove()
	after := reviewThenApprove()
	after.States[1].Name = "Under review"
	after.States[3].Description = "Sent back"
	after.States = append(after.States, State{ID: "effective", Name: "Effective", Kind: KindApproved})
	after.Transitions[1].RequiresComment = true
	after.Transitions = append(after.Transitions[:3], Transition{ID: "publish", Name: "Make effective", From: "approved", To: "effective"})
	got := map[string]string{}
	for _, c := range Diff(before, after) {
		got[c.Kind+":"+c.ID] = c.To
	}
	for _, want := range []string{StateRenamed + ":review", StateChanged + ":rejected", StateAdded + ":effective",
		TransitionChanged + ":approve", TransitionRemoved + ":rework", TransitionAdded + ":publish"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if got[StateRenamed+":review"] != "Under review" {
		t.Fatalf("the rename lost its new name: %v", got)
	}
	if len(Diff(before, before)) != 0 {
		t.Fatal("a workflow differs from itself")
	}
}

func TestTheHelpersFindStatesAndTransitions(t *testing.T) {
	d := reviewThenApprove()
	if d.StartState().ID != "draft" || d.FirstApproved().ID != "approved" {
		t.Fatalf("start %q, first approved %q", d.StartState().ID, d.FirstApproved().ID)
	}
	if from := d.From("review"); len(from) != 2 || from[0].ID != "approve" {
		t.Fatalf("from review: %+v", from)
	}
	if _, ok := d.State("missing"); ok {
		t.Fatal("found a state that is not there")
	}
	if _, ok := d.Transition("approve"); !ok {
		t.Fatal("did not find a transition that is there")
	}
}
