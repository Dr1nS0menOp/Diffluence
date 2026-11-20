// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// sortedNames lists the names in one list of an answer, sorted, so a test can compare them whole.
func sortedNames(m map[string]any, key string) []string {
	out := []string{}
	for _, row := range threads(m, key) {
		out = append(out, row["name"].(string))
	}
	slices.Sort(out)
	return out
}

func (e *env) rename(u *user, name string) {
	e.t.Helper()
	e.sql("UPDATE users SET display_name = $2 WHERE id = $1", u.id, name)
}

// TestPeopleFieldsFindTheStartOfAnyWordOfAName is NAV-121: every typed word is the start of a
// word of the name, in any order; a part inside a word finds nothing. Fields for people also
// find the start of the address they show (spec 086, D3).
func TestPeopleFieldsFindTheStartOfAnyWordOfAName(t *testing.T) {
	e := newEnv(t)
	root := e.user("root@example.com", identity.AllGlobalPermissions...)
	picker := e.user("picker@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	jan := e.user("jdv@example.com", identity.PermUseProduct)
	johan := e.user("johan@example.com", identity.PermUseProduct)
	anne := e.user("am@example.com", identity.PermUseProduct)
	e.rename(jan, "Jan de Vries")
	e.rename(johan, "Johan Smit")
	e.rename(anne, "Anne-Marie Peters")
	root.ok("POST", "/api/v1/admin/groups", map[string]any{"name": "Planning team"})
	space, _ := root.space("PPL")
	page := root.page(space, nil, "Page", "x")

	directory := func(text string) []string {
		t.Helper()
		return sortedNames(picker.ok("GET", "/api/v1/directory?q="+url.QueryEscape(text), nil), "entries")
	}
	for typed, want := range map[string][]string{
		"jan":       {"Jan de Vries"},
		"vries j":   {"Jan de Vries"},
		"jan vr":    {"Jan de Vries"},
		"an":        {"Anne-Marie Peters"},
		"marie":     {"Anne-Marie Peters"},
		"ohan":      {},
		"smit jan":  {},
		"johan@ex":  {"Johan Smit"},
		"@example":  {},
		"team":      {"Planning team"},
		"plan":      {"Planning team"},
		"lanning":   {},
		"example.c": {},
	} {
		if got := directory(typed); !slices.Equal(got, want) {
			t.Errorf("a field for %q finds %v, want %v", typed, got, want)
		}
	}

	if got := sortedNames(picker.ok("GET", "/api/v1/search/quick?q=vries+ja", nil), "people"); !slices.Equal(got, []string{"Jan de Vries"}) {
		t.Errorf("the box finds by the start of every word: %v", got)
	}
	if got := sortedNames(picker.ok("GET", "/api/v1/search/quick?q=ohan", nil), "people"); len(got) != 0 {
		t.Errorf("the box finds a part inside a word: %v", got)
	}
	if got := sortedNames(root.ok("GET", "/api/v1/pages/"+page+"/mention-candidates?q=peters+an", nil), "candidates"); !slices.Equal(got, []string{"Anne-Marie Peters"}) {
		t.Errorf("mentions find by the start of every word: %v", got)
	}
	if got := sortedNames(picker.ok("GET", "/api/v1/people?q=de+jan", nil), "people"); !slices.Equal(got, []string{"Jan de Vries"}) {
		t.Errorf("the People screen finds by the start of every word: %v", got)
	}
	if got := sortedNames(picker.ok("GET", "/api/v1/people?q=ohan", nil), "people"); len(got) != 0 {
		t.Errorf("the People screen finds a part inside a word: %v", got)
	}
}

// TestFieldsWithoutViewProfilesOfferOnlyPeopleOfTheSpace is NAV-059 and ACC-036 for word starts:
// without "view profiles", mentions and Share offer the people with a grant in the space and
// nobody else, the box offers nobody and the list for fields is closed.
func TestFieldsWithoutViewProfilesOfferOnlyPeopleOfTheSpace(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	inside := e.user("inside@example.com", identity.PermUseProduct)
	outside := e.user("outside@example.com", identity.PermUseProduct)
	e.rename(inside, "Mila Inside")
	e.rename(outside, "Mila Outside")
	space, _ := owner.space("GST")
	owner.grant(space, "user", guest.id, permissions.TemplateViewer)
	owner.grant(space, "user", inside.id, permissions.TemplateViewer)
	page := owner.page(space, nil, "Page", "x")

	if got := sortedNames(guest.ok("GET", "/api/v1/pages/"+page+"/mention-candidates?q=mila", nil), "candidates"); !slices.Equal(got, []string{"Mila Inside"}) {
		t.Fatalf("a guest is offered someone outside the space: %v", got)
	}
	if got := sortedNames(guest.ok("GET", "/api/v1/search/quick?q=mila", nil), "people"); len(got) != 0 {
		t.Fatalf("the box offers people without view profiles: %v", got)
	}
	guest.want(403, "GET", "/api/v1/directory?q=mila", nil)
}

// TestThePeopleTabKeepsToWhomTheReaderMaySee is NAV-059 for the People tab of the results,
// which asks the directory of colleagues (spec 086, D5): a guest finds the people of a space
// they share and nobody else, and the list has no address in it.
func TestThePeopleTabKeepsToWhomTheReaderMaySee(t *testing.T) {
	e := newEnv(t)
	owner := e.user("owner@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	guest := e.userOfType("guest@example.com", identity.TypeGuest, identity.PermUseProduct)
	member := e.user("member@example.com", identity.PermUseProduct, identity.PermViewProfiles)
	inside := e.user("inside@example.com", identity.PermUseProduct)
	outside := e.user("outside@example.com", identity.PermUseProduct)
	e.rename(inside, "Mila Inside")
	e.rename(outside, "Mila Outside")
	space, _ := owner.space("TAB")
	owner.grant(space, "user", guest.id, permissions.TemplateViewer)
	owner.grant(space, "user", inside.id, permissions.TemplateViewer)

	got := guest.ok("GET", "/api/v1/people?q=mila", nil)
	if names := sortedNames(got, "people"); !slices.Equal(names, []string{"Mila Inside"}) {
		t.Fatalf("a guest finds someone they share no space with: %v", names)
	}
	if got["total"].(float64) != 1 {
		t.Fatalf("a guest's count holds someone out of reach: %v", got["total"])
	}
	if strings.Contains(mustJSON(t, got), "@example.com") {
		t.Fatalf("the People tab shows an address: %v", got)
	}
	if names := sortedNames(member.ok("GET", "/api/v1/people?q=mila", nil), "people"); !slices.Equal(names, []string{"Mila Inside", "Mila Outside"}) {
		t.Fatalf("someone with view profiles finds everyone: %v", names)
	}
}
