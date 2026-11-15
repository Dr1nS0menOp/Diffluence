// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

// The release status of a draft page, in words (PUB-330, PUB-331, PUB-421, PUB-702).
const (
	StatusNotReleased = "not_released"
	StatusReleased    = "released"
	// StatusWaiting is a released controlled page with a newer version under review: releasing
	// it now would change nothing (PUB-702).
	StatusWaiting         = "waiting_approval"
	StatusChanged         = "changed"
	StatusCannot          = "cannot_release"
	StatusLeftOut         = "left_out"
	StatusLeftOutReleased = "left_out_released"
	StatusRemoved         = "removed"
)

// Aspects of out of step that PB2 adds to those of PB1 (PUB-305, PUB-306, PUB-1013); the
// others are text, title, place, look and labels.
const (
	aspectLinks   = "links"
	aspectArchive = "archive"
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
	// LeftOut is the mark that leaves a draft page out of releases (PUB-420).
	LeftOut *Exclusion `json:"left_out,omitempty"`
	// ReaderComments counts the comments readers left on the twin, for those who may view it
	// (PUB-223, PUB-1232). No comment text crosses.
	ReaderComments int `json:"reader_comments"`
	// ApprovalRelease says why the last approval release of the approved version did not go,
	// on a draft page; nil when it went or there was none (PUB-711).
	ApprovalRelease *ApprovalRelease `json:"approval_release,omitempty"`
	// Approval is what readers of a twin see of the approval of what they read (PUB-730).
	Approval *TwinApproval `json:"approval,omitempty"`
	// CanVerify offers verifying the twin, to those who see the draft space's audit (PUB-732).
	CanVerify bool `json:"can_verify"`
}

