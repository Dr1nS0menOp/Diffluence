// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"errors"
	"testing"
)

// TestASignatureMeaningIsCheckedByTheValidator is the validator test of DOC-721: a meaning
// that is not a key is refused, and so is a signature on a request for changes, each with
// the step it is about.
func TestASignatureMeaningIsCheckedByTheValidator(t *testing.T) {
	cases := map[string]struct {
		change  func(*Definition)
		rule    string
		subject string
	}{
		"a meaning that is not a key": {func(d *Definition) { d.Transitions[1].SignatureMeaning = "Approved!" }, "bad_meaning", "approve"},
		"a meaning too long": {func(d *Definition) {
			d.Transitions[1].SignatureMeaning = "a234567890123456789012345678901234567890x"
		}, "bad_meaning", "approve"},
		"a signed request for changes": {func(d *Definition) { d.Transitions[2].SignatureMeaning = MeaningReviewed }, "signature_on_changes", "reject"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := reviewedFlow()
			tc.change(&d)
			var p *Problem
			if err := Validate(d); !errors.As(err, &p) || p.Rule != tc.rule || p.Subject != tc.subject {
				t.Fatalf("got %v, want %s on %s", err, tc.rule, tc.subject)
			}
		})
	}
}

// A signature can be required on the step an author takes and on the step that follows an
// approved review (DOC-700).
func TestASignatureFitsAnAuthorsStepAndAnApprovedReview(t *testing.T) {
	d := reviewedFlow()
	d.Transitions[0].SignatureMeaning = MeaningReviewed
	d.Transitions[1].SignatureMeaning = MeaningApproved
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
	if got := d.SignatureMeanings(); len(got) != 2 || got[0] != MeaningReviewed || got[1] != MeaningApproved {
		t.Fatalf("meanings: %v", got)
	}
	if ch := Diff(reviewedFlow(), d); len(ch) != 2 || ch[0].Kind != TransitionChanged {
		t.Fatalf("a new signature is a change of its step: %v", ch)
	}
}

func TestEveryBuiltInMeaningHasWordsAndAKey(t *testing.T) {
	list := BuiltInMeanings()
	if len(list) != 4 {
		t.Fatalf("four meanings, got %d", len(list))
	}
	for _, m := range list {
		if !MeaningKeyPattern.MatchString(m.Key) || m.Name == "" || m.Description == "" || !m.BuiltIn {
			t.Errorf("meaning %+v", m)
		}
		if got, ok := BuiltInMeaning(m.Key); !ok || got != m {
			t.Errorf("lookup of %s", m.Key)
		}
	}
	if _, ok := BuiltInMeaning("signed"); ok {
		t.Fatal("an unknown meaning was found")
	}
}

// The two-step example signs its quality approval (DC2-15).
func TestTheTwoStepExampleSignsItsQualityApproval(t *testing.T) {
	for _, ex := range Examples() {
		if ex.Key != "two-step" {
			continue
		}
		tr, ok := ex.Definition.Transition("quality-ok")
		if !ok || tr.SignatureMeaning != MeaningApproved {
			t.Fatalf("quality approval: %+v", tr)
		}
		return
	}
	t.Fatal("no two-step example")
}
