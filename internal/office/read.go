// SPDX-License-Identifier: Apache-2.0

package office

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
)

// Limits on a Word file, which is always hostile until proven otherwise.
const (
	maxPartBytes   int64 = 64 << 20
	maxMediaBytes  int64 = 100 << 20
	maxWordEntries       = 5000
	maxXMLDepth          = 256
	maxXMLTokens         = 5_000_000
)

// xmlNode is an element of a small tree. Parts of a Word file are read whole, with limits.
type xmlNode struct {
	name     string
	attrs    map[string]string
	children []*xmlNode
	text     string
}

func (n *xmlNode) child(name string) *xmlNode {
	for _, c := range n.children {
		if c.name == name {
			return c
		}
	}
	return nil
}

func (n *xmlNode) walk(visit func(*xmlNode)) {
	visit(n)
	for _, c := range n.children {
		c.walk(visit)
	}
}

// parseXML reads a part. encoding/xml fetches no external entities and expands none of
// its own, so the usual XML attacks do not apply. Depth and size are ours to limit.
func parseXML(data []byte) (*xmlNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	root := &xmlNode{name: "#root"}
	stack := []*xmlNode{root}
	for tokens := 0; ; tokens++ {
		if tokens > maxXMLTokens {
			return nil, apperr.TooLarge("This Word file is too large to read.")
		}
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			return root, nil
		}
		if err != nil {
			return nil, apperr.Invalid("This Word file cannot be read.")
		}
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) > maxXMLDepth {
				return nil, apperr.Invalid("This Word file is nested too deeply.")
			}
			n := &xmlNode{name: t.Name.Local, attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.attrs[a.Name.Local] = a.Value
			}
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			top.text += string(t)
		}
	}
}

// ReadResult is what a Word file holds.
type ReadResult struct {
	Document document.Node
	Title    string
	Warnings []string
}

// Attach stores an image from a Word file, and gives back the attrs of its node.
type Attach func(name string, data []byte) (map[string]any, bool)

type wordReader struct {
	files     map[string]*zip.File
	base      string
	rels      map[string]relation
	outlines  map[string]int
	numbering map[string]string
	attach    Attach
	warnings  map[string]bool
	order     []string
}

type relation struct {
	target   string
	external bool
}

func (r *wordReader) warn(text string) {
	if !r.warnings[text] {
		r.warnings[text] = true
		r.order = append(r.order, text)
	}
}

func readPart(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, apperr.Invalid("This Word file cannot be read.")
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, apperr.Invalid("This Word file cannot be read.")
	}
	if int64(len(data)) > limit {
		return nil, apperr.TooLarge("A part of this Word file is too large.")
	}
	return data, nil
}

func (r *wordReader) xmlPart(name string) (*xmlNode, error) {
	f, ok := r.files[name]
	if !ok {
		return nil, nil
	}
	data, err := readPart(f, maxPartBytes)
	if err != nil {
		return nil, err
	}
	return parseXML(data)
}

// relations reads the relationships of a part. Targets are only names inside the
// package, and an external one is noted, never fetched.
func (r *wordReader) relations(part string) (map[string]relation, error) {
	name := path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	root, err := r.xmlPart(name)
	if root == nil || err != nil {
		return map[string]relation{}, err
	}
	out := map[string]relation{}
	root.walk(func(n *xmlNode) {
		if n.name == "Relationship" {
			out[n.attrs["Id"]] = relation{target: n.attrs["Target"], external: n.attrs["TargetMode"] == "External"}
		}
	})
	return out, nil
}

func mainPart(r *wordReader) (string, error) {
	root, err := r.xmlPart("_rels/.rels")
	if err != nil {
		return "", err
	}
	if root != nil {
		found := ""
		root.walk(func(n *xmlNode) {
			if n.name == "Relationship" && strings.HasSuffix(n.attrs["Type"], "/officeDocument") && n.attrs["TargetMode"] != "External" {
				found = strings.TrimPrefix(n.attrs["Target"], "/")
			}
		})
		if found != "" {
			return path.Clean(found), nil
		}
	}
	return "", apperr.Invalid("This file is not a Word document.")
}

