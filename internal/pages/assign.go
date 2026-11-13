// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// Which workflow governs which page (spec 064, 09.5). The space workflow comes first, for
// the kinds it names; then a page workflow on the page itself; then the nearest branch
// workflow above it. A page has at most one (DOC-411). Folders, links and templates hold
// nothing to approve and are never governed (DOC-400).

// governableKinds are the kinds a workflow can govern.
var governableKinds = []string{KindPage, KindPost, KindDatabase}

// Why pages are settled again.
const (
	settleCreated  = "created"
	settleMoved    = "moved"
	settleKind     = "kind_changed"
	settleAssigned = causeAssigned
)

// governing is what governs one page.
type governing struct {
	workflowID uuid.UUID
	by         string
	from       *uuid.UUID
}

// governingOf works out, in one query, the workflow that governs each of the pages.
// Pages that none governs are absent.
func governingOf(ctx context.Context, tx pgx.Tx, pageIDs []uuid.UUID) (map[uuid.UUID]governing, error) {
	rows, err := tx.Query(ctx, `SELECT p.id,
			COALESCE(sw.workflow_id, pw.workflow_id, bw.workflow_id),
			CASE WHEN sw.workflow_id IS NOT NULL THEN 'space' WHEN pw.workflow_id IS NOT NULL THEN 'page' ELSE 'branch' END,
			CASE WHEN sw.workflow_id IS NOT NULL THEN NULL WHEN pw.workflow_id IS NOT NULL THEN p.id ELSE bw.page_id END
		FROM pages p
		LEFT JOIN space_workflows sw ON sw.space_id = p.space_id AND p.kind = ANY(sw.kinds)
		LEFT JOIN page_workflows pw ON pw.page_id = p.id
		LEFT JOIN LATERAL (SELECT w.page_id, w.workflow_id FROM page_tree t JOIN page_workflows w ON w.page_id = t.ancestor_id AND w.branch
			WHERE t.descendant_id = p.id AND t.depth > 0 ORDER BY t.depth LIMIT 1) bw ON true
		WHERE p.id = ANY($1) AND p.kind = ANY($2) AND p.status <> 'trashed'
		  AND COALESCE(sw.workflow_id, pw.workflow_id, bw.workflow_id) IS NOT NULL`, pageIDs, governableKinds)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]governing{}
	for rows.Next() {
		var id uuid.UUID
		var g governing
		if err := rows.Scan(&id, &g.workflowID, &g.by, &g.from); err != nil {
			return nil, err
		}
		out[id] = g
	}
	return out, rows.Err()
}

// Landing is where a page lands when control changes, for the preview (DOC-140).
type Landing struct {
	// Pages is how many pages the change touches.
	Pages int `json:"pages"`
	// Approved land in an approved state: readers keep seeing them.
	Approved int `json:"approved"`
	// Start land in the start state without an approved version: readers stop seeing them
	// until they are approved (DOC-401).
	Start int `json:"start"`
	// Kept keep an approved version that readers see, while a change waits for review.
	Kept int `json:"kept"`
	// Leaving become ordinary pages again (DOC-141).
	Leaving int `json:"leaving"`
	// Revealed are those of Leaving that readers did not see, never approved or withdrawn:
	// as ordinary pages they show their current text, so the preview names them.
	Revealed int `json:"revealed"`
	// Unchanged stay as they are.
	Unchanged int `json:"unchanged"`
	// ReplacedWorkflows are page workflows the change takes away (DOC-411, DOC-420).
	ReplacedWorkflows []PageRef `json:"replaced_workflows"`
}

