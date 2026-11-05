// SPDX-License-Identifier: Apache-2.0

// Package exchange moves content in and out as Markdown (spec 003). Everything
// goes through the public methods of pages and attachments, so exports and
// imports obey the same permission checks as the UI.
package exchange

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/storage"
)

// JobKindImport is the background job for imports.
const JobKindImport = "markdown_import"

// Import limits (spec 003).
const (
	MaxArchiveBytes  int64 = 200 << 20
	MaxUnpackedBytes int64 = 1 << 30
	MaxFiles               = 10_000
	MaxMarkdownBytes int64 = 20 << 20
)

// Service exports and imports.
type Service struct {
	pool        *pgxpool.Pool
	perms       *permissions.Service
	pages       *pages.Service
	attachments *attachments.Service
	store       storage.Store
	jobs        *jobs.Service
	tempDir     string
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, pg *pages.Service, att *attachments.Service,
	store storage.Store, jb *jobs.Service, tempDir string) *Service {
	s := &Service{pool: pool, perms: perms, pages: pg, attachments: att, store: store, jobs: jb, tempDir: tempDir}
	jb.Register(JobKindImport, s.runImport)
	return s
}

// safeName turns a title into a file name that works on every system.
func safeName(title string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, strings.TrimSpace(title))
	s = strings.Trim(s, ". ")
	if s == "" {
		s = "untitled"
	}
	if r := []rune(s); len(r) > 100 {
		s = string(r[:100])
	}
	return s
}

func unique(used map[string]bool, name string) string {
	candidate := name
	for i := 2; used[strings.ToLower(candidate)]; i++ {
		candidate = fmt.Sprintf("%s (%d)", name, i)
	}
	used[strings.ToLower(candidate)] = true
	return candidate
}

func (s *Service) recordExport(ctx context.Context, c identity.Caller, action, objType string, objID, spaceID uuid.UUID, pageCount int) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := audit.Record(ctx, tx, c.Event(action, objType, objID.String(), spaceID.String(),
			map[string]any{"pages": pageCount, "format": "markdown"})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"object_id": objID, "space_id": spaceID})
		return err
	})
}

// PageMarkdown exports one page. It returns a file name and the content.
func (s *Service) PageMarkdown(ctx context.Context, c identity.Caller, pageID uuid.UUID) (string, []byte, error) {
	spaceID, err := s.pages.SpaceOf(ctx, c, pageID)
	if err != nil {
		return "", nil, err
	}
	list, err := s.pages.ForExport(ctx, c, spaceID, &pageID)
	if err != nil {
		return "", nil, err
	}
	for _, p := range list {
		if p.ID != pageID {
			continue
		}
		doc, err := document.Parse(p.Document)
		if err != nil {
			return "", nil, err
		}
		files, err := s.attachments.ListForPage(ctx, pageID)
		if err != nil {
			return "", nil, err
		}
		names := map[uuid.UUID]string{}
		for _, f := range files {
			names[f.ID] = f.Filename
		}
		md := document.ToMarkdown(doc, func(id uuid.UUID) string {
			if n, ok := names[id]; ok {
				return escapePath(n)
			}
			return id.String()
		})
		if err := s.recordExport(ctx, c, "page.exported", "page", pageID, spaceID, 1); err != nil {
			return "", nil, err
		}
		return safeName(p.Title) + ".md", []byte(md), nil
	}
	return "", nil, apperr.NotFound()
}

func escapePath(p string) string {
	return strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29").Replace(p)
}

