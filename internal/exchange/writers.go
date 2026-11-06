// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/office"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	maxInlineImageBytes = 50 * mebibyte
	styleFile           = "style.css"
	indexFile           = "index.html"
	missingFile         = "missing.txt"
	filesFolder         = "files"
)

// imageExtensions are the images a Word, ODT or PDF file can hold.
var imageExtensions = map[string]string{".png": "png", ".jpg": "jpeg", ".jpeg": "jpeg", ".gif": "gif"}

// exportWriter turns prepared pages into the file of one run.
type exportWriter struct {
	s      *Service
	c      identity.Caller
	run    ExportRun
	items  []exportItem
	report *ExportReport
	title  string
	out    io.Writer
}

func (w *exportWriter) write(ctx context.Context) error {
	switch w.run.Format {
	case FormatText:
		return w.text(ctx)
	case FormatMarkdown:
		return w.markdown(ctx)
	case FormatHTML:
		return w.html(ctx)
	case FormatWord:
		return office.WriteDOCX(w.out, w.sections(ctx), w.officeOptions(ctx))
	case FormatODT:
		return office.WriteODT(w.out, w.sections(ctx), w.officeOptions(ctx))
	case FormatPDF:
		return w.pdf(ctx)
	}
	return apperr.Invalid("Choose a format.")
}

// comment is a comment as it appears in a file.
type comment struct {
	author string
	when   time.Time
	body   document.Node
}

// commentsOf gives what the person could read on the page itself.
func (w *exportWriter) commentsOf(ctx context.Context, id uuid.UUID) ([]comment, error) {
	if !w.run.Options.Comments {
		return nil, nil
	}
	mayEdit, err := w.s.perms.CheckPage(ctx, w.s.pool, w.c.UserID, id, permissions.PageEdit, uuid.Nil)
	if err != nil {
		return nil, err
	}
	rows, err := w.s.pool.Query(ctx, `SELECT COALESCE(u.display_name, ''), c.created_at, c.body FROM comments c
		LEFT JOIN users u ON u.id = c.author_id
		WHERE c.page_id = $1 AND c.trash_item_id IS NULL AND NOT c.purged AND (NOT c.editors_only OR $2)
		ORDER BY c.created_at`, id, mayEdit.Allowed)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (comment, error) {
		var out comment
		var body []byte
		if err := r.Scan(&out.author, &out.when, &body); err != nil {
			return out, err
		}
		parsed, err := document.Parse(body)
		out.body = parsed
		return out, err
	})
}

func (w *exportWriter) sections(ctx context.Context) []office.Section {
	out := make([]office.Section, 0, len(w.items)+1)
	for _, item := range w.items {
		out = append(out, office.Section{Title: item.page.Title, Depth: item.depth, Document: w.withComments(ctx, item)})
	}
	if note := missingNote(w.report); note != "" {
		out = append(out, office.Section{Title: "Missing pages", Document: textDocument(note)})
	}
	return out
}

func paragraph(text string) document.Node {
	return document.Node{Type: document.TypeParagraph, Content: []document.Node{{Type: document.TypeText, Text: text}}}
}

func textDocument(text string) document.Node {
	doc := document.Node{Type: document.TypeDoc}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line != "" {
			doc.Content = append(doc.Content, paragraph(line))
		}
	}
	return doc
}

// withComments adds the comments after the page, as a section of the same document.
func (w *exportWriter) withComments(ctx context.Context, item exportItem) document.Node {
	list, err := w.commentsOf(ctx, item.page.ID)
	if err != nil {
		w.report.Failed = append(w.report.Failed, item.page.Title)
		return item.doc
	}
	if len(list) == 0 {
		return item.doc
	}
	doc := item.doc
	doc.Content = append(append([]document.Node{}, doc.Content...), document.Node{
		Type: document.TypeHeading, Attrs: map[string]any{"level": 2},
		Content: []document.Node{{Type: document.TypeText, Text: "Comments"}}})
	for _, one := range list {
		doc.Content = append(doc.Content, paragraph(one.author+" · "+one.when.Format(time.DateOnly)))
		doc.Content = append(doc.Content, one.body.Content...)
	}
	return doc
}

