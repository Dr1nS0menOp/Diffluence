// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"encoding/json"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/pages"
)

// world is a draft space with a released page, a page not released yet, a database, a page of
// another space and a file, as a rewrite sees them.
type world struct {
	in                                   rewriteInput
	self, twin, released, releasedTwin   uuid.UUID
	unreleased, database, elsewhere      uuid.UUID
	file, fileCopy, foreignFile, oldFile uuid.UUID
}

func newWorld() world {
	w := world{self: uuid.New(), twin: uuid.New(), released: uuid.New(), releasedTwin: uuid.New(), unreleased: uuid.New(),
		database: uuid.New(), elsewhere: uuid.New(), file: uuid.New(), fileCopy: uuid.New(), foreignFile: uuid.New(), oldFile: uuid.New()}
	draft, other := uuid.New(), uuid.New()
	w.in = rewriteInput{
		draft:     spaceRef{ID: draft, Key: "DRAFTKEY"},
		published: spaceRef{ID: uuid.New(), Key: "PUBKEY"},
		twins:     map[uuid.UUID]uuid.UUID{w.self: w.twin, w.released: w.releasedTwin, w.database: uuid.New()},
		targets: map[uuid.UUID]pages.LinkTarget{
			w.self:       {SpaceID: draft, Title: "Self", Kind: pages.KindPage},
			w.released:   {SpaceID: draft, Title: "Released", Kind: pages.KindPage},
			w.unreleased: {SpaceID: draft, Title: "Not yet", Kind: pages.KindPage},
			w.database:   {SpaceID: draft, Title: "Rows", Kind: pages.KindDatabase},
			w.elsewhere:  {SpaceID: other, Title: "Elsewhere", Kind: pages.KindPage},
		},
		files:   map[uuid.UUID]uuid.UUID{w.file: w.fileCopy},
		foreign: map[uuid.UUID]bool{w.foreignFile: true},
	}
	return w
}

func para(content ...document.Node) document.Node {
	return document.Node{Type: document.TypeParagraph, Content: content}
}

func doc(blocks ...document.Node) document.Node {
	return document.Node{Type: document.TypeDoc, Content: blocks}
}

func link(id uuid.UUID, fragment string) document.Node {
	attrs := map[string]any{"page_id": id.String()}
	if fragment != "" {
		attrs["fragment"] = fragment
	}
	return document.Node{Type: document.TypePageLink, Attrs: attrs}
}

