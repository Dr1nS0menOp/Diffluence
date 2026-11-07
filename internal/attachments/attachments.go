// SPDX-License-Identifier: Apache-2.0

// Package attachments stores files on pages (spec 003). Access always follows
// the page. Uploaded files are never trusted: the type comes from the content,
// and only a few raster image types are ever shown inline.
package attachments

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/storage"
	"github.com/diffluence/diffluence/internal/trash"
)

// Attachment is a file on a page.
type Attachment struct {
	ID          uuid.UUID `json:"id"`
	PageID      uuid.UUID `json:"page_id"`
	SpaceID     uuid.UUID `json:"space_id"`
	Filename    string    `json:"filename"`
	Version     int       `json:"version"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	Inline      bool      `json:"inline"`
	Office      bool      `json:"office"`
	CreatedBy   uuid.UUID `json:"created_by"`
	UpdatedAt   time.Time `json:"updated_at"`
	// SHA256 is the hash of the current version, in hex. It is the file's ETag.
	SHA256 string `json:"-"`
}

// Service manages attachments.
type Service struct {
	pool       *pgxpool.Pool
	perms      *permissions.Service
	store      storage.Store
	settings   *settings.Service
	instanceID uuid.UUID
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, store storage.Store, st *settings.Service, instanceID uuid.UUID) *Service {
	return &Service{pool: pool, perms: perms, store: store, settings: st, instanceID: instanceID}
}

// inlineTypes are the only types a browser may ever display from us. SVG is not
// here: it can carry scripts.
var inlineTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// IsInline reports whether a detected content type may be shown in the browser.
func IsInline(contentType string) bool { return inlineTypes[contentType] }

// IsImageOf reports whether an attachment is an active image on this page, which
// is what a page may show as its header.
func (s *Service) IsImageOf(ctx context.Context, q db.Querier, id, pageID uuid.UUID) (bool, error) {
	var contentType string
	err := q.QueryRow(ctx, `SELECT v.content_type FROM attachments a
		JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version
		WHERE a.id = $1 AND a.page_id = $2 AND a.status = 'active'`, id, pageID).Scan(&contentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return IsInline(contentType), err
}

// MoveToSpace keeps the files of moved pages in step with their pages, inside the
// transaction of the move.
func (s *Service) MoveToSpace(ctx context.Context, tx pgx.Tx, pageIDs []uuid.UUID, spaceID uuid.UUID) error {
	_, err := tx.Exec(ctx, "UPDATE attachments SET space_id = $2 WHERE page_id = ANY($1)", pageIDs, spaceID)
	return err
}

// Found is a file that matched a quick search.
type Found struct {
	ID        uuid.UUID `json:"id"`
	Filename  string    `json:"filename"`
	PageID    uuid.UUID `json:"page_id"`
	PageTitle string    `json:"page_title"`
	SpaceName string    `json:"space_name"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Find looks for files by name or label on pages the user may see. A zero spaceID means
// every space. Files in archived spaces stay out of quick search (SPC-069).
func (s *Service) Find(ctx context.Context, c identity.Caller, text string, spaceID uuid.UUID, limit int) ([]Found, error) {
	var within *uuid.UUID
	if spaceID != uuid.Nil {
		within = &spaceID
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id, a.filename, a.page_id, p.title, sp.name, a.updated_at
		FROM attachments a
		JOIN perm_listed_pages($1) v ON v.page_id = a.page_id
		JOIN pages p ON p.id = a.page_id
		JOIN spaces sp ON sp.id = a.space_id
		WHERE a.status = 'active' AND sp.status = 'active' AND (position(lower($2) IN lower(a.filename)) > 0
			OR EXISTS (SELECT 1 FROM attachment_labels l WHERE l.attachment_id = a.id AND l.label = lower($2)))
		  AND ($3::uuid IS NULL OR a.space_id = $3)
		ORDER BY a.updated_at DESC, a.id LIMIT $4`, c.UserID, text, within, limit)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Found, error) {
		var f Found
		err := r.Scan(&f.ID, &f.Filename, &f.PageID, &f.PageTitle, &f.SpaceName, &f.UpdatedAt)
		return f, err
	})
	if out == nil {
		out = []Found{}
	}
	return out, err
}

// cleanFilename keeps a usable display name. Names are only ever shown, never
// used as a path, but a clean name protects whoever downloads the file.
func cleanFilename(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || name == "/" {
		return "", apperr.Invalid("The file needs a name.")
	}
	if len(name) > 255 {
		return "", apperr.Invalid("The file name is longer than 255 bytes.")
	}
	return name, nil
}

func (s *Service) pageSpace(ctx context.Context, q db.Querier, pageID uuid.UUID) (uuid.UUID, error) {
	var spaceID uuid.UUID
	err := q.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", pageID).Scan(&spaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return spaceID, apperr.NotFound()
	}
	return spaceID, err
}

func (s *Service) reindexNames(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) error {
	rows, err := tx.Query(ctx, "SELECT filename FROM attachments WHERE page_id = $1 AND status = 'active' ORDER BY filename", pageID)
	if err != nil {
		return err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	return search.SetAttachmentNames(ctx, tx, pageID, names)
}

// incoming is a file in storage that no page holds yet.
type incoming struct {
	key         string
	contentType string
	size        int64
	sha256      []byte
}

// receive stores what came in, after looking for a blocked type in the content and in
// each of the names. The caller removes it again if it does not end up on a page.
func (s *Service) receive(ctx context.Context, spaceID uuid.UUID, names []string, body io.Reader) (incoming, error) {
	limit, err := s.allowanceFor(ctx, spaceID)
	if err != nil {
		return incoming{}, err
	}
	if limit.bytes <= 0 {
		return incoming{}, limit.refusal
	}
	reader := bufio.NewReaderSize(body, 512)
	head, err := reader.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return incoming{}, err
	}
	contentType, _, _ := strings.Cut(http.DetectContentType(head), ";")
	blocked, err := s.settings.BlockedFileTypes(ctx)
	if err != nil {
		return incoming{}, err
	}
	for _, name := range names {
		if isBlocked(blocked, name, contentType, head) {
			return incoming{}, errBlocked()
		}
	}
	key := "attachments/" + ids.New().String()
	obj, err := s.store.Put(ctx, key, reader, limit.bytes)
	if errors.Is(err, storage.ErrTooLarge) {
		return incoming{}, limit.refusal
	}
	if err != nil {
		return incoming{}, err
	}
	return incoming{key: key, contentType: contentType, size: obj.Size, sha256: obj.SHA256}, nil
}

// addVersion makes a stored file the next version, with all that goes with it.
func (s *Service) addVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, a Attachment, in incoming) error {
	if _, err := tx.Exec(ctx, `INSERT INTO attachment_versions (id, attachment_id, number, object_key, sha256, size, content_type, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, ids.New(), a.ID, a.Version, in.key, in.sha256, in.size, in.contentType, c.UserID); err != nil {
		return err
	}
	if err := s.reindexNames(ctx, tx, a.PageID); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("attachment.uploaded", "attachment", a.ID.String(), a.SpaceID.String(), map[string]any{
		"page_id": a.PageID, "filename": a.Filename, "version": a.Version, "size": in.size, "content_type": in.contentType})); err != nil {
		return err
	}
	if _, err := outbox.Enqueue(ctx, tx, "attachment.uploaded", map[string]any{"attachment_id": a.ID, "page_id": a.PageID, "space_id": a.SpaceID}); err != nil {
		return err
	}
	action := eventVersion
	if a.Version == 1 {
		action = eventAdded
	}
	return addEvent(ctx, tx, c, a.PageID, a.ID, a.Filename, action)
}

// Upload stores a file on a page. A file with the same name gets a new version.
func (s *Service) Upload(ctx context.Context, c identity.Caller, pageID uuid.UUID, filename string, body io.Reader) (Attachment, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.AttachmentAdd, uuid.Nil); err != nil {
		return Attachment{}, err
	}
	name, err := cleanFilename(filename)
	if err != nil {
		return Attachment{}, err
	}
	spaceID, err := s.pageSpace(ctx, s.pool, pageID)
	if err != nil {
		return Attachment{}, err
	}
	in, err := s.receive(ctx, spaceID, []string{name}, body)
	if err != nil {
		return Attachment{}, err
	}
	var a Attachment
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Checked again: the rights may have changed during a long upload.
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.AttachmentAdd, uuid.Nil); err != nil {
			return err
		}
		spaceID, err := s.pageSpace(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if err := requireRoom(ctx, tx, spaceID, in.size); err != nil {
			return err
		}
		id, version := uuid.Nil, 1
		err = tx.QueryRow(ctx, `SELECT id, current_version + 1 FROM attachments
			WHERE page_id = $1 AND lower(filename) = lower($2) AND status = 'active' FOR UPDATE`, pageID, name).Scan(&id, &version)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			id = ids.New()
			if _, err := tx.Exec(ctx, `INSERT INTO attachments (id, origin_instance_id, space_id, page_id, filename, status, created_by)
				VALUES ($1, $2, $3, $4, $5, 'active', $6)`, id, s.instanceID, spaceID, pageID, name, c.UserID); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			// The existing row keeps its creator; "delete own" follows who started it.
			if _, err := tx.Exec(ctx, "UPDATE attachments SET current_version = $2, updated_at = now() WHERE id = $1", id, version); err != nil {
				return err
			}
		}
		target := Attachment{ID: id, PageID: pageID, SpaceID: spaceID, Filename: name, Version: version}
		if err := s.addVersion(ctx, tx, c, target, in); err != nil {
			return err
		}
		a, err = get(ctx, tx, id)
		return err
	})
	if err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), in.key)
		return Attachment{}, err
	}
	return a, nil
}

