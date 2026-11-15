// SPDX-License-Identifier: Apache-2.0

package publishing

import (
	"context"
	"encoding/json"
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
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/spaces"
)

// Starting a draft space from a space that already has content (spec 073, wave PB4, PUB-1302 to
// PUB-1311). The space becomes the published space of a new pair; a new draft space gets a copy
// of every page, folder and link at its current version, with its files, labels, look and
// restrictions, each copy paired with its source in step; and the draft space gets only the
// grants of the old space that write, so its readers never see a draft.

// JobKindStart is the background work of a start: one transaction, whatever the size, so a
// half-made draft space is never seen (PUB-1309).
const JobKindStart = "publishing.start"

// scopeStart is the run that records a start, so verification, the history and undoing the start
// find what it did.
const scopeStart = "start"

// outcomeStarted marks a page a start paired with its copy. The page readers have was not written
// by the start, so it is no release: it tells nobody and the feed shows nothing of it.
const outcomeStarted = "started"

// StartInput names the new draft space.
type StartInput struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// StartPreview says what a start changes for whom, in numbers, before anyone confirms it
// (PUB-1309).
type StartPreview struct {
	// Pages are copied and paired; Hidden of them the person cannot see, and are copied with their
	// restrictions; Restricted carry a view restriction of their own.
	Pages      int `json:"pages"`
	Hidden     int `json:"hidden"`
	Restricted int `json:"restricted"`
	// Files are copied with the pages, Bytes their size.
	Files int   `json:"files"`
	Bytes int64 `json:"bytes"`
	// Posts and Databases stay in the space and become read-only, as everything in a published
	// space; Archived and Unpublished pages stay as they are, and are not copied.
	Posts       int `json:"posts"`
	Databases   int `json:"databases"`
	Archived    int `json:"archived"`
	Unpublished int `json:"unpublished"`
	// MoveToDraft are those who edit the space now: they will edit the draft space instead.
	MoveToDraft LosingEditors `json:"move_to_draft"`
	// Seconds is how long the start is likely to take; editing the space waits meanwhile.
	Seconds int `json:"seconds"`
}

// startPayload is what the job of a start carries.
type startPayload struct {
	SpaceID uuid.UUID `json:"space_id"`
	Key     string    `json:"key"`
	Name    string    `json:"name"`
}

// StartResult is what a start made, as its job's result.
type StartResult struct {
	DraftSpaceID uuid.UUID `json:"draft_space_id"`
	DraftKey     string    `json:"draft_key"`
	Pages        int       `json:"pages"`
}

// StartJob is the job of a start, for the screen to follow.
type StartJob struct {
	JobID uuid.UUID `json:"job_id"`
}

// requireStartRights checks the rights of a start: space.administer in the space and the right
// to create a space (PUB-1302). The right comes first, so nobody learns more than that they may not.
func (s *Service) requireStartRights(ctx context.Context, q db.Querier, c identity.Caller, spaceID uuid.UUID) error {
	if err := s.perms.RequireSpace(ctx, q, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return err
	}
	u, err := s.perms.LoadUser(ctx, q, c.UserID)
	if err != nil {
		return err
	}
	if !u.Active || !slices.Contains(u.Global, identity.PermCreateSpace) {
		return apperr.Forbidden()
	}
	return nil
}

// startable loads the space a start begins from and refuses one that cannot become a published
// space: a personal space, one not in use, one in a pair, one with pages under a workflow, or one
// larger than a run may be.
func (s *Service) startable(ctx context.Context, q db.Querier, spaceID uuid.UUID) (spaces.Space, startPlan, error) {
	sp, err := spaces.LoadIn(ctx, q, spaceID)
	if err != nil {
		return sp, startPlan{}, err
	}
	if err := checkSides(sp, nil); err != nil {
		return sp, startPlan{}, err
	}
	controlled, err := pages.ControlledPagesIn(ctx, q, spaceID)
	if err != nil {
		return sp, startPlan{}, err
	}
	if controlled > 0 {
		return sp, startPlan{}, apperr.Conflict("controlled_pages", fmt.Sprintf("%d pages of this space follow a workflow, and a published space "+
			"holds none. End their workflows first, or link a new published space instead.", controlled))
	}
	sources, err := pages.StartSources(ctx, q, spaceID)
	if err != nil {
		return sp, startPlan{}, err
	}
	plan := planStart(sp, sources)
	if len(plan.copies) > s.limits.MaxPages {
		return sp, plan, apperr.Conflict("too_many_pages", fmt.Sprintf("A draft space is started from a space of at most %d pages.", s.limits.MaxPages))
	}
	return sp, plan, nil
}

