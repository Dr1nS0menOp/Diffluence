// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	capabilityVerify = "page.verify"
	maxStages        = 6
	maxStageRunes    = 30
	maxReviewMonths  = 120
)

// Stage is the status a writer gives a page: a word and a colour name. It is
// called a stage in code, because `status` already means draft or published.
type Stage struct {
	Text  string `json:"text"`
	Color string `json:"color"`
}

// defaultStages are suggested until a space makes its own list.
var defaultStages = []Stage{{"Early draft", "yellow"}, {"Being written", "blue"}, {"Needs review", "purple"}}

// Verified says who stands behind a page, and until when.
type Verified struct {
	By           uuid.UUID `json:"by"`
	ByName       string    `json:"by_name"`
	At           time.Time `json:"at"`
	Until        string    `json:"until"`
	Version      int       `json:"version"`
	ChangedAfter bool      `json:"changed_after"`
	Expired      bool      `json:"expired"`
}

func (s *Service) requireFeature(ctx context.Context, q db.Querier, spaceID uuid.UUID, feature string) error {
	on, err := s.features.Enabled(ctx, q, spaceID, feature)
	if err != nil {
		return err
	}
	if !on {
		return apperr.Conflict("feature_off", "This is turned off for this space.")
	}
	return nil
}

// fillOrganising adds what Get shows around a page.
func (s *Service) fillOrganising(ctx context.Context, c identity.Caller, d *Detail) error {
	var status string
	err := s.pool.QueryRow(ctx, "SELECT display_name, status FROM users WHERE id = $1", d.OwnerID).Scan(&d.OwnerName, &status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	d.OwnerDisabled = status != "active"
	if d.Labels, err = labelsOf(ctx, s.pool, d.ID); err != nil {
		return err
	}
	if d.Verified, err = verificationOf(ctx, s.pool, d.Page); err != nil {
		return err
	}
	err = s.pool.QueryRow(ctx, "SELECT COALESCE(seen_version, 0) FROM recent_pages WHERE page_id = $1 AND user_id = $2", d.ID, c.UserID).Scan(&d.SeenVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if d.Can[permissions.PageEdit] {
		if d.Scheduled, err = scheduleOf(ctx, s.pool, d.ID); err != nil {
			return err
		}
	}
	d.Approval, err = approvalOf(ctx, s.pool, c.UserID, d.Page)
	return err
}

func verificationOf(ctx context.Context, q db.Querier, p Page) (*Verified, error) {
	var v Verified
	var until time.Time
	err := q.QueryRow(ctx, `SELECT v.verified_by, COALESCE(u.display_name, ''), v.verified_at, v.valid_until, v.version
		FROM page_verifications v LEFT JOIN users u ON u.id = v.verified_by WHERE v.page_id = $1`, p.ID).
		Scan(&v.By, &v.ByName, &v.At, &until, &v.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v.Until = until.Format(time.DateOnly)
	v.ChangedAfter = p.CurrentVersion > v.Version
	v.Expired = until.Before(time.Now().UTC().Truncate(24 * time.Hour))
	return &v, nil
}

// SetOwner hands a page to someone else. It gives no right: "delete own" stays
// with the creator (spec 011, D9).
func (s *Service) SetOwner(ctx context.Context, c identity.Caller, id, owner uuid.UUID) (Page, error) {
	var out Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		if _, err := lockPage(ctx, tx, id); err != nil {
			return err
		}
		d, err := s.perms.CheckPage(ctx, tx, owner, id, permissions.ActionPageView, uuid.Nil)
		if err != nil {
			return err
		}
		if !d.Allowed {
			return apperr.Invalid("That person cannot see this page.")
		}
		out, err = scanPage(tx.QueryRow(ctx, "UPDATE pages SET owner_id = $2, updated_at = now() WHERE id = $1 RETURNING "+pageColumns, id, owner))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.owner_changed", out, map[string]any{"owner_id": owner})
	})
	return out, err
}

// Stages lists what can be chosen as a status in a space, and whether a person
// may type their own.
func (s *Service) Stages(ctx context.Context, q db.Querier, spaceID uuid.UUID) ([]Stage, bool, error) {
	var free bool
	if err := q.QueryRow(ctx, "SELECT free_stage FROM spaces WHERE id = $1", spaceID).Scan(&free); err != nil {
		return nil, false, err
	}
	rows, err := q.Query(ctx, "SELECT text, color FROM space_stages WHERE space_id = $1 ORDER BY position", spaceID)
	if err != nil {
		return nil, false, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Stage])
	if err != nil {
		return nil, false, err
	}
	if len(list) == 0 {
		list = defaultStages
	}
	return list, free, nil
}

// SpaceStages shows the list of a space to someone who may view it.
func (s *Service) SpaceStages(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Stage, bool, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return nil, false, err
	}
	return s.Stages(ctx, s.pool, spaceID)
}

// SetStages replaces the list of a space. A word that left the list stays on the
// pages that have it, until they change.
func (s *Service) SetStages(ctx context.Context, c identity.Caller, spaceID uuid.UUID, list []Stage, free bool) error {
	if len(list) > maxStages {
		return apperr.Invalid("A space can have at most six statuses.")
	}
	for _, st := range list {
		if strings.TrimSpace(st.Text) == "" || utf8.RuneCountInString(st.Text) > maxStageRunes || !document.IsMarkColor(st.Color) {
			return apperr.Invalid("A status has a word of at most 30 characters and a known colour.")
		}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET free_stage = $2 WHERE id = $1", spaceID, free); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM space_stages WHERE space_id = $1", spaceID); err != nil {
			return err
		}
		for i, st := range list {
			if _, err := tx.Exec(ctx, "INSERT INTO space_stages (space_id, position, text, color) VALUES ($1, $2, $3, $4)",
				spaceID, i, strings.TrimSpace(st.Text), st.Color); err != nil {
				return err
			}
		}
		_, err := audit.Record(ctx, tx, c.Event("space.stages_changed", "space", spaceID.String(), spaceID.String(), map[string]any{"count": len(list)}))
		return err
	})
}

