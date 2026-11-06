// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/spaces"
)

// maxBundlePages keeps a space template small enough to apply at once.
const maxBundlePages = 200

func starterBundle(st starterSpace) []BundlePage {
	empty, _ := document.Marshal(document.Empty())
	out := make([]BundlePage, len(st.pages))
	for i, title := range st.pages {
		out[i] = BundlePage{Title: title, Parent: -1, Labels: []string{}, Document: empty}
	}
	return out
}

func (s *Service) seedSpaces(ctx context.Context) error {
	for _, st := range starterSpaces {
		bundle, err := json.Marshal(starterBundle(st))
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO space_templates (id, name, description, bundle, starter_key, created_by)
			VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`, ids.New(), st.name, st.description, bundle, st.key, uuid.Nil); err != nil {
			return err
		}
	}
	return nil
}

// SpaceTemplates lists what a new space can start from.
func (s *Service) SpaceTemplates(ctx context.Context) ([]SpaceTemplate, error) {
	if err := s.seedSpaces(ctx); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT id, name, description, starter_key IS NOT NULL FROM space_templates ORDER BY lower(name), id")
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[SpaceTemplate])
	if out == nil {
		out = []SpaceTemplate{}
	}
	return out, err
}

// SaveSpace keeps the pages of a space as a template. It holds text only: files
// would make a bundle large for little gain (spec 011, D32). The route requires
// administrator, and only pages this person can see are in it.
func (s *Service) SaveSpace(ctx context.Context, c identity.Caller, spaceID uuid.UUID, name, description string) (SpaceTemplate, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SpaceTemplate{}, apperr.Invalid("A template needs a name of at most 100 characters.")
	}
	list, err := s.pages.ForExport(ctx, c, spaceID, nil)
	if err != nil {
		return SpaceTemplate{}, err
	}
	if len(list) > maxBundlePages {
		return SpaceTemplate{}, apperr.Invalid("A space template can hold at most 200 pages.")
	}
	places := map[uuid.UUID]int{}
	bundle := make([]BundlePage, 0, len(list))
	for _, p := range list {
		doc, err := document.Parse(p.Document)
		if err != nil {
			return SpaceTemplate{}, err
		}
		raw, err := document.Marshal(document.WithoutFiles(doc))
		if err != nil {
			return SpaceTemplate{}, err
		}
		parent := -1
		if p.ParentID != nil {
			if at, ok := places[*p.ParentID]; ok {
				parent = at
			}
		}
		places[p.ID] = len(bundle)
		bundle = append(bundle, BundlePage{Title: p.Title, Parent: parent, Labels: []string{}, Document: raw})
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return SpaceTemplate{}, err
	}
	out := SpaceTemplate{ID: ids.New(), Name: name, Description: description}
	_, err = s.pool.Exec(ctx, "INSERT INTO space_templates (id, name, description, bundle, created_by) VALUES ($1, $2, $3, $4, $5)",
		out.ID, name, description, raw, c.UserID)
	return out, err
}

// StartSpace makes a space from a template.
func (s *Service) StartSpace(ctx context.Context, c identity.Caller, in spaces.Input, templateID uuid.UUID) (spaces.Space, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, "SELECT bundle FROM space_templates WHERE id = $1", templateID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return spaces.Space{}, apperr.NotFound()
	}
	if err != nil {
		return spaces.Space{}, err
	}
	var bundle []BundlePage
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return spaces.Space{}, err
	}
	sp, err := s.spaces.Create(ctx, c, in)
	if err != nil {
		return sp, err
	}
	made := make([]uuid.UUID, len(bundle))
	for i, entry := range bundle {
		var parent *uuid.UUID
		if entry.Parent >= 0 && entry.Parent < i {
			parent = &made[entry.Parent]
		} else {
			parent = sp.HomePageID
		}
		p, err := s.pages.Create(ctx, c, pages.NewPage{SpaceID: sp.ID, ParentID: parent, Title: entry.Title})
		if err != nil {
			return sp, err
		}
		if _, err := s.pages.Publish(ctx, c, p.ID, pages.PublishInput{Title: entry.Title, Document: entry.Document}); err != nil {
			return sp, err
		}
		made[i] = p.ID
	}
	return sp, nil
}
