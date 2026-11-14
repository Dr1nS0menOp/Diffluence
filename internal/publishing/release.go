// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

// The release status of a draft page, in words (PUB-330). Left out, waiting for approval and
// removed in the draft come with PB2 and PB3.
const (
	StatusNotReleased = "not_released"
	StatusReleased    = "released"
	StatusChanged     = "changed"
	StatusCannot      = "cannot_release"
)

// What a release does to one page.
const (
	actionMake   = "make"   // the first release: a twin is made
	actionChange = "change" // a new version of the twin
	actionUpdate = "update" // only place, look or labels: no version (PUB-411)
	actionNone   = "none"   // in step: nothing (PUB-403)
)

// Reasons a page cannot be released (PUB-231, PUB-232, PUB-234).
const (
	reasonKind         = "kind"
	reasonNotPublished = "not_published"
	reasonArchived     = "archived"
	reasonNotApproved  = "not_approved"
)

// pagePair is the row of a released draft page in the active pair.
type pagePair struct {
	Twin            uuid.UUID
	ReleasedVersion int
	TwinVersion     int
	ReleasedTitle   string
	ReleasedHash    string
	ReleasedAt      *time.Time
	Changed         []string
}

func loadPagePair(ctx context.Context, q db.Querier, pairID, draftPage uuid.UUID) (*pagePair, error) {
	var pp pagePair
	err := q.QueryRow(ctx, `SELECT twin_page_id, released_version, twin_version, released_title, released_hash, released_at, changed
		FROM page_pairs WHERE draft_page_id = $1 AND pair_id = $2`, draftPage, pairID).
		Scan(&pp.Twin, &pp.ReleasedVersion, &pp.TwinVersion, &pp.ReleasedTitle, &pp.ReleasedHash, &pp.ReleasedAt, &pp.Changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &pp, err
}

// statusOf is the status of a draft page from its pair row (PUB-330, PUB-341): no row, or a
// home page paired but never released, is not released yet.
func statusOf(pp *pagePair) (string, []string) {
	switch {
	case pp == nil || pp.ReleasedVersion == 0:
		return StatusNotReleased, []string{}
	case len(pp.Changed) > 0:
		return StatusChanged, pp.Changed
	}
	return StatusReleased, []string{}
}

// cannotRelease says why a page is not released at all, or "" when it can be.
func cannotRelease(p pages.Page, controlHidden bool) string {
	switch {
	case p.Kind != pages.KindPage && p.Kind != pages.KindFolder && p.Kind != pages.KindLink:
		return reasonKind
	case p.Status != pages.StatusPublished || p.CurrentVersion == 0:
		return reasonNotPublished
	case p.ArchivedAt != nil:
		return reasonArchived
	case controlHidden:
		return reasonNotApproved
	}
	return ""
}

// PageStatus is what a page shows of its pair: on a draft page its release status, on a twin
// that it was released. Each other side is named only to those who may view it (PUB-603).
type PageStatus struct {
	Side            string     `json:"side"`
	Status          string     `json:"status,omitempty"`
	Aspects         []string   `json:"aspects"`
	Reason          string     `json:"reason,omitempty"`
	ReleasedVersion int        `json:"released_version,omitempty"`
	TwinVersion     int        `json:"twin_version,omitempty"`
	ReleasedAt      *time.Time `json:"released_at,omitempty"`
	TwinPageID      *uuid.UUID `json:"twin_page_id,omitempty"`
	DraftPageID     *uuid.UUID `json:"draft_page_id,omitempty"`
	DraftSpace      *SpaceRef  `json:"draft_space,omitempty"`
	CanRelease      bool       `json:"can_release"`
}

func (s *Service) mayPage(ctx context.Context, c identity.Caller, id uuid.UUID, action string) (bool, error) {
	d, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, action, uuid.Nil)
	if apperr.IsNotFound(err) {
		return false, nil
	}
	return d.Allowed, err
}

