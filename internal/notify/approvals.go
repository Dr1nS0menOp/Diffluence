// SPDX-License-Identifier: Apache-2.0

package notify

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/audit"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/outbox"
	"github.com/kenwerk/kenwerk/internal/permissions"
)

// The approvals digest of a space (spec 064, DOC-607): a person who views a space, such as a
// quality lead, can ask to hear of every approval in it. It goes through the channels and
// rate limits of every other notification, and adds no channel of its own (DOC-609).

// ApprovalsDigest says whether a person hears of every approval in a space.
type ApprovalsDigest struct {
	On bool `json:"on"`
}

// ApprovalsSubscribed reads whether a person asked for the approvals digest of a space.
func (s *Service) ApprovalsSubscribed(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (ApprovalsDigest, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return ApprovalsDigest{}, err
	}
	var out ApprovalsDigest
	err := s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM approval_subscriptions WHERE user_id = $1 AND space_id = $2)", c.UserID, spaceID).
		Scan(&out.On)
	return out, err
}

// SetApprovalsDigest asks for, or stops, the approvals digest of a space. Each approval still
// reaches the person only for pages they may see.
func (s *Service) SetApprovalsDigest(ctx context.Context, c identity.Caller, spaceID uuid.UUID, on bool) (ApprovalsDigest, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceView); err != nil {
			return err
		}
		sql := "DELETE FROM approval_subscriptions WHERE user_id = $1 AND space_id = $2"
		action := "notify.approvals_digest_stopped"
		if on {
			sql = "INSERT INTO approval_subscriptions (user_id, space_id) VALUES ($1, $2) ON CONFLICT DO NOTHING"
			action = "notify.approvals_digest_started"
		}
		tag, err := tx.Exec(ctx, sql, c.UserID, spaceID)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event(action, "space", spaceID.String(), spaceID.String(), nil)); err != nil {
			return err
		}
		payload := map[string]any{"space_id": spaceID, "user_id": c.UserID}
		c.Stamp(payload)
		_, err = outbox.Enqueue(ctx, tx, action, payload)
		return err
	})
	return ApprovalsDigest{On: on}, err
}

// approvalSubscribers lists who asked for every approval in a space.
func approvalSubscribers(ctx context.Context, q db.Querier, spaceID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, "SELECT user_id FROM approval_subscriptions WHERE space_id = $1 ORDER BY user_id", spaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}
