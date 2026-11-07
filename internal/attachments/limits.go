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
const spaceBytesSQL = `(SELECT COALESCE(sum(v.size), 0)::bigint FROM attachments a
	JOIN attachment_versions v ON v.attachment_id = a.id WHERE a.space_id = $1 AND a.status = 'active')`

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
