// SPDX-License-Identifier: Apache-2.0

package document

import (
	"bytes"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList)).Parser()

// AttachmentResolver turns a destination from Markdown into an attachment. It
// returns false when the destination is not available as an attachment.
type AttachmentResolver func(destination string) (uuid.UUID, bool)

// FromMarkdown converts Markdown into a document. It always succeeds: anything
// that has no safe equivalent becomes plain text or a link, with a warning.
// The result passes Normalize.
func FromMarkdown(source []byte, attachments AttachmentResolver) (Node, []string) {
	return FromMarkdownLinked(source, attachments, nil)
}

// LinkResolver turns a destination from Markdown into the address of a page.
type LinkResolver func(destination string) (href string, ok bool)

// FromMarkdownLinked is FromMarkdown for a set of files, whose links lead to each other.
func FromMarkdownLinked(source []byte, attachments AttachmentResolver, links LinkResolver) (Node, []string) {
	if tooHardToParse(source) {
		block := Node{Type: TypeCodeBlock, Content: textNode(string(source), nil)}
		return Node{Type: TypeDoc, Content: []Node{block}}, []string{"This text holds a block too large to read as Markdown, so it is kept as plain text."}
	}
	c := &converter{src: source, attachments: attachments, links: links}
	root := markdownParser.Parse(text.NewReader(source))
	doc := Node{Type: TypeDoc, Content: c.blocks(root)}
	if len(doc.Content) == 0 {
		doc.Content = []Node{{Type: TypeParagraph}}
	}
	return doc, c.warnings
}

// maxMarkdownBlockMarks is how many marks one block may hold. The parser's work grows with
// the square of them: at this limit a block takes under a second, and at a megabyte of open
// brackets it took minutes (spec 025).
const maxMarkdownBlockMarks = 16 << 10

// tooHardToParse reports whether a block, the lines between two blank lines, holds more marks
// than the parser can take. It counts every line, also in code and tables: skipping those
// would need the parser's own view of the text, and a wrong guess would let an attack through.
func tooHardToParse(source []byte) bool {
	marks, blankLine := 0, true
	for _, c := range source {
		switch c {
		case '\n':
			if blankLine {
				marks = 0
			}
			marks++
			blankLine = true
		case ' ', '\t', '\r':
		case '[', ']', '(', '!', '*', '_', '~', '>', '-', '+', '.':
			marks++
			blankLine = false
		default:
			blankLine = false
		}
		if marks > maxMarkdownBlockMarks {
			return true
		}
	}
	return false
}

type converter struct {
	src         []byte
	attachments AttachmentResolver
	links       LinkResolver
	warnings    []string
}

func (c *converter) warn(format string, args ...any) {
	if len(c.warnings) < 100 {
		c.warnings = append(c.warnings, fmt.Sprintf(format, args...))
	}
}

func (c *converter) blocks(parent ast.Node) []Node {
	var out []Node
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		out = append(out, c.block(n)...)
	}
	return out
}

func (c *converter) lines(n ast.Node) string {
	var b bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(c.src))
	}
	return b.String()
}

func textNode(s string, marks []Mark) []Node {
	if s == "" {
		return nil
	}
	return []Node{{Type: TypeText, Text: s, Marks: marks}}
}

