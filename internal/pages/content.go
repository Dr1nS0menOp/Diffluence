// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/permissions"
)

const (
	jobCopy    = "pages.copy"
	jobContent = "pages.content"

	defaultContentRows = 50
	maxContentRows     = 200
	maxContentPages    = 1000
	// farFromTheTop is how deep a page sits before it counts as easy to lose.
	farFromTheTop = 3
)

func recordSpace(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, action string, details map[string]any) error {
	_, err := audit.Record(ctx, tx, c.Event(action, "space", spaceID.String(), spaceID.String(), details))
	return err
}

// ContentQuery are the filters of the Content screen.
type ContentQuery struct {
	OwnerID        *uuid.UUID
	OwnerDisabled  bool
	Stage          string
	Label          string
	ContributorID  *uuid.UUID
	InactiveMonths int
	WithParents    bool
	Restricted     bool
	BrokenLinks    bool
	Orphaned       bool
	PastReview     bool
	// ChangedFrom and the rest bound the last change and the creation of a page, as whole
	// days: From is the first day that counts, Until the last.
	ChangedFrom  *time.Time
	ChangedUntil *time.Time
	CreatedFrom  *time.Time
	CreatedUntil *time.Time
	Sort         string
	Limit        int
	Offset       int
}

// ContentRow is a line of the Content screen.
type ContentRow struct {
	ID            uuid.UUID  `json:"id"`
	ParentID      *uuid.UUID `json:"parent_id"`
	Title         string     `json:"title"`
	OwnerID       uuid.UUID  `json:"owner_id"`
	OwnerName     string     `json:"owner_name"`
	OwnerDisabled bool       `json:"owner_disabled"`
	StageText     string     `json:"stage_text"`
	StageColor    string     `json:"stage_color"`
	Labels        []string   `json:"labels"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Children      int        `json:"children"`
}

// The screen shows what the person may see, also for a space administrator:
// administrators do not read content by default (spec 011, D22).
const contentSQL = `SELECT p.id, p.parent_id, p.title, COALESCE(p.owner_id, p.created_by), COALESCE(o.display_name, ''),
		o.status IS DISTINCT FROM 'active', p.stage_text, p.stage_color,
		COALESCE((SELECT array_agg(l.label ORDER BY l.label) FROM page_labels l WHERE l.page_id = p.id), '{}'),
		p.created_at, p.updated_at,
		(SELECT count(*) FROM pages c WHERE c.parent_id = p.id AND c.status <> 'trashed' AND c.archived_at IS NULL),
		count(*) OVER ()
	FROM pages p
	JOIN perm_listed_pages($1) v ON v.page_id = p.id
	LEFT JOIN users o ON o.id = COALESCE(p.owner_id, p.created_by)
	WHERE p.space_id = $2 AND p.status = 'published' AND p.kind = ANY($14)
	  AND ($3::uuid IS NULL OR COALESCE(p.owner_id, p.created_by) = $3)
	  AND (NOT $4 OR o.status IS DISTINCT FROM 'active')
	  AND ($5 = '' OR p.stage_text = $5)
	  AND ($6 = '' OR EXISTS (SELECT 1 FROM page_labels l WHERE l.page_id = p.id AND l.label = $6))
	  AND ($7::uuid IS NULL OR EXISTS (SELECT 1 FROM page_versions w WHERE w.page_id = p.id AND w.author_id = $7))
	  AND ($8 = 0 OR (p.updated_at < now() - make_interval(months => $8)
	       AND ($9 OR NOT EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND c.status <> 'trashed' AND c.archived_at IS NULL))))
	  AND (NOT $10 OR EXISTS (SELECT 1 FROM page_restrictions r WHERE r.page_id = p.id))
	  AND (NOT $11 OR EXISTS (SELECT 1 FROM page_links k WHERE k.source_id = p.id
	                          AND k.target_id NOT IN (SELECT g.page_id FROM perm_visible_pages($1) g)))
	  AND (NOT $12 OR (NOT EXISTS (SELECT 1 FROM page_links k WHERE k.target_id = p.id)
	                   AND (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id) > $15))
	  AND (NOT $13 OR p.review_on <= current_date)
	  AND ($19::timestamptz IS NULL OR p.updated_at >= $19)
	  AND ($20::timestamptz IS NULL OR p.updated_at < $20::timestamptz + interval '1 day')
	  AND ($21::timestamptz IS NULL OR p.created_at >= $21)
	  AND ($22::timestamptz IS NULL OR p.created_at < $22::timestamptz + interval '1 day')
	ORDER BY
		CASE WHEN $16 = 'changed' THEN p.updated_at END DESC NULLS LAST,
		CASE WHEN $16 = 'created' THEN p.created_at END DESC NULLS LAST,
		CASE WHEN $16 = 'owner' THEN lower(o.display_name) END,
		lower(p.title), p.id
	LIMIT $17 OFFSET $18`

// Content lists the pages of a space for the people who keep it tidy.
func (s *Service) Content(ctx context.Context, c identity.Caller, spaceID uuid.UUID, q ContentQuery) ([]ContentRow, int, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 || q.Limit > maxContentRows {
		q.Limit = defaultContentRows
	}
	rows, err := s.pool.Query(ctx, contentSQL, c.UserID, spaceID, q.OwnerID, q.OwnerDisabled, q.Stage, q.Label, q.ContributorID,
		q.InactiveMonths, q.WithParents, q.Restricted, q.BrokenLinks, q.Orphaned, q.PastReview, treeKinds, farFromTheTop, q.Sort, q.Limit, max(q.Offset, 0),
		q.ChangedFrom, q.ChangedUntil, q.CreatedFrom, q.CreatedUntil)
	if err != nil {
		return nil, 0, err
	}
	total := 0
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ContentRow, error) {
		var v ContentRow
		err := r.Scan(&v.ID, &v.ParentID, &v.Title, &v.OwnerID, &v.OwnerName, &v.OwnerDisabled, &v.StageText, &v.StageColor,
			&v.Labels, &v.CreatedAt, &v.UpdatedAt, &v.Children, &total)
		return v, err
	})
	if list == nil {
		list = []ContentRow{}
	}
	return list, total, err
}

// The bulk actions of the Content screen.
const (
	ActionArchive     = "archive"
	ActionDelete      = "delete"
	ActionOwner       = "owner"
	ActionStage       = "stage"
	ActionLabelAdd    = "label_add"
	ActionLabelRemove = "label_remove"
	ActionMove        = "move"
)

// ContentAction is one thing to do to a selection of pages.
type ContentAction struct {
	Action     string      `json:"action"`
	PageIDs    []uuid.UUID `json:"page_ids"`
	OwnerID    uuid.UUID   `json:"owner_id"`
	StageText  string      `json:"stage_text"`
	StageColor string      `json:"stage_color"`
	Label      string      `json:"label"`
	ParentID   *uuid.UUID  `json:"parent_id"`
}

func (a ContentAction) validate() error {
	if len(a.PageIDs) == 0 || len(a.PageIDs) > maxContentPages {
		return apperr.Invalid("Choose between 1 and 1000 pages.")
	}
	if !slices.Contains([]string{ActionArchive, ActionDelete, ActionOwner, ActionStage, ActionLabelAdd, ActionLabelRemove, ActionMove}, a.Action) {
		return apperr.Invalid("That is not something you can do to pages here.")
	}
	return nil
}

// permissionFor is what the action needs on each page.
func (a ContentAction) permissionFor() string {
	switch a.Action {
	case ActionArchive:
		return permissions.PageArchive
	case ActionDelete:
		return permissions.ActionPageDelete
	}
	return permissions.PageEdit
}

// ContentPlan is the dry run: what an action would do, before it runs.
type ContentPlan struct {
	Pages   int           `json:"pages"`
	Allowed int           `json:"allowed"`
	Refused []BulkFailure `json:"refused"`
}

// PreviewContent says what an action would do, page by page.
func (s *Service) PreviewContent(ctx context.Context, c identity.Caller, spaceID uuid.UUID, a ContentAction) (ContentPlan, error) {
	out := ContentPlan{Pages: len(a.PageIDs), Refused: []BulkFailure{}}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
		return out, err
	}
	if err := a.validate(); err != nil {
		return out, err
	}
	for _, id := range a.PageIDs {
		d, err := s.perms.CheckPage(ctx, s.pool, c.UserID, id, a.permissionFor(), uuid.Nil)
		if err != nil || !d.Allowed {
			out.Refused = append(out.Refused, BulkFailure{ID: id, Reason: "You may not do this to this page."})
			continue
		}
		out.Allowed++
	}
	return out, nil
}

type contentJob struct {
	SpaceID uuid.UUID     `json:"space_id"`
	Action  ContentAction `json:"action"`
}

// StartContent runs an action in the background, one transaction per page.
func (s *Service) StartContent(ctx context.Context, c identity.Caller, spaceID uuid.UUID, a ContentAction) (uuid.UUID, error) {
	if err := a.validate(); err != nil {
		return uuid.Nil, err
	}
	var job uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceAdminister); err != nil {
			return err
		}
		var err error
		if job, err = s.jobs.Enqueue(ctx, tx, jobContent, contentJob{SpaceID: spaceID, Action: a}, c.UserID); err != nil {
			return err
		}
		return recordSpace(ctx, tx, c, spaceID, "space.content_action", map[string]any{"action": a.Action, "pages": len(a.PageIDs)})
	})
	return job, err
}

// applyContent runs one action on one page, through the same call a person uses,
// so the rights are checked page by page. A selection is never a permission.
func (s *Service) applyContent(ctx context.Context, c identity.Caller, id uuid.UUID, a ContentAction) error {
	var err error
	switch a.Action {
	case ActionArchive:
		err = s.Archive(ctx, c, id, false)
	case ActionDelete:
		err = s.Delete(ctx, c, id)
	case ActionOwner:
		_, err = s.SetOwner(ctx, c, id, a.OwnerID)
	case ActionStage:
		_, err = s.SetStage(ctx, c, id, a.StageText, a.StageColor)
	case ActionLabelAdd:
		_, err = s.AddLabel(ctx, c, id, a.Label)
	case ActionLabelRemove:
		_, err = s.RemoveLabel(ctx, c, id, a.Label)
	case ActionMove:
		_, err = s.Move(ctx, c, id, MoveInput{ParentID: a.ParentID})
	default:
		err = fmt.Errorf("pages: unknown action %q", a.Action)
	}
	return err
}

// ContentResult is the report of a bulk action.
type ContentResult struct {
	Done   int           `json:"done"`
	Failed []BulkFailure `json:"failed"`
}

func (s *Service) runContentJob(ctx context.Context, j jobs.Job) (any, error) {
	var in contentJob
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	c := identity.Caller{UserID: j.CreatedBy}
	out := ContentResult{Failed: []BulkFailure{}}
	for _, id := range in.Action.PageIDs {
		if err := s.applyContent(ctx, c, id, in.Action); err != nil {
			out.Failed = append(out.Failed, BulkFailure{ID: id, Reason: reasonOf(err)})
			continue
		}
		out.Done++
	}
	return out, nil
}

type copyJob struct {
	PageID uuid.UUID `json:"page_id"`
	Input  CopyInput `json:"input"`
}

// BranchSize counts the pages a copy with children would make.
func (s *Service) BranchSize(ctx context.Context, c identity.Caller, id uuid.UUID) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM page_tree t
		JOIN perm_listed_pages($1) v ON v.page_id = t.descendant_id WHERE t.ancestor_id = $2`, c.UserID, id).Scan(&n)
	return n, err
}

