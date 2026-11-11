// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"reflect"
	"testing"
)

// A field this release knows but leaves out of its JSON (omitempty) is cleared on purpose;
// only a field it does not know at all belongs to a newer release and is kept.
func TestTheFieldsASectionKnowsIncludeOmittedAndEmbeddedOnes(t *testing.T) {
	type inner struct {
		Deep string `json:"deep"`
	}
	type section struct {
		inner
		Named    string `json:"named"`
		Optional string `json:"optional,omitempty"`
		Plain    string
		Skipped  string `json:"-"`
		hidden   string
	}
	_ = section{}.hidden
	got := jsonFields(reflect.TypeOf(&section{}))
	want := map[string]bool{"deep": true, "named": true, "optional": true, "Plain": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields %v, want %v", got, want)
	}
}