func (s *Service) maySpace(ctx context.Context, c identity.Caller, id uuid.UUID, action string) (bool, error) {
	d, err := s.perms.CheckSpace(ctx, s.pool, c.UserID, id, action)
	if apperr.IsNotFound(err) {
		return false, nil
	}
	return d.Allowed, err
}

// Status reads the pair of a page. On a draft page only writers and those who release see
// anything (PUB-1221); on a twin every reader sees that it was released (PUB-1230).
func (s *Service) Status(ctx context.Context, c identity.Caller, pageID uuid.UUID) (PageStatus, error) {
	out := PageStatus{Aspects: []string{}}
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return out, err
	}
	src, err := pages.SourceForRelease(ctx, s.pool, pageID)
	if err != nil {
		return out, err
	}
	space, err := spaces.LoadIn(ctx, s.pool, src.SpaceID)
	if err != nil {
		return out, err
	}
	switch space.PairRole {
	case permissions.PairDraft:
		return s.draftStatus(ctx, c, src)
	case permissions.PairPublished:
		return s.twinStatus(ctx, c, src)
	}
	return out, nil
}

func (s *Service) draftStatus(ctx context.Context, c identity.Caller, src pages.ReleaseSource) (PageStatus, error) {
	out := PageStatus{Aspects: []string{}}
	edits, err := s.mayPage(ctx, c, src.ID, permissions.PageEdit)
	if err != nil {
		return out, err
	}
	releases, err := s.maySpace(ctx, c, src.SpaceID, permissions.SpaceRelease)
	if err != nil || (!edits && !releases) {
		return out, err
	}
	p, err := activePairOf(ctx, s.pool, src.SpaceID, false)
	if err != nil {
		return out, err
	}
	out.Side = permissions.PairDraft
	_, facts, err := s.perms.LoadPage(ctx, s.pool, src.ID)
	if err != nil {
		return out, err
	}
	if out.Reason = cannotRelease(src.Page, facts.ControlHidden); out.Reason != "" {
		out.Status = StatusCannot
		return out, nil
	}
	pp, err := loadPagePair(ctx, s.pool, p.ID, src.ID)
	if err != nil {
		return out, err
	}
	out.Status, out.Aspects = statusOf(pp)
	if pp != nil {
		out.ReleasedVersion, out.TwinVersion, out.ReleasedAt = pp.ReleasedVersion, pp.TwinVersion, pp.ReleasedAt
		seen, err := s.mayPage(ctx, c, pp.Twin, permissions.ActionPageView)
		if err != nil {
			return out, err
		}
		if seen {
			twin := pp.Twin
			out.TwinPageID = &twin
		}
	}
	viewsPublished, err := s.maySpace(ctx, c, p.Published, permissions.SpaceView)
	if err != nil {
		return out, err
	}
	out.CanRelease = releases && viewsPublished
	return out, nil
}

func (s *Service) twinStatus(ctx context.Context, c identity.Caller, src pages.ReleaseSource) (PageStatus, error) {
	out := PageStatus{Aspects: []string{}, Side: permissions.PairPublished}
	var draft, draftSpace uuid.UUID
	var releasedAt *time.Time
	err := s.pool.QueryRow(ctx, `SELECT pp.draft_page_id, sp.draft_space_id, pp.released_at FROM page_pairs pp
		JOIN space_pairs sp ON sp.id = pp.pair_id AND sp.ended_at IS NULL
		WHERE pp.twin_page_id = $1 AND pp.released_version > 0`, src.ID).Scan(&draft, &draftSpace, &releasedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil // a home page that waits for its first release
	}
	if err != nil {
		return out, err
	}
	out.ReleasedAt, out.TwinVersion = releasedAt, src.CurrentVersion
	seen, err := s.mayPage(ctx, c, draft, permissions.ActionPageView)
	if err != nil {
		return out, err
	}
	if seen {
		out.DraftPageID = &draft
	}
	viewsDraftSpace, err := s.maySpace(ctx, c, draftSpace, permissions.SpaceView)
	if err != nil {
		return out, err
	}
	if viewsDraftSpace {
		sp, err := spaces.LoadIn(ctx, s.pool, draftSpace)
		if err != nil {
			return out, err
		}
		out.DraftSpace = &SpaceRef{ID: sp.ID, Key: sp.Key, Name: sp.Name}
	}
	return out, nil
}