func marshal(t *testing.T, n document.Node) string {
	t.Helper()
	raw, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestALinkToAReleasedPageNamesItsTwinAndKeepsItsHeading(t *testing.T) {
	w := newWorld()
	out := rewrite(doc(para(link(w.released, "steps"), link(w.self, "top"))), w.in)
	got := marshal(t, out.doc)
	for _, want := range []string{`"page_id":"` + w.releasedTwin.String() + `","fragment":"steps"`, `"page_id":"` + w.twin.String() + `","fragment":"top"`} {
		if !containsAttrs(out.doc, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, w.released.String()) || strings.Contains(got, w.self.String()) {
		t.Fatalf("a draft id survived: %s", got)
	}
	if out.linkMap[w.released.String()] != w.releasedTwin.String() {
		t.Fatalf("the link map misses the rewrite: %v", out.linkMap)
	}
}

// containsAttrs looks for a node whose page_id and fragment are as the want string says.
func containsAttrs(n document.Node, want string) bool {
	found := false
	document.Walk(n, func(c document.Node) {
		id, _ := c.Attrs["page_id"].(string)
		fragment, _ := c.Attrs["fragment"].(string)
		if want == `"page_id":"`+id+`","fragment":"`+fragment+`"` {
			found = true
		}
	})
	return found
}

func TestALinkToAPageWithoutATwinBecomesItsTitle(t *testing.T) {
	w := newWorld()
	card := document.Node{Type: document.TypePageCard, Attrs: map[string]any{"page_id": w.unreleased.String()}}
	gone := uuid.New()
	out := rewrite(doc(para(link(w.unreleased, "")), card, para(link(gone, ""))), w.in)
	got := marshal(t, out.doc)
	if strings.Contains(got, w.unreleased.String()) || strings.Contains(got, gone.String()) {
		t.Fatalf("an id of a page readers cannot have survived: %s", got)
	}
	if !strings.Contains(got, "Not yet") || !strings.Contains(got, document.LinkedPageWords) {
		t.Fatalf("the links did not become their words: %s", got)
	}
	if len(out.textLinks) != 1 || out.textLinks[0] != w.unreleased {
		t.Fatalf("links that stayed text: %v", out.textLinks)
	}
}

func TestLinksToOtherSpacesAndOutsideAddressesStay(t *testing.T) {
	w := newWorld()
	outside := document.Node{Type: document.TypeText, Text: "site", Marks: []document.Mark{{Type: document.MarkLink, Attrs: map[string]any{"href": "https://example.org/x"}}}}
	mail := document.Node{Type: document.TypeText, Text: "mail", Marks: []document.Mark{{Type: document.MarkLink, Attrs: map[string]any{"href": "mailto:a@example.org"}}}}
	in := doc(para(link(w.elsewhere, "a"), outside, mail))
	out := rewrite(in, w.in)
	if marshal(t, out.doc) != marshal(t, in) {
		t.Fatalf("a link outside the pair changed:\n%s\n%s", marshal(t, out.doc), marshal(t, in))
	}
}

func TestAddressesIntoTheDraftSpaceAreRewritten(t *testing.T) {
	w := newWorld()
	text := func(href string) document.Node {
		return document.Node{Type: document.TypeText, Text: "x", Marks: []document.Mark{{Type: document.MarkLink, Attrs: map[string]any{"href": href}}}}
	}
	out := rewrite(doc(para(text("/pages/"+w.released.String()+"#steps"), text("/pages/"+w.unreleased.String()),
		text("/spaces/draftkey/overview"), text("/spaces/"+w.in.draft.ID.String()))), w.in)
	got := marshal(t, out.doc)
	for _, want := range []string{"/pages/" + w.releasedTwin.String() + "#steps", "/spaces/PUBKEY/overview", `"/spaces/PUBKEY"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, w.unreleased.String()) || strings.Contains(strings.ToLower(got), "draftkey") || strings.Contains(got, w.in.draft.ID.String()) {
		t.Fatalf("a draft address survived: %s", got)
	}
}

func TestDraftingMarksAndPlaceholdersAreRemoved(t *testing.T) {
	w := newWorld()
	instruction := document.Node{Type: document.TypeInstruction, Attrs: map[string]any{"text": "Write the scope here"}}
	placeholder := document.Node{Type: document.TypePlaceholderLink, Attrs: map[string]any{"title": "Future page"}}
	out := marshal(t, rewrite(doc(para(instruction, placeholder)), w.in).doc)
	if strings.Contains(out, "Write the scope") || strings.Contains(out, document.TypePlaceholderLink) || !strings.Contains(out, "Future page") {
		t.Fatalf("instruction or placeholder: %s", out)
	}
}

func TestBlocksThatNamePagesFollowTheirTwinsOrSayNotAvailable(t *testing.T) {
	w := newWorld()
	dynamic := func(kind string, settings map[string]any) document.Node {
		return document.Node{Type: document.TypeDynamic, Attrs: map[string]any{"kind": kind, "settings": settings}}
	}
	out := rewrite(doc(
		dynamic("child_pages", map[string]any{"page": w.released.String()}),
		dynamic(document.KindEmbeddedPage, map[string]any{"page": w.unreleased.String()}),
		dynamic(document.KindShowExcerpt, map[string]any{"page": w.database.String(), "name": "a"}),
		dynamic("link_cards", map[string]any{"pages": []any{w.released.String(), w.unreleased.String()}}),
		dynamic("filter_list", map[string]any{"spaces": []any{"draftKEY", "OTHER"}, "labels": []any{"x"}}),
		dynamic(kindCreateFromTemplate, map[string]any{"template": uuid.NewString()}),
	), w.in)
	got := marshal(t, out.doc)
	if strings.Count(got, notAvailable) != 3 {
		t.Fatalf("want three blocks not available (an unreleased include, a database, a template): %s", got)
	}
	for _, gone := range []string{w.released.String(), w.unreleased.String(), w.database.String(), "draftKEY"} {
		if strings.Contains(got, gone) {
			t.Fatalf("%s survived: %s", gone, got)
		}
	}
	if strings.Count(got, w.releasedTwin.String()) != 2 || !strings.Contains(got, `"PUBKEY","OTHER"`) {
		t.Fatalf("blocks did not follow the twins and the published space: %s", got)
	}
}

func TestFilesPointAtTheirCopies(t *testing.T) {
	w := newWorld()
	file := func(typ string, id uuid.UUID) document.Node {
		return document.Node{Type: typ, Attrs: map[string]any{"attachment_id": id.String(), "filename": "a.pdf"}}
	}
	out := marshal(t, rewrite(doc(file(document.TypeFile, w.file), file(document.TypeImage, w.foreignFile), file(document.TypeFile, w.oldFile),
		para(file(document.TypeInlineImage, w.oldFile))), w.in).doc)
	if !strings.Contains(out, w.fileCopy.String()) || !strings.Contains(out, w.foreignFile.String()) {
		t.Fatalf("copies and files of other spaces: %s", out)
	}
	if strings.Contains(out, w.file.String()) || strings.Contains(out, w.oldFile.String()) {
		t.Fatalf("a draft file survived: %s", out)
	}
}

// FuzzRewriteLeavesNoDraftID is PUB-534: over random documents with every kind of reference,
// rewriting and the gate always succeed, and no id of the draft space survives.
func FuzzRewriteLeavesNoDraftID(f *testing.F) {
	for _, seed := range []uint64{1, 2, 3, 42, 1000} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, seed uint64) {
		w := newWorld()
		rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
		draftIDs := []uuid.UUID{w.self, w.released, w.unreleased, w.database, uuid.New()}
		anyID := func() uuid.UUID {
			if rng.IntN(4) == 0 {
				return w.elsewhere
			}
			return draftIDs[rng.IntN(len(draftIDs))]
		}
		inline := func() document.Node {
			switch rng.IntN(6) {
			case 0:
				return link(anyID(), "h")
			case 1:
				return document.Node{Type: document.TypeText, Text: "w", Marks: []document.Mark{{Type: document.MarkLink,
					Attrs: map[string]any{"href": "/pages/" + anyID().String() + "#x"}}}}
			case 2:
				return document.Node{Type: document.TypePlaceholderLink, Attrs: map[string]any{"title": "p"}}
			case 3:
				return document.Node{Type: document.TypeInstruction, Attrs: map[string]any{"text": "i"}}
			case 4:
				return document.Node{Type: document.TypeText, Text: "s", Marks: []document.Mark{{Type: document.MarkLink,
					Attrs: map[string]any{"href": "/spaces/DRAFTKEY/x"}}}}
			}
			return document.Node{Type: document.TypeText, Text: "words"}
		}
		var block func(depth int) document.Node
		block = func(depth int) document.Node {
			switch n := rng.IntN(7); {
			case n == 0:
				return document.Node{Type: document.TypePageCard, Attrs: map[string]any{"page_id": anyID().String()}}
			case n == 1:
				return document.Node{Type: document.TypeDynamic, Attrs: map[string]any{"kind": "child_pages", "settings": map[string]any{"page": anyID().String()}}}
			case n == 2:
				return document.Node{Type: document.TypeDynamic, Attrs: map[string]any{"kind": "link_cards",
					"settings": map[string]any{"pages": []any{anyID().String(), anyID().String()}, "below": anyID().String()}}}
			case n == 3:
				ids := []uuid.UUID{w.file, w.oldFile, w.foreignFile}
				return document.Node{Type: document.TypeImage, Attrs: map[string]any{"attachment_id": ids[rng.IntN(3)].String()}}
			case n == 4 && depth < 4:
				return document.Node{Type: document.TypeBlockquote, Content: []document.Node{block(depth + 1), block(depth + 1)}}
			}
			return para(inline(), inline(), inline())
		}
		in := doc(block(0), block(0), block(0), block(0))
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		normal, err := document.Normalize(raw)
		if err != nil {
			t.Fatalf("the generated document does not pass the gate: %v", err)
		}
		out := rewrite(normal, w.in)
		raw, err = json.Marshal(out.doc)
		if err != nil {
			t.Fatal(err)
		}
		again, err := document.Normalize(raw)
		if err != nil {
			t.Fatalf("a released document did not pass the gate: %v\n%s", err, raw)
		}
		got := marshal(t, again)
		for _, id := range append(draftIDs, w.file, w.oldFile, w.in.draft.ID) {
			if strings.Contains(got, id.String()) {
				t.Fatalf("draft id %s survived:\n%s", id, got)
			}
		}
		if strings.Contains(got, "DRAFTKEY") {
			t.Fatalf("the draft key survived:\n%s", got)
		}
	})
}

func TestATwinGoesUnderTheTwinOfItsNearestReleasedAncestor(t *testing.T) {
	parent, grandparent, home := uuid.New(), uuid.New(), uuid.New()
	homeTwin, grandTwin := uuid.New(), uuid.New()
	twins := map[uuid.UUID]uuid.UUID{home: homeTwin, grandparent: grandTwin}
	if got := placeTwin([]uuid.UUID{grandparent, home}, twins); got.Parent == nil || *got.Parent != grandTwin || got.Higher {
		t.Fatalf("under a released parent: %+v", got)
	}
	if got := placeTwin([]uuid.UUID{parent, grandparent, home}, twins); got.Parent == nil || *got.Parent != grandTwin || !got.Higher {
		t.Fatalf("under an unreleased parent: %+v", got)
	}
	if got := placeTwin(nil, twins); got.Parent != nil || got.Higher {
		t.Fatalf("at the top: %+v", got)
	}
	if got := placeTwin([]uuid.UUID{parent}, twins); got.Parent != nil || !got.Higher {
		t.Fatalf("no released ancestor: %+v", got)
	}
}

// TestNoCycleArisesOverRandomTrees is PUB-1006: over random draft trees released in random
// order, every twin's parent is the twin of a draft ancestor, and the published tree has no
// cycle.
func TestNoCycleArisesOverRandomTrees(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	for round := range 200 {
		size := 2 + rng.IntN(40)
		draft := make([]uuid.UUID, size)
		parentOf := map[uuid.UUID]*uuid.UUID{}
		for i := range draft {
			draft[i] = uuid.New()
			if i > 0 && rng.IntN(5) > 0 {
				p := draft[rng.IntN(i)]
				parentOf[draft[i]] = &p
			}
		}
		ancestors := func(id uuid.UUID) []uuid.UUID {
			var out []uuid.UUID
			for p := parentOf[id]; p != nil; p = parentOf[*p] {
				out = append(out, *p)
			}
			return out
		}
		twins := map[uuid.UUID]uuid.UUID{}
		twinParent := map[uuid.UUID]*uuid.UUID{}
		draftOf := map[uuid.UUID]uuid.UUID{}
		for _, i := range rng.Perm(size) {
			id := draft[i]
			place := placeTwin(ancestors(id), twins)
			twin := uuid.New()
			twins[id], twinParent[twin], draftOf[twin] = twin, place.Parent, id
			if place.Parent != nil {
				above := draftOf[*place.Parent]
				isAncestor := false
				for _, a := range ancestors(id) {
					isAncestor = isAncestor || a == above
				}
				if !isAncestor {
					t.Fatalf("round %d: a twin went under the twin of a page that is not its ancestor", round)
				}
			}
		}
		for twin := range twinParent {
			seen := map[uuid.UUID]bool{}
			for at := &twin; at != nil; at = twinParent[*at] {
				if seen[*at] {
					t.Fatalf("round %d: the published tree has a cycle", round)
				}
				seen[*at] = true
			}
		}
	}
}