func (c *converter) block(n ast.Node) []Node {
	switch v := n.(type) {
	case *ast.Heading:
		level := min(max(v.Level, 1), 6)
		inline, hoisted := c.inlineWithImages(v)
		return append([]Node{{Type: TypeHeading, Attrs: map[string]any{"level": level}, Content: inline}}, hoisted...)
	case *ast.Paragraph, *ast.TextBlock:
		inline, hoisted := c.inlineWithImages(n)
		if len(inline) == 0 {
			return hoisted
		}
		return append([]Node{{Type: TypeParagraph, Content: inline}}, hoisted...)
	case *ast.Blockquote:
		content := c.blocks(v)
		if kind, rest, ok := panelOf(content); ok {
			return []Node{{Type: TypePanel, Attrs: map[string]any{"kind": kind}, Content: rest}}
		}
		if len(content) == 0 {
			content = []Node{{Type: TypeParagraph}}
		}
		return []Node{{Type: TypeBlockquote, Content: content}}
	case *ast.ThematicBreak:
		return []Node{{Type: TypeHorizontalRule}}
	case *ast.FencedCodeBlock:
		node := Node{Type: TypeCodeBlock, Content: textNode(strings.TrimSuffix(c.lines(v), "\n"), nil)}
		if lang := string(v.Language(c.src)); languagePattern.MatchString(lang) {
			node.Attrs = map[string]any{"language": lang}
		}
		return []Node{node}
	case *ast.CodeBlock:
		return []Node{{Type: TypeCodeBlock, Content: textNode(strings.TrimSuffix(c.lines(v), "\n"), nil)}}
	case *ast.HTMLBlock:
		var b strings.Builder
		b.WriteString(c.lines(v))
		if v.HasClosure() {
			b.Write(v.ClosureLine.Value(c.src))
		}
		c.warn("Raw HTML was kept as a code block.")
		return []Node{{Type: TypeCodeBlock, Content: textNode(strings.TrimSuffix(b.String(), "\n"), nil)}}
	case *ast.List:
		return []Node{c.list(v)}
	case *extast.Table:
		return []Node{c.table(v)}
	}
	return c.blocks(n)
}

// alertNames maps a panel kind to the name of the widely used Markdown alert.
var alertNames = map[string]string{"info": "NOTE", "note": "IMPORTANT", "success": "TIP", "warning": "WARNING", "error": "CAUTION"}

var alertPattern = regexp.MustCompile(`^\[!(NOTE|IMPORTANT|TIP|WARNING|CAUTION)\]\s*`)

func panelKind(alert string) string {
	for kind, name := range alertNames {
		if name == alert {
			return kind
		}
	}
	return ""
}

// panelOf recognises a quote that starts with an alert such as [!NOTE], and
// returns the panel's kind and its content without that marker.
func panelOf(content []Node) (string, []Node, bool) {
	if len(content) == 0 || content[0].Type != TypeParagraph || len(content[0].Content) == 0 {
		return "", nil, false
	}
	lead := content[0].Content
	match := alertPattern.FindStringSubmatch(lead[0].Text)
	if lead[0].Type != TypeText || match == nil {
		return "", nil, false
	}
	lead[0].Text = lead[0].Text[len(match[0]):]
	if lead[0].Text == "" {
		lead = lead[1:]
	}
	rest := content[1:]
	if len(lead) > 0 {
		rest = append([]Node{{Type: TypeParagraph, Content: lead}}, rest...)
	}
	if len(rest) == 0 {
		rest = []Node{{Type: TypeParagraph}}
	}
	return panelKind(match[1]), rest, true
}

func isTaskList(l *ast.List) bool {
	if l.FirstChild() == nil {
		return false
	}
	for item := l.FirstChild(); item != nil; item = item.NextSibling() {
		first := item.FirstChild()
		if first == nil || first.FirstChild() == nil {
			return false
		}
		if _, ok := first.FirstChild().(*extast.TaskCheckBox); !ok {
			return false
		}
	}
	return true
}

func (c *converter) list(l *ast.List) Node {
	task := isTaskList(l)
	node := Node{Type: TypeBulletList}
	itemType := TypeListItem
	switch {
	case task:
		node.Type, itemType = TypeTaskList, TypeTaskItem
	case l.IsOrdered():
		node.Type = TypeOrderedList
		if l.Start > 1 {
			node.Attrs = map[string]any{"start": l.Start}
		}
	}
	for item := l.FirstChild(); item != nil; item = item.NextSibling() {
		child := Node{Type: itemType}
		if task {
			box, _ := item.FirstChild().FirstChild().(*extast.TaskCheckBox)
			child.Attrs = map[string]any{"checked": box != nil && box.IsChecked}
		}
		child.Content = c.blocks(item)
		if len(child.Content) == 0 {
			child.Content = []Node{{Type: TypeParagraph}}
		}
		node.Content = append(node.Content, child)
	}
	return node
}