// Preview is what releasing one page would do (PUB-430), held for an hour (PUB-433).
type Preview struct {
	ID           uuid.UUID `json:"id"`
	Version      int       `json:"version"`
	Action       string    `json:"action"`
	Aspects      []string  `json:"aspects"`
	PlacedHigher bool      `json:"placed_higher"`
	Files        int       `json:"files"`
	Bytes        int64     `json:"bytes"`
	LinksAsText  int       `json:"links_as_text"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// plan is one page of a release, worked out from the database.
type plan struct {
	pair      pair
	published spaces.Space
	draft     spaces.Space
	src       pages.ReleaseSource
	existing  *pagePair
	version   pages.ReleasedVersion
	number    int
	action    string
	aspects   []string
	place     placement
	files     []fileStep
	targets   map[uuid.UUID]pages.LinkTarget
	twins     map[uuid.UUID]uuid.UUID
	foreign   map[uuid.UUID]bool
}

// fileStep is one file the released document shows.
type fileStep struct {
	from    uuid.UUID
	copy    *uuid.UUID
	version int
	size    int64
	copied  int // the version of the draft file the copy holds, 0 for none
}

func (f fileStep) needed() bool { return f.copy == nil || f.copied != f.version }

// requireRelease checks the rights of a release at the moment of acting: view of the page,
// space.release in the draft space and view of the published space (PUB-413, PUB-1420). The
// page is looked at first, so a page the person may not see stays "not found".
func (s *Service) requireRelease(ctx context.Context, q db.Querier, c identity.Caller, pageID uuid.UUID, p pair) error {
	if err := s.perms.RequirePage(ctx, q, c.UserID, pageID, permissions.ActionPageView, uuid.Nil); err != nil {
		return err
	}
	if err := s.perms.RequireSpace(ctx, q, c.UserID, p.Draft, permissions.SpaceRelease); err != nil {
		return err
	}
	if d, err := s.perms.CheckSpace(ctx, q, c.UserID, p.Published, permissions.SpaceView); err != nil || !d.Allowed {
		if err != nil && !apperr.IsNotFound(err) {
			return err
		}
		return apperr.Conflict("published_space_hidden", "You cannot see the published space, so you cannot release into it.")
	}
	return nil
}

// planPage works out the release of one page at a version, through q.
func (s *Service) planPage(ctx context.Context, q db.Querier, p pair, src pages.ReleaseSource, number int) (plan, error) {
	pl := plan{pair: p, src: src, number: number}
	var err error
	if pl.draft, err = spaces.LoadIn(ctx, q, p.Draft); err != nil {
		return pl, err
	}
	if pl.published, err = spaces.LoadIn(ctx, q, p.Published); err != nil {
		return pl, err
	}
	if pl.draft.Status != spaces.StatusActive || pl.published.Status != spaces.StatusActive {
		return pl, apperr.Conflict("space_not_active", "Both spaces of the pair must be in use to release.")
	}
	if pl.existing, err = loadPagePair(ctx, q, p.ID, src.ID); err != nil {
		return pl, err
	}
	if pl.version, err = pages.VersionForRelease(ctx, q, src.ID, number); err != nil {
		return pl, err
	}
	if pl.twins, err = twinsOf(ctx, q, p.ID, append(slices.Clone(src.Ancestors), referencedPages(pl.version.Doc)...)); err != nil {
		return pl, err
	}
	pl.place = placeTwin(src.Ancestors, pl.twins)
	pl.aspects, pl.action = pl.diff()
	if pl.targets, err = pages.ReleaseTargets(ctx, q, append(referencedPages(pl.version.Doc), src.ID)); err != nil {
		return pl, err
	}
	shown := document.FilesShown(pl.version.Doc)
	if src.HeaderAttachmentID != nil && !slices.Contains(shown, *src.HeaderAttachmentID) {
		shown = append(shown, *src.HeaderAttachmentID)
	}
	pl.files, pl.foreign, err = filesOf(ctx, q, p.Draft, pl.existing, shown)
	if err != nil {
		return pl, err
	}
	if pl.action == actionNone && slices.ContainsFunc(pl.files, fileStep.needed) {
		pl.action = actionUpdate // a new version of a file shown, and nothing else (PUB-320)
	}
	return pl, nil
}

// diff compares the version to release and the draft page now with the last release.
func (pl plan) diff() ([]string, string) {
	if pl.existing == nil || pl.existing.TwinVersion == 0 {
		return []string{}, actionMake
	}
	aspects := []string{}
	if pl.version.Hash != pl.existing.ReleasedHash {
		aspects = append(aspects, "text")
	}
	if pl.version.Title != pl.existing.ReleasedTitle {
		aspects = append(aspects, "title")
	}
	for _, a := range pl.existing.Changed {
		if a != "text" && a != "title" && !slices.Contains(aspects, a) {
			aspects = append(aspects, a)
		}
	}
	switch {
	case slices.Contains(aspects, "text") || slices.Contains(aspects, "title"):
		return aspects, actionChange
	case len(aspects) > 0:
		return aspects, actionUpdate
	}
	return aspects, actionNone
}

// twinsOf maps the draft pages among list to their twins in a pair.
func twinsOf(ctx context.Context, q db.Querier, pairID uuid.UUID, list []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := q.Query(ctx, `SELECT draft_page_id, twin_page_id FROM page_pairs WHERE pair_id = $1 AND draft_page_id = ANY($2)`, pairID, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]uuid.UUID{}
	for rows.Next() {
		var draft, twin uuid.UUID
		if err := rows.Scan(&draft, &twin); err != nil {
			return nil, err
		}
		out[draft] = twin
	}
	return out, rows.Err()
}

// referencedPages lists the pages a document names: links, cards and blocks.
func referencedPages(doc document.Node) []uuid.UUID {
	out := document.LinkedPages(doc)
	document.Walk(doc, func(n document.Node) {
		if n.Type != document.TypeDynamic {
			return
		}
		kind, _ := n.Attrs["kind"].(string)
		settings, _ := n.Attrs["settings"].(map[string]any)
		single, lists := document.DynamicPageSettings(kind)
		for _, key := range single {
			if id, err := uuid.Parse(fmt.Sprint(settings[key])); err == nil {
				out = append(out, id)
			}
		}
		for _, key := range lists {
			items, _ := settings[key].([]any)
			for _, item := range items {
				if id, err := uuid.Parse(fmt.Sprint(item)); err == nil {
					out = append(out, id)
				}
			}
		}
	})
	return out
}

// filesOf works out the files a release copies: those of the draft space that the document
// shows, with what their twin holds already. Files of other spaces stay as they are.
func filesOf(ctx context.Context, q db.Querier, draftSpace uuid.UUID, existing *pagePair, shown []uuid.UUID) ([]fileStep, map[uuid.UUID]bool, error) {
	sources, err := attachments.SourcesForRelease(ctx, q, shown)
	if err != nil {
		return nil, nil, err
	}
	copies := map[uuid.UUID]fileStep{}
	if existing != nil {
		rows, err := q.Query(ctx, `SELECT draft_attachment_id, copy_attachment_id, copied_version FROM file_pairs WHERE twin_page_id = $1`, existing.Twin)
		if err != nil {
			return nil, nil, err
		}
		for rows.Next() {
			var f fileStep
			var to uuid.UUID
			if err := rows.Scan(&f.from, &to, &f.copied); err != nil {
				rows.Close()
				return nil, nil, err
			}
			f.copy = &to
			copies[f.from] = f
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, nil, err
		}
	}
	var steps []fileStep
	foreign := map[uuid.UUID]bool{}
	for _, id := range shown {
		src, ok := sources[id]
		switch {
		case !ok:
			continue // gone or in the trash: the document shows it as not available
		case src.SpaceID != draftSpace:
			foreign[id] = true
			continue
		}
		step := copies[id]
		step.from, step.version, step.size = id, src.Version, src.Size
		steps = append(steps, step)
	}
	return steps, foreign, nil
}

// Preview works out the release of one page and keeps it for an hour, with the version it
// shows (PUB-430, PUB-431, PUB-433).
func (s *Service) Preview(ctx context.Context, c identity.Caller, pageID uuid.UUID) (Preview, error) {
	var out Preview
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		src, p, err := s.draftPage(ctx, tx, c, pageID)
		if err != nil {
			return err
		}
		pl, err := s.planPage(ctx, tx, p, src, src.CurrentVersion)
		if err != nil {
			return err
		}
		out = Preview{ID: ids.New(), Version: pl.number, Action: pl.action, Aspects: pl.aspects, PlacedHigher: pl.place.Higher}
		for _, f := range pl.files {
			if f.needed() {
				out.Files++
				out.Bytes += f.size
			}
		}
		in := pl.rewriteInput(nil)
		if _, ok := in.twins[src.ID]; !ok {
			in.twins[src.ID] = uuid.New() // the twin this release will make: links to the page itself work
		}
		out.LinksAsText = len(rewrite(pl.version.Doc, in).textLinks)
		counts := map[string]any{"pages": 1, "action": pl.action, "files": out.Files, "bytes": out.Bytes, "links_as_text": out.LinksAsText}
		if err := tx.QueryRow(ctx, `INSERT INTO release_runs (id, pair_id, scope, root_page_id, status, actor_id, counts)
			VALUES ($1, $2, 'page', $3, 'previewed', $4, $5) RETURNING previewed_at + $6::interval`,
			out.ID, p.ID, pageID, c.UserID, counts, previewLife.String()).Scan(&out.ExpiresAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO release_run_pages (run_id, draft_page_id, draft_version, twin_page_id, action, content_hash)
			VALUES ($1, $2, $3, $4, $5, $6)`, out.ID, pageID, pl.number, twinOf(pl.existing), pl.action, pl.version.Hash); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("release.previewed", "release_run", out.ID.String(), p.Draft.String(),
			map[string]any{"pair_id": p.ID, "page_id": pageID, "version": pl.number, "counts": counts}))
		return err
	})
	return out, err
}

