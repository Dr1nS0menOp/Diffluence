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
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

// Runs of more than one page (PUB-401, PUB-402, PUB-430, PUB-440): a branch, the whole space,
// a choice of pages from the release screen, or taking pages off. Each starts as a preview
// that holds the versions it will release; confirming it queues the run for the worker.

// The scopes of a run.
const (
	ScopePage    = "page"
	ScopeBranch  = "branch"
	ScopeSpace   = "space"
	ScopePages   = "pages"
	ScopeTakeOff = "take_off"
)

// What a run does to a page, besides those of a single page (make, change, update, none).
const (
	actionTakeOff = "take_off"
	actionArchive = "archive"
	actionSkip    = "skip"
)

// Why a page in the scope of a run is not in it.
const (
	skipLeftOut     = "left_out"
	skipBelowLeft   = "below_left_out"
	skipRestricted  = "restricted"
	skipGone        = "gone"
	skipFileMissing = "file_missing"
	skipBelowFailed = "below_failed"
	skipHidden      = "hidden"
)

// RunScope says what a run takes.
type RunScope struct {
	Scope string `json:"scope"`
	// RootPageID is the top of a branch.
	RootPageID *uuid.UUID `json:"root_page_id"`
	// PageIDs are the pages chosen on the release screen, or those to take off: draft pages,
	// or twins whose draft page is gone.
	PageIDs []uuid.UUID `json:"page_ids"`
	// Named are pages with a view restriction that the person names, so they go too (PUB-422).
	Named []uuid.UUID `json:"named"`
	// WithChildren takes off the pages below as well (PUB-440).
	WithChildren bool `json:"with_children"`
	// KeepRemovals unticks taking off what was removed in the draft (PUB-1410).
	KeepRemovals bool `json:"keep_removals"`
}

func (in RunScope) check() error {
	switch in.Scope {
	case ScopeSpace:
	case ScopeBranch:
		if in.RootPageID == nil {
			return apperr.Invalid("Choose the page at the top of the branch.")
		}
	case ScopePages, ScopeTakeOff:
		if len(in.PageIDs) == 0 {
			return apperr.Invalid("Choose at least one page.")
		}
	default:
		return apperr.Invalid("A run releases a branch, the whole space, chosen pages, or takes pages off.")
	}
	return nil
}

// RunPage is one page in a preview or a report, named only for those who may see it.
type RunPage struct {
	PageID       uuid.UUID  `json:"page_id"`
	TwinPageID   *uuid.UUID `json:"twin_page_id,omitempty"`
	Title        string     `json:"title"`
	Action       string     `json:"action"`
	Aspects      []string   `json:"aspects"`
	Reason       string     `json:"reason,omitempty"`
	PlacedHigher bool       `json:"placed_higher,omitempty"`
	Outcome      string     `json:"outcome,omitempty"`
}

// RunCounts are the numbers of a preview, each also a list (PUB-430).
type RunCounts struct {
	Make       int `json:"make"`
	Change     int `json:"change"`
	Update     int `json:"update"`
	TakeOff    int `json:"take_off"`
	Archive    int `json:"archive"`
	LeftOut    int `json:"left_out"`
	Restricted int `json:"restricted"`
	Cannot     int `json:"cannot"`
	Hidden     int `json:"hidden"`
}

// writes is the number of pages a run writes.
func (c RunCounts) writes() int { return c.Make + c.Change + c.Update + c.TakeOff + c.Archive }

// RunPreview is what a run would do (PUB-430), held for an hour (PUB-433).
type RunPreview struct {
	ID      uuid.UUID `json:"id"`
	Scope   string    `json:"scope"`
	Counts  RunCounts `json:"counts"`
	Pages   []RunPage `json:"pages"`
	Skipped []RunPage `json:"skipped"`
	// KeptBelow are twins below those taken off that stay, moving up (PUB-440).
	KeptBelow   []RunPage `json:"kept_below"`
	Files       int       `json:"files"`
	Bytes       int64     `json:"bytes"`
	Room        *int64    `json:"room"`
	Fits        bool      `json:"fits"`
	LinksAsText int       `json:"links_as_text"`
	// Batches is 0 for one transaction; past that readers see the run land in steps (PUB-451).
	Batches   int       `json:"batches"`
	ExpiresAt time.Time `json:"expires_at"`
}