// startPlan is what a start copies, parents first, and what it leaves as it is.
type startPlan struct {
	copies []pages.Page
	// parent is the copied page each copy goes below: its own parent, or the nearest copied page
	// above it when its parent is not copied.
	parent      map[uuid.UUID]*uuid.UUID
	posts       int
	databases   int
	archived    int
	unpublished int
}

// copiedByStart says whether a start copies a page: a published page, folder or link that is not
// archived. Posts, databases and templates are not released, so they stay (PUB-231, PUB-232).
func copiedByStart(p pages.Page) bool {
	return p.Status == "published" && p.ArchivedAt == nil && slices.Contains([]string{pages.KindPage, pages.KindFolder, pages.KindLink}, p.Kind)
}

// planStart sorts the pages of a space into what a start copies, parents before their children
// and siblings in their order, and what it leaves.
func planStart(sp spaces.Space, list []pages.Page) startPlan {
	plan := startPlan{parent: map[uuid.UUID]*uuid.UUID{}}
	byID := map[uuid.UUID]pages.Page{}
	children := map[uuid.UUID][]pages.Page{}
	var roots []pages.Page
	for _, p := range list {
		byID[p.ID] = p
	}
	for _, p := range list {
		switch {
		case p.Kind == pages.KindPost:
			plan.posts++
		case p.Kind == pages.KindDatabase:
			plan.databases++
		case p.Kind == pages.KindTemplate:
		case p.ArchivedAt != nil:
			plan.archived++
		case p.Status != "published":
			plan.unpublished++
		}
		if _, ok := byID[derefOr(p.ParentID)]; ok && p.ParentID != nil {
			children[*p.ParentID] = append(children[*p.ParentID], p)
		} else {
			roots = append(roots, p)
		}
	}
	// The list comes ordered by position, so the order among siblings is kept.
	var walk func(p pages.Page, above *uuid.UUID)
	walk = func(p pages.Page, above *uuid.UUID) {
		next := above
		if copiedByStart(p) {
			plan.copies = append(plan.copies, p)
			plan.parent[p.ID] = above
			id := p.ID
			next = &id
		}
		for _, child := range children[p.ID] {
			walk(child, next)
		}
	}
	// The home page first: it is paired with the draft space's own home page.
	slices.SortStableFunc(roots, func(a, b pages.Page) int {
		return boolOrder(sp.HomePageID != nil && b.ID == *sp.HomePageID) - boolOrder(sp.HomePageID != nil && a.ID == *sp.HomePageID)
	})
	for _, r := range roots {
		walk(r, nil)
	}
	return plan
}

func boolOrder(b bool) int {
	if b {
		return 1
	}
	return 0
}

func derefOr(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}