func looksLikeOldWord(data []byte) bool {
	return bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0})
}

// ReadDOCX turns a Word file into a document. What it cannot carry over is
// dropped with a warning, and nothing is ever fetched.
func ReadDOCX(data []byte, attach Attach) (ReadResult, error) {
	if looksLikeOldWord(data) {
		return ReadResult{}, apperr.Invalid("This is an old Word file, or one with a password. Save it as .docx without a password first.")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ReadResult{}, apperr.Invalid("This file is not a Word document.")
	}
	if len(zr.File) > maxWordEntries {
		return ReadResult{}, apperr.TooLarge("This Word file has too many parts.")
	}
	r := &wordReader{files: map[string]*zip.File{}, attach: attach, warnings: map[string]bool{},
		outlines: map[string]int{}, numbering: map[string]string{}}
	for _, f := range zr.File {
		r.files[f.Name] = f
		if strings.HasSuffix(strings.ToLower(f.Name), "vbaproject.bin") {
			r.warn("Macros were left out.")
		}
		if strings.Contains(f.Name, "/embeddings/") {
			r.warn("Embedded objects were left out.")
		}
	}
	part, err := mainPart(r)
	if err != nil {
		return ReadResult{}, err
	}
	r.base = path.Dir(part)
	if r.rels, err = r.relations(part); err != nil {
		return ReadResult{}, err
	}
	if err := r.readStyles(); err != nil {
		return ReadResult{}, err
	}
	if err := r.readNumbering(); err != nil {
		return ReadResult{}, err
	}
	root, err := r.xmlPart(part)
	if err != nil {
		return ReadResult{}, err
	}
	if root == nil {
		return ReadResult{}, apperr.Invalid("This file is not a Word document.")
	}
	var body *xmlNode
	root.walk(func(n *xmlNode) {
		if n.name == "body" && body == nil {
			body = n
		}
	})
	if body == nil {
		return ReadResult{}, apperr.Invalid("This Word file has no text.")
	}
	doc := document.Node{Type: document.TypeDoc, Content: r.blocks(body)}
	if len(doc.Content) == 0 {
		doc = document.Empty()
	}
	return ReadResult{Document: doc, Warnings: r.order}, nil
}

func (r *wordReader) readStyles() error {
	root, err := r.xmlPart(path.Join(r.base, "styles.xml"))
	if root == nil || err != nil {
		return err
	}
	root.walk(func(n *xmlNode) {
		if n.name != "style" {
			return
		}
		// The name differs per language, and the outline level does not.
		pPr := n.child("pPr")
		if pPr == nil {
			return
		}
		if lvl := pPr.child("outlineLvl"); lvl != nil {
			if v, err := strconv.Atoi(lvl.attrs["val"]); err == nil && v >= 0 && v < maxHeadingLevel {
				r.outlines[n.attrs["styleId"]] = v + 1
			}
		}
	})
	return nil
}

func (r *wordReader) readNumbering() error {
	root, err := r.xmlPart(path.Join(r.base, "numbering.xml"))
	if root == nil || err != nil {
		return err
	}
	formats := map[string]string{}
	root.walk(func(n *xmlNode) {
		if n.name == "abstractNum" {
			if lvl := n.child("lvl"); lvl != nil {
				if f := lvl.child("numFmt"); f != nil {
					formats[n.attrs["abstractNumId"]] = f.attrs["val"]
				}
			}
		}
	})
	root.walk(func(n *xmlNode) {
		if n.name == "num" {
			if a := n.child("abstractNumId"); a != nil {
				r.numbering[n.attrs["numId"]] = formats[a.attrs["val"]]
			}
		}
	})
	return nil
}

