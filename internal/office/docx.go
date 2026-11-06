// SPDX-License-Identifier: Apache-2.0

package office

import (
	"archive/zip"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/diffluence/diffluence/internal/document"
)

const (
	wordNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
		`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"`
	xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	relBase   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"

	bulletNumbering  = 1
	orderedNumbering = 2
	imageWidthEMU    = 5_000_000
	imageHeightEMU   = 3_000_000
	panelShade       = "F5F7FA"
)

type wordWriter struct {
	b       strings.Builder
	o       Options
	depth   int
	rels    []string
	media   map[string][]byte
	exts    map[string]bool
	nextRel int
	nextPic int
}

// WriteDOCX writes pages as one Word file.
func WriteDOCX(w io.Writer, sections []Section, o Options) error {
	d := &wordWriter{o: o, media: map[string][]byte{}, exts: map[string]bool{}, nextRel: 10}
	d.b.WriteString(xmlHeader + `<w:document ` + wordNS + `><w:body>`)
	for _, s := range sections {
		level := min(s.Depth+1, maxHeadingLevel)
		d.paragraph("Heading"+strconv.Itoa(level), `<w:r><w:t xml:space="preserve">`+escape(s.Title)+`</w:t></w:r>`, "")
		d.depth = s.Depth + 1
		d.blocks(s.Document.Content, 0)
	}
	d.b.WriteString(`<w:sectPr/></w:body></w:document>`)
	return d.pack(w)
}

func (d *wordWriter) pack(w io.Writer) error {
	zw := zip.NewWriter(w)
	types := `<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>`
	for ext := range d.exts {
		types += `<Default Extension="` + ext + `" ContentType="image/` + ext + `"/>`
	}
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` + types +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
			`<Override PartName="/word/numbering.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"/></Types>`},
		{"_rels/.rels", xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relBase + `/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/_rels/document.xml.rels", xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + relBase + `/styles" Target="styles.xml"/>` +
			`<Relationship Id="rId2" Type="` + relBase + `/numbering" Target="numbering.xml"/>` +
			strings.Join(d.rels, "") + `</Relationships>`},
		{"word/document.xml", d.b.String()},
		{"word/styles.xml", wordStyles()},
		{"word/numbering.xml", wordNumbering()},
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
		f, err := zw.Create("word/media/" + name)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func wordStyles() string {
	var b strings.Builder
	b.WriteString(xmlHeader + `<w:styles ` + wordNS + `><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>`)
	sizes := []int{40, 32, 28, 24, 22, 20}
	for i, size := range sizes {
		level := strconv.Itoa(i + 1)
		fmt.Fprintf(&b, `<w:style w:type="paragraph" w:styleId="Heading%s"><w:name w:val="heading %s"/><w:basedOn w:val="Normal"/>`+
			`<w:pPr><w:keepNext/><w:outlineLvl w:val="%d"/></w:pPr><w:rPr><w:b/><w:sz w:val="%d"/></w:rPr></w:style>`, level, level, i, size)
	}
	b.WriteString(`<w:style w:type="paragraph" w:styleId="Quote"><w:name w:val="Quote"/><w:basedOn w:val="Normal"/><w:pPr><w:ind w:left="400"/></w:pPr><w:rPr><w:i/></w:rPr></w:style>`)
	b.WriteString(`<w:style w:type="paragraph" w:styleId="Code"><w:name w:val="Code"/><w:basedOn w:val="Normal"/><w:rPr><w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/></w:rPr></w:style>`)
	b.WriteString(`</w:styles>`)
	return b.String()
}

func wordNumbering() string {
	return xmlHeader + `<w:numbering ` + wordNS + `>` +
		`<w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/><w:lvlText w:val="•"/></w:lvl></w:abstractNum>` +
		`<w:abstractNum w:abstractNumId="2"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl></w:abstractNum>` +
		`<w:num w:numId="1"><w:abstractNumId w:val="1"/></w:num><w:num w:numId="2"><w:abstractNumId w:val="2"/></w:num></w:numbering>`
}

func (d *wordWriter) paragraph(style, runs, extra string) {
	d.b.WriteString(`<w:p><w:pPr>`)
	if style != "" {
		d.b.WriteString(`<w:pStyle w:val="` + style + `"/>`)
	}
	d.b.WriteString(extra)
	d.b.WriteString(`</w:pPr>` + runs + `</w:p>`)
}

func alignment(n document.Node) string {
	switch attr(n, "align") {
	case "center":
		return `<w:jc w:val="center"/>`
	case "end":
		return `<w:jc w:val="end"/>`
	}
	return ""
}

func (d *wordWriter) blocks(list []document.Node, listLevel int) {
	for _, n := range list {
		d.block(n, listLevel)
	}
}

func (d *wordWriter) list(n document.Node, numbering, level int) {
	for _, item := range n.Content {
		for i, c := range item.Content {
			if c.Type == document.TypeParagraph && i == 0 {
				prefix := ""
				if item.Type == document.TypeTaskItem {
					prefix = "☐ "
					if done, _ := item.Attrs["checked"].(bool); done {
						prefix = "☑ "
					}
				}
				extra := fmt.Sprintf(`<w:numPr><w:ilvl w:val="%d"/><w:numId w:val="%d"/></w:numPr>`, level, numbering)
				if item.Type == document.TypeTaskItem {
					extra = ""
				}
				d.paragraph("", textRun(prefix, "")+d.runs(c), extra)
				continue
			}
			d.block(c, level+1)
		}
	}
}

func (d *wordWriter) block(n document.Node, level int) {
	switch n.Type {
	case document.TypeParagraph:
		d.paragraph("", d.runs(n), alignment(n))
	case document.TypeHeading:
		d.paragraph("Heading"+strconv.Itoa(headingLevel(n, d.depth)), d.runs(n), alignment(n))
	case document.TypeBulletList:
		d.list(n, bulletNumbering, level)
	case document.TypeOrderedList:
		d.list(n, orderedNumbering, level)
	case document.TypeTaskList:
		d.list(n, bulletNumbering, level)
	case document.TypeBlockquote:
		for _, c := range n.Content {
			d.paragraph("Quote", d.runs(c), "")
		}
	case document.TypeCodeBlock:
		if title := attr(n, "title"); title != "" {
			d.paragraph("", textRun(title, "<w:b/>"), "")
		}
		for _, line := range strings.Split(inlineWords(n), "\n") {
			d.paragraph("Code", textRun(line, ""), "")
		}
	case document.TypeHorizontalRule:
		d.paragraph("", "", `<w:pBdr><w:bottom w:val="single" w:sz="6" w:space="1" w:color="DDE2EA"/></w:pBdr>`)
	case document.TypeImage:
		d.image(n)
	case document.TypeFile:
		d.paragraph("", textRun(attr(n, "filename"), ""), "")
	case document.TypePanel:
		d.oneCell(n, panelLabel(n), panelShade)
	case document.TypeExpand:
		d.paragraph("", textRun(attr(n, "title"), "<w:b/>"), "")
		d.blocks(n.Content, level)
	case document.TypeColumns:
		d.columns(n)
	case document.TypeDecision:
		d.paragraph("", d.runs(n), `<w:pBdr><w:left w:val="single" w:sz="18" w:space="6" w:color="1B7A3D"/></w:pBdr>`)
	case document.TypeTable:
		caption, below := document.TableCaption(n)
		if caption != "" && !below {
			d.paragraph("", textRun(caption, "<w:i/>"), "")
		}
		d.table(n)
		if caption != "" && below {
			d.paragraph("", textRun(caption, "<w:i/>"), "")
		}
	default:
		if words := inlineWords(n); words != "" {
			d.paragraph("", textRun(words, ""), "")
		}
	}
}

func (d *wordWriter) oneCell(n document.Node, label, shade string) {
	d.b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/></w:tblPr><w:tr><w:tc><w:tcPr><w:shd w:val="clear" w:fill="` + shade + `"/></w:tcPr>`)
	d.paragraph("", textRun(label, "<w:b/>"), "")
	d.blocks(n.Content, 0)
	d.b.WriteString(`</w:tc></w:tr></w:tbl>`)
	d.paragraph("", "", "")
}