// settle makes the control of pages follow what governs them now, inside the caller's
// transaction. It counts where each page lands, so the preview and the change are one code.
func (s *Service) settle(ctx context.Context, tx pgx.Tx, c identity.Caller, pageIDs []uuid.UUID, cause string) (Landing, error) {
	var land Landing
	if len(pageIDs) == 0 {
		return land, nil
	}
	// The pages rows first, as a publish and a transition lock them: the same order.
	if _, err := tx.Exec(ctx, "SELECT 1 FROM pages WHERE id = ANY($1) ORDER BY id FOR NO KEY UPDATE", pageIDs); err != nil {
		return land, err
	}
	gov, err := governingOf(ctx, tx, pageIDs)
	if err != nil {
		return land, err
	}
	controls, err := lockControls(ctx, tx, pageIDs)
	if err != nil {
		return land, err
	}
	for _, id := range pageIDs {
		g, governed := gov[id]
		ctl, controlled := controls[id]
		switch {
		case !governed && !controlled:
			continue
		case !governed:
			land.Pages++
			land.Leaving++
			if !ctl.approved() || ctl.stateKind == workflow.KindObsolete {
				land.Revealed++
			}
			if err := s.removeControl(ctx, tx, c, id, cause); err != nil {
				return land, err
			}
		// The same workflow under the same kind of rule changes nothing. A space workflow
		// taking over from a page workflow ends its rounds, even when it is the same one (DOC-420).
		case controlled && ctl.workflowID == g.workflowID && (g.by != governedBySpace || ctl.governedBy == governedBySpace):
			land.Pages++
			land.Unchanged++
			if _, err := tx.Exec(ctx, "UPDATE page_control SET governed_by = $2, governed_from = $3 WHERE page_id = $1", id, g.by, g.from); err != nil {
				return land, err
			}
		default:
			land.Pages++
			where, err := s.landPage(ctx, tx, c, id, g, ctl, controlled, cause)
			if err != nil {
				return land, err
			}
			switch where {
			case workflow.KindApproved:
				land.Approved++
			case workflow.KindDraft:
				land.Start++
			default:
				land.Kept++
			}
		}
	}
	return land, nil
}

func lockControls(ctx context.Context, tx pgx.Tx, pageIDs []uuid.UUID) (map[uuid.UUID]control, error) {
	rows, err := tx.Query(ctx, "SELECT page_id, "+controlColumns+" FROM page_control WHERE page_id = ANY($1) ORDER BY page_id FOR UPDATE", pageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]control{}
	for rows.Next() {
		var id uuid.UUID
		var c control
		if err := rows.Scan(&id, &c.workflowID, &c.workflowVersion, &c.stateID, &c.stateKind, &c.approvedVersion, &c.approvedStateID,
			&c.headVersion, &c.governedBy, &c.governedFrom); err != nil {
			return nil, err
		}
		out[id] = c
	}
	return out, rows.Err()
}

