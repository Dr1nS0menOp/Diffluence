// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/document"
	"github.com/kenwerk/kenwerk/internal/pages"
	"github.com/kenwerk/kenwerk/internal/spaces"
)

// TestAStartLeadsEveryReferenceToTheCopiesAndTakesNothingAway is PUB-1310 and D57: links, cards,
// blocks, files and the space's key in a copy lead to the copies and the draft space; what a
// release would take away, such as a placeholder link, stays; and a page of another space keeps
// its link.
func TestAStartLeadsEveryReferenceToTheCopiesAndTakesNothingAway(t *testing.T) {
	from, to := uuid.New(), uuid.New()
	page, copied, other := uuid.New(), uuid.New(), uuid.New()
	file, fileCopy := uuid.New(), uuid.New()
	m := startMap{from: spaceRef{ID: from, Key: "OLD"}, to: spaceRef{ID: to, Key: "NEWDRAFT"},
		pages: map[uuid.UUID]uuid.UUID{page: copied}, files: map[uuid.UUID]uuid.UUID{file: fileCopy}}
	source := doc(
		para(link(page, "top"), document.Node{Type: document.TypeText, Text: " and "}, link(other, "")),
		document.Node{Type: document.TypePageCard, Attrs: map[string]any{"page_id": page.String()}},
		document.Node{Type: document.TypeDynamic, Attrs: map[string]any{"kind": "link_cards",
			"settings": map[string]any{"pages": []any{page.String(), other.String()}}}},
		document.Node{Type: document.TypeImage, Attrs: map[string]any{"attachment_id": file.String()}},
		para(document.Node{Type: document.TypeText, Text: "the space", Marks: []document.Mark{{Type: document.MarkLink,
			Attrs: map[string]any{"href": "/spaces/OLD/pages"}}}}),
		para(document.Node{Type: document.TypePlaceholderLink, Attrs: map[string]any{"title": "fill this in"}}),
	)
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	normal, err := document.Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, links, err := startDocument(normal, m)
	if err != nil {
		t.Fatal(err)
	}
	got := marshal(t, out)
	for _, want := range []string{copied.String(), fileCopy.String(), other.String(), "/spaces/NEWDRAFT/pages", "fill this in", `"fragment":"top"`} {
		if !strings.Contains(got, want) {
			t.Errorf("the copy lacks %s: %s", want, got)
		}
	}
	for _, gone := range []string{page.String(), file.String(), "/spaces/OLD"} {
		if strings.Contains(got, gone) {
			t.Errorf("the copy still names %s: %s", gone, got)
		}
	}
	// Verification does the start again from the map alone, and gets the same copy.
	again, err := startFromMap(normal, links)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != got {
		t.Fatalf("the start done again differs:\n%s\n%s", again, got)
	}
	delete(links, mapStartPage+page.String())
	if changed, err := startFromMap(normal, links); err != nil || string(changed) == got {
		t.Fatalf("a map without its page changed nothing: %v", err)
	}
}

// TestAStartDoneAgainFromItsMapIsTheSameOverRandomDocuments is PUB-920: whatever a document
// holds, the start done again from the map it kept gives exactly the copy it made.
func TestAStartDoneAgainFromItsMapIsTheSameOverRandomDocuments(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	from := uuid.New()
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	m := startMap{from: spaceRef{ID: from, Key: "OLD"}, to: spaceRef{ID: uuid.New(), Key: "NEW"},
		pages: map[uuid.UUID]uuid.UUID{ids[0]: uuid.New(), ids[1]: uuid.New()}, files: map[uuid.UUID]uuid.UUID{ids[2]: uuid.New()}}
	anyID := func() uuid.UUID { return ids[rng.IntN(len(ids))] }
	for round := range 300 {
		block := func() document.Node {
			switch rng.IntN(4) {
			case 0:
				return document.Node{Type: document.TypePageCard, Attrs: map[string]any{"page_id": anyID().String()}}
			case 1:
				return document.Node{Type: document.TypeImage, Attrs: map[string]any{"attachment_id": anyID().String()}}
			case 2:
				return para(document.Node{Type: document.TypeText, Text: "s", Marks: []document.Mark{{Type: document.MarkLink,
					Attrs: map[string]any{"href": "/spaces/" + from.String() + "/x"}}}})
			}
			return para(link(anyID(), ""), document.Node{Type: document.TypeText, Text: " words"})
		}
		raw, err := json.Marshal(doc(block(), block(), block()))
		if err != nil {
			t.Fatal(err)
		}
		normal, err := document.Normalize(raw)
		if err != nil {
			t.Fatal(err)
		}
		out, links, err := startDocument(normal, m)
		if err != nil {
			t.Fatal(err)
		}
		again, err := startFromMap(normal, links)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != marshal(t, out) {
			t.Fatalf("round %d: the start done again differs", round)
		}
	}
}

// TestAStartCopiesParentsFirstAndCountsWhatStays is PUB-1302 and D53: published pages, folders
// and links are copied with their parents before them, in the order of their siblings, the home
// page first; posts, databases, archived and unpublished pages are counted and stay, and a page
// below one that stays goes under the nearest copied page above.
func TestAStartCopiesParentsFirstAndCountsWhatStays(t *testing.T) {
	home, guide, archived, below, post, db, draft, link := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	when := time.Now()
	sp := spaces.Space{ID: uuid.New(), HomePageID: &home}
	page := func(id uuid.UUID, parent *uuid.UUID, kind, status string, pos int) pages.Page {
		return pages.Page{ID: id, ParentID: parent, Kind: kind, Status: status, Position: pos}
	}
	arch := page(archived, &home, pages.KindPage, "published", 2)
	arch.ArchivedAt = &when
	list := []pages.Page{
		page(guide, &home, pages.KindPage, "published", 1),
		arch,
		page(below, &draft, pages.KindPage, "published", 0),
		page(post, nil, pages.KindPost, "published", 0),
		page(db, &home, pages.KindDatabase, "published", 3),
		page(draft, &home, pages.KindPage, "draft", 4),
		page(link, &guide, pages.KindLink, "published", 0),
		page(home, nil, pages.KindPage, "published", 0),
	}
	plan := planStart(sp, list)
	order := []uuid.UUID{}
	for _, p := range plan.copies {
		order = append(order, p.ID)
	}
	want := []uuid.UUID{home, guide, link, below}
	if len(order) != len(want) {
		t.Fatalf("copied %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("copied %v, want %v", order, want)
		}
	}
	if plan.posts != 1 || plan.databases != 1 || plan.archived != 1 || plan.unpublished != 1 {
		t.Fatalf("what stays: %+v", plan)
	}
	if above := plan.parent[below]; above == nil || *above != home {
		t.Fatalf("a page below an unpublished one goes under %v, want the home page", above)
	}
}