func (c *converter) table(t *extast.Table) Node {
	node := Node{Type: TypeTable}
	for row := t.FirstChild(); row != nil; row = row.NextSibling() {
		_, header := row.(*extast.TableHeader)
		r := Node{Type: TypeTableRow}
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			kind := TypeTableCell
			if header {
				kind = TypeTableHeader
			}
			inline, hoisted := c.inlineWithImages(cell)
			content := []Node{{Type: TypeParagraph, Content: inline}}
			r.Content = append(r.Content, Node{Type: kind, Content: append(content, hoisted...)})
		}
		if len(r.Content) > 0 {
			node.Content = append(node.Content, r)
		}
	}
	return node
}

// inlineWithImages converts inline content. Images and files are block nodes in
// our model, so they are lifted out and placed after the text they appeared in.
func (c *converter) inlineWithImages(parent ast.Node) ([]Node, []Node) {
	var hoisted []Node
	inline := c.inline(parent, nil, &hoisted)
	return inline, hoisted
}

func withMark(marks []Mark, m Mark) []Mark {
	out := make([]Mark, 0, len(marks)+1)
	for _, e := range marks {
		if e.Type != m.Type {
			out = append(out, e)
		}
	}
	return append(out, m)
}

func (c *converter) inline(parent ast.Node, marks []Mark, hoisted *[]Node) []Node {
	var out []Node
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch v := n.(type) {
		case *ast.Text:
			out = append(out, textNode(string(v.Segment.Value(c.src)), marks)...)
			switch {
			case v.HardLineBreak():
				out = append(out, Node{Type: TypeHardBreak})
			case v.SoftLineBreak():
				out = append(out, textNode(" ", marks)...)
			}
		case *ast.String:
			out = append(out, textNode(string(v.Value), marks)...)
		case *ast.CodeSpan:
			var b strings.Builder
			for ch := v.FirstChild(); ch != nil; ch = ch.NextSibling() {
				if t, ok := ch.(*ast.Text); ok {
					b.Write(t.Segment.Value(c.src))
				}
			}
			out = append(out, textNode(b.String(), []Mark{{Type: MarkCode}})...)
		case *ast.Emphasis:
			m := MarkItalic
			if v.Level >= 2 {
				m = MarkBold
			}
			out = append(out, c.inline(v, withMark(marks, Mark{Type: m}), hoisted)...)
		case *extast.Strikethrough:
			out = append(out, c.inline(v, withMark(marks, Mark{Type: MarkStrike}), hoisted)...)
		case *ast.Link:
			out = append(out, c.link(string(v.Destination), string(v.Title), v, marks, hoisted)...)
		case *ast.AutoLink:
			url := string(v.URL(c.src))
			label := string(v.Label(c.src))
			if href, err := SafeHref(autoLinkHref(v, url)); err == nil {
				out = append(out, textNode(label, withMark(marks, Mark{Type: MarkLink, Attrs: map[string]any{"href": href}}))...)
			} else {
				out = append(out, textNode(label, marks)...)
			}
		case *ast.Image:
			out = append(out, c.image(v, marks, hoisted)...)
		case *ast.RawHTML:
			var b strings.Builder
			for i := 0; i < v.Segments.Len(); i++ {
				seg := v.Segments.At(i)
				b.Write(seg.Value(c.src))
			}
			c.warn("Inline HTML was kept as plain text.")
			out = append(out, textNode(b.String(), marks)...)
		case *extast.TaskCheckBox:
			// Represented by the task item itself.
		default:
			out = append(out, c.inline(n, marks, hoisted)...)
		}
	}
	return mergeText(out)
}

