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
	CreatedBy   uuid.UUID `json:"created_by"`
	UpdatedAt   time.Time `json:"updated_at"`
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

// Upload stores a file on a page. A file with the same name gets a new version.
func (s *Service) Upload(ctx context.Context, c identity.Caller, pageID uuid.UUID, filename string, body io.Reader) (Attachment, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.AttachmentAdd, uuid.Nil); err != nil {
		return Attachment{}, err
	}
	name, err := cleanFilename(filename)
	if err != nil {
		return Attachment{}, err
	}
	limit, err := s.settings.MaxUploadBytes(ctx)
	if err != nil {
		return Attachment{}, err
	}
	reader := bufio.NewReaderSize(body, 512)
	head, err := reader.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) {
		return Attachment{}, err
	}
	contentType, _, _ := strings.Cut(http.DetectContentType(head), ";")

	key := "attachments/" + ids.New().String()
	obj, err := s.store.Put(ctx, key, reader, limit)
	if errors.Is(err, storage.ErrTooLarge) {
		return Attachment{}, apperr.TooLarge("The file is larger than the upload limit.")
	}
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
		var id uuid.UUID
		version := 1
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
		if _, err := tx.Exec(ctx, `INSERT INTO attachment_versions (id, attachment_id, number, object_key, sha256, size, content_type, uploaded_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, ids.New(), id, version, key, obj.SHA256, obj.Size, contentType, c.UserID); err != nil {
			return err
		}
		if err := s.reindexNames(ctx, tx, pageID); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("attachment.uploaded", "attachment", id.String(), spaceID.String(), map[string]any{
			"page_id": pageID, "filename": name, "version": version, "size": obj.Size, "content_type": contentType})); err != nil {
			return err
		}
		if _, err := outbox.Enqueue(ctx, tx, "attachment.uploaded", map[string]any{"attachment_id": id, "page_id": pageID, "space_id": spaceID}); err != nil {
			return err
		}
		a, err = get(ctx, tx, id)
		return err
	})
	if err != nil {
		_ = s.store.Delete(context.WithoutCancel(ctx), key)
		return Attachment{}, err
	}
	return a, nil
}

const selectSQL = `SELECT a.id, a.page_id, a.space_id, a.filename, a.current_version, v.size, v.content_type, a.created_by, a.updated_at
	FROM attachments a JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version`

func scan(r pgx.Row) (Attachment, error) {
	var a Attachment
	err := r.Scan(&a.ID, &a.PageID, &a.SpaceID, &a.Filename, &a.Version, &a.Size, &a.ContentType, &a.CreatedBy, &a.UpdatedAt)
	a.Inline = IsInline(a.ContentType)
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
		item, err := trash.Add(ctx, tx, trash.Item{SpaceID: a.SpaceID, ObjectType: trash.TypeAttachment, ObjectID: id,
			Title: a.Filename, DeletedBy: c.UserID}, retention)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE attachments SET status = 'trashed', trash_item_id = $2, updated_at = now() WHERE id = $1", id, item); err != nil {
			return err
		}
		if err := s.reindexNames(ctx, tx, a.PageID); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event("attachment.trashed", "attachment", id.String(), a.SpaceID.String(),
			map[string]any{"page_id": a.PageID, "filename": a.Filename})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "attachment.trashed", map[string]any{"attachment_id": id, "page_id": a.PageID})
		return err
	})
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
	_, err = outbox.Enqueue(ctx, tx, "attachment.restored", map[string]any{"attachment_id": it.ObjectID, "page_id": pageID})
	return err
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
