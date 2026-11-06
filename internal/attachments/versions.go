// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	maxTextPreview = 1 << 20
	maxCommentRune = 500
	maxLookupIDs   = 500
	filesPageSize  = 50
	largestShown   = 10
)

// mediaTypes are the video and audio types the browser may play, detected from the
// content. HTML named .mp4 is never one of them (spec 012, D16).
var mediaTypes = map[string]bool{"video/mp4": true, "video/webm": true, "audio/mpeg": true,
	"audio/wave": true, "application/ogg": true}

// IsMedia reports whether a detected type may play in the browser.
func IsMedia(contentType string) bool { return mediaTypes[contentType] }

// Version is one stored version of a file.
type Version struct {
	Number     int       `json:"number"`
	Size       int64     `json:"size"`
	UploadedBy string    `json:"uploaded_by"`
	CreatedAt  time.Time `json:"created_at"`
	Current    bool      `json:"current"`
}

func (s *Service) viewable(ctx context.Context, c identity.Caller, id uuid.UUID) (Attachment, error) {
	a, err := get(ctx, s.pool, id)
	if err != nil {
		return a, err
	}
	return a, s.perms.RequirePage(ctx, s.pool, c.UserID, a.PageID, permissions.ActionPageView, uuid.Nil)
}

// Versions lists every version of a file, newest first.
func (s *Service) Versions(ctx context.Context, c identity.Caller, id uuid.UUID) ([]Version, error) {
	a, err := s.viewable(ctx, c, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT v.number, v.size, COALESCE(u.display_name, ''), v.created_at
		FROM attachment_versions v LEFT JOIN users u ON u.id = v.uploaded_by
		WHERE v.attachment_id = $1 ORDER BY v.number DESC`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Version, error) {
		var v Version
		err := r.Scan(&v.Number, &v.Size, &v.UploadedBy, &v.CreatedAt)
		v.Current = v.Number == a.Version
		return v, err
	})
}

func record(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, a Attachment, details map[string]any) error {
	details["page_id"], details["filename"] = a.PageID, a.Filename
	if _, err := audit.Record(ctx, tx, c.Event(action, "attachment", a.ID.String(), a.SpaceID.String(), details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"attachment_id": a.ID, "page_id": a.PageID, "space_id": a.SpaceID})
	return err
}

// MakeCurrent adds a new version with the content of an older one. Nothing is lost.
func (s *Service) MakeCurrent(ctx context.Context, c identity.Caller, id uuid.UUID, number int) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.AttachmentAdd, uuid.Nil); err != nil {
			return err
		}
		next := a.Version + 1
		tag, err := tx.Exec(ctx, `INSERT INTO attachment_versions (id, attachment_id, number, object_key, sha256, size, content_type, uploaded_by)
			SELECT $1, attachment_id, $2, object_key, sha256, size, content_type, $3 FROM attachment_versions
			WHERE attachment_id = $4 AND number = $5`, ids.New(), next, c.UserID, id, number)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		if _, err := tx.Exec(ctx, "UPDATE attachments SET current_version = $2, updated_at = now() WHERE id = $1", id, next); err != nil {
			return err
		}
		return record(ctx, tx, c, "attachment.version_restored", a, map[string]any{"from": number, "version": next})
	})
}

// DeleteVersion removes an old version. It needs delete any, and never takes the current
// one. A version made current again shares its stored file, which then stays.
func (s *Service) DeleteVersion(ctx context.Context, c identity.Caller, id uuid.UUID, number int) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.AttachmentDeleteAny, uuid.Nil); err != nil {
			return err
		}
		if number == a.Version {
			return apperr.Conflict("current_version", "The current version cannot be removed.")
		}
		var key string
		err = tx.QueryRow(ctx, "DELETE FROM attachment_versions WHERE attachment_id = $1 AND number = $2 RETURNING object_key", id, number).Scan(&key)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO storage_deletions (object_key)
			SELECT $1 WHERE NOT EXISTS (SELECT 1 FROM attachment_versions WHERE object_key = $1)
			ON CONFLICT DO NOTHING`, key); err != nil {
			return err
		}
		return record(ctx, tx, c, "attachment.version_removed", a, map[string]any{"version": number})
	})
}