func autoLinkHref(v *ast.AutoLink, url string) string {
	if v.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(url), "mailto:") {
		return "mailto:" + url
	}
	return url
}

// fileName is the last part of a Markdown destination, as a person would read it.
func fileName(destination string) string {
	base := path.Base(destination)
	if decoded, err := url.PathUnescape(base); err == nil {
		base = decoded
	}
	return truncate(base, maxFilenameBytes)
}

func (c *converter) link(dest, title string, n ast.Node, marks []Mark, hoisted *[]Node) []Node {
	if c.attachments != nil {
		if id, ok := c.attachments(dest); ok {
			*hoisted = append(*hoisted, Node{Type: TypeFile, Attrs: map[string]any{"attachment_id": id.String(), "filename": fileName(dest)}})
			if n.PreviousSibling() == nil && n.NextSibling() == nil {
				return nil // the file was the whole line
			}
			return c.inline(n, marks, hoisted) // a sentence keeps its words
		}
	}
	if c.links != nil {
		if page, ok := c.links(dest); ok {
			dest = page
		}
	}
	href, err := SafeHref(dest)
	if err != nil {
		c.warn("A link to %q is not allowed and was removed.", truncate(dest, 80))
		return c.inline(n, marks, hoisted)
	}
	attrs := map[string]any{"href": href}
	if title != "" {
		attrs["title"] = truncate(title, 1000)
	}
	return c.inline(n, withMark(marks, Mark{Type: MarkLink, Attrs: attrs}), hoisted)
}

func (c *converter) image(v *ast.Image, marks []Mark, hoisted *[]Node) []Node {
	dest := string(v.Destination)
	var alt strings.Builder
	for ch := v.FirstChild(); ch != nil; ch = ch.NextSibling() {
		if t, ok := ch.(*ast.Text); ok {
			alt.Write(t.Segment.Value(c.src))
		}
	}
	if c.attachments != nil {
		if id, ok := c.attachments(dest); ok {
			attrs := map[string]any{"attachment_id": id.String()}
			if alt.Len() > 0 {
				attrs["alt"] = truncate(alt.String(), 1000)
			}
			if len(v.Title) > 0 {
				attrs["caption"] = truncate(strings.ReplaceAll(string(v.Title), `\"`, `"`), maxCaptionBytes)
			}
			*hoisted = append(*hoisted, Node{Type: TypeImage, Attrs: attrs})
			return nil
		}
	}
	// External images would leak who is reading, so they become links.
	label := alt.String()
	if label == "" {
		label = dest
	}
	if href, err := SafeHref(dest); err == nil {
		c.warn("The image %q is not an attachment and became a link.", truncate(dest, 80))
		return textNode(label, withMark(marks, Mark{Type: MarkLink, Attrs: map[string]any{"href": href}}))
	}
	c.warn("The image %q could not be imported.", truncate(dest, 80))
	return textNode(label, marks)
}

func sameMarks(a, b []Mark) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Type != b[i].Type || fmt.Sprint(a[i].Attrs) != fmt.Sprint(b[i].Attrs) {
			return false
		}
	}
	return true
}

func mergeText(in []Node) []Node {
	var out []Node
	for _, n := range in {
		if k := len(out) - 1; k >= 0 && n.Type == TypeText && out[k].Type == TypeText && sameMarks(out[k].Marks, n.Marks) {
			out[k].Text += n.Text
			continue
		}
		out = append(out, n)
	}
	return out
}

// AttachmentTarget returns the Markdown destination of an attachment.
type AttachmentTarget func(id uuid.UUID) string

// ToMarkdown writes a document as GitHub-flavoured Markdown.
func ToMarkdown(doc Node, target AttachmentTarget) string {
	w := &mdWriter{target: target}
	w.blocks(doc.Content, "")
	return strings.TrimRight(w.b.String(), "\n") + "\n"
}

