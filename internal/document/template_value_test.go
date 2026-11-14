// SPDX-License-Identifier: Apache-2.0

package document

import (
	"slices"
	"strings"
	"testing"
)

func valueDocument(source string) string {
	return `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"See "},
		{"type":"templateValue","attrs":{"source":"` + source + `"}}]}]}`
}

func TestATemplateValueNeedsAKnownSource(t *testing.T) {
	for _, bad := range []string{"", "today", "entry:", "entry:" + strings.Repeat("k", 65), "entry:a b", "Entry:x"} {
		if _, err := Normalize([]byte(valueDocument(bad))); err == nil {
			t.Errorf("source %q was accepted", bad)
		}
	}
	for _, good := range []string{"date", "space", "creator", "set", "template", "entry:0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"} {
		if _, err := Normalize([]byte(valueDocument(good))); err != nil {
			t.Errorf("source %q: %v", good, err)
		}
	}
}

func TestTemplateValuesListsTheEntriesATemplateLinksTo(t *testing.T) {
	doc, err := Normalize([]byte(`{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"templateValue","attrs":{"source":"entry:b"}},{"type":"templateValue","attrs":{"source":"space"}},
		{"type":"templateValue","attrs":{"source":"entry:a"}},{"type":"templateValue","attrs":{"source":"entry:b"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := LinkedEntries(doc); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("linked entries: %v", got)
	}
	if !HasTemplateValues(doc) || HasTemplateValues(Empty()) {
		t.Fatal("HasTemplateValues")
	}
}

func TestAPageWithATemplateValueCannotBePublished(t *testing.T) {
	doc, err := Normalize([]byte(valueDocument("space")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ForPublish(doc); err == nil {
		t.Fatal("a template value reached a reader")
	}
}

func TestFillValuesPutsWhatTheTemplateKnowsInItsPlace(t *testing.T) {
	doc, err := Normalize([]byte(`{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"templateValue","attrs":{"source":"space"}},{"type":"templateValue","attrs":{"source":"entry:a"}},
		{"type":"templateValue","attrs":{"source":"entry:gone"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	filled := FillValues(doc, func(source string) (Node, bool) {
		switch source {
		case "space":
			return Node{Type: TypeText, Text: "Engineering"}, true
		case "entry:a":
			return Node{Type: TypePageLink, Attrs: map[string]any{"page_id": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"}}, true
		}
		return Node{}, false
	})
	if HasTemplateValues(filled) {
		t.Fatal("a value stayed")
	}
	raw, _ := Marshal(filled)
	if !strings.Contains(string(raw), "Engineering") || !strings.Contains(string(raw), "pageLink") {
		t.Fatalf("filled: %s", raw)
	}
	if strings.Count(string(raw), `"type":"text"`) != 1 {
		t.Fatalf("a value with no source must leave nothing behind: %s", raw)
	}
}
