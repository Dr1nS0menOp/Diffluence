// SPDX-License-Identifier: Apache-2.0

package exchange

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/office"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
)

// converted is a file turned into a document.
type converted struct {
	doc      document.Node
	title    string
	labels   []string
	stage    string
	warnings []string
}

// planPage is one page to make.
type planPage struct {
	key     string // the path of its file
	title   string
	convert func(ctx context.Context, m *importer, pageID uuid.UUID, key string, data []byte) (converted, error)
	limit   int64
	folder  bool // this file stands for its folder, as index.html does
}

// loggedItem is a page an import made, for Undo.
type loggedItem struct {
	page    uuid.UUID
	source  string
	version int
}

type itemLog struct{ list []loggedItem }

func (l *itemLog) add(item loggedItem) {
	for i := range l.list {
		if l.list[i].page == item.page {
			l.list[i].version = item.version
			return
		}
	}
	l.list = append(l.list, item)
}

// liveSink carries out a plan through the public methods of pages and attachments.
type liveSink struct {
	s       *Service
	c       identity.Caller
	spaceID uuid.UUID
}

func (l *liveSink) create(ctx context.Context, parent *uuid.UUID, title string) (uuid.UUID, error) {
	p, err := l.s.pages.Create(ctx, l.c, pages.NewPage{SpaceID: l.spaceID, ParentID: parent, Title: title})
	return p.ID, err
}

func (l *liveSink) publish(ctx context.Context, id uuid.UUID, title string, doc document.Node, base int) (int, error) {
	raw, err := document.Marshal(doc)
	if err != nil {
		return 0, err
	}
	p, err := l.s.pages.Publish(ctx, l.c, id, pages.PublishInput{Title: title, Document: raw, BaseVersion: base, Silent: base > 0})
	return p.CurrentVersion, err
}

func (l *liveSink) upload(ctx context.Context, pageID uuid.UUID, name string, data []byte) (uuid.UUID, error) {
	a, err := l.s.attachments.Upload(ctx, l.c, pageID, name, bytes.NewReader(data))
	return a.ID, err
}

func (l *liveSink) label(ctx context.Context, id uuid.UUID, label string) error {
	_, err := l.s.pages.AddLabel(ctx, l.c, id, label)
	return err
}

func (l *liveSink) stage(ctx context.Context, id uuid.UUID, text string) error {
	_, err := l.s.pages.SetStage(ctx, l.c, id, text, importedStageColor)
	return err
}

func (l *liveSink) closeToImporter(ctx context.Context, id uuid.UUID) error {
	return l.s.perms.SetRestrictions(ctx, l.c, id, "view", []permissions.Subject{{Type: permissions.SubjectUser, ID: l.c.UserID}})
}

// dryRun answers like the real thing and changes nothing.
type dryRun struct{ result *ImportResult }

func (d *dryRun) create(context.Context, *uuid.UUID, string) (uuid.UUID, error) {
	return ids.New(), nil
}
func (d *dryRun) publish(_ context.Context, _ uuid.UUID, _ string, doc document.Node, base int) (int, error) {
	raw, err := document.Marshal(doc)
	if err != nil {
		return 0, err
	}
	if _, err := document.Normalize(raw); err != nil {
		return 0, err
	}
	return base + 1, nil
}
func (d *dryRun) upload(context.Context, uuid.UUID, string, []byte) (uuid.UUID, error) {
	return ids.New(), nil
}
func (d *dryRun) label(context.Context, uuid.UUID, string) error   { return nil }
func (d *dryRun) stage(context.Context, uuid.UUID, string) error   { return nil }
func (d *dryRun) closeToImporter(context.Context, uuid.UUID) error { return nil }

