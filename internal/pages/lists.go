// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
)

// The tabs of the Recent screen.
const (
	RecentVisited = "visited"
	RecentWorked  = "worked"
	RecentDrafts  = "drafts"
)

const (
	maxRecentPages    = 50
	maxDraftPages     = 200
	maxFavouritePages = 200
	maxRecentSpaces   = 6
	recentChangeCount = 10
	spaceAuditRows    = 100
)

// ListItem is an entry of a personal list.
type ListItem struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Emoji       string    `json:"emoji"`
	SpaceID     uuid.UUID `json:"space_id"`
	SpaceKey    string    `json:"space_key"`
	SpaceName   string    `json:"space_name"`
	At          time.Time `json:"at"`
	Unpublished bool      `json:"unpublished"`
}

func collectList(rows pgx.Rows, err error) ([]ListItem, error) {
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ListItem, error) {
		var v ListItem
		err := r.Scan(&v.ID, &v.Title, &v.Emoji, &v.SpaceID, &v.SpaceKey, &v.SpaceName, &v.At, &v.Unpublished)
		return v, err
	})
	if out == nil {
		out = []ListItem{}
	}
	return out, err
}

// Recent lists pages for one tab of the Recent screen, as far as the user may
// still see them. Every tab goes through perm_listed_pages.
func (s *Service) Recent(ctx context.Context, c identity.Caller, kind string) ([]ListItem, error) {
	switch kind {
	case "", RecentVisited:
		rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, r.viewed_at, p.status = 'draft'
			FROM recent_pages r
			JOIN perm_listed_pages($1) v ON v.page_id = r.page_id
			JOIN pages p ON p.id = r.page_id
			JOIN spaces s ON s.id = p.space_id
			WHERE r.user_id = $1 ORDER BY r.viewed_at DESC LIMIT $2`, c.UserID, maxRecentPages)
		return collectList(rows, err)
	case RecentWorked:
		rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, w.at, false
			FROM (SELECT page_id, max(at) AS at FROM (
					SELECT page_id, created_at AS at FROM page_versions WHERE author_id = $1
					UNION ALL
					SELECT page_id, created_at FROM comments WHERE author_id = $1 AND trash_item_id IS NULL
				) made GROUP BY page_id) w
			JOIN perm_listed_pages($1) v ON v.page_id = w.page_id
			JOIN pages p ON p.id = w.page_id
			JOIN spaces s ON s.id = p.space_id
			ORDER BY w.at DESC LIMIT $2`, c.UserID, maxRecentPages)
		return collectList(rows, err)
	case RecentDrafts:
		rows, err := s.pool.Query(ctx, `SELECT p.id, COALESCE(NULLIF(d.title, ''), p.title), p.emoji, p.space_id, s.key, s.name,
				COALESCE(d.updated_at, p.updated_at), p.status = 'draft'
			FROM pages p
			JOIN perm_listed_pages($1) v ON v.page_id = p.id
			JOIN spaces s ON s.id = p.space_id
			LEFT JOIN drafts d ON d.page_id = p.id AND d.user_id = $1
			WHERE d.page_id IS NOT NULL OR (p.status = 'draft' AND p.created_by = $1)
			ORDER BY COALESCE(d.updated_at, p.updated_at) DESC LIMIT $2`, c.UserID, maxDraftPages)
		return collectList(rows, err)
	}
	return nil, apperr.Invalid("Recent lists are visited, worked or drafts.")
}

