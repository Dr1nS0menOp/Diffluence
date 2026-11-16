// SPDX-License-Identifier: Apache-2.0

package search

import (
	"slices"
	"testing"

	"github.com/google/uuid"
)

// Spec 074, wave GT5, TPL-703: template: in the search syntax.

func TestTemplateIsAFilterOnlyWhereGuidedTemplatesAreOn(t *testing.T) {
	for _, input := range []string{`template:Incident`, `template:"Incident review" outage`, `budget template:`} {
		off := parse(input, false, false)
		if len(off.Templates) != 0 {
			t.Fatalf("%q off: a filter %v", input, off.Templates)
		}
		before := Parse(input, false)
		if off.TSQuery != before.TSQuery || off.Plain != before.Plain {
			t.Fatalf("%q off: %+v, not as before %+v", input, off, before)
		}
	}
	on := parse(`template:"Incident review" outage`, false, true)
	if !slices.Equal(on.Templates, []string{"Incident review"}) || on.TSQuery == "" {
		t.Fatalf("on: %+v", on)
	}
	if got := TemplateNames(`template:Onboarding template:"Incident review" words`); !slices.Equal(got, []string{"Onboarding", "Incident review"}) {
		t.Fatalf("names: %v", got)
	}
}

func TestATemplateNameThatStandsForNothingFindsNothing(t *testing.T) {
	parsed := parse(`template:Hidden`, false, true)
	if !findsNothing(Query{FromTemplates: true}, parsed) {
		t.Fatal("a name the reader may not see must find nothing")
	}
	if findsNothing(Query{FromTemplates: true, Templates: []uuid.UUID{uuid.New()}}, parsed) {
		t.Fatal("a name that stands for a template searches")
	}
	if findsNothing(Query{}, parse(`template:Hidden`, false, false)) {
		t.Fatal("without the filter the words are searched")
	}
	if !hasFilters(Query{Templates: []uuid.UUID{uuid.New()}}, parsed) {
		t.Fatal("a template filter alone is a search of filters")
	}
}
