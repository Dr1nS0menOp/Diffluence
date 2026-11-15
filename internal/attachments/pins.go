// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
)

// Document control (spec 064, wave DC3). Each version of a controlled page pins the files it
// shows, at the version and under the name they had when it was written. Readers of the page
// get the pins of the version they read, so an approved procedure keeps its approved form
// after an author uploads a new one (DOC-510). Whoever may see the working version gets the
// files as they are.

// ContentChanges is told, inside the transaction of the change, that a file got a new
// version, an older version back, or a new name. A controlled page whose working version
// shows the file writes a new version and starts its review again (DOC-511). It is defined
// here, where it is used, because pages imports attachments.
type ContentChanges interface {
	FileChanged(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID, attachmentID uuid.UUID) error
}

// SetContentChanges connects the pages module. It is set once while the application is
// wired, before any request: pages is made after attachments, so it cannot be passed in.
func (s *Service) SetContentChanges(cc ContentChanges) { s.changes = cc }

// fileChanged tells the pages module about a change of a file.
func (s *Service) fileChanged(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID, id uuid.UUID) error {
	if s.changes == nil {
		return nil
	}
	return s.changes.FileChanged(ctx, tx, c, pageID, id)
}

// Pin records the files a version of a controlled page shows, from what RecordUse wrote for
// it. A version that was pinned once keeps its pins.
func Pin(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, version int) error {
	_, err := tx.Exec(ctx, `INSERT INTO attachment_pins (page_id, page_version, attachment_ids, versions, filenames)
		SELECT $1, $2, COALESCE(array_agg(a.id ORDER BY a.id), '{}'), COALESCE(array_agg(a.current_version ORDER BY a.id), '{}'),
			COALESCE(array_agg(a.filename ORDER BY a.id), '{}')
		FROM attachments a JOIN attachment_refs r ON r.attachment_id = a.id AND r.page_id = $1 AND r.version = $2
		WHERE a.page_id = $1 AND a.status = 'active'
		ON CONFLICT (page_id, page_version) DO NOTHING`, pageID, version)
	return err
}

// SamePins reports whether two versions of a page pin the same files at the same versions
// and names, so a version with an unchanged text but a new file is a change (DOC-511).
func SamePins(ctx context.Context, q db.Querier, pageID uuid.UUID, a, b int) (bool, error) {
	var same bool
	err := q.QueryRow(ctx, `SELECT (SELECT ROW(attachment_ids, versions, filenames) FROM attachment_pins WHERE page_id = $1 AND page_version = $2)
		IS NOT DISTINCT FROM (SELECT ROW(attachment_ids, versions, filenames) FROM attachment_pins WHERE page_id = $1 AND page_version = $3)`,
		pageID, a, b).Scan(&same)
	return same, err
}

