// SPDX-License-Identifier: Apache-2.0

// Package analytics counts what is read (spec 015, part 1). A page view connects a
// named colleague to what they read, so this is a feature with its own switch, and
// counts mode keeps nothing that points at a person once a day has closed.
package analytics

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
)

// The two privacy modes.
const (
	ModeCounts = "counts"
	ModeNames  = "names"
)

const (
	settingsKey          = "analytics"
	defaultRetentionDays = 365
	minRetentionDays     = 30
	maxRetentionDays     = 3650
	saltBytes            = 32
	countAgainAfter      = 30 * time.Minute
	searchTermDays       = 90
	maxTermRunes         = 100
	minTermUses          = 3
	maxListRows          = 200
)

// Settings are what an administrator chooses.
type Settings struct {
	Enabled       bool   `json:"enabled"`
	Mode          string `json:"mode"`
	RetentionDays int    `json:"retention_days"`
}

func defaults() Settings {
	return Settings{Enabled: false, Mode: ModeCounts, RetentionDays: defaultRetentionDays}
}

// Service records and reports.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	features *features.Service
	now      func() time.Time
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, f *features.Service) *Service {
	return &Service{pool: pool, perms: perms, features: f, now: time.Now}
}

func (s *Service) today() time.Time { return s.now().UTC().Truncate(24 * time.Hour) }

func readSettings(ctx context.Context, q db.Querier) (Settings, error) {
	out := defaults()
	err := settings.Load(ctx, q, settingsKey, &out)
	return out, err
}

// Settings returns the choices of the administrator.
func (s *Service) Settings(ctx context.Context) (Settings, error) { return readSettings(ctx, s.pool) }

func (v Settings) validate() error {
	if v.Mode != ModeCounts && v.Mode != ModeNames {
		return apperr.Invalid("Analytics show counts or names.")
	}
	if v.RetentionDays < minRetentionDays || v.RetentionDays > maxRetentionDays {
		return apperr.Invalid("View data is kept for 30 to 3650 days.")
	}
	return nil
}

// SetSettings stores the choices. Leaving name mode deletes every name in the same
// transaction, so going back can never restore them (PLT-003). The route requires
// administrator.
func (s *Service) SetSettings(ctx context.Context, c identity.Caller, v Settings) error {
	if err := v.validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := settings.Store(ctx, tx, settingsKey, raw); err != nil {
			return err
		}
		if v.Mode != ModeNames {
			if _, err := tx.Exec(ctx, "DELETE FROM page_view_people"); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("analytics.settings_changed", "settings", "", "",
			map[string]any{"enabled": v.Enabled, "mode": v.Mode, "retention_days": v.RetentionDays})); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "analytics.settings_changed", map[string]any{"mode": v.Mode})
		return err
	})
}

// DeleteData removes all view data. The route requires administrator.
func (s *Service) DeleteData(ctx context.Context, c identity.Caller) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, statement := range []string{"DELETE FROM page_view_days", "DELETE FROM page_view_seen", "DELETE FROM analytics_salts",
			"DELETE FROM page_view_people", "DELETE FROM page_view_links", "DELETE FROM analytics_days", "DELETE FROM search_terms"} {
			if _, err := tx.Exec(ctx, statement); err != nil {
				return err
			}
		}
		if _, err := audit.Record(ctx, tx, c.Event("analytics.data_deleted", "settings", "", "", nil)); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "analytics.data_deleted", map[string]any{})
		return err
	})
}

type pageFacts struct {
	spaceID uuid.UUID
	owner   uuid.UUID
	version int
	active  bool
}

func loadFacts(ctx context.Context, q db.Querier, id uuid.UUID) (pageFacts, error) {
	var f pageFacts
	err := q.QueryRow(ctx, `SELECT p.space_id, COALESCE(p.owner_id, p.created_by), p.current_version,
			s.status = 'active' AND p.status = 'published' AND p.archived_at IS NULL
		FROM pages p JOIN spaces s ON s.id = p.space_id WHERE p.id = $1`, id).
		Scan(&f.spaceID, &f.owner, &f.version, &f.active)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, apperr.NotFound()
	}
	return f, err
}

// Visit is one report from a reading screen.
type Visit struct {
	PageID  uuid.UUID
	From    *uuid.UUID
	Engaged bool
}

func (s *Service) daySalt(ctx context.Context, tx pgx.Tx, day time.Time) ([]byte, error) {
	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO analytics_salts (day, salt) VALUES ($1, $2) ON CONFLICT DO NOTHING", day, salt); err != nil {
		return nil, err
	}
	err := tx.QueryRow(ctx, "SELECT salt FROM analytics_salts WHERE day = $1", day).Scan(&salt)
	return salt, err
}

func readerHash(salt []byte, user uuid.UUID) []byte {
	mac := hmac.New(sha256.New, salt)
	mac.Write(user[:])
	return mac.Sum(nil)
}

// Record counts a visit, when analytics are on for this page. It is private to the
// person and not audited: page views stay out of the audit log on purpose.
func (s *Service) Record(ctx context.Context, c identity.Caller, v Visit) error {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, v.PageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	st, err := s.Settings(ctx)
	if err != nil || !st.Enabled {
		return err
	}
	f, err := loadFacts(ctx, s.pool, v.PageID)
	if err != nil {
		return err
	}
	on, err := s.features.Enabled(ctx, s.pool, f.spaceID, features.Analytics)
	if err != nil || !on || !f.active || f.owner == c.UserID {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		day := s.today()
		salt, err := s.daySalt(ctx, tx, day)
		if err != nil {
			return err
		}
		reader := readerHash(salt, c.UserID)
		if v.Engaged {
			return s.markEngaged(ctx, tx, day, reader, v.PageID)
		}
		return s.countOpen(ctx, tx, st, c.UserID, day, reader, f, v)
	})
}