// Archive writes a zip with the page tree. With root nil, the whole space.
// Layout: Title.md, its files in "Title.files/", its children in "Title/".
func (s *Service) Archive(ctx context.Context, c identity.Caller, spaceID uuid.UUID, root *uuid.UUID, w io.Writer) error {
	list, err := s.pages.ForExport(ctx, c, spaceID, root)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return apperr.NotFound()
	}
	if root != nil {
		if err := s.recordExport(ctx, c, "page.exported", "page", *root, spaceID, len(list)); err != nil {
			return err
		}
	} else if err := s.recordExport(ctx, c, "space.exported", "space", spaceID, spaceID, len(list)); err != nil {
		return err
	}

	dirs := map[uuid.UUID]string{}         // page id -> folder for its children
	usedIn := map[string]map[string]bool{} // folder -> names taken
	zw := zip.NewWriter(w)
	for _, p := range list {
		folder := ""
		if p.ParentID != nil {
			folder = dirs[*p.ParentID] // parents come first; a hidden parent means the top level
		}
		if usedIn[folder] == nil {
			usedIn[folder] = map[string]bool{}
		}
		name := unique(usedIn[folder], safeName(p.Title))
		dirs[p.ID] = path.Join(folder, name)

		doc, err := document.Parse(p.Document)
		if err != nil {
			return err
		}
		files, err := s.attachments.ListForPage(ctx, p.ID)
		if err != nil {
			return err
		}
		names := map[uuid.UUID]string{}
		for _, f := range files {
			names[f.ID] = name + ".files/" + f.Filename
		}
		md := document.ToMarkdown(doc, func(id uuid.UUID) string {
			if n, ok := names[id]; ok {
				return escapePath(n)
			}
			return id.String()
		})
		f, err := zw.Create(path.Join(folder, name+".md"))
		if err != nil {
			return err
		}
		if _, err := io.WriteString(f, md); err != nil {
			return err
		}
		for _, a := range files {
			if err := s.addFile(ctx, zw, path.Join(folder, name+".files", a.Filename), a); err != nil {
				return err
			}
		}
	}
	return zw.Close()
}

func (s *Service) addFile(ctx context.Context, zw *zip.Writer, name string, a attachments.Attachment) error {
	src, err := s.attachments.OpenUnchecked(ctx, a)
	if err != nil {
		if apperr.IsNotFound(err) {
			return nil // a missing file must not stop the export of everything else
		}
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

type importPayload struct {
	SpaceID  uuid.UUID  `json:"space_id"`
	ParentID *uuid.UUID `json:"parent_id"`
	Object   string     `json:"object"`
	Filename string     `json:"filename"`
}

// ImportResult is what the user sees when an import ends.
type ImportResult struct {
	Pages    int      `json:"pages"`
	Files    int      `json:"files"`
	Warnings []string `json:"warnings"`
}

// StartImport stores the upload and queues the import.
func (s *Service) StartImport(ctx context.Context, c identity.Caller, spaceID uuid.UUID, parent *uuid.UUID, filename string, body io.Reader) (uuid.UUID, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.PageCreate); err != nil {
		return uuid.Nil, err
	}
	if parent != nil {
		if err := s.perms.RequirePage(ctx, s.pool, c.UserID, *parent, permissions.PageCreate, uuid.Nil); err != nil {
			return uuid.Nil, err
		}
	}
	lower := strings.ToLower(filename)
	if !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") && !strings.HasSuffix(lower, ".zip") {
		return uuid.Nil, apperr.Invalid("Upload a .md file or a .zip file.")
	}
	key := "imports/" + ids.New().String()
	if _, err := s.store.Put(ctx, key, body, MaxArchiveBytes); err != nil {
		if errors.Is(err, storage.ErrTooLarge) {
			return uuid.Nil, apperr.TooLarge("The upload is larger than the limit of 200 MiB.")
		}
		return uuid.Nil, err
	}
	var id uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		id, err = s.jobs.Enqueue(ctx, tx, JobKindImport, importPayload{SpaceID: spaceID, ParentID: parent, Object: key, Filename: filename}, c.UserID)
		if err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("import.started", "space", spaceID.String(), spaceID.String(),
			map[string]any{"filename": filename, "job": id})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "import.started", map[string]any{"job_id": id, "space_id": spaceID})
		return err
	})
	if err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), key)
		return uuid.Nil, err
	}
	return id, nil
}

// entry is one file of an import.
type entry struct {
	name string // cleaned path, forward slashes, never used on disk
	open func() (io.ReadCloser, error)
	size uint64
}

