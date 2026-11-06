// SPDX-License-Identifier: Apache-2.0

// Package exchange moves content in and out (specs 003 and 014). Everything but a
// bundle goes through the public methods of pages and attachments, so exports and
// imports obey the same permission checks as the UI.
package exchange

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path"
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
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/storage"
)

// Import limits (spec 003). What a file may unpack to grows with what an
// administrator allows, at this many times its size.
const (
	unpackFactor           = 5
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
	settings    *settings.Service
	keyring     *keys.Keyring
	instanceID  uuid.UUID
	log         *slog.Logger
	tempDir     string
	pdfURL      string
	// collaboraURL is an office service that an administrator configured. Empty = none.
	collaboraURL string
}

// Options are the parts of the service that are not other modules.
type Options struct {
	TempDir string
	PDFURL  string // the helper process that makes PDF files; empty = no PDF
	// CollaboraURL converts office files when it is set, in place of the PDF helper.
	CollaboraURL string
	Log          *slog.Logger
	Keyring      *keys.Keyring
	// InstanceID is what a bundle says it came from.
	InstanceID uuid.UUID
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, pg *pages.Service, att *attachments.Service,
	store storage.Store, jb *jobs.Service, st *settings.Service, o Options) *Service {
	s := &Service{pool: pool, perms: perms, pages: pg, attachments: att, store: store, jobs: jb, settings: st,
		log: o.Log, tempDir: o.TempDir, pdfURL: o.PDFURL, collaboraURL: o.CollaboraURL, keyring: o.Keyring, instanceID: o.InstanceID}
	jb.Register(JobKindImport, s.runImport)
	jb.Register(JobKindImportRead, s.readImport)
	jb.Register(JobKindExport, s.runExport)
	jb.Register(JobKindBundle, s.runBundle)
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

	zw := zip.NewWriter(w)
	if err := s.writeMarkdownTree(ctx, zw, list); err != nil {
		return err
	}
	return zw.Close()
}

func exportPagesOf(items []exportItem) []pages.ExportPage {
	out := make([]pages.ExportPage, 0, len(items))
	for _, item := range items {
		out = append(out, item.page)
	}
	return out
}

// writeMarkdownTree adds pages to a zip. Parents must come first.
func (s *Service) writeMarkdownTree(ctx context.Context, zw *zip.Writer, list []pages.ExportPage) error {
	dirs := map[uuid.UUID]string{}         // page id -> folder for its children
	usedIn := map[string]map[string]bool{} // folder -> names taken
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
	return nil
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
