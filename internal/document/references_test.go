// SPDX-License-Identifier: Apache-2.0

package document

import (
	"slices"
	"testing"
)

// wordAttributes hold words a writer typed, which keep whatever was typed, an id too.
var wordAttributes = []string{"title", "caption", "alt", "facts_id", "name", "text", "button_text", "filename",
	"x_title", "y_title", "options", "original"}

// TestEveryAttributeThatHoldsAnIDIsListed is the guard of decision D7 of spec 073 (PUB-502):
// every attribute of every block that keeps an id after the gate is a page reference, a file,
// or one of the ids that name neither, so a release cannot forget to rewrite a new one.
func TestEveryAttributeThatHoldsAnIDIsListed(t *testing.T) {
	const id = "018f0000-0000-7000-8000-000000000001"
	candidates := []string{"page_id", "page", "pages", "space", "space_id", "space_key", "key", "target", "parent",
		"template", "id", "attachment_id", "root", "below", "from", "source", "ref", "link", "href", "table", "view", "database"}
	for _, settings := range dynamicKinds {
		for key := range settings {
			if !slices.Contains(candidates, key) {
				candidates = append(candidates, key)
			}
		}
	}
	for nodeType, cleaner := range attrCleaners {
		out := map[string]any{}
		// Some blocks refuse without a known kind or name; give them one so the rest is read.
		for _, kind := range []string{"info", "video", "bar", "neutral"} {
			in := map[string]any{}
			for _, key := range candidates {
				in[key] = id
			}
			in["kind"], in["name"], in["filename"], in["text"], in["color"] = kind, "name", "a.pdf", "words", "red"
			kept := map[string]any{}
			if err := cleaner(in, kept); err == nil {
				for key, value := range kept {
					out[key] = value
				}
			}
		}
		for key, value := range out {
			if value != id {
				continue
			}
			page, _ := PageAttribute(nodeType)
			switch {
			case slices.Contains(wordAttributes, key):
			case key == page:
			case key == FileAttribute && ShowsFile(nodeType):
			case slices.Contains(otherIDAttributes[nodeType], key):
			default:
				t.Errorf("%s keeps an id in %q, which no list of references names", nodeType, key)
			}
		}
	}
	for kind := range dynamicKinds {
		single, lists := DynamicPageSettings(kind)
		for key, rule := range dynamicKinds[kind] {
			if rule.kind == settingText || rule.kind == settingList {
				continue // words and lists of words, such as labels, a heading or a person
			}
			if got := CleanSettings(kind, map[string]any{key: id}); got[key] == id && !slices.Contains(single, key) {
				t.Errorf("dynamic %s keeps a page in %q, which DynamicPageSettings does not name", kind, key)
			}
			if got := CleanSettings(kind, map[string]any{key: []any{id}}); got[key] != nil && dynamicKinds[kind][key].kind == settingPages &&
				!slices.Contains(lists, key) {
				t.Errorf("dynamic %s keeps pages in %q, which DynamicPageSettings does not name", kind, key)
			}
		}
	}
	for nodeType := range pageAttributes {
		if !slices.Contains(NodeTypes(), nodeType) {
			t.Errorf("%s is listed as a page reference and is no node type", nodeType)
		}
	}
}

// TestTheBlocksThatNamePagesAreFound: the lists name what a release must rewrite.
func TestTheBlocksThatNamePagesAreFound(t *testing.T) {
	single, lists := DynamicPageSettings("child_pages")
	if !slices.Equal(single, []string{"page"}) || len(lists) != 0 {
		t.Fatalf("child pages: %v %v", single, lists)
	}
	single, lists = DynamicPageSettings("link_cards")
	if !slices.Contains(single, "below") || !slices.Equal(lists, []string{"pages"}) {
		t.Fatalf("link cards: %v %v", single, lists)
	}
	if got := DynamicSpaceSettings("filter_list"); !slices.Equal(got, []string{"spaces"}) {
		t.Fatalf("filter list spaces: %v", got)
	}
	if got := DynamicSpaceSettings("toc"); len(got) != 0 {
		t.Fatalf("a table of contents names no space: %v", got)
	}
}
