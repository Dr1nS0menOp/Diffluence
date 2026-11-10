// SPDX-License-Identifier: Apache-2.0

package attachments

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
)

// spaceBytesSQL is what a space stores. A limit counts the same number as the total on
// the Files screen, so a refusal never disagrees with what people see (spec 024, D10).
// Files in the trash still take room until they are purged, and a version made current
// again shares its stored object, so each object counts once (spec 047, D19).
const spaceBytesSQL = `(SELECT COALESCE(sum(stored.size), 0)::bigint FROM (SELECT DISTINCT v.object_key, v.size
	FROM attachments a JOIN attachment_versions v ON v.attachment_id = a.id WHERE a.space_id = $1) stored)`

const mebibyte = 1 << 20

func errSpaceFull() error {
	return apperr.TooLarge("This space has reached its storage limit.")
}

// room says how many bytes a space can still take, and whether it has a limit at all.
func room(ctx context.Context, q db.Querier, spaceID uuid.UUID) (left int64, limited bool, err error) {
	err = q.QueryRow(ctx, `SELECT storage_limit_mib::bigint * $2 - `+spaceBytesSQL+`, storage_limit_mib > 0
		FROM spaces WHERE id = $1`, spaceID, mebibyte).Scan(&left, &limited)
	return left, limited, err
}

// requireRoom is the check that counts. The lock makes two uploads at once take turns, so
// they cannot both take the last room (spec 024, D11).
func requireRoom(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, size int64) error {
	if _, err := tx.Exec(ctx, "SELECT 1 FROM spaces WHERE id = $1 AND storage_limit_mib > 0 FOR NO KEY UPDATE", spaceID); err != nil {
		return err
	}
	left, limited, err := room(ctx, tx, spaceID)
	if err != nil {
		return err
	}
	if limited && size > left {
		return errSpaceFull()
	}
	return nil
}

// copiedBytesSQL is what copying the files of a page stores anew: the current version of
// each file that is not in the trash, as CopyAll copies them.
const copiedBytesSQL = `SELECT COALESCE(sum(v.size), 0)::bigint FROM attachments a
	JOIN attachment_versions v ON v.attachment_id = a.id AND v.number = a.current_version
	WHERE a.page_id = $1 AND a.status = 'active'`

// movedBytesSQL is what moving pages brings into another space: every stored object of
// their files, in the trash too, that the target does not hold yet.
const movedBytesSQL = `SELECT COALESCE(sum(stored.size), 0)::bigint FROM (SELECT DISTINCT v.object_key, v.size
	FROM attachments a JOIN attachment_versions v ON v.attachment_id = a.id
	WHERE a.page_id = ANY($1) AND a.space_id <> $2) stored`

// requireRoomForCopy refuses a copy of a page's files into a space that has no room for
// them. Every way that copies files goes through CopyAll, which asks this before it
// stores anything (spec 047, D20).
func requireRoomForCopy(ctx context.Context, tx pgx.Tx, from, spaceID uuid.UUID) error {
	var size int64
	if err := tx.QueryRow(ctx, copiedBytesSQL, from).Scan(&size); err != nil {
		return err
	}
	return requireRoom(ctx, tx, spaceID, size)
}

// FitsCopy says, without locking, whether the files of a page would fit in a space, so
// a dialog can warn before the copy. The copy itself checks again (spec 047, D24).
func FitsCopy(ctx context.Context, q db.Querier, from, spaceID uuid.UUID) (bool, error) {
	var size int64
	if err := q.QueryRow(ctx, copiedBytesSQL, from).Scan(&size); err != nil {
		return false, err
	}
	return fits(ctx, q, spaceID, size)
}

// RefuseCopyThatDoesNotFit gives the refusal of a full space early, before work that a
// later refusal inside CopyAll would leave half done.
func RefuseCopyThatDoesNotFit(ctx context.Context, q db.Querier, from, spaceID uuid.UUID) error {
	ok, err := FitsCopy(ctx, q, from, spaceID)
	if err != nil {
		return err
	}
	if !ok {
		return errSpaceFull()
	}
	return nil
}

// FitsMove says, without locking, whether the files of moved pages would fit in a space.
func FitsMove(ctx context.Context, q db.Querier, pageIDs []uuid.UUID, spaceID uuid.UUID) (bool, error) {
	var size int64
	if err := q.QueryRow(ctx, movedBytesSQL, pageIDs, spaceID).Scan(&size); err != nil {
		return false, err
	}
	return fits(ctx, q, spaceID, size)
}

func fits(ctx context.Context, q db.Querier, spaceID uuid.UUID, size int64) (bool, error) {
	left, limited, err := room(ctx, q, spaceID)
	if err != nil {
		return false, err
	}
	return !limited || size <= left, nil
}

// allowance is the most one upload may store, and what to say when it goes over.
type allowance struct {
	bytes   int64
	refusal error
}

func (s *Service) allowanceFor(ctx context.Context, spaceID uuid.UUID) (allowance, error) {
	limit, err := s.settings.MaxUploadBytes(ctx)
	if err != nil {
		return allowance{}, err
	}
	left, limited, err := room(ctx, s.pool, spaceID)
	if err != nil {
		return allowance{}, err
	}
	if limited && left < limit {
		return allowance{bytes: left, refusal: errSpaceFull()}, nil
	}
	return allowance{bytes: limit, refusal: apperr.TooLarge("The file is larger than the upload limit.")}, nil
}