// scopeRow is a draft page in the scope of a run, with what decides its part in it.
type scopeRow struct {
	ID              uuid.UUID
	ParentID        *uuid.UUID
	Title           string
	Kind            string
	Status          string
	CurrentVersion  int
	Archived        bool
	Restricted      bool
	Visible         bool
	NeverApproved   bool
	ReleasedVersion int
	Twin            *uuid.UUID
	Changed         []string
	LeftOut         bool
	TwinArchived    bool
	Header          *uuid.UUID
}

func (r scopeRow) released() bool { return r.ReleasedVersion > 0 }

// scopeSQL reads the draft pages of a scope, parents before children. Visibility is the rule
// of perm_visible_pages for a person who views the space, which the releaser does (PUB-413).
const scopeSQL = `SELECT p.id, p.parent_id, p.title, p.kind, p.status, p.current_version, p.archived_at IS NOT NULL,
		EXISTS (SELECT 1 FROM page_tree t JOIN page_restrictions r ON r.page_id = t.ancestor_id AND r.kind = 'view'
			WHERE t.descendant_id = p.id),
		p.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($2) h) AND p.id NOT IN (SELECT ch.page_id FROM perm_control_hidden_pages($2) ch),
		EXISTS (SELECT 1 FROM page_control pc WHERE pc.page_id = p.id AND (pc.approved_version IS NULL OR pc.state_kind = 'obsolete')),
		COALESCE(pp.released_version, 0), pp.twin_page_id, COALESCE(pp.changed, '{}'),
		EXISTS (SELECT 1 FROM page_tree t JOIN release_exclusions e ON e.draft_page_id = t.ancestor_id
			WHERE t.descendant_id = p.id AND (t.depth = 0 OR e.with_children)),
		COALESCE(tw.archived_at IS NOT NULL, false), p.header_attachment_id
	FROM pages p
	LEFT JOIN page_pairs pp ON pp.draft_page_id = p.id AND pp.pair_id = $3
	LEFT JOIN pages tw ON tw.id = pp.twin_page_id
	WHERE p.space_id = $1 AND p.kind = ANY($4) AND (p.status = 'published' OR (p.status = 'trashed' AND pp.released_version > 0))
	  AND ($5::uuid IS NULL OR p.id IN (SELECT d.descendant_id FROM page_tree d WHERE d.ancestor_id = $5))
	  AND ($6::uuid[] IS NULL OR p.id = ANY($6))
	ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id), p.parent_id, p.position, p.id`

// scopeKinds are the kinds a run looks at: those released, and databases, which it lists as
// never released (PUB-230, PUB-232). Blog posts are not in the tree and never in a run.
var scopeKinds = []string{pages.KindPage, pages.KindFolder, pages.KindLink, pages.KindDatabase}

func readScope(ctx context.Context, q db.Querier, user uuid.UUID, p pair, root *uuid.UUID, only []uuid.UUID) ([]scopeRow, error) {
	var list []uuid.UUID
	if len(only) > 0 {
		list = only
	}
	rows, err := q.Query(ctx, scopeSQL, p.Draft, user, p.ID, scopeKinds, root, list)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (scopeRow, error) {
		var s scopeRow
		err := r.Scan(&s.ID, &s.ParentID, &s.Title, &s.Kind, &s.Status, &s.CurrentVersion, &s.Archived, &s.Restricted, &s.Visible,
			&s.NeverApproved, &s.ReleasedVersion, &s.Twin, &s.Changed, &s.LeftOut, &s.TwinArchived, &s.Header)
		return s, err
	})
}

// decision is what a run does with one page of its scope.
type decision struct {
	action  string
	reason  string
	aspects []string
}

// scopeRules are what decide pages beyond the page itself.
type scopeRules struct {
	// mirror takes off and archives what was removed or archived in the draft (PUB-1410).
	mirror bool
	// named are pages with a view restriction that go anyway (PUB-422).
	named map[uuid.UUID]bool
	// chosen says every page was chosen one by one, which names them all.
	chosen bool
}

