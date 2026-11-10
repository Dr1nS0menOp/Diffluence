// SPDX-License-Identifier: Apache-2.0

package search

import (
	"strings"
	"testing"
)

func TestTypedSignsBecomeLettersInALikePattern(t *testing.T) {
	for typed, want := range map[string]string{
		"Port":       "%port%",
		"50%":        `%50\%%`,
		"n_a":        `%n\_a%`,
		`a\b`:        `%a\\b%`,
		"  spaced  ": "%spaced%",
	} {
		if got := LikeOf(typed); got != want {
			t.Errorf("LikeOf(%q) = %q, want %q", typed, got, want)
		}
	}
}

func TestNearMatchesWaitForThreeLettersAndStopAfterSixtyFour(t *testing.T) {
	for typed, want := range map[string]bool{
		"hb": false, " hb ": false, "hbk": true, "één": true,
		strings.Repeat("a", 64): true, strings.Repeat("a", 65): false,
	} {
		if got := NearLength(typed); got != want {
			t.Errorf("NearLength(%q) = %v, want %v", typed, got, want)
		}
	}
	if TypoLength("week") || !TypoLength("weeks") {
		t.Error("typos are looked for from five letters")
	}
	if !wantsNear(Parse("handbok", false), "handbok") || wantsNear(Parse(`"handbok"`, false), `"handbok"`) {
		t.Error("only plain typing gets near matches")
	}
}
