// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/ids"
)

func doc(content string) []byte { return []byte(`{"type":"doc","content":[` + content + `]}`) }

func TestNormalizeRefusesDangerousContent(t *testing.T) {
	bad := map[string]string{
		"unknown node":      `{"type":"script","content":[{"type":"text","text":"x"}]}`,
		"unknown mark":      `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"onclick"}]}]}`,
		"javascript link":   `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}`,
		"mixed case js":     `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"JaVaScRiPt:alert(1)"}}]}]}`,
		"js with tab":       `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"java\tscript:alert(1)"}}]}]}`,
		"data link":         `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"data:text/html,<script>1</script>"}}]}]}`,
		"protocol relative": `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"//evil.example/x"}}]}]}`,
		"backslash path":    `{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"/\\evil.example"}}]}]}`,
		"external image":    `{"type":"image","attrs":{"src":"https://evil.example/pixel.gif"}}`,
		"heading no level":  `{"type":"heading","content":[{"type":"text","text":"x"}]}`,
		"heading level 7":   `{"type":"heading","attrs":{"level":7}}`,
		"text at top":       `{"type":"text","text":"x"}`,
		"list without item": `{"type":"bulletList","content":[{"type":"paragraph"}]}`,
	}
	for name, body := range bad {
		if _, err := Normalize(doc(body)); err == nil {
			t.Errorf("%s: accepted %s", name, body)
		}
	}
	if _, err := Normalize([]byte(`{"type":"paragraph"}`)); err == nil {
		t.Error("a non-doc root was accepted")
	}
	if _, err := Normalize([]byte(`not json`)); err == nil {
		t.Error("invalid JSON was accepted")
	}
	deep := strings.Repeat(`{"type":"blockquote","content":[`, MaxDepth+5) + `{"type":"paragraph"}` + strings.Repeat(`]}`, MaxDepth+5)
	if _, err := Normalize(doc(deep)); err == nil {
		t.Error("too deep nesting was accepted")
	}
	if _, err := Normalize(make([]byte, MaxBytes+1)); err == nil {
		t.Error("an oversized document was accepted")
	}
}

