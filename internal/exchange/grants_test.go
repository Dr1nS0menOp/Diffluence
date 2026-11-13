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
	for _, broad := range []string{permissions.SpaceAdminister, permissions.PageEdit, permissions.SpaceManageAccess, permissions.PageCreate} {
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
	want := slices.Sorted(slices.Values([]string{permissions.PageEdit, permissions.PageMoveOut, permissions.DatabaseEdit,
		permissions.PageWorkflow}))
	if got := permissionsOfGrant(2, permissions.PageEdit); !slices.Equal(got, want) {
		t.Fatalf("editing in a version 2 bundle moved pages out and edited databases too: %v", got)
	}
}

func TestABundleKeepsAGrantAsItIs(t *testing.T) {
	// A current bundle holds the finer permissions itself: administering alone stays alone.
	if got := permissionsOfGrant(bundleVersion, permissions.SpaceAdminister); !slices.Equal(got, []string{permissions.SpaceAdminister}) {
		t.Fatalf("a current grant changed on the way in: %v", got)
	}
	// An old grant of something nothing was split from stays alone too.
	if got := permissionsOfGrant(bundleVersionBeforeSplit, permissions.CommentAdd); !slices.Equal(got, []string{permissions.CommentAdd}) {
		t.Fatalf("an old grant gained permissions: %v", got)
	}
}

// versionOfSpec040 is the bundle version that first held the permissions of spec 040.
const versionOfSpec040 = 3

func TestEveryOlderBundleGivesAdministratorsTheEightOfSpec040(t *testing.T) {
	for version := bundleVersionBeforeSplit; version < versionOfSpec040; version++ {
		got := permissionsOfGrant(version, permissions.SpaceAdminister)
		for finer, v := range heldSince {
			if v == versionOfSpec040 && permissions.SplitFrom[finer] == permissions.SpaceAdminister && !slices.Contains(got, finer) {
				t.Fatalf("a bundle of format %d gave administration without %s: %v", version, finer, got)
			}
		}
	}
}

func TestABundleOfVersionThreeGivesWhoeverCreatesPagesPostsAndDatabases(t *testing.T) {
	got := permissionsOfGrant(versionOfSpec040, permissions.PageCreate)
	want := []string{permissions.PageCreate, permissions.PostCreate, permissions.DatabaseCreate}
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		t.Fatalf("a version 3 bundle knew nothing of posts and databases apart from pages: %v", got)
	}
	if got := permissionsOfGrant(versionOfSpec040, permissions.SpaceAdminister); !slices.Equal(got,
		[]string{permissions.SpaceAdminister, permissions.SpaceWorkflows}) {
		t.Fatalf("a version 3 bundle holds the eight of spec 040 itself, and knew nothing of workflows: %v", got)
	}
	if got := permissionsOfGrant(bundleVersion, permissions.PageCreate); !slices.Equal(got, []string{permissions.PageCreate}) {
		t.Fatalf("a current bundle holds posts and databases itself, so pages alone must stay alone: %v", got)
	}
}

// versionOfSpec045 is the bundle version that first held the create permissions of spec 045.
const versionOfSpec045 = 4

func TestABundleOfVersionFourGivesWhoeverEditsPagesDatabasesToo(t *testing.T) {
	got := permissionsOfGrant(versionOfSpec045, permissions.PageEdit)
	if want := []string{permissions.DatabaseEdit, permissions.PageEdit, permissions.PageWorkflow}; !slices.Equal(got, want) {
		t.Fatalf("a version 4 bundle knew nothing of editing databases apart from pages: %v", got)
	}
	if got := permissionsOfGrant(bundleVersion, permissions.PageEdit); !slices.Equal(got, []string{permissions.PageEdit}) {
		t.Fatalf("a current bundle holds database.edit itself, so editing pages alone must stay alone: %v", got)
	}
}

// versionOfSpec049 is the bundle version that first held database.edit, the last before the
// workflow permissions of spec 064.
const versionOfSpec049 = 5

func TestABundleOfVersionFiveGivesEditorsAndAdministratorsTheWorkflowPermissions(t *testing.T) {
	if got := permissionsOfGrant(versionOfSpec049, permissions.PageEdit); !slices.Equal(got,
		[]string{permissions.PageEdit, permissions.PageWorkflow}) {
		t.Fatalf("editing in a version 5 bundle set workflows as well, and must still: %v", got)
	}
	if got := permissionsOfGrant(versionOfSpec049, permissions.SpaceAdminister); !slices.Equal(got,
		[]string{permissions.SpaceAdminister, permissions.SpaceWorkflows}) {
		t.Fatalf("administering in a version 5 bundle ran the workflows as well, and must still: %v", got)
	}
	if got := permissionsOfGrant(bundleVersion, permissions.SpaceAdminister); !slices.Equal(got, []string{permissions.SpaceAdminister}) {
		t.Fatalf("a current bundle holds space.workflows itself: %v", got)
	}
}