// decide says what a run does with a page (PUB-401 to PUB-403, PUB-420 to PUB-422, PUB-1010,
// PUB-1013). skippedAbove holds the pages left out without a twin above it: a page below one
// is skipped too, as releasing it would put it where readers do not expect it (PUB-401).
func decide(r scopeRow, rules scopeRules, skippedAbove map[uuid.UUID]bool) decision {
	switch {
	case r.Status == pages.StatusTrashed:
		if rules.mirror {
			return decision{action: actionTakeOff, reason: "deleted"}
		}
		return decision{action: actionNone}
	case r.Kind != pages.KindPage && r.Kind != pages.KindFolder && r.Kind != pages.KindLink:
		return decision{action: actionSkip, reason: reasonKind}
	case r.LeftOut:
		return decision{action: actionSkip, reason: skipLeftOut}
	case r.ParentID != nil && skippedAbove[*r.ParentID]:
		return decision{action: actionSkip, reason: skipBelowLeft}
	case r.Archived && r.released() && !r.TwinArchived:
		if rules.mirror {
			return decision{action: actionArchive, reason: "archived"}
		}
		return decision{action: actionNone}
	case r.Archived && r.released():
		return decision{action: actionNone}
	case r.Archived:
		return decision{action: actionSkip, reason: reasonArchived}
	case r.NeverApproved:
		return decision{action: actionSkip, reason: reasonNotApproved}
	case r.Restricted && !r.released() && !rules.chosen && !rules.named[r.ID]:
		return decision{action: actionSkip, reason: skipRestricted}
	case !r.released():
		return decision{action: actionMake, aspects: []string{}}
	case len(r.Changed) == 0:
		return decision{action: actionNone}
	}
	if slices.Contains(r.Changed, "text") || slices.Contains(r.Changed, "title") || slices.Contains(r.Changed, aspectLinks) {
		return decision{action: actionChange, aspects: r.Changed}
	}
	return decision{action: actionUpdate, aspects: r.Changed}
}

// runRights checks the rights of a run at the moment of acting: space.release in the draft
// space and view of the published space (PUB-413, PUB-454). The draft space is looked at
// first, so someone who may not see it learns nothing.
func (s *Service) runRights(ctx context.Context, q db.Querier, user uuid.UUID, p pair) error {
	if err := s.perms.RequireSpace(ctx, q, user, p.Draft, permissions.SpaceRelease); err != nil {
		return err
	}
	if d, err := s.perms.CheckSpace(ctx, q, user, p.Published, permissions.SpaceView); err != nil || !d.Allowed {
		if err != nil && !apperr.IsNotFound(err) {
			return err
		}
		return apperr.Conflict("published_space_hidden", "You cannot see the published space, so you cannot release into it.")
	}
	return nil
}

// draftPair finds the pair of a draft space for someone who may release from it.
func (s *Service) draftPair(ctx context.Context, q db.Querier, c identity.Caller, spaceID uuid.UUID, lock bool) (pair, error) {
	if err := s.perms.RequireSpace(ctx, q, c.UserID, spaceID, permissions.SpaceRelease); err != nil {
		return pair{}, err
	}
	p, err := activePairOf(ctx, q, spaceID, lock)
	if err != nil {
		return p, err
	}
	if p.Draft != spaceID {
		return p, apperr.Conflict("not_the_draft_space", "Releases are made from the draft space.")
	}
	return p, s.runRights(ctx, q, c.UserID, p)
}

// bothActive refuses a run while either space is archived or in the trash (PUB-1048).
func bothActive(ctx context.Context, q db.Querier, p pair) (spaces.Space, spaces.Space, error) {
	draft, err := spaces.LoadIn(ctx, q, p.Draft)
	if err != nil {
		return draft, spaces.Space{}, err
	}
	published, err := spaces.LoadIn(ctx, q, p.Published)
	if err != nil {
		return draft, published, err
	}
	if draft.Status != spaces.StatusActive || published.Status != spaces.StatusActive {
		return draft, published, apperr.Conflict("space_not_active", "Both spaces of the pair must be in use to release. Unarchive them first.")
	}
	return draft, published, nil
}

