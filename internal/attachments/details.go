// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/document"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/ids"
	"github.com/kenwerk/kenwerk/internal/labels"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/storage"
)

const (
	maxLabelsPerFile = 50
	maxListedEvents  = 200
	maxBulkFiles     = 200
)

// What happened to a file, as the history of its page tells it.
const (
	eventAdded     = "added"
	eventVersion   = "version"
	eventRemoved   = "removed"
	eventRestored  = "restored"
	eventMovedHere = "moved_here"
	eventMovedAway = "moved_away"
)

// heldSQL says whether anything still needs a file: any version of any page, because a
// moved file leaves its tiles behind, and on its own page a draft, a comment or a header.
const heldSQL = `(EXISTS (SELECT 1 FROM attachment_refs r WHERE r.attachment_id = a.id)
	OR EXISTS (SELECT 1 FROM drafts d WHERE d.page_id = a.page_id AND d.document::text LIKE '%' || a.id::text || '%')
	OR EXISTS (SELECT 1 FROM comments m WHERE m.page_id = a.page_id
		AND (m.body::text LIKE '%' || a.id::text || '%' OR COALESCE(m.anchor::text, '') LIKE '%' || a.id::text || '%'))
	OR EXISTS (SELECT 1 FROM pages h WHERE h.header_attachment_id = a.id))`

// clearableSQL leaves young files alone: an editor may have one that it has not saved yet.
const clearableSQL = `(a.created_at < now() - interval '1 day' AND NOT ` + heldSQL + `)`

const inUseSQL = `EXISTS (SELECT 1 FROM attachment_refs r JOIN pages p ON p.id = r.page_id AND p.current_version = r.version
	WHERE r.attachment_id = a.id AND r.page_id = a.page_id)`

// Row is a file in the list of a page.
type Row struct {
	Attachment
	Comment   string    `json:"comment"`
	AddedBy   string    `json:"added_by"`
	CreatedAt time.Time `json:"created_at"`
	Labels    []string  `json:"labels"`
	// InUse says the current version of the page shows the file.
	InUse bool `json:"in_use"`
	// Clearable says nothing needs the file any more (FIL-017).
	Clearable bool `json:"clearable"`
}

// Rows lists the files of a page with what the list shows about each.
func (s *Service) Rows(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Row, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+selectColumns+`, a.comment, a.created_at, COALESCE(u.display_name, ''),
			`+labelsSQL+`, `+inUseSQL+`, `+clearableSQL+`
		`+selectFrom+` LEFT JOIN users u ON u.id = a.created_by
		WHERE a.page_id = $1 AND a.status = 'active' ORDER BY lower(a.filename)`, pageID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Row, error) {
		var v Row
		err := r.Scan(&v.ID, &v.PageID, &v.SpaceID, &v.Filename, &v.Version, &v.Size, &v.ContentType, &v.CreatedBy, &v.UpdatedAt, &v.SHA256,
			&v.Comment, &v.CreatedAt, &v.AddedBy, &v.Labels, &v.InUse, &v.Clearable)
		v.Inline, v.Office = IsInline(v.ContentType), IsOffice(v.Filename)
		return v, err
	})
	if err != nil {
		return nil, err
	}
	p, err := s.pinsFor(ctx, s.pool, c, pageID)
	if err != nil {
		return nil, err
	}
	if out, err = keepPinned(ctx, s.pool, out, p, func(v *Row) *Attachment { return &v.Attachment }); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Row{}
	}
	return out, nil
}

// RecordUse writes which files a new version shows, in the transaction that makes it.
func RecordUse(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, version int, doc document.Node) error {
	shown := document.FilesShown(doc)
	if len(shown) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `INSERT INTO attachment_refs (attachment_id, page_id, version)
		SELECT unnest($1::uuid[]), $2, $3 ON CONFLICT DO NOTHING`, shown, pageID, version)
	return err
}

// Outcome says what became of a list of files. Hidden, unknown and refused files all
// count as skipped, so the answer cannot tell them apart.
type Outcome struct {
	Deleted int `json:"deleted"`
	Skipped int `json:"skipped"`
}

// trashAll sends to the trash what the caller may delete and belongs. One transaction,
// so a failure leaves all files as they were.
func (s *Service) trashAll(ctx context.Context, c identity.Caller, list []uuid.UUID, belongs func(Attachment) bool) (Outcome, error) {
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return Outcome{}, err
	}
	var out Outcome
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		out = Outcome{}
		for _, id := range list {
			a, err := get(ctx, tx, id)
			if err == nil && belongs(a) {
				err = s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.ActionAttachmentDelete, a.CreatedBy)
				if err == nil {
					err = requireNotPinned(ctx, tx, a, 0)
				}
			} else if err == nil {
				err = apperr.NotFound()
			}
			if _, refused := apperr.As(err); refused {
				out.Skipped++
				continue
			}
			if err != nil {
				return err
			}
			if err := s.trashOne(ctx, tx, c, a, retention); err != nil {
				return err
			}
			out.Deleted++
		}
		return nil
	})
	return out, err
}