type importer struct {
	s        *Service
	out      sink
	root     *uuid.UUID
	result   *ImportResult
	items    *itemLog
	byName   map[string]entry
	folders  map[string]*uuid.UUID // folder path -> page its children go under
	byKey    map[string]uuid.UUID  // file -> page
	byTitle  map[string]uuid.UUID  // title -> page, for wiki links
	attached map[string]uuid.UUID  // page and file -> attachment, so a second pass finds it
	depths   map[uuid.UUID]int
	unpacked int64
	limit    int64
	pending  bool // the page being read links to a page that does not exist yet
	warnings map[string][]string
}

func (m *importer) warn(format string, args ...any) {
	if len(m.result.Warnings) < maxWarnings {
		m.result.Warnings = append(m.result.Warnings, fmt.Sprintf(format, args...))
	}
}

// read returns a file's content, counting it against the unpacked limit.
func (m *importer) read(e entry, limit int64) ([]byte, error) {
	rc, err := e.open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, apperr.TooLarge("A file in the archive is larger than allowed.")
	}
	m.unpacked += int64(len(b))
	if m.unpacked > m.limit {
		return nil, apperr.TooLarge("The archive unpacks to more than the limit.")
	}
	return b, nil
}

func stem(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}

func cutTitle(title string) string {
	if r := []rune(title); len(r) > maxTitleRunes {
		return string(r[:maxTitleRunes])
	}
	return title
}

// plan reads a run's files and hands the pages to a sink.
func (s *Service) plan(ctx context.Context, c identity.Caller, run ImportRun, out sink, result *ImportResult, items *itemLog) error {
	maxUnpacked, err := s.maxUnpacked(ctx)
	if err != nil {
		return err
	}
	entries, done, err := s.openEntries(ctx, run, result, maxUnpacked)
	defer done()
	if err != nil {
		return err
	}
	if items == nil {
		items = &itemLog{}
	}
	m := &importer{s: s, out: out, root: run.ParentID, result: result, items: items, limit: maxUnpacked,
		byName: map[string]entry{}, folders: map[string]*uuid.UUID{"": run.ParentID}, byKey: map[string]uuid.UUID{},
		byTitle: map[string]uuid.UUID{}, attached: map[string]uuid.UUID{}, depths: map[uuid.UUID]int{}, warnings: map[string][]string{}}
	for _, e := range entries {
		m.byName[e.name] = e
	}
	list, err := m.pagesOf(entries)
	if err != nil {
		return err
	}
	result.Kind = kindOfPages(entries)
	return m.run(ctx, list)
}

func kindOfPages(entries []entry) string {
	for _, e := range entries {
		switch {
		case isAny(e.name, ".md", ".markdown"):
			return KindMarkdown
		case isAny(e.name, ".docx"):
			return KindWord
		case isAny(e.name, ".html", ".htm"):
			return KindHTML
		}
	}
	return ""
}

// pagesOf decides what each file becomes. Files of the first kind found are pages,
// and the others can still be attachments of those pages.
func (m *importer) pagesOf(entries []entry) ([]planPage, error) {
	var list []planPage
	kind := kindOfPages(entries)
	for _, e := range entries {
		switch {
		case isAny(e.name, ".doc"):
			if len(entries) == 1 {
				return nil, apperr.Invalid("This is an old Word file. Save it as .docx first.")
			}
			m.warn("%s: an old Word file. Save it as .docx first.", e.name)
		case kind == KindMarkdown && isAny(e.name, ".md", ".markdown"):
			list = append(list, planPage{key: e.name, title: stem(e.name), convert: convertMarkdown, limit: MaxMarkdownBytes})
		case kind == KindWord && isAny(e.name, ".docx"):
			list = append(list, planPage{key: e.name, title: stem(e.name), convert: convertWord, limit: maxWordBytes})
		case kind == KindHTML && isAny(e.name, ".html", ".htm"):
			list = append(list, planPage{key: e.name, title: stem(e.name), convert: convertHTML, limit: MaxMarkdownBytes,
				folder: strings.EqualFold(path.Base(e.name), "index.html")})
		}
	}
	if len(list) == 0 {
		return nil, apperr.Invalid("The upload has no Markdown files.")
	}
	sortShallowFirst(list)
	return list, nil
}