// Favourites lists favourite pages the user may still see. New ones come first,
// then the order the person chose.
func (s *Service) Favourites(ctx context.Context, c identity.Caller) ([]ListItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, f.created_at, p.status = 'draft'
		FROM favourites f
		JOIN perm_listed_pages($1) v ON v.page_id = f.page_id
		JOIN pages p ON p.id = f.page_id
		JOIN spaces s ON s.id = p.space_id
		WHERE f.user_id = $1 ORDER BY f.position NULLS FIRST, f.created_at DESC LIMIT $2`, c.UserID, maxFavouritePages)
	return collectList(rows, err)
}

// AuditCensor says what a reader of the audit log may not see: the title of a page
// they cannot open (ADR-029). A page that is gone counts as one they cannot open.
func (s *Service) AuditCensor(ctx context.Context, c identity.Caller) (func(*audit.Event), error) {
	rows, err := s.pool.Query(ctx, `SELECT page_id::text FROM perm_visible_pages($1)
		UNION SELECT a.id::text FROM attachments a JOIN perm_visible_pages($1) v ON v.page_id = a.page_id`, c.UserID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	visible := make(map[string]bool, len(ids))
	for _, id := range ids {
		visible[id] = true
	}
	return func(e *audit.Event) {
		if id := auditedContent(e); id != "" && !visible[id] {
			e.Details = map[string]any{"title": audit.CensoredTitle}
		}
	}, nil
}

// auditedContent finds the page or file an event is about, if any.
func auditedContent(e *audit.Event) string {
	if e.ObjectType == "page" || e.ObjectType == "attachment" {
		return e.ObjectID
	}
	id, _ := e.Details["page_id"].(string)
	return id
}

// PageNote is what an administrator may know of a page they found in the audit log.
type PageNote struct {
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	SpaceKey   string    `json:"space_key"`
	SpaceName  string    `json:"space_name"`
	Status     string    `json:"status"`
	Restricted bool      `json:"restricted"`
	Readable   bool      `json:"readable"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Note looks up a page by its id, for someone who may not be able to read it. The title
// stays hidden from them, and looking is audited. The route requires administrator.
func (s *Service) Note(ctx context.Context, c identity.Caller, id uuid.UUID) (PageNote, error) {
	note := PageNote{ID: id}
	err := s.pool.QueryRow(ctx, `SELECT p.title, s.key, s.name, p.status, p.updated_at,
			EXISTS (SELECT 1 FROM page_restrictions r WHERE r.page_id = p.id),
			EXISTS (SELECT 1 FROM perm_visible_pages($2) v WHERE v.page_id = p.id)
		FROM pages p JOIN spaces s ON s.id = p.space_id WHERE p.id = $1`, id, c.UserID).
		Scan(&note.Title, &note.SpaceKey, &note.SpaceName, &note.Status, &note.UpdatedAt, &note.Restricted, &note.Readable)
	if errors.Is(err, pgx.ErrNoRows) {
		return note, apperr.NotFound()
	}
	if err != nil {
		return note, err
	}
	if !note.Readable {
		note.Title = audit.CensoredTitle
	}
	return note, audit.RecordStandalone(ctx, s.pool, c.Event("page.looked_up", "page", id.String(), "", nil))
}

// ByPerson lists the pages a person last worked on, as far as the viewer may see
// them: a profile must not reveal a page through its title.
func (s *Service) ByPerson(ctx context.Context, c identity.Caller, personID uuid.UUID) ([]ListItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, w.at, false
		FROM (SELECT page_id, max(created_at) AS at FROM page_versions WHERE author_id = $2 GROUP BY page_id) w
		JOIN perm_visible_pages($1) v ON v.page_id = w.page_id
		JOIN pages p ON p.id = w.page_id
		JOIN spaces s ON s.id = p.space_id
		ORDER BY w.at DESC LIMIT $3`, c.UserID, personID, recentChangeCount)
	return collectList(rows, err)
}

// MentionsOf lists the pages that mention a person, as far as the viewer may see them
// (COL-035).
func (s *Service) MentionsOf(ctx context.Context, c identity.Caller, personID uuid.UUID) ([]ListItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, p.updated_at, false
		FROM page_mentions m
		JOIN perm_visible_pages($1) v ON v.page_id = m.page_id
		JOIN pages p ON p.id = m.page_id
		JOIN spaces s ON s.id = p.space_id
		WHERE m.user_id = $2
		ORDER BY p.updated_at DESC LIMIT $3`, c.UserID, personID, recentChangeCount)
	return collectList(rows, err)
}