func (r *wordReader) headingLevel(pPr *xmlNode) int {
	if pPr == nil {
		return 0
	}
	if lvl := pPr.child("outlineLvl"); lvl != nil {
		if v, err := strconv.Atoi(lvl.attrs["val"]); err == nil && v >= 0 && v < maxHeadingLevel {
			return v + 1
		}
	}
	if style := pPr.child("pStyle"); style != nil {
		if level := r.outlines[style.attrs["val"]]; level > 0 {
			return level
		}
		id := strings.ToLower(style.attrs["val"])
		if rest, ok := strings.CutPrefix(id, "heading"); ok {
			if v, err := strconv.Atoi(rest); err == nil && v >= 1 && v <= maxHeadingLevel {
				return v
			}
		}
	}
	return 0
}

func (r *wordReader) blocks(parent *xmlNode) []document.Node {
	var out []document.Node
	var list *document.Node
	flush := func() {
		if list != nil {
			out = append(out, *list)
			list = nil
		}
	}
	for _, c := range parent.children {
		switch c.name {
		case "p":
			if kind := r.listKind(c); kind != "" {
				if list == nil || list.Type != kind {
					flush()
					list = &document.Node{Type: kind}
				}
				list.Content = append(list.Content, document.Node{Type: document.TypeListItem, Content: []document.Node{r.paragraph(c, 0)}})
				continue
			}
			flush()
			if block, ok := r.block(c); ok {
				out = append(out, block...)
			}
		case "tbl":
			flush()
			out = append(out, r.table(c))
		case "sdt":
			flush()
			if content := c.child("sdtContent"); content != nil {
				out = append(out, r.blocks(content)...)
			}
		}
	}
	flush()
	return out
}

func (r *wordReader) listKind(p *xmlNode) string {
	pPr := p.child("pPr")
	if pPr == nil || pPr.child("numPr") == nil {
		return ""
	}
	id := pPr.child("numPr").child("numId")
	if id == nil || id.attrs["val"] == "0" {
		return ""
	}
	if r.numbering[id.attrs["val"]] == "bullet" {
		return document.TypeBulletList
	}
	return document.TypeOrderedList
}

func (r *wordReader) block(p *xmlNode) ([]document.Node, bool) {
	level := r.headingLevel(p.child("pPr"))
	para := r.paragraph(p, level)
	var images []document.Node
	kept := para.Content[:0:0]
	for _, c := range para.Content {
		if c.Type == document.TypeImage {
			images = append(images, c)
		} else {
			kept = append(kept, c)
		}
	}
	para.Content = kept
	if len(kept) == 0 {
		return images, len(images) > 0
	}
	return append([]document.Node{para}, images...), true
}

func (r *wordReader) paragraph(p *xmlNode, level int) document.Node {
	n := document.Node{Type: document.TypeParagraph}
	if level > 0 {
		n.Type = document.TypeHeading
		n.Attrs = map[string]any{"level": level}
	}
	for _, c := range p.children {
		n.Content = append(n.Content, r.inline(c, nil)...)
	}
	return n
}

func runMarks(run *xmlNode) []document.Mark {
	var marks []document.Mark
	props := run.child("rPr")
	if props == nil {
		return nil
	}
	isOn := func(name string) bool {
		c := props.child(name)
		return c != nil && c.attrs["val"] != "0" && c.attrs["val"] != "false" && c.attrs["val"] != "none"
	}
	for name, mark := range map[string]string{"b": document.MarkBold, "i": document.MarkItalic, "u": document.MarkUnderline, "strike": document.MarkStrike} {
		if isOn(name) {
			marks = append(marks, document.Mark{Type: mark})
		}
	}
	if v := props.child("vertAlign"); v != nil {
		switch v.attrs["val"] {
		case "subscript":
			marks = append(marks, document.Mark{Type: document.MarkSubscript})
		case "superscript":
			marks = append(marks, document.Mark{Type: document.MarkSuperscript})
		}
	}
	return marks
}

