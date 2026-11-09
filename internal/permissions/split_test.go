// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"slices"
	"testing"
)

func TestAListFromBeforeASplitGetsWhatWasSplitOff(t *testing.T) {
	got := WithSplitOff([]string{SpaceView, SpaceAdminister})
	for f, broad := range SplitFrom {
		if broad == SpaceAdminister && !slices.Contains(got, f) {
			t.Fatalf("%s is missing: %v", f, got)
		}
	}
	if got := WithSplitOff([]string{SpaceView, PageEdit}); !slices.Contains(got, PageMoveOut) {
		t.Fatalf("editing lost moving out: %v", got)
	}
	// Written after spec 036 but before spec 040: it knows publishing, not labels.
	got = WithSplitOff([]string{SpaceView, SpaceAdminister, SitePublish})
	if !slices.Contains(got, SpaceLabels) || slices.Contains(got, SpaceAudit) {
		t.Fatalf("a list from between the splits: %v", got)
	}
}

func TestAListThatKnowsTheSplitStaysAsItIs(t *testing.T) {
	in := []string{SpaceView, SpaceAdminister, SitePublish, SpaceLabels}
	if got := WithSplitOff(in); !slices.Equal(got, slices.Sorted(slices.Values(in))) {
		t.Fatalf("a list naming finer permissions changed: %v", got)
	}
	if got := WithSplitOff([]string{SpaceView, SpaceLabels}); slices.Contains(got, SpaceAdminister) || len(got) != 2 {
		t.Fatalf("a finer permission brought something along: %v", got)
	}
	for f := range SplitFrom {
		if _, ok := splitInSpec[f]; !ok {
			t.Fatalf("%s has no spec of its split", f)
		}
	}
}
