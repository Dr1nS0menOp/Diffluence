// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	maxLabelRunes    = 64
	maxLabelsPerPage = 50
	maxListedLabels  = 200
	maxLabelPages    = 200
)

// CleanLabel writes a label the one allowed way: small letters, dashes for
// spaces, and letters of any language, digits, dashes and underscores.
func CleanLabel(raw string) (string, error) {
	label := strings.Join(strings.Fields(strings.ToLower(raw)), "-")
	bad := apperr.Invalid("A label has letters, digits, dashes and underscores, and at most 64 characters.")
	if label == "" || utf8.RuneCountInString(label) > maxLabelRunes {
		return "", bad
	}
	for _, r := range label {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return "", bad
		}
	}
	return label, nil
}

func labelsOf(ctx context.Context, q db.Querier, pageID uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, "SELECT label FROM page_labels WHERE page_id = $1 ORDER BY label", pageID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

func fixedLabelRefused(ctx context.Context, q db.Querier, spaceID uuid.UUID, label string) (bool, error) {
	var refused bool
	err := q.QueryRow(ctx, `SELECT s.fixed_labels AND NOT EXISTS
			(SELECT 1 FROM space_fixed_labels f WHERE f.space_id = s.id AND f.label = $2)
		FROM spaces s WHERE s.id = $1`, spaceID, label).Scan(&refused)
	return refused, err
}

// addLabel puts a label on a page inside tx. The caller has checked the rights.
func addLabel(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, label string) error {
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM page_labels WHERE page_id = $1", p.ID).Scan(&count); err != nil {
		return err
	}
	if count >= maxLabelsPerPage {
		return apperr.Invalid("A page can have at most 50 labels.")
	}
	tag, err := tx.Exec(ctx, "INSERT INTO page_labels (page_id, label, created_by) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", p.ID, label, c.UserID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	return record(ctx, tx, c, "page.labelled", p, map[string]any{"label": label})
}

// AddLabel adds a label. It makes no version.
func (s *Service) AddLabel(ctx context.Context, c identity.Caller, id uuid.UUID, raw string) ([]string, error) {
	label, err := CleanLabel(raw)
	if err != nil {
		return nil, err
	}
	var out []string
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		refused, err := fixedLabelRefused(ctx, tx, p.SpaceID, label)
		if err != nil {
			return err
		}
		if refused {
			return apperr.Invalid("This space has a fixed list of labels.")
		}
		if err := addLabel(ctx, tx, c, p, label); err != nil {
			return err
		}
		out, err = labelsOf(ctx, tx, id)
		return err
	})
	return out, err
}

// RemoveLabel takes a label off. A label nobody uses any more is simply gone.
func (s *Service) RemoveLabel(ctx context.Context, c identity.Caller, id uuid.UUID, label string) ([]string, error) {
	var out []string
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM page_labels WHERE page_id = $1 AND label = $2", id, label)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			if err := record(ctx, tx, c, "page.unlabelled", p, map[string]any{"label": label}); err != nil {
				return err
			}
		}
		out, err = labelsOf(ctx, tx, id)
		return err
	})
	return out, err
}

// PageLabels lists the labels of a page.
func (s *Service) PageLabels(ctx context.Context, c identity.Caller, id uuid.UUID) ([]string, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return nil, err
	}
	return labelsOf(ctx, s.pool, id)
}

// LabelCount is a label with the number of pages the person can see.
type LabelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// LabelQuery narrows a list of labels.
type LabelQuery struct {
	Text    string
	SpaceID *uuid.UUID
	ByName  bool
}

// Labels lists labels from pages the person may see. A label that only exists on
// hidden pages does not show up, and a count never includes them.
func (s *Service) Labels(ctx context.Context, c identity.Caller, q LabelQuery) ([]LabelCount, error) {
	rows, err := s.pool.Query(ctx, `SELECT l.label, count(*) FROM page_labels l
		JOIN perm_listed_pages($1) v ON v.page_id = l.page_id
		JOIN pages p ON p.id = l.page_id
		WHERE p.status = 'published' AND ($2 = '' OR position(lower($2) IN l.label) > 0)
		  AND ($3::uuid IS NULL OR p.space_id = $3)
		GROUP BY l.label
		ORDER BY CASE WHEN $4 THEN 0 ELSE count(*) END DESC, l.label LIMIT $5`, c.UserID, q.Text, q.SpaceID, q.ByName, maxListedLabels)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[LabelCount])
	if out == nil {
		out = []LabelCount{}
	}
	return out, err
}