func (s *Service) markEngaged(ctx context.Context, tx pgx.Tx, day time.Time, reader []byte, page uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE page_view_seen SET engaged = true
		WHERE day = $1 AND reader = $2 AND page_id = $3 AND NOT engaged`, day, reader, page)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	_, err = tx.Exec(ctx, "UPDATE page_view_days SET engaged = engaged + 1 WHERE page_id = $1 AND day = $2", page, day)
	return err
}

func (s *Service) countOpen(ctx context.Context, tx pgx.Tx, st Settings, user uuid.UUID, day time.Time, reader []byte, f pageFacts, v Visit) error {
	now := s.now()
	var last time.Time
	err := tx.QueryRow(ctx, "SELECT last_at FROM page_view_seen WHERE day = $1 AND reader = $2 AND page_id = $3 FOR UPDATE",
		day, reader, v.PageID).Scan(&last)
	first := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !first {
		return err
	}
	if !first && now.Sub(last) < countAgainAfter {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_view_seen (day, reader, page_id, last_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (day, reader, page_id) DO UPDATE SET last_at = EXCLUDED.last_at`, day, reader, v.PageID, now); err != nil {
		return err
	}
	readers := 0
	if first {
		readers = 1
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_view_days (page_id, day, views, readers) VALUES ($1, $2, 1, $3)
		ON CONFLICT (page_id, day) DO UPDATE SET views = page_view_days.views + 1, readers = page_view_days.readers + $3`,
		v.PageID, day, readers); err != nil {
		return err
	}
	if v.From != nil && *v.From != v.PageID {
		if _, err := tx.Exec(ctx, `INSERT INTO page_view_links (day, from_id, to_id) SELECT $1, id, $3 FROM pages WHERE id = $2
			ON CONFLICT (day, from_id, to_id) DO UPDATE SET count = page_view_links.count + 1`, day, *v.From, v.PageID); err != nil {
			return err
		}
	}
	if st.Mode != ModeNames {
		return nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO page_view_people (page_id, user_id, first_at, last_at, last_version) VALUES ($1, $2, $3, $3, $4)
		ON CONFLICT (page_id, user_id) DO UPDATE SET last_at = EXCLUDED.last_at, views = page_view_people.views + 1,
			last_version = EXCLUDED.last_version`, v.PageID, user, now, f.version)
	return err
}

// RecordSearch counts a search term, without any person. It must not fail a search,
// so the caller logs what comes back.
func (s *Service) RecordSearch(ctx context.Context, text string, found int) error {
	st, err := s.Settings(ctx)
	if err != nil || !st.Enabled {
		return err
	}
	term := strings.ToLower(strings.TrimSpace(text))
	if term == "" || !utf8.ValidString(term) || utf8.RuneCountInString(term) > maxTermRunes {
		return nil
	}
	empty := 0
	if found == 0 {
		empty = 1
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO search_terms (day, term, empty) VALUES ($1, $2, $3)
		ON CONFLICT (day, term) DO UPDATE SET searches = search_terms.searches + 1, empty = search_terms.empty + $3`,
		s.today(), term, empty)
	return err
}

// CloseDays is the daily task: it rolls up finished days and removes the hashes and
// salts of those days, after which nothing can be traced back.
func (s *Service) CloseDays(ctx context.Context) error {
	st, err := s.Settings(ctx)
	if err != nil {
		return err
	}
	today := s.today()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO analytics_days (day, active_users, views, new_pages, comments, searches)
			SELECT d.day,
				(SELECT count(DISTINCT reader) FROM page_view_seen s WHERE s.day = d.day),
				COALESCE((SELECT sum(views) FROM page_view_days v WHERE v.day = d.day), 0),
				(SELECT count(*) FROM pages p WHERE p.created_at::date = d.day),
				(SELECT count(*) FROM comments c WHERE c.created_at::date = d.day),
				COALESCE((SELECT sum(searches) FROM search_terms t WHERE t.day = d.day), 0)
			FROM (SELECT day FROM analytics_salts WHERE day < $1) d
			ON CONFLICT (day) DO UPDATE SET active_users = EXCLUDED.active_users, views = EXCLUDED.views,
				new_pages = EXCLUDED.new_pages, comments = EXCLUDED.comments, searches = EXCLUDED.searches`, today); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM page_view_seen WHERE day < $1", today); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM analytics_salts WHERE day < $1", today); err != nil {
			return err
		}
		keepFrom := today.AddDate(0, 0, -st.RetentionDays)
		for _, statement := range []string{"DELETE FROM page_view_days WHERE day < $1", "DELETE FROM page_view_links WHERE day < $1",
			"DELETE FROM analytics_days WHERE day < $1", "DELETE FROM page_view_people WHERE last_at < $1"} {
			if _, err := tx.Exec(ctx, statement, keepFrom); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "DELETE FROM search_terms WHERE day < $1", today.AddDate(0, 0, -searchTermDays))
		return err
	})
}