func twinOf(pp *pagePair) *uuid.UUID {
	if pp == nil {
		return nil
	}
	return &pp.Twin
}

// draftPage loads a page that a release may take, with its active pair, after the rights.
func (s *Service) draftPage(ctx context.Context, tx pgx.Tx, c identity.Caller, pageID uuid.UUID) (pages.ReleaseSource, pair, error) {
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
	p, err := activePairOf(ctx, tx, src.SpaceID, true)
	if err != nil {
		return src, p, err
	}
	if p.Draft != src.SpaceID {
		return src, p, apperr.Conflict("not_a_draft_page", "Only pages of the draft space are released.")
	}
	if err := s.requireRelease(ctx, tx, c, pageID, p); err != nil {
		return src, p, err
	}
	_, facts, err := s.perms.LoadPage(ctx, tx, pageID)
	if err != nil {
		return src, p, err
	}
	if reason := cannotRelease(src.Page, facts.ControlHidden); reason != "" {
		return src, p, apperr.Conflict("cannot_release", cannotReleaseWords[reason])
	}
	return src, p, nil
}

var cannotReleaseWords = map[string]string{
	reasonKind:         "Blog posts, databases and templates are not released; they stay in the draft space.",
	reasonNotPublished: "Publish the page in the draft space first. A release takes a published version, never a draft.",
	reasonArchived:     "This page is archived. Unarchive it to release it.",
	reasonNotApproved:  "This page has no approved version yet. Only an approved version can be released.",
}

