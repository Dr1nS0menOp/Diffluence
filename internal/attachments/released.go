// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

// ReleaseCopy says which file of a draft page goes onto a twin page (spec 073, PUB-320).
type ReleaseCopy struct {
	// From is the file of the draft space.
	From uuid.UUID
	// Copy is the copy a release made of it before, or nil the first time.
	Copy *uuid.UUID
	// TwinPage and SpaceID are the twin and the published space.
	TwinPage, SpaceID uuid.UUID
}

// ReleaseSource is the version of a draft file a release would copy.
type ReleaseSource struct {
	Version int
	Size    int64
	SpaceID uuid.UUID
}

// SourcesForRelease reads the current version and size of the active files among list, by
// id. A file that is gone or in the trash is left out.
func SourcesForRelease(ctx context.Context, q db.Querier, list []uuid.UUID) (map[uuid.UUID]ReleaseSource, error) {
	rows, err := q.Query(ctx, `SELECT a.id, a.current_version, v.size, a.space_id `+selectFrom+`
		WHERE a.id = ANY($1) AND a.status = 'active'`, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]ReleaseSource{}
	for rows.Next() {
		var id uuid.UUID
		var src ReleaseSource
		if err := rows.Scan(&id, &src.Version, &src.Size, &src.SpaceID); err != nil {
			return nil, err
		}
		out[id] = src
	}
	return out, rows.Err()
}

// CountInSpace counts the files of a space that are not in the trash.
func CountInSpace(ctx context.Context, q db.Querier, spaceID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM attachments WHERE space_id = $1 AND status = 'active'`, spaceID).Scan(&n)
	return n, err
}

// CopyForRelease stores the current version of a draft file as a separate file of the twin,
// so purging in the draft can never take a reader's file away, and a reader's download can
// never return a draft file (PUB-320, PUB-624). The first time the twin gets a new file with
// only that version, and a comment on the file goes with it (PUB-321); after that, the earlier
// copy gets it as its next version, so links to the copy stay. Old versions are never copied.
// It counts against the published space's room (PUB-1419). The caller has checked the rights
// and marked its transaction as a release; it returns the copy and the version it holds.
func (s *Service) CopyForRelease(ctx context.Context, tx pgx.Tx, c identity.Caller, in ReleaseCopy) (uuid.UUID, int, error) {
	var filename, comment, key, contentType string
	var version int
	var size int64
	err := tx.QueryRow(ctx, `SELECT a.filename, a.comment, a.current_version, v.object_key, v.content_type, v.size `+selectFrom+`
		WHERE a.id = $1 AND a.status = 'active'`, in.From).Scan(&filename, &comment, &version, &key, &contentType, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, apperr.NotFound()
	}
	if err != nil {
		return uuid.Nil, 0, err
	}
	if err := requireRoom(ctx, tx, in.SpaceID, size); err != nil {
		return uuid.Nil, 0, err
	}
	f, err := s.store.Open(ctx, key)
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf("attachments: open a file to release: %w", err)
	}
	newKey := "attachments/" + ids.New().String()
	limit := int64(1) << 42 // stored once already; the upload limit does not apply again
	obj, err := s.store.Put(ctx, newKey, f, limit)
	_ = f.Close()
	if err != nil {
		return uuid.Nil, 0, fmt.Errorf("attachments: store a released copy: %w", err)
	}
	id, number := ids.New(), 1
	if in.Copy == nil {
		_, err = tx.Exec(ctx, `INSERT INTO attachments (id, origin_instance_id, space_id, page_id, filename, status, created_by, comment)
			VALUES ($1, $2, $3, $4, $5, 'active', $6, $7)`, id, s.instanceID, in.SpaceID, in.TwinPage, filename, c.UserID, comment)
	} else {
		id = *in.Copy
		err = tx.QueryRow(ctx, `UPDATE attachments SET current_version = current_version + 1, filename = $2, comment = $3,
			status = 'active', trash_item_id = NULL, updated_at = now() WHERE id = $1 RETURNING current_version`, id, filename, comment).Scan(&number)
	}
	if db.IsUniqueViolation(err) {
		return uuid.Nil, 0, apperr.Conflict("file_name_taken", "Two files of this page have the same name. Rename one in the draft space and release again.")
	}
	if err != nil {
		return uuid.Nil, 0, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO attachment_versions (id, attachment_id, number, object_key, sha256, size, content_type, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, ids.New(), id, number, newKey, obj.SHA256, obj.Size, contentType, c.UserID); err != nil {
		return uuid.Nil, 0, err
	}
	if err := s.reindexNames(ctx, tx, in.TwinPage); err != nil {
		return uuid.Nil, 0, err
	}
	// The published space's audit names the copy and its page only (PUB-901, PUB-923).
	if _, err := audit.Record(ctx, tx, c.Event("attachment.released", "attachment", id.String(), in.SpaceID.String(),
		map[string]any{"page_id": in.TwinPage, "version": number, "size": obj.Size})); err != nil {
		return uuid.Nil, 0, err
	}
	return id, version, nil
}

// TrashForRelease sends a copy that the released document no longer shows to the published
// space's trash (PUB-320). The caller has checked the rights and marked its transaction.
func (s *Service) TrashForRelease(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID) error {
	a, err := get(ctx, tx, id)
	if apperr.IsNotFound(err) {
		return nil // already gone, by hand in an ended pair or by the trash
	}
	if err != nil {
		return err
	}
	retention, err := s.settings.TrashRetention(ctx)
	if err != nil {
		return err
	}
	return s.trashOne(ctx, tx, c, a, retention)
}
