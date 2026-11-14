// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/pages"
)

// The release screen of a draft space (PUB-1103, PUB-1210 to PUB-1220): every page with its
// status, read from the page pairs in one query, paged by 200, the page that waited longest
// first.

// screenPage is the number of rows the release screen shows at once (PUB-1103).
const screenPage = 200

// The tabs of the release screen (PUB-1210).
const (
	TabToRelease = "to_release"
	TabLeftOut   = "left_out"
	TabRemoved   = "removed"
)

var tabStatuses = map[string][]string{
	TabToRelease: {StatusNotReleased, StatusChanged, StatusCannot},
	TabLeftOut:   {StatusLeftOut, StatusLeftOutReleased},
	TabRemoved:   {StatusRemoved},
}

// ScreenRow is one page of the release screen.
type ScreenRow struct {
	PageID          uuid.UUID  `json:"page_id"`
	TwinPageID      *uuid.UUID `json:"twin_page_id,omitempty"`
	Title           string     `json:"title"`
	Emoji           string     `json:"emoji"`
	Path            []string   `json:"path"`
	Status          string     `json:"status"`
	Aspects         []string   `json:"aspects"`
	Reason          string     `json:"reason,omitempty"`
	ReleasedVersion int        `json:"released_version"`
	CurrentVersion  int        `json:"current_version"`
	ReleasedAt      *time.Time `json:"released_at"`
	Since           time.Time  `json:"since"`
	LeftOut         *Exclusion `json:"left_out,omitempty"`
	// TwinRestricted says that the twin has a restriction of the published space, so some
	// readers do not get it (PUB-222).
	TwinRestricted bool `json:"twin_restricted"`
}

// Tiles are the counts above the list (PUB-1210).
type Tiles struct {
	NotReleased int `json:"not_released"`
	Changed     int `json:"changed"`
	Cannot      int `json:"cannot_release"`
	Removed     int `json:"removed"`
	LeftOut     int `json:"left_out"`
}

// Screen is one part of the release screen.
type Screen struct {
	Tiles Tiles       `json:"tiles"`
	Rows  []ScreenRow `json:"rows"`
	More  bool        `json:"more"`
	// Going is a run of the pair that waits or goes, whose progress the screen shows.
	Going *Run `json:"going"`
}

// screenFacts are the pages of a draft space with their status, for one person: the rule of
// perm_visible_pages for someone who views the space, which a releaser does; and the twins
// whose draft page left for good (PUB-331).
const screenFacts = `WITH facts AS (
	SELECT p.id, pp.twin_page_id, p.title, p.emoji, COALESCE(pp.released_version, 0) AS released_version, p.current_version,
		pp.released_at, COALESCE(pp.changed_since, p.updated_at) AS since, COALESCE(pp.changed, '{}') AS changed,
		CASE
			WHEN p.status = 'trashed' THEN 'removed'
			WHEN x.draft_page_id IS NOT NULL THEN
				CASE WHEN COALESCE(pp.released_version, 0) > 0 THEN 'left_out_released' ELSE 'left_out' END
			WHEN p.archived_at IS NOT NULL AND COALESCE(pp.released_version, 0) > 0 THEN
				CASE WHEN tw.archived_at IS NULL THEN 'removed' ELSE 'released' END
			WHEN p.kind NOT IN ('page', 'folder', 'link') OR p.archived_at IS NOT NULL
				OR EXISTS (SELECT 1 FROM page_control pc WHERE pc.page_id = p.id AND (pc.approved_version IS NULL OR pc.state_kind = 'obsolete'))
				THEN 'cannot_release'
			WHEN COALESCE(pp.released_version, 0) = 0 THEN 'not_released'
			WHEN cardinality(pp.changed) > 0 THEN 'changed'
			ELSE 'released' END AS status,
		CASE WHEN p.status = 'trashed' THEN 'deleted' WHEN p.archived_at IS NOT NULL THEN 'archived'
			WHEN p.kind NOT IN ('page', 'folder', 'link') THEN 'kind'
			WHEN EXISTS (SELECT 1 FROM page_control pc WHERE pc.page_id = p.id) THEN 'not_approved' ELSE '' END AS reason,
		x.draft_page_id AS ex_page, x.with_children AS ex_children, x.reason AS ex_reason, x.created_at AS ex_at,
		tw.id IS NOT NULL AND EXISTS (SELECT 1 FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view'
			WHERE t.descendant_id = tw.id) AS twin_restricted
	FROM pages p
	LEFT JOIN page_pairs pp ON pp.draft_page_id = p.id AND pp.pair_id = $3
	LEFT JOIN pages tw ON tw.id = pp.twin_page_id
	LEFT JOIN LATERAL (SELECT e.draft_page_id, e.with_children, e.reason, e.created_at FROM page_tree t
		JOIN release_exclusions e ON e.draft_page_id = t.ancestor_id
		WHERE t.descendant_id = p.id AND (t.depth = 0 OR e.with_children) ORDER BY t.depth LIMIT 1) x ON true
	WHERE p.space_id = $1 AND p.kind = ANY($4)
	  AND (p.status = 'published' OR (p.status = 'trashed' AND pp.released_version > 0))
	  AND p.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($2) h)
	  AND p.id NOT IN (SELECT ch.page_id FROM perm_control_hidden_pages($2) ch)
	UNION ALL
	SELECT r.draft_page_id, r.twin_page_id, t.title, t.emoji, 0, 0, NULL, r.removed_at, '{}', 'removed', r.cause,
		NULL, NULL, NULL, NULL, false
	FROM release_removals r JOIN pages t ON t.id = r.twin_page_id AND t.status <> 'trashed'
	WHERE r.pair_id = $3 AND t.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($2) h)
)`

