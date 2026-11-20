// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/pages"
)

// TestEachPageOfAScopeGetsItsPartInARun is PUB-401 to PUB-403, PUB-420 to PUB-422, PUB-1010 and
// PUB-1013 as a table: from the facts of a page to what a run does with it.
func TestEachPageOfAScopeGetsItsPartInARun(t *testing.T) {
	parent := uuid.New()
	named := uuid.New()
	page := func(change func(*scopeRow)) scopeRow {
		r := scopeRow{ID: uuid.New(), Kind: pages.KindPage, Status: pages.StatusPublished, CurrentVersion: 2, Visible: true}
		change(&r)
		return r
	}
	released := func(r *scopeRow) { r.ReleasedVersion = 1 }
	space := scopeRules{mirror: true, named: map[uuid.UUID]bool{}}
	cases := []struct {
		name  string
		row   scopeRow
		rules scopeRules
		want  string
	}{
		{"never released", page(func(*scopeRow) {}), space, "make:"},
		{"in step", page(released), space, "none:"},
		{"text changed", page(func(r *scopeRow) { released(r); r.Changed = []string{"text"} }), space, "change:"},
		{"a link waiting", page(func(r *scopeRow) { released(r); r.Changed = []string{aspectLinks} }), space, "change:"},
		{"moved only", page(func(r *scopeRow) { released(r); r.Changed = []string{"place"} }), space, "update:"},
		{"a file only", page(func(r *scopeRow) { released(r); r.Changed = []string{"files"} }), space, "update:"},
		{"left out", page(func(r *scopeRow) { r.LeftOut = true }), space, "skip:left_out"},
		{"left out and released", page(func(r *scopeRow) { released(r); r.LeftOut = true; r.Changed = []string{"text"} }), space, "skip:left_out"},
		{"below a page left out", page(func(r *scopeRow) { r.ParentID = &parent }), space, "skip:below_left_out"},
		{"restricted, never released", page(func(r *scopeRow) { r.Restricted = true }), space, "skip:restricted"},
		{"restricted and named", page(func(r *scopeRow) { r.ID = named; r.Restricted = true }),
			scopeRules{mirror: true, named: map[uuid.UUID]bool{named: true}}, "make:"},
		{"restricted and chosen", page(func(r *scopeRow) { r.Restricted = true }), scopeRules{chosen: true}, "make:"},
		{"restricted, released alone before", page(func(r *scopeRow) { released(r); r.Restricted = true; r.Changed = []string{"text"} }), space, "change:"},
		{"a database", page(func(r *scopeRow) { r.Kind = pages.KindDatabase }), space, "skip:kind"},
		{"never approved", page(func(r *scopeRow) { r.NeverApproved = true }), space, "skip:not_approved"},
		{"archived, never released", page(func(r *scopeRow) { r.Archived = true }), space, "skip:archived"},
		{"archived after its release", page(func(r *scopeRow) { released(r); r.Archived = true }), space, "archive:archived"},
		{"archived on both sides", page(func(r *scopeRow) { released(r); r.Archived = true; r.TwinArchived = true }), space, "none:"},
		{"archived, removals kept", page(func(r *scopeRow) { released(r); r.Archived = true }), scopeRules{}, "none:"},
		{"deleted after its release", page(func(r *scopeRow) { released(r); r.Status = pages.StatusTrashed }), space, "take_off:deleted"},
		{"deleted, removals kept", page(func(r *scopeRow) { released(r); r.Status = pages.StatusTrashed }), scopeRules{}, "none:"},
	}
	for _, c := range cases {
		d := decide(c.row, c.rules, map[uuid.UUID]bool{parent: true})
		if got := fmt.Sprint(d.action, ":", d.reason); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// TestATwinTakesItsPlaceAfterTheSiblingBeforeItsDraftPage is PUB-412: twins written are placed
// as in the draft, and twins not written keep their order.
func TestATwinTakesItsPlaceAfterTheSiblingBeforeItsDraftPage(t *testing.T) {
	a, b, c, d, other := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	name := map[uuid.UUID]string{a: "A", b: "B", c: "C", d: "D", other: "x"}
	words := func(list []uuid.UUID) string {
		out := ""
		for _, id := range list {
			out += name[id]
		}
		return out
	}
	cases := []struct {
		current, draft []uuid.UUID
		written        []uuid.UUID
		want           string
	}{
		{[]uuid.UUID{c, a}, []uuid.UUID{a, c}, []uuid.UUID{a}, "AC"},
		{[]uuid.UUID{a, c, b, d}, []uuid.UUID{a, b, c, d}, []uuid.UUID{a, b, c, d}, "ABCD"},
		{[]uuid.UUID{a, b, c, d}, []uuid.UUID{d, a, b, c}, []uuid.UUID{d}, "DABC"},
		{[]uuid.UUID{a, other, c}, []uuid.UUID{a, b, c}, []uuid.UUID{b}, "ABxC"},
		{[]uuid.UUID{b, a}, []uuid.UUID{a, b}, []uuid.UUID{}, "BA"},
	}
	for _, c := range cases {
		written := map[uuid.UUID]bool{}
		for _, id := range c.written {
			written[id] = true
		}
		if got := words(placeInOrder(c.current, c.draft, written)); got != c.want {
			t.Errorf("%s with %s written in %s: %s, want %s", words(c.current), words(c.written), words(c.draft), got, c.want)
		}
	}
}

// TestARunThatWouldTakeLongGoesInBatches is PUB-450, PUB-451 and PUB-1106: up to the limit in
// one transaction, past it or past the time budget in batches of 250.
func TestARunThatWouldTakeLongGoesInBatches(t *testing.T) {
	l := DefaultLimits()
	if n := l.batchesFor(1000, 0); n != 0 {
		t.Errorf("1,000 pages: %d batches", n)
	}
	if n := l.batchesFor(1001, 0); n != 5 {
		t.Errorf("1,001 pages: %d batches, want 5", n)
	}
	if n := l.batchesFor(2000, 0); n != 8 {
		t.Errorf("2,000 pages: %d batches, want 8", n)
	}
	if n := l.batchesFor(10, 2<<30); n != 1 {
		t.Errorf("ten pages with 2 GB of files: %d batches, want 1", n)
	}
	slow := l
	slow.PerPage = time.Minute
	if n := slow.batchesFor(2, 0); n != 1 {
		t.Errorf("a slow run of two pages: %d batches, want 1", n)
	}
}