// Change is what a person can change about a file.
type Change struct {
	Filename string `json:"filename"`
	Comment  string `json:"comment"`
}

// Update renames a file and sets its comment. Tiles hold the id, so they keep working.
func (s *Service) Update(ctx context.Context, c identity.Caller, id uuid.UUID, in Change) (Attachment, error) {
	name, err := cleanFilename(in.Filename)
	if err != nil {
		return Attachment{}, err
	}
	if utf8.RuneCountInString(in.Comment) > maxCommentRune {
		return Attachment{}, apperr.Invalid("A comment on a file can have at most 500 characters.")
	}
	var out Attachment
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.AttachmentAdd, uuid.Nil); err != nil {
			return err
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments WHERE page_id = $1 AND id <> $2
			AND status = 'active' AND lower(filename) = lower($3))`, a.PageID, id, name).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return apperr.Conflict("name_taken", "This page already has a file with that name.")
		}
		if _, err := tx.Exec(ctx, "UPDATE attachments SET filename = $2, comment = $3, updated_at = now() WHERE id = $1", id, name, in.Comment); err != nil {
			return err
		}
		if err := s.reindexNames(ctx, tx, a.PageID); err != nil {
			return err
		}
		if err := record(ctx, tx, c, "attachment.updated", a, map[string]any{"new_name": name}); err != nil {
			return err
		}
		out, err = get(ctx, tx, id)
		return err
	})
	return out, err
}

// NameTaken reports whether a page has a file with this name, for the question before
// an upload. The upload checks its own rights.
func (s *Service) NameTaken(ctx context.Context, c identity.Caller, pageID uuid.UUID, filename string) (bool, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.AttachmentAdd, uuid.Nil); err != nil {
		return false, err
	}
	name, err := cleanFilename(filename)
	if err != nil {
		return false, err
	}
	var taken bool
	err = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments WHERE page_id = $1
		AND status = 'active' AND lower(filename) = lower($2))`, pageID, name).Scan(&taken)
	return taken, err
}

// Text is a file as plain text, for a preview. It is data, never a document.
type Text struct {
	Filename string `json:"filename"`
	Text     string `json:"text"`
	Cut      bool   `json:"cut"`
}

// PreviewText reads a text file, up to the limit. Other files have no text preview.
func (s *Service) PreviewText(ctx context.Context, c identity.Caller, id uuid.UUID) (Text, error) {
	a, err := s.viewable(ctx, c, id)
	if err != nil {
		return Text{}, err
	}
	if !strings.HasPrefix(a.ContentType, "text/") {
		return Text{}, apperr.Conflict("no_preview", "This file has no text preview.")
	}
	f, err := s.OpenUnchecked(ctx, a)
	if err != nil {
		return Text{}, err
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxTextPreview+1))
	if err != nil {
		return Text{}, err
	}
	cut := len(raw) > maxTextPreview
	if cut {
		raw = raw[:maxTextPreview]
	}
	return Text{Filename: a.Filename, Text: strings.ToValidUTF8(string(raw), ""), Cut: cut}, nil
}

// Facts are what a tile shows. An id that is hidden or gone is simply left out, so
// the two cannot be told apart.
type Facts struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type"`
	Inline      bool      `json:"inline"`
	Media       bool      `json:"media"`
}