// landPage puts a page under a workflow, or under another one. It answers the kind of where
// it landed: approved, draft for the start state with nothing approved, or in_review for
// the start state with an approved version readers keep seeing.
func (s *Service) landPage(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, g governing, ctl control, controlled bool, cause string) (workflow.Kind, error) {
	var version, current int
	var published, approvedToday bool
	if err := tx.QueryRow(ctx, `SELECT w.current_version, p.current_version, p.status = 'published',
			EXISTS (SELECT 1 FROM approval_rounds r WHERE r.page_id = p.id AND r.status = 'approved' AND r.approved_version = p.current_version)
		FROM workflows w, pages p WHERE w.id = $1 AND p.id = $2`, g.workflowID, id).Scan(&version, &current, &published, &approvedToday); err != nil {
		return "", err
	}
	def, err := definitionOf(ctx, tx, g.workflowID, version)
	if err != nil {
		return "", err
	}
	next := control{workflowID: g.workflowID, workflowVersion: version, headVersion: current, governedBy: g.by, governedFrom: g.from}
	switch {
	case controlled && ctl.approved():
		// It stays approved as it was; a change waiting for review starts again.
		next.approvedVersion, next.headVersion = ctl.approvedVersion, ctl.headVersion
		next.approvedStateID = def.FirstApproved().ID
	case cause == settleAssigned && published && approvedToday:
		// Approved under today's approval rounds (DOC-401, 09.12).
		next.approvedVersion = &current
		next.approvedStateID = def.FirstApproved().ID
	}
	state := def.StartState()
	if next.approved() && next.headVersion == *next.approvedVersion {
		state = def.FirstApproved()
	}
	next.stateID, next.stateKind = state.ID, state.Kind
	from := ""
	if controlled {
		from = ctl.stateID
	}
	reason := cause
	if g.by == governedBySpace && controlled && ctl.governedBy != governedBySpace {
		reason = causeSpaceWorkflow
	}
	if err := endRound(ctx, tx, id, roundStatusSuperseded, reason); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO page_control (page_id, workflow_id, workflow_version, state_id, state_kind, approved_version,
			approved_state_id, approved_at, head_version, governed_by, governed_from)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), CASE WHEN $6::int IS NOT NULL THEN now() END, $8, $9, $10)
		ON CONFLICT (page_id) DO UPDATE SET workflow_id = EXCLUDED.workflow_id, workflow_version = EXCLUDED.workflow_version,
			state_id = EXCLUDED.state_id, state_kind = EXCLUDED.state_kind, approved_state_id = EXCLUDED.approved_state_id,
			governed_by = EXCLUDED.governed_by, governed_from = EXCLUDED.governed_from, updated_at = now()`,
		id, g.workflowID, version, state.ID, state.Kind, next.approvedVersion, next.approvedStateID, next.headVersion, g.by, g.from); err != nil {
		return "", err
	}
	var round *uuid.UUID
	if state.Kind != workflow.KindApproved {
		r, err := openRound(ctx, tx, id, next)
		if err != nil {
			return "", err
		}
		round = &r
	}
	if err := systemAnswer(ctx, tx, id, round, from, state.ID, reason, next.headVersion); err != nil {
		return "", err
	}
	p, err := load(ctx, tx, id)
	if err != nil {
		return "", err
	}
	details := map[string]any{"workflow_id": g.workflowID, "governed_by": g.by, "state": state.ID, "cause": reason}
	// The event lets public sites take a page down at once when readers stop seeing it.
	if err := record(ctx, tx, c, "page.controlled", p, details); err != nil {
		return "", err
	}
	switch {
	case state.Kind == workflow.KindApproved:
		return workflow.KindApproved, nil
	case next.approved():
		return workflow.KindInReview, nil
	}
	return workflow.KindDraft, nil
}

// removeControl makes a page an ordinary page at its approved version (DOC-141). A page
// that was never approved shows its newest text to readers again (decided for DC1).
func (s *Service) removeControl(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, cause string) error {
	if err := endRound(ctx, tx, id, roundStatusEnded, causeControlRemoved); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM page_control WHERE page_id = $1", id); err != nil {
		return err
	}
	p, err := load(ctx, tx, id)
	if err != nil {
		return err
	}
	return record(ctx, tx, c, "page.control_removed", p, map[string]any{"cause": cause})
}

// settleNew puts a new page under the workflow that governs where it was made (DOC-400,
// DOC-410): it starts in the start state, like every change.
func (s *Service) settleNew(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID) error {
	// A new page has no control yet, so where nothing governs, one query is all it costs.
	gov, err := governingOf(ctx, tx, []uuid.UUID{id})
	if err != nil || len(gov) == 0 {
		return err
	}
	_, err = s.settle(ctx, tx, c, []uuid.UUID{id}, settleCreated)
	return err
}

// settleBranch follows a move or a change of kind: the page and everything below it.
func (s *Service) settleBranch(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, cause string) error {
	tree, err := subtree(ctx, tx, id)
	if err != nil {
		return err
	}
	_, err = s.settle(ctx, tx, c, tree, cause)
	return err
}

// SpaceWorkflowInput turns the space workflow on, changes it, or turns it off.
type SpaceWorkflowInput struct {
	// WorkflowID nil turns the space workflow off (DOC-421).
	WorkflowID *uuid.UUID
	Kinds      []string
	// Preview counts what would change and changes nothing (DOC-140).
	Preview bool
}

// errPreview rolls a preview back after it has counted.
var errPreview = errors.New("pages: preview only")

// SetSpaceWorkflow sets the workflow of a whole space (DOC-400, DOC-401). It overrides
// page workflows, which it removes for good (DOC-420, DOC-421). The preview runs the same
// change and rolls it back, so its counts are the change's own.
func (s *Service) SetSpaceWorkflow(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in SpaceWorkflowInput) (Landing, error) {
	var land Landing
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		kinds, err := cleanKinds(in.Kinds, in.WorkflowID != nil)
		if err != nil {
			return err
		}
		if err := lockTree(ctx, tx, spaceID); err != nil {
			return err
		}
		if in.WorkflowID != nil {
			if err := workflowOfSpace(ctx, tx, *in.WorkflowID, spaceID); err != nil {
				return err
			}
		}
		replaced, err := pageWorkflowRefs(ctx, tx, `SELECT pw.page_id FROM page_workflows pw JOIN pages p ON p.id = pw.page_id WHERE p.space_id = $1`, spaceID)
		if err != nil {
			return err
		}
		if in.WorkflowID == nil {
			replaced = []PageRef{}
			if _, err := tx.Exec(ctx, "DELETE FROM space_workflows WHERE space_id = $1", spaceID); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, "DELETE FROM page_workflows pw USING pages p WHERE p.id = pw.page_id AND p.space_id = $1", spaceID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO space_workflows (space_id, workflow_id, kinds, set_by) VALUES ($1, $2, $3, $4)
				ON CONFLICT (space_id) DO UPDATE SET workflow_id = EXCLUDED.workflow_id, kinds = EXCLUDED.kinds, set_by = EXCLUDED.set_by, set_at = now()`,
				spaceID, *in.WorkflowID, kinds, c.UserID); err != nil {
				return err
			}
		}
		rows, err := tx.Query(ctx, "SELECT id FROM pages WHERE space_id = $1 AND kind = ANY($2) AND status <> 'trashed' ORDER BY id", spaceID, governableKinds)
		if err != nil {
			return err
		}
		all, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		if land, err = s.settle(ctx, tx, c, all, settleAssigned); err != nil {
			return err
		}
		land.ReplacedWorkflows = replaced
		if in.Preview {
			return errPreview
		}
		return recordAudit(ctx, tx, c, "space.workflow_set", "space", spaceID, spaceID, map[string]any{
			"workflow_id": in.WorkflowID, "kinds": kinds, "pages": land.Pages, "start": land.Start, "replaced": len(replaced)})
	})
	if errors.Is(err, errPreview) {
		err = nil
	}
	return land, err
}

