// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const linkedID = "11111111-1111-4111-8111-111111111111"

func paragraphWith(inline string) []byte {
	return doc(`{"type":"paragraph","content":[` + inline + `]}`)
}

func TestAPageLinkNeedsAPage(t *testing.T) {
	for _, node := range []string{
		`{"type":"pageLink"}`,
		`{"type":"pageLink","attrs":{"page_id":"not-an-id"}}`,
		`{"type":"pageLink","attrs":{"page_id":""}}`,
	} {
		if _, err := Normalize(paragraphWith(node)); err == nil {
			t.Fatalf("%s got through", node)
		}
	}
	if _, err := Normalize(doc(`{"type":"pageCard","attrs":{"page_id":"x"}}`)); err == nil {
		t.Fatal("a card without a page got through")
	}
}

func TestAPageLinkKeepsOnlyWhatIsAllowed(t *testing.T) {
	n, err := Normalize(paragraphWith(`{"type":"pageLink","attrs":{"page_id":"` + linkedID +
		`","display":"tag","fragment":"next-steps","title":"A stored title","href":"javascript:alert(1)"}}`))
	if err != nil {
		t.Fatal(err)
	}
	attrs := n.Content[0].Content[0].Attrs
	if len(attrs) != 3 || attrs["display"] != "tag" || attrs["fragment"] != "next-steps" {
		t.Fatalf("got %v", attrs)
	}
}

