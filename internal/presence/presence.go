// SPDX-License-Identifier: Apache-2.0

// Package presence says who else is on a page (spec 015, part 4). An entry lives for
// a minute: this is not a record of who was where, so it is not audited.
package presence

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/settings"
)

// The two things someone can be doing.
const (
	ModeReading = "reading"
	ModeEditing = "editing"
)

const (
	entryLifetime = 60 * time.Second
	maxPeopleSent = 50
)

// Person is someone else on the page.
type Person struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Mode string    `json:"mode"`
}

// Answer is who is here, and a stamp that changes when comments, labels or the
// status do, so a screen knows when to refresh (COL-097).
type Answer struct {
	People []Person `json:"people"`
	Stamp  string   `json:"stamp"`
}

// Beat is one report from an open page.
type Beat struct {
	PageID uuid.UUID
	Mode   string
}

// Service keeps the entries.
type Service struct {
	pool     *pgxpool.Pool
	perms    *permissions.Service
	identity *identity.Service
	settings *settings.Service
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service, id *identity.Service, st *settings.Service) *Service {
	return &Service{pool: pool, perms: perms, identity: id, settings: st}
}

func (s *Service) enabled(ctx context.Context) (bool, error) {
	f := settings.DefaultFeatures
	err := s.settings.ReadSection(ctx, settings.SectionFeatures, &f)
	return f.Presence, err
}

// Beat says this person is here, unless they turned that off, and answers who else
// is. Everyone who hears it can see the page, and may see that person.
func (s *Service) Beat(ctx context.Context, c identity.Caller, b Beat) (Answer, error) {
	out := Answer{People: []Person{}}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, b.PageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return out, err
	}
	if b.Mode != ModeReading && b.Mode != ModeEditing {
		return out, apperr.Invalid("Someone is reading or editing.")
	}
	var err error
	if out.Stamp, err = s.stamp(ctx, b.PageID); err != nil {
		return out, err
	}
	on, err := s.enabled(ctx)
	if err != nil || !on {
		return out, err
	}
	if err := s.store(ctx, c.UserID, b); err != nil {
		return out, err
	}
	out.People, err = s.others(ctx, c.UserID, b.PageID)
	return out, err
}

func (s *Service) store(ctx context.Context, userID uuid.UUID, b Beat) error {
	prefs, err := s.identity.Preferences(ctx, userID)
	if err != nil {
		return err
	}
	if !prefs.ShowPresence {
		_, err := s.pool.Exec(ctx, "DELETE FROM page_presence WHERE user_id = $1", userID)
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO page_presence (page_id, user_id, mode) VALUES ($1, $2, $3)
		ON CONFLICT (page_id, user_id) DO UPDATE SET mode = EXCLUDED.mode, seen_at = now()`, b.PageID, userID, b.Mode)
	return err
}

func (s *Service) others(ctx context.Context, userID, pageID uuid.UUID) ([]Person, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.display_name, e.mode FROM page_presence e JOIN users u ON u.id = e.user_id
		WHERE e.page_id = $1 AND e.user_id <> $2 AND e.seen_at > now() - $3::interval
		ORDER BY e.mode, u.display_name LIMIT $4`, pageID, userID, entryLifetime.String(), maxPeopleSent)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Person, error) {
		var p Person
		err := r.Scan(&p.ID, &p.Name, &p.Mode)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	out := []Person{}
	for _, p := range list {
		// Someone who lost the page within the last minute must not stay listed, and
		// a guest does not get to see everyone.
		d, err := s.perms.CheckPage(ctx, s.pool, p.ID, pageID, permissions.ActionPageView, uuid.Nil)
		if err != nil {
			return nil, err
		}
		seen, err := s.perms.CheckPerson(ctx, s.pool, userID, p.ID)
		if err != nil {
			return nil, err
		}
		if d.Allowed && seen {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Service) stamp(ctx context.Context, pageID uuid.UUID) (string, error) {
	var stamp string
	err := s.pool.QueryRow(ctx, `SELECT md5(concat_ws('|', p.stage_text, p.current_version,
			(SELECT count(*) FROM comments c WHERE c.page_id = p.id),
			(SELECT count(*) FROM comments c WHERE c.page_id = p.id AND c.status = 'resolved'),
			(SELECT max(COALESCE(c.edited_at, c.created_at)) FROM comments c WHERE c.page_id = p.id),
			(SELECT string_agg(l.label, ',' ORDER BY l.label) FROM page_labels l WHERE l.page_id = p.id)))
		FROM pages p WHERE p.id = $1`, pageID).Scan(&stamp)
	return stamp, err
}

// Forget removes entries past their minute. The periodic task calls it, so nothing
// about who was here stays behind.
func (s *Service) Forget(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, "DELETE FROM page_presence WHERE seen_at < now() - $1::interval", entryLifetime.String())
	return err
}
