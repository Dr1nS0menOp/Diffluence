// SPDX-License-Identifier: Apache-2.0

package document

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func parsed(t *testing.T, doc string) Node {
	t.Helper()
	var n Node
	if err := json.Unmarshal([]byte(doc), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestToHTMLEscapesEveryValue(t *testing.T) {
	doc := parsed(t, `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"<script>alert(1)</script>"}]},
		{"type":"expand","attrs":{"title":"\"><img src=x onerror=1>"},"content":[]},
		{"type":"codeBlock","attrs":{"language":"\"><b>","title":"<i>"},"content":[{"type":"text","text":"<x>"}]},
		{"type":"paragraph","content":[{"type":"status","attrs":{"text":"<u>","color":"\"x"}}]}
	]}`)
	out := ToHTML(doc, HTMLOptions{})
	for _, bad := range []string{"<script", "<img", "<b>", "<i>", "<u>", "<x>", `"x"`} {
		if strings.Contains(out, bad) {
			t.Errorf("%q got through: %s", bad, out)
		}
	}
}

func TestToHTMLKnowsEveryBlockAndKeepsWordsOfUnknownOnes(t *testing.T) {
	doc := parsed(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Notes"}]},
		{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"Notes"}]},
		{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]}]},
		{"type":"taskList","content":[{"type":"taskItem","attrs":{"checked":true},"content":[{"type":"paragraph"}]}]},
		{"type":"panel","attrs":{"kind":"warning"},"content":[{"type":"paragraph"}]},
		{"type":"expand","attrs":{"title":"More"},"content":[{"type":"paragraph"}]},
		{"type":"columns","content":[{"type":"column","content":[{"type":"paragraph"}]}]},
		{"type":"decision","content":[{"type":"text","text":"Go"}]},
		{"type":"table","content":[{"type":"tableRow","content":[
			{"type":"tableHeader","attrs":{"colspan":2},"content":[{"type":"paragraph"}]},
			{"type":"tableCell","attrs":{"background":"yellow"},"content":[{"type":"paragraph"}]}]}]},
		{"type":"futureBlock","content":[{"type":"text","text":"still readable"}]}
	]}`)
	out := ToHTML(doc, HTMLOptions{})
	for _, want := range []string{`<h3 id="notes">`, `<h3 id="notes-2">`, "<ul><li><p>one</p>", `<input type="checkbox" disabled checked>`,
		`class="panel panel-warning"`, ">Warning<", "<details open><summary>More</summary>", `class="columns"`, `class="decision"`,
		`<th scope="col" colspan="2">`, `class="mark-yellow"`, "still readable"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestToHTMLOnlyLinksWhatTheCallerAllows(t *testing.T) {
	target := uuid.New()
	link := func(href string) string {
		return `{"type":"text","text":"x","marks":[{"type":"link","attrs":{"href":"` + href + `"}}]}`
	}
	doc := parsed(t, `{"type":"doc","content":[{"type":"paragraph","content":[`+
		link("/pages/"+target.String()+"#part")+`,`+link("/pages/"+uuid.NewString())+`,`+
		link("/spaces/SECRET")+`,`+link("https://example.com/a")+`]}]}`)
	out := ToHTML(doc, HTMLOptions{PageHref: func(id uuid.UUID) (string, bool) {
		return "page.html", id == target
	}})
	if !strings.Contains(out, `<a href="page.html#part">`) {
		t.Errorf("page link: %s", out)
	}
	if strings.Contains(out, "SECRET") || strings.Count(out, "<a ") != 2 {
		t.Errorf("an address inside the instance leaked, or a link to a page outside the set stayed: %s", out)
	}
}

func TestToHTMLLeavesImagesOutWithoutAnAddress(t *testing.T) {
	id := uuid.NewString()
	doc := parsed(t, `{"type":"doc","content":[
		{"type":"image","attrs":{"attachment_id":"`+id+`","alt":"A map"}},
		{"type":"file","attrs":{"attachment_id":"`+id+`","filename":"plan.pdf"}}]}`)
	if out := ToHTML(doc, HTMLOptions{}); strings.Contains(out, "<img") || strings.Contains(out, id) ||
		!strings.Contains(out, "A map") || !strings.Contains(out, "plan.pdf") {
		t.Fatalf("got %s", out)
	}
	out := ToHTML(doc, HTMLOptions{ImageSrc: func(uuid.UUID) (string, bool) { return "files/a.png", true }})
	if !strings.Contains(out, `<img src="files/a.png" alt="A map">`) {
		t.Fatalf("got %s", out)
	}
}

func TestToHTMLUsesTheCallersWordsForAMention(t *testing.T) {
	doc := parsed(t, `{"type":"doc","content":[{"type":"paragraph","content":[
		{"type":"mention","attrs":{"kind":"user","id":"`+uuid.NewString()+`","label":"Anna Visser"}}]}]}`)
	if out := ToHTML(doc, HTMLOptions{}); !strings.Contains(out, "@Anna Visser") {
		t.Fatalf("got %s", out)
	}
	out := ToHTML(doc, HTMLOptions{Mention: func(Node) string { return "a colleague" }})
	if strings.Contains(out, "Anna") || !strings.Contains(out, "a colleague") {
		t.Fatalf("a name reached a public page: %s", out)
	}
}

func TestExportStyleHasEveryColourName(t *testing.T) {
	for _, name := range TextColors() {
		if !strings.Contains(ExportStyle, ".text-"+name+" ") {
			t.Errorf("no style for text colour %s", name)
		}
	}
	for _, name := range MarkColors() {
		if !strings.Contains(ExportStyle, ".mark-"+name+" ") {
			t.Errorf("no style for mark colour %s", name)
		}
	}
}

func TestHeadingsMatchTheIdsInTheHTML(t *testing.T) {
	doc := parsed(t, `{"type":"doc","content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Your first day"}]},
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"!!"}]}]}`)
	got := Headings(doc)
	if len(got) != 2 || got[0].ID != "your-first-day" || got[1].ID != "section" {
		t.Fatalf("got %v", got)
	}
}