// PreviewStart says what starting a draft space from a space would do, and refuses as the start
// would (PUB-1309).
func (s *Service) PreviewStart(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in StartInput) (StartPreview, error) {
	out := StartPreview{MoveToDraft: LosingEditors{Names: []string{}}}
	if err := s.requireStartRights(ctx, s.pool, c, spaceID); err != nil {
		return out, err
	}
	if err := checkStartInput(ctx, s.pool, in); err != nil {
		return out, err
	}
	_, plan, err := s.startable(ctx, s.pool, spaceID)
	if err != nil {
		return out, err
	}
	list := make([]uuid.UUID, len(plan.copies))
	for i, p := range plan.copies {
		list[i] = p.ID
	}
	out.Pages, out.Posts, out.Databases, out.Archived, out.Unpublished = len(list), plan.posts, plan.databases, plan.archived, plan.unpublished
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE id IN (SELECT h.page_id FROM perm_hidden_pages($2) h)) FROM unnest($1::uuid[]) id`,
		list, c.UserID).Scan(&out.Hidden); err != nil {
		return out, err
	}
	if out.Restricted, err = permissions.ViewRestrictedAmong(ctx, s.pool, list); err != nil {
		return out, err
	}
	if out.Files, out.Bytes, err = attachments.CopiedSizeOf(ctx, s.pool, list); err != nil {
		return out, err
	}
	holders, err := s.perms.HoldersOf(ctx, s.pool, spaceID, writePermissions())
	if err != nil {
		return out, err
	}
	out.MoveToDraft = losing(holders)
	out.Seconds = int(s.limits.estimate(len(list), out.Bytes).Round(time.Second).Seconds())
	return out, nil
}

// checkStartInput checks the name and key of the new draft space before any work.
func checkStartInput(ctx context.Context, q db.Querier, in StartInput) error {
	if err := spaces.CheckNew(spaces.Input{Key: in.Key, Name: in.Name}); err != nil {
		return err
	}
	free, err := spaces.KeyFreeIn(ctx, q, in.Key)
	if err != nil {
		return err
	}
	if !free {
		return apperr.Conflict("key_taken", "This key is already in use.")
	}
	return nil
}

// Start queues the start of a draft space from a space with content, after the same checks as
// the preview; the worker does the work in one transaction (PUB-1302, PUB-1309).
func (s *Service) Start(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in StartInput) (StartJob, error) {
	var out StartJob
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireStartRights(ctx, tx, c, spaceID); err != nil {
			return err
		}
		if err := checkStartInput(ctx, tx, in); err != nil {
			return err
		}
		if _, _, err := s.startable(ctx, tx, spaceID); err != nil {
			return err
		}
		id, err := s.jobs.Enqueue(ctx, tx, JobKindStart, startPayload{SpaceID: spaceID, Key: in.Key, Name: in.Name}, c.UserID)
		if err != nil {
			return err
		}
		out.JobID = id
		_, err = audit.Record(ctx, tx, c.Event("space.draft_start_asked", "space", spaceID.String(), spaceID.String(),
			map[string]any{"job_id": id, "key": in.Key}))
		return err
	})
	return out, err
}

// startJob carries out a start. A start that fails leaves nothing of itself.
func (s *Service) startJob(ctx context.Context, j jobs.Job) (any, error) {
	var in startPayload
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	c := identity.Caller{UserID: j.CreatedBy, Meta: identity.RequestMeta{RequestID: "job-" + j.ID.String()}}
	var out StartResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		out, err = s.start(ctx, tx, c, in)
		return err
	})
	return out, err
}

// start does the work of a start inside tx, with the rights checked again as the work begins.
func (s *Service) start(ctx context.Context, tx pgx.Tx, c identity.Caller, in startPayload) (StartResult, error) {
	var out StartResult
	if err := s.requireStartRights(ctx, tx, c, in.SpaceID); err != nil {
		return out, err
	}
	// Every page is held first, so what is copied is what readers have when the pair begins.
	if err := pages.HoldForStart(ctx, tx, in.SpaceID); err != nil {
		return out, err
	}
	existing, plan, err := s.startable(ctx, tx, in.SpaceID)
	if err != nil {
		return out, err
	}
	draft, err := s.spaces.CreateIn(ctx, tx, c, spaces.Input{Key: in.Key, Name: in.Name})
	if err != nil {
		return out, err
	}
	if err := spaces.LockForPairing(ctx, tx, draft.ID, existing.ID); err != nil {
		return out, err
	}
	// The new space gives the old one's writers their grants, and nobody who only reads (PUB-1302).
	if _, err := permissions.CopyWriterGrants(ctx, tx, existing.ID, draft.ID); err != nil {
		return out, err
	}
	// A page that was a twin of an ended pair, such as a start undone, is this pair's now: linking
	// the old draft space again could not bring its page pair back, as the space is not empty.
	sources := make([]uuid.UUID, len(plan.copies))
	for i, p := range plan.copies {
		sources[i] = p.ID
	}
	if _, err := tx.Exec(ctx, `DELETE FROM page_pairs pp USING space_pairs sp
		WHERE sp.id = pp.pair_id AND sp.ended_at IS NOT NULL AND pp.twin_page_id = ANY($1)`, sources); err != nil {
		return out, err
	}
	if err := s.makePair(ctx, tx, c, draft, existing, nil); err != nil {
		return out, err
	}
	p, err := activePairOf(ctx, tx, draft.ID, true)
	if err != nil {
		return out, err
	}
	w := startWork{s: s, tx: tx, c: c, pair: p, from: existing, to: draft, plan: plan,
		copies: map[uuid.UUID]uuid.UUID{}, files: map[uuid.UUID]uuid.UUID{}}
	if err := w.copyRows(ctx); err != nil {
		return out, err
	}
	if err := w.writeVersions(ctx); err != nil {
		return out, err
	}
	out = StartResult{DraftSpaceID: draft.ID, DraftKey: draft.Key, Pages: len(plan.copies)}
	_, err = audit.Record(ctx, tx, c.Event("space.draft_started", "space", draft.ID.String(), draft.ID.String(),
		map[string]any{"pair_id": p.ID, "draft_space_id": draft.ID, "published_space_id": existing.ID, "run_id": w.runID, "pages": len(plan.copies)}))
	return out, err
}

// startWork is a start under way: the ids it has made so far.
type startWork struct {
	s        *Service
	tx       pgx.Tx
	c        identity.Caller
	pair     pair
	from, to spaces.Space
	plan     startPlan
	runID    uuid.UUID
	// copies maps a page of the old space to its copy, and files a file to its copy.
	copies map[uuid.UUID]uuid.UUID
	files  map[uuid.UUID]uuid.UUID
}

// copyRows makes the copy of every page with its files, restrictions, labels and look, parents
// first; the home page of the old space goes into the draft space's own home page.
func (w *startWork) copyRows(ctx context.Context) error {
	var home pages.Page
	if w.to.HomePageID != nil {
		src, err := pages.SourceForRelease(ctx, w.tx, *w.to.HomePageID)
		if err != nil {
			return err
		}
		home = src.Page
	}
	for _, src := range w.plan.copies {
		// The text of the twins stays theirs; their task rows go, as a twin keeps none (PUB-817),
		// so the copies keep the ids of the tasks they hold.
		if err := pages.DropTwinTasks(ctx, w.tx, src.ID, w.from.ID); err != nil {
			return err
		}
		var made pages.StartCopy
		var err error
		if w.from.HomePageID != nil && src.ID == *w.from.HomePageID && home.ID != uuid.Nil {
			made, err = w.s.pages.CopyIntoForStart(ctx, w.tx, w.c, src, home)
		} else {
			made, err = w.s.pages.CopyRowForStart(ctx, w.tx, w.c, src, w.to.ID, w.parentOf(src))
		}
		if err != nil {
			return err
		}
		w.copies[src.ID] = made.Page.ID
		for from, to := range made.Files {
			w.files[from] = to
		}
	}
	return nil
}

// parentOf is the copy a page goes below: the copy of its parent, or of the nearest copied page
// above it, or the draft home page.
func (w *startWork) parentOf(src pages.Page) *uuid.UUID {
	if above := w.plan.parent[src.ID]; above != nil {
		if to, ok := w.copies[*above]; ok {
			return &to
		}
	}
	return w.to.HomePageID
}

// writeVersions writes each copy's version, with its links led to the copies, and records the
// pair of each page in step: the facts of the copy, as if it had been released (PUB-1302,
// PUB-1310).
func (w *startWork) writeVersions(ctx context.Context) error {
	w.runID = ids.New()
	counts := map[string]any{"pages": len(w.plan.copies), "posts": w.plan.posts, "databases": w.plan.databases,
		"archived": w.plan.archived, "unpublished": w.plan.unpublished, "files": len(w.files)}
	if _, err := w.tx.Exec(ctx, `INSERT INTO release_runs (id, pair_id, scope, status, actor_id, previewed_at, started_at, finished_at,
			counts, total, done) VALUES ($1, $2, $3, 'done', $4, now(), now(), now(), $5, $6, $6)`,
		w.runID, w.pair.ID, scopeStart, w.c.UserID, counts, len(w.plan.copies)); err != nil {
		return err
	}
	for i, src := range w.plan.copies {
		if err := w.writeOne(ctx, i, src); err != nil {
			return err
		}
	}
	// Compared once more by the rule every change follows: a page paired out of step would show here.
	_, err := w.tx.Exec(ctx, "SELECT page_pair_refresh(draft_page_id) FROM page_pairs WHERE pair_id = $1", w.pair.ID)
	return err
}

func (w *startWork) writeOne(ctx context.Context, ord int, src pages.Page) error {
	copyID := w.copies[src.ID]
	v, err := pages.VersionForRelease(ctx, w.tx, src.ID, src.CurrentVersion)
	if err != nil {
		return err
	}
	m := startMap{from: spaceRef{ID: w.from.ID, Key: w.from.Key}, to: spaceRef{ID: w.to.ID, Key: w.to.Key}, pages: w.copies, files: w.files}
	doc, links, err := startDocument(v.Doc, m)
	if err != nil {
		return err
	}
	if _, err := w.s.pages.WriteStartVersion(ctx, w.tx, w.c, copyID, v.Title, doc); err != nil {
		return err
	}
	made, err := pages.SourceForRelease(ctx, w.tx, copyID)
	if err != nil {
		return err
	}
	written, err := pages.VersionForRelease(ctx, w.tx, copyID, made.CurrentVersion)
	if err != nil {
		return err
	}
	if _, err := w.tx.Exec(ctx, `INSERT INTO page_pairs (draft_page_id, pair_id, twin_page_id, released_version, twin_version, released_at,
			released_by, run_id, released_title, released_parent_id, released_look, released_labels, released_hash)
		VALUES ($1, $2, $3, $4, $5, now(), $6, $7, $8, $9, page_pair_look($10, $11, $12, $13, $14), $15, $16)
		ON CONFLICT (draft_page_id) DO UPDATE SET twin_page_id = EXCLUDED.twin_page_id, released_version = EXCLUDED.released_version,
			twin_version = EXCLUDED.twin_version, released_at = EXCLUDED.released_at, released_by = EXCLUDED.released_by,
			run_id = EXCLUDED.run_id, released_title = EXCLUDED.released_title, released_parent_id = EXCLUDED.released_parent_id,
			released_look = EXCLUDED.released_look, released_labels = EXCLUDED.released_labels, released_hash = EXCLUDED.released_hash,
			text_links = '{}', changed = '{}', changed_since = NULL`,
		copyID, w.pair.ID, src.ID, made.CurrentVersion, src.CurrentVersion, w.c.UserID, w.runID, written.Title, made.ParentID,
		made.Emoji, made.FullWidth, made.HeaderAttachmentID, made.HeaderPosition, made.Language, made.Labels, written.Hash); err != nil {
		return err
	}
	// Each copied file is paired with the file it came from, which the twin keeps showing; the
	// next release of a newer version gives that file its next version (PUB-320).
	shown := document.FilesShown(v.Doc)
	if src.HeaderAttachmentID != nil {
		shown = append(shown, *src.HeaderAttachmentID)
	}
	for _, from := range shown {
		to, ok := w.files[from]
		if !ok {
			continue // a file of another space, which stays as it is
		}
		if _, err := w.tx.Exec(ctx, `INSERT INTO file_pairs (draft_attachment_id, twin_page_id, copy_attachment_id, copied_version)
			VALUES ($1, $2, $3, 1) ON CONFLICT DO NOTHING`, to, src.ID, from); err != nil {
			return err
		}
	}
	_, err = w.tx.Exec(ctx, `INSERT INTO release_run_pages (run_id, draft_page_id, draft_version, twin_page_id, twin_version, action, outcome,
			content_hash, link_map, ord, title) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		w.runID, copyID, made.CurrentVersion, src.ID, src.CurrentVersion, scopeStart, outcomeStarted, written.Hash, links, ord, written.Title)
	return err
}

