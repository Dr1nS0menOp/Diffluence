// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"slices"
	"testing"

	"github.com/diffluence/diffluence/internal/permissions"
)

func TestEverySplitPermissionKnowsTheBundleVersionThatHoldsIt(t *testing.T) {
	for finer := range permissions.SplitFrom {
		v, ok := heldSince[finer]
		if !ok || v <= bundleVersionBeforeSplit || v > bundleVersion {
			t.Fatalf("%s is held since bundle version %d (known %v): an older bundle would lose it", finer, v, ok)
		}
	}
}

func TestABundleFromBeforeTheSplitKeepsWhatTheBroaderPermissionGave(t *testing.T) {
	for _, broad := range []string{permissions.SpaceAdminister, permissions.PageEdit, permissions.SpaceManageAccess} {
		got := permissionsOfGrant(bundleVersionBeforeSplit, broad)
		for finer, from := range permissions.SplitFrom {
			if from == broad && !slices.Contains(got, finer) {
				t.Fatalf("an old grant of %s came in without %s: %v", broad, finer, got)
			}
		}
		if !slices.Contains(got, broad) {
			t.Fatalf("the grant itself went missing: %v", got)
		}
	}
}

func TestABundleOfVersionTwoGainsOnlyTheLaterSplit(t *testing.T) {
	got := permissionsOfGrant(2, permissions.SpaceAdminister)
	if slices.Contains(got, permissions.SitePublish) {
		t.Fatalf("a version 2 bundle holds publishing itself, so administering alone must not bring it: %v", got)
	}
	if !slices.Contains(got, permissions.SpaceLabels) || !slices.Contains(got, permissions.SpaceLook) {
		t.Fatalf("a version 2 bundle knew nothing of labels and the look: %v", got)
	}
	if got := permissionsOfGrant(2, permissions.PageEdit); !slices.Equal(got, []string{permissions.PageEdit, permissions.PageMoveOut}) {
		t.Fatalf("editing in a version 2 bundle moved pages out too: %v", got)
	}
}

func TestABundleKeepsAGrantAsItIs(t *testing.T) {
	// A current bundle holds the finer permissions itself: administering alone stays alone.
	if got := permissionsOfGrant(bundleVersion, permissions.SpaceAdminister); !slices.Equal(got, []string{permissions.SpaceAdminister}) {
		t.Fatalf("a current grant changed on the way in: %v", got)
	}
	// An old grant of something nothing was split from stays alone too.
	if got := permissionsOfGrant(bundleVersionBeforeSplit, permissions.PageCreate); !slices.Equal(got, []string{permissions.PageCreate}) {
		t.Fatalf("an old grant gained permissions: %v", got)
	}
}

func TestEveryOlderBundleGivesAdministratorsTheEightOfSpec040(t *testing.T) {
	for version := bundleVersionBeforeSplit; version < bundleVersion; version++ {
		got := permissionsOfGrant(version, permissions.SpaceAdminister)
		for finer, v := range heldSince {
			if v == bundleVersion && permissions.SplitFrom[finer] == permissions.SpaceAdminister && !slices.Contains(got, finer) {
				t.Fatalf("a bundle of format %d gave administration without %s: %v", version, finer, got)
			}
		}
	}
}