func (pl plan) rewriteInput(files map[uuid.UUID]uuid.UUID) rewriteInput {
	twins := map[uuid.UUID]uuid.UUID{}
	for draft, twin := range pl.twins {
		twins[draft] = twin
	}
	if pl.existing != nil {
		twins[pl.src.ID] = pl.existing.Twin
	}
	return rewriteInput{draft: spaceRef{ID: pl.draft.ID, Key: pl.draft.Key}, published: spaceRef{ID: pl.published.ID, Key: pl.published.Key},
		twins: twins, targets: pl.targets, files: files, foreign: pl.foreign}
}

// RunInput confirms a preview.
type RunInput struct {
	PreviewID uuid.UUID `json:"preview_id"`
	Note      string    `json:"note"`
	Silent    bool      `json:"silent"`
}

// Report is what a run of one page did (PUB-453).
type Report struct {
	RunID        uuid.UUID `json:"run_id"`
	Action       string    `json:"action"`
	Version      int       `json:"version"`
	TwinVersion  int       `json:"twin_version"`
	PlacedHigher bool      `json:"placed_higher"`
	FilesCopied  int       `json:"files_copied"`
	LinksAsText  int       `json:"links_as_text"`
}

// Release releases one page at the version its preview showed, in one transaction (PUB-400,
// PUB-410, PUB-431, PUB-450). The pair row is locked, so two runs of one pair take turns
// (PUB-1060), and a publish in the draft meanwhile makes a newer version that waits for the
// next release (PUB-1062).
func (s *Service) Release(ctx context.Context, c identity.Caller, pageID uuid.UUID, in RunInput) (Report, error) {
	if err := checkNote(in.Note); err != nil {
		return Report{}, err
	}
	var out Report
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// The mark lets the trigger of a published space through, for this transaction only.
		if _, err := tx.Exec(ctx, "SELECT set_config('diffluence.release', 'on', true)"); err != nil {
			return err
		}
		src, p, err := s.draftPage(ctx, tx, c, pageID)
		if err != nil {
			return err
		}
		number, err := s.claimPreview(ctx, tx, c, p, pageID, in.PreviewID)
		if err != nil {
			return err
		}
		pl, err := s.planPage(ctx, tx, p, src, number)
		if err != nil {
			return err
		}
		out, err = s.releaseOne(ctx, tx, c, pl, in)
		return err
	})
	return out, err
}