// PreviewRun works out a run of more than one page and keeps it for an hour (PUB-430).
func (s *Service) PreviewRun(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in RunScope) (RunPreview, error) {
	out := RunPreview{Pages: []RunPage{}, Skipped: []RunPage{}, KeptBelow: []RunPage{}}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// The right first, so nobody learns more than that they may not (deny by default).
		p, err := s.draftPair(ctx, tx, c, spaceID, false)
		if err != nil {
			return err
		}
		if err := in.check(); err != nil {
			return err
		}
		_, published, err := bothActive(ctx, tx, p)
		if err != nil {
			return err
		}
		var plan []runPagePlan
		if in.Scope == ScopeTakeOff {
			plan, err = s.planTakeOff(ctx, tx, c, p, published, in, &out)
		} else {
			plan, err = s.planRelease(ctx, tx, c, p, in, &out)
		}
		if err != nil {
			return err
		}
		if out.Counts.writes() > s.limits.MaxPages {
			return apperr.Conflict("run_too_large", fmt.Sprintf("A run takes at most %d pages. Release this space branch by branch.", s.limits.MaxPages))
		}
		if out.Counts.writes() == 0 {
			return apperr.Conflict("nothing_to_release", "Everything in this choice is released. Pages you change will show on the release screen.")
		}
		left, limited, err := attachments.RoomForRelease(ctx, tx, published.ID)
		if err != nil {
			return err
		}
		out.Fits = !limited || out.Bytes <= left
		if limited {
			out.Room = &left
		}
		out.ID, out.Scope = ids.New(), in.Scope
		out.Batches = s.limits.batchesFor(out.Counts.writes(), out.Bytes)
		return s.keepPreview(ctx, tx, c, p, in, plan, &out)
	})
	return out, err
}

// runPagePlan is a page of a previewed run, as its row keeps it.
type runPagePlan struct {
	draft   uuid.UUID
	version int
	twin    *uuid.UUID
	action  string
	reason  string
	title   string
	aspects []string
}

func (s *Service) planRelease(ctx context.Context, tx pgx.Tx, c identity.Caller, p pair, in RunScope, out *RunPreview) ([]runPagePlan, error) {
	var only []uuid.UUID
	if in.Scope == ScopePages {
		only = in.PageIDs
	}
	var root *uuid.UUID
	if in.Scope == ScopeBranch {
		root = in.RootPageID
		if err := s.perms.RequirePage(ctx, tx, c.UserID, *root, permissions.ActionPageView, uuid.Nil); err != nil {
			return nil, err
		}
	}
	rows, err := readScope(ctx, tx, c.UserID, p, root, only)
	if err != nil {
		return nil, err
	}
	rules := scopeRules{mirror: in.Scope == ScopeSpace && p.Settings.MirrorRemovals && !in.KeepRemovals, named: map[uuid.UUID]bool{},
		chosen: in.Scope == ScopePages}
	for _, id := range in.Named {
		rules.named[id] = true
	}
	skippedAbove := map[uuid.UUID]bool{}
	var plan []runPagePlan
	var writing []uuid.UUID
	for _, r := range rows {
		if !r.Visible {
			// A page restricted from the releaser is not in the run, and is counted without its
			// title (PUB-413).
			out.Counts.Hidden++
			if !r.released() {
				skippedAbove[r.ID] = true
			}
			continue
		}
		d := decide(r, rules, skippedAbove)
		page := RunPage{PageID: r.ID, TwinPageID: r.Twin, Title: r.Title, Action: d.action, Aspects: nonNilAspects(d.aspects), Reason: d.reason}
		switch d.action {
		case actionNone:
			continue
		case actionSkip:
			out.Skipped = append(out.Skipped, page)
			switch d.reason {
			case skipLeftOut, skipBelowLeft:
				out.Counts.LeftOut++
			case skipRestricted:
				out.Counts.Restricted++
			default:
				out.Counts.Cannot++
			}
			if !r.released() {
				skippedAbove[r.ID] = true
			}
			plan = append(plan, runPagePlan{draft: r.ID, twin: r.Twin, action: actionSkip, reason: d.reason, title: r.Title})
			continue
		case actionMake:
			out.Counts.Make++
		case actionChange:
			out.Counts.Change++
		case actionUpdate:
			out.Counts.Update++
		case actionTakeOff:
			out.Counts.TakeOff++
		case actionArchive:
			out.Counts.Archive++
		}
		out.Pages = append(out.Pages, page)
		plan = append(plan, runPagePlan{draft: r.ID, version: r.CurrentVersion, twin: r.Twin, action: d.action, reason: d.reason,
			title: r.Title, aspects: page.Aspects})
		if d.action == actionMake || d.action == actionChange || d.action == actionUpdate {
			writing = append(writing, r.ID)
		}
	}
	if rules.mirror {
		gone, err := s.removedTwins(ctx, tx, c, p, nil)
		if err != nil {
			return nil, err
		}
		for _, g := range gone {
			out.Counts.TakeOff++
			out.Pages = append(out.Pages, RunPage{PageID: g.draft, TwinPageID: &g.twin, Title: g.title, Action: actionTakeOff, Aspects: []string{}, Reason: g.cause})
			twin := g.twin
			plan = append(plan, runPagePlan{draft: g.draft, twin: &twin, action: actionTakeOff, reason: g.cause, title: g.title})
		}
	}
	return plan, s.measureWrites(ctx, tx, p, rows, writing, out)
}