// ClearUp sends to the trash what nothing needs any more, as far as the caller may delete
// it (FIL-017).
func (s *Service) ClearUp(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Outcome, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return Outcome{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT a.id FROM attachments a
		WHERE a.page_id = $1 AND a.status = 'active' AND `+clearableSQL, pageID)
	if err != nil {
		return Outcome{}, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return Outcome{}, err
	}
	return s.trashAll(ctx, c, list, func(a Attachment) bool { return a.PageID == pageID })
}

// TrashFiles deletes many files of a space at once (FIL-045). The right on the space is
// not enough: each page still decides about its own files.
func (s *Service) TrashFiles(ctx context.Context, c identity.Caller, spaceID uuid.UUID, list []uuid.UUID) (Outcome, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return Outcome{}, err
	}
	if len(list) > maxBulkFiles {
		return Outcome{}, apperr.Invalid("Delete at most 200 files at once.")
	}
	return s.trashAll(ctx, c, list, func(a Attachment) bool { return a.SpaceID == spaceID })
}

func addEvent(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID, id uuid.UUID, filename, action string) error {
	_, err := tx.Exec(ctx, `INSERT INTO attachment_events (id, page_id, attachment_id, filename, action, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6)`, ids.New(), pageID, id, filename, action, c.UserID)
	return err
}

// Event is one line of what happened to the files of a page. A move names no other page:
// the reader of this one may not see it.
type Event struct {
	Filename string    `json:"filename"`
	Action   string    `json:"action"`
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
}

// Events lists what happened to the files of a page, newest first (FIL-050).
func (s *Service) Events(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Event, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	p, err := s.pinsFor(ctx, s.pool, c, pageID)
	if err != nil {
		return nil, err
	}
	// A reader of a controlled page hears only of the files the approved version holds, up to
	// when it was written: what happened since belongs to the working version (DOC-512).
	var pinnedIDs []uuid.UUID
	var until *time.Time
	if p != nil {
		pinnedIDs = make([]uuid.UUID, 0, len(p.files))
		for id := range p.files {
			pinnedIDs = append(pinnedIDs, id)
		}
		until = &p.at
	}
	rows, err := s.pool.Query(ctx, `SELECT e.filename, e.action, COALESCE(u.display_name, ''), e.created_at
		FROM attachment_events e LEFT JOIN users u ON u.id = e.actor_id
		WHERE e.page_id = $1 AND ($3::timestamptz IS NULL OR (e.attachment_id = ANY($4) AND e.created_at <= $3))
		ORDER BY e.created_at DESC, e.id DESC LIMIT $2`, pageID, maxListedEvents, until, pinnedIDs)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Event, error) {
		var v Event
		err := r.Scan(&v.Filename, &v.Action, &v.Actor, &v.At)
		return v, err
	})
	if out == nil {
		out = []Event{}
	}
	return out, err
}

func labelsOf(ctx context.Context, q db.Querier, id uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, "SELECT label FROM attachment_labels WHERE attachment_id = $1 ORDER BY label", id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

// changeable finds a file inside tx, for someone who may change it.
func (s *Service) changeable(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID) (Attachment, error) {
	a, err := get(ctx, tx, id)
	if err != nil {
		return a, err
	}
	return a, s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.AttachmentAdd, uuid.Nil)
}

// AddLabel puts a label on a file, by the rules of labels on pages.
func (s *Service) AddLabel(ctx context.Context, c identity.Caller, id uuid.UUID, raw string) ([]string, error) {
	var out []string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := s.changeable(ctx, tx, c, id)
		if err != nil {
			return err
		}
		label, err := labels.Clean(raw)
		if err != nil {
			return err
		}
		refused, err := labels.Refused(ctx, tx, a.SpaceID, label)
		if err != nil {
			return err
		}
		if refused {
			return apperr.Invalid("This space has a fixed list of labels.")
		}
		if out, err = labelsOf(ctx, tx, id); err != nil {
			return err
		}
		if len(out) >= maxLabelsPerFile {
			return apperr.Invalid("A file can have at most 50 labels.")
		}
		tag, err := tx.Exec(ctx, `INSERT INTO attachment_labels (attachment_id, label, created_by)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, label, c.UserID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		if err := record(ctx, tx, c, "attachment.labelled", a, map[string]any{"label": label}); err != nil {
			return err
		}
		out, err = labelsOf(ctx, tx, id)
		return err
	})
	return out, err
}

// RemoveLabel takes a label off a file.
func (s *Service) RemoveLabel(ctx context.Context, c identity.Caller, id uuid.UUID, label string) ([]string, error) {
	var out []string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := s.changeable(ctx, tx, c, id)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM attachment_labels WHERE attachment_id = $1 AND label = $2", id, strings.ToLower(label))
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			if err := record(ctx, tx, c, "attachment.unlabelled", a, map[string]any{"label": label}); err != nil {
				return err
			}
		}
		out, err = labelsOf(ctx, tx, id)
		return err
	})
	return out, err
}

// OpenVersion reads one stored version. It checks view on every request, so a link to a
// version gives nothing to someone without access (FIL-042).
func (s *Service) OpenVersion(ctx context.Context, c identity.Caller, id uuid.UUID, number int) (Attachment, io.ReadSeekCloser, error) {
	a, pinnedVersion, err := s.viewablePinned(ctx, c, id)
	if err != nil {
		return a, nil, err
	}
	if pinnedVersion && number != a.Version {
		// A reader of a controlled page has the approved version of a file, and no other.
		return a, nil, apperr.NotFound()
	}
	var key string
	err = s.pool.QueryRow(ctx, `SELECT object_key, size, content_type, encode(sha256, 'hex') FROM attachment_versions
		WHERE attachment_id = $1 AND number = $2`, id, number).Scan(&key, &a.Size, &a.ContentType, &a.SHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, nil, apperr.NotFound()
	}
	if err != nil {
		return a, nil, err
	}
	a.Version = number
	f, err := s.store.Open(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return a, nil, apperr.NotFound()
	}
	if err != nil {
		return a, nil, err
	}
	if err := audit.RecordStandalone(ctx, s.pool, c.Event("attachment.downloaded", "attachment", id.String(), a.SpaceID.String(),
		map[string]any{"page_id": a.PageID, "filename": a.Filename, "version": number})); err != nil {
		_ = f.Close()
		return a, nil, err
	}
	return a, f, nil
}