// claimPreview takes the preview of a run, and the version it showed. An older preview, or one
// of someone else, is refused (PUB-431, PUB-433).
func (s *Service) claimPreview(ctx context.Context, tx pgx.Tx, c identity.Caller, p pair, pageID, previewID uuid.UUID) (int, error) {
	var number int
	var fresh bool
	err := tx.QueryRow(ctx, `SELECT rp.draft_version, r.previewed_at > now() - $5::interval FROM release_runs r
		JOIN release_run_pages rp ON rp.run_id = r.id AND rp.draft_page_id = $3
		WHERE r.id = $1 AND r.pair_id = $2 AND r.actor_id = $4 AND r.status = 'previewed' FOR UPDATE OF r`,
		previewID, p.ID, pageID, c.UserID, previewLife.String()).Scan(&number, &fresh)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, apperr.Conflict("no_preview", "Make a preview of this release first. A preview is confirmed once.")
	}
	if err != nil {
		return 0, err
	}
	if !fresh {
		return 0, apperr.Conflict("preview_expired", "This preview is more than an hour old. Make a new one to see what the release will do now.")
	}
	return number, nil
}

func (s *Service) releaseOne(ctx context.Context, tx pgx.Tx, c identity.Caller, pl plan, in RunInput) (Report, error) {
	out := Report{RunID: in.PreviewID, Action: pl.action, Version: pl.number, PlacedHigher: pl.place.Higher}
	silent := in.Silent || !pl.pair.Settings.NotifyWatchers
	if pl.action == actionNone {
		return out, s.finishRun(ctx, tx, c, pl, out, in, nil)
	}
	twin := twinOf(pl.existing)
	if twin == nil {
		made, err := s.pages.MakeTwin(ctx, tx, c, pl.published.ID, pl.place.Parent, pl.src.Page)
		if err != nil {
			return out, err
		}
		twin = &made.ID
		pl.existing = &pagePair{Twin: made.ID}
	}
	files := map[uuid.UUID]uuid.UUID{}
	for i, f := range pl.files {
		if !f.needed() {
			files[f.from] = *f.copy
			continue
		}
		copied, version, err := s.attachments.CopyForRelease(ctx, tx, c, attachments.ReleaseCopy{From: f.from, Copy: f.copy, TwinPage: *twin, SpaceID: pl.published.ID})
		if err != nil {
			return out, err
		}
		files[f.from], pl.files[i].copy, pl.files[i].copied = copied, &copied, version
		out.FilesCopied++
	}
	res := rewrite(pl.version.Doc, pl.rewriteInput(files))
	raw, err := document.Marshal(res.doc)
	if err != nil {
		return out, err
	}
	// A released document passes the same gate as any version (PUB-534).
	doc, err := document.Normalize(raw)
	if err != nil {
		return out, fmt.Errorf("publishing: a released document did not pass the gate: %w", err)
	}
	write := pages.TwinWrite{ID: *twin, Parent: pl.place.Parent, Look: pl.src.Look, Labels: pl.src.Labels}
	write.Look.HeaderAttachmentID = nil
	if h := pl.src.HeaderAttachmentID; h != nil {
		if copied, ok := files[*h]; ok {
			write.Look.HeaderAttachmentID = &copied
		}
	}
	if pl.action == actionMake || pl.action == actionChange {
		// The releaser is the author and the note the summary, never the draft's (PUB-1408).
		write.Version = &pages.TwinVersion{Title: pl.version.Title, Doc: doc, Summary: in.Note, Silent: silent}
	}
	written, err := s.pages.WriteTwin(ctx, tx, c, write)
	if err != nil {
		return out, err
	}
	out.TwinVersion, out.LinksAsText = written.CurrentVersion, len(res.textLinks)
	if err := s.keepFacts(ctx, tx, c, pl, written, res, in.PreviewID); err != nil {
		return out, err
	}
	if err := s.keepFiles(ctx, tx, c, pl); err != nil {
		return out, err
	}
	return out, s.finishRun(ctx, tx, c, pl, out, in, &res)
}