// cleanEntryName normalises a path from a zip. A path that tries to climb out
// is refused. Paths are only used as labels, but a label should not lie.
func cleanEntryName(name string) (string, bool) {
	name = strings.ReplaceAll(name, `\`, "/")
	clean := path.Clean("/" + name)[1:]
	if clean == "" || strings.Contains(name, "\x00") {
		return "", false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false
		}
	}
	for _, part := range strings.Split(clean, "/") {
		if strings.HasPrefix(part, ".") || part == "__MACOSX" {
			return "", false
		}
	}
	return clean, true
}

func (s *Service) runImport(ctx context.Context, j jobs.Job) (any, error) {
	var in importPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	defer func() { _ = s.store.Delete(context.WithoutCancel(ctx), in.Object) }()

	c := identity.Caller{UserID: j.CreatedBy, Meta: identity.RequestMeta{RequestID: "job-" + j.ID.String()}}
	result := &ImportResult{Warnings: []string{}}
	src, err := s.store.Open(ctx, in.Object)
	if err != nil {
		return result, err
	}
	defer func() { _ = src.Close() }()

	var entries []entry
	if strings.HasSuffix(strings.ToLower(in.Filename), ".zip") {
		tmp, err := os.CreateTemp(s.tempDir, "import-*.zip")
		if err != nil {
			return result, err
		}
		defer func() { _ = os.Remove(tmp.Name()); _ = tmp.Close() }()
		size, err := io.Copy(tmp, src)
		if err != nil {
			return result, err
		}
		zr, err := zip.NewReader(tmp, size)
		if err != nil {
			return result, apperr.Invalid("The file is not a valid zip archive.")
		}
		if entries, err = zipEntries(zr, result); err != nil {
			return result, err
		}
	} else {
		name := path.Base(strings.ReplaceAll(in.Filename, `\`, "/"))
		entries = []entry{{name: name, open: func() (io.ReadCloser, error) { return io.NopCloser(src), nil }}}
	}

	imp := &importer{s: s, c: c, spaceID: in.SpaceID, root: in.ParentID, entries: entries, result: result,
		byName: map[string]entry{}, folders: map[string]*uuid.UUID{"": in.ParentID}}
	err = imp.run(ctx)

	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10_000_000_000)
	defer cancel()
	_ = pgx.BeginFunc(finish, s.pool, func(tx pgx.Tx) error {
		ev := c.Event("import.finished", "space", in.SpaceID.String(), in.SpaceID.String(),
			map[string]any{"job": j.ID, "pages": result.Pages, "files": result.Files, "ok": err == nil})
		if _, e := audit.Record(finish, tx, ev); e != nil {
			return e
		}
		_, e := outbox.Enqueue(finish, tx, "import.finished", map[string]any{"job_id": j.ID, "space_id": in.SpaceID})
		return e
	})
	return result, err
}

// zipEntries checks the limits a zip claims. The real size is enforced again
// while reading, because headers can lie.
func zipEntries(zr *zip.Reader, result *ImportResult) ([]entry, error) {
	if len(zr.File) > MaxFiles {
		return nil, apperr.TooLarge("The archive has more than 10,000 files.")
	}
	var total uint64
	var out []entry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		clean, ok := cleanEntryName(f.Name)
		if !ok {
			if strings.Contains(f.Name, "..") && len(result.Warnings) < 100 {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Skipped the unsafe path %q.", truncate(f.Name, 80)))
			}
			continue
		}
		total += f.UncompressedSize64
		if total > uint64(MaxUnpackedBytes) {
			return nil, apperr.TooLarge("The archive unpacks to more than 1 GiB.")
		}
		out = append(out, entry{name: clean, open: f.Open, size: f.UncompressedSize64})
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return strings.ToValidUTF8(s[:n], "")
	}
	return s
}

type importer struct {
	s        *Service
	c        identity.Caller
	spaceID  uuid.UUID
	root     *uuid.UUID
	entries  []entry
	result   *ImportResult
	byName   map[string]entry
	folders  map[string]*uuid.UUID // folder path -> page its children go under
	unpacked int64
}

func isMarkdown(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

func stem(name string) string {
	base := path.Base(name)
	return strings.TrimSuffix(base, path.Ext(base))
}

func (m *importer) warn(format string, args ...any) {
	if len(m.result.Warnings) < 200 {
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
	if m.unpacked > MaxUnpackedBytes {
		return nil, apperr.TooLarge("The archive unpacks to more than 1 GiB.")
	}
	return b, nil
}

func (m *importer) run(ctx context.Context) error {
	var markdown []entry
	for _, e := range m.entries {
		m.byName[e.name] = e
		if isMarkdown(e.name) {
			markdown = append(markdown, e)
		}
	}
	if len(markdown) == 0 {
		return apperr.Invalid("The upload has no Markdown files.")
	}
	// Shallow paths first, so a parent exists before its children.
	sort.Slice(markdown, func(i, j int) bool {
		a, b := strings.Count(markdown[i].name, "/"), strings.Count(markdown[j].name, "/")
		if a != b {
			return a < b
		}
		return markdown[i].name < markdown[j].name
	})
	for _, e := range markdown {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := m.page(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// parentFor returns the page that holds a folder, creating empty pages for
// folders that have no Markdown file of their own.
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
	id, err := m.publish(ctx, parent, path.Base(folder), document.Empty(), nil)
	if err != nil {
		return nil, err
	}
	m.folders[folder] = &id
	return &id, nil
}

func (m *importer) publish(ctx context.Context, parent *uuid.UUID, title string, doc document.Node, fill func(uuid.UUID) (document.Node, error)) (uuid.UUID, error) {
	p, err := m.s.pages.Create(ctx, m.c, pages.NewPage{SpaceID: m.spaceID, ParentID: parent, Title: title})
	if err != nil {
		return uuid.Nil, err
	}
	if fill != nil {
		if doc, err = fill(p.ID); err != nil {
			return uuid.Nil, err
		}
	}
	raw, err := document.Marshal(doc)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := m.s.pages.Publish(ctx, m.c, p.ID, pages.PublishInput{Title: title, Document: raw}); err != nil {
		return uuid.Nil, err
	}
	m.result.Pages++
	return p.ID, nil
}

func (m *importer) page(ctx context.Context, e entry) error {
	source, err := m.read(e, MaxMarkdownBytes)
	if err != nil {
		return err
	}
	folder := path.Dir(e.name)
	parent, err := m.parentFor(ctx, folder)
	if err != nil {
		return err
	}
	title := stem(e.name)
	if r := []rune(title); len(r) > 255 {
		title = string(r[:255])
	}
	id, err := m.publish(ctx, parent, title, document.Node{}, func(pageID uuid.UUID) (document.Node, error) {
		doc, warnings := document.FromMarkdown(source, func(dest string) (uuid.UUID, bool) {
			return m.attachment(ctx, pageID, folder, dest)
		})
		for _, w := range warnings {
			m.warn("%s: %s", e.name, w)
		}
		return doc, nil
	})
	if err != nil {
		return err
	}
	// Children of "Title.md" live in the folder "Title".
	m.folders[strings.TrimPrefix(path.Join(folder, stem(e.name)), "./")] = &id
	return nil
}

// attachment uploads a file the Markdown points to, if the archive has it. A
// link to another Markdown file is a link between pages, not a file.
func (m *importer) attachment(ctx context.Context, pageID uuid.UUID, folder, dest string) (uuid.UUID, bool) {
	dest = strings.NewReplacer("%20", " ", "%28", "(", "%29", ")").Replace(dest)
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "/") {
		return uuid.Nil, false
	}
	name := strings.TrimPrefix(path.Clean(path.Join(folder, dest)), "./")
	e, ok := m.byName[name]
	if !ok || isMarkdown(name) {
		return uuid.Nil, false
	}
	data, err := m.read(e, MaxUnpackedBytes)
	if err != nil {
		m.warn("%s: could not read this file.", name)
		return uuid.Nil, false
	}
	a, err := m.s.attachments.Upload(ctx, m.c, pageID, path.Base(name), bytes.NewReader(data))
	if err != nil {
		m.warn("%s: could not store this file.", name)
		return uuid.Nil, false
	}
	m.result.Files++
	return a.ID, true
}