func (m *importer) run(ctx context.Context, list []planPage) error {
	if len(list) > 1 && m.result.Kind == KindWord {
		if err := m.holdingPage(ctx); err != nil {
			return err
		}
	}
	var again []planPage
	for _, e := range list {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.pending = false
		if err := m.page(ctx, e); err != nil {
			return err
		}
		if m.pending {
			again = append(again, e)
		}
	}
	for _, e := range again {
		if err := m.relink(ctx, e); err != nil {
			return err
		}
	}
	for _, e := range list {
		for _, w := range m.warnings[e.key] {
			m.warn("%s: %s", e.key, w)
		}
	}
	return nil
}

// holdingPage keeps several Word files out of sight, so they can be reviewed first (G10).
func (m *importer) holdingPage(ctx context.Context) error {
	id, err := m.publishNew(ctx, m.root, holdingPage, document.Empty(), "")
	if err != nil {
		return err
	}
	if err := m.out.closeToImporter(ctx, id); err != nil {
		return err
	}
	m.folders[""] = &id
	return nil
}

// parentFor returns the page that holds a folder, creating empty pages for
// folders that have no file of their own.
func (m *importer) parentFor(ctx context.Context, folder string) (*uuid.UUID, error) {
	if folder == "." {
		folder = ""
	}
	if p, ok := m.folders[folder]; ok {
		return p, nil
	}
	parent, err := m.parentFor(ctx, path.Dir(folder))
	if err != nil {
		return nil, err
	}
	id, err := m.publishNew(ctx, parent, path.Base(folder), document.Empty(), "")
	if err != nil {
		return nil, err
	}
	m.folders[folder] = &id
	return &id, nil
}

func (m *importer) note(parent *uuid.UUID, id uuid.UUID, title string) {
	depth := 0
	if parent != nil {
		if d, ok := m.depths[*parent]; ok {
			depth = d + 1
		}
	}
	m.depths[id] = depth
	m.result.Pages++
	m.result.Tree = append(m.result.Tree, ReportPage{Title: title, Depth: depth})
}

func (m *importer) publishNew(ctx context.Context, parent *uuid.UUID, title string, doc document.Node, source string) (uuid.UUID, error) {
	id, err := m.out.create(ctx, parent, title)
	if err != nil {
		return uuid.Nil, err
	}
	version, err := m.out.publish(ctx, id, title, doc, 0)
	if err != nil {
		return uuid.Nil, err
	}
	m.note(parent, id, title)
	m.items.add(loggedItem{page: id, source: source, version: version})
	return id, nil
}

func (m *importer) page(ctx context.Context, e planPage) error {
	data, err := m.read(m.byName[e.key], e.limit)
	if err != nil {
		return err
	}
	folder := path.Dir(e.key)
	if folder == "." {
		folder = ""
	}
	inFolder := folder
	if e.folder && folder != "" {
		inFolder = path.Dir(folder)
	}
	parent, err := m.parentFor(ctx, inFolder)
	if err != nil {
		return err
	}
	title := cutTitle(e.title)
	id, err := m.out.create(ctx, parent, title)
	if err != nil {
		return err
	}
	m.byKey[e.key] = id
	m.items.add(loggedItem{page: id, source: e.key})
	got, err := e.convert(ctx, m, id, e.key, data)
	if err != nil {
		return fmt.Errorf("%s: %w", truncate(e.key, maxReportedName), err)
	}
	m.warnings[e.key] = got.warnings
	if got.title != "" {
		title = cutTitle(got.title)
	}
	m.byTitle[strings.ToLower(title)] = id
	version, err := m.out.publish(ctx, id, title, got.doc, 0)
	if err != nil {
		return err
	}
	m.note(parent, id, title)
	m.items.add(loggedItem{page: id, source: e.key, version: version})
	for _, label := range got.labels {
		if err := m.out.label(ctx, id, label); err != nil {
			m.warnings[e.key] = append(m.warnings[e.key], fmt.Sprintf("The label %q could not be added.", truncate(label, maxReportedName)))
		}
	}
	if got.stage != "" {
		if err := m.out.stage(ctx, id, got.stage); err != nil {
			m.warnings[e.key] = append(m.warnings[e.key], fmt.Sprintf("The status %q could not be set.", truncate(got.stage, maxReportedName)))
		}
	}
	// Children of "Title.md" live in the folder "Title", and of "a/index.html" in "a".
	childFolder := strings.TrimPrefix(path.Join(folder, stem(e.key)), "./")
	if e.folder {
		childFolder = folder
	}
	m.folders[childFolder] = &id
	return nil
}

