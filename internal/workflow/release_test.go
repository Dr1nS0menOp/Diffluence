// SPDX-License-Identifier: Apache-2.0

package workflow

import "testing"

// TestOnlyAnApprovedStateReleasesWhenApproved is PUB-710: the mark "release when approved"
// belongs to a state of kind approved, and the validator names the state that carries it
// anywhere else.
func TestOnlyAnApprovedStateReleasesWhenApproved(t *testing.T) {
	for _, kind := range []Kind{KindDraft, KindInReview, KindRejected, KindObsolete} {
		d := reviewThenApprove()
		d.States = append(d.States, State{ID: "other", Name: "Other", Kind: kind, ReleaseWhenApproved: true})
		d.Transitions = append(d.Transitions, Transition{ID: "to-other", Name: "Other", From: "approved", To: "other"},
			Transition{ID: "back", Name: "Back", From: "other", To: "draft"})
		err := Validate(d)
		if err == nil {
			t.Fatalf("a state of kind %s carries the mark", kind)
		}
		if rule := ruleOf(t, err); rule != "release_not_approved" {
			t.Fatalf("kind %s: rule %s", kind, rule)
		}
		if p := err.(*Problem); p.Subject != "other" {
			t.Fatalf("the problem names %q", p.Subject)
		}
	}
	d := reviewThenApprove()
	d.States[2].ReleaseWhenApproved = true
	if err := Validate(d); err != nil {
		t.Fatalf("an approved state with the mark: %v", err)
	}
}

// TestTheReleaseMarksOfTwoVersionsAreCompared is what the save of a workflow asks, so that only
// someone who runs the space turns approval releases on or off (PUB-710); a change of the mark
// is a change of the state in the workflow's history (DOC-112).
func TestTheReleaseMarksOfTwoVersionsAreCompared(t *testing.T) {
	before := reviewThenApprove()
	after := reviewThenApprove()
	if ReleaseMarksDiffer(before, after) {
		t.Fatal("two equal versions differ")
	}
	after.States[2].ReleaseWhenApproved = true
	if !ReleaseMarksDiffer(before, after) || !ReleaseMarksDiffer(after, before) {
		t.Fatal("a mark set or taken away is not a difference")
	}
	if !ReleaseMarksDiffer(Definition{}, after) {
		t.Fatal("a new workflow with a mark is not a difference")
	}
	renamed := reviewThenApprove()
	renamed.States[2].ReleaseWhenApproved = true
	renamed.States[2].Name = "Effective"
	if ReleaseMarksDiffer(after, renamed) {
		t.Fatal("a rename changed the marks")
	}
	changes := Diff(before, after)
	if len(changes) != 1 || changes[0].Kind != StateChanged || changes[0].ID != "approved" {
		t.Fatalf("the history of a new mark: %v", changes)
	}
}
