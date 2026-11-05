// SPDX-License-Identifier: Apache-2.0

// Package watches holds who wants to hear about what, and who follows whom
// (spec 010, C9 and C15). A person's own watch and follow are personal, so they are
// not audited. A space administrator changing someone else's watch is.
package watches

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/permissions"
)

// What can be watched.
const (
	TargetPage   = "page"
	TargetBranch = "branch"
	TargetSpace  = "space"
)

// Service manages watches and follows.
type Service struct {
	pool  *pgxpool.Pool
	perms *permissions.Service
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, perms *permissions.Service) *Service {
	return &Service{pool: pool, perms: perms}
}

func add(ctx context.Context, q db.Querier, user uuid.UUID, target string, id uuid.UUID) error {
	_, err := q.Exec(ctx, "INSERT INTO watches (user_id, target_type, target_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", user, target, id)
	return err
}

func remove(ctx context.Context, q db.Querier, user uuid.UUID, target string, id uuid.UUID) error {
	_, err := q.Exec(ctx, "DELETE FROM watches WHERE user_id = $1 AND target_type = $2 AND target_id = $3", user, target, id)
	return err
}

// Auto starts watching a page because someone worked on it. The worker calls it.
func (s *Service) Auto(ctx context.Context, user, pageID uuid.UUID) error {
	return add(ctx, s.pool, user, TargetPage, pageID)
}

func checkPageTarget(target string) error {
	if target != TargetPage && target != TargetBranch {
		return apperr.Invalid("A page is watched alone or with everything below it.")
	}
	return nil
}

// WatchPage starts or stops watching a page, or a page with everything below it.
func (s *Service) WatchPage(ctx context.Context, c identity.Caller, pageID uuid.UUID, target string, on bool) error {
	if target == "" {
		target = TargetPage
	}
	if err := checkPageTarget(target); err != nil {
		return err
	}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	if on {
		return add(ctx, s.pool, c.UserID, target, pageID)
	}
	return remove(ctx, s.pool, c.UserID, target, pageID)
}

// WatchSpace starts or stops watching a space.
func (s *Service) WatchSpace(ctx context.Context, c identity.Caller, spaceID uuid.UUID, on bool) error {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return err
	}
	if on {
		return add(ctx, s.pool, c.UserID, TargetSpace, spaceID)
	}
	return remove(ctx, s.pool, c.UserID, TargetSpace, spaceID)
}

// State says what a person watches around a page.
type State struct {
	Page   bool `json:"page"`
	Branch bool `json:"branch"`
	Space  bool `json:"space"`
}

// PageState tells the page screen what to offer.
func (s *Service) PageState(ctx context.Context, c identity.Caller, pageID uuid.UUID) (State, error) {
	var out State
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return out, err
	}
	err := s.pool.QueryRow(ctx, `SELECT
			EXISTS (SELECT 1 FROM watches WHERE user_id = $1 AND target_type = 'page' AND target_id = $2),
			EXISTS (SELECT 1 FROM watches WHERE user_id = $1 AND target_type = 'branch' AND target_id = $2),
			EXISTS (SELECT 1 FROM watches w JOIN pages p ON p.space_id = w.target_id
				WHERE w.user_id = $1 AND w.target_type = 'space' AND p.id = $2)`, c.UserID, pageID).
		Scan(&out.Page, &out.Branch, &out.Space)
	return out, err
}

// SpaceWatched says whether a person watches a space.
func (s *Service) SpaceWatched(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (bool, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return false, err
	}
	var on bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM watches
		WHERE user_id = $1 AND target_type = 'space' AND target_id = $2)`, c.UserID, spaceID).Scan(&on)
	return on, err
}

// Entry is one row of the Watching screen.
type Entry struct {
	Type      string    `json:"type"`
	ID        uuid.UUID `json:"id"`
	Title     string    `json:"title"`
	SpaceName string    `json:"space_name"`
	Since     time.Time `json:"since"`
}

// Mine lists what a person watches, as far as they can still see it. A row out of
// reach stays stored, and works again when access returns.
func (s *Service) Mine(ctx context.Context, c identity.Caller) ([]Entry, error) {
	rows, err := s.pool.Query(ctx, `SELECT w.target_type, w.target_id, p.title, sp.name, w.created_at
			FROM watches w
			JOIN perm_visible_pages($1) v ON v.page_id = w.target_id
			JOIN pages p ON p.id = w.target_id
			JOIN spaces sp ON sp.id = p.space_id
			WHERE w.user_id = $1 AND w.target_type IN ('page', 'branch')
		UNION ALL
		SELECT w.target_type, w.target_id, sp.name, sp.name, w.created_at
			FROM watches w
			JOIN spaces sp ON sp.id = w.target_id
			WHERE w.user_id = $1 AND w.target_type = 'space'
			  AND sp.id IN (SELECT space_id FROM perm_spaces($1, 'space.view'))
		ORDER BY 5 DESC LIMIT 500`, c.UserID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Entry, error) {
		var e Entry
		err := r.Scan(&e.Type, &e.ID, &e.Title, &e.SpaceName, &e.Since)
		return e, err
	})
	if out == nil {
		out = []Entry{}
	}
	return out, err
}

// Watcher is someone who hears about a page.
type Watcher struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Watchers lists who watches a page directly. Space administrators only.
func (s *Service) Watchers(ctx context.Context, c identity.Caller, pageID uuid.UUID) ([]Watcher, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.SpaceAdminister, uuid.Nil); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.display_name FROM watches w JOIN users u ON u.id = w.user_id
		WHERE w.target_type = 'page' AND w.target_id = $1 ORDER BY lower(u.display_name), u.id LIMIT 500`, pageID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Watcher, error) {
		var w Watcher
		err := r.Scan(&w.ID, &w.Name)
		return w, err
	})
	if out == nil {
		out = []Watcher{}
	}
	return out, err
}