// LabelPages lists pages that carry all these labels, as far as the person may see.
func (s *Service) LabelPages(ctx context.Context, c identity.Caller, labels []string, spaceID *uuid.UUID) ([]ListItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, sp.key, sp.name, p.updated_at, false
		FROM pages p
		JOIN perm_listed_pages($1) v ON v.page_id = p.id
		JOIN spaces sp ON sp.id = p.space_id
		WHERE p.status = 'published' AND ($3::uuid IS NULL OR p.space_id = $3)
		  AND (SELECT count(*) FROM page_labels l WHERE l.page_id = p.id AND l.label = ANY($2)) = $4
		ORDER BY p.updated_at DESC, p.id LIMIT $5`, c.UserID, labels, spaceID, len(labels), maxLabelPages)
	return collectList(rows, err)
}

// SpaceLabels is the label list of a space, for its screen and its settings.
type SpaceLabels struct {
	Labels      []LabelCount `json:"labels"`
	Fixed       bool         `json:"fixed"`
	FixedLabels []string     `json:"fixed_labels"`
}

// SpaceLabelList shows the labels of a space.
func (s *Service) SpaceLabelList(ctx context.Context, c identity.Caller, spaceID uuid.UUID, byName bool) (SpaceLabels, error) {
	var out SpaceLabels
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return out, err
	}
	var err error
	if out.Labels, err = s.Labels(ctx, c, LabelQuery{SpaceID: &spaceID, ByName: byName}); err != nil {
		return out, err
	}
	if err := s.pool.QueryRow(ctx, "SELECT fixed_labels FROM spaces WHERE id = $1", spaceID).Scan(&out.Fixed); err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, "SELECT label FROM space_fixed_labels WHERE space_id = $1 ORDER BY label", spaceID)
	if err != nil {
		return out, err
	}
	out.FixedLabels, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if out.FixedLabels == nil {
		out.FixedLabels = []string{}
	}
	return out, err
}

// SetFixedLabels turns the fixed list of a space on or off. Labels already on
// pages stay.
func (s *Service) SetFixedLabels(ctx context.Context, c identity.Caller, spaceID uuid.UUID, fixed bool, labels []string) error {
	clean := make([]string, 0, len(labels))
	for _, raw := range labels {
		label, err := CleanLabel(raw)
		if err != nil {
			return err
		}
		clean = append(clean, label)
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET fixed_labels = $2 WHERE id = $1", spaceID, fixed); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_fixed_labels WHERE space_id = $1", spaceID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO space_fixed_labels (space_id, label)
			SELECT $1, label FROM unnest($2::text[]) AS label ON CONFLICT DO NOTHING`, spaceID, clean); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, c.Event("space.labels_fixed", "space", spaceID.String(), spaceID.String(), map[string]any{"fixed": fixed, "count": len(clean)}))
		return err
	})
}

// RenameLabel renames a label in a space. Renaming onto an existing label merges them.
func (s *Service) RenameLabel(ctx context.Context, c identity.Caller, spaceID uuid.UUID, from, to string) error {
	target, err := CleanLabel(to)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO page_labels (page_id, label, created_by)
			SELECT l.page_id, $3, l.created_by FROM page_labels l JOIN pages p ON p.id = l.page_id
			WHERE p.space_id = $1 AND l.label = $2 ON CONFLICT DO NOTHING`, spaceID, from, target); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM page_labels l USING pages p
			WHERE p.id = l.page_id AND p.space_id = $1 AND l.label = $2 AND l.label <> $3`, spaceID, from, target); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, c.Event("space.label_renamed", "space", spaceID.String(), spaceID.String(), map[string]any{"from": from, "to": target}))
		return err
	})
}
