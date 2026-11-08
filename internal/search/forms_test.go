// SPDX-License-Identifier: Apache-2.0

package search

import (
	"slices"
	"testing"
)

func TestAWordAlsoAsksForItsCommonForms(t *testing.T) {
	cases := []struct {
		word string
		want []string
	}{
		{"plans", []string{"plans", "plan", "planen"}},
		{"plan", []string{"plan", "plans", "planen"}},
		{"policies", []string{"policies", "policy", "policyen"}},
		{"policy", []string{"policy", "policies", "policyen"}},
		{"boxes", []string{"boxes", "box", "boxs", "boxen"}},
		{"plannen", []string{"plannen", "plann", "planns"}},
		// Too short to strip: a stem of two letters would match far too much.
		{"bus", []string{"bus", "buss", "busen"}},
		{"is", []string{"is"}},
	}
	for _, c := range cases {
		if got := forms(c.word); !slices.Equal(got, c.want) {
			t.Errorf("forms(%q) = %v, want %v", c.word, got, c.want)
		}
	}
}

func TestAFormsGroupKeepsItsSuffixOnEveryForm(t *testing.T) {
	if got := formsTerm("Plans", ":*A"); got != "('plans':*A | 'plan':*A | 'planen':*A)" {
		t.Fatalf("got %s", got)
	}
	if got := formsTerm("is", ""); got != "'is'" {
		t.Fatalf("a word with one form needs no group: %s", got)
	}
}
