// SPDX-License-Identifier: Apache-2.0

package scim

import (
	"slices"
	"strings"
	"testing"
)

// FuzzParseFilter reads hostile filters from a provider (SEC-1022). A filter is understood whole
// or refused: an ignored filter would answer with everyone, and a provider then makes duplicates.
func FuzzParseFilter(f *testing.F) {
	f.Add(`userName eq "anna@example.com"`)
	f.Add(`emails[type eq "work"].value eq "a\"b" and externalId eq "x"`)
	f.Add(`displayName co "a" or userName pr`)
	f.Fuzz(func(t *testing.T, filter string) {
		got, err := parseFilter(filter, userFilterAttributes)
		if err != nil {
			return
		}
		if strings.TrimSpace(filter) != "" && len(got) == 0 {
			t.Fatalf("the filter %q was ignored", filter)
		}
		for attr := range got {
			if !slices.Contains(userFilterAttributes, attr) {
				t.Fatalf("the filter %q gave %q, which no route reads", filter, attr)
			}
		}
	})
}

func TestAFilterIsUnderstoodWholeOrRefused(t *testing.T) {
	got, err := parseFilter(`userName eq "anna@example.com" and emails[type eq "work"].value eq "a@b.c"`, userFilterAttributes)
	if err != nil || got["username"] != "anna@example.com" || got["emails.value"] != "a@b.c" {
		t.Fatalf("%v %v", got, err)
	}
	for _, filter := range []string{`userName eq "a" or userName eq "b"`, `userName co "a"`, `userName eq "a" and active pr`, `nickName eq "a"`} {
		if _, err := parseFilter(filter, userFilterAttributes); err == nil {
			t.Errorf("%q was taken in part", filter)
		}
	}
}

func TestAGroupFilterOnlyTakesWhatTheGroupRouteReads(t *testing.T) {
	if _, err := parseFilter(`displayName eq "Team"`, groupFilterAttributes); err != nil {
		t.Fatal(err)
	}
	if _, err := parseFilter(`userName eq "a"`, groupFilterAttributes); err == nil {
		t.Fatal("a filter on users was taken for groups")
	}
}
