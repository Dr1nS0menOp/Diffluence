// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// ExportPage is a page as an export sees it.
type ExportPage struct {
	ID       uuid.UUID
	ParentID *uuid.UUID
	Title    string
	Document json.RawMessage
}

// ForExport returns the published pages the user may view, parents first. With
// root set, only that page and what is below it. It goes through the same SQL
// rule as every list, so an export can never contain more than the tree shows.
func (s *Service) ForExport(ctx context.Context, c identity.Caller, spaceID uuid.UUID, root *uuid.UUID) ([]ExportPage, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceExport); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.parent_id, p.title, v.document
		FROM pages p
		JOIN perm_visible_pages($1) vis ON vis.page_id = p.id
		JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version
		LEFT JOIN page_tree d ON d.descendant_id = p.id AND d.ancestor_id = $3
		WHERE p.space_id = $2 AND p.status = 'published' AND ($3::uuid IS NULL OR d.ancestor_id IS NOT NULL)
		ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id), p.position, p.id`, c.UserID, spaceID, root)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExportPage, error) {
		var p ExportPage
		err := r.Scan(&p.ID, &p.ParentID, &p.Title, &p.Document)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Document, err = s.withNames(ctx, out[i].Document); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SpaceOf returns the space of a page the user may view.
func (s *Service) SpaceOf(ctx context.Context, c identity.Caller, id uuid.UUID) (uuid.UUID, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return uuid.Nil, err
	}
	p, err := load(ctx, s.pool, id)
	return p.SpaceID, err
}
