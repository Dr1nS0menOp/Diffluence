// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/diffluence/diffluence/internal/document"
)

const maxHTMLDepth = 200

// Elements whose content is never text for a reader. Everything else is read for
// its words, so that what we do not know stays visible.
var droppedElements = map[atom.Atom]bool{atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Iframe: true,
	atom.Object: true, atom.Embed: true, atom.Template: true, atom.Head: true, atom.Svg: true, atom.Canvas: true, atom.Form: true}

var inlineMarks = map[atom.Atom]string{atom.Strong: document.MarkBold, atom.B: document.MarkBold, atom.Em: document.MarkItalic,
	atom.I: document.MarkItalic, atom.U: document.MarkUnderline, atom.S: document.MarkStrike, atom.Del: document.MarkStrike,
	atom.Code: document.MarkCode, atom.Sub: document.MarkSubscript, atom.Sup: document.MarkSuperscript}

type htmlReader struct {
	m       *importer
	folder  string
	store   func(src string) (uuid.UUID, bool)
	dropped bool
}

func attribute(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

func titleOf(root *html.Node) string {
	var title, heading string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.DataAtom == atom.Title && title == "" {
				title = strings.TrimSpace(wordsOf(n))
			}
			if n.DataAtom == atom.H1 && heading == "" {
				heading = strings.TrimSpace(wordsOf(n))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if title != "" {
		return title
	}
	return heading
}

func wordsOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		if n.Type == html.ElementNode && droppedElements[n.DataAtom] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func bodyOf(root *html.Node) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.DataAtom == atom.Body && found == nil {
			found = n
		}
		for c := n.FirstChild; c != nil && found == nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		return root
	}
	return found
}

func convertHTML(ctx context.Context, m *importer, pageID uuid.UUID, key string, data []byte) (converted, error) {
	root, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return converted{warnings: []string{"This file could not be read as HTML."}, doc: document.Empty()}, nil
	}
	folder := path.Dir(key)
	r := &htmlReader{m: m, folder: folder, store: func(src string) (uuid.UUID, bool) { return m.attachment(ctx, pageID, folder, src) }}
	doc, warnings := r.document(root)
	return converted{doc: doc, title: titleOf(root), warnings: warnings}, nil
}

// document turns parsed HTML into a page that passes the gate, with warnings for what was left out.
func (r *htmlReader) document(root *html.Node) (document.Node, []string) {
	doc := document.Node{Type: document.TypeDoc, Content: r.blocks(bodyOf(root), 0)}
	if len(doc.Content) == 0 {
		doc = document.Empty()
	}
	var warnings []string
	if r.dropped {
		warnings = append(warnings, "Scripts, forms and embedded objects were left out.")
	}
	doc = checked(doc, &warnings)
	return doc, warnings
}

func isBlock(a atom.Atom) bool {
	switch a {
	case atom.P, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol, atom.Blockquote, atom.Pre, atom.Hr,
		atom.Table, atom.Div, atom.Section, atom.Article, atom.Main, atom.Aside, atom.Header, atom.Footer, atom.Nav, atom.Img,
		atom.Figure, atom.Details, atom.Dl:
		return true
	}
	return false
}

// blocks reads the children of an element. Loose inline content is gathered into
// a paragraph, because a document only holds text inside blocks.
func (r *htmlReader) blocks(parent *html.Node, depth int) []document.Node {
	if depth > maxHTMLDepth {
		return nil
	}
	var out []document.Node
	var loose []document.Node
	flush := func() {
		if hasWords(loose) {
			out = append(out, document.Node{Type: document.TypeParagraph, Content: loose})
		}
		loose = nil
	}
	for c := parent.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && droppedElements[c.DataAtom] {
			r.dropped = r.dropped || c.DataAtom != atom.Head
			continue
		}
		if c.Type == html.ElementNode && isBlock(c.DataAtom) {
			flush()
			out = append(out, r.block(c, depth)...)
			continue
		}
		loose = append(loose, r.inline(c, nil, depth)...)
	}
	flush()
	return out
}

func hasWords(list []document.Node) bool {
	for _, n := range list {
		if n.Type != document.TypeText || strings.TrimSpace(n.Text) != "" {
			return true
		}
	}
	return false
}

func (r *htmlReader) block(n *html.Node, depth int) []document.Node {
	switch n.DataAtom {
	case atom.P:
		return r.paragraphs(n, depth)
	case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
		level, _ := strconv.Atoi(n.Data[1:])
		return []document.Node{{Type: document.TypeHeading, Attrs: map[string]any{"level": level}, Content: r.inlines(n, depth)}}
	case atom.Ul, atom.Ol:
		return r.list(n, depth)
	case atom.Blockquote:
		return []document.Node{{Type: document.TypeBlockquote, Content: paragraphsOnly(r.blocks(n, depth+1))}}
	case atom.Pre:
		return []document.Node{{Type: document.TypeCodeBlock, Content: []document.Node{{Type: document.TypeText, Text: wordsOf(n)}}}}
	case atom.Hr:
		return []document.Node{{Type: document.TypeHorizontalRule}}
	case atom.Table:
		return r.table(n, depth)
	case atom.Img:
		return r.image(n)
	}
	return r.blocks(n, depth+1)
}

// paragraphsOnly keeps a quote simple: other blocks become their words.
func paragraphsOnly(list []document.Node) []document.Node {
	out := make([]document.Node, 0, len(list))
	for _, n := range list {
		if n.Type == document.TypeParagraph {
			out = append(out, n)
			continue
		}
		if words := document.Text(n); words != "" {
			out = append(out, paragraph(words))
		}
	}
	if len(out) == 0 {
		out = append(out, document.Node{Type: document.TypeParagraph})
	}
	return out
}