// PageWorkflowInput sets a workflow on a page or a branch, or takes it off.
type PageWorkflowInput struct {
	// WorkflowID nil takes the page workflow off.
	WorkflowID *uuid.UUID
	// Branch sets it on the page and every page below it (DOC-410).
	Branch bool
	// ReplaceChildren allows taking away the page workflows below; without it a branch
	// over them changes nothing (DOC-411).
	ReplaceChildren bool
	Preview         bool
}

// SetPageWorkflow sets or removes the workflow of a page or a branch (DOC-410, DOC-411).
func (s *Service) SetPageWorkflow(ctx context.Context, c identity.Caller, id uuid.UUID, in PageWorkflowInput) (Landing, error) {
	var land Landing
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequirePage(ctx, tx, c.UserID, id, permissions.PageWorkflow, uuid.Nil); err != nil {
			return err
		}
		p, err := load(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := lockTree(ctx, tx, p.SpaceID); err != nil {
			return err
		}
		var spaceWorkflow, aboveBranch bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM space_workflows WHERE space_id = $1),
			EXISTS (SELECT 1 FROM page_tree t JOIN page_workflows w ON w.page_id = t.ancestor_id AND w.branch
				WHERE t.descendant_id = $2 AND t.depth > 0)`, p.SpaceID, id).Scan(&spaceWorkflow, &aboveBranch); err != nil {
			return err
		}
		if spaceWorkflow {
			return apperr.Conflict("space_workflow", "The whole space follows a workflow, so a page cannot have its own.")
		}
		tree := []uuid.UUID{id}
		replaced := []PageRef{}
		if in.WorkflowID != nil {
			if aboveBranch {
				return apperr.Conflict("branch_workflow", "This page follows the workflow of the branch above it.")
			}
			if !in.Branch && !slices.Contains(governableKinds, p.Kind) {
				return apperr.Invalid("Only pages, blog posts and databases follow a workflow; a branch can start at a folder.")
			}
			if err := workflowOfSpace(ctx, tx, *in.WorkflowID, p.SpaceID); err != nil {
				return err
			}
		}
		var wasBranch bool
		if err := tx.QueryRow(ctx, "SELECT COALESCE((SELECT branch FROM page_workflows WHERE page_id = $1), false)", id).Scan(&wasBranch); err != nil {
			return err
		}
		if in.Branch || wasBranch {
			if tree, err = subtree(ctx, tx, id); err != nil {
				return err
			}
		}
		if in.WorkflowID != nil && in.Branch {
			if replaced, err = pageWorkflowRefs(ctx, tx, `SELECT pw.page_id FROM page_workflows pw JOIN page_tree t ON t.descendant_id = pw.page_id
				WHERE t.ancestor_id = $1 AND t.depth > 0`, id); err != nil {
				return err
			}
			if len(replaced) > 0 && !in.ReplaceChildren && !in.Preview {
				return apperr.Conflict("children_have_workflows", "Pages below this one have a workflow of their own. Choose to replace them first.")
			}
			if _, err := tx.Exec(ctx, `DELETE FROM page_workflows pw USING page_tree t
				WHERE t.descendant_id = pw.page_id AND t.ancestor_id = $1 AND t.depth > 0`, id); err != nil {
				return err
			}
		}
		if in.WorkflowID == nil {
			if _, err := tx.Exec(ctx, "DELETE FROM page_workflows WHERE page_id = $1", id); err != nil {
				return err
			}
		} else if _, err := tx.Exec(ctx, `INSERT INTO page_workflows (page_id, workflow_id, branch, set_by) VALUES ($1, $2, $3, $4)
			ON CONFLICT (page_id) DO UPDATE SET workflow_id = EXCLUDED.workflow_id, branch = EXCLUDED.branch, set_by = EXCLUDED.set_by, set_at = now()`,
			id, *in.WorkflowID, in.Branch, c.UserID); err != nil {
			return err
		}
		if land, err = s.settle(ctx, tx, c, tree, settleAssigned); err != nil {
			return err
		}
		land.ReplacedWorkflows = replaced
		if in.Preview {
			return errPreview
		}
		return recordAudit(ctx, tx, c, "page.workflow_set", "page", id, p.SpaceID, map[string]any{
			"workflow_id": in.WorkflowID, "branch": in.Branch, "pages": land.Pages, "replaced": len(replaced)})
	})
	if errors.Is(err, errPreview) {
		err = nil
	}
	return land, err
}

// workflowOfSpace refuses a workflow of another space (DOC-430) and an archived one.
func workflowOfSpace(ctx context.Context, tx pgx.Tx, workflowID, spaceID uuid.UUID) error {
	var space uuid.UUID
	var archived bool
	err := tx.QueryRow(ctx, "SELECT space_id, archived_at IS NOT NULL FROM workflows WHERE id = $1", workflowID).Scan(&space, &archived)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && space != spaceID) {
		return apperr.Invalid("Choose a workflow of this space.")
	}
	if err != nil {
		return err
	}
	if archived {
		return apperr.Conflict("workflow_archived", "This workflow is archived.")
	}
	return nil
}

func cleanKinds(kinds []string, on bool) ([]string, error) {
	if !on {
		return []string{}, nil
	}
	if len(kinds) == 0 {
		return nil, apperr.Invalid("Choose which kinds of item the space workflow governs.")
	}
	out := []string{}
	for _, k := range kinds {
		if !slices.Contains(governableKinds, k) {
			return nil, apperr.Invalid("A workflow governs pages, blog posts and databases.")
		}
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out, nil
}

// pageWorkflowRefs names the pages a query finds, for the preview. The titles are of pages
// in a space the person runs, or below a page they may set workflows on.
func pageWorkflowRefs(ctx context.Context, tx pgx.Tx, sql string, arg uuid.UUID) ([]PageRef, error) {
	rows, err := tx.Query(ctx, `SELECT p.id, p.title FROM pages p WHERE p.id IN (`+sql+`) ORDER BY lower(p.title), p.id LIMIT 200`, arg)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PageRef, error) {
		var ref PageRef
		err := r.Scan(&ref.ID, &ref.Title)
		return ref, err
	})
	if out == nil {
		out = []PageRef{}
	}
	return out, err
}
