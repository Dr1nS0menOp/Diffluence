// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"encoding/json"
	"math/rand/v2"
	"testing"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/document"
)

// TestAReleaseTakesTheApprovedVersionOfAControlledPage is PUB-700: of a controlled page a
// release takes the approved version, never the head under review; an ordinary page goes at
// its current version.
func TestAReleaseTakesTheApprovedVersionOfAControlledPage(t *testing.T) {
	approved := 3
	if got := releaseVersion(3, &controlFacts{approved: &approved, head: 5}); got != 3 {
		t.Fatalf("a page in review goes at version %d", got)
	}
	if got := releaseVersion(4, &controlFacts{approved: &approved, head: 5}); got != 3 {
		t.Fatalf("a page whose row says 4 goes at version %d, not its approved 3", got)
	}
	if got := releaseVersion(7, nil); got != 7 {
		t.Fatalf("an ordinary page goes at version %d", got)
	}
}

// TestControlledPagesAreLeftToApprovalWhenThePairAsks is PUB-714 in a run: a controlled page is
// skipped with its reason, an ordinary page goes.
func TestControlledPagesAreLeftToApprovalWhenThePairAsks(t *testing.T) {
	approved := 1
	controlled := scopeRow{ID: uuid.New(), Kind: "page", Status: "published", CurrentVersion: 1, Visible: true,
		Control: &controlFacts{approved: &approved, head: 1}}
	ordinary := scopeRow{ID: uuid.New(), Kind: "page", Status: "published", CurrentVersion: 1, Visible: true}
	rules := scopeRules{approvalOnly: true}
	if d := decide(controlled, rules, map[uuid.UUID]bool{}); d.action != actionSkip || d.reason != reasonApprovalOnly {
		t.Fatalf("a controlled page under only approval releases: %+v", d)
	}
	if d := decide(ordinary, rules, map[uuid.UUID]bool{}); d.action != actionMake {
		t.Fatalf("an ordinary page under only approval releases: %+v", d)
	}
	if d := decide(controlled, scopeRules{}, map[uuid.UUID]bool{}); d.action != actionMake {
		t.Fatalf("a controlled page without the setting: %+v", d)
	}
	for _, reason := range []string{reasonNotApproved, reasonApprovalOnly, reasonKind, reasonArchived} {
		if hidesBelow(reason) {
			t.Fatalf("a page skipped as %s keeps the pages below it out", reason)
		}
	}
	for _, reason := range []string{skipLeftOut, skipBelowLeft, skipRestricted} {
		if !hidesBelow(reason) {
			t.Fatalf("a page skipped as %s lets the pages below it go", reason)
		}
	}
}

// TestTheRewriteIsDoneAgainFromItsLinkMapAlone is PUB-732: over random documents, the rewrite
// done again from the link map a release kept gives exactly the document the twin got, and a
// map without its page links does not.
func TestTheRewriteIsDoneAgainFromItsLinkMapAlone(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 5))
	differed := 0
	for round := range 300 {
		w := newWorld()
		draftIDs := []uuid.UUID{w.self, w.released, w.unreleased, w.database, uuid.New(), w.elsewhere}
		anyID := func() uuid.UUID { return draftIDs[rng.IntN(len(draftIDs))] }
		block := func() document.Node {
			switch rng.IntN(5) {
			case 0:
				return document.Node{Type: document.TypePageCard, Attrs: map[string]any{"page_id": anyID().String()}}
			case 1:
				return document.Node{Type: document.TypeDynamic, Attrs: map[string]any{"kind": "link_cards",
					"settings": map[string]any{"pages": []any{anyID().String(), anyID().String()}}}}
			case 2:
				files := []uuid.UUID{w.file, w.oldFile, w.foreignFile}
				return document.Node{Type: document.TypeImage, Attrs: map[string]any{"attachment_id": files[rng.IntN(3)].String()}}
			case 3:
				return para(document.Node{Type: document.TypeText, Text: "s", Marks: []document.Mark{{Type: document.MarkLink,
					Attrs: map[string]any{"href": "/spaces/DRAFTKEY/x"}}}})
			}
			return para(link(anyID(), "h"), document.Node{Type: document.TypeText, Text: " words"})
		}
		raw, err := json.Marshal(doc(block(), block(), block()))
		if err != nil {
			t.Fatal(err)
		}
		draft, err := document.Normalize(raw)
		if err != nil {
			t.Fatal(err)
		}
		out := rewrite(draft, w.in)
		raw, err = json.Marshal(out.doc)
		if err != nil {
			t.Fatal(err)
		}
		twin, err := document.Normalize(raw)
		if err != nil {
			t.Fatal(err)
		}
		want := marshal(t, twin)
		again, err := rewriteFromMap(draft, out.linkMap)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != want {
			t.Fatalf("round %d: the rewrite done again differs:\n%s\n%s", round, again, want)
		}
		without := map[string]string{}
		for k, v := range out.linkMap {
			if _, err := uuid.Parse(k); err != nil {
				without[k] = v
			}
		}
		if len(without) < len(out.linkMap) {
			stripped, err := rewriteFromMap(draft, without)
			if err != nil {
				t.Fatal(err)
			}
			if string(stripped) == want {
				t.Fatalf("round %d: the page links of the map changed nothing", round)
			}
			differed++
		}
	}
	if differed == 0 {
		t.Fatal("no round had a page link to compare without")
	}
}