// OwnTitles lists the titles of pages a person made and can still see, for the
// download of their own data.
func (s *Service) OwnTitles(ctx context.Context, c identity.Caller) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.title FROM pages p JOIN perm_visible_pages($1) v ON v.page_id = p.id
		WHERE p.created_by = $1 ORDER BY lower(p.title)`, c.UserID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

// SpaceEvent is one row of the audit of a space.
type SpaceEvent struct {
	ID uuid.UUID `json:"id"`
	// Seq is zero while the event waits for the sealer (ADR-041).
	Seq        int64          `json:"seq"`
	OccurredAt time.Time      `json:"occurred_at"`
	ActorID    string         `json:"actor_id"`
	Action     string         `json:"action"`
	ObjectType string         `json:"object_type"`
	ObjectID   string         `json:"object_id"`
	Result     string         `json:"result"`
	Details    map[string]any `json:"details"`
}

// SpaceAudit shows a space administrator what happened in their space. A row about
// a page they may not see shows the action and the time, and nothing else: the
// audit may not reveal a hidden page (NAV-005).
func (s *Service) SpaceAudit(ctx context.Context, c identity.Caller, spaceID uuid.UUID, beforeSeq int64) ([]SpaceEvent, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAudit); err != nil {
		return nil, err
	}
	events, err := audit.List(ctx, s.pool, audit.Filter{SpaceID: spaceID.String(), BeforeSeq: beforeSeq, Limit: spaceAuditRows})
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, "SELECT p.id::text FROM pages p JOIN perm_visible_pages($1) v ON v.page_id = p.id WHERE p.space_id = $2", c.UserID, spaceID)
	if err != nil {
		return nil, err
	}
	seen, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make([]SpaceEvent, 0, len(events))
	for _, e := range events {
		row := SpaceEvent{ID: e.ID, Seq: e.Seq, OccurredAt: e.OccurredAt, ActorID: e.ActorID, Action: e.Action,
			ObjectType: e.ObjectType, ObjectID: e.ObjectID, Result: e.Result, Details: e.Details}
		if e.ObjectType == "page" && !slices.Contains(seen, e.ObjectID) {
			row.ObjectID, row.Details = "", nil
		}
		out = append(out, row)
	}
	return out, nil
}

// SetFavourite adds or removes a favourite. It is a personal preference, so it
// is not audited.
func (s *Service) SetFavourite(ctx context.Context, c identity.Caller, id uuid.UUID, on bool) error {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	var err error
	if on {
		_, err = s.pool.Exec(ctx, "INSERT INTO favourites (user_id, page_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", c.UserID, id)
	} else {
		_, err = s.pool.Exec(ctx, "DELETE FROM favourites WHERE user_id = $1 AND page_id = $2", c.UserID, id)
	}
	return err
}

// OrderFavourites stores the order a person chose. It only touches their own
// rows, so ids of anything else simply do nothing.
func (s *Service) OrderFavourites(ctx context.Context, c identity.Caller, pageIDs []uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE favourites f SET position = o.ord
		FROM unnest($2::uuid[]) WITH ORDINALITY AS o(id, ord)
		WHERE f.user_id = $1 AND f.page_id = o.id`, c.UserID, pageIDs)
	return err
}

// RecentSpaceIDs lists the spaces of recently visited pages, newest first.
func (s *Service) RecentSpaceIDs(ctx context.Context, c identity.Caller) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.space_id FROM recent_pages r
		JOIN perm_listed_pages($1) v ON v.page_id = r.page_id
		JOIN pages p ON p.id = r.page_id
		WHERE r.user_id = $1 GROUP BY p.space_id ORDER BY max(r.viewed_at) DESC LIMIT $2`, c.UserID, maxRecentSpaces*2)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// SpaceActivity is what the overview of a space shows about its pages.
type SpaceActivity struct {
	PageCount int        `json:"page_count"`
	Recent    []ListItem `json:"recent"`
}

// Activity counts and lists the pages of a space the user may see.
func (s *Service) Activity(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (SpaceActivity, error) {
	var out SpaceActivity
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return out, err
	}
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pages p JOIN perm_listed_pages($1) v ON v.page_id = p.id
		WHERE p.space_id = $2 AND p.status = 'published'`, c.UserID, spaceID).Scan(&out.PageCount)
	if err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.title, p.emoji, p.space_id, s.key, s.name, p.updated_at, false
		FROM pages p
		JOIN perm_listed_pages($1) v ON v.page_id = p.id
		JOIN spaces s ON s.id = p.space_id
		WHERE p.space_id = $2 AND p.status = 'published'
		ORDER BY p.updated_at DESC, p.id LIMIT $3`, c.UserID, spaceID, recentChangeCount)
	out.Recent, err = collectList(rows, err)
	return out, err
}
