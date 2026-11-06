// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	maxDuePerRun      = 100
	defaultDraftDays  = 365
	maxDraftDays      = 3650
	draftWarningDays  = 14
	recentEditMinutes = 10
)

// Scheduled says when a page will be published by itself.
type Scheduled struct {
	At     time.Time `json:"at"`
	Silent bool      `json:"silent"`
}

func scheduleOf(ctx context.Context, q db.Querier, pageID uuid.UUID) (*Scheduled, error) {
	var v Scheduled
	err := q.QueryRow(ctx, "SELECT publish_at, silent FROM scheduled_publications WHERE page_id = $1", pageID).Scan(&v.At, &v.Silent)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// Schedule publishes the person's draft at a later moment. Editing goes on: the
// draft of that moment is what goes out (PAG-026).
func (s *Service) Schedule(ctx context.Context, c identity.Caller, id uuid.UUID, at time.Time, silent bool) error {
	if !at.After(time.Now()) {
		return apperr.Invalid("A page is scheduled for a moment in the future.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO scheduled_publications (page_id, user_id, publish_at, silent) VALUES ($1, $2, $3, $4)
			ON CONFLICT (page_id) DO UPDATE SET user_id = EXCLUDED.user_id, publish_at = EXCLUDED.publish_at, silent = EXCLUDED.silent`,
			id, c.UserID, at, silent); err != nil {
			return err
		}
		return record(ctx, tx, c, "page.scheduled", p, map[string]any{"at": at})
	})
}

// Unschedule cancels a scheduled publication.
func (s *Service) Unschedule(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
			return err
		}
		p, err := lockPage(ctx, tx, id)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM scheduled_publications WHERE page_id = $1", id)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return record(ctx, tx, c, "page.unscheduled", p, nil)
	})
}

type duePublication struct {
	page   uuid.UUID
	user   uuid.UUID
	silent bool
}

// textToPublish is the person's draft of this moment, or else the current text.
func (s *Service) textToPublish(ctx context.Context, due duePublication) (string, json.RawMessage, int, error) {
	p, err := load(ctx, s.pool, due.page)
	if err != nil {
		return "", nil, 0, err
	}
	var title string
	var doc json.RawMessage
	err = s.pool.QueryRow(ctx, "SELECT title, document FROM drafts WHERE page_id = $1 AND user_id = $2", due.page, due.user).Scan(&title, &doc)
	if err == nil {
		if title == "" {
			title = p.Title
		}
		return title, doc, p.CurrentVersion, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", nil, 0, err
	}
	if p.CurrentVersion == 0 {
		return p.Title, json.RawMessage(`{"type":"doc","content":[{"type":"paragraph"}]}`), 0, nil
	}
	err = s.pool.QueryRow(ctx, "SELECT title, document FROM page_versions WHERE page_id = $1 AND number = $2", due.page, p.CurrentVersion).Scan(&title, &doc)
	return title, doc, p.CurrentVersion, err
}

func (s *Service) publishDue(ctx context.Context, due duePublication) error {
	title, doc, base, err := s.textToPublish(ctx, due)
	if err == nil {
		// Under the rights of this moment, as the person who asked for it.
		_, err = s.Publish(ctx, identity.Caller{UserID: due.user}, due.page, PublishInput{Title: title, Document: doc,
			BaseVersion: base, Overwrite: true, Silent: due.silent})
	}
	if err == nil {
		return nil
	}
	if _, ok := apperr.As(err); !ok {
		return err
	}
	// Refused: it stops here, and the person is told.
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM scheduled_publications WHERE page_id = $1", due.page); err != nil {
			return err
		}
		_, err := outbox.Enqueue(ctx, tx, "page.schedule_failed", map[string]any{"page_id": due.page, "user_id": due.user})
		return err
	})
}

// PublishDue publishes what was scheduled for now. Each page stands on its own, so
// one failure does not stop the rest.
func (s *Service) PublishDue(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT page_id, user_id, silent FROM scheduled_publications
		WHERE publish_at <= now() ORDER BY publish_at LIMIT $1`, maxDuePerRun)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (duePublication, error) {
		var d duePublication
		err := r.Scan(&d.page, &d.user, &d.silent)
		return d, err
	})
	if err != nil {
		return err
	}
	var failed error
	for _, due := range list {
		if err := s.publishDue(ctx, due); err != nil {
			failed = errors.Join(failed, err)
		}
	}
	return failed
}

// ExpireDrafts warns about old drafts and removes them after the warning. A
// draft is private, so this is an event for the person and not an audit entry.
func (s *Service) ExpireDrafts(ctx context.Context) error {
	days := defaultDraftDays
	if err := s.settings.Read(ctx, draftRetentionKey, &days); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM drafts WHERE updated_at < now() - make_interval(days => $1)`, days); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `UPDATE drafts SET warned_at = now()
			WHERE warned_at IS NULL AND updated_at < now() - make_interval(days => $1)
			RETURNING page_id, user_id`, days-draftWarningDays)
		if err != nil {
			return err
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (map[string]any, error) {
			var page, user uuid.UUID
			err := r.Scan(&page, &user)
			return map[string]any{"page_id": page, "user_id": user}, err
		})
		if err != nil {
			return err
		}
		for _, warned := range list {
			if _, err := outbox.Enqueue(ctx, tx, "page.draft_expiring", warned); err != nil {
				return err
			}
		}
		return nil
	})
}

const draftRetentionKey = "draft_retention_days"

// OtherEditor is someone who saved a draft of a page a moment ago.
type OtherEditor struct {
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

// OtherEditors warns about people working on the same page (PAG-034).
func (s *Service) OtherEditors(ctx context.Context, c identity.Caller, id uuid.UUID) ([]OtherEditor, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.PageEdit, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT COALESCE(u.display_name, ''), d.updated_at FROM drafts d
		LEFT JOIN users u ON u.id = d.user_id
		WHERE d.page_id = $1 AND d.user_id <> $2 AND d.updated_at > now() - make_interval(mins => $3)
		ORDER BY d.updated_at DESC`, id, c.UserID, recentEditMinutes)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[OtherEditor])
	if out == nil {
		out = []OtherEditor{}
	}
	return out, err
}

// SameTitle finds a page with this title in a space, for the warning in the editor
// (PAG-008). It only answers with a page the person can see.
func (s *Service) SameTitle(ctx context.Context, c identity.Caller, spaceID, except uuid.UUID, title string) (*PageRef, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+refColumns+` FROM pages p `+refJoins+`
		WHERE p.space_id = $2 AND lower(p.title) = lower($3) AND p.id <> $4 AND p.status = 'published'
		ORDER BY p.id LIMIT 1`, c.UserID, spaceID, title, except)
	list, err := collectRefs(rows, err)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}