// keepFacts records the release in the page pair: the version, and the facts of each aspect
// that out of step compares with from now on (PUB-340).
func (s *Service) keepFacts(ctx context.Context, tx pgx.Tx, c identity.Caller, pl plan, twin pages.Page, res rewritten, runID uuid.UUID) error {
	if _, err := tx.Exec(ctx, `INSERT INTO page_pairs (draft_page_id, pair_id, twin_page_id, released_version, twin_version, released_at,
			released_by, run_id, released_title, released_parent_id, released_look, released_labels, released_hash, text_links)
		VALUES ($1, $2, $3, $4, $5, now(), $6, $7, $8, $9,
			page_pair_look($10, $11, $12, $13, $14), $15, $16, $17)
		ON CONFLICT (draft_page_id) DO UPDATE SET pair_id = EXCLUDED.pair_id, twin_page_id = EXCLUDED.twin_page_id,
			released_version = EXCLUDED.released_version, twin_version = EXCLUDED.twin_version, released_at = EXCLUDED.released_at,
			released_by = EXCLUDED.released_by, run_id = EXCLUDED.run_id, released_title = EXCLUDED.released_title,
			released_parent_id = EXCLUDED.released_parent_id, released_look = EXCLUDED.released_look,
			released_labels = EXCLUDED.released_labels, released_hash = EXCLUDED.released_hash, text_links = EXCLUDED.text_links,
			changed = '{}', changed_since = NULL`,
		pl.src.ID, pl.pair.ID, twin.ID, pl.number, twin.CurrentVersion, c.UserID, runID, pl.version.Title, pl.src.ParentID,
		pl.src.Emoji, pl.src.FullWidth, pl.src.HeaderAttachmentID, pl.src.HeaderPosition, pl.src.Language,
		pl.src.Labels, pl.version.Hash, nonNil(res.textLinks)); err != nil {
		return err
	}
	// The draft may have changed since the preview: compare again, so a newer version stays
	// Changed since release (PUB-431).
	_, err := tx.Exec(ctx, "SELECT page_pair_refresh($1)", pl.src.ID)
	return err
}