func (d *wordWriter) columns(n document.Node) {
	d.b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/></w:tblPr><w:tr>`)
	for _, column := range n.Content {
		d.b.WriteString(`<w:tc>`)
		d.blocks(column.Content, 0)
		d.b.WriteString(`<w:p/></w:tc>`)
	}
	d.b.WriteString(`</w:tr></w:tbl>`)
	d.paragraph("", "", "")
}

func (d *wordWriter) table(n document.Node) {
	d.b.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="5000" w:type="pct"/><w:tblBorders>`)
	for _, side := range []string{"top", "left", "bottom", "right", "insideH", "insideV"} {
		d.b.WriteString(`<w:` + side + ` w:val="single" w:sz="4" w:color="DDE2EA"/>`)
	}
	d.b.WriteString(`</w:tblBorders></w:tblPr>`)
	for _, row := range n.Content {
		d.b.WriteString(`<w:tr>`)
		if len(row.Content) > 0 && row.Content[0].Type == document.TypeTableHeader {
			// Word repeats this row on every page, and reads it as the header.
			d.b.WriteString(`<w:trPr><w:tblHeader/></w:trPr>`)
		}
		for _, cell := range row.Content {
			d.b.WriteString(`<w:tc><w:tcPr>`)
			if span := number(cell, "colspan"); span > 1 {
				d.b.WriteString(`<w:gridSpan w:val="` + strconv.Itoa(span) + `"/>`)
			}
			if fill := markColors[attr(cell, "background")]; fill != "" {
				d.b.WriteString(`<w:shd w:val="clear" w:fill="` + fill + `"/>`)
			}
			d.b.WriteString(`</w:tcPr>`)
			d.blocks(cell.Content, 0)
			if len(cell.Content) == 0 {
				d.b.WriteString(`<w:p/>`)
			}
			d.b.WriteString(`</w:tc>`)
		}
		d.b.WriteString(`</w:tr>`)
	}
	d.b.WriteString(`</w:tbl>`)
	d.paragraph("", "", "")
}

