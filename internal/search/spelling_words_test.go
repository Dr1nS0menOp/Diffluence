// SPDX-License-Identifier: Apache-2.0

package search

import "testing"

func TestWordsToSpellLeaveTheFiltersOutAndKeepATitle(t *testing.T) {
	cases := map[string]string{
		"handbok label:nowhere":      "handbok",
		"space:ENG plan author:anna": "plan",
		"title:handbok":              "handbok",
		"label:hr":                   "",
		"  release   notes ":         "release notes",
		"labels:hr is not a filter":  "labels:hr is not a filter",
	}
	for in, want := range cases {
		if got := wordsToSpell(in); got != want {
			t.Errorf("wordsToSpell(%q) = %q, want %q", in, got, want)
		}
	}
}
