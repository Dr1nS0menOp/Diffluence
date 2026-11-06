// SPDX-License-Identifier: Apache-2.0

package document

import (
	"fmt"
	"strings"
	"testing"
)

var changeLabels = ChangeLabels{Added: "Added", Removed: "Removed", More: "And {count} more changes"}

func paragraphs(texts ...string) Node {
	n := Node{Type: TypeDoc}
	for _, text := range texts {
		n.Content = append(n.Content, Node{Type: TypeParagraph, Content: []Node{{Type: TypeText, Text: text}}})
	}
	return n
}

func TestChangesLeaveOutWhatStayedTheSame(t *testing.T) {
	blocks := Compare(paragraphs("same", "old words here"), paragraphs("same", "new words here", "brand new"))
	text := ChangesText(blocks, changeLabels)
	if strings.Contains(text, "same") {
		t.Fatalf("an unchanged block is in the mail: %q", text)
	}
	for _, want := range []string{"[-old-]", "{+new+}", "Added: brand new"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	html := ChangesHTML(blocks, changeLabels)
	for _, want := range []string{"<del", ">old</del>", "<ins", ">new</ins>", "Added"} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %q", want, html)
		}
	}
}

func TestChangesSayNothingWhenNothingChanged(t *testing.T) {
	blocks := Compare(paragraphs("same"), paragraphs("same"))
	if got := ChangesText(blocks, changeLabels) + ChangesHTML(blocks, changeLabels); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestChangesEscapeEverythingFromThePage(t *testing.T) {
	hostile := `<script>alert("x")</script> & <img src=x onerror=y>`
	for name, blocks := range map[string][]DiffBlock{
		"added":   Compare(paragraphs(), paragraphs(hostile)),
		"removed": Compare(paragraphs(hostile), paragraphs()),
		"changed": Compare(paragraphs("a "+hostile), paragraphs("b "+hostile+" <b>more</b>")),
	} {
		html := ChangesHTML(blocks, ChangeLabels{Added: "<i>Added</i>", Removed: "<i>Removed</i>", More: "<i>{count}</i>"})
		if strings.Contains(html, "<script") || strings.Contains(html, "<img") || strings.Contains(html, "<b>") || strings.Contains(html, "<i>") {
			t.Fatalf("%s: not escaped: %s", name, html)
		}
		if !strings.Contains(html, "&lt;script&gt;") {
			t.Fatalf("%s: the text is gone: %s", name, html)
		}
	}
}

func TestChangesStopAfterALimitAndSayHowManyMore(t *testing.T) {
	var lines []string
	for i := range maxMailChanges + 3 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	blocks := Compare(paragraphs(), paragraphs(lines...))
	text := ChangesText(blocks, changeLabels)
	if got := strings.Count(text, "Added:"); got != maxMailChanges {
		t.Fatalf("%d changes shown", got)
	}
	if !strings.Contains(text, "And 3 more changes") {
		t.Fatalf("no line about the rest: %q", text)
	}
	if !strings.Contains(ChangesHTML(blocks, changeLabels), "And 3 more changes") {
		t.Fatal("no line about the rest in the HTML")
	}
}

func TestComparisonCarriesTheBlocksThemselves(t *testing.T) {
	got := Compare(paragraphs("same", "old words", "gone"), paragraphs("same", "new words"))
	if got[0].Node == nil || got[0].Previous != nil {
		t.Fatalf("equal: %+v", got[0])
	}
	if got[1].Op != OpChanged || BlockText(*got[1].Node) != "new words" || BlockText(*got[1].Previous) != "old words" {
		t.Fatalf("changed: %+v", got[1])
	}
	if got[2].Op != OpRemoved || BlockText(*got[2].Node) != "gone" {
		t.Fatalf("removed: %+v", got[2])
	}
}
