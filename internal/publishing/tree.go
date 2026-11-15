// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// maxChangedListed bounds the pages the view Changed since release reads; the tree shows fewer.
const maxChangedListed = 1000

// ChangedSinceRelease lists the released pages of a draft space that changed since, the page
// that waited longest first, for the view of the tree of that name (PUB-1224). It lists them to
// those who write in the draft space or release from it, who see the status of a draft page
// (PUB-1221); to anyone else, in any other space and while the part is off, it lists none. The
// tree leaves out what the person may not see.
func (s *Service) ChangedSinceRelease(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]uuid.UUID, error) {
	none := []uuid.UUID{}
	on, err := s.features.ExperimentalOn(ctx, s.pool, features.ExperimentalPublishing)
	if err != nil || !on {
		return none, err
	}
	p, err := activePairOf(ctx, s.pool, spaceID, false)
	if hasCode(err, codeNotPaired) {
		return none, nil
	}
	if err != nil || p.Draft != spaceID {
		return none, err
	}
	edits, err := s.maySpace(ctx, c, spaceID, permissions.PageEdit)
	if err != nil {
		return none, err
	}
	releases, err := s.maySpace(ctx, c, spaceID, permissions.SpaceRelease)
	if err != nil || (!edits && !releases) {
		return none, err
	}
	rows, err := s.pool.Query(ctx, `SELECT draft_page_id FROM page_pairs
		WHERE pair_id = $1 AND released_version > 0 AND cardinality(changed) > 0
		ORDER BY changed_since, draft_page_id LIMIT $2`, p.ID, maxChangedListed)
	if err != nil {
		return none, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