// Screen reads one tab of the release screen, for those with space.release in the draft space.
func (s *Service) Screen(ctx context.Context, c identity.Caller, spaceID uuid.UUID, tab string, offset int) (Screen, error) {
	out := Screen{Rows: []ScreenRow{}}
	statuses, ok := tabStatuses[tab]
	if !ok {
		return out, apperr.Invalid("That is not a tab of the release screen.")
	}
	p, err := s.draftPairToRead(ctx, c, spaceID)
	if err != nil {
		return out, err
	}
	args := []any{p.Draft, c.UserID, p.ID, scopeKinds}
	tiles, err := s.pool.Query(ctx, screenFacts+` SELECT status, count(*) FROM facts GROUP BY status`, args...)
	if err != nil {
		return out, err
	}
	for tiles.Next() {
		var status string
		var n int
		if err := tiles.Scan(&status, &n); err != nil {
			tiles.Close()
			return out, err
		}
		out.Tiles.add(status, n)
	}
	tiles.Close()
	if err := tiles.Err(); err != nil {
		return out, err
	}
	rows, err := s.pool.Query(ctx, screenFacts+` SELECT id, twin_page_id, title, emoji, status, changed, reason, released_version, current_version,
			released_at, since, ex_page, ex_children, ex_reason, ex_at, twin_restricted
		FROM facts WHERE status = ANY($5) ORDER BY since, id LIMIT $6 OFFSET $7`, append(args, statuses, screenPage+1, max(offset, 0))...)
	if err != nil {
		return out, err
	}
	out.Rows, err = pgx.CollectRows(rows, scanScreenRow)
	if err != nil {
		return out, err
	}
	if len(out.Rows) > screenPage {
		out.Rows, out.More = out.Rows[:screenPage], true
	}
	if err := s.addPaths(ctx, c, p, out.Rows); err != nil {
		return out, err
	}
	out.Going, err = s.goingRun(ctx, c, spaceID, p)
	return out, err
}

func (t *Tiles) add(status string, n int) {
	switch status {
	case StatusNotReleased:
		t.NotReleased += n
	case StatusChanged:
		t.Changed += n
	case StatusCannot:
		t.Cannot += n
	case StatusRemoved:
		t.Removed += n
	case StatusLeftOut, StatusLeftOutReleased:
		t.LeftOut += n
	}
}

func scanScreenRow(r pgx.CollectableRow) (ScreenRow, error) {
	var row ScreenRow
	var exPage *uuid.UUID
	var exChildren *bool
	var exReason *string
	var exAt *time.Time
	err := r.Scan(&row.PageID, &row.TwinPageID, &row.Title, &row.Emoji, &row.Status, &row.Aspects, &row.Reason, &row.ReleasedVersion,
		&row.CurrentVersion, &row.ReleasedAt, &row.Since, &exPage, &exChildren, &exReason, &exAt, &row.TwinRestricted)
	if exPage != nil && exChildren != nil && exReason != nil && exAt != nil {
		row.LeftOut = &Exclusion{PageID: *exPage, WithChildren: *exChildren, Reason: *exReason, At: *exAt}
	}
	if row.Status != StatusChanged {
		row.Aspects = []string{}
	}
	row.Path = []string{}
	return row, err
}

