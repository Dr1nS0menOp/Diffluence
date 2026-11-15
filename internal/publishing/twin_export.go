// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"

	"github.com/google/uuid"

	"github.com/diffluence/diffluence/internal/pages"
)

// twinState is the state a file of a published page names: the page readers have is always the
// approved version, and the published space holds no workflow whose states it could name (PUB-703).
const twinState = "Approved"

// TwinExportControl is what a file of a published page of a controlled page says of its approval
// (PUB-731): a controlled document, the version readers cite, when it was approved, who approved
// it only when the pair shows approvers (PUB-730), and the signatures of the version released, as
// the band of the page shows them (DC4-21). Nil for a page that is no twin of a controlled page,
// or whose version readers have was written by no release of the pair.
func (s *Service) TwinExportControl(ctx context.Context, pageID uuid.UUID) (*pages.ExportControl, error) {
	var draft uuid.UUID
	var released, twinVersion int
	var settings []byte
	err := s.pool.QueryRow(ctx, `SELECT pp.draft_page_id, pp.released_version, pp.twin_version, sp.settings
		FROM page_pairs pp JOIN space_pairs sp ON sp.id = pp.pair_id AND sp.ended_at IS NULL
		WHERE pp.twin_page_id = $1 AND pp.released_version > 0`, pageID).Scan(&draft, &released, &twinVersion, &settings)
	if noRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	twin, err := pages.SourceForRelease(ctx, s.pool, pageID)
	if err != nil || twin.CurrentVersion != twinVersion {
		return nil, err
	}
	a, err := twinApproval(ctx, s.pool, draft, released, twinVersion, settings)
	if err != nil || a == nil {
		return nil, err
	}
	version := a.Version
	return &pages.ExportControl{StateName: twinState, Version: version, ApprovedVersion: &version, ApprovedAt: &a.ApprovedAt,
		ApprovedBy: a.ApprovedBy, Signatures: a.Signatures}, nil
}
