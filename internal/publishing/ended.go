// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"

	"github.com/google/uuid"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/spaces"
)

// maxToldAdmins bounds the people told that deleting a space ended its pair.
const maxToldAdmins = 100

// EndedByDelete is a pair that deleting one of its spaces ended (PUB-1046, PUB-1047): the space
// that stays, its home page, the people who run it and the side that was deleted.
type EndedByDelete struct {
	SpaceID uuid.UUID
	Home    uuid.UUID
	Admins  []uuid.UUID
	Side    string
}

// EndedByTrash finds the pair that sending a space to the trash ended, for the notifications of
// the worker; nil when the space was in no pair, or the pair was ended by hand. The database ends
// such a pair in the statement that trashes the space (decision D30), so the pair is read here.
func (s *Service) EndedByTrash(ctx context.Context, spaceID uuid.UUID) (*EndedByDelete, error) {
	var draft, published uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT draft_space_id, published_space_id FROM space_pairs
		WHERE (draft_space_id = $1 OR published_space_id = $1) AND ended_at IS NOT NULL AND ended_by IS NULL
		ORDER BY ended_at DESC LIMIT 1`, spaceID).Scan(&draft, &published)
	if noRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &EndedByDelete{SpaceID: draft, Side: permissions.PairPublished}
	if draft == spaceID {
		out.SpaceID, out.Side = published, permissions.PairDraft
	}
	sp, err := spaces.LoadIn(ctx, s.pool, out.SpaceID)
	if apperr.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if sp.HomePageID == nil || sp.Status != spaces.StatusActive {
		return nil, nil
	}
	out.Home = *sp.HomePageID
	if out.Admins, err = permissions.AdministratorUsers(ctx, s.pool, out.SpaceID, maxToldAdmins); err != nil {
		return nil, err
	}
	return out, nil
}