// Shows reports whether a version of a page shows a file.
func Shows(ctx context.Context, q db.Querier, pageID uuid.UUID, version int, id uuid.UUID) (bool, error) {
	var shown bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachment_refs WHERE page_id = $1 AND version = $2 AND attachment_id = $3)`,
		pageID, version, id).Scan(&shown)
	return shown, err
}

// pin is what readers get of one file.
type pin struct {
	version  int
	filename string
}

// pinned is the files readers of a page get, with when they were pinned. A nil value
// means the files as they are.
type pinned struct {
	files map[uuid.UUID]pin
	at    time.Time
}

// readerPins reads the pins of the version readers see, or nil when that version pins
// nothing.
func readerPins(ctx context.Context, q db.Querier, pageID uuid.UUID) (*pinned, error) {
	var at *time.Time
	if err := q.QueryRow(ctx, "SELECT reader_pinned_at($1)", pageID).Scan(&at); err != nil {
		return nil, err
	}
	if at == nil {
		return nil, nil
	}
	out := &pinned{files: map[uuid.UUID]pin{}, at: *at}
	rows, err := q.Query(ctx, "SELECT attachment_id, version, filename FROM reader_pins($1)", pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var p pin
		if err := rows.Scan(&id, &p.version, &p.filename); err != nil {
			return nil, err
		}
		out.files[id] = p
	}
	return out, rows.Err()
}

// pinsFor is what a person gets of the files of a page: nil when they get the files as they
// are, because the page pins nothing or they may see its working version (DOC-300).
func (s *Service) pinsFor(ctx context.Context, q db.Querier, c identity.Caller, pageID uuid.UUID) (*pinned, error) {
	p, err := readerPins(ctx, q, pageID)
	if err != nil || p == nil {
		return nil, err
	}
	working, err := s.perms.ReadsWorking(ctx, q, c.UserID, pageID)
	if err != nil || working {
		return nil, err
	}
	return p, nil
}

// atPin gives a file as readers get it: the pinned version and name, or not found when the
// version readers see does not show it.
func atPin(ctx context.Context, q db.Querier, a Attachment, p *pinned) (Attachment, error) {
	if p == nil {
		return a, nil
	}
	at, ok := p.files[a.ID]
	if !ok {
		return a, apperr.NotFound()
	}
	err := q.QueryRow(ctx, `SELECT size, content_type, encode(sha256, 'hex') FROM attachment_versions WHERE attachment_id = $1 AND number = $2`,
		a.ID, at.version).Scan(&a.Size, &a.ContentType, &a.SHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, apperr.NotFound()
	}
	a.Version, a.Filename = at.version, at.filename
	a.Inline, a.Office = IsInline(a.ContentType), IsOffice(a.Filename)
	return a, err
}

// keepPinned narrows a list of files to what readers get.
func keepPinned[T any](ctx context.Context, q db.Querier, list []T, p *pinned, of func(*T) *Attachment) ([]T, error) {
	if p == nil {
		return list, nil
	}
	out := list[:0:0]
	for i := range list {
		a := of(&list[i])
		pinnedFile, err := atPin(ctx, q, *a, p)
		if apperr.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		*a = pinnedFile
		out = append(out, list[i])
	}
	return out, nil
}

// ListForReaders lists the files of a page as its readers get them, for a module that shows
// a page to the world, such as a public site, or copies what readers see.
func (s *Service) ListForReaders(ctx context.Context, pageID uuid.UUID) ([]Attachment, error) {
	list, err := s.ListForPage(ctx, pageID)
	if err != nil {
		return nil, err
	}
	p, err := readerPins(ctx, s.pool, pageID)
	if err != nil {
		return nil, err
	}
	return keepPinned(ctx, s.pool, list, p, func(a *Attachment) *Attachment { return a })
}

// errPinned refuses to take away a file that readers have as part of the approved version.
func errPinned() error {
	return apperr.Conflict("file_approved", "This file is part of the approved version. Take it out of the page and have that approved first.")
}

// requireNotPinned refuses a removal of a file, or of one of its versions, that the version
// readers see holds (DOC-510). number 0 means the whole file.
func requireNotPinned(ctx context.Context, q db.Querier, a Attachment, number int) error {
	if err := requireNotSigned(ctx, q, a, number); err != nil {
		return err
	}
	p, err := readerPins(ctx, q, a.PageID)
	if err != nil || p == nil {
		return err
	}
	if at, ok := p.files[a.ID]; ok && (number == 0 || at.version == number) {
		return errPinned()
	}
	return nil
}

// requireNotSigned refuses a removal of a file, or of one of its versions, that a signed
// version of its page shows: it stays as long as the signature (DOC-770). number 0 means the
// whole file. The pages module owns signatures and answers through signed_file_versions.
func requireNotSigned(ctx context.Context, q db.Querier, a Attachment, number int) error {
	var signed bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM signed_file_versions($1) f WHERE f.attachment_id = $2 AND ($3 = 0 OR f.version = $3))`,
		a.PageID, a.ID, number).Scan(&signed); err != nil {
		return err
	}
	if signed {
		return apperr.Conflict("file_signed", "A signed version of the page shows this file. It is kept for the retention of signed records.")
	}
	return nil
}

// PinnedFile is one file a version of a page pins, with the SHA-256 of its pinned bytes.
type PinnedFile struct {
	ID       uuid.UUID
	Version  int
	Filename string
	SHA256   string
}

// PinnedFiles lists the files a version of a page pins, at their pinned version and name, with
// the hash of that version's bytes, for the content a signature covers (spec 064, DOC-711).
func PinnedFiles(ctx context.Context, q db.Querier, pageID uuid.UUID, version int) ([]PinnedFile, error) {
	rows, err := q.Query(ctx, `SELECT f.attachment_id, f.version, f.filename, COALESCE(encode(av.sha256, 'hex'), '')
		FROM attachment_pins pin
		CROSS JOIN LATERAL unnest(pin.attachment_ids, pin.versions, pin.filenames) AS f (attachment_id, version, filename)
		LEFT JOIN attachment_versions av ON av.attachment_id = f.attachment_id AND av.number = f.version
		WHERE pin.page_id = $1 AND pin.page_version = $2`, pageID, version)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (PinnedFile, error) {
		var f PinnedFile
		err := r.Scan(&f.ID, &f.Version, &f.Filename, &f.SHA256)
		return f, err
	})
}