func TestAFragmentCannotCarryMarkup(t *testing.T) {
	for _, bad := range []string{`\"><script>`, `a b`, `a/b`, `?x=1`, strings.Repeat("a", maxFragmentBytes+1)} {
		n, err := Normalize(paragraphWith(`{"type":"pageLink","attrs":{"page_id":"` + linkedID + `","fragment":"` + bad + `"}}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, kept := n.Content[0].Content[0].Attrs["fragment"]; kept {
			t.Fatalf("%q was kept", bad)
		}
	}
}

func TestAFragmentMayHoldEveryLetter(t *testing.T) {
	n, err := Normalize(paragraphWith(`{"type":"pageLink","attrs":{"page_id":"` + linkedID + `","fragment":"über-uns"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if n.Content[0].Content[0].Attrs["fragment"] != "über-uns" {
		t.Fatalf("got %v", n.Content[0].Content[0].Attrs)
	}
}

func TestALinkDisplayComesFromTheList(t *testing.T) {
	n, err := Normalize(paragraphWith(`{"type":"pageLink","attrs":{"page_id":"` + linkedID + `","display":"banner"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := n.Content[0].Content[0].Attrs["display"]; kept {
		t.Fatal("an unknown display was kept")
	}
}

func TestAPlaceholderNeedsATitle(t *testing.T) {
	for _, node := range []string{`{"type":"placeholderLink"}`, `{"type":"placeholderLink","attrs":{"title":"   "}}`} {
		if _, err := Normalize(paragraphWith(node)); err == nil {
			t.Fatalf("%s got through", node)
		}
	}
}

func TestLinkedPagesCountEveryKindOfLink(t *testing.T) {
	other := "22222222-2222-4222-8222-222222222222"
	third := "33333333-3333-4333-8333-333333333333"
	n, err := Normalize(doc(`{"type":"paragraph","content":[
		{"type":"pageLink","attrs":{"page_id":"` + linkedID + `"}},
		{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"/pages/` + other + `"}}]}]},
		{"type":"pageCard","attrs":{"page_id":"` + third + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := LinkedPages(n); len(got) != 3 {
		t.Fatalf("got %v", got)
	}
}

func TestAPageLinkAddsNoWordsToSearch(t *testing.T) {
	n, err := Normalize(paragraphWith(`{"type":"pageLink","attrs":{"page_id":"` + linkedID + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(Text(n)); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestPlaceholdersAreListedOnce(t *testing.T) {
	n, err := Normalize(paragraphWith(`{"type":"placeholderLink","attrs":{"title":"Notes"}},{"type":"placeholderLink","attrs":{"title":"Notes"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := Placeholders(n); len(got) != 1 || got[0] != "Notes" {
		t.Fatalf("got %v", got)
	}
}

func resolvedDoc(t *testing.T, title PageTitle) string {
	t.Helper()
	n, err := Normalize(doc(`{"type":"paragraph","content":[
		{"type":"pageLink","attrs":{"page_id":"` + linkedID + `","fragment":"steps"}},
		{"type":"placeholderLink","attrs":{"title":"Notes"}}]},
		{"type":"pageCard","attrs":{"page_id":"` + linkedID + `"}}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(ResolveLinks(n, title))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestResolvingUsesATitleOnlyWhenThereIsOne(t *testing.T) {
	shown := resolvedDoc(t, func(uuid.UUID) (string, bool) { return "Plans", true })
	if !strings.Contains(shown, `"text":"Plans"`) || !strings.Contains(shown, "/pages/"+linkedID+"#steps") {
		t.Fatalf("got %s", shown)
	}
	for _, title := range []PageTitle{nil, func(uuid.UUID) (string, bool) { return "Plans", false }} {
		hidden := resolvedDoc(t, title)
		if strings.Contains(hidden, "Plans") || strings.Contains(hidden, linkedID) {
			t.Fatalf("a hidden page shows: %s", hidden)
		}
		if !strings.Contains(hidden, LinkedPageWords) {
			t.Fatalf("got %s", hidden)
		}
	}
}

func TestNothingOfALinkTypeLeavesAResolvedDocument(t *testing.T) {
	got := resolvedDoc(t, nil)
	for _, name := range []string{TypePageLink, TypePageCard, TypePlaceholderLink} {
		if strings.Contains(got, name) {
			t.Fatalf("%s is still there: %s", name, got)
		}
	}
	if !strings.Contains(got, `"text":"Notes"`) {
		t.Fatalf("the words of the placeholder are gone: %s", got)
	}
}

func tableAround(cell string) string {
	return `{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[` + cell + `]}]}]}`
}

func TestATableInATableKeepsItsWordsAndLosesItsGrid(t *testing.T) {
	words := `{"type":"paragraph","content":[{"type":"text","text":"Inner words"}]}`
	n, err := Normalize(doc(tableAround(`{"type":"panel","attrs":{"kind":"info"},"content":[` + tableAround(words) + `]}`)))
	if err != nil {
		t.Fatal(err)
	}
	tables := 0
	Walk(n, func(c Node) {
		if c.Type == TypeTable {
			tables++
		}
	})
	if tables != 1 {
		t.Fatalf("%d tables", tables)
	}
	if !strings.Contains(Text(n), "Inner words") {
		t.Fatalf("the words are gone: %q", Text(n))
	}
}

func TestATableOutsideATableStays(t *testing.T) {
	cell := `{"type":"paragraph","content":[{"type":"text","text":"x"}]}`
	n, err := Normalize(doc(`{"type":"panel","attrs":{"kind":"info"},"content":[` + tableAround(cell) + `]},` + tableAround(cell)))
	if err != nil {
		t.Fatal(err)
	}
	tables := 0
	Walk(n, func(c Node) {
		if c.Type == TypeTable {
			tables++
		}
	})
	if tables != 2 {
		t.Fatalf("%d tables", tables)
	}
}

func TestATableKeepsItsSettings(t *testing.T) {
	n, err := Normalize(doc(`{"type":"table","attrs":{"numbered":true,"flexible":true,"caption":"Costs","caption_below":true,"style":"color:red"},
		"content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	attrs := n.Content[0].Attrs
	if len(attrs) != 4 || attrs["numbered"] != true || attrs["flexible"] != true || attrs["caption"] != "Costs" || attrs["caption_below"] != true {
		t.Fatalf("got %v", attrs)
	}
}

func TestACaptionIsEscapedInHTML(t *testing.T) {
	n, err := Normalize(doc(`{"type":"table","attrs":{"caption":"<b>Costs</b>"},
		"content":[{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph"}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := ToHTML(n, HTMLOptions{})
	if strings.Contains(got, "<b>") || !strings.Contains(got, "<caption>&lt;b&gt;Costs&lt;/b&gt;</caption>") {
		t.Fatalf("got %s", got)
	}
}