func (w *exportWriter) officeOptions(ctx context.Context) office.Options {
	byID := map[uuid.UUID]attachments.Attachment{}
	for _, item := range w.items {
		for id, a := range item.files {
			byID[id] = a
		}
	}
	return office.Options{Image: func(id uuid.UUID) (office.Image, bool) {
		a, ok := byID[id]
		if !ok {
			return office.Image{}, false
		}
		ext, ok := imageExtensions[strings.ToLower(path.Ext(a.Filename))]
		if !ok {
			return office.Image{}, false
		}
		data, err := w.readFile(ctx, a)
		if err != nil {
			return office.Image{}, false
		}
		return office.Image{Data: data, Ext: ext}, true
	}}
}

func (w *exportWriter) readFile(ctx context.Context, a attachments.Attachment) ([]byte, error) {
	src, err := w.s.attachments.OpenUnchecked(ctx, a)
	if err != nil {
		return nil, err
	}
	defer func() { _ = src.Close() }()
	data, err := io.ReadAll(io.LimitReader(src, maxInlineImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxInlineImageBytes {
		return nil, apperr.TooLarge("An image is too large to place in the file.")
	}
	return data, nil
}

func addMissing(zw *zip.Writer, report *ExportReport) error {
	note := missingNote(report)
	if note == "" {
		return nil
	}
	f, err := zw.Create(missingFile)
	if err != nil {
		return err
	}
	_, err = io.WriteString(f, note)
	return err
}

func (w *exportWriter) plainText(ctx context.Context, item exportItem) string {
	return item.page.Title + "\n\n" + document.Text(w.withComments(ctx, item)) + "\n"
}

func (w *exportWriter) text(ctx context.Context) error {
	if len(w.items) == 1 && !w.run.Options.Attachments {
		_, err := io.WriteString(w.out, w.plainText(ctx, w.items[0]))
		return err
	}
	zw := zip.NewWriter(w.out)
	for _, item := range w.items {
		f, err := zw.Create(item.name + ".txt")
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, w.plainText(ctx, item)); err != nil {
			return err
		}
		if w.run.Options.Attachments {
			for _, a := range item.files {
				if err := w.s.addFile(ctx, zw, path.Join(item.name+".files", a.Filename), a); err != nil {
					return err
				}
			}
		}
	}
	if err := addMissing(zw, w.report); err != nil {
		return err
	}
	return zw.Close()
}

func (w *exportWriter) markdown(ctx context.Context) error {
	zw := zip.NewWriter(w.out)
	if err := w.s.writeMarkdownTree(ctx, zw, exportPagesOf(w.items)); err != nil {
		return err
	}
	if err := addMissing(zw, w.report); err != nil {
		return err
	}
	return zw.Close()
}

// htmlFile gives the address of a file in the export, from one folder down or not.
func htmlFile(name string) string { return url.PathEscape(name) + ".html" }

func (w *exportWriter) navItems(current uuid.UUID) []document.NavItem {
	nodes := map[uuid.UUID]*document.NavItem{}
	children := map[uuid.UUID][]uuid.UUID{}
	var top []uuid.UUID
	for _, item := range w.items {
		nodes[item.page.ID] = &document.NavItem{Title: item.page.Title, Href: htmlFile(item.name), Current: item.page.ID == current}
	}
	for _, item := range w.items {
		if item.page.ParentID != nil && nodes[*item.page.ParentID] != nil {
			children[*item.page.ParentID] = append(children[*item.page.ParentID], item.page.ID)
			continue
		}
		top = append(top, item.page.ID)
	}
	var build func(ids []uuid.UUID) []document.NavItem
	build = func(ids []uuid.UUID) []document.NavItem {
		out := make([]document.NavItem, 0, len(ids))
		for _, id := range ids {
			n := *nodes[id]
			n.Children = build(children[id])
			out = append(out, n)
		}
		return out
	}
	return build(top)
}

func (w *exportWriter) html(ctx context.Context) error {
	zw := zip.NewWriter(w.out)
	if err := addText(zw, styleFile, document.ExportStyle); err != nil {
		return err
	}
	links := map[uuid.UUID]string{}
	for _, item := range w.items {
		links[item.page.ID] = htmlFile(item.name)
	}
	if err := addText(zw, indexFile, w.shell(w.title, "", document.NavHTML(w.navItems(uuid.Nil)))); err != nil {
		return err
	}
	for _, item := range w.items {
		if err := w.htmlPage(ctx, zw, item, links); err != nil {
			return err
		}
	}
	if err := addMissing(zw, w.report); err != nil {
		return err
	}
	return zw.Close()
}

func addText(zw *zip.Writer, name, body string) error {
	f, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(f, body)
	return err
}

func (w *exportWriter) shell(title, body, nav string) string {
	return document.Shell{Title: title, Site: w.title, StyleHref: styleFile, NavLabel: "Pages", SkipLabel: "Skip to the content",
		Nav: nav, Body: body}.HTML()
}

func (w *exportWriter) htmlPage(ctx context.Context, zw *zip.Writer, item exportItem, links map[uuid.UUID]string) error {
	stored := map[uuid.UUID]string{}
	for id, a := range item.files {
		name := path.Join(filesFolder, item.name, a.Filename)
		wanted := document.AttachmentIDs(item.doc)
		if !slices.Contains(wanted, id) && !w.run.Options.Attachments {
			continue
		}
		if err := w.s.addFile(ctx, zw, name, a); err != nil {
			return err
		}
		stored[id] = filesFolder + "/" + url.PathEscape(item.name) + "/" + url.PathEscape(a.Filename)
	}
	localFile := func(id uuid.UUID) (string, bool) {
		href, ok := stored[id]
		return href, ok
	}
	options := document.HTMLOptions{ImageSrc: localFile, FileHref: localFile, PageHref: func(id uuid.UUID) (string, bool) {
		href, ok := links[id]
		if !ok {
			w.report.OutsideSet++
		}
		return href, ok
	}}
	body := document.ToHTML(w.withComments(ctx, item), options)
	if w.run.Options.Versions {
		older, err := w.htmlVersions(ctx, zw, item, options)
		if err != nil {
			return err
		}
		body += older
	}
	return addText(zw, htmlFileName(item.name), w.shell(item.page.Title, body, document.NavHTML(w.navItems(item.page.ID))))
}

func htmlFileName(name string) string { return name + ".html" }

// htmlVersions writes each old version as a page, and returns the links to them.
func (w *exportWriter) htmlVersions(ctx context.Context, zw *zip.Writer, item exportItem, options document.HTMLOptions) (string, error) {
	rows, err := w.s.pool.Query(ctx, `SELECT number, document FROM page_versions
		WHERE page_id = $1 AND number < (SELECT current_version FROM pages WHERE id = $1) ORDER BY number DESC`, item.page.ID)
	if err != nil {
		return "", err
	}
	type version struct {
		number int
		raw    []byte
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (version, error) {
		var v version
		err := r.Scan(&v.number, &v.raw)
		return v, err
	})
	if err != nil || len(list) == 0 {
		return "", err
	}
	var links strings.Builder
	links.WriteString("<h2>Old versions</h2><ul>")
	for _, v := range list {
		doc, err := document.Parse(v.raw)
		if err != nil {
			continue
		}
		name := fmt.Sprintf("%s.v%d", item.name, v.number)
		title := fmt.Sprintf("%s, version %d", item.page.Title, v.number)
		if err := addText(zw, htmlFileName(name), w.shell(title, document.ToHTML(doc, options), "")); err != nil {
			return "", err
		}
		fmt.Fprintf(&links, `<li><a href="%s">Version %d</a></li>`, htmlFile(name), v.number)
	}
	links.WriteString("</ul>")
	return links.String(), nil
}