// addPaths gives each row the titles of the pages above it that the person may see, the home
// page left out.
func (s *Service) addPaths(ctx context.Context, c identity.Caller, p pair, rows []ScreenRow) error {
	if len(rows) == 0 {
		return nil
	}
	list := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		list[i] = r.PageID
	}
	paths, err := pages.VisiblePaths(ctx, s.pool, c.UserID, p.Draft, list)
	if err != nil {
		return err
	}
	for i := range rows {
		if path, ok := paths[rows[i].PageID]; ok {
			rows[i].Path = path
		}
	}
	return nil
}

// goingRun is the newest run of a pair that waits, goes or stopped, for the line of progress.
func (s *Service) goingRun(ctx context.Context, c identity.Caller, spaceID uuid.UUID, p pair) (*Run, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM release_runs WHERE pair_id = $1 AND scope <> 'page'
		AND status IN ('queued', 'running', 'stopped') ORDER BY previewed_at DESC LIMIT 1`, p.ID).Scan(&id)
	if noRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := s.RunView(ctx, c, spaceID, id)
	if err != nil {
		return nil, err
	}
	r.Pages = nil
	return &r, nil
}

// AsOfPage is a twin as readers had it at a moment, and the draft version it came from.
type AsOfPage struct {
	TwinPageID   uuid.UUID `json:"twin_page_id"`
	Title        string    `json:"title"`
	TwinVersion  int       `json:"twin_version"`
	DraftPageID  uuid.UUID `json:"draft_page_id"`
	DraftVersion int       `json:"draft_version"`
	ReleasedAt   time.Time `json:"released_at"`
}

// AsOf answers which twin version readers had at a moment, and which draft version it came
// from, for one page or the whole published space: the twin's own history read at that time,
// with the runs that wrote it (PUB-912). A twin taken off before the moment is left out.
func (s *Service) AsOf(ctx context.Context, c identity.Caller, spaceID uuid.UUID, at time.Time, page *uuid.UUID) ([]AsOfPage, error) {
	p, err := s.draftPairToRead(ctx, c, spaceID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `WITH twins AS (
			SELECT DISTINCT rp.twin_page_id AS id FROM release_run_pages rp JOIN release_runs r ON r.id = rp.run_id
			WHERE r.pair_id = $1 AND rp.outcome = 'released' AND rp.twin_page_id IS NOT NULL
			  AND ($3::uuid IS NULL OR rp.draft_page_id = $3 OR rp.twin_page_id = $3)
		)
		SELECT v.page_id, v.title, v.number, rp.draft_page_id, rp.draft_version, v.created_at
		FROM twins t
		JOIN LATERAL (SELECT pv.page_id, pv.title, pv.number, pv.created_at FROM page_versions pv
			WHERE pv.page_id = t.id AND pv.created_at <= $2 ORDER BY pv.number DESC LIMIT 1) v ON true
		JOIN LATERAL (SELECT x.draft_page_id, x.draft_version FROM release_run_pages x JOIN release_runs xr ON xr.id = x.run_id
			WHERE x.twin_page_id = t.id AND x.twin_version = v.number AND xr.pair_id = $1 LIMIT 1) rp ON true
		WHERE NOT EXISTS (SELECT 1 FROM release_run_pages o JOIN release_runs orun ON orun.id = o.run_id
			WHERE o.twin_page_id = t.id AND o.outcome = 'taken_off' AND orun.finished_at <= $2)
		  AND t.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($4) h)
		ORDER BY v.title, v.page_id`, p.ID, at, page, c.UserID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AsOfPage, error) {
		var a AsOfPage
		err := r.Scan(&a.TwinPageID, &a.Title, &a.TwinVersion, &a.DraftPageID, &a.DraftVersion, &a.ReleasedAt)
		return a, err
	})
	if out == nil {
		out = []AsOfPage{}
	}
	return out, err
}