// relink reads a file again now that every page exists, so its links can lead to
// them. It keeps the attachments of the first pass.
func (m *importer) relink(ctx context.Context, e planPage) error {
	id := m.byKey[e.key]
	data, err := m.reread(e)
	if err != nil {
		return err
	}
	m.pending = false
	got, err := e.convert(ctx, m, id, e.key, data)
	if err != nil {
		return err
	}
	m.warnings[e.key] = got.warnings
	title := e.title
	if got.title != "" {
		title = got.title
	}
	version, err := m.out.publish(ctx, id, cutTitle(title), got.doc, 1)
	if err != nil {
		return err
	}
	m.items.add(loggedItem{page: id, source: e.key, version: version})
	return nil
}

func (m *importer) reread(e planPage) ([]byte, error) {
	before := m.unpacked
	data, err := m.read(m.byName[e.key], e.limit)
	m.unpacked = before // the same file does not count twice
	return data, err
}

// attachment stores a file a document points to, if the archive has it. A link to
// another page file is a link between pages, not a file.
func (m *importer) attachment(ctx context.Context, pageID uuid.UUID, folder, dest string) (uuid.UUID, bool) {
	dest = strings.NewReplacer("%20", " ", "%28", "(", "%29", ")").Replace(dest)
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") {
		return uuid.Nil, false
	}
	name := strings.TrimPrefix(path.Clean(path.Join(folder, dest)), "./")
	e, ok := m.byName[name]
	if !ok || isPageFile(name) {
		return uuid.Nil, false
	}
	return m.store(ctx, pageID, name, func() ([]byte, error) { return m.read(e, m.limit) })
}

func (m *importer) store(ctx context.Context, pageID uuid.UUID, name string, read func() ([]byte, error)) (uuid.UUID, bool) {
	key := pageID.String() + "/" + name
	if id, ok := m.attached[key]; ok {
		return id, true
	}
	data, err := read()
	if err != nil {
		m.warn("%s: could not read this file.", name)
		return uuid.Nil, false
	}
	id, err := m.out.upload(ctx, pageID, path.Base(name), data)
	if err != nil {
		m.warn("%s: could not store this file.", name)
		return uuid.Nil, false
	}
	m.attached[key] = id
	m.result.Files++
	return id, true
}

func isPageFile(name string) bool { return isAny(name, ".md", ".markdown", ".html", ".htm", ".docx") }

// pageLink turns a link between files into the address of the new page. When that
// page does not exist yet, the file gets a second pass.
func (m *importer) pageLink(folder, dest string) (string, bool) {
	target, fragment, _ := strings.Cut(dest, "#")
	if target == "" || strings.Contains(target, ":") || strings.HasPrefix(target, "/") {
		return "", false
	}
	target = strings.NewReplacer("%20", " ", "%28", "(", "%29", ")").Replace(target)
	name := strings.TrimPrefix(path.Clean(path.Join(folder, target)), "./")
	if _, inArchive := m.byName[name]; !inArchive || !isPageFile(name) {
		return "", false
	}
	id, ok := m.byKey[name]
	if !ok {
		m.pending = true
		return "", false
	}
	href := "/pages/" + id.String()
	if fragment != "" {
		href += "#" + fragment
	}
	return href, true
}

