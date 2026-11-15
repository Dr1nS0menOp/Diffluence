// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/document"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/workflow"
)

// The move from today's approvals (spec 064 wave DC5, blueprint 09.12). A space that requires
// approval today is offered, never forced, a generated workflow that means the same: a draft,
// a review by every fixed approver, and approved, with authors allowed to count because
// today's rule does not ask for independence (DOC-1101). Accepting it assigns the workflow
// (DOC-1102); the old rule is switched off and kept on record (DOC-1105); and until a person
// first acts under the workflow the space can go back (DOC-1107).

// generatedWorkflowName is the name the generated workflow is saved under; the screens
// translate it.
const generatedWorkflowName = "Approval as before"

// ApprovalRuleOffer is what the generated workflow would be and do.
type ApprovalRuleOffer struct {
	Definition workflow.Definition `json:"definition"`
	Landing    Landing             `json:"landing"`
	// NoApprovers warns that the space names no fixed approver, so a round cannot start until
	// someone names reviewers.
	NoApprovers bool `json:"no_approvers"`
}

// generatedDefinition is today's rule as a workflow.
func generatedDefinition(approvers []uuid.UUID) workflow.Definition {
	reviewers := make([]workflow.Reviewer, 0, len(approvers))
	for _, id := range approvers {
		reviewers = append(reviewers, workflow.Reviewer{Type: workflow.ReviewerUser, ID: id})
	}
	return workflow.Definition{
		States: []workflow.State{
			{ID: "draft", Name: "Draft", Kind: workflow.KindDraft, Start: true},
			{ID: "review", Name: "In review", Kind: workflow.KindInReview,
				Review: &workflow.Review{Reviewers: reviewers, Rule: workflow.RuleAll, AuthorsMayReview: true, AuthorsMayAdd: true}},
			{ID: "approved", Name: "Approved", Kind: workflow.KindApproved},
			{ID: "changes", Name: "Changes asked", Kind: workflow.KindRejected},
		},
		Transitions: []workflow.Transition{
			{ID: "ask", Name: "Ask for approval", From: "draft", To: "review"},
			{ID: "approve", Name: "Approve", From: "review", To: "approved", Outcome: workflow.OutcomeApproved},
			{ID: "changes", Name: "Ask for changes", From: "review", To: "changes", Outcome: workflow.OutcomeChanges},
			{ID: "rework", Name: "Work on it again", From: "changes", To: "draft"},
		},
	}
}