type mdWriter struct {
	b      strings.Builder
	target AttachmentTarget
}

// destination returns where a node's attachment lives in the export.
func (w *mdWriter) destination(n Node) string {
	id, _ := n.Attrs["attachment_id"].(string)
	if parsed, err := uuid.Parse(id); err == nil && w.target != nil {
		return w.target(parsed)
	}
	return id
}

func (w *mdWriter) blocks(nodes []Node, prefix string) {
	for i, n := range nodes {
		if i > 0 {
			w.b.WriteString(strings.TrimRight(prefix, " ") + "\n")
		}
		w.block(n, prefix)
	}
}

func (w *mdWriter) line(prefix, s string) {
	for i, l := range strings.Split(s, "\n") {
		_ = i
		w.b.WriteString(prefix + l + "\n")
	}
}

func (w *mdWriter) block(n Node, prefix string) {
	switch n.Type {
	case TypeParagraph:
		w.line(prefix, inlineMarkdown(n.Content))
	case TypeHeading:
		level, _ := intAttr(n.Attrs["level"])
		w.line(prefix, strings.Repeat("#", min(max(level, 1), 6))+" "+inlineMarkdown(n.Content))
	case TypeBlockquote:
		w.blocks(n.Content, prefix+"> ")
	case TypeHorizontalRule:
		w.line(prefix, "---")
	case TypeCodeBlock:
		lang, _ := n.Attrs["language"].(string)
		code := BlockText(n)
		fence := "```"
		for strings.Contains(code, fence) {
			fence += "`"
		}
		w.line(prefix, fence+lang)
		w.line(prefix, code)
		w.line(prefix, fence)
	case TypeBulletList, TypeOrderedList, TypeTaskList:
		start := 1
		if v, ok := intAttr(n.Attrs["start"]); ok {
			start = v
		}
		for i, item := range n.Content {
			marker := "- "
			switch n.Type {
			case TypeOrderedList:
				marker = strconv.Itoa(start+i) + ". "
			case TypeTaskList:
				marker = "- [ ] "
				if checked, _ := item.Attrs["checked"].(bool); checked {
					marker = "- [x] "
				}
			}
			w.listItem(item, prefix, marker)
		}
	case TypeImage:
		alt, _ := n.Attrs["alt"].(string)
		title := ""
		if caption := wordsOf(n); caption != "" {
			title = ` "` + strings.ReplaceAll(caption, `"`, `\"`) + `"`
		}
		w.line(prefix, "!["+escapeInline(alt)+"]("+w.destination(n)+title+")")
	case TypeFile:
		name, _ := n.Attrs["filename"].(string)
		w.line(prefix, "["+escapeInline(name)+"]("+w.destination(n)+")")
	case TypePanel:
		kind, _ := n.Attrs["kind"].(string)
		w.line(prefix+"> ", "[!"+alertNames[kind]+"]")
		w.blocks(n.Content, prefix+"> ")
	case TypeExpand:
		if title := wordsOf(n); title != "" {
			w.line(prefix, "**"+escapeInline(title)+"**")
			w.line(prefix, "")
		}
		w.blocks(n.Content, prefix)
	case TypeDecision:
		w.line(prefix, "**Decision:** "+inlineMarkdown(n.Content))
	case TypeTable:
		w.table(n, prefix)
	case TypeDynamic:
		kind, _ := n.Attrs["kind"].(string)
		w.line(prefix, "*(A dynamic block: "+kind+")*")
	case TypeSheet:
		w.b.WriteString(sheetMarkdown(n))
	case TypeChart:
		title, _ := n.Attrs["title"].(string)
		w.line(prefix, "*(A chart: "+title+")*")
	default:
		w.blocks(n.Content, prefix)
	}
}