func nonNil(list []uuid.UUID) []uuid.UUID {
	if list == nil {
		return []uuid.UUID{}
	}
	return list
}

// keepFiles records the copies, and takes off the twin what its draft no longer shows.
func (s *Service) keepFiles(ctx context.Context, tx pgx.Tx, c identity.Caller, pl plan) error {
	kept := []uuid.UUID{}
	for _, f := range pl.files {
		kept = append(kept, f.from)
		if _, err := tx.Exec(ctx, `INSERT INTO file_pairs (draft_attachment_id, twin_page_id, copy_attachment_id, copied_version)
			VALUES ($1, $2, $3, $4) ON CONFLICT (draft_attachment_id, twin_page_id)
			DO UPDATE SET copy_attachment_id = EXCLUDED.copy_attachment_id, copied_version = EXCLUDED.copied_version`,
			f.from, pl.existing.Twin, *f.copy, f.copied); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `DELETE FROM file_pairs WHERE twin_page_id = $1 AND NOT (draft_attachment_id = ANY($2))
		RETURNING copy_attachment_id`, pl.existing.Twin, kept)
	if err != nil {
		return err
	}
	gone, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, copied := range gone {
		if err := s.attachments.TrashForRelease(ctx, tx, c, copied); err != nil {
			return err
		}
	}
	return nil
}

// finishRun closes the run and writes its audit: in the draft space with both sides, in the
// published space with the twin only (PUB-900 to PUB-903, PUB-922, PUB-923).
func (s *Service) finishRun(ctx context.Context, tx pgx.Tx, c identity.Caller, pl plan, out Report, in RunInput, res *rewritten) error {
	outcome := "unchanged"
	var twin *uuid.UUID
	twinHash, links := "", map[string]string{}
	if res != nil {
		outcome, twin, links = "released", &pl.existing.Twin, res.linkMap
		v, err := pages.VersionForRelease(ctx, tx, pl.existing.Twin, out.TwinVersion)
		if err != nil {
			return err
		}
		twinHash = v.Hash
	}
	counts := map[string]any{"pages": 1, "released": boolCount(res != nil), "files_copied": out.FilesCopied, "links_as_text": out.LinksAsText}
	if _, err := tx.Exec(ctx, `UPDATE release_runs SET status = 'done', note = $2, silent = $3, started_at = now(), finished_at = now(),
		counts = $4, report = $5 WHERE id = $1`, in.PreviewID, in.Note, in.Silent, counts, out); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE release_run_pages SET twin_page_id = $3, twin_version = $4, outcome = $5, link_map = $6
		WHERE run_id = $1 AND draft_page_id = $2`, in.PreviewID, pl.src.ID, twin, out.TwinVersion, outcome, links); err != nil {
		return err
	}
	details := map[string]any{"pair_id": pl.pair.ID, "page_id": pl.src.ID, "version": pl.number, "content_hash": pl.version.Hash,
		"action": pl.action, "aspects": pl.aspects, "placed_higher": pl.place.Higher, "note": in.Note, "silent": in.Silent, "outcome": outcome}
	if twin != nil {
		details["twin_page_id"], details["twin_version"], details["twin_hash"] = *twin, out.TwinVersion, twinHash
	}
	if _, err := audit.Record(ctx, tx, c.Event("release.page", "page", pl.src.ID.String(), pl.pair.Draft.String(), details)); err != nil {
		return err
	}
	if twin == nil {
		return nil
	}
	if _, err := audit.Record(ctx, tx, c.Event("page.released", "page", twin.String(), pl.published.ID.String(),
		map[string]any{"run_id": in.PreviewID, "version": out.TwinVersion, "content_hash": twinHash, "note": in.Note})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, "page.released", map[string]any{"page_id": *twin, "space_id": pl.published.ID, "version": out.TwinVersion,
		"run_id": in.PreviewID, "actor_id": c.UserID})
	return err
}

func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}