// applyStage checks a status against its space and stores it, inside tx.
func (s *Service) applyStage(ctx context.Context, tx pgx.Tx, c identity.Caller, p Page, text, color string) error {
	text = strings.TrimSpace(text)
	if text != "" {
		if err := s.requireFeature(ctx, tx, p.SpaceID, features.Stages); err != nil {
			return err
		}
		if !document.IsMarkColor(color) || utf8.RuneCountInString(text) > maxStageRunes {
			return apperr.Invalid("A status has a word of at most 30 characters and a known colour.")
		}
		list, free, err := s.Stages(ctx, tx, p.SpaceID)
		if err != nil {
			return err
		}
		known := false
		for _, st := range list {
			if st.Text == text {
				known, color = true, st.Color
			}
		}
		if !known && !free {
			return apperr.Invalid("This space has a fixed list of statuses.")
		}
	} else {
		color = ""
	}
	out, err := scanPage(tx.QueryRow(ctx, "UPDATE pages SET stage_text = $2, stage_color = $3 WHERE id = $1 RETURNING "+pageColumns, p.ID, text, color))
	if err != nil {
		return err
	}
	return record(ctx, tx, c, "page.stage_changed", out, map[string]any{"stage": text})
}

// SetStage gives a page a status. It is not part of a version.
func (s *Service) SetStage(ctx context.Context, c identity.Caller, id uuid.UUID, text, color string) (Page, error) {
	var out Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.applyStage(ctx, tx, c, p, text, color); err != nil {
			return err
		}
		out, err = load(ctx, tx, id)
		return err
	})
	return out, err
}

// requireVerifier: the owner and space administrators stand behind a page.
func (s *Service) requireVerifier(ctx context.Context, q db.Querier, c identity.Caller, p Page) error {
	if err := s.perms.RequirePage(ctx, q, c.UserID, p.ID, permissions.PageEdit, uuid.Nil); err != nil {
		return err
	}
	if p.OwnerID == c.UserID {
		return nil
	}
	return s.perms.RequirePage(ctx, q, c.UserID, p.ID, permissions.SpaceAdminister, uuid.Nil)
}

func parseDate(value string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return day, apperr.Invalid("A date looks like 2026-11-05.")
	}
	return day, nil
}

// Verify marks a page as checked until a date.
func (s *Service) Verify(ctx context.Context, c identity.Caller, id uuid.UUID, until string) error {
	day, err := parseDate(until)
	if err != nil {
		return err
	}
	if !day.After(time.Now()) {
		return apperr.Invalid("A page is verified until a date in the future.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.requireVerifier(ctx, tx, c, p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO page_verifications (page_id, verified_by, valid_until, version) VALUES ($1, $2, $3, $4)
			ON CONFLICT (page_id) DO UPDATE SET verified_by = EXCLUDED.verified_by, verified_at = now(),
				valid_until = EXCLUDED.valid_until, version = EXCLUDED.version`, id, c.UserID, day, p.CurrentVersion); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.verified", p, map[string]any{"until": until})
	})
}

// Unverify removes the mark.
func (s *Service) Unverify(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.requireVerifier(ctx, tx, c, p); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM page_verifications WHERE page_id = $1", id); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.unverified", p, nil)
	})
}

// SetReview sets the date the owner is reminded to check a page. Empty removes it.
func (s *Service) SetReview(ctx context.Context, c identity.Caller, id uuid.UUID, on string) error {
	var day *time.Time
	if on != "" {
		parsed, err := parseDate(on)
		if err != nil {
			return err
		}
		day = &parsed
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := scanPage(tx.QueryRow(ctx, "UPDATE pages SET review_on = $2, review_reminded_on = NULL WHERE id = $1 RETURNING "+pageColumns, id, day))
		if err != nil {
			return err
		}
		return record(ctx, tx, c, "page.review_changed", p, map[string]any{"on": on})
	})
}

// SetReviewDefault sets how many months a new page has before its first review.
func (s *Service) SetReviewDefault(ctx context.Context, c identity.Caller, spaceID uuid.UUID, months int) error {
	if months < 0 || months > maxReviewMonths {
		return apperr.Invalid("A review period is between 0 and 120 months.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE spaces SET review_months = $2 WHERE id = $1", spaceID, months); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, c.Event("space.review_changed", "space", spaceID.String(), spaceID.String(), map[string]any{"months": months}))
		return err
	})
}

// RemindReviews tells owners about pages past their date, once for each date. It
// is an event: wave C turns it into a notification.
func (s *Service) RemindReviews(ctx context.Context) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE pages SET review_reminded_on = review_on
			WHERE review_on <= current_date AND review_reminded_on IS DISTINCT FROM review_on
			  AND status = 'published' AND archived_at IS NULL
			RETURNING id, space_id, COALESCE(owner_id, created_by)`)
		if err != nil {
			return err
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (map[string]any, error) {
			var page, space, owner uuid.UUID
			err := r.Scan(&page, &space, &owner)
			return map[string]any{"page_id": page, "space_id": space, "owner_id": owner}, err
		})
		if err != nil {
			return err
		}
		for _, due := range list {
			if _, err := outbox.Enqueue(ctx, tx, "page.review_due", due); err != nil {
				return err
			}
		}
		return nil
	})
}