// undoable says whether a pair was made by a start and nothing was written for readers since:
// ending it then may also delete the draft space (PUB-1311).
func undoable(ctx context.Context, q db.Querier, pairID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM release_runs WHERE pair_id = $1 AND scope = $2)
		AND NOT EXISTS (SELECT 1 FROM release_run_pages rp JOIN release_runs r ON r.id = rp.run_id
			WHERE r.pair_id = $1 AND r.scope <> $2 AND rp.outcome NOT IN ('skipped', 'unchanged', 'waiting'))`, pairID, scopeStart).Scan(&ok)
	return ok, err
}

// UndoStart takes back a start before its first release: the pair ends, the draft space goes to
// the trash, and the space it began from is ordinary again (PUB-1311). It needs what ending
// needs, and stays open while the part is off, as ending does (PUB-1304).
func (s *Service) UndoStart(ctx context.Context, c identity.Caller, spaceID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		p, err := activePairOf(ctx, tx, spaceID, true)
		if err != nil {
			return err
		}
		for _, side := range []uuid.UUID{p.Draft, p.Published} {
			if err := s.perms.RequireSpace(ctx, tx, c.UserID, side, permissions.SpaceAdminister); err != nil {
				return err
			}
		}
		ok, err := undoable(ctx, tx, p.ID)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.Conflict("released_since_start", "Pages were released since the draft space was started, so the start cannot be "+
				"undone. End the pair instead: both spaces stay as they are.")
		}
		if err := s.endPair(ctx, tx, c, p); err != nil {
			return err
		}
		if err := s.spaces.TrashIn(ctx, tx, c, p.Draft); err != nil {
			return err
		}
		_, err = audit.Record(ctx, tx, c.Event("space.draft_start_undone", "space", p.Published.String(), p.Published.String(),
			map[string]any{"pair_id": p.ID}))
		return err
	})
}
