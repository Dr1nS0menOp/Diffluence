// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Spec 074, wave GT5: search (TPL-700 to TPL-703). Search never shows a template, nor a page,
// outside who may use or see it. The refusals come first.

func foundTitles(u *user, query string) []string {
	u.e.t.Helper()
	var titles []string
	for _, r := range u.ok("GET", "/api/v1/search?q="+url.QueryEscape(query), nil)["results"].([]any) {
		titles = append(titles, r.(map[string]any)["title"].(string))
	}
	return titles
}

func gallerySearch(u *user, space, text string) []string {
	u.e.t.Helper()
	var names []string
	for _, x := range u.ok("GET", "/api/v1/spaces/"+space+"/templates?q="+url.QueryEscape(text), nil)["templates"].([]any) {
		names = append(names, x.(map[string]any)["name"].(string))
	}
	return names
}

func TestTheGallerySearchNeverReturnsATemplateOutsideItsAudience(t *testing.T) {
	tt := newTemplateTeam(t)
	for i := range 150 {
		id := tt.owner.guided(tt.space, fmt.Sprintf("Report %03d", i), "x")
		if i%15 == 0 {
			tt.owner.ok("PUT", "/api/v1/templates/"+id+"/rights", map[string]any{"use": []any{userSubject(tt.colleague)}})
		}
	}
	listed := tt.owner.guided(tt.space, "Quarterly close", "x")
	tt.owner.entry(listed, map[string]any{"name": "Budget sheet", "parent": "root", "document": para("x")})
	tt.owner.ok("PUT", "/api/v1/templates/"+listed+"/rights", map[string]any{"use": []any{userSubject(tt.colleague)}})
	// Off the users list, a template is never found, by its name or the names of its pages.
	for _, text := range []string{"Report 000", "budget", "quarterly"} {
		if got := gallerySearch(tt.editor, tt.space, text); len(got) != 0 {
			t.Fatalf("%q found for someone off the list: %v", text, got)
		}
	}
	if got := gallerySearch(tt.editor, tt.space, "report"); len(got) != 140 {
		t.Fatalf("the editor finds %d reports, not the 140 offered to them", len(got))
	}
	// On it, the name of a page finds the template (TPL-701).
	if got := gallerySearch(tt.colleague, tt.space, "budget"); len(got) != 1 || got[0] != "Quarterly close" {
		t.Fatalf("the name of a page: %v", got)
	}
	if got := gallerySearch(tt.colleague, tt.space, "report"); len(got) != 150 {
		t.Fatalf("the colleague finds %d reports", len(got))
	}
	// Signs of LIKE are letters here.
	if got := gallerySearch(tt.colleague, tt.space, "%"); len(got) != 0 {
		t.Fatalf("a percent sign matched everything: %d", len(got))
	}
	tt.colleague.want(http.StatusBadRequest, "GET", "/api/v1/spaces/"+tt.space+"/templates?q="+strings.Repeat("x", 101), nil)
}

func TestTemplateInSearchFindsOnlyPagesFromTemplatesTheReaderMaySee(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl, _, _ := publishedSet(tt, "Outage review")
	tt.owner.page(tt.space, nil, "Outage by hand", "x")
	tt.e.settle()
	if got := foundTitles(tt.viewer, `template:"Outage review"`); len(got) != 2 {
		t.Fatalf("the pages of a set: %v", got)
	}
	if got := foundTitles(tt.viewer, `outage template:"Outage review"`); len(got) != 1 || got[0] != "Outage review one" {
		t.Fatalf("words and the filter: %v", got)
	}
	// A name the reader may not see finds nothing and gives no error (TPL-703, ORG-032).
	tt.owner.ok("PUT", "/api/v1/templates/"+tpl+"/rights", map[string]any{"use": []any{userSubject(tt.colleague)}})
	if got := foundTitles(tt.viewer, `template:"Outage review"`); len(got) != 0 {
		t.Fatalf("a template the reader may not see filtered: %v", got)
	}
	if got := foundTitles(tt.viewer, `outage template:"Outage review"`); len(got) != 0 {
		t.Fatalf("words and a template the reader may not see: %v", got)
	}
	if got := foundTitles(tt.viewer, `template:Nothing`); len(got) != 0 {
		t.Fatalf("an unknown name: %v", got)
	}
	if got := foundTitles(tt.colleague, `template:"outage review"`); len(got) != 2 {
		t.Fatalf("the name ignores capitals: %v", got)
	}
}

func TestTemplatesAndTheirEntriesAreNeverIndexedAndSetsOnlyWhenPublished(t *testing.T) {
	tt := newTemplateTeam(t)
	tpl := tt.owner.guided(tt.space, "Hidden words", "Rootword")
	tt.owner.entry(tpl, map[string]any{"name": "Child", "parent": "root", "document": para("Childword")})
	tt.e.settle()
	for _, word := range []string{"Rootword", "Childword", "Hidden"} {
		if got := foundTitles(tt.owner, word); len(got) != 0 {
			t.Fatalf("%s of a template was found (TPL-700): %v", word, got)
		}
	}
	// A set made as drafts is not found, by anyone; published, it is (TPL-702).
	if status, out := tt.owner.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Drafted"}); status != http.StatusCreated {
		t.Fatalf("a set: %d %v", status, out)
	}
	tt.e.settle()
	if got := foundTitles(tt.owner, "Childword"); len(got) != 0 {
		t.Fatalf("a set of drafts was found: %v", got)
	}
	tt.owner.want(http.StatusNoContent, "PUT", "/api/v1/templates/"+tpl+"/publishing", map[string]any{"publish": true})
	tt.owner.makeSet(tpl, map[string]any{"space_id": tt.space, "title": "Published"})
	tt.e.settle()
	if got := foundTitles(tt.viewer, "Childword"); len(got) != 1 {
		t.Fatalf("a published set: %v", got)
	}
}
