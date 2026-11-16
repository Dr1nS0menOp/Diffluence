// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diffluence/diffluence/internal/identity"
)

// searchAnswer runs a full search and gives its total, whether it was capped and its rows.
func searchAnswer(u *user, query string) (total float64, capped bool, rows []map[string]any) {
	u.e.t.Helper()
	got := u.ok("GET", "/api/v1/search?"+query, nil)
	for _, row := range got["results"].([]any) {
		rows = append(rows, row.(map[string]any))
	}
	capped, _ = got["capped"].(bool)
	return got["total"].(float64), capped, rows
}

// TestASearchCountsOnlyPagesTheReaderMaySee is NAV-005, NAV-006 and ORG-011 for the count:
// a page the reader may not see, whether a restriction or document control keeps it from
// them, is neither listed nor counted, so a total never tells that a hidden page holds a word
// (spec 080, SR1).
func TestASearchCountsOnlyPagesTheReaderMaySee(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "SRC")
	for i := range 3 {
		w.author.page(w.space, w.home, fmt.Sprintf("Open zebra %d", i), "zebraword in an open page")
	}
	w.controlled("Never approved aardvarktitle", "zebraword and aardvarkword")
	w.controlled("Never approved second", "zebraword")
	restricted := w.author.page(w.space, w.home, "Restricted page", "zebraword aardvarkword")
	w.admin.restrict(restricted, "view", w.admin.id, w.author.id)

	for _, u := range []*user{w.reader, w.guest} {
		for _, query := range []string{"q=aardvarkword", "q=aardvarktitle", "q=aardvarktitl", "q=aardvark", "q=aardvarkword&sort=date",
			"q=aardvarkword&titles=1", "q=aardvarkword&label=nothing", "q=aardvarkword&stage=Draft"} {
			total, capped, rows := searchAnswer(u, query)
			if total != 0 || capped || len(rows) != 0 {
				t.Fatalf("%s, %s: a hidden page is counted: total %v, capped %v, rows %v", u.email, query, total, capped, rows)
			}
		}
		total, _, rows := searchAnswer(u, "q=zebraword")
		if total != 3 || len(rows) != 3 {
			t.Fatalf("%s: the open pages are not counted alone: total %v, %d rows", u.email, total, len(rows))
		}
		_, _, first := searchAnswer(u, "q=zebraword&limit=2")
		_, _, rest := searchAnswer(u, "q=zebraword&limit=2&offset=2")
		if len(first) != 2 || len(rest) != 1 {
			t.Fatalf("%s: paging skips or repeats over hidden pages: %d then %d", u.email, len(first), len(rest))
		}
	}
	if total, _, _ := searchAnswer(w.author, "q=zebraword"); total != 6 {
		t.Fatalf("the author does not count what they may see: %v", total)
	}
}

// TestSearchBreadcrumbsNameOnlyPagesTheReaderMaySee is NAV-005: a page under a controlled page
// that was never approved is found, and its way back never names the hidden page.
func TestSearchBreadcrumbsNameOnlyPagesTheReaderMaySee(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "SRB")
	parent := w.controlled("Hidden okapiparent", "the parent text")
	child := w.author.page(w.space, parent, "Child under it", "lemurword in the child")
	found, row := searchFinds(w.reader, child, "lemurword")
	if !found {
		t.Fatal("the reader does not find the child page")
	}
	if got := fmt.Sprint(row["breadcrumbs"]); strings.Contains(got, "okapiparent") || strings.Contains(got, parent) {
		t.Fatalf("the breadcrumbs name a hidden page: %s", got)
	}
	if _, row := searchFinds(w.author, child, "lemurword"); !strings.Contains(fmt.Sprint(row["breadcrumbs"]), "okapiparent") {
		t.Fatalf("the author lost the way back: %v", row["breadcrumbs"])
	}
	// The page itself gives the same way back as its search result (spec 080).
	if got := fmt.Sprint(w.reader.ok("GET", "/api/v1/pages/"+child, nil)["breadcrumbs"]); strings.Contains(got, "okapiparent") || strings.Contains(got, parent) {
		t.Fatalf("the page names a hidden page in its way back: %s", got)
	}
	if got := fmt.Sprint(w.author.ok("GET", "/api/v1/pages/"+child, nil)["breadcrumbs"]); !strings.Contains(got, "okapiparent") {
		t.Fatalf("the author lost the way back on the page: %s", got)
	}
}

// TestSearchBelowAPageTheReaderMayNotSeeFindsNothing is ORG-015 with NAV-005: the filter "below
// a page" answers as if a hidden page had no pages below it, and a page the reader may see
// narrows the search to its branch.
func TestSearchBelowAPageTheReaderMayNotSeeFindsNothing(t *testing.T) {
	e := newEnv(t)
	w := newControlWorld(e, "SRP")
	hidden := w.controlled("Hidden branch", "the parent text")
	w.author.page(w.space, hidden, "Under hidden", "tapirword below a hidden page")
	open := w.author.page(w.space, w.home, "Open branch", "the open parent")
	w.author.page(w.space, open, "Under open", "tapirword below an open page")
	w.author.page(w.space, w.home, "Elsewhere", "tapirword elsewhere")

	for _, query := range []string{"q=tapirword&below=" + hidden, "below=" + hidden, "below=" + hidden + "&label=x"} {
		if total, capped, rows := searchAnswer(w.reader, query); total != 0 || capped || len(rows) != 0 {
			t.Fatalf("%s: a hidden page narrows a search: total %v, rows %v", query, total, rows)
		}
	}
	_, _, rows := searchAnswer(w.reader, "q=tapirword&below="+open)
	if len(rows) != 1 || rows[0]["title"] != "Under open" {
		t.Fatalf("below an open page: %v", rows)
	}
	if _, _, rows := searchAnswer(w.author, "q=tapirword&below="+hidden); len(rows) != 1 {
		t.Fatalf("the author lost the branch they may see: %v", rows)
	}
}

// TestASpellingSuggestionLeavesTheFieldWordsOut is ORG-023 with ORG-031: a similar spelling is
// offered for the plain words only, so a filter such as label: never turns into words to find.
func TestASpellingSuggestionLeavesTheFieldWordsOut(t *testing.T) {
	e := newEnv(t)
	owner := e.user("srs@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("SRS")
	owner.page(space, home, "Handbook of the team", "how we work")
	got := owner.ok("GET", "/api/v1/search?q=handbok+label:nowhere", nil)["suggestion"]
	if got != "handbook" {
		t.Fatalf("suggestion %q, want the plain word alone", got)
	}
}

// TestSearchResultsSayWhichPageIsAHomePage is NAV-156: every result carries its kind, and the
// home page of a space is marked as one, so the list can show the right mark.
func TestSearchResultsSayWhichPageIsAHomePage(t *testing.T) {
	e := newEnv(t)
	owner := e.user("srh@example.com", identity.PermUseProduct, identity.PermCreateSpace)
	space, home := owner.space("SRH")
	owner.ok("POST", "/api/v1/pages/"+home+"/publish", map[string]any{"title": "Welcome gnuword", "document": para("gnuword home"),
		"base_version": owner.ok("GET", "/api/v1/pages/"+home, nil)["current_version"]})
	page := owner.page(space, home, "Plain gnuword", "gnuword plain")
	homeFound, homeRow := searchFinds(owner, home, "gnuword")
	pageFound, pageRow := searchFinds(owner, page, "gnuword")
	if !homeFound || !pageFound || homeRow["home"] != true || pageRow["home"] != false || pageRow["kind"] != "page" {
		t.Fatalf("home marks: home %v, page %v", homeRow, pageRow)
	}
}