func nonNilAspects(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// removedTwin is a twin whose draft page left the pair for good: moved away or purged.
type removedTwin struct {
	twin  uuid.UUID
	draft uuid.UUID
	title string
	cause string
}

// removedTwins lists the twins whose draft pages left the pair for good, which the person may
// see; only lists those among only when it is set.
func (s *Service) removedTwins(ctx context.Context, q db.Querier, c identity.Caller, p pair, only []uuid.UUID) ([]removedTwin, error) {
	rows, err := q.Query(ctx, `SELECT r.twin_page_id, r.draft_page_id, t.title, r.cause FROM release_removals r
		JOIN pages t ON t.id = r.twin_page_id AND t.status <> 'trashed'
		WHERE r.pair_id = $1 AND ($2::uuid[] IS NULL OR r.twin_page_id = ANY($2))
		  AND t.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($3) h)
		ORDER BY r.removed_at, r.twin_page_id`, p.ID, nilIfEmpty(only), c.UserID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (removedTwin, error) {
		var g removedTwin
		err := r.Scan(&g.twin, &g.draft, &g.title, &g.cause)
		return g, err
	})
}

func nilIfEmpty(list []uuid.UUID) []uuid.UUID {
	if len(list) == 0 {
		return nil
	}
	return list
}

// measureWrites counts the files a run copies and the links that stay text, from the current
// documents of the pages it writes (PUB-430, PUB-1104).
func (s *Service) measureWrites(ctx context.Context, tx pgx.Tx, p pair, rows []scopeRow, writing []uuid.UUID, out *RunPreview) error {
	if len(writing) == 0 {
		return nil
	}
	inRun := map[uuid.UUID]bool{}
	twinned := map[uuid.UUID]bool{}
	headers := map[uuid.UUID]*uuid.UUID{}
	twins := map[uuid.UUID]uuid.UUID{}
	for _, r := range rows {
		if r.released() {
			twinned[r.ID] = true
		}
		headers[r.ID] = r.Header
		if r.Twin != nil {
			twins[r.ID] = *r.Twin
		}
	}
	for _, id := range writing {
		inRun[id] = true
	}
	const chunk = 500
	var shown []uuid.UUID
	shownBy := map[uuid.UUID][]uuid.UUID{}
	referenced := map[uuid.UUID]bool{}
	for start := 0; start < len(writing); start += chunk {
		docs, err := pages.CurrentDocuments(ctx, tx, writing[start:min(start+chunk, len(writing))])
		if err != nil {
			return err
		}
		for id, doc := range docs {
			files := document.FilesShown(doc)
			if h := headers[id]; h != nil && !slices.Contains(files, *h) {
				files = append(files, *h)
			}
			shownBy[id] = files
			shown = append(shown, files...)
			for _, target := range referencedPages(doc) {
				if !inRun[target] && !twinned[target] {
					referenced[target] = true
				}
			}
		}
	}
	targets, err := pages.ReleaseTargets(ctx, tx, mapKeys(referenced))
	if err != nil {
		return err
	}
	for _, t := range targets {
		if t.SpaceID == p.Draft {
			out.LinksAsText++
		}
	}
	sources, err := attachments.SourcesForRelease(ctx, tx, shown)
	if err != nil {
		return err
	}
	copied, err := copiedVersions(ctx, tx, mapValues(twins))
	if err != nil {
		return err
	}
	for id, files := range shownBy {
		for _, f := range files {
			src, ok := sources[f]
			if !ok || src.SpaceID != p.Draft {
				continue
			}
			if v, ok := copied[fileOnTwin{file: f, twin: twins[id]}]; ok && v == src.Version {
				continue
			}
			out.Files++
			out.Bytes += src.Size
		}
	}
	return nil
}

// fileOnTwin is a draft file and a twin that holds a copy of it.
type fileOnTwin struct{ file, twin uuid.UUID }

// copiedVersions reads which version of each draft file the twins among list hold.
func copiedVersions(ctx context.Context, q db.Querier, list []uuid.UUID) (map[fileOnTwin]int, error) {
	out := map[fileOnTwin]int{}
	if len(list) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT draft_attachment_id, twin_page_id, copied_version FROM file_pairs WHERE twin_page_id = ANY($1)`, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k fileOnTwin
		var v int
		if err := rows.Scan(&k.file, &k.twin, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func mapKeys[V any](m map[uuid.UUID]V) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func mapValues(m map[uuid.UUID]uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// planTakeOff works out taking pages off (PUB-440): the twins of the pages chosen, with those
// of the pages below when asked, and the twins below that stay and move up.
func (s *Service) planTakeOff(ctx context.Context, tx pgx.Tx, c identity.Caller, p pair, published spaces.Space, in RunScope, out *RunPreview) ([]runPagePlan, error) {
	var plan []runPagePlan
	var chosen []uuid.UUID
	taken := map[uuid.UUID]bool{}
	add := func(draft, twin uuid.UUID, title, reason string) {
		if taken[twin] {
			return
		}
		taken[twin] = true
		t := twin
		out.Counts.TakeOff++
		out.Pages = append(out.Pages, RunPage{PageID: draft, TwinPageID: &t, Title: title, Action: actionTakeOff, Aspects: []string{}, Reason: reason})
		plan = append(plan, runPagePlan{draft: draft, twin: &t, action: actionTakeOff, title: title, reason: reason})
		chosen = append(chosen, twin)
	}
	gone, err := s.removedTwins(ctx, tx, c, p, in.PageIDs)
	if err != nil {
		return nil, err
	}
	for _, g := range gone {
		add(g.draft, g.twin, g.title, g.cause)
	}
	var drafts []uuid.UUID
	for _, id := range in.PageIDs {
		if !slices.ContainsFunc(gone, func(g removedTwin) bool { return g.twin == id }) {
			drafts = append(drafts, id)
		}
	}
	if len(drafts) > 0 {
		var roots []uuid.UUID
		for _, id := range drafts {
			if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
				return nil, err
			}
			roots = append(roots, id)
		}
		rows, err := tx.Query(ctx, `SELECT pp.draft_page_id, pp.twin_page_id, t.title FROM page_pairs pp
			JOIN pages t ON t.id = pp.twin_page_id AND t.status <> 'trashed'
			JOIN pages d ON d.id = pp.draft_page_id
			WHERE pp.pair_id = $1 AND pp.released_version > 0
			  AND (pp.draft_page_id = ANY($2) OR pp.twin_page_id = ANY($2) OR ($3 AND pp.draft_page_id IN
				(SELECT descendant_id FROM page_tree WHERE ancestor_id = ANY($2))))
			  AND d.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($4) h)
			ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = d.id), d.position, d.id`, p.ID, roots, in.WithChildren, c.UserID)
		if err != nil {
			return nil, err
		}
		type found struct {
			draft, twin uuid.UUID
			title       string
		}
		list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (found, error) {
			var f found
			err := r.Scan(&f.draft, &f.twin, &f.title)
			return f, err
		})
		if err != nil {
			return nil, err
		}
		for _, f := range list {
			add(f.draft, f.twin, f.title, "")
		}
	}
	if published.HomePageID != nil && taken[*published.HomePageID] {
		return nil, apperr.Conflict("home_page", "The home page of the published space cannot be taken off.")
	}
	kept, err := tx.Query(ctx, `SELECT c.id, c.title FROM pages c WHERE c.parent_id = ANY($1) AND NOT (c.id = ANY($1))
		AND c.status <> 'trashed' AND c.id NOT IN (SELECT h.page_id FROM perm_hidden_pages($2) h) ORDER BY c.position, c.id`, chosen, c.UserID)
	if err != nil {
		return nil, err
	}
	for kept.Next() {
		var id uuid.UUID
		var title string
		if err := kept.Scan(&id, &title); err != nil {
			kept.Close()
			return nil, err
		}
		t := id
		out.KeptBelow = append(out.KeptBelow, RunPage{PageID: id, TwinPageID: &t, Title: title, Action: actionNone, Aspects: []string{}})
	}
	kept.Close()
	return plan, kept.Err()
}

// keepPreview stores a previewed run and its pages, in the order they will be written.
func (s *Service) keepPreview(ctx context.Context, tx pgx.Tx, c identity.Caller, p pair, in RunScope, plan []runPagePlan, out *RunPreview) error {
	options, err := json.Marshal(in)
	if err != nil {
		return err
	}
	counts := map[string]any{"counts": out.Counts, "files": out.Files, "bytes": out.Bytes, "links_as_text": out.LinksAsText, "fits": out.Fits}
	if err := tx.QueryRow(ctx, `INSERT INTO release_runs (id, pair_id, scope, root_page_id, status, actor_id, counts, options, total, batches)
		VALUES ($1, $2, $3, $4, 'previewed', $5, $6, $7, $8, $9) RETURNING previewed_at + $10::interval`,
		out.ID, p.ID, in.Scope, in.RootPageID, c.UserID, counts, options, out.Counts.writes(), out.Batches, previewLife.String()).Scan(&out.ExpiresAt); err != nil {
		return err
	}
	drafts := make([]uuid.UUID, len(plan))
	versions := make([]int, len(plan))
	twins := make([]*uuid.UUID, len(plan))
	actions := make([]string, len(plan))
	reasons := make([]string, len(plan))
	titles := make([]string, len(plan))
	outcomes := make([]string, len(plan))
	aspects := make([]string, len(plan))
	for i, pp := range plan {
		drafts[i], versions[i], twins[i], actions[i], reasons[i], titles[i] = pp.draft, pp.version, pp.twin, pp.action, pp.reason, pp.title
		outcomes[i] = "waiting"
		if pp.action == actionSkip {
			outcomes[i] = "skipped"
		}
		raw, err := json.Marshal(nonNilAspects(pp.aspects))
		if err != nil {
			return err
		}
		aspects[i] = string(raw)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO release_run_pages (run_id, draft_page_id, draft_version, twin_page_id, action, outcome, reason, title, ord, aspects)
		SELECT $1, d, v, t, a, o, r, ti, n, ARRAY(SELECT jsonb_array_elements_text(asp::jsonb))
		FROM unnest($2::uuid[], $3::int[], $4::uuid[], $5::text[], $6::text[], $7::text[], $8::text[], $9::text[]) WITH ORDINALITY
			AS x(d, v, t, a, o, r, ti, asp, n)
		ON CONFLICT (run_id, draft_page_id) DO NOTHING`,
		out.ID, drafts, versions, twins, actions, outcomes, reasons, titles, aspects); err != nil {
		return err
	}
	_, err = audit.Record(ctx, tx, c.Event("release.previewed", "release_run", out.ID.String(), p.Draft.String(),
		map[string]any{"pair_id": p.ID, "scope": in.Scope, "counts": out.Counts, "batches": out.Batches}))
	return err
}

// errRunGoing is the answer while a run of the pair is queued or going (PUB-244, PUB-452).
func errRunGoing() error {
	return apperr.Conflict("run_going", "A release of this space is going. Try again when it has finished.")
}

// runGoing says whether a run of a pair is queued, or running with a recent sign of life.
func runGoing(ctx context.Context, q db.Querier, pairID uuid.UUID, staleAfter time.Duration) (bool, error) {
	var going bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM release_runs WHERE pair_id = $1 AND (status = 'queued'
		OR (status = 'running' AND heartbeat_at > now() - $2::interval)))`, pairID, staleAfter.String()).Scan(&going)
	return going, err
}

// refuseWhileRunning refuses a change of a pair while a larger run is going.
func (s *Service) refuseWhileRunning(ctx context.Context, q db.Querier, pairID uuid.UUID) error {
	going, err := runGoing(ctx, q, pairID, s.limits.StaleAfter)
	if err != nil {
		return err
	}
	if going {
		return errRunGoing()
	}
	return nil
}

// noRows says whether err means a row was not there.
func noRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