func (d *wordWriter) image(n document.Node) {
	id, ok := imageID(n)
	var img Image
	if ok && d.o.Image != nil {
		img, ok = d.o.Image(id)
	} else {
		ok = false
	}
	if !ok {
		if words := fallbackWords(n); words != "" {
			d.paragraph("", textRun(words, ""), "")
		}
		return
	}
	d.nextPic++
	d.nextRel++
	name := fmt.Sprintf("image%d.%s", d.nextPic, img.Ext)
	rel := fmt.Sprintf("rId%d", d.nextRel)
	d.media[name] = img.Data
	d.exts[img.Ext] = true
	d.rels = append(d.rels, `<Relationship Id="`+rel+`" Type="`+relBase+`/image" Target="media/`+name+`"/>`)
	alt := escape(attr(n, "alt"))
	pic := fmt.Sprintf(`<w:r><w:drawing><wp:inline><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="%s" descr="%s"/>`+
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic>`+
		`<pic:nvPicPr><pic:cNvPr id="%d" name="%s" descr="%s"/><pic:cNvPicPr/></pic:nvPicPr>`+
		`<pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>`+
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>`+
		`</pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`,
		imageWidthEMU, imageHeightEMU, d.nextPic, name, alt, d.nextPic, name, alt, rel, imageWidthEMU, imageHeightEMU)
	d.paragraph("", pic, "")
	if caption := attr(n, "caption"); caption != "" {
		d.paragraph("", textRun(caption, "<w:i/>"), "")
	}
}

func textRun(text, props string) string {
	if text == "" {
		return ""
	}
	return `<w:r><w:rPr>` + props + `</w:rPr><w:t xml:space="preserve">` + escape(text) + `</w:t></w:r>`
}

func markProps(n document.Node) (props, href string) {
	var b strings.Builder
	for _, m := range n.Marks {
		switch m.Type {
		case document.MarkBold:
			b.WriteString(`<w:b/>`)
		case document.MarkItalic:
			b.WriteString(`<w:i/>`)
		case document.MarkUnderline:
			b.WriteString(`<w:u w:val="single"/>`)
		case document.MarkStrike:
			b.WriteString(`<w:strike/>`)
		case document.MarkCode:
			b.WriteString(`<w:rFonts w:ascii="Consolas" w:hAnsi="Consolas"/>`)
		case document.MarkSubscript:
			b.WriteString(`<w:vertAlign w:val="subscript"/>`)
		case document.MarkSuperscript:
			b.WriteString(`<w:vertAlign w:val="superscript"/>`)
		case document.MarkTextColor:
			color, _ := m.Attrs["color"].(string)
			if v := textColors[color]; v != "" {
				b.WriteString(`<w:color w:val="` + v + `"/>`)
			}
		case document.MarkHighlight:
			color, _ := m.Attrs["color"].(string)
			if v := markColors[color]; v != "" {
				b.WriteString(`<w:shd w:val="clear" w:fill="` + v + `"/>`)
			}
		case document.MarkLink:
			href, _ = m.Attrs["href"].(string)
		}
	}
	return b.String(), href
}

func (d *wordWriter) runs(n document.Node) string {
	var b strings.Builder
	for _, c := range n.Content {
		switch c.Type {
		case document.TypeText:
			props, href := markProps(c)
			run := textRun(c.Text, props)
			// Only addresses that mean something outside the instance stay links.
			if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "mailto:") {
				d.nextRel++
				rel := fmt.Sprintf("rId%d", d.nextRel)
				d.rels = append(d.rels, `<Relationship Id="`+rel+`" Type="`+relBase+`/hyperlink" Target="`+escape(href)+`" TargetMode="External"/>`)
				run = `<w:hyperlink r:id="` + rel + `">` + run + `</w:hyperlink>`
			}
			b.WriteString(run)
		case document.TypeHardBreak:
			b.WriteString(`<w:r><w:br/></w:r>`)
		case document.TypeStatus:
			b.WriteString(textRun(attr(c, "text"), "<w:b/>"))
		case document.TypeDate:
			b.WriteString(textRun(attr(c, "value"), ""))
		case document.TypeMention:
			b.WriteString(textRun(mentionWords(c), ""))
		default:
			b.WriteString(textRun(inlineWords(c), ""))
		}
	}
	return b.String()
}
