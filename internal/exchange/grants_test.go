// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"slices"
	"testing"

	"github.com/diffluence/diffluence/internal/permissions"
)

func TestABundleFromBeforeTheSplitKeepsWhatAdministeringGave(t *testing.T) {
	got := permissionsOfGrant(bundleVersionBeforeSplit, permissions.SpaceAdminister)
	for finer, broad := range permissions.SplitFrom {
		if broad == permissions.SpaceAdminister && !slices.Contains(got, finer) {
			t.Fatalf("an old grant of space administration came in without %s: %v", finer, got)
		}
	}
	if !slices.Contains(got, permissions.SpaceAdminister) {
		t.Fatalf("the grant itself went missing: %v", got)
	}
}

func TestABundleKeepsAGrantAsItIs(t *testing.T) {
	// A current bundle holds the finer permissions itself: administering alone stays alone.
	if got := permissionsOfGrant(bundleVersion, permissions.SpaceAdminister); !slices.Equal(got, []string{permissions.SpaceAdminister}) {
		t.Fatalf("a current grant changed on the way in: %v", got)
	}
	// An old grant of something nothing was split from stays alone too.
	if got := permissionsOfGrant(bundleVersionBeforeSplit, permissions.PageEdit); !slices.Equal(got, []string{permissions.PageEdit}) {
		t.Fatalf("an old grant gained permissions: %v", got)
	}
}