// Lookup gives the live facts of files on pages the caller may see.
func (s *Service) Lookup(ctx context.Context, c identity.Caller, list []uuid.UUID) ([]Facts, error) {
	if len(list) > maxLookupIDs {
		return nil, apperr.Invalid("Too many files at once.")
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id, a.filename, v.size, v.content_type FROM attachments a
		JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version
		JOIN perm_visible_pages($1) p ON p.page_id = a.page_id
		WHERE a.id = ANY($2) AND a.status = 'active'`, c.UserID, list)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Facts, error) {
		var f Facts
		err := r.Scan(&f.ID, &f.Filename, &f.Size, &f.ContentType)
		f.Inline, f.Media = IsInline(f.ContentType), IsMedia(f.ContentType)
		return f, err
	})
	if out == nil {
		out = []Facts{}
	}
	return out, err
}

// SpaceFile is one row of the Files screen.
type SpaceFile struct {
	ID        uuid.UUID `json:"id"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	PageID    uuid.UUID `json:"page_id"`
	PageTitle string    `json:"page_title"`
	Uploader  string    `json:"uploader"`
	UpdatedAt time.Time `json:"updated_at"`
}

func scanSpaceFile(r pgx.CollectableRow) (SpaceFile, error) {
	var f SpaceFile
	err := r.Scan(&f.ID, &f.Filename, &f.Size, &f.PageID, &f.PageTitle, &f.Uploader, &f.UpdatedAt)
	return f, err
}

const spaceFileColumns = `a.id, a.filename, v.size, a.page_id, p.title, COALESCE(u.display_name, ''), a.updated_at`

// SpaceFiles lists the files of a space that the caller may see (FIL-043).
func (s *Service) SpaceFiles(ctx context.Context, c identity.Caller, spaceID uuid.UUID, name string, offset int) ([]SpaceFile, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+spaceFileColumns+` FROM attachments a
		JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version
		JOIN perm_visible_pages($1) vp ON vp.page_id = a.page_id
		JOIN pages p ON p.id = a.page_id
		LEFT JOIN users u ON u.id = a.created_by
		WHERE a.space_id = $2 AND a.status = 'active' AND ($3 = '' OR position(lower($3) IN lower(a.filename)) > 0)
		ORDER BY a.updated_at DESC, a.id LIMIT $4 OFFSET $5`, c.UserID, spaceID, name, filesPageSize, max(offset, 0))
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scanSpaceFile)
	if out == nil {
		out = []SpaceFile{}
	}
	return out, err
}

// Usage is how much a space stores. Only space administrators see it, because a
// total would tell how much is hidden (spec 012, E12).
type Usage struct {
	Bytes   int64       `json:"bytes"`
	Largest []SpaceFile `json:"largest"`
}

// SpaceUsage adds up the storage of a space.
func (s *Service) SpaceUsage(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (Usage, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return Usage{}, err
	}
	var out Usage
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(v.size), 0) FROM attachments a
		JOIN attachment_versions v ON v.attachment_id = a.id WHERE a.space_id = $1 AND a.status = 'active'`, spaceID).Scan(&out.Bytes)
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+spaceFileColumns+` FROM attachments a
		JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version
		JOIN perm_visible_pages($1) vp ON vp.page_id = a.page_id
		JOIN pages p ON p.id = a.page_id
		LEFT JOIN users u ON u.id = a.created_by
		WHERE a.space_id = $2 AND a.status = 'active' ORDER BY v.size DESC, a.id LIMIT $3`, c.UserID, spaceID, largestShown)
	if err != nil {
		return out, err
	}
	out.Largest, err = pgx.CollectRows(rows, scanSpaceFile)
	if out.Largest == nil {
		out.Largest = []SpaceFile{}
	}
	return out, err
}

// WriteZip streams all files of a page into one zip, with one audit event.
func (s *Service) WriteZip(ctx context.Context, c identity.Caller, pageID uuid.UUID, w io.Writer) error {
	list, err := s.List(ctx, c, pageID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return apperr.NotFound()
	}
	if err := audit.RecordStandalone(ctx, s.pool, c.Event("attachment.zip_downloaded", "page", pageID.String(), list[0].SpaceID.String(),
		map[string]any{"files": len(list)})); err != nil {
		return err
	}
	archive := zip.NewWriter(w)
	for _, a := range list {
		if err := s.addToZip(ctx, archive, a); err != nil {
			return err
		}
	}
	return archive.Close()
}

func (s *Service) addToZip(ctx context.Context, archive *zip.Writer, a Attachment) error {
	f, err := s.OpenUnchecked(ctx, a)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	entry, err := archive.Create(a.Filename)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, f)
	return err
}