func (r *wordReader) inline(c *xmlNode, link []document.Mark) []document.Node {
	switch c.name {
	case "r":
		return r.run(c, link)
	case "ins", "smartTag", "sdt", "sdtContent", "fldSimple":
		var out []document.Node
		for _, inner := range c.children {
			out = append(out, r.inline(inner, link)...)
		}
		return out
	case "del", "moveFrom":
		r.warn("Tracked changes were accepted.")
		return nil
	case "commentRangeStart":
		r.warn("Comments in the file were left out.")
		return nil
	case "hyperlink":
		return r.hyperlink(c)
	}
	return nil
}

func (r *wordReader) hyperlink(c *xmlNode) []document.Node {
	var marks []document.Mark
	if rel, ok := r.rels[c.attrs["id"]]; ok && rel.external {
		if href, err := document.SafeHref(rel.target); err == nil {
			marks = []document.Mark{{Type: document.MarkLink, Attrs: map[string]any{"href": href}}}
		}
	}
	var out []document.Node
	for _, inner := range c.children {
		out = append(out, r.inline(inner, marks)...)
	}
	return out
}

func (r *wordReader) run(c *xmlNode, link []document.Mark) []document.Node {
	marks := append(runMarks(c), link...)
	var out []document.Node
	for _, part := range c.children {
		switch part.name {
		case "t":
			if part.text != "" {
				out = append(out, document.Node{Type: document.TypeText, Text: part.text, Marks: marks})
			}
		case "tab":
			out = append(out, document.Node{Type: document.TypeText, Text: "\t", Marks: marks})
		case "br":
			out = append(out, document.Node{Type: document.TypeHardBreak})
		case "drawing", "pict":
			out = append(out, r.images(part)...)
		case "object":
			r.warn("Embedded objects were left out.")
		}
	}
	return out
}

func imageName(target string) string { return path.Base(target) }

func (r *wordReader) images(part *xmlNode) []document.Node {
	var out []document.Node
	alt := ""
	part.walk(func(n *xmlNode) {
		if n.name == "docPr" && alt == "" {
			alt = n.attrs["descr"]
		}
	})
	part.walk(func(n *xmlNode) {
		if n.name != "blip" {
			return
		}
		rel, ok := r.rels[n.attrs["embed"]]
		if !ok && n.attrs["link"] != "" {
			r.warn("An image outside the file was left out.")
			return
		}
		if !ok || rel.external {
			r.warn("An image outside the file was left out.")
			return
		}
		name := path.Clean(path.Join(r.base, rel.target))
		f, found := r.files[name]
		if !found || r.attach == nil {
			return
		}
		data, err := readPart(f, maxMediaBytes)
		if err != nil {
			r.warn("An image could not be read.")
			return
		}
		attrs, stored := r.attach(imageName(rel.target), data)
		if !stored {
			r.warn("An image could not be stored.")
			return
		}
		if alt != "" {
			attrs["alt"] = alt
		}
		out = append(out, document.Node{Type: document.TypeImage, Attrs: attrs})
	})
	return out
}

func (r *wordReader) table(t *xmlNode) document.Node {
	table := document.Node{Type: document.TypeTable}
	first := true
	for _, tr := range t.children {
		if tr.name != "tr" {
			continue
		}
		row := document.Node{Type: document.TypeTableRow}
		isHeader := first
		if props := tr.child("trPr"); props == nil || props.child("tblHeader") == nil {
			isHeader = false
		}
		for _, tc := range tr.children {
			if tc.name != "tc" {
				continue
			}
			cell := document.Node{Type: document.TypeTableCell}
			if isHeader {
				cell.Type = document.TypeTableHeader
			}
			if props := tc.child("tcPr"); props != nil {
				if span := props.child("gridSpan"); span != nil {
					if v, err := strconv.Atoi(span.attrs["val"]); err == nil && v > 1 {
						cell.Attrs = map[string]any{"colspan": v}
					}
				}
			}
			cell.Content = r.blocks(tc)
			if len(cell.Content) == 0 {
				cell.Content = []document.Node{{Type: document.TypeParagraph}}
			}
			row.Content = append(row.Content, cell)
		}
		if len(row.Content) > 0 {
			table.Content = append(table.Content, row)
		}
		first = false
	}
	return table
}
