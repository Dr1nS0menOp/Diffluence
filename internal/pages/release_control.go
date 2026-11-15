// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/workflow"
)

// What a release of draft and published spaces reads of document control (spec 073, wave PB3):
// the approval of a page, who wrote what an approval carries, and when a version was approved.
// As in release.go, none of these checks a permission: the publishing module has.

// ReleaseControl is the document control of a page as a release reads it.
type ReleaseControl struct {
	// ApprovedVersion is the version readers have; nil when the page was never approved.
	ApprovedVersion *int
	HeadVersion     int
	StateKind       workflow.Kind
	// ApprovedBy is the person whose act approved it: the author who took the step, or the
	// reviewer whose answer passed the round.
	ApprovedBy *uuid.UUID
	// ApprovedState is the state the approved version was approved into, from the version of
	// the workflow the page follows.
	ApprovedState workflow.State
	// Round is the round that approved the approved version, when there was one.
	Round *uuid.UUID
}

// ReleaseControlOf reads the document control of a page, through q; ok is false for a page
// that follows no workflow.
func ReleaseControlOf(ctx context.Context, q db.Querier, pageID uuid.UUID) (ReleaseControl, bool, error) {
	var c ReleaseControl
	var stateID string
	var raw []byte
	err := q.QueryRow(ctx, `SELECT pc.approved_version, pc.head_version, pc.state_kind, pc.approved_by, COALESCE(pc.approved_state_id, ''),
			v.definition,
			(SELECT r.id FROM workflow_rounds r WHERE r.page_id = pc.page_id AND r.status = 'approved'
				AND r.version_number = pc.approved_version ORDER BY r.ended_at DESC NULLS LAST LIMIT 1)
		FROM page_control pc
		JOIN workflow_versions v ON v.workflow_id = pc.workflow_id AND v.version = pc.workflow_version
		WHERE pc.page_id = $1`, pageID).Scan(&c.ApprovedVersion, &c.HeadVersion, &c.StateKind, &c.ApprovedBy, &stateID, &raw, &c.Round)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	var def workflow.Definition
	if err := json.Unmarshal(raw, &def); err != nil {
		return c, false, fmt.Errorf("pages: read the workflow of page %s: %v", pageID, err)
	}
	c.ApprovedState, _ = def.State(stateID)
	return c, true, nil
}

// AuthorsBetween lists, at most limit, the people who wrote the versions of a page after one
// number up to another.
func AuthorsBetween(ctx context.Context, q db.Querier, pageID uuid.UUID, after, upTo, limit int) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT author_id FROM page_versions WHERE page_id = $1 AND number > $2 AND number <= $3
		ORDER BY author_id LIMIT $4`, pageID, after, upTo, limit)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if out == nil {
		out = []uuid.UUID{}
	}
	return out, err
}

// VersionApproval is when a version of a controlled page was approved, and the name of who
// approved it when that is still known.
type VersionApproval struct {
	At     time.Time
	ByName string
}

// ApprovalOfVersion says when a version of a page was approved, through q; nil when it never
// was, or the page follows no workflow. The approver is known while that version is still the
// approved one.
func ApprovalOfVersion(ctx context.Context, q db.Querier, pageID uuid.UUID, number int) (*VersionApproval, error) {
	var at *time.Time
	var name string
	err := q.QueryRow(ctx, `SELECT COALESCE(
				(SELECT r.ended_at FROM workflow_rounds r WHERE r.page_id = pc.page_id AND r.status = 'approved'
					AND r.version_number = $2 ORDER BY r.ended_at DESC NULLS LAST LIMIT 1),
				CASE WHEN pc.approved_version = $2 THEN pc.approved_at END),
			CASE WHEN pc.approved_version = $2 THEN COALESCE(u.display_name, '') ELSE '' END
		FROM page_control pc LEFT JOIN users u ON u.id = pc.approved_by WHERE pc.page_id = $1`, pageID, number).Scan(&at, &name)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && at == nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &VersionApproval{At: *at, ByName: name}, nil
}
