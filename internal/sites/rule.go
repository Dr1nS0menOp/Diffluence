// SPDX-License-Identifier: Apache-2.0

package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
)

// publicKinds are what a site can show. A template is no page of the space, and a
// folder or a link has nothing to read.
var publicKinds = []string{pages.KindPage, pages.KindPost}

// eventsThatCanHide are all that can take a page's right to be public away (GAP-015).
var eventsThatCanHide = []string{"restriction.added", "permission.restriction.added", "page.moved", "page.archived", "page.trashed",
	"page.purged", "page.version_removed", "space.archived", "space.trashed", "space.purged"}

// mayBePublic is the one rule for what the world may see (PRM-096, PRM-098): chosen
// pages of this space that are published, not archived, in a space that is in use,
// with no view restriction on them or above them. It answers with those that pass.
// No person's rights come into it.
func (s *Service) mayBePublic(ctx context.Context, q db.Querier, spaceID uuid.UUID, list []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := q.Query(ctx, `SELECT p.id FROM pages p JOIN spaces sp ON sp.id = p.space_id
		WHERE p.id = ANY($1) AND p.space_id = $2 AND p.status = 'published' AND p.archived_at IS NULL AND sp.status = 'active' AND p.kind = ANY($3)
		AND NOT EXISTS (SELECT 1 FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view'
			WHERE t.descendant_id = p.id)`, list, spaceID, publicKinds)
	if err != nil {
		return nil, err
	}
	allowed, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	return uniqueIDs(allowed), nil
}

// versionsGone lists what a build shows of a version that was removed since.
func (s *Service) versionsGone(ctx context.Context, shown map[uuid.UUID]int) (bool, error) {
	for page, number := range shown {
		var exists bool
		err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM page_versions WHERE page_id = $1 AND number = $2)`, page, number).Scan(&exists)
		if err != nil {
			return false, err
		}
		if !exists {
			return true, nil
		}
	}
	return false, nil
}

// Handle reacts to events. A page that may no longer be public leaves at once, and
// a published page goes out with the next build of a site that wants that.
func (s *Service) Handle(ctx context.Context, m outbox.Message) error {
	if slices.Contains(eventsThatCanHide, m.Type) {
		return s.Enforce(ctx)
	}
	if m.Type != "page.published" {
		return nil
	}
	var p struct {
		PageID uuid.UUID `json:"page_id"`
	}
	if err := json.Unmarshal(m.Payload, &p); err != nil {
		return fmt.Errorf("sites: read %s: %w", m.Type, err)
	}
	_, err := s.pool.Exec(ctx, `UPDATE public_sites SET build_requested = now() + $2::interval
		WHERE update_mode = $3 AND live_build > 0 AND withdrawn_at IS NULL
		AND id IN (SELECT site_id FROM public_site_pages WHERE page_id = $1)`, p.PageID, publishDebounce.String(), UpdateOnPublish)
	return err
}

// system is who acts when the rule itself takes something down.
func system() identity.Caller {
	return identity.Caller{Meta: identity.RequestMeta{RequestID: "task-public-rule"}}
}

// Enforce holds every live site and link to the rule. It builds at once, and when
// that fails it takes the whole site down: a page that may not be public must never
// wait for someone to fix a build.
func (s *Service) Enforce(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT `+siteColumns+` FROM public_sites WHERE live_build > 0 AND withdrawn_at IS NULL`)
	if err != nil {
		return err
	}
	live, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Site, error) { return scan(r) })
	if err != nil {
		return err
	}
	for _, v := range live {
		if err := s.enforceSite(ctx, v); err != nil {
			return err
		}
	}
	return s.enforceLinks(ctx)
}

func (s *Service) enforceSite(ctx context.Context, v Site) error {
	report, err := s.liveReport(ctx, v)
	if err != nil {
		return err
	}
	shown := make([]uuid.UUID, 0, len(report.Versions))
	for page := range report.Versions {
		shown = append(shown, page)
	}
	allowed, err := s.mayBePublic(ctx, s.pool, v.SpaceID, shown)
	if err != nil {
		return err
	}
	gone, err := s.versionsGone(ctx, report.Versions)
	if err != nil {
		return err
	}
	if len(allowed) == len(shown) && !gone {
		return nil
	}
	if _, err := s.build(ctx, system(), v); err == nil {
		return nil
	}
	s.options.Log.Error("a site could not be rebuilt and was taken down", "site", v.ID)
	return s.withdraw(ctx, system(), v)
}

// RunDue builds sites whose time has come, on publish after the pause, or daily.
func (s *Service) RunDue(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `UPDATE public_sites SET build_requested = NULL
		WHERE build_requested <= now() AND withdrawn_at IS NULL RETURNING `+siteColumns)
	if err != nil {
		return err
	}
	due, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Site, error) { return scan(r) })
	if err != nil {
		return err
	}
	for _, v := range due {
		if _, err := s.build(ctx, system(), v); err != nil {
			s.options.Log.Error("a site build failed", "site", v.ID)
		}
	}
	return nil
}

// RunDaily asks for a build of the sites that update once a day.
func (s *Service) RunDaily(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE public_sites SET build_requested = $1
		WHERE update_mode = $2 AND live_build > 0 AND withdrawn_at IS NULL`, time.Now(), UpdateDaily)
	if err != nil {
		return err
	}
	return s.RunDue(ctx)
}

// PublicState tells readers in the wiki that a page can be seen by the world (EXC-043).
type PublicState struct {
	Public bool `json:"public"`
}

// StateOf says whether a page is on a live site or has a live public link.
func (s *Service) StateOf(ctx context.Context, c identity.Caller, pageID uuid.UUID) (PublicState, error) {
	if _, err := s.pages.SpaceOf(ctx, c, pageID); err != nil {
		return PublicState{}, err
	}
	var out PublicState
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public_site_pages k JOIN public_sites v ON v.id = k.site_id
			WHERE k.page_id = $1 AND v.withdrawn_at IS NULL)
		OR EXISTS (SELECT 1 FROM public_links WHERE page_id = $1 AND withdrawn_at IS NULL)`, pageID).Scan(&out.Public)
	return out, err
}
