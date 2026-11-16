// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"strings"
	"testing"
)

func TestNameWordsAreStartsOfWords(t *testing.T) {
	for typed, want := range map[string]string{
		"":                    "",
		"   ":                 "",
		"Jan":                 "'jan':*",
		"jan VR":              "'jan':* & 'vr':*",
		"anne-m":              "'anne':* & 'm':*",
		"o'brien":             "'o':* & 'brien':*",
		"Zoë":                 "'zoë':*",
		"user42":              "'user42':*",
		"a & b | !c:* (d)":    "'a':* & 'b':* & 'c':* & 'd':*",
		`back\slash 'quoted'`: "'back':* & 'slash':* & 'quoted':*",
		"@#$%":                "",
		"one two three four five six seven eight nine": "'one':* & 'two':* & 'three':* & 'four':* & 'five':* & 'six':* & 'seven':* & 'eight':*",
	} {
		if got := NameWords(typed); got != want {
			t.Errorf("NameWords(%q) = %q, want %q", typed, got, want)
		}
	}
}

func TestNameWordsNeverCarryTheSyntaxOfAQuery(t *testing.T) {
	got := NameWords(`x' | 'y':* & !z`)
	for _, r := range got {
		if strings.ContainsRune("|!()\\", r) {
			t.Fatalf("a typed sign reached the query: %q", got)
		}
	}
	if strings.Count(got, "'") != 2*strings.Count(got, ":*") {
		t.Fatalf("a typed quote reached the query: %q", got)
	}
}

func TestAnAddressStartIsMatchedLiterally(t *testing.T) {
	for typed, want := range map[string]string{
		"":          "",
		"Jan@Ex":    "jan@ex%",
		"50%_off\\": `50\%\_off\\%`,
	} {
		if got := addressStart(typed); got != want {
			t.Errorf("addressStart(%q) = %q, want %q", typed, got, want)
		}
	}
}
