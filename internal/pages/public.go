// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PublicPage is a page on its way to the world.
type PublicPage struct {
	ID        uuid.UUID
	ParentID  *uuid.UUID
	Title     string
	Version   int
	Document  json.RawMessage
	UpdatedAt time.Time
	Author    string
}

// ForPublic reads the current versions of pages the caller has already judged public,
// parents first. No person's rights decide what the world sees (PRM-096), so this
// reads without a person. Names are only filled in when the site chose to show them.
func (s *Service) ForPublic(ctx context.Context, list []uuid.UUID, withNames bool) ([]PublicPage, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.parent_id, p.title, p.current_version, v.document, v.created_at, COALESCE(u.display_name, '')
		FROM pages p JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version
		LEFT JOIN users u ON u.id = v.author_id
		WHERE p.id = ANY($1)
		ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id), p.position, p.id`, list)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PublicPage, error) {
		var p PublicPage
		err := r.Scan(&p.ID, &p.ParentID, &p.Title, &p.Version, &p.Document, &p.UpdatedAt, &p.Author)
		return p, err
	})
	if err != nil || !withNames {
		return out, err
	}
	for i := range out {
		if out[i].Document, err = s.withNames(ctx, out[i].Document); err != nil {
			return nil, err
		}
	}
	return out, nil
}
