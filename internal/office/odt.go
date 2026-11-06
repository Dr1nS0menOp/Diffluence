// SPDX-License-Identifier: Apache-2.0

package office

import (
	"archive/zip"
	"fmt"
	"io"
	"strings"

	"github.com/diffluence/diffluence/internal/document"
)

const (
	odtNS = `xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" ` +
		`xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" ` +
		`xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0" ` +
		`xmlns:style="urn:oasis:names:tc:opendocument:xmlns:style:1.0" ` +
		`xmlns:fo="urn:oasis:names:tc:opendocument:xmlns:xsl-fo-compatible:1.0" ` +
		`xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0" ` +
		`xmlns:xlink="http://www.w3.org/1999/xlink"`
	odtMime = "application/vnd.oasis.opendocument.text"
)

type odtWriter struct {
	b       strings.Builder
	o       Options
	depth   int
	media   map[string][]byte
	types   map[string]string
	nextPic int
}

// WriteODT writes pages as one OpenDocument text file.
func WriteODT(w io.Writer, sections []Section, o Options) error {
	d := &odtWriter{o: o, media: map[string][]byte{}, types: map[string]string{}}
	for _, s := range sections {
		fmt.Fprintf(&d.b, `<text:h text:style-name="H%d" text:outline-level="%d">%s</text:h>`,
			min(s.Depth+1, maxHeadingLevel), min(s.Depth+1, maxHeadingLevel), escape(s.Title))
		d.depth = s.Depth + 1
		for _, n := range s.Document.Content {
			d.block(n)
		}
	}
	return d.pack(w)
}

func odtStyles() string {
	var b strings.Builder
	b.WriteString(`<style:style style:name="Body" style:family="paragraph"/>`)
	sizes := []string{"20pt", "16pt", "14pt", "12pt", "11pt", "10pt"}
	for i, size := range sizes {
		fmt.Fprintf(&b, `<style:style style:name="H%d" style:family="paragraph" style:default-outline-level="%d">`+
			`<style:text-properties fo:font-size="%s" fo:font-weight="bold"/></style:style>`, i+1, i+1, size)
	}
	b.WriteString(`<style:style style:name="Code" style:family="paragraph"><style:text-properties style:font-name="Liberation Mono"/></style:style>`)
	b.WriteString(`<style:style style:name="Quote" style:family="paragraph"><style:text-properties fo:font-style="italic"/></style:style>`)
	b.WriteString(`<style:style style:name="Center" style:family="paragraph"><style:paragraph-properties fo:text-align="center"/></style:style>`)
	b.WriteString(`<style:style style:name="End" style:family="paragraph"><style:paragraph-properties fo:text-align="end"/></style:style>`)
	for name, props := range map[string]string{
		"Bold": `fo:font-weight="bold"`, "Italic": `fo:font-style="italic"`,
		"Underline": `style:text-underline-style="solid"`, "Strike": `style:text-line-through-style="solid"`,
		"Mono": `style:font-name="Liberation Mono"`, "Sub": `style:text-position="sub 58%"`, "Super": `style:text-position="super 58%"`,
	} {
		b.WriteString(`<style:style style:name="` + name + `" style:family="text"><style:text-properties ` + props + `/></style:style>`)
	}
	for name, value := range textColors {
		b.WriteString(`<style:style style:name="Text-` + name + `" style:family="text"><style:text-properties fo:color="#` + value + `"/></style:style>`)
	}
	for name, value := range markColors {
		b.WriteString(`<style:style style:name="Mark-` + name + `" style:family="text"><style:text-properties fo:background-color="#` + value + `"/></style:style>`)
		b.WriteString(`<style:style style:name="Cell-` + name + `" style:family="table-cell"><style:table-cell-properties fo:background-color="#` + value + `"/></style:style>`)
	}
	return b.String()
}