// IsLargeCopy says whether a copy is better left to the background (PAG-050).
func IsLargeCopy(pages int) bool { return pages > largeCopyPages }

// StartCopy runs a large copy in the background, with the rights checked again there.
func (s *Service) StartCopy(ctx context.Context, c identity.Caller, id uuid.UUID, in CopyInput) (uuid.UUID, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return uuid.Nil, err
	}
	var job uuid.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		job, err = s.jobs.Enqueue(ctx, tx, jobCopy, copyJob{PageID: id, Input: in}, c.UserID)
		return err
	})
	return job, err
}

// CopyResult is the report of a copy that ran in the background.
type CopyResult struct {
	PageID uuid.UUID     `json:"page_id"`
	Done   int           `json:"done"`
	Failed []BulkFailure `json:"failed"`
}

// runCopyJob copies the page itself as a person would, and then the rest one page at
// a time. The audit log holds a lock until a transaction commits, so a large branch
// in one transaction would stop everyone else for as long as it runs.
func (s *Service) runCopyJob(ctx context.Context, j jobs.Job) (any, error) {
	var in copyJob
	if err := json.Unmarshal(j.Payload, &in); err != nil {
		return nil, err
	}
	c := identity.Caller{UserID: j.CreatedBy}
	top := in.Input
	top.IncludeChildren = false
	p, err := s.Copy(ctx, c, in.PageID, top)
	if err != nil {
		return nil, err
	}
	out := CopyResult{PageID: p.ID, Done: 1, Failed: []BulkFailure{}}
	plan := copyPlan{CopyInput: in.Input, spaceID: p.SpaceID}
	if err := s.copyBelow(ctx, c, in.PageID, p.ID, plan, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// copyStep is a page whose children still have to go below its copy.
type copyStep struct {
	from uuid.UUID
	to   uuid.UUID
}

// copyBelow works through a branch from the top down. A page that fails is reported,
// and what is below it stays behind with it.
func (s *Service) copyBelow(ctx context.Context, c identity.Caller, from, to uuid.UUID, plan copyPlan, out *CopyResult) error {
	queue := []copyStep{{from: from, to: to}}
	for len(queue) > 0 {
		step := queue[0]
		queue = queue[1:]
		children, err := childrenToCopy(ctx, s.pool, c.UserID, step.from)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := ctx.Err(); err != nil {
				return err
			}
			made, err := s.copyOneStep(ctx, c, child, step.to, plan)
			if err != nil {
				out.Failed = append(out.Failed, BulkFailure{ID: child, Reason: reasonOf(err)})
				continue
			}
			out.Done++
			queue = append(queue, copyStep{from: child, to: made.ID})
		}
	}
	return nil
}

// copyOneStep is one page in its own transaction, under the rights of that moment.
func (s *Service) copyOneStep(ctx context.Context, c identity.Caller, id, parent uuid.UUID, plan copyPlan) (Page, error) {
	var made Page
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
			return err
		}
		if err := lockSpaces(ctx, tx, p.SpaceID, plan.spaceID); err != nil {
			return err
		}
		// copyOne asks for the right to make this kind of item in the target space.
		made, err = s.copyOne(ctx, tx, c, p, &parent, plan.Titles.apply(p.Title), plan)
		return err
	})
	return made, err
}
