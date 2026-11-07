// SPDX-License-Identifier: Apache-2.0

package document

import (
	_ "embed" // the style sheet of exported HTML
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// ExportStyle is the style sheet of exported HTML, a public site and PDF. A
// document only holds names of colours, so this file gives them their values.
//
//go:embed export.css
var ExportStyle string

// HTMLOptions says how a document refers to what is outside it. HTML from this
// renderer only ever leaves the instance as a file: the instance itself never
// serves it to a browser (spec 014, D1).
type HTMLOptions struct {
	// ImageSrc gives the address of an image. Without one the image becomes its text.
	ImageSrc func(id uuid.UUID) (string, bool)
	// FileHref gives the address of a file. Without one a file is only its name.
	FileHref func(id uuid.UUID) (string, bool)
	// PageHref gives the address of a linked page. Without one the link becomes text.
	PageHref func(id uuid.UUID) (string, bool)
	// Mention replaces a person's name, as a public site does unless it shows names.
	Mention func(n Node) string
}

// imageWidthStep is how fine an export follows the width of an image.
const imageWidthStep = 5

// Heading is a heading of a document, with the id ToHTML gave it.
type Heading struct {
	ID    string
	Level int
	Text  string
}

var notInSlug = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func slug(text string, seen map[string]int) string {
	base := strings.Trim(notInSlug.ReplaceAllString(strings.ToLower(text), "-"), "-")
	if base == "" {
		base = "section"
	}
	seen[base]++
	if n := seen[base]; n > 1 {
		return fmt.Sprintf("%s-%d", base, n)
	}
	return base
}

// Headings lists the headings of a document in order.
func Headings(doc Node) []Heading {
	seen := map[string]int{}
	out := []Heading{}
	Walk(doc, func(c Node) {
		if c.Type != TypeHeading {
			return
		}
		text := BlockText(c)
		out = append(out, Heading{ID: slug(text, seen), Level: headingLevel(c), Text: text})
	})
	return out
}

func headingLevel(n Node) int {
	level, ok := intAttr(n.Attrs["level"])
	if !ok || level < 1 || level > 6 {
		return 2
	}
	return level
}

type htmlWriter struct {
	b    strings.Builder
	o    HTMLOptions
	seen map[string]int
}

// ToHTML turns a document into HTML. Everything is escaped, and only what this
// function writes itself is markup, so no value in a document can become a tag.
func ToHTML(doc Node, o HTMLOptions) string {
	w := &htmlWriter{o: o, seen: map[string]int{}}
	w.children(doc)
	return w.b.String()
}

func esc(s string) string { return html.EscapeString(s) }

func (w *htmlWriter) children(n Node) {
	for _, c := range n.Content {
		w.node(c)
	}
}

func (w *htmlWriter) tag(name, class string, n Node) {
	w.b.WriteString("<" + name)
	if class != "" {
		w.b.WriteString(` class="` + esc(class) + `"`)
	}
	w.b.WriteString(">")
	w.children(n)
	w.b.WriteString("</" + name + ">")
}

func blockClass(n Node) string {
	var classes []string
	if a, _ := n.Attrs["align"].(string); a != "" {
		classes = append(classes, "align-"+a)
	}
	if indent, ok := intAttr(n.Attrs["indent"]); ok && indent > 0 {
		classes = append(classes, "indent-"+strconv.Itoa(indent))
	}
	return strings.Join(classes, " ")
}

// open writes the start of a block, with the class and direction it has.
func (w *htmlWriter) open(name string, n Node, extra string) {
	w.b.WriteString("<" + name + extra)
	if c := blockClass(n); c != "" {
		w.b.WriteString(` class="` + esc(c) + `"`)
	}
	if dir, _ := n.Attrs["dir"].(string); dir != "" {
		w.b.WriteString(` dir="` + esc(dir) + `"`)
	}
	w.b.WriteString(">")
}

func (w *htmlWriter) node(n Node) {
	switch n.Type {
	case TypeText:
		w.text(n)
	case TypeParagraph:
		w.open("p", n, "")
		w.children(n)
		w.b.WriteString("</p>")
	case TypeHeading:
		level := strconv.Itoa(headingLevel(n))
		id := slug(BlockText(n), w.seen)
		w.open("h"+level, n, ` id="`+esc(id)+`"`)
		w.children(n)
		w.b.WriteString("</h" + level + ">")
	case TypeBulletList:
		w.tag("ul", "", n)
	case TypeOrderedList:
		w.tag("ol", "", n)
	case TypeListItem:
		w.tag("li", "", n)
	case TypeTaskList:
		w.tag("ul", "tasks", n)
	case TypeTaskItem:
		w.task(n)
	case TypeBlockquote:
		w.tag("blockquote", "", n)
	case TypeCodeBlock:
		w.code(n)
	case TypeHorizontalRule:
		w.b.WriteString("<hr>")
	case TypeHardBreak:
		w.b.WriteString("<br>")
	case TypeImage:
		w.image(n)
	case TypeInlineImage:
		w.inlineImage(n)
	case TypeSpotlight:
		w.spotlight(n)
	case TypeUnknownBlock:
		// The name alone: what is kept in it was never checked, so it never leaves.
		name, _ := n.Attrs["name"].(string)
		w.b.WriteString(`<p class="unknown-block">Block not shown: ` + esc(name) + `</p>`)
	case TypeFile:
		w.file(n)
	case TypePanel:
		w.panel(n)
	case TypeExpand:
		// Open, because a file or a printed page has no way to open it (EXC-013).
		title, _ := n.Attrs["title"].(string)
		w.b.WriteString("<details open><summary>" + esc(title) + "</summary>")
		w.children(n)
		w.b.WriteString("</details>")
	case TypeColumns:
		layout, _ := n.Attrs["layout"].(string)
		w.tag("div", strings.TrimSpace("columns "+layout), n)
	case TypeColumn:
		w.tag("div", "column", n)
	case TypeDecision:
		w.tag("div", "decision", n)
	case TypeStatus:
		text, _ := n.Attrs["text"].(string)
		color, _ := n.Attrs["color"].(string)
		w.b.WriteString(`<span class="status mark-` + esc(color) + `">` + esc(text) + `</span>`)
	case TypeDate:
		value, _ := n.Attrs["value"].(string)
		w.b.WriteString(`<time datetime="` + esc(value) + `">` + esc(value) + `</time>`)
	case TypeMention:
		w.b.WriteString(esc(w.mention(n)))
	case TypeTable:
		w.b.WriteString(`<div class="table"><table>`)
		if caption, below := TableCaption(n); caption != "" {
			w.b.WriteString(captionTag(below) + esc(caption) + "</caption>")
		}
		w.children(n)
		w.b.WriteString("</table></div>")
	case TypeSheet:
		w.sheet(n)
	case TypeTableRow:
		w.tag("tr", "", n)
	case TypeTableCell:
		w.cell("td", n)
	case TypeTableHeader:
		w.cell("th", n)
	default:
		// A kind of block this renderer does not know keeps its words (EXC-036).
		if words := BlockText(n); words != "" {
			w.b.WriteString("<p>" + esc(words) + "</p>")
		}
	}
}

// sheet writes a database as a plain table. Formulas are worked out in the browser,
// so a static copy shows the formula itself.
func (w *htmlWriter) sheet(n Node) {
	w.b.WriteString("<table>")
	for i, line := range sheetLines(n) {
		cell := "td"
		if i == 0 {
			cell = "th"
		}
		w.b.WriteString("<tr>")
		for _, value := range line {
			w.b.WriteString("<" + cell + ">" + esc(value) + "</" + cell + ">")
		}
		w.b.WriteString("</tr>")
	}
	w.b.WriteString("</table>")
}

func captionTag(below bool) string {
	if below {
		return `<caption class="below">`
	}
	return "<caption>"
}

func (w *htmlWriter) mention(n Node) string {
	if w.o.Mention != nil {
		return w.o.Mention(n)
	}
	return mentionText(n)
}

func (w *htmlWriter) task(n Node) {
	mark := ""
	if done, _ := n.Attrs["checked"].(bool); done {
		mark = " checked"
	}
	w.b.WriteString(`<li><input type="checkbox" disabled` + mark + `> `)
	w.children(n)
	w.b.WriteString("</li>")
}

func (w *htmlWriter) code(n Node) {
	if title, _ := n.Attrs["title"].(string); title != "" {
		w.b.WriteString(`<p class="code-title">` + esc(title) + `</p>`)
	}
	language, _ := n.Attrs["language"].(string)
	w.b.WriteString(`<pre dir="ltr"><code`)
	if language != "" {
		w.b.WriteString(` class="language-` + esc(language) + `"`)
	}
	w.b.WriteString(">" + esc(BlockText(n)) + "</code></pre>")
}

func (w *htmlWriter) image(n Node) {
	alt, _ := n.Attrs["alt"].(string)
	caption, _ := n.Attrs["caption"].(string)
	id, err := uuid.Parse(fmt.Sprint(n.Attrs["attachment_id"]))
	src, ok := "", false
	if err == nil && w.o.ImageSrc != nil {
		src, ok = w.o.ImageSrc(id)
	}
	if !ok {
		if words := strings.TrimSpace(alt + " " + caption); words != "" {
			w.b.WriteString("<p>" + esc(words) + "</p>")
		}
		return
	}
	w.b.WriteString(`<figure class="` + esc(imageClasses(n)) + `">`)
	href, hasLink := n.Attrs["href"].(string)
	if hasLink {
		w.b.WriteString(`<a href="` + esc(href) + `" rel="noopener noreferrer">`)
	}
	w.b.WriteString(`<img src="` + esc(src) + `" alt="` + esc(alt) + `">`)
	if hasLink {
		w.b.WriteString("</a>")
	}
	if caption != "" {
		w.b.WriteString("<figcaption>" + esc(caption) + "</figcaption>")
	}
	w.b.WriteString("</figure>")
}

// imageClasses names the look of an image. A style sheet cannot know a number, so a
// share of the column goes to the nearest step, which an export has a class for.
func imageClasses(n Node) string {
	size, _ := n.Attrs["size"].(string)
	classes := []string{"image", "size-" + size}
	if width, ok := intAttr(n.Attrs["width"]); ok {
		step := (width + imageWidthStep/2) / imageWidthStep * imageWidthStep
		classes = append(classes, "share-"+strconv.Itoa(step))
	}
	if wrap, _ := n.Attrs["wrap"].(bool); wrap {
		align, _ := n.Attrs["align"].(string)
		classes = append(classes, "wrap-"+align)
	}
	if border, _ := n.Attrs["border"].(string); border != "" {
		classes = append(classes, "border-"+border)
	}
	if color, _ := n.Attrs["border_color"].(string); color != "" {
		classes = append(classes, "line-"+color)
	}
	return strings.Join(classes, " ")
}

func (w *htmlWriter) inlineImage(n Node) {
	alt, _ := n.Attrs["alt"].(string)
	id, err := uuid.Parse(fmt.Sprint(n.Attrs["attachment_id"]))
	if err == nil && w.o.ImageSrc != nil {
		if src, ok := w.o.ImageSrc(id); ok {
			w.b.WriteString(`<img class="inline-image" src="` + esc(src) + `" alt="` + esc(alt) + `">`)
			return
		}
	}
	w.b.WriteString(esc(alt))
}

func (w *htmlWriter) spotlight(n Node) {
	class := "spotlight"
	if color, _ := n.Attrs["color"].(string); color != "" {
		class += " mark-" + color
	}
	w.b.WriteString(`<aside class="` + esc(class) + `">`)
	w.children(n)
	text, _ := n.Attrs["button_text"].(string)
	href, _ := n.Attrs["button_href"].(string)
	// An address inside the instance means nothing in a file, so the words stay alone.
	if text != "" && strings.HasPrefix(href, "/") {
		w.b.WriteString(`<p>` + esc(text) + `</p>`)
	} else if text != "" {
		w.b.WriteString(`<a class="button" href="` + esc(href) + `" rel="noopener noreferrer">` + esc(text) + `</a>`)
	}
	w.b.WriteString("</aside>")
}

func (w *htmlWriter) file(n Node) {
	name, _ := n.Attrs["filename"].(string)
	id, err := uuid.Parse(fmt.Sprint(n.Attrs["attachment_id"]))
	if err == nil && w.o.FileHref != nil {
		if href, ok := w.o.FileHref(id); ok {
			w.b.WriteString(`<p class="file"><a href="` + esc(href) + `">` + esc(name) + `</a></p>`)
			return
		}
	}
	w.b.WriteString(`<p class="file">` + esc(name) + `</p>`)
}

func (w *htmlWriter) panel(n Node) {
	kind, _ := n.Attrs["kind"].(string)
	if kind == panelNeutral {
		emoji, _ := n.Attrs["emoji"].(string)
		w.b.WriteString(`<aside class="panel panel-` + esc(kind) + `"><p class="panel-kind">` + esc(emoji) + `</p>`)
		w.children(n)
		w.b.WriteString("</aside>")
		return
	}
	// The word carries the meaning, because colour alone is not enough.
	label := []rune(kind)
	if len(label) > 0 {
		label[0] = unicode.ToUpper(label[0])
	}
	w.b.WriteString(`<aside class="panel panel-` + esc(kind) + `"><p class="panel-kind">` + esc(string(label)) + `</p>`)
	w.children(n)
	w.b.WriteString("</aside>")
}

func (w *htmlWriter) cell(name string, n Node) {
	w.b.WriteString("<" + name)
	if name == "th" {
		w.b.WriteString(` scope="col"`)
	}
	for _, span := range []string{"colspan", "rowspan"} {
		if v, ok := intAttr(n.Attrs[span]); ok && v > 1 {
			w.b.WriteString(` ` + span + `="` + strconv.Itoa(v) + `"`)
		}
	}
	if bg, _ := n.Attrs["background"].(string); bg != "" {
		w.b.WriteString(` class="mark-` + esc(bg) + `"`)
	}
	w.b.WriteString(">")
	w.children(n)
	w.b.WriteString("</" + name + ">")
}

func (w *htmlWriter) link(mark Mark) (open, closing string) {
	href, _ := mark.Attrs["href"].(string)
	if found := pageLink.FindStringSubmatch(href); found != nil {
		id, err := uuid.Parse(found[1])
		if err != nil || w.o.PageHref == nil {
			return "", ""
		}
		target, ok := w.o.PageHref(id)
		if !ok {
			return "", ""
		}
		return `<a href="` + esc(target+strings.TrimPrefix(href, found[0][:len("/pages/")+len(found[1])])) + `">`, "</a>"
	}
	safe, err := SafeHref(href)
	// An address inside the instance means nothing in a file, and must not leak out.
	if err != nil || strings.HasPrefix(safe, "/") {
		return "", ""
	}
	if strings.HasPrefix(safe, "#") {
		return `<a href="` + esc(safe) + `">`, "</a>"
	}
	return `<a href="` + esc(safe) + `" rel="noopener noreferrer">`, "</a>"
}

func (w *htmlWriter) text(n Node) {
	var open, closing []string
	add := func(start, end string) {
		open = append(open, start)
		closing = append([]string{end}, closing...)
	}
	for _, m := range n.Marks {
		switch m.Type {
		case MarkBold:
			add("<strong>", "</strong>")
		case MarkItalic:
			add("<em>", "</em>")
		case MarkUnderline:
			add("<u>", "</u>")
		case MarkStrike:
			add("<s>", "</s>")
		case MarkCode:
			add("<code>", "</code>")
		case MarkSubscript:
			add("<sub>", "</sub>")
		case MarkSuperscript:
			add("<sup>", "</sup>")
		case MarkTextColor:
			color, _ := m.Attrs["color"].(string)
			add(`<span class="text-`+esc(color)+`">`, "</span>")
		case MarkHighlight:
			color, _ := m.Attrs["color"].(string)
			add(`<mark class="mark-`+esc(color)+`">`, "</mark>")
		case MarkLink:
			if start, end := w.link(m); start != "" {
				add(start, end)
			}
		}
	}
	w.b.WriteString(strings.Join(open, "") + esc(n.Text) + strings.Join(closing, ""))
}