var wikiLink = regexp.MustCompile(`\[\[([^\[\]|]+)(?:\|([^\[\]]+))?\]\]`)

// wikiLinks turns [[Page]] and [[Page|text]] into links, by title within the import.
func (m *importer) wikiLinks(n *document.Node) {
	var out []document.Node
	for _, c := range n.Content {
		if c.Type != document.TypeText || !strings.Contains(c.Text, "[[") {
			m.wikiLinks(&c)
			out = append(out, c)
			continue
		}
		out = append(out, m.splitWiki(c)...)
	}
	n.Content = out
}

func (m *importer) splitWiki(c document.Node) []document.Node {
	var out []document.Node
	rest := c.Text
	for {
		loc := wikiLink.FindStringSubmatchIndex(rest)
		if loc == nil {
			break
		}
		title := strings.TrimSpace(rest[loc[2]:loc[3]])
		words := title
		if loc[4] >= 0 {
			words = strings.TrimSpace(rest[loc[4]:loc[5]])
		}
		id, ok := m.byTitle[strings.ToLower(title)]
		if !ok {
			m.pending = true
		}
		if before := rest[:loc[0]]; before != "" {
			out = append(out, document.Node{Type: document.TypeText, Text: before, Marks: c.Marks})
		}
		link := document.Node{Type: document.TypeText, Text: words, Marks: c.Marks}
		if ok {
			link.Marks = append(append([]document.Mark{}, c.Marks...), document.Mark{Type: document.MarkLink,
				Attrs: map[string]any{"href": "/pages/" + id.String()}})
		}
		out = append(out, link)
		rest = rest[loc[1]:]
	}
	if rest != "" {
		out = append(out, document.Node{Type: document.TypeText, Text: rest, Marks: c.Marks})
	}
	return out
}

func convertMarkdown(ctx context.Context, m *importer, pageID uuid.UUID, key string, data []byte) (converted, error) {
	folder := path.Dir(key)
	front, body, warnings := splitFrontMatter(data)
	resolve := func(dest string) (uuid.UUID, bool) { return m.attachment(ctx, pageID, folder, dest) }
	links := func(dest string) (string, bool) { return m.pageLink(folder, dest) }
	doc, more := document.FromMarkdownLinked(body, resolve, links)
	if m.pending {
		more = nil // the second pass says what is true
	}
	m.wikiLinks(&doc)
	return converted{doc: doc, title: front.title, labels: front.labels, stage: front.status, warnings: append(warnings, more...)}, nil
}

const (
	maxWordBytes       int64 = 200 << 20
	importedStageColor       = "grey"
)

func convertWord(ctx context.Context, m *importer, pageID uuid.UUID, _ string, data []byte) (converted, error) {
	n := 0
	got, err := office.ReadDOCX(data, func(name string, image []byte) (map[string]any, bool) {
		n++
		id, ok := m.store(ctx, pageID, fmt.Sprintf("%d-%s", n, name), func() ([]byte, error) { return image, nil })
		if !ok {
			return nil, false
		}
		return map[string]any{"attachment_id": id.String()}, true
	})
	if err != nil {
		return converted{}, err
	}
	return converted{doc: checked(got.Document, &got.Warnings), title: got.Title, warnings: got.Warnings}, nil
}

// checked keeps what the allow-list refuses readable, as text in a note (D9).
func checked(doc document.Node, warnings *[]string) document.Node {
	raw, err := json.Marshal(doc)
	if err == nil {
		if _, err := document.Normalize(raw); err == nil {
			return doc
		}
	}
	*warnings = append(*warnings, "Part of this file could only be kept as text.")
	return noteOf(document.Text(doc))
}

// noteOf is a Note panel with readable text.
func noteOf(text string) document.Node {
	return document.Node{Type: document.TypeDoc, Content: []document.Node{{Type: document.TypePanel,
		Attrs: map[string]any{"kind": "note"}, Content: textDocument(text).Content}}}
}