func (d *odtWriter) pack(w io.Writer) error {
	zw := zip.NewWriter(w)
	// The mimetype comes first and is not compressed: that is how a reader knows the file.
	head, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := io.WriteString(head, odtMime); err != nil {
		return err
	}
	manifest := `<manifest:file-entry manifest:full-path="/" manifest:media-type="` + odtMime + `"/>` +
		`<manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/>`
	for name, kind := range d.types {
		manifest += `<manifest:file-entry manifest:full-path="Pictures/` + name + `" manifest:media-type="` + kind + `"/>`
	}
	parts := []struct{ name, body string }{
		{"META-INF/manifest.xml", xmlHeader + `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2">` + manifest + `</manifest:manifest>`},
		{"content.xml", xmlHeader + `<office:document-content ` + odtNS + ` office:version="1.2"><office:automatic-styles>` + odtStyles() +
			`</office:automatic-styles><office:body><office:text>` + d.b.String() + `</office:text></office:body></office:document-content>`},
	}
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return err
		}
	}
	for name, data := range d.media {
		f, err := zw.Create("Pictures/" + name)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func odtParagraphStyle(n document.Node) string {
	switch attr(n, "align") {
	case "center":
		return "Center"
	case "end":
		return "End"
	}
	return "Body"
}

func (d *odtWriter) paragraph(style, inner string) {
	d.b.WriteString(`<text:p text:style-name="` + style + `">` + inner + `</text:p>`)
}

func (d *odtWriter) children(list []document.Node) {
	for _, n := range list {
		d.block(n)
	}
}

func (d *odtWriter) list(n document.Node) {
	d.b.WriteString(`<text:list>`)
	for _, item := range n.Content {
		d.b.WriteString(`<text:list-item>`)
		for i, c := range item.Content {
			if item.Type == document.TypeTaskItem && i == 0 && c.Type == document.TypeParagraph {
				box := "☐ "
				if done, _ := item.Attrs["checked"].(bool); done {
					box = "☑ "
				}
				d.paragraph("Body", escape(box)+d.spans(c))
				continue
			}
			d.block(c)
		}
		d.b.WriteString(`</text:list-item>`)
	}
	d.b.WriteString(`</text:list>`)
}

func (d *odtWriter) block(n document.Node) {
	switch n.Type {
	case document.TypeParagraph:
		d.paragraph(odtParagraphStyle(n), d.spans(n))
	case document.TypeHeading:
		level := headingLevel(n, d.depth)
		fmt.Fprintf(&d.b, `<text:h text:style-name="H%d" text:outline-level="%d">%s</text:h>`, level, level, d.spans(n))
	case document.TypeBulletList, document.TypeOrderedList, document.TypeTaskList:
		d.list(n)
	case document.TypeBlockquote:
		for _, c := range n.Content {
			d.paragraph("Quote", d.spans(c))
		}
	case document.TypeCodeBlock:
		for _, line := range strings.Split(inlineWords(n), "\n") {
			d.paragraph("Code", escape(line))
		}
	case document.TypeHorizontalRule:
		d.paragraph("Body", "")
	case document.TypeImage:
		d.image(n)
	case document.TypeFile:
		d.paragraph("Body", escape(attr(n, "filename")))
	case document.TypePanel:
		d.oneCell(n)
	case document.TypeExpand:
		d.paragraph("Body", `<text:span text:style-name="Bold">`+escape(attr(n, "title"))+`</text:span>`)
		d.children(n.Content)
	case document.TypeColumns:
		d.columns(n)
	case document.TypeDecision:
		d.paragraph("Body", d.spans(n))
	case document.TypeTable:
		caption, below := document.TableCaption(n)
		if caption != "" && !below {
			d.paragraph("Body", escape(caption))
		}
		d.table(n)
		if caption != "" && below {
			d.paragraph("Body", escape(caption))
		}
	default:
		if words := inlineWords(n); words != "" {
			d.paragraph("Body", escape(words))
		}
	}
}

func (d *odtWriter) oneCell(n document.Node) {
	d.b.WriteString(`<table:table><table:table-column/><table:table-row><table:table-cell>`)
	d.paragraph("Body", `<text:span text:style-name="Bold">`+escape(panelLabel(n))+`</text:span>`)
	d.children(n.Content)
	d.b.WriteString(`</table:table-cell></table:table-row></table:table>`)
}

func (d *odtWriter) columns(n document.Node) {
	d.b.WriteString(`<table:table><table:table-column table:number-columns-repeated="` + fmt.Sprint(len(n.Content)) + `"/><table:table-row>`)
	for _, column := range n.Content {
		d.b.WriteString(`<table:table-cell>`)
		d.children(column.Content)
		d.b.WriteString(`</table:table-cell>`)
	}
	d.b.WriteString(`</table:table-row></table:table>`)
}

func (d *odtWriter) table(n document.Node) {
	width := 0
	for _, row := range n.Content {
		cells := 0
		for _, c := range row.Content {
			cells += max(number(c, "colspan"), 1)
		}
		width = max(width, cells)
	}
	d.b.WriteString(`<table:table><table:table-column table:number-columns-repeated="` + fmt.Sprint(max(width, 1)) + `"/>`)
	for i, row := range n.Content {
		header := i == 0 && len(row.Content) > 0 && row.Content[0].Type == document.TypeTableHeader
		if header {
			d.b.WriteString(`<table:table-header-rows>`)
		}
		d.b.WriteString(`<table:table-row>`)
		for _, cell := range row.Content {
			d.b.WriteString(`<table:table-cell`)
			if span := number(cell, "colspan"); span > 1 {
				d.b.WriteString(fmt.Sprintf(` table:number-columns-spanned="%d"`, span))
			}
			if bg := attr(cell, "background"); markColors[bg] != "" {
				d.b.WriteString(` table:style-name="Cell-` + bg + `"`)
			}
			d.b.WriteString(`>`)
			d.children(cell.Content)
			d.b.WriteString(`</table:table-cell>`)
		}
		d.b.WriteString(`</table:table-row>`)
		if header {
			d.b.WriteString(`</table:table-header-rows>`)
		}
	}
	d.b.WriteString(`</table:table>`)
}

func (d *odtWriter) image(n document.Node) {
	id, ok := imageID(n)
	var img Image
	if ok && d.o.Image != nil {
		img, ok = d.o.Image(id)
	} else {
		ok = false
	}
	if !ok {
		if words := fallbackWords(n); words != "" {
			d.paragraph("Body", escape(words))
		}
		return
	}
	d.nextPic++
	name := fmt.Sprintf("image%d.%s", d.nextPic, img.Ext)
	d.media[name] = img.Data
	d.types[name] = "image/" + img.Ext
	frame := `<draw:frame svg:width="14cm" svg:height="8cm"><draw:image xlink:href="Pictures/` + name +
		`" xlink:type="simple"/><svg:desc>` + escape(attr(n, "alt")) + `</svg:desc></draw:frame>`
	d.paragraph("Body", frame)
	if caption := attr(n, "caption"); caption != "" {
		d.paragraph("Body", escape(caption))
	}
}

func spanStyles(n document.Node) (styles []string, href string) {
	for _, m := range n.Marks {
		color, _ := m.Attrs["color"].(string)
		switch m.Type {
		case document.MarkBold:
			styles = append(styles, "Bold")
		case document.MarkItalic:
			styles = append(styles, "Italic")
		case document.MarkUnderline:
			styles = append(styles, "Underline")
		case document.MarkStrike:
			styles = append(styles, "Strike")
		case document.MarkCode:
			styles = append(styles, "Mono")
		case document.MarkSubscript:
			styles = append(styles, "Sub")
		case document.MarkSuperscript:
			styles = append(styles, "Super")
		case document.MarkTextColor:
			if textColors[color] != "" {
				styles = append(styles, "Text-"+color)
			}
		case document.MarkHighlight:
			if markColors[color] != "" {
				styles = append(styles, "Mark-"+color)
			}
		case document.MarkLink:
			href, _ = m.Attrs["href"].(string)
		}
	}
	return styles, href
}

func (d *odtWriter) spans(n document.Node) string {
	var b strings.Builder
	for _, c := range n.Content {
		switch c.Type {
		case document.TypeText:
			styles, href := spanStyles(c)
			text := escape(c.Text)
			for _, style := range styles {
				text = `<text:span text:style-name="` + style + `">` + text + `</text:span>`
			}
			if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "mailto:") {
				text = `<text:a xlink:type="simple" xlink:href="` + escape(href) + `">` + text + `</text:a>`
			}
			b.WriteString(text)
		case document.TypeHardBreak:
			b.WriteString(`<text:line-break/>`)
		case document.TypeStatus:
			b.WriteString(`<text:span text:style-name="Bold">` + escape(attr(c, "text")) + `</text:span>`)
		case document.TypeDate:
			b.WriteString(escape(attr(c, "value")))
		case document.TypeMention:
			b.WriteString(escape(mentionWords(c)))
		default:
			b.WriteString(escape(inlineWords(c)))
		}
	}
	return b.String()
}