// SetWatcher lets a space administrator add or remove someone. It is audited, because
// it changes what another person receives.
func (s *Service) SetWatcher(ctx context.Context, c identity.Caller, pageID, user uuid.UUID, on bool) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, pageID, permissions.SpaceAdminister, uuid.Nil); err != nil {
			return err
		}
		var spaceID uuid.UUID
		if err := tx.QueryRow(ctx, "SELECT space_id FROM pages WHERE id = $1", pageID).Scan(&spaceID); err != nil {
			return err
		}
		var known bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", user).Scan(&known); err != nil {
			return err
		}
		if !known {
			return apperr.Invalid("That person does not exist.")
		}
		action := "watcher.removed"
		var err error
		if on {
			action = "watcher.added"
			err = add(ctx, tx, user, TargetPage, pageID)
		} else {
			err = remove(ctx, tx, user, TargetPage, pageID)
		}
		if err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, c.Event(action, "page", pageID.String(), spaceID.String(), map[string]any{"user_id": user})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, action, map[string]any{"page_id": pageID, "space_id": spaceID, "user_id": user, "actor_id": c.UserID})
		return err
	})
}

// Who lists the people who watch a page, its branch or its space. Access is checked
// by the caller, per person: a row out of reach must not hear anything.
func Who(ctx context.Context, q db.Querier, pageID, spaceID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT user_id FROM watches
		WHERE (target_type = 'page' AND target_id = $1)
		   OR (target_type = 'space' AND target_id = $2)
		   OR (target_type = 'branch' AND target_id IN (SELECT ancestor_id FROM page_tree WHERE descendant_id = $1))`, pageID, spaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// StopCovering ends every watch that brings a page to a person: on the page, on a branch
// above it, and on its space. A link in a mail promises that this page goes quiet.
func StopCovering(ctx context.Context, q db.Querier, user, pageID, spaceID uuid.UUID) error {
	_, err := q.Exec(ctx, `DELETE FROM watches WHERE user_id = $1 AND (
		   (target_type = 'page' AND target_id = $2)
		OR (target_type = 'space' AND target_id = $3)
		OR (target_type = 'branch' AND target_id IN (SELECT ancestor_id FROM page_tree WHERE descendant_id = $2)))`, user, pageID, spaceID)
	return err
}

// Follow starts or stops following a person. It only feeds For you.
func (s *Service) Follow(ctx context.Context, c identity.Caller, person uuid.UUID, on bool) error {
	if person == c.UserID {
		return apperr.Invalid("You cannot follow yourself.")
	}
	if !on {
		_, err := s.pool.Exec(ctx, "DELETE FROM follows WHERE user_id = $1 AND followed_id = $2", c.UserID, person)
		return err
	}
	if err := s.maySee(ctx, c, person); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, "INSERT INTO follows (user_id, followed_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", c.UserID, person)
	return err
}

// maySee is the rule for seeing a person. Without view profiles every id gets the
// answer of an id that does not exist, so nobody can test who is here.
func (s *Service) maySee(ctx context.Context, c identity.Caller, person uuid.UUID) error {
	facts, err := s.perms.LoadUser(ctx, s.pool, c.UserID)
	if err != nil {
		return err
	}
	var known bool
	err = s.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND status = 'active' AND type <> 'service')", person).Scan(&known)
	if err != nil {
		return err
	}
	if !known || !slices.Contains(facts.Global, identity.PermViewProfiles) {
		return apperr.NotFound()
	}
	return nil
}

// Following lists who a person follows.
func (s *Service) Following(ctx context.Context, c identity.Caller) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, "SELECT followed_id FROM follows WHERE user_id = $1", c.UserID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if out == nil {
		out = []uuid.UUID{}
	}
	return out, err
}
