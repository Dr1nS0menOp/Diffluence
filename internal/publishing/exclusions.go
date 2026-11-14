// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

// Leaving pages out of releases (PUB-420 to PUB-423).

// codeLeftOut answers a release of a page that is left out.
const codeLeftOut = "left_out"

// maxReasonRunes bounds the reason of a page left out, shown on the release screen.
const maxReasonRunes = 500

// LeaveOutInput marks a page left out, alone or with everything below it.
type LeaveOutInput struct {
	WithChildren bool   `json:"with_children"`
	Reason       string `json:"reason"`
}

// leftOut says whether a draft page is left out: marked itself, or below a page marked with
// everything below it. New pages below such a page and pages moved out of it follow the tree,
// with no write (PUB-420).
func leftOut(ctx context.Context, q db.Querier, src pages.ReleaseSource) (bool, error) {
	var out bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM release_exclusions
		WHERE draft_page_id = $1 OR (with_children AND draft_page_id = ANY($2)))`, src.ID, src.Ancestors).Scan(&out)
	return out, err
}

// leaveOutPage loads a draft page that a person with space.release may leave out or include,
// with its active pair, inside tx.
func (s *Service) leaveOutPage(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID uuid.UUID) (pages.ReleaseSource, pair, error) {
	if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return pages.ReleaseSource{}, pair{}, err
	}
	src, err := pages.SourceForRelease(ctx, tx, pageID)
	if err != nil {
		return src, pair{}, err
	}
	if err := s.perms.RequireSpace(ctx, tx, c.UserID, src.SpaceID, permissions.SpaceRelease); err != nil {
		return src, pair{}, err
	}
	p, err := activePairOf(ctx, tx, src.SpaceID, false)
	if err != nil {
		return src, p, err
	}
	if p.Draft != src.SpaceID {
		return src, p, apperr.Conflict("not_a_draft_page", "Only pages of the draft space are left out of releases.")
	}
	return src, p, nil
}

// LeaveOut marks a draft page left out of releases (PUB-420). A page that is released keeps
// its twin as it is, Left out (still published), until someone takes it off (PUB-421).
func (s *Service) LeaveOut(ctx context.Context, c identity.Caller, pageID uuid.UUID, in LeaveOutInput) (PageStatus, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if !utf8.ValidString(in.Reason) || utf8.RuneCountInString(in.Reason) > maxReasonRunes || strings.ContainsRune(in.Reason, 0) {
		return PageStatus{}, apperr.Invalid("A reason has at most 500 characters.")
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		src, p, err := s.leaveOutPage(ctx, tx, c, pageID)
		if err != nil {
			return err
		}
		draft, err := spaces.LoadIn(ctx, tx, p.Draft)
		if err != nil {
			return err
		}
		if draft.HomePageID != nil && *draft.HomePageID == src.ID {
			return apperr.Conflict("home_page", "The home page is what readers see first, so it is not left out.")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO release_exclusions (draft_page_id, with_children, reason, created_by) VALUES ($1, $2, $3, $4)
			ON CONFLICT (draft_page_id) DO UPDATE SET with_children = EXCLUDED.with_children, reason = EXCLUDED.reason,
				created_by = EXCLUDED.created_by, created_at = now()`, src.ID, in.WithChildren, in.Reason, c.UserID); err != nil {
			return err
		}
		pp, err := loadPagePair(ctx, tx, p.ID, src.ID)
		if err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("release.left_out", "page", src.ID.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "with_children": in.WithChildren, "reason": in.Reason, "released": pp != nil && pp.ReleasedVersion > 0}))
		return err
	})
	if err != nil {
		return PageStatus{}, err
	}
	return s.Status(ctx, c, pageID)
}

// Include takes the mark off a page, which is then Not released yet or Changed since release,
// as it is (PUB-423). A page left out by a page above it is included through that page.
func (s *Service) Include(ctx context.Context, c identity.Caller, pageID uuid.UUID) (PageStatus, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		src, p, err := s.leaveOutPage(ctx, tx, c, pageID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM release_exclusions WHERE draft_page_id = $1", src.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			still, err := leftOut(ctx, tx, src)
			if err != nil {
				return err
			}
			if still {
				return apperr.Conflict("left_out_above", "A page above this one is left out with the pages below it. Include that page instead.")
			}
			return apperr.Conflict("not_left_out", "This page is not left out.")
		}
		_, err = audit.Record(ctx, tx, c.Event("release.included", "page", src.ID.String(), p.Draft.String(), map[string]any{"pair_id": p.ID}))
		return err
	})
	if err != nil {
		return PageStatus{}, err
	}
	return s.Status(ctx, c, pageID)
}

// exclusionOf is the mark that leaves a page out, for the status of a page.
func exclusionOf(ctx context.Context, q db.Querier, src pages.ReleaseSource) (*Exclusion, error) {
	var e Exclusion
	err := q.QueryRow(ctx, `SELECT draft_page_id, with_children, reason, created_at FROM release_exclusions
		WHERE draft_page_id = $1 OR (with_children AND draft_page_id = ANY($2))
		ORDER BY draft_page_id = $1 DESC LIMIT 1`, src.ID, src.Ancestors).Scan(&e.PageID, &e.WithChildren, &e.Reason, &e.At)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &e, err
}
