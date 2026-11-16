// SPDX-License-Identifier: Apache-2.0

package document

import (
	"strings"
	"testing"
)

// firstLink finds the first link mark of a document.
func firstLink(n Node) (Mark, bool) {
	for _, m := range n.Marks {
		if m.Type == MarkLink {
			return m, true
		}
	}
	for _, c := range n.Content {
		if m, ok := firstLink(c); ok {
			return m, true
		}
	}
	return Mark{}, false
}

// A tooltip could name another address than the one a link opens, so a link keeps none.
func TestASavedLinkKeepsNoTitle(t *testing.T) {
	n, err := Normalize(doc(`{"type":"paragraph","content":[{"type":"text","text":"bank",
		"marks":[{"type":"link","attrs":{"href":"https://evil.example/","title":"https://bank.example/login"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	link, ok := firstLink(n)
	if !ok {
		t.Fatal("the link must stay")
	}
	if _, has := link.Attrs["title"]; has || link.Attrs["href"] != "https://evil.example/" {
		t.Fatalf("link attrs %v", link.Attrs)
	}
}

func TestAMarkdownLinkTitleIsNotImported(t *testing.T) {
	n, _ := FromMarkdown([]byte(`[bank](https://evil.example/ "https://bank.example/login")`), nil)
	link, ok := firstLink(n)
	if !ok {
		t.Fatal("the link must stay")
	}
	if _, has := link.Attrs["title"]; has {
		t.Fatalf("link attrs %v", link.Attrs)
	}
	if out := ToMarkdown(n, nil); strings.Contains(out, "bank.example") {
		t.Fatalf("the title came back in Markdown: %s", out)
	}
}