// AddVersion makes a file the next version of one that exists, whatever it was called
// (FIL-013). The name stays, so tiles and links stay right, but the name it came with
// still counts for blocked types.
func (s *Service) AddVersion(ctx context.Context, c identity.Caller, id uuid.UUID, droppedName string, body io.Reader) (Attachment, error) {
	current, err := get(ctx, s.pool, id)
	if err != nil {
		return Attachment{}, err
	}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, current.PageID, permissions.AttachmentAdd, uuid.Nil); err != nil {
		return Attachment{}, err
	}
	name, err := cleanFilename(droppedName)
	if err != nil {
		return Attachment{}, err
	}
	in, err := s.receive(ctx, current.SpaceID, []string{current.Filename, name}, body)
	if err != nil {
		return Attachment{}, err
	}
	var out Attachment
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := s.changeable(ctx, tx, c, id)
		if err != nil {
			return err
		}
		if err := requireRoom(ctx, tx, a.SpaceID, in.size); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE attachments SET current_version = current_version + 1, updated_at = now()
			WHERE id = $1 RETURNING current_version`, id).Scan(&a.Version); err != nil {
			return err
		}
		if err := s.addVersion(ctx, tx, c, a, in); err != nil {
			return err
		}
		out, err = get(ctx, tx, id)
		return err
	})
	if err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), in.key)
		return Attachment{}, err
	}
	return out, nil
}

const (
	selectColumns = `a.id, a.page_id, a.space_id, a.filename, a.current_version, v.size, v.content_type, a.created_by, a.updated_at, encode(v.sha256, 'hex')`
	selectFrom    = `FROM attachments a JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version`
	selectSQL     = `SELECT ` + selectColumns + ` ` + selectFrom
	labelsSQL     = `COALESCE((SELECT array_agg(l.label ORDER BY l.label) FROM attachment_labels l WHERE l.attachment_id = a.id), '{}')`
)

func scan(r pgx.Row) (Attachment, error) {
	var a Attachment
	err := r.Scan(&a.ID, &a.PageID, &a.SpaceID, &a.Filename, &a.Version, &a.Size, &a.ContentType, &a.CreatedBy, &a.UpdatedAt, &a.SHA256)
	a.Inline = IsInline(a.ContentType)
	a.Office = IsOffice(a.Filename)
	return a, err
}

func get(ctx context.Context, q db.Querier, id uuid.UUID) (Attachment, error) {
	a, err := scan(q.QueryRow(ctx, selectSQL+" WHERE a.id = $1 AND a.status = 'active'", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, apperr.NotFound()
	}
	return a, err
}

// List returns a page's files.
func (s *Service) List(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Attachment, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	return s.ListForPage(ctx, pageID)
}

// ListForPage lists without a check, for callers that have already checked.
func (s *Service) ListForPage(ctx context.Context, pageID uuid.UUID) ([]Attachment, error) {
	rows, err := s.pool.Query(ctx, selectSQL+" WHERE a.page_id = $1 AND a.status = 'active' ORDER BY lower(a.filename)", pageID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Attachment, error) { return scan(r) })
	if out == nil {
		out = []Attachment{}
	}
	return out, err
}

// Open returns a file for reading. Downloads are audited; inline display of an
// image inside a page is not, as it counts as viewing the page.
func (s *Service) Open(ctx context.Context, c identity.Caller, id uuid.UUID, download bool) (Attachment, io.ReadSeekCloser, error) {
	a, err := get(ctx, s.pool, id)
	if err != nil {
		return a, nil, err
	}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, a.PageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return a, nil, err
	}
	f, err := s.OpenUnchecked(ctx, a)
	if err != nil {
		return a, nil, err
	}
	if download {
		err := audit.RecordStandalone(ctx, s.pool, c.Event("attachment.downloaded", "attachment", id.String(), a.SpaceID.String(),
			map[string]any{"page_id": a.PageID, "filename": a.Filename, "version": a.Version}))
		if err != nil {
			_ = f.Close()
			return a, nil, err
		}
	}
	return a, f, nil
}

// OpenOfPage reads a file that must belong to this page. It is for a module with a
// rule of its own, such as templates, whose pages the access rule refuses on purpose.
func (s *Service) OpenOfPage(ctx context.Context, pageID, id uuid.UUID) (Attachment, io.ReadSeekCloser, error) {
	a, err := get(ctx, s.pool, id)
	if err != nil {
		return a, nil, err
	}
	if a.PageID != pageID {
		return a, nil, apperr.NotFound()
	}
	f, err := s.OpenUnchecked(ctx, a)
	return a, f, err
}

// OpenUnchecked reads the current version, for callers that have already checked.
func (s *Service) OpenUnchecked(ctx context.Context, a Attachment) (io.ReadSeekCloser, error) {
	var key string
	if err := s.pool.QueryRow(ctx, "SELECT object_key FROM attachment_versions WHERE attachment_id = $1 AND number = $2", a.ID, a.Version).Scan(&key); err != nil {
		return nil, err
	}
	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, apperr.NotFound()
	}
	return f, err
}

// Delete sends a file to the trash.
func (s *Service) Delete(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.ActionAttachmentDelete, a.CreatedBy); err != nil {
			return err
		}
		return s.trashOne(ctx, tx, c, a, retention)
	})
}

// trashOne does the work of a delete. The caller has checked the rights.
func (s *Service) trashOne(ctx context.Context, tx pgx.Tx, c identity.Caller, a Attachment, retention time.Duration) error {
	item, err := trash.Add(ctx, tx, trash.Item{SpaceID: a.SpaceID, ObjectType: trash.TypeAttachment, ObjectID: a.ID,
		Title: a.Filename, DeletedBy: c.UserID}, retention)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE attachments SET status = 'trashed', trash_item_id = $2, updated_at = now() WHERE id = $1", a.ID, item); err != nil {
		return err
	}
	if err := s.reindexNames(ctx, tx, a.PageID); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("attachment.trashed", "attachment", a.ID.String(), a.SpaceID.String(),
		map[string]any{"page_id": a.PageID, "filename": a.Filename})); err != nil {
		return err
	}
	if _, err := outbox.Enqueue(ctx, tx, "attachment.trashed", map[string]any{"attachment_id": a.ID, "page_id": a.PageID}); err != nil {
		return err
	}
	return addEvent(ctx, tx, c, a.PageID, a.ID, a.Filename, eventRemoved)
}

// Restore implements trash.Handler.
func (s *Service) Restore(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	var pageID uuid.UUID
	var pageStatus, filename string
	err := tx.QueryRow(ctx, `SELECT a.page_id, p.status, a.filename FROM attachments a JOIN pages p ON p.id = a.page_id WHERE a.id = $1`, it.ObjectID).
		Scan(&pageID, &pageStatus, &filename)
	if err != nil {
		return err
	}
	if pageStatus == "trashed" {
		return apperr.Conflict("parent_in_trash", "The page of this file is in the trash. Restore the page first.")
	}
	_, err = tx.Exec(ctx, "UPDATE attachments SET status = 'active', trash_item_id = NULL, updated_at = now() WHERE id = $1", it.ObjectID)
	if db.IsUniqueViolation(err) {
		return apperr.Conflict("name_taken", "A file with this name exists on the page. Rename or delete it first.")
	}
	if err != nil {
		return err
	}
	if err := s.reindexNames(ctx, tx, pageID); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("attachment.restored", "attachment", it.ObjectID.String(), it.SpaceID.String(),
		map[string]any{"page_id": pageID, "filename": filename})); err != nil {
		return err
	}
	if _, err := outbox.Enqueue(ctx, tx, "attachment.restored", map[string]any{"attachment_id": it.ObjectID, "page_id": pageID}); err != nil {
		return err
	}
	return addEvent(ctx, tx, c, pageID, it.ObjectID, filename, eventRestored)
}

// Purge implements trash.Handler.
func (s *Service) Purge(ctx context.Context, tx pgx.Tx, c identity.Caller, it trash.Item) error {
	if _, err := tx.Exec(ctx, `INSERT INTO storage_deletions (object_key)
		SELECT object_key FROM attachment_versions WHERE attachment_id = $1 ON CONFLICT DO NOTHING`, it.ObjectID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM attachments WHERE id = $1", it.ObjectID); err != nil {
		return err
	}
	if _, err := audit.Record(ctx, tx, c.Event("attachment.purged", "attachment", it.ObjectID.String(), it.SpaceID.String(),
		map[string]any{"filename": it.Title})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, "attachment.purged", map[string]any{"attachment_id": it.ObjectID})
	return err
}

// QueueDeletionForPages schedules the stored files of pages for removal, inside
// the transaction that purges them.
func QueueDeletionForPages(ctx context.Context, tx pgx.Tx, pageIDs []uuid.UUID) error {
	_, err := tx.Exec(ctx, `INSERT INTO storage_deletions (object_key)
		SELECT v.object_key FROM attachment_versions v JOIN attachments a ON a.id = v.attachment_id
		WHERE a.page_id = ANY($1) ON CONFLICT DO NOTHING`, pageIDs)
	return err
}

// CopyAll gives a new page its own attachment rows pointing at new copies of
// the current files. It returns the mapping from old to new ids.
func (s *Service) CopyAll(ctx context.Context, tx pgx.Tx, c identity.Caller, from, to, spaceID uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT a.id, a.filename, v.object_key, v.content_type FROM attachments a
		JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version
		WHERE a.page_id = $1 AND a.status = 'active'`, from)
	if err != nil {
		return nil, err
	}
	type source struct {
		id                 uuid.UUID
		filename, key, typ string
	}
	sources, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (source, error) {
		var v source
		err := r.Scan(&v.id, &v.filename, &v.key, &v.typ)
		return v, err
	})
	if err != nil {
		return nil, err
	}
	limit := int64(1) << 42 // already stored once; the upload limit does not apply again
	mapping := map[uuid.UUID]uuid.UUID{}
	for _, src := range sources {
		f, err := s.store.Open(ctx, src.key)
		if err != nil {
			return nil, err
		}
		key := "attachments/" + ids.New().String()
		obj, err := s.store.Put(ctx, key, f, limit)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		id := ids.New()
		if _, err := tx.Exec(ctx, `INSERT INTO attachments (id, origin_instance_id, space_id, page_id, filename, status, created_by)
			VALUES ($1, $2, $3, $4, $5, 'active', $6)`, id, s.instanceID, spaceID, to, src.filename, c.UserID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO attachment_versions (id, attachment_id, number, object_key, sha256, size, content_type, uploaded_by)
			VALUES ($1, $2, 1, $3, $4, $5, $6, $7)`, ids.New(), id, key, obj.SHA256, obj.Size, src.typ, c.UserID); err != nil {
			return nil, err
		}
		mapping[src.id] = id
	}
	if len(mapping) > 0 {
		if err := s.reindexNames(ctx, tx, to); err != nil {
			return nil, err
		}
	}
	return mapping, nil
}

// DrainDeletions removes stored files queued by purges. Runs in the worker.
func (s *Service) DrainDeletions(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, "SELECT object_key FROM storage_deletions ORDER BY queued_at LIMIT 1000")
	if err != nil {
		return 0, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	n := 0
	for _, key := range keys {
		if err := s.store.Delete(ctx, key); err != nil {
			return n, err
		}
		if _, err := s.pool.Exec(ctx, "DELETE FROM storage_deletions WHERE object_key = $1", key); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
