// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// syncBlocks fills what reports read, in the transaction that publishes, so a report
// never runs ahead of a published version (spec 012, D12).
func syncBlocks(ctx context.Context, tx pgx.Tx, pageID uuid.UUID, doc document.Node) error {
	for _, table := range []string{
		"DELETE FROM page_facts WHERE page_id = $1",
		"DELETE FROM page_decisions WHERE page_id = $1",
		"DELETE FROM page_includes WHERE source_id = $1",
	} {
		if _, err := tx.Exec(ctx, table, pageID); err != nil {
			return err
		}
	}
	for i, f := range document.Facts(doc) {
		if _, err := tx.Exec(ctx, `INSERT INTO page_facts (page_id, facts_id, key, position, value, value_date, value_num)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, pageID, f.FactsID, f.Key, i, f.Text, f.Date, f.Number); err != nil {
			return err
		}
	}
	for i, text := range document.Decisions(doc) {
		if _, err := tx.Exec(ctx, "INSERT INTO page_decisions (page_id, position, text) VALUES ($1, $2, $3)", pageID, i, text); err != nil {
			return err
		}
	}
	for _, in := range document.Includes(doc) {
		if _, err := tx.Exec(ctx, `INSERT INTO page_includes (source_id, target_id, name) VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, pageID, in.Page, in.Name); err != nil {
			return err
		}
	}
	return nil
}

// UsedBy lists the pages that show this page, as far as the caller may see (BLK-050).
func (s *Service) UsedBy(ctx context.Context, c identity.Caller, id uuid.UUID) ([]PageRef, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT `+refColumns+`, lower(p.title) FROM page_includes i
		JOIN pages p ON p.id = i.source_id `+refJoins+`
		WHERE i.target_id = $2 AND p.status = 'published' ORDER BY lower(p.title) LIMIT $3`, c.UserID, id, maxListedLinks)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PageRef, error) {
		var v PageRef
		var sortKey string
		err := r.Scan(&v.ID, &v.Title, &v.Emoji, &v.SpaceKey, &v.SpaceName, &sortKey)
		return v, err
	})
	if out == nil {
		out = []PageRef{}
	}
	return out, err
}
