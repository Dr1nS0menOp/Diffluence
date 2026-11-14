// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// Instance templates of workflows (DOC-130). An instance administrator keeps them; a space
// copies one and owns the copy, which records the template and version it came from. A
// later change of the template never moves a copy: a space takes it by choice (DOC-1405).

// WorkflowTemplate is an instance template at its newest version.
type WorkflowTemplate struct {
	ID          uuid.UUID           `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Version     int                 `json:"version"`
	Definition  workflow.Definition `json:"definition"`
	SavedAt     time.Time           `json:"saved_at"`
	// Names gives the people, groups and roles it names, for its administrators only.
	Names map[string]string `json:"names,omitempty"`
	// Copies lists the spaces that use it, for its administrators only.
	Copies []TemplateCopy `json:"copies,omitempty"`
}

// TemplateCopy is a workflow of a space made from a template, and the version it follows.
type TemplateCopy struct {
	WorkflowID   uuid.UUID `json:"workflow_id"`
	WorkflowName string    `json:"workflow_name"`
	SpaceID      uuid.UUID `json:"space_id"`
	SpaceName    string    `json:"space_name"`
	Version      int       `json:"version"`
}

const (
	maxWorkflowTemplates = 100
	templateColumns      = "id, name, description, version, definition, saved_at"
)

func scanTemplate(r pgx.Row) (WorkflowTemplate, error) {
	var t WorkflowTemplate
	var raw []byte
	if err := r.Scan(&t.ID, &t.Name, &t.Description, &t.Version, &raw, &t.SavedAt); err != nil {
		return t, err
	}
	if err := json.Unmarshal(raw, &t.Definition); err != nil {
		return t, fmt.Errorf("pages: read workflow template %s: %v", t.ID, err)
	}
	return t, nil
}

// requireInstanceAdministrator checks in the module, so no route can forget it.
func requireInstanceAdministrator(ctx context.Context, q db.Querier, c identity.Caller) error {
	var ok bool
	if err := q.QueryRow(ctx, "SELECT perm_has_global($1, $2)", c.UserID, identity.PermAdministrator).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return apperr.Forbidden()
	}
	return nil
}

// WorkflowStarts lists what a space can start a workflow from: the instance templates and
// the two examples Diffluence ships (DOC-131). Any person of the instance may see them.
func (s *Service) WorkflowStarts(ctx context.Context) ([]WorkflowTemplate, []workflow.Example, error) {
	rows, err := s.pool.Query(ctx, "SELECT "+templateColumns+" FROM workflow_templates WHERE archived_at IS NULL ORDER BY lower(name) LIMIT $1",
		maxWorkflowTemplates)
	if err != nil {
		return nil, nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (WorkflowTemplate, error) { return scanTemplate(r) })
	if list == nil {
		list = []WorkflowTemplate{}
	}
	return list, workflow.Examples(), err
}

// AdminWorkflowTemplates lists the templates with the spaces that use each, so an
// administrator sees which follow an older version (DOC-130).
func (s *Service) AdminWorkflowTemplates(ctx context.Context, c identity.Caller) ([]WorkflowTemplate, error) {
	if err := requireInstanceAdministrator(ctx, s.pool, c); err != nil {
		return nil, err
	}
	list, _, err := s.WorkflowStarts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		rows, err := s.pool.Query(ctx, `SELECT w.id, w.name, sp.id, sp.name, w.template_version FROM workflows w
			JOIN spaces sp ON sp.id = w.space_id
			WHERE w.template_id = $1 AND w.archived_at IS NULL ORDER BY lower(sp.name), lower(w.name) LIMIT 1000`, list[i].ID)
		if err != nil {
			return nil, err
		}
		if list[i].Copies, err = pgx.CollectRows(rows, pgx.RowToStructByPos[TemplateCopy]); err != nil {
			return nil, err
		}
		if list[i].Copies == nil {
			list[i].Copies = []TemplateCopy{}
		}
		if list[i].Names, err = reviewerNames(ctx, s.pool, reviewerIDs(list[i].Definition)); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func loadTemplate(ctx context.Context, q db.Querier, id uuid.UUID, lock bool) (WorkflowTemplate, error) {
	sql := "SELECT " + templateColumns + " FROM workflow_templates WHERE id = $1 AND archived_at IS NULL"
	if lock {
		sql += " FOR NO KEY UPDATE"
	}
	t, err := scanTemplate(q.QueryRow(ctx, sql, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return t, apperr.NotFound()
	}
	return t, err
}

func insertTemplateName(ctx context.Context, tx pgx.Tx, sql string, args ...any) error {
	_, err := tx.Exec(ctx, sql, args...)
	if db.IsUniqueViolation(err) {
		return apperr.Conflict("template_name_taken", "There is already a template with that name.")
	}
	return err
}

// CreateWorkflowTemplate publishes a workflow as an instance template.
func (s *Service) CreateWorkflowTemplate(ctx context.Context, c identity.Caller, in WorkflowInput) (WorkflowTemplate, error) {
	id := ids.New()
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := requireInstanceAdministrator(ctx, tx, c); err != nil {
			return err
		}
		in, err := in.cleaned()
		if err != nil {
			return err
		}
		if err := checkReviewerRefs(ctx, tx, in.Definition); err != nil {
			return err
		}
		raw, err := json.Marshal(in.Definition)
		if err != nil {
			return err
		}
		if err := insertTemplateName(ctx, tx, `INSERT INTO workflow_templates (id, name, description, definition, saved_by)
			VALUES ($1, $2, $3, $4, $5)`, id, in.Name, in.Description, raw, c.UserID); err != nil {
			return err
		}
		return recordInstance(ctx, tx, c, "workflow_template.created", id, map[string]any{"name": in.Name})
	})
	if err != nil {
		return WorkflowTemplate{}, err
	}
	return loadTemplate(ctx, s.pool, id, false)
}

// SaveWorkflowTemplate stores the next version of a template. Its copies stay as they are.
func (s *Service) SaveWorkflowTemplate(ctx context.Context, c identity.Caller, id uuid.UUID, in WorkflowInput) (WorkflowTemplate, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := requireInstanceAdministrator(ctx, tx, c); err != nil {
			return err
		}
		t, err := loadTemplate(ctx, tx, id, true)
		if err != nil {
			return err
		}
		in, err := in.cleaned()
		if err != nil {
			return err
		}
		if in.BaseVersion != t.Version {
			return apperr.Conflict("version_conflict", "Someone else changed this template while you were editing it.")
		}
		if err := checkReviewerRefs(ctx, tx, in.Definition); err != nil {
			return err
		}
		raw, err := json.Marshal(in.Definition)
		if err != nil {
			return err
		}
		if err := insertTemplateName(ctx, tx, `UPDATE workflow_templates SET name = $2, description = $3, definition = $4,
			version = version + 1, saved_by = $5, saved_at = now() WHERE id = $1`, id, in.Name, in.Description, raw, c.UserID); err != nil {
			return err
		}
		return recordInstance(ctx, tx, c, "workflow_template.saved", id, map[string]any{"name": in.Name, "version": t.Version + 1,
			"changes": len(workflow.Diff(t.Definition, in.Definition))})
	})
	if err != nil {
		return WorkflowTemplate{}, err
	}
	return loadTemplate(ctx, s.pool, id, false)
}

// ArchiveWorkflowTemplate stops offering a template. Its copies are the spaces' own and stay.
func (s *Service) ArchiveWorkflowTemplate(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := requireInstanceAdministrator(ctx, tx, c); err != nil {
			return err
		}
		t, err := loadTemplate(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE workflow_templates SET archived_at = now() WHERE id = $1", id); err != nil {
			return err
		}
		return recordInstance(ctx, tx, c, "workflow_template.archived", id, map[string]any{"name": t.Name})
	})
}

// CopyWorkflowTemplate makes a workflow of a space from a template. The copy records the
// template and its version, and is the space's to change (DOC-130).
func (s *Service) CopyWorkflowTemplate(ctx context.Context, c identity.Caller, spaceID, templateID uuid.UUID, name string) (Workflow, error) {
	// Access first: a stranger learns nothing of the templates from a refusal.
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceWorkflows); err != nil {
		return Workflow{}, err
	}
	t, err := loadTemplate(ctx, s.pool, templateID, false)
	if err != nil {
		return Workflow{}, err
	}
	if name == "" {
		name = t.Name
	}
	w, err := s.createWorkflow(ctx, c, spaceID, WorkflowInput{Name: name, Description: t.Description, Definition: t.Definition},
		&TemplateCopy{WorkflowID: t.ID, Version: t.Version})
	return w, err
}

// TemplateChanges lists what taking the newest version of its template would change in a
// workflow, so a space can compare before it chooses (DOC-130).
func (s *Service) TemplateChanges(ctx context.Context, c identity.Caller, id uuid.UUID) ([]workflow.Change, int, error) {
	w, err := s.Workflow(ctx, c, id)
	if err != nil {
		return nil, 0, err
	}
	if w.TemplateID == nil {
		return nil, 0, apperr.Conflict("no_template", "This workflow was not made from a template.")
	}
	t, err := loadTemplate(ctx, s.pool, *w.TemplateID, false)
	if err != nil {
		return nil, 0, err
	}
	return workflow.Diff(w.Definition, t.Definition), t.Version, nil
}

// TakeTemplate saves the newest version of a workflow's template as the workflow's next
// version, by the space's choice. Running rounds keep their version (DOC-110).
func (s *Service) TakeTemplate(ctx context.Context, c identity.Caller, id uuid.UUID, baseVersion int) (Workflow, error) {
	w, err := s.Workflow(ctx, c, id)
	if err != nil {
		return w, err
	}
	if w.TemplateID == nil {
		return w, apperr.Conflict("no_template", "This workflow was not made from a template.")
	}
	t, err := loadTemplate(ctx, s.pool, *w.TemplateID, false)
	if err != nil {
		return w, err
	}
	return s.saveWorkflow(ctx, c, id, WorkflowInput{Name: w.Name, Description: w.Description, Definition: t.Definition, BaseVersion: baseVersion},
		&TemplateCopy{WorkflowID: t.ID, Version: t.Version})
}

// checkReviewerRefs refuses reviewers that do not exist, roles that are not roles of a
// space, and guests, who never review (DOC-200, DOC-1224).
func checkReviewerRefs(ctx context.Context, q db.Querier, d workflow.Definition) error {
	for _, st := range d.States {
		if st.Review == nil {
			continue
		}
		for _, r := range st.Review.Reviewers {
			var ok bool
			var err error
			switch r.Type {
			case workflow.ReviewerUser:
				var typ string
				err = q.QueryRow(ctx, "SELECT type FROM users WHERE id = $1", r.ID).Scan(&typ)
				if err == nil && typ == "guest" {
					return apperr.Conflict("guest_reviewer", fmt.Sprintf("A guest cannot review, in %q.", st.Name))
				}
				ok = err == nil && typ == "member"
			case workflow.ReviewerGroup:
				err = q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1)", r.ID).Scan(&ok)
			case workflow.ReviewerRole:
				err = q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM roles WHERE id = $1 AND scope = 'space')", r.ID).Scan(&ok)
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if !ok {
				return apperr.Conflict("unknown_reviewer", fmt.Sprintf("A reviewer of %q does not exist, or is not a person, group or space role.", st.Name))
			}
		}
	}
	return nil
}

// recordInstance audits a change of the instance's templates.
func recordInstance(ctx context.Context, tx pgx.Tx, c identity.Caller, action string, id uuid.UUID, details map[string]any) error {
	_, err := audit.Record(ctx, tx, c.Event(action, "workflow_template", id.String(), "", details))
	return err
}
