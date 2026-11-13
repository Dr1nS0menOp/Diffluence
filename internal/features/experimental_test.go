// SPDX-License-Identifier: Apache-2.0

package features_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/testutil"
)

// The test database turns on every experimental part, so each suite tests the part
// itself; a part added to the list without it would be off in every suite.
func TestTheTestDatabaseTurnsOnEveryExperimentalPart(t *testing.T) {
	var v features.Experimental
	if err := json.Unmarshal([]byte(testutil.AllExperimentalOn), &v); err != nil {
		t.Fatal(err)
	}
	want := slices.Clone(features.ExperimentalParts)
	slices.Sort(want)
	slices.Sort(v.On)
	if !slices.Equal(v.On, want) {
		t.Fatalf("testutil turns on %v, the parts are %v", v.On, want)
	}
}
