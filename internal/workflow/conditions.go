// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"github.com/diffluence/diffluence/internal/conditions"
)

// Conditions on a step (DOC-120, spec 064 wave DC5): simple tests picked from a list, joined
// by "all of" or "any of", with no scripting (DOC-121, DOC-1404). They share their joining and
// their bounds with the conditions of guided templates (internal/conditions, TPL-433).

// The kinds of test a step can ask.
const (
	// TestLabelPresent and TestLabelAbsent look for a label. A label the space no longer
	// allows, under a fixed list of labels, fails both: closed, not open.
	TestLabelPresent = "label_present"
	TestLabelAbsent  = "label_absent"
	// TestChanged asks that the change since the approved version touches a part.
	TestChanged = "changed"
	// TestHasAttachment asks for at least one file on the page.
	TestHasAttachment = "has_attachment"
)

// The parts of a page a change can touch.
const (
	PartTitle = "title"
	PartBody  = "body"
	PartFiles = "files"
)

// Why a step cannot be taken, for the button that says so.
const (
	ReasonLabelMissing  = "label_missing"
	ReasonLabelPresent  = "label_present"
	ReasonLabelGone     = "label_gone"
	ReasonUnchanged     = "unchanged"
	ReasonNoAttachment  = "no_attachment"
	ReasonNoneHolds     = "none_holds"
	ReasonBadConditions = "bad_conditions"
)

// Test is one test of a step's conditions.
type Test struct {
	Kind  string `json:"kind"`
	Label string `json:"label,omitempty"`
	Part  string `json:"part,omitempty"`
}

// Conditions are the tests a step asks before it can be taken.
type Conditions struct {
	Match conditions.Match `json:"match"`
	Tests []Test           `json:"tests"`
}

// Facts are what the tests of a page look at, read once for every step of the band.
type Facts struct {
	// Labels are the labels of the page; Known says which of the labels the tests name still
	// exist in the instance.
	Labels map[string]bool
	Known  map[string]bool
	// Changed says which parts differ from the approved version; a page never approved has
	// every part changed.
	Changed     map[string]bool
	Attachments int
}

// LabelsNamed lists the labels the conditions of a definition name, so a caller reads only
// those.
func (d Definition) LabelsNamed() []string {
	var out []string
	for _, tr := range d.Transitions {
		if tr.Conditions == nil {
			continue
		}
		for _, t := range tr.Conditions.Tests {
			if t.Label != "" {
				out = append(out, t.Label)
			}
		}
	}
	return out
}

// HasConditions says whether any step of a definition asks for a test.
func (d Definition) HasConditions() bool {
	for _, tr := range d.Transitions {
		if tr.Conditions != nil {
			return true
		}
	}
	return false
}

// Allows says whether the facts of a page let a step be taken, and if not, why. A step
// without conditions is always allowed. A malformed condition is false, never true.
func (tr Transition) Allows(f Facts) (bool, string) {
	c := tr.Conditions
	if c == nil {
		return true, ""
	}
	reasons := make([]string, len(c.Tests))
	ok := conditions.Combine(c.Match, len(c.Tests), func(i int) bool {
		held, reason := c.Tests[i].holds(f)
		reasons[i] = reason
		return held
	})
	switch {
	case ok:
		return true, ""
	case len(c.Tests) == 0 || len(c.Tests) > conditions.MaxTests:
		return false, ReasonBadConditions
	case c.Match == conditions.Any:
		return false, ReasonNoneHolds
	}
	// Under "all of" the first test that fails says why; Combine stops there.
	for _, r := range reasons {
		if r != "" {
			return false, r
		}
	}
	return false, ReasonBadConditions
}

func (t Test) holds(f Facts) (bool, string) {
	switch t.Kind {
	case TestLabelPresent, TestLabelAbsent:
		if !f.Known[t.Label] {
			return false, ReasonLabelGone
		}
		has := f.Labels[t.Label]
		if t.Kind == TestLabelPresent && !has {
			return false, ReasonLabelMissing
		}
		if t.Kind == TestLabelAbsent && has {
			return false, ReasonLabelPresent
		}
		return true, ""
	case TestChanged:
		if !f.Changed[t.Part] {
			return false, ReasonUnchanged
		}
		return true, ""
	case TestHasAttachment:
		if f.Attachments == 0 {
			return false, ReasonNoAttachment
		}
		return true, ""
	}
	return false, ReasonBadConditions
}

// checkConditions checks the conditions of one step when it is saved.
func checkConditions(tr Transition) *Problem {
	c := tr.Conditions
	if c == nil {
		return nil
	}
	if tr.Outcome != "" {
		return problem("conditions_on_outcome", tr.ID, "The step %q is taken by the system, so it has no conditions.", tr.Name)
	}
	if c.Match != conditions.All && c.Match != conditions.Any {
		return problem("bad_conditions", tr.ID, "The conditions of %q are joined by all or any.", tr.Name)
	}
	if len(c.Tests) == 0 || len(c.Tests) > conditions.MaxTests {
		return problem("bad_conditions", tr.ID, "The step %q has 1 to %d conditions.", tr.Name, conditions.MaxTests)
	}
	for _, t := range c.Tests {
		valid := false
		switch t.Kind {
		case TestLabelPresent, TestLabelAbsent:
			valid = t.Label != "" && t.Part == ""
		case TestChanged:
			valid = t.Label == "" && (t.Part == PartTitle || t.Part == PartBody || t.Part == PartFiles)
		case TestHasAttachment:
			valid = t.Label == "" && t.Part == ""
		}
		if !valid {
			return problem("bad_conditions", tr.ID, "A condition of %q is not one Diffluence knows.", tr.Name)
		}
	}
	return nil
}