// paragraphs reads a <p>, which can hold an image a document keeps as a block.
func (r *htmlReader) paragraphs(n *html.Node, depth int) []document.Node {
	content := r.inlines(n, depth)
	var images []document.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.DataAtom == atom.Img {
			images = append(images, r.image(c)...)
		}
	}
	if !hasWords(content) {
		return images
	}
	return append([]document.Node{{Type: document.TypeParagraph, Content: content}}, images...)
}

func (r *htmlReader) inlines(n *html.Node, depth int) []document.Node {
	var out []document.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, r.inline(c, nil, depth)...)
	}
	return out
}

func (r *htmlReader) inline(n *html.Node, marks []document.Mark, depth int) []document.Node {
	if depth > maxHTMLDepth {
		return nil
	}
	switch n.Type {
	case html.TextNode:
		text := strings.Join(strings.Fields(strings.ReplaceAll(n.Data, " ", " ")), " ")
		if text == "" {
			return nil
		}
		// One space stays between words of neighbouring elements.
		if strings.TrimLeft(n.Data, " \n\t") != n.Data {
			text = " " + text
		}
		if strings.TrimRight(n.Data, " \n\t") != n.Data {
			text += " "
		}
		return []document.Node{{Type: document.TypeText, Text: text, Marks: marks}}
	case html.ElementNode:
	default:
		return nil
	}
	if droppedElements[n.DataAtom] {
		r.dropped = true
		return nil
	}
	if n.DataAtom == atom.Br {
		return []document.Node{{Type: document.TypeHardBreak}}
	}
	if n.DataAtom == atom.Img {
		return nil
	}
	if mark, ok := inlineMarks[n.DataAtom]; ok {
		marks = append(append([]document.Mark{}, marks...), document.Mark{Type: mark})
	}
	if n.DataAtom == atom.A {
		marks = r.linkMark(n, marks)
	}
	var out []document.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, r.inline(c, marks, depth+1)...)
	}
	return out
}

func (r *htmlReader) linkMark(n *html.Node, marks []document.Mark) []document.Mark {
	dest := attribute(n, "href")
	if page, ok := r.m.pageLink(r.folder, dest); ok {
		dest = page
	}
	href, err := document.SafeHref(dest)
	if err != nil {
		return marks
	}
	return append(append([]document.Mark{}, marks...), document.Mark{Type: document.MarkLink, Attrs: map[string]any{"href": href}})
}

func (r *htmlReader) list(n *html.Node, depth int) []document.Node {
	list := document.Node{Type: document.TypeBulletList}
	if n.DataAtom == atom.Ol {
		list.Type = document.TypeOrderedList
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.DataAtom != atom.Li {
			continue
		}
		content := r.blocks(c, depth+1)
		if len(content) == 0 {
			content = []document.Node{{Type: document.TypeParagraph}}
		}
		if content[0].Type != document.TypeParagraph {
			content = append([]document.Node{{Type: document.TypeParagraph}}, content...)
		}
		list.Content = append(list.Content, document.Node{Type: document.TypeListItem, Content: content})
	}
	if len(list.Content) == 0 {
		return nil
	}
	return []document.Node{list}
}

func (r *htmlReader) table(n *html.Node, depth int) []document.Node {
	table := document.Node{Type: document.TypeTable}
	var walk func(*html.Node)
	walk = func(p *html.Node) {
		for c := p.FirstChild; c != nil; c = c.NextSibling {
			switch c.DataAtom {
			case atom.Tr:
				if row := r.row(c, depth); len(row.Content) > 0 {
					table.Content = append(table.Content, row)
				}
			case atom.Thead, atom.Tbody, atom.Tfoot:
				walk(c)
			}
		}
	}
	walk(n)
	if len(table.Content) == 0 {
		return nil
	}
	return []document.Node{table}
}

func (r *htmlReader) row(tr *html.Node, depth int) document.Node {
	row := document.Node{Type: document.TypeTableRow}
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.DataAtom != atom.Td && c.DataAtom != atom.Th {
			continue
		}
		cell := document.Node{Type: document.TypeTableCell}
		if c.DataAtom == atom.Th {
			cell.Type = document.TypeTableHeader
		}
		if span, err := strconv.Atoi(attribute(c, "colspan")); err == nil && span > 1 && span < 1000 {
			cell.Attrs = map[string]any{"colspan": span}
		}
		cell.Content = r.blocks(c, depth+1)
		if len(cell.Content) == 0 {
			cell.Content = []document.Node{{Type: document.TypeParagraph}}
		}
		row.Content = append(row.Content, cell)
	}
	return row
}

// image stores an image from the archive. An outside image becomes a link, because
// we never show what lives elsewhere (G11).
func (r *htmlReader) image(n *html.Node) []document.Node {
	src := attribute(n, "src")
	alt := attribute(n, "alt")
	if id, ok := r.store(src); ok {
		attrs := map[string]any{"attachment_id": id.String()}
		if alt != "" {
			attrs["alt"] = alt
		}
		return []document.Node{{Type: document.TypeImage, Attrs: attrs}}
	}
	href, err := document.SafeHref(src)
	if err != nil || !strings.Contains(src, "://") {
		if alt == "" {
			return nil
		}
		return []document.Node{paragraph(alt)}
	}
	words := alt
	if words == "" {
		words = fmt.Sprintf("Image: %s", truncate(src, maxReportedName))
	}
	return []document.Node{{Type: document.TypeParagraph, Content: []document.Node{{Type: document.TypeText, Text: words,
		Marks: []document.Mark{{Type: document.MarkLink, Attrs: map[string]any{"href": href}}}}}}}
}
