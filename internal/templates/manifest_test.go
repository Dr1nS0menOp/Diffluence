// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/permissions"
)

func pageEntry(key, parent string) Entry {
	id := uuid.New()
	return Entry{Key: key, Name: "Page " + key, Kind: EntryPage, Parent: parent, PageID: &id, PageVersion: 1}
}

func manifestOf(entries ...Entry) Manifest {
	root := pageEntry(RootKey, "")
	root.Name = ""
	return Manifest{Entries: append([]Entry{root}, entries...)}
}

func TestAManifestRefusesWhatCannotBeMade(t *testing.T) {
	cases := map[string]Manifest{
		"no first page":         {Entries: []Entry{pageEntry("a", "")}},
		"a parent not in it":    manifestOf(pageEntry("a", "nowhere")),
		"a loop":                manifestOf(pageEntry("a", "b"), pageEntry("b", "a")),
		"a key twice":           manifestOf(pageEntry("a", RootKey), pageEntry("a", RootKey)),
		"an entry without name": manifestOf(Entry{Key: "a", Kind: EntryPage, Parent: RootKey, PageID: new(uuid.UUID), PageVersion: 1}),
		"a page without text":   manifestOf(Entry{Key: "a", Name: "A", Kind: EntryPage, Parent: RootKey}),
		"a folder with text":    manifestOf(Entry{Key: "a", Name: "A", Kind: EntryFolder, Parent: RootKey, PageID: new(uuid.UUID)}),
		"a kind of its own":     manifestOf(Entry{Key: "a", Name: "A", Kind: "post", Parent: RootKey}),
		"a restriction for everyone": manifestOf(Entry{Key: "a", Name: "A", Kind: EntryFolder, Parent: RootKey,
			Restrictions: Restrictions{View: []permissions.Subject{{Type: permissions.SubjectAllMembers}}}}),
	}
	for name, m := range cases {
		if err := m.validate(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestAManifestHoldsAHundredEntriesAndSixLevels(t *testing.T) {
	wide := manifestOf()
	for i := range maxEntries - 1 {
		wide.Entries = append(wide.Entries, pageEntry(fmt.Sprintf("e%d", i), RootKey))
	}
	if err := wide.validate(); err != nil {
		t.Fatalf("100 entries: %v", err)
	}
	wide.Entries = append(wide.Entries, pageEntry("one-more", RootKey))
	if err := wide.validate(); err == nil || !strings.Contains(err.Error(), "100") {
		t.Fatalf("101 entries: %v", err)
	}
	deep := manifestOf()
	parent := RootKey
	for i := range maxLevels {
		key := fmt.Sprintf("l%d", i+1)
		deep.Entries = append(deep.Entries, pageEntry(key, parent))
		parent = key
	}
	if err := deep.validate(); err != nil {
		t.Fatalf("6 levels: %v", err)
	}
	deep.Entries = append(deep.Entries, pageEntry("l7", parent))
	if err := deep.validate(); err == nil {
		t.Fatal("7 levels were accepted")
	}
}

func TestEntriesAreMadeAfterTheirParentInTheTemplatesOrder(t *testing.T) {
	m := manifestOf(pageEntry("child-of-b", "b"), pageEntry("a", RootKey), pageEntry("b", RootKey), pageEntry("child-of-a", "a"))
	var keys []string
	for _, e := range m.ordered() {
		keys = append(keys, e.Key)
	}
	if got := strings.Join(keys, " "); got != "root a child-of-a b child-of-b" {
		t.Fatalf("order: %s", got)
	}
}

func TestTheTextsOfATemplateStayWithinTheirBounds(t *testing.T) {
	m := manifestOf(pageEntry("a", RootKey))
	fields := func(from, n int) document.Node {
		doc := document.Node{Type: document.TypeDoc, Content: []document.Node{{Type: document.TypeParagraph}}}
		for i := range n {
			doc.Content[0].Content = append(doc.Content[0].Content, document.Node{Type: document.TypeTemplateField,
				Attrs: map[string]any{"name": fmt.Sprintf("Field %d", from+i), "kind": "line"}})
		}
		return doc
	}
	texts := map[string]document.Node{RootKey: fields(0, 20), "a": fields(20, 20)}
	if err := m.checkTexts(texts, nil); err != nil {
		t.Fatalf("40 fields: %v", err)
	}
	texts["a"] = fields(20, 21)
	if err := m.checkTexts(texts, nil); err == nil {
		t.Fatal("41 fields were accepted")
	}
	texts["a"] = fields(0, 20) // the same names are the same questions
	if err := m.checkTexts(texts, nil); err != nil {
		t.Fatalf("shared names: %v", err)
	}
	if err := m.checkTexts(texts, map[string]int{RootKey: maxTemplateText / 2, "a": maxTemplateText / 2}); err != nil {
		t.Fatalf("5 MiB: %v", err)
	}
	if err := m.checkTexts(texts, map[string]int{RootKey: maxTemplateText / 2, "a": maxTemplateText/2 + 1}); err == nil {
		t.Fatal("more than 5 MiB was accepted")
	}
	link := document.Node{Type: document.TypeDoc, Content: []document.Node{{Type: document.TypeParagraph, Content: []document.Node{
		{Type: document.TypeTemplateValue, Attrs: map[string]any{"source": "entry:gone"}}}}}}
	if err := m.checkTexts(map[string]document.Node{RootKey: link}, nil); err == nil {
		t.Fatal("a link to an entry that is not there was accepted")
	}
}

func TestATemplateNeedsTheGuidedPartForAnyOfItsParts(t *testing.T) {
	plain := manifestOf()
	if plain.usesGuidedParts(nil) {
		t.Fatal("a plain template")
	}
	group := uuid.New()
	for name, m := range map[string]Manifest{
		"two pages":  manifestOf(pageEntry("a", RootKey)),
		"publishing": {Entries: plain.Entries, Publish: true},
		"restricted": {Entries: []Entry{{Key: RootKey, Kind: EntryPage, Restrictions: Restrictions{Edit: []permissions.Subject{{Type: "group", ID: group}}}}}},
		"a workflow": {Entries: []Entry{{Key: RootKey, Kind: EntryPage, WorkflowID: &group}}},
	} {
		if !m.usesGuidedParts(nil) {
			t.Errorf("%s", name)
		}
	}
	value := document.Node{Type: document.TypeDoc, Content: []document.Node{{Type: document.TypeParagraph, Content: []document.Node{
		{Type: document.TypeTemplateValue, Attrs: map[string]any{"source": "date"}}}}}}
	if !plain.usesGuidedParts(map[string]document.Node{RootKey: value}) {
		t.Fatal("a value of the template")
	}
}

func TestALongTitleIsCutAtAWord(t *testing.T) {
	short := strings.Repeat("a", maxTitleRunes)
	if cutTitle(short) != short {
		t.Fatal("a title at the limit was cut")
	}
	long := strings.Repeat("word ", 60)
	got := cutTitle(long)
	if n := len([]rune(got)); n > maxTitleRunes || !strings.HasSuffix(got, "word…") {
		t.Fatalf("%d runes: %q", n, got)
	}
}
