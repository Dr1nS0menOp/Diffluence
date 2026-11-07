// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const anAttachment = "018f0000-0000-7000-8000-000000000000"

func firstBlock(t *testing.T, content string) Node {
	t.Helper()
	n, err := Normalize(doc(content))
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	return n.Content[0]
}

func TestNewBlocksRefuseDangerousContent(t *testing.T) {
	bad := map[string]string{
		"spotlight with a script link": `{"type":"spotlight","attrs":{"button_text":"Go","button_href":"javascript:alert(1)"},"content":[{"type":"paragraph"}]}`,
		"spotlight with a data link":   `{"type":"spotlight","attrs":{"button_text":"Go","button_href":"data:text/html,x"},"content":[{"type":"paragraph"}]}`,
		"image with a script link":     `{"type":"image","attrs":{"attachment_id":"` + anAttachment + `","href":"javascript:alert(1)"}}`,
		"inline image from outside":    `{"type":"paragraph","content":[{"type":"inlineImage","attrs":{"src":"https://evil.example/pixel.gif"}}]}`,
		"inline image with a bad id":   `{"type":"paragraph","content":[{"type":"inlineImage","attrs":{"attachment_id":"../../x"}}]}`,
		"panel emoji with markup":      `{"type":"panel","attrs":{"kind":"neutral","emoji":"<img>"},"content":[{"type":"paragraph"}]}`,
		"panel emoji with letters":     `{"type":"panel","attrs":{"kind":"neutral","emoji":"hello"},"content":[{"type":"paragraph"}]}`,
		"unknown block with children":  `{"type":"unknownBlock","attrs":{"name":"x","original":"{}"},"content":[{"type":"paragraph"}]}`,
		"unknown block without a name": `{"type":"unknownBlock","attrs":{"original":"{}"}}`,
	}
	for name, content := range bad {
		if _, err := Normalize(doc(content)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestAttributesOutsideTheListsAreDropped(t *testing.T) {
	image := firstBlock(t, `{"type":"image","attrs":{"attachment_id":"`+anAttachment+`","width":900,"border":"url(evil)","border_color":"#ff0000","wrap":"yes"}}`)
	for _, key := range []string{"width", "border", "border_color", "wrap"} {
		if _, kept := image.Attrs[key]; kept {
			t.Errorf("image kept %s", key)
		}
	}
	paragraph := firstBlock(t, `{"type":"paragraph","attrs":{"dir":"auto;color:red"}}`)
	if _, kept := paragraph.Attrs["dir"]; kept {
		t.Error("a direction outside the list was kept")
	}
	columns := firstBlock(t, `{"type":"columns","attrs":{"layout":"evil","width":"100vw"},"content":[{"type":"column","content":[{"type":"paragraph"}]},{"type":"column","content":[{"type":"paragraph"}]}]}`)
	if len(columns.Attrs) != 0 {
		t.Errorf("columns kept %v", columns.Attrs)
	}
	code := firstBlock(t, `{"type":"codeBlock","attrs":{"line_start":-4}}`)
	if _, kept := code.Attrs["line_start"]; kept {
		t.Error("a negative first line was kept")
	}
	panel := firstBlock(t, `{"type":"panel","attrs":{"kind":"info","emoji":"🌊","color":"blue"},"content":[{"type":"paragraph"}]}`)
	if len(panel.Attrs) != 1 {
		t.Errorf("a panel of a kind kept its own look: %v", panel.Attrs)
	}
	chart := firstBlock(t, `{"type":"chart","attrs":{"table":"abcdefgh","total":"eval","height":"9999px","legend_position":"fixed"}}`)
	for _, key := range []string{"total", "height", "legend_position"} {
		if _, kept := chart.Attrs[key]; kept {
			t.Errorf("chart kept %s", key)
		}
	}
}

func TestNewAttributesAreKept(t *testing.T) {
	image := firstBlock(t, `{"type":"image","attrs":{"attachment_id":"`+anAttachment+`","width":40,"wrap":true,"align":"end","border":"thin","border_color":"blue","href":"https://example.org/a"}}`)
	if image.Attrs["width"] != 40 || image.Attrs["wrap"] != true || image.Attrs["border"] != "thin" ||
		image.Attrs["border_color"] != "blue" || image.Attrs["href"] != "https://example.org/a" {
		t.Errorf("image: %v", image.Attrs)
	}
	paragraph := firstBlock(t, `{"type":"paragraph","attrs":{"dir":"rtl"}}`)
	if paragraph.Attrs["dir"] != "rtl" {
		t.Errorf("paragraph: %v", paragraph.Attrs)
	}
	code := firstBlock(t, `{"type":"codeBlock","attrs":{"line_start":40,"folded":true}}`)
	if code.Attrs["line_start"] != 40 || code.Attrs["folded"] != true {
		t.Errorf("code: %v", code.Attrs)
	}
	panel := firstBlock(t, `{"type":"panel","attrs":{"kind":"neutral","emoji":"🌊","color":"blue"},"content":[{"type":"paragraph"}]}`)
	if panel.Attrs["emoji"] != "🌊" || panel.Attrs["color"] != "blue" {
		t.Errorf("panel: %v", panel.Attrs)
	}
	columns := firstBlock(t, `{"type":"columns","attrs":{"layout":"sidebar-start","width":"wide"},"content":[{"type":"column","content":[{"type":"paragraph"}]},{"type":"column","content":[{"type":"paragraph"}]}]}`)
	if columns.Attrs["layout"] != "sidebar-start" || columns.Attrs["width"] != "wide" {
		t.Errorf("columns: %v", columns.Attrs)
	}
	spotlight := firstBlock(t, `{"type":"spotlight","attrs":{"color":"blue","button_text":"Go","button_href":"/pages/x"},"content":[{"type":"paragraph"}]}`)
	if spotlight.Attrs["button_text"] != "Go" || spotlight.Attrs["color"] != "blue" {
		t.Errorf("spotlight: %v", spotlight.Attrs)
	}
	chart := firstBlock(t, `{"type":"chart","attrs":{"table":"abcdefgh","total":"sum","height":"large","legend_position":"bottom","points":true,"no_legend":true,"x_title":"Month","y_title":"Sales"}}`)
	for _, key := range []string{"total", "height", "legend_position", "points", "no_legend", "x_title", "y_title"} {
		if _, kept := chart.Attrs[key]; !kept {
			t.Errorf("chart lost %s", key)
		}
	}
}

func TestAButtonWithoutAnAddressIsNoButton(t *testing.T) {
	spotlight := firstBlock(t, `{"type":"spotlight","attrs":{"button_text":"Go"},"content":[{"type":"paragraph"}]}`)
	if _, kept := spotlight.Attrs["button_text"]; kept {
		t.Error("a button without an address was kept")
	}
}

func TestAnInlineImageCountsAsAnAttachment(t *testing.T) {
	n, err := Normalize(doc(`{"type":"paragraph","content":[{"type":"inlineImage","attrs":{"attachment_id":"` + anAttachment + `","alt":"Logo"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.MustParse(anAttachment)
	if got := AttachmentIDs(n); len(got) != 1 || got[0] != id {
		t.Errorf("AttachmentIDs = %v", got)
	}
	other := uuid.MustParse("018f0000-0000-7000-8000-000000000001")
	RemapAttachments(&n, map[uuid.UUID]uuid.UUID{id: other})
	if got := AttachmentIDs(n); got[0] != other {
		t.Errorf("after a copy the image still points at %v", got)
	}
}

func TestAnUnknownBlockKeepsItsOriginalAndNeverRendersIt(t *testing.T) {
	original := `{"type":"future","attrs":{"src":"https://evil.example/x"},"content":[{"type":"text","text":"<script>alert(1)</script>"}]}`
	quoted, _ := json.Marshal(original)
	n, err := Normalize(doc(`{"type":"unknownBlock","attrs":{"name":"future","original":` + string(quoted) + `}}`))
	if err != nil {
		t.Fatal(err)
	}
	if n.Content[0].Attrs["original"] != original {
		t.Errorf("the original changed: %v", n.Content[0].Attrs["original"])
	}
	out := ToHTML(n, HTMLOptions{})
	if strings.Contains(out, "evil.example") || strings.Contains(out, "script") {
		t.Errorf("the original reached the export: %s", out)
	}
	if !strings.Contains(out, "future") {
		t.Errorf("the export does not say what is missing: %s", out)
	}
	if strings.Contains(Text(n), "evil") {
		t.Error("the original reached search")
	}
}

func TestExportsShowTheNewLook(t *testing.T) {
	n, err := Normalize(doc(`{"type":"paragraph","attrs":{"dir":"rtl"},"content":[{"type":"text","text":"x"}]},
		{"type":"image","attrs":{"attachment_id":"` + anAttachment + `","width":43,"wrap":true,"align":"end","border":"thin","border_color":"blue","href":"https://example.org/a"}},
		{"type":"paragraph","content":[{"type":"inlineImage","attrs":{"attachment_id":"` + anAttachment + `","alt":"Logo"}}]},
		{"type":"spotlight","attrs":{"color":"blue","button_text":"Go on","button_href":"https://example.org/b"},"content":[{"type":"paragraph"}]},
		{"type":"panel","attrs":{"kind":"neutral","emoji":"🌊"},"content":[{"type":"paragraph"}]},
		{"type":"columns","attrs":{"layout":"sidebar-end"},"content":[{"type":"column","content":[{"type":"paragraph"}]},{"type":"column","content":[{"type":"paragraph"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	out := ToHTML(n, HTMLOptions{ImageSrc: func(uuid.UUID) (string, bool) { return "a.png", true }})
	for _, want := range []string{
		`<p dir="rtl">`, "share-45", "wrap-end", "border-thin", "line-blue", `<a href="https://example.org/a"`,
		`class="inline-image"`, `class="spotlight mark-blue"`, `<a class="button" href="https://example.org/b"`, "Go on",
		"🌊", `class="columns sidebar-end"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q: %s", want, out)
		}
	}
}

func TestStoredDocumentsUpgradeOnRead(t *testing.T) {
	renameOld := func(raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "oldBlock", "paragraph")), nil
	}
	addRule := func(raw []byte) ([]byte, error) {
		return []byte(strings.ReplaceAll(string(raw), "paragraph", "heading")), nil
	}
	steps := []UpgradeStep{{From: 1, Run: renameOld}, {From: 2, Run: addRule}}
	raw := []byte(`{"type":"doc","content":[{"type":"oldBlock"}]}`)

	got, err := upgrade(raw, 1, 3, steps)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"type":"doc","content":[{"type":"heading"}]}`; string(got) != want {
		t.Errorf("two steps gave %s", got)
	}
	got, err = upgrade(raw, 2, 3, steps)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "heading") || !strings.Contains(string(got), "oldBlock") {
		t.Errorf("a step older than the document ran: %s", got)
	}
	got, err = upgrade(raw, 7, 3, steps)
	if err != nil || string(got) != string(raw) {
		t.Errorf("a document from a newer release changed: %s, %v", got, err)
	}
	if got, err := Upgrade(raw, SchemaVersion); err != nil || string(got) != string(raw) {
		t.Errorf("a current document changed: %s, %v", got, err)
	}
}
