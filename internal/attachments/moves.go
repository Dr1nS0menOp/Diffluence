// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

func nameTaken(ctx context.Context, q db.Querier, pageID uuid.UUID, name string) (bool, error) {
	var taken bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM attachments WHERE page_id = $1
		AND status = 'active' AND lower(filename) = lower($2))`, pageID, name).Scan(&taken)
	return taken, err
}

func errNameTaken() error {
	return apperr.Conflict("name_taken", "This page already has a file with that name.")
}

func errSamePage() error {
	return apperr.Invalid("The file is already on that page.")
}

// Move puts a file on another page. It needs delete on the source and add on the target.
// The id stays, so tiles keep working, and from now on the rules of the new page decide
// who gets the file.
func (s *Service) Move(ctx context.Context, c identity.Caller, id, target uuid.UUID) (Attachment, error) {
	var out Attachment
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		a, err := get(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, a.PageID, permissions.ActionAttachmentDelete, a.CreatedBy); err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, target, permissions.AttachmentAdd, uuid.Nil); err != nil {
			return err
		}
		if target == a.PageID {
			return errSamePage()
		}
		taken, err := nameTaken(ctx, tx, target, a.Filename)
		if err != nil {
			return err
		}
		if taken {
			return errNameTaken()
		}
		spaceID, err := s.pageSpace(ctx, tx, target)
		if err != nil {
			return err
		}
		if spaceID != a.SpaceID {
			var size int64
			if err := tx.QueryRow(ctx, "SELECT COALESCE(sum(size), 0)::bigint FROM attachment_versions WHERE attachment_id = $1", id).Scan(&size); err != nil {
				return err
			}
			if err := requireRoom(ctx, tx, spaceID, size); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE attachments SET page_id = $2, space_id = $3, updated_at = now() WHERE id = $1", id, target, spaceID); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, c, a.PageID, id, a.Filename, eventMovedAway); err != nil {
			return err
		}
		if err := addEvent(ctx, tx, c, target, id, a.Filename, eventMovedHere); err != nil {
			return err
		}
		if err := s.reindexNames(ctx, tx, a.PageID); err != nil {
			return err
		}
		if err := s.reindexNames(ctx, tx, target); err != nil {
			return err
		}
		if err := record(ctx, tx, c, "attachment.moved", a, map[string]any{"to_page": target, "to_space": spaceID}); err != nil {
			return err
		}
		out, err = get(ctx, tx, id)
		return err
	})
	return out, err
}

// Copy adds the current version of a file to another page, as a file of its own: deleting
// one leaves the other. It needs view on the source and add on the target, and goes
// through Upload, so a limit or a blocked type counts for a copy too.
func (s *Service) Copy(ctx context.Context, c identity.Caller, id, target uuid.UUID) (Attachment, error) {
	a, err := s.viewable(ctx, c, id)
	if err != nil {
		return Attachment{}, err
	}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, target, permissions.AttachmentAdd, uuid.Nil); err != nil {
		return Attachment{}, err
	}
	if target == a.PageID {
		return Attachment{}, errSamePage()
	}
	taken, err := nameTaken(ctx, s.pool, target, a.Filename)
	if err != nil {
		return Attachment{}, err
	}
	if taken {
		return Attachment{}, errNameTaken()
	}
	f, err := s.OpenUnchecked(ctx, a)
	if err != nil {
		return Attachment{}, err
	}
	defer func() { _ = f.Close() }()
	return s.Upload(ctx, c, target, a.Filename, f)
}