// ruleToMove reads today's rule of a space and refuses a space that has none or already has a
// space workflow (DOC-1104).
func ruleToMove(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) ([]uuid.UUID, error) {
	var required, hasWorkflow bool
	if err := tx.QueryRow(ctx, `SELECT approval_required, EXISTS (SELECT 1 FROM space_workflows WHERE space_id = $1) FROM spaces WHERE id = $1`,
		spaceID).Scan(&required, &hasWorkflow); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound()
		}
		return nil, err
	}
	switch {
	case !required:
		return nil, apperr.Conflict("no_approval_rule", "This space does not require approval, so there is nothing to move.")
	case hasWorkflow:
		return nil, apperr.Conflict("has_space_workflow", "This space follows a space workflow already.")
	}
	rows, err := tx.Query(ctx, "SELECT user_id FROM space_approvers WHERE space_id = $1 ORDER BY user_id", spaceID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// moveRule makes the generated workflow and assigns it, in tx.
func (s *Service) moveRule(ctx context.Context, tx pgx.Tx, c identity.Caller, spaceID uuid.UUID, preview bool) (ApprovalRuleOffer, error) {
	var out ApprovalRuleOffer
	approvers, err := ruleToMove(ctx, tx, spaceID)
	if err != nil {
		return out, err
	}
	out.Definition, out.NoApprovers = generatedDefinition(approvers), len(approvers) == 0
	if err := workflow.Validate(out.Definition); err != nil {
		return out, err
	}
	raw, err := json.Marshal(out.Definition)
	if err != nil {
		return out, err
	}
	id := ids.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workflows (id, space_id, name, description, created_by) VALUES ($1, $2, $3, '', $4)`,
		id, spaceID, generatedWorkflowName, c.UserID); err != nil {
		return out, err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO workflow_versions (workflow_id, version, definition, saved_by) VALUES ($1, 1, $2, $3)", id, raw, c.UserID); err != nil {
		return out, err
	}
	out.Landing, err = s.setSpaceWorkflowIn(ctx, tx, c, spaceID, SpaceWorkflowInput{WorkflowID: &id, Kinds: governableKinds, Preview: preview,
		fromApprovalRule: true})
	return out, err
}

// OfferApprovalRuleMove shows what the generated workflow is and where every page would land,
// and changes nothing (DOC-1101, DOC-1102).
func (s *Service) OfferApprovalRuleMove(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (ApprovalRuleOffer, error) {
	var out ApprovalRuleOffer
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		var err error
		if out, err = s.moveRule(ctx, tx, c, spaceID, true); err != nil {
			return err
		}
		return errPreview
	})
	if errors.Is(err, errPreview) {
		err = nil
	}
	return out, err
}

// AcceptApprovalRuleMove moves the space to the generated workflow (DOC-1102).
func (s *Service) AcceptApprovalRuleMove(ctx context.Context, c identity.Caller, spaceID uuid.UUID) (ApprovalRuleOffer, error) {
	var out ApprovalRuleOffer
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		var err error
		if out, err = s.moveRule(ctx, tx, c, spaceID, false); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "space.approval_rule_moved", "space", spaceID, spaceID,
			map[string]any{"pages": out.Landing.Pages, "approved": out.Landing.Approved, "kept": out.Landing.Kept, "start": out.Landing.Start})
	})
	return out, err
}

// UndoApprovalRuleMove brings today's rule back, while no person has acted under the generated
// workflow; after that, removing it follows DOC-141 (DOC-1107).
func (s *Service) UndoApprovalRuleMove(ctx context.Context, c identity.Caller, spaceID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		var workflowID uuid.UUID
		err := tx.QueryRow(ctx, "SELECT workflow_id FROM space_workflows WHERE space_id = $1 AND from_approval_rule FOR UPDATE", spaceID).Scan(&workflowID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.Conflict("not_moved", "This space did not move from its approval rule.")
		}
		if err != nil {
			return err
		}
		var acted bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workflow_answers a JOIN workflow_rounds r ON r.id = a.round_id
				WHERE r.workflow_id = $1 AND a.user_id IS NOT NULL)
			OR EXISTS (SELECT 1 FROM workflow_reviewers wr JOIN workflow_rounds r ON r.id = wr.round_id WHERE r.workflow_id = $1)`,
			workflowID).Scan(&acted); err != nil {
			return err
		}
		if acted {
			return apperr.Conflict("round_ran", "A review ran under the workflow already, so the move cannot be undone. Remove the workflow instead.")
		}
		if _, err := s.setSpaceWorkflowIn(ctx, tx, c, spaceID, SpaceWorkflowInput{}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE workflows SET archived_at = now() WHERE id = $1", workflowID); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "space.approval_rule_restored", "space", spaceID, spaceID, map[string]any{"workflow_id": workflowID})
	})
}

// showCarriedVersion makes the version that today's rounds approved the one readers see, and
// keeps the newer one for its authors, as a working version waiting for review (DOC-1102).
func (s *Service) showCarriedVersion(ctx context.Context, tx pgx.Tx, c identity.Caller, id uuid.UUID, approved, head int) error {
	p, err := load(ctx, tx, id)
	if err != nil {
		return err
	}
	var title string
	var raw []byte
	var author uuid.UUID
	if err := tx.QueryRow(ctx, "SELECT title, document, author_id FROM page_versions WHERE page_id = $1 AND number = $2", id, approved).
		Scan(&title, &raw, &author); err != nil {
		return err
	}
	doc, err := document.Parse(raw)
	if err != nil {
		return err
	}
	if err := attachments.Pin(ctx, tx, id, approved); err != nil {
		return err
	}
	if _, err := s.makeCurrent(ctx, tx, c, p, current{number: approved, title: title, doc: doc, author: author, approval: true}); err != nil {
		return err
	}
	var headTitle string
	var headRaw []byte
	if err := tx.QueryRow(ctx, "SELECT title, document FROM page_versions WHERE page_id = $1 AND number = $2", id, head).Scan(&headTitle, &headRaw); err != nil {
		return err
	}
	headDoc, err := document.Parse(headRaw)
	if err != nil {
		return err
	}
	return search.IndexWorking(ctx, tx, id, headTitle, document.Text(headDoc))
}
