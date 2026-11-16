// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func mustChanges(t *testing.T, before, after any, safe SafeFields) map[string]any {
	t.Helper()
	got, err := Changes(before, after, safe)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAFieldNotListedAsSafeIsRecordedWithoutItsValues(t *testing.T) {
	before := map[string]any{"limit": 1, "smtp_password": "old secret", "same": true}
	after := map[string]any{"limit": 2, "smtp_password": "new secret", "same": true, "added": "x"}
	got := mustChanges(t, before, after, SafeFields{"limit", "same"})
	want := map[string]any{
		"limit":         map[string]any{"before": float64(1), "after": float64(2)},
		"smtp_password": map[string]any{"redacted": true},
		"added":         map[string]any{"redacted": true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes %v, want %v", got, want)
	}
}

func TestNothingIsSafeUnlessListed(t *testing.T) {
	got := mustChanges(t, map[string]any{"token": "a"}, map[string]any{"token": "b"}, nil)
	if !reflect.DeepEqual(got, map[string]any{"token": map[string]any{"redacted": true}}) {
		t.Fatalf("changes %v", got)
	}
}

func TestAValueThatIsNoObjectIsOneField(t *testing.T) {
	got := mustChanges(t, nil, 60, SafeFields{WholeValue})
	if !reflect.DeepEqual(got, map[string]any{WholeValue: map[string]any{"before": nil, "after": float64(60)}}) {
		t.Fatalf("changes %v", got)
	}
	if got := mustChanges(t, 60, 60, SafeFields{WholeValue}); len(got) != 0 {
		t.Fatalf("an unchanged value records nothing: %v", got)
	}
}

func TestALongValueIsCutInTheAuditRow(t *testing.T) {
	long := strings.Repeat("é", maxAuditValueRunes+50)
	got := mustChanges(t, map[string]any{"text": ""}, map[string]any{"text": long, "list": strings.Split(long, "")},
		SafeFields{"text", "list"})
	text := got["text"].(map[string]any)
	if text["truncated"] != true || utf8.RuneCountInString(text["after"].(string)) > maxAuditValueRunes+1 {
		t.Fatalf("text kept %d characters: %v", utf8.RuneCountInString(text["after"].(string)), text["truncated"])
	}
	list := got["list"].(map[string]any)
	if s, ok := list["after"].(string); !ok || list["truncated"] != true || utf8.RuneCountInString(s) > maxAuditValueRunes+1 {
		t.Fatalf("a long list is kept as cut text: %v", list)
	}
}

func TestAnAddressIsRecordedWithoutItsUserAndPassword(t *testing.T) {
	got := mustChanges(t, map[string]any{"help_url": "https://name:secret@help.example.com/a"},
		map[string]any{"help_url": "https://help.example.com/b"}, SafeFields{"help_url"})
	if text := fmt.Sprint(got); strings.Contains(text, "secret") || strings.Contains(text, "name:") {
		t.Fatalf("credentials reached the row: %v", got)
	}
	before := got["help_url"].(map[string]any)["before"]
	if before != "https://help.example.com/a" {
		t.Fatalf("before %v", before)
	}
}

// A field nobody decided on stays out of the audit log by default; this makes the decision
// explicit for every field a section has now.
func TestEveryFieldOfEverySectionHasAnAuditDecision(t *testing.T) {
	for _, name := range []string{SectionGeneral, SectionFeatures, SectionAccess, SectionRetention, SectionLook, SectionBackups, SectionHealth} {
		v, err := sectionValue(name)
		if err != nil {
			t.Fatal(err)
		}
		safe, ok := sectionSafeFields[name]
		if !ok {
			t.Errorf("%s has no list of safe fields", name)
		}
		for field := range jsonFields(reflect.TypeOf(v)) {
			listed := 0
			for _, list := range [][]string{safe, sectionRedactedFields[name]} {
				for _, f := range list {
					if f == field {
						listed++
					}
				}
			}
			if listed != 1 {
				t.Errorf("%s.%s must be either safe or redacted on purpose, once", name, field)
			}
		}
	}
}

func TestTheOrganisationNameIsRedactedOnPurpose(t *testing.T) {
	got, err := SectionChanges(SectionLook, &Look{OrganisationName: "A"}, &Look{OrganisationName: "B"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"organisation_name": map[string]any{"redacted": true}}) {
		t.Fatalf("changes %v", got)
	}
}

func TestAnAddressWithAUserOrPasswordIsRefused(t *testing.T) {
	for _, address := range []string{"https://name:secret@help.example.com", "https://bank.example@help.example.com", "http://user@intranet"} {
		if safeAddress(address) {
			t.Errorf("%s must be refused", address)
		}
	}
	for _, address := range []string{"", "http://intranet/help", "https://help.example.com/a?b=c"} {
		if !safeAddress(address) {
			t.Errorf("%s must be accepted", address)
		}
	}
}
