// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
)

// What a bundle of a space carries of draft and published spaces (spec 073, wave PB4, PUB-1042,
// PUB-1043): the marks that leave pages out of releases, which belong to the pages, and never the
// pair, which is between two spaces of one installation. A bundle imported elsewhere makes an
// ordinary space whose pages remember which were left out.

// LeftOutMark is a page left out of releases, alone or with every page below it, and why.
type LeftOutMark struct {
	PageID       uuid.UUID
	WithChildren bool
	Reason       string
}

// LeftOutAmong reads the marks on the pages among list, for a bundle of their space.
func LeftOutAmong(ctx context.Context, q db.Querier, list []uuid.UUID) ([]LeftOutMark, error) {
	rows, err := q.Query(ctx, `SELECT draft_page_id, with_children, reason FROM release_exclusions WHERE draft_page_id = ANY($1)
		ORDER BY draft_page_id`, list)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (LeftOutMark, error) {
		var m LeftOutMark
		err := r.Scan(&m.PageID, &m.WithChildren, &m.Reason)
		return m, err
	})
}

// RestoreLeftOut writes the marks a bundle carried onto the pages it made, inside the transaction
// of the import, as the importer's. The bundle carries marks only of the pages it holds.
func RestoreLeftOut(ctx context.Context, tx pgx.Tx, marks []LeftOutMark, by uuid.UUID) error {
	for _, m := range marks {
		if _, err := tx.Exec(ctx, `INSERT INTO release_exclusions (draft_page_id, with_children, reason, created_by)
			VALUES ($1, $2, $3, $4) ON CONFLICT (draft_page_id) DO NOTHING`, m.PageID, m.WithChildren, m.Reason, by); err != nil {
			return err
		}
	}
	return nil
}