func TestNormalizeDropsUnknownAttributes(t *testing.T) {
	n, err := Normalize(doc(`{"type":"paragraph","attrs":{"style":"position:fixed","onclick":"x"},"content":[
		{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"https://example.com","target":"_blank","rel":"x","class":"y"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p := n.Content[0]
	if p.Attrs != nil {
		t.Fatalf("paragraph attributes kept: %v", p.Attrs)
	}
	attrs := p.Content[0].Marks[0].Attrs
	if len(attrs) != 1 || attrs["href"] != "https://example.com" {
		t.Fatalf("link attributes: %v", attrs)
	}
}

func TestNormalizeKeepsEverythingTheEditorProduces(t *testing.T) {
	id := ids.New()
	in := doc(`{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]},
		{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"bold"},{"type":"italic"}]},{"type":"hardBreak"},{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"/pages/1"}}]}]},
		{"type":"orderedList","attrs":{"start":3},"content":[{"type":"listItem","content":[{"type":"paragraph"}]}]},
		{"type":"taskList","content":[{"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph"}]}]},
		{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"x := 1"}]},
		{"type":"image","attrs":{"attachment_id":"` + id.String() + `","alt":"a"}},
		{"type":"horizontalRule"},
		{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colspan":2,"rowspan":1,"colwidth":[100,null]},"content":[{"type":"paragraph"}]}]}]}`)
	n, err := Normalize(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := AttachmentIDs(n); len(got) != 1 || got[0] != id {
		t.Fatalf("attachment ids: %v", got)
	}
	if n.Content[2].Attrs["start"] != 3 || n.Content[3].Content[0].Attrs["checked"] != true {
		t.Fatalf("attributes lost: %+v", n.Content)
	}
	again, err := Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := Normalize(again)
	if err != nil {
		t.Fatal(err)
	}
	third, _ := Marshal(n2)
	if string(again) != string(third) {
		t.Fatal("Normalize is not idempotent")
	}
}

func TestCompare(t *testing.T) {
	a, _ := Normalize(doc(`{"type":"paragraph","content":[{"type":"text","text":"same"}]},
		{"type":"paragraph","content":[{"type":"text","text":"the old text here"}]},
		{"type":"paragraph","content":[{"type":"text","text":"gone"}]}`))
	b, _ := Normalize(doc(`{"type":"paragraph","content":[{"type":"text","text":"same"}]},
		{"type":"paragraph","content":[{"type":"text","text":"the new text here"}]},
		{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"added"}]}`))
	got := Compare(a, b)
	ops := []string{}
	for _, d := range got {
		ops = append(ops, d.Op)
	}
	if strings.Join(ops, ",") != "equal,changed,removed,added" {
		t.Fatalf("ops: %v", ops)
	}
	words := got[1].Words
	if len(words) != 4 || words[1].Op != OpRemoved || words[1].Text != "old" || words[2].Op != OpAdded || words[2].Text != "new" {
		t.Fatalf("words: %+v", words)
	}
}

const roundTripSource = `# Heading

Some **bold**, *italic*, ~~strike~~ and ` + "`code`" + ` with a [link](https://example.com).

## Lists

- one
- two
  - nested

3. three
4. four

- [x] done
- [ ] todo

> A quote

` + "```go\nfmt.Println(\"hi\")\n```" + `

---

| Name | Value |
| --- | --- |
| a | 1 |
| b | 2 |

![Diagram](images/diagram.png)
`

func TestMarkdownRoundTrip(t *testing.T) {
	id := ids.New()
	resolve := func(dest string) (uuid.UUID, bool) { return id, dest == "images/diagram.png" }
	n, warnings := FromMarkdown([]byte(roundTripSource), resolve)
	if len(warnings) > 0 {
		t.Fatalf("warnings: %v", warnings)
	}
	raw, _ := Marshal(n)
	first, err := Normalize(raw)
	if err != nil {
		t.Fatalf("imported Markdown must pass the gate: %v\n%s", err, raw)
	}
	types := map[string]bool{}
	Walk(first, func(c Node) { types[c.Type] = true })
	for _, want := range []string{TypeHeading, TypeBulletList, TypeOrderedList, TypeTaskList, TypeBlockquote, TypeCodeBlock, TypeHorizontalRule, TypeTable, TypeImage} {
		if !types[want] {
			t.Errorf("node type %s missing after import", want)
		}
	}
	out := ToMarkdown(first, func(uuid.UUID) string { return "images/diagram.png" })
	for _, want := range []string{"# Heading", "**bold**", "*italic*", "~~strike~~", "`code`", "[link](https://example.com)", "- [x] done", "```go", "| a | 1 |", "![Diagram](images/diagram.png)", "3. three"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from export:\n%s", want, out)
		}
	}
	second, _ := FromMarkdown([]byte(out), resolve)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Fatalf("round trip changed the document\nfirst:  %s\nsecond: %s\nmarkdown:\n%s", a, b, out)
	}
}

func TestMarkdownNeverLetsDangerousContentThrough(t *testing.T) {
	src := `[click](javascript:alert(1))

<script>alert(1)</script>

![pixel](https://evil.example/p.gif)

<img src=x onerror=alert(1)>
`
	n, warnings := FromMarkdown([]byte(src), nil)
	raw, _ := Marshal(n)
	if _, err := Normalize(raw); err != nil {
		t.Fatalf("converted document refused: %v", err)
	}
	if strings.Contains(string(raw), `"href":"javascript`) || strings.Contains(string(raw), `"type":"image"`) {
		t.Fatalf("dangerous content survived: %s", raw)
	}
	if len(warnings) == 0 {
		t.Fatal("the user must be warned about what was changed")
	}
}

func FuzzFromMarkdown(f *testing.F) {
	f.Add([]byte(roundTripSource))
	f.Add([]byte("[x](javascript:1) ![a](b) <b>c</b>\n\n| a |\n|---|\n| ![i](j) |\n"))
	f.Add([]byte("- [ ] ![x](y)\n> # z\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		n, _ := FromMarkdown(src, nil)
		raw, err := Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		// Whatever comes in, the result must pass the security gate or be
		// refused for size or depth only, and writing it back must not panic.
		if _, err := Normalize(raw); err == nil {
			_ = ToMarkdown(n, nil)
		}
	})
}

func FuzzNormalize(f *testing.F) {
	f.Add(doc(`{"type":"paragraph","content":[{"type":"text","text":"x"}]}`))
	f.Add(doc(`{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colwidth":[1,2]}}]}]}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		n, err := Normalize(raw)
		if err != nil {
			return
		}
		again, err := Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Normalize(again); err != nil {
			t.Fatalf("normalized document refused the second time: %v\n%s", err, again)
		}
	})
}