// ApprovalRelease is an approval release that did not go: stopped, cancelled or skipped, with
// its reason as a code.
type ApprovalRelease struct {
	Outcome string    `json:"outcome"`
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// TwinApproval is when the version on a twin was approved, its number on the twin so a reader
// can cite it, and the approver when the pair shows approvers (PUB-730).
type TwinApproval struct {
	ApprovedAt time.Time `json:"approved_at"`
	Version    int       `json:"version"`
	ApprovedBy string    `json:"approved_by,omitempty"`
	// Signatures are those of the released draft version: the printed name, date and time, and
	// meaning of each, shown whatever the setting of approvers, since a signature means nothing
	// without its name (PUB-731, Part 11 §11.50(b)). The records stay in the draft space.
	Signatures []pages.SignatureView `json:"signatures,omitempty"`
}

// Exclusion is the mark that leaves a page out: on the page itself, or on a page above it
// with everything below.
type Exclusion struct {
	PageID       uuid.UUID `json:"page_id"`
	WithChildren bool      `json:"with_children"`
	Reason       string    `json:"reason"`
	At           time.Time `json:"at"`
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
	if out.LeftOut, err = exclusionOf(ctx, s.pool, src); err != nil {
		return out, err
	}
	if out.LeftOut != nil {
		out.Status = StatusLeftOut
		if pp != nil && pp.ReleasedVersion > 0 {
			out.Status = StatusLeftOutReleased
		}
	}
	if pp != nil {
		out.ReleasedVersion, out.TwinVersion, out.ReleasedAt = pp.ReleasedVersion, pp.TwinVersion, pp.ReleasedAt
		seen, err := s.mayPage(ctx, c, pp.Twin, permissions.ActionPageView)
		if err != nil {
			return out, err
		}
		if seen {
			twin := pp.Twin
			out.TwinPageID = &twin
			if out.ReaderComments, err = comments.CountForReaders(ctx, s.pool, twin); err != nil {
				return out, err
			}
		}
	}
	viewsPublished, err := s.maySpace(ctx, c, p.Published, permissions.SpaceView)
	if err != nil {
		return out, err
	}
	out.CanRelease = releases && viewsPublished
	if err := s.addApprovalStatus(ctx, p, src, &out); err != nil {
		return out, err
	}
	if out.TwinPageID != nil {
		if out.CanVerify, err = s.maySpace(ctx, c, src.SpaceID, permissions.SpaceAudit); err != nil {
			return out, err
		}
	}
	return out, nil
}

// addApprovalStatus adds what document control changes in the status of a controlled draft
// page: Waiting for approval while a newer version is under review and nothing else differs
// (PUB-702), and why the approval release of the approved version did not go (PUB-711).
func (s *Service) addApprovalStatus(ctx context.Context, p pair, src pages.ReleaseSource, out *PageStatus) error {
	ctl, err := controlOf(ctx, s.pool, src.ID)
	if err != nil || ctl == nil {
		return err
	}
	if out.Status == StatusReleased && ctl.head > src.CurrentVersion {
		out.Status = StatusWaiting
	}
	if out.Status != StatusChanged && out.Status != StatusNotReleased {
		return nil
	}
	var a ApprovalRelease
	var status string
	err = s.pool.QueryRow(ctx, `SELECT r.status, rp.outcome, COALESCE(NULLIF(r.stop_reason, ''), rp.reason), COALESCE(r.finished_at, r.previewed_at)
		FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id
		WHERE r.pair_id = $1 AND r.actor_type = $2 AND r.root_page_id = $3 AND rp.draft_version = $4
		ORDER BY r.previewed_at DESC LIMIT 1`, p.ID, actorApproval, src.ID, releaseVersion(src.CurrentVersion, ctl)).
		Scan(&status, &a.Outcome, &a.Reason, &a.At)
	if noRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	switch {
	case status == "stopped" || status == "cancelled":
		a.Outcome = status
	case a.Outcome != "skipped":
		return nil // it went, or it waits for its turn
	}
	out.ApprovalRelease = &a
	return nil
}

func (s *Service) twinStatus(ctx context.Context, c identity.Caller, src pages.ReleaseSource) (PageStatus, error) {
	out := PageStatus{Aspects: []string{}, Side: permissions.PairPublished}
	var draft, draftSpace uuid.UUID
	var releasedAt *time.Time
	var releasedVersion int
	var rawSettings []byte
	err := s.pool.QueryRow(ctx, `SELECT pp.draft_page_id, sp.draft_space_id, pp.released_at, pp.released_version, sp.settings
		FROM page_pairs pp JOIN space_pairs sp ON sp.id = pp.pair_id AND sp.ended_at IS NULL
		WHERE pp.twin_page_id = $1 AND pp.released_version > 0`, src.ID).Scan(&draft, &draftSpace, &releasedAt, &releasedVersion, &rawSettings)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil // a home page that waits for its first release
	}
	if err != nil {
		return out, err
	}
	out.ReleasedAt, out.TwinVersion = releasedAt, src.CurrentVersion
	if out.Approval, err = twinApproval(ctx, s.pool, draft, releasedVersion, src.CurrentVersion, rawSettings); err != nil {
		return out, err
	}
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

// twinApproval is the approved-on line of a twin of a controlled page: when the released
// version was approved, the twin's version number, and the approver only when the pair shows
// approvers, since who approved is often confidential to the team (PUB-730).
func twinApproval(ctx context.Context, q db.Querier, draft uuid.UUID, released, twinVersion int, rawSettings []byte) (*TwinApproval, error) {
	a, err := pages.ApprovalOfVersion(ctx, q, draft, released)
	if err != nil || a == nil {
		return nil, err
	}
	settings := defaultSettings()
	if err := json.Unmarshal(rawSettings, &settings); err != nil {
		return nil, fmt.Errorf("publishing: read the settings of a pair: %v", err)
	}
	out := &TwinApproval{ApprovedAt: a.At, Version: twinVersion}
	if settings.ShowApprovers {
		out.ApprovedBy = a.ByName
	}
	if out.Signatures, err = pages.SignaturesOfVersion(ctx, q, draft, released); err != nil {
		return nil, err
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
	// made is the twin a run made for this page before writing it, so links from pages
	// written earlier could lead to it (PUB-513, PUB-514).
	made *uuid.UUID
	// unarchive brings the twin back from the archive, as the draft page came back (PUB-1013).
	unarchive bool
}

// fileStep is one file the released document shows.
type fileStep struct {
	from    uuid.UUID
	copy    *uuid.UUID
	version int
	size    int64
	copied  int // the version of the draft file the copy holds, 0 for none
	// pin is the version a controlled page's released version pinned the file at (DC3).
	pin *attachments.ReleasePin
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
	draft, published, err := bothActive(ctx, q, p)
	if err != nil {
		return plan{}, err
	}
	return s.planPageWith(ctx, q, planInput{pair: p, src: src, number: number, draft: draft, published: published})
}

// planInput is what working out the release of one page starts from: in a run, the spaces it
// read once and the twins it made already, so the run's links resolve to each other (PUB-513).
type planInput struct {
	pair             pair
	src              pages.ReleaseSource
	number           int
	draft, published spaces.Space
	runTwins         map[uuid.UUID]uuid.UUID
}

// planPageWith works out the release of one page from what the caller read already.
func (s *Service) planPageWith(ctx context.Context, q db.Querier, in planInput) (plan, error) {
	p, src, runTwins := in.pair, in.src, in.runTwins
	pl := plan{pair: p, src: src, number: in.number, draft: in.draft, published: in.published}
	var err error
	if pl.existing, err = loadPagePair(ctx, q, p.ID, src.ID); err != nil {
		return pl, err
	}
	if pl.version, err = pages.VersionForRelease(ctx, q, src.ID, in.number); err != nil {
		return pl, err
	}
	if pl.twins, err = twinsOf(ctx, q, p.ID, append(slices.Clone(src.Ancestors), referencedPages(pl.version.Doc)...)); err != nil {
		return pl, err
	}
	for draft, twin := range runTwins {
		if _, ok := pl.twins[draft]; !ok {
			pl.twins[draft] = twin
		}
	}
	if made, ok := runTwins[src.ID]; ok && pl.existing == nil {
		pl.made = &made
	}
	pl.place = placeTwin(src.Ancestors, pl.twins)
	pl.aspects, pl.action = pl.diff()
	pl.unarchive = slices.Contains(pl.aspects, aspectArchive)
	if pl.targets, err = pages.ReleaseTargets(ctx, q, append(referencedPages(pl.version.Doc), src.ID)); err != nil {
		return pl, err
	}
	shown := document.FilesShown(pl.version.Doc)
	if src.HeaderAttachmentID != nil && !slices.Contains(shown, *src.HeaderAttachmentID) {
		shown = append(shown, *src.HeaderAttachmentID)
	}
	pins, err := attachments.ReleasePinsOf(ctx, q, src.ID, in.number)
	if err != nil {
		return pl, err
	}
	pl.files, pl.foreign, err = filesOf(ctx, q, p.Draft, pl.existing, shown, pins)
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
	// A link waiting turns back into a link, which is a new version (PUB-512).
	case slices.Contains(aspects, "text") || slices.Contains(aspects, "title") || slices.Contains(aspects, aspectLinks):
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
// shows, with what their twin holds already. Files of other spaces stay as they are. A file
// the released version pinned goes at its pinned version, never a newer one under review.
func filesOf(ctx context.Context, q db.Querier, draftSpace uuid.UUID, existing *pagePair, shown []uuid.UUID,
	pins map[uuid.UUID]attachments.ReleasePin) ([]fileStep, map[uuid.UUID]bool, error) {
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
		if pin, ok := pins[id]; ok {
			step.version, step.pin = pin.Version, &pin
		}
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
		ctl, err := controlOf(ctx, tx, pageID)
		if err != nil {
			return err
		}
		pl, err := s.planPage(ctx, tx, p, src, releaseVersion(src.CurrentVersion, ctl))
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
	out, err := leftOut(ctx, tx, src)
	if err != nil {
		return src, p, err
	}
	if out {
		return src, p, apperr.Conflict(codeLeftOut, "This page is left out of releases. Include it again to release it.")
	}
	return src, p, s.requireManualAllowed(ctx, tx, p, src)
}

// requireManualAllowed refuses releasing a controlled page by hand when the pair leaves those
// to approval (PUB-714). The one exception is an approved version whose approval release did
// not go: releasing it by hand is still that approval's release.
func (s *Service) requireManualAllowed(ctx context.Context, tx pgx.Tx, p pair, src pages.ReleaseSource) error {
	if !p.Settings.OnlyApprovalReleases {
		return nil
	}
	ctl, err := controlOf(ctx, tx, src.ID)
	if err != nil || ctl == nil {
		return err
	}
	var missed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM release_runs r JOIN release_run_pages rp ON rp.run_id = r.id
		WHERE r.pair_id = $1 AND r.actor_type = $2 AND r.root_page_id = $3 AND rp.draft_version = $4
		  AND (r.status IN ('stopped', 'cancelled') OR rp.outcome = 'skipped'))`,
		p.ID, actorApproval, src.ID, releaseVersion(src.CurrentVersion, ctl)).Scan(&missed); err != nil || missed {
		return err
	}
	return apperr.Conflict(reasonApprovalOnly, "This space releases controlled pages only by their approval.")
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
		// One run per pair: a release of one page waits until a larger run has finished
		// (PUB-452, decision D25).
		if err := s.refuseWhileRunning(ctx, tx, p.ID); err != nil {
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
	if pl.action == actionNone {
		return out, s.finishRun(ctx, tx, c, pl, out, in, nil)
	}
	w, err := s.writePage(ctx, tx, c, &pl, pageWrite{note: in.Note, silent: in.Silent || !pl.pair.Settings.NotifyWatchers, runID: in.PreviewID})
	if err != nil {
		return out, err
	}
	out.TwinVersion, out.LinksAsText, out.FilesCopied = w.twinVersion, len(w.res.textLinks), w.filesCopied
	// The twin takes its place among its siblings as in the draft (PUB-412).
	if err := s.orderWritten(ctx, tx, batchState{pair: pl.pair, published: pl.published},
		[]writtenPage{{draft: pl.src.ID, draftParent: pl.src.ParentID, twin: w.twin, twinParent: pl.place.Parent}}); err != nil {
		return out, err
	}
	return out, s.finishRun(ctx, tx, c, pl, out, in, &w.res)
}

// pageWrite is how a run writes one page: the note and silence of the run, and the run.
type pageWrite struct {
	note   string
	silent bool
	runID  uuid.UUID
	// bulk is set for a run of more than one page, whose watchers hear of the run once
	// (PUB-813); a release of one page tells them as a change of that page.
	bulk *uuid.UUID
}

// written is what writing one page did.
type written struct {
	twin        uuid.UUID
	twinVersion int
	filesCopied int
	res         rewritten
}

// writePage writes one page of a run onto its twin: the twin is made if it has none, the
// files it shows are copied when they changed, the document is rewritten and passes the gate,
// and the facts of the release are kept (PUB-410 to PUB-412, PUB-320, PUB-500 to PUB-534).
func (s *Service) writePage(ctx context.Context, tx pgx.Tx, c identity.Caller, pl *plan, w pageWrite) (written, error) {
	var out written
	twin := twinOf(pl.existing)
	if twin == nil && pl.made != nil {
		twin = pl.made
		pl.existing = &pagePair{Twin: *pl.made}
	}
	if twin == nil {
		made, err := s.makeTwin(ctx, tx, c, pl.pair, pl.published.ID, pl.place.Parent, pl.src.Page)
		if err != nil {
			return out, err
		}
		twin = &made.ID
		pl.existing = &pagePair{Twin: made.ID}
	}
	out.twin = *twin
	files := map[uuid.UUID]uuid.UUID{}
	for i, f := range pl.files {
		if !f.needed() {
			files[f.from] = *f.copy
			continue
		}
		copied, version, err := s.attachments.CopyForRelease(ctx, tx, c, attachments.ReleaseCopy{From: f.from, Copy: f.copy, TwinPage: *twin, SpaceID: pl.published.ID, Pin: f.pin})
		if err != nil {
			return out, err
		}
		files[f.from], pl.files[i].copy, pl.files[i].copied = copied, &copied, version
		out.filesCopied++
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
	// The look and the labels are written when the twin is new or they changed (PUB-303, PUB-304).
	write := pages.TwinWrite{ID: *twin, Parent: pl.place.Parent, Look: pl.src.Look, Labels: pl.src.Labels,
		Same: pl.action != actionMake && !slices.Contains(pl.aspects, "look") && !slices.Contains(pl.aspects, "labels")}
	write.Look.HeaderAttachmentID = nil
	if h := pl.src.HeaderAttachmentID; h != nil {
		if copied, ok := files[*h]; ok {
			write.Look.HeaderAttachmentID = &copied
		}
	}
	if pl.action == actionMake || pl.action == actionChange {
		// The releaser is the author and the note the summary, never the draft's (PUB-1408).
		write.Version = &pages.TwinVersion{Title: twinTitle(*pl), Doc: doc, Summary: w.note, Silent: w.silent, Run: w.bulk}
	}
	twinPage, err := s.pages.WriteTwin(ctx, tx, c, write)
	if err != nil {
		return out, err
	}
	if pl.unarchive {
		if err := s.pages.SetTwinArchived(ctx, tx, c, *twin, false); err != nil {
			return out, err
		}
	}
	out.twinVersion, out.res = twinPage.CurrentVersion, res
	if err := s.keepFacts(ctx, tx, c, *pl, twinPage, res, w.runID); err != nil {
		return out, err
	}
	return out, s.keepFiles(ctx, tx, c, *pl)
}

// makeTwin makes the twin of a draft page. A twin a run of the system makes has no person as its
// maker, so the person who made the pair answers for it, as the owner readers see in its byline;
// its versions keep no person as author (decision D41).
func (s *Service) makeTwin(ctx context.Context, tx pgx.Tx, c identity.Caller, p pair, space uuid.UUID, parent *uuid.UUID, src pages.Page) (pages.Page, error) {
	made, err := s.pages.MakeTwin(ctx, tx, c, space, parent, src)
	if err != nil || c.System == "" {
		return made, err
	}
	return made, pages.SetTwinOwner(ctx, tx, made.ID, p.CreatedBy)
}

// twinTitle is the title a twin gets. A home page is named after its space until someone
// renames it, so a draft home page with the draft space's name gives readers the name of their
// own space instead: the name of the draft space never reaches them (PUB-108, decision D28).
func twinTitle(pl plan) string {
	if pl.draft.HomePageID != nil && *pl.draft.HomePageID == pl.src.ID && pl.version.Title == pl.draft.Name {
		return pl.published.Name
	}
	return pl.version.Title
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
	// The draft may have changed since the preview; orderWritten compares every page of the
	// group again, so a newer version stays Changed since release (PUB-431).
	return nil
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
		counts = $4, report = $5, total = 1, done = 1 WHERE id = $1`, in.PreviewID, in.Note, in.Silent, counts, out); err != nil {
		return err
	}
	return s.recordPage(ctx, tx, c, pl, pageRecord{runID: in.PreviewID, note: in.Note, silent: in.Silent, outcome: outcome,
		twin: twin, twinVersion: out.TwinVersion, twinHash: twinHash, links: links})
}

// pageRecord is what a run did to one page, for its row and its audit.
type pageRecord struct {
	runID       uuid.UUID
	note        string
	silent      bool
	outcome     string
	twin        *uuid.UUID
	twinVersion int
	twinHash    string
	links       map[string]string
	// hash is the content hash of the draft version released (PUB-732, D9).
	hash string
	// causedBy is the person whose act started a run of the system (PUB-902).
	causedBy *uuid.UUID
}

// recordPage keeps what a run did to one page, and audits it: in the draft space with both
// sides, in the published space with the twin only (PUB-900 to PUB-903, PUB-922, PUB-923).
func (s *Service) recordPage(ctx context.Context, tx pgx.Tx, c identity.Caller, pl plan, r pageRecord) error {
	if _, err := tx.Exec(ctx, `UPDATE release_run_pages SET twin_page_id = $3, twin_version = $4, outcome = $5, link_map = $6,
		content_hash = COALESCE(NULLIF($7, ''), content_hash) WHERE run_id = $1 AND draft_page_id = $2`,
		r.runID, pl.src.ID, r.twin, r.twinVersion, r.outcome, r.links, r.hash); err != nil {
		return err
	}
	details := map[string]any{"pair_id": pl.pair.ID, "page_id": pl.src.ID, "version": pl.number, "content_hash": pl.version.Hash,
		"action": pl.action, "aspects": pl.aspects, "placed_higher": pl.place.Higher, "note": r.note, "silent": r.silent, "outcome": r.outcome,
		"run_id": r.runID}
	if r.twin != nil {
		details["twin_page_id"], details["twin_version"], details["twin_hash"] = *r.twin, r.twinVersion, r.twinHash
	}
	if r.causedBy != nil {
		details["caused_by"] = *r.causedBy
	}
	if _, err := audit.Record(ctx, tx, c.Event("release.page", "page", pl.src.ID.String(), pl.pair.Draft.String(), details)); err != nil {
		return err
	}
	if r.twin == nil {
		return nil
	}
	if _, err := audit.Record(ctx, tx, c.Event("page.released", "page", r.twin.String(), pl.published.ID.String(),
		map[string]any{"run_id": r.runID, "version": r.twinVersion, "content_hash": r.twinHash, "note": r.note})); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, "page.released", map[string]any{"page_id": *r.twin, "space_id": pl.published.ID, "version": r.twinVersion,
		"run_id": r.runID, "actor_id": c.UserID})
	return err
}

func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}
