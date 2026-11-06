// SPDX-License-Identifier: Apache-2.0

package search

import (
	"reflect"
	"regexp"
	"testing"
)

func TestSearchLanguage(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"plan", "'plan':*"},
		{"plan budget", "'plan' & 'budget':*"},
		{`"open plan"`, "'open' <-> 'plan'"},
		{"plan AND budget", "'plan' & 'budget'"},
		{"plan OR budget", "'plan' | 'budget'"},
		{"plan or budget", "'plan' & 'or' & 'budget':*"},
		{"NOT draft", "!('draft')"},
		{"(plan OR budget) AND 2026", "('plan' | 'budget') & '2026'"},
		{"plan*", "'plan':*"},
		{"title:plan", "'plan':A"},
		{`title:"open plan"`, "'open':A <-> 'plan':A"},
		{"foo:bar", "'foo' & 'bar':*"},
		{"pl*n budget", "'pl':* & 'budget'"},
		{"*plan", "'plan'"},
		// What cannot be read falls back to plain words.
		{"(plan", "'plan':*"},
		{"plan AND", "'plan' & 'and':*"},
		{"plan)", "'plan':*"},
		{"NOT", "'not':*"},
		{`"unfinished quote`, "'unfinished' & 'quote':*"},
		{"!!! :: &&", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := Parse(c.input, false).TSQuery; got != c.want {
			t.Errorf("Parse(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestFieldsBecomeFilters(t *testing.T) {
	got := Parse(`plan label:hr space:ENG author:"Anna Visser" label:legal`, false)
	want := Parsed{TSQuery: "'plan'", Labels: []string{"hr", "legal"}, Spaces: []string{"eng"}, Authors: []string{"Anna Visser"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestWildcardsInsideAWordAreChecked(t *testing.T) {
	got := Parse("pla? pl*n", false)
	want := []string{`\mpla\w\M`, `\mpl\w*n\M`}
	if !reflect.DeepEqual(got.Patterns, want) {
		t.Fatalf("got %v", got.Patterns)
	}
	if many := Parse("a?b c?d e?f g?h i?j", false); len(many.Patterns) != maxWildcards {
		t.Fatalf("patterns: %v", many.Patterns)
	}
}

func TestTitlesOnlyLimitsEveryTerm(t *testing.T) {
	if got := Parse("plan budget", true).TSQuery; got != "'plan':A & 'budget':*A" {
		t.Fatalf("got %q", got)
	}
	if got := Parse(`"open plan" OR budget`, true).TSQuery; got != "'open':A <-> 'plan':A | 'budget':A" {
		t.Fatalf("got %q", got)
	}
}

func TestASearchHasAtMostTwentyTerms(t *testing.T) {
	long := ""
	for range 30 {
		long += "(a) "
	}
	if got := Parse(long, false).TSQuery; len(regexp.MustCompile(`'a'`).FindAllString(got, -1)) != maxTerms {
		t.Fatalf("got %q", got)
	}
}

var safeQuery = regexp.MustCompile(`^[\p{L}\p{M}\p{N}'&|!()<>:* -]*$`)

func FuzzParseQuery(f *testing.F) {
	for _, seed := range []string{`"a b" OR (c AND NOT d)`, "title:x label:y", "pl*n ?", "((((", `'; DROP TABLE pages; --`, "a\x00b"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		for _, titles := range []bool{false, true} {
			got := Parse(input, titles)
			if !safeQuery.MatchString(got.TSQuery) {
				t.Fatalf("Parse(%q) wrote unsafe query %q", input, got.TSQuery)
			}
			for _, pattern := range got.Patterns {
				if _, err := regexp.Compile(pattern); err != nil && len(pattern) < 200 {
					t.Logf("pattern %q is for PostgreSQL only: %v", pattern, err)
				}
			}
		}
	})
}

func TestSuggestionsComeFromTitlesOnly(t *testing.T) {
	titles := []string{"Holiday plan", "Budget 2026"}
	if got := suggest("holliday plam", titles); got != "holiday plan" {
		t.Fatalf("got %q", got)
	}
	if got := suggest("zzzzzz", titles); got != "" {
		t.Fatalf("a word nothing like any title: %q", got)
	}
	if got := suggest("plan", titles); got != "" {
		t.Fatalf("a word that exists needs no suggestion: %q", got)
	}
}