func (w *mdWriter) listItem(item Node, prefix, marker string) {
	inner := &mdWriter{target: w.target}
	inner.blocks(item.Content, "")
	lines := strings.Split(strings.TrimRight(inner.b.String(), "\n"), "\n")
	pad := strings.Repeat(" ", len(marker))
	for i, l := range lines {
		switch {
		case i == 0:
			w.b.WriteString(prefix + marker + l + "\n")
		case l == "":
			w.b.WriteString(strings.TrimRight(prefix, " ") + "\n")
		default:
			w.b.WriteString(prefix + pad + l + "\n")
		}
	}
}

func (w *mdWriter) table(n Node, prefix string) {
	caption, below := TableCaption(n)
	if caption != "" && !below {
		w.line(prefix, "*"+escapeInline(caption)+"*")
		w.line(prefix, "")
	}
	for i, row := range n.Content {
		cells := make([]string, 0, len(row.Content))
		for _, cell := range row.Content {
			var parts []string
			for _, b := range cell.Content {
				parts = append(parts, inlineMarkdown(b.Content))
			}
			cells = append(cells, strings.ReplaceAll(strings.Join(parts, " "), "|", `\|`))
		}
		w.line(prefix, "| "+strings.Join(cells, " | ")+" |")
		if i == 0 {
			w.line(prefix, "|"+strings.Repeat(" --- |", max(len(cells), 1)))
		}
	}
	if caption != "" && below {
		w.line(prefix, "")
		w.line(prefix, "*"+escapeInline(caption)+"*")
	}
}

var inlineEscaper = strings.NewReplacer(`\`, `\\`, "`", "\\`", `*`, `\*`, `_`, `\_`, `[`, `\[`, `]`, `\]`, `<`, `\<`, `~`, `\~`)

func escapeInline(s string) string { return inlineEscaper.Replace(s) }

func hasMark(n Node, t string) (Mark, bool) {
	for _, m := range n.Marks {
		if m.Type == t {
			return m, true
		}
	}
	return Mark{}, false
}

func inlineMarkdown(nodes []Node) string {
	var b strings.Builder
	for _, n := range nodes {
		if n.Type == TypeHardBreak {
			b.WriteString("\\\n")
			continue
		}
		if n.Type == TypeStatus || n.Type == TypeDate || n.Type == TypeMention {
			b.WriteString(escapeInline(wordsOf(n)))
			continue
		}
		if n.Type != TypeText {
			continue
		}
		if _, code := hasMark(n, MarkCode); code {
			fence := "`"
			for strings.Contains(n.Text, fence) {
				fence += "`"
			}
			s := fence + n.Text + fence
			if link, ok := hasMark(n, MarkLink); ok {
				s = "[" + s + "](" + hrefOf(link) + ")"
			}
			b.WriteString(s)
			continue
		}
		s := escapeInline(n.Text)
		// Emphasis cannot wrap surrounding spaces, so keep them outside the markers.
		trimmed := strings.TrimSpace(s)
		lead := s[:strings.Index(s, trimmed)]
		trail := s[len(lead)+len(trimmed):]
		if trimmed == "" {
			b.WriteString(s)
			continue
		}
		s = trimmed
		if _, ok := hasMark(n, MarkStrike); ok {
			s = "~~" + s + "~~"
		}
		if _, ok := hasMark(n, MarkItalic); ok {
			s = "*" + s + "*"
		}
		if _, ok := hasMark(n, MarkBold); ok {
			s = "**" + s + "**"
		}
		if link, ok := hasMark(n, MarkLink); ok {
			s = "[" + s + "](" + hrefOf(link) + ")"
		}
		b.WriteString(lead + s + trail)
	}
	return b.String()
}

func hrefOf(m Mark) string {
	href, _ := m.Attrs["href"].(string)
	href = strings.NewReplacer("(", "%28", ")", "%29", " ", "%20").Replace(href)
	return href
}
