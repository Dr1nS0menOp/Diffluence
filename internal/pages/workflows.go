// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

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

// Workflow is a document-control workflow of a space, at its newest version (spec 064).
type Workflow struct {
	ID          uuid.UUID           `json:"id"`
	SpaceID     uuid.UUID           `json:"space_id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Version     int                 `json:"version"`
	Definition  workflow.Definition `json:"definition"`
	Archived    bool                `json:"archived"`
	CreatedAt   time.Time           `json:"created_at"`
	// Pages is how many pages it governs now.
	Pages int `json:"pages"`
	// SpaceWorkflow says it is the workflow of the whole space.
	SpaceWorkflow bool `json:"space_workflow"`
	// TemplateID and TemplateVersion name the instance template it was copied from; and
	// TemplateLatest the template's newest version, which the space may take (DOC-130).
	TemplateID      *uuid.UUID `json:"template_id"`
	TemplateVersion *int       `json:"template_version"`
	TemplateLatest  *int       `json:"template_latest"`
	// SavedAt is when its newest version was saved.
	SavedAt time.Time `json:"saved_at"`
	// Names gives the people, groups and roles its review states name, for those who may
	// change it; others get the ids only, so the list reveals no one they may not see.
	Names map[string]string `json:"names,omitempty"`
}

// WorkflowInput is a new workflow, or the next version of one.
type WorkflowInput struct {
	Name        string
	Description string
	Definition  workflow.Definition
	// BaseVersion is the version the change was made on; a save on an older one is refused.
	BaseVersion int
}

// WorkflowVersion is one saved version of a workflow and what changed in it (DOC-112).
type WorkflowVersion struct {
	Version    int                 `json:"version"`
	Definition workflow.Definition `json:"definition"`
	SavedBy    string              `json:"saved_by"`
	SavedAt    time.Time           `json:"saved_at"`
	Changes    []workflow.Change   `json:"changes"`
}

const (
	maxWorkflowNameRunes = 80
	maxWorkflowDescRunes = 500
	maxWorkflowsPerSpace = 50
)

func (in WorkflowInput) cleaned() (WorkflowInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxWorkflowNameRunes || strings.ContainsAny(in.Name, "\r\n\x00") {
		return in, apperr.Invalid("A workflow needs a name of one line and at most 80 characters.")
	}
	if utf8.RuneCountInString(in.Description) > maxWorkflowDescRunes || strings.ContainsRune(in.Description, 0) {
		return in, apperr.Invalid("A description can have at most 500 characters.")
	}
	if err := workflow.Validate(in.Definition); err != nil {
		var p *workflow.Problem
		if errors.As(err, &p) {
			return in, apperr.Conflict("workflow_"+p.Rule, p.Message)
		}
		return in, err
	}
	return in, nil
}

const workflowColumns = `w.id, w.space_id, w.name, w.description, w.current_version, v.definition, w.archived_at IS NOT NULL, w.created_at,
	(SELECT count(*) FROM page_control pc WHERE pc.workflow_id = w.id),
	EXISTS (SELECT 1 FROM space_workflows sw WHERE sw.workflow_id = w.id),
	w.template_id, w.template_version, (SELECT t.version FROM workflow_templates t WHERE t.id = w.template_id AND t.archived_at IS NULL), v.saved_at`

func scanWorkflow(r pgx.Row) (Workflow, error) {
	var w Workflow
	var raw []byte
	if err := r.Scan(&w.ID, &w.SpaceID, &w.Name, &w.Description, &w.Version, &raw, &w.Archived, &w.CreatedAt, &w.Pages, &w.SpaceWorkflow,
		&w.TemplateID, &w.TemplateVersion, &w.TemplateLatest, &w.SavedAt); err != nil {
		return w, err
	}
	if err := json.Unmarshal(raw, &w.Definition); err != nil {
		return w, fmt.Errorf("pages: read workflow %s: %v", w.ID, err)
	}
	return w, nil
}

func loadWorkflow(ctx context.Context, q db.Querier, id uuid.UUID) (Workflow, error) {
	w, err := scanWorkflow(q.QueryRow(ctx, `SELECT `+workflowColumns+` FROM workflows w
		JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version WHERE w.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return w, apperr.NotFound()
	}
	return w, err
}

// definitionOf reads one version of a workflow: a page follows the version its round
// started under (DOC-110).
func definitionOf(ctx context.Context, q db.Querier, id uuid.UUID, version int) (workflow.Definition, error) {
	var raw []byte
	var d workflow.Definition
	if err := q.QueryRow(ctx, "SELECT definition FROM workflow_versions WHERE workflow_id = $1 AND version = $2", id, version).Scan(&raw); err != nil {
		return d, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("pages: read workflow %s version %d: %v", id, version, err)
	}
	return d, nil
}

// recordAudit writes an audit entry without an event: what it is about is not for
// watchers, such as a working version readers may not see.
func recordAudit(ctx context.Context, tx pgx.Tx, c identity.Caller, action, objectType string, objectID, spaceID uuid.UUID, details map[string]any) error {
	_, err := audit.Record(ctx, tx, c.Event(action, objectType, objectID.String(), spaceID.String(), details))
	return err
}

// Workflows lists the workflows of a space. Whoever views the space may see them, so a
// person who sets a workflow on a page can choose one.
func (s *Service) Workflows(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Workflow, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceView); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+workflowColumns+` FROM workflows w
		JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version
		WHERE w.space_id = $1 ORDER BY w.archived_at IS NOT NULL, lower(w.name) LIMIT $2`, spaceID, maxWorkflowsPerSpace*2)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Workflow, error) { return scanWorkflow(r) })
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []Workflow{}
	}
	return out, s.nameReviewers(ctx, c, spaceID, out)
}

// nameReviewers fills in the names of the people, groups and roles the workflows name, for
// someone who may change the workflows of the space.
func (s *Service) nameReviewers(ctx context.Context, c identity.Caller, spaceID uuid.UUID, list []Workflow) error {
	d, err := s.perms.CheckSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceWorkflows)
	if err != nil || !d.Allowed {
		return err
	}
	var ids []uuid.UUID
	for _, w := range list {
		ids = append(ids, reviewerIDs(w.Definition)...)
	}
	names, err := reviewerNames(ctx, s.pool, ids)
	if err != nil {
		return err
	}
	for i := range list {
		list[i].Names = map[string]string{}
		for _, id := range reviewerIDs(list[i].Definition) {
			if n, ok := names[id.String()]; ok {
				list[i].Names[id.String()] = n
			}
		}
	}
	return nil
}

func reviewerIDs(d workflow.Definition) []uuid.UUID {
	var ids []uuid.UUID
	for _, st := range d.States {
		if st.Review != nil {
			for _, r := range st.Review.Reviewers {
				ids = append(ids, r.ID)
			}
		}
	}
	return ids
}

// reviewerNames reads the names of people, groups and roles by id, in one query.
func reviewerNames(ctx context.Context, q db.Querier, ids []uuid.UUID) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT id::text, display_name FROM users WHERE id = ANY($1)
		UNION ALL SELECT id::text, name FROM groups WHERE id = ANY($1)
		UNION ALL SELECT id::text, name FROM roles WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// Workflow returns one workflow.
func (s *Service) Workflow(ctx context.Context, c identity.Caller, id uuid.UUID) (Workflow, error) {
	w, err := loadWorkflow(ctx, s.pool, id)
	if err != nil {
		return w, err
	}
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, w.SpaceID, permissions.SpaceView); err != nil {
		return Workflow{}, err
	}
	list := []Workflow{w}
	if err := s.nameReviewers(ctx, c, w.SpaceID, list); err != nil {
		return Workflow{}, err
	}
	return list[0], nil
}

// CreateWorkflow adds a workflow to a space (DOC-100).
func (s *Service) CreateWorkflow(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in WorkflowInput) (Workflow, error) {
	return s.createWorkflow(ctx, c, spaceID, in, nil)
}

// createWorkflow adds a workflow, made from a template when from names one.
func (s *Service) createWorkflow(ctx context.Context, c identity.Caller, spaceID uuid.UUID, in WorkflowInput, from *TemplateCopy) (Workflow, error) {
	id := ids.New()
	var templateID *uuid.UUID
	var templateVersion *int
	if from != nil {
		templateID, templateVersion = &from.WorkflowID, &from.Version
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Access first: a stranger learns nothing of the space from a refused input.
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, spaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		in, err := in.cleaned()
		if err != nil {
			return err
		}
		raw, err := json.Marshal(in.Definition)
		if err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM workflows WHERE space_id = $1 AND archived_at IS NULL", spaceID).Scan(&count); err != nil {
			return err
		}
		if count >= maxWorkflowsPerSpace {
			return apperr.Conflict("too_many_workflows", "A space has at most 50 workflows. Archive one first.")
		}
		if err := checkReviewerRefs(ctx, tx, in.Definition); err != nil {
			return err
		}
		if err := insertWorkflowName(ctx, tx, `INSERT INTO workflows (id, space_id, name, description, created_by, template_id, template_version)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, id, spaceID, in.Name, in.Description, c.UserID, templateID, templateVersion); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO workflow_versions (workflow_id, version, definition, saved_by) VALUES ($1, 1, $2, $3)",
			id, raw, c.UserID); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "workflow.created", "workflow", id, spaceID, map[string]any{"name": in.Name, "template": templateID})
	})
	if err != nil {
		return Workflow{}, err
	}
	return loadWorkflow(ctx, s.pool, id)
}

// insertWorkflowName runs a statement that sets a name, and turns the unique name into a
// conflict a person can act on.
func insertWorkflowName(ctx context.Context, tx pgx.Tx, sql string, args ...any) error {
	_, err := tx.Exec(ctx, sql, args...)
	if db.IsUniqueViolation(err) {
		return apperr.Conflict("workflow_name_taken", "This space already has a workflow with that name.")
	}
	return err
}

// SaveWorkflow stores the next version of a workflow. Pages in a running round keep the
// version they started under; a state some page is in cannot be removed (DOC-110).
func (s *Service) SaveWorkflow(ctx context.Context, c identity.Caller, id uuid.UUID, in WorkflowInput) (Workflow, error) {
	return s.saveWorkflow(ctx, c, id, in, nil)
}

// saveWorkflow stores the next version; from names the template version it takes, if any.
func (s *Service) saveWorkflow(ctx context.Context, c identity.Caller, id uuid.UUID, in WorkflowInput, from *TemplateCopy) (Workflow, error) {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		w, err := scanWorkflow(tx.QueryRow(ctx, `SELECT `+workflowColumns+` FROM workflows w
			JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version WHERE w.id = $1 FOR NO KEY UPDATE OF w`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, w.SpaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		in, err := in.cleaned()
		if err != nil {
			return err
		}
		raw, err := json.Marshal(in.Definition)
		if err != nil {
			return err
		}
		if w.Archived {
			return apperr.Conflict("workflow_archived", "This workflow is archived.")
		}
		if in.BaseVersion != w.Version {
			return apperr.Conflict("version_conflict", "Someone else changed this workflow while you were editing it.")
		}
		changes := workflow.Diff(w.Definition, in.Definition)
		if err := refuseOccupiedStates(ctx, tx, id, changes); err != nil {
			return err
		}
		if err := checkReviewerRefs(ctx, tx, in.Definition); err != nil {
			return err
		}
		if from != nil {
			if _, err := tx.Exec(ctx, "UPDATE workflows SET template_version = $2 WHERE id = $1", id, from.Version); err != nil {
				return err
			}
		}
		next := w.Version + 1
		if _, err := tx.Exec(ctx, "INSERT INTO workflow_versions (workflow_id, version, definition, saved_by) VALUES ($1, $2, $3, $4)",
			id, next, raw, c.UserID); err != nil {
			return err
		}
		if err := insertWorkflowName(ctx, tx, "UPDATE workflows SET name = $2, description = $3, current_version = $4 WHERE id = $1",
			id, in.Name, in.Description, next); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "workflow.saved", "workflow", id, w.SpaceID, map[string]any{"name": in.Name, "version": next, "changes": len(changes)})
	})
	if err != nil {
		return Workflow{}, err
	}
	return loadWorkflow(ctx, s.pool, id)
}

// CheckDefinition runs the validator for the editor while a person types, so each problem
// shows beside its state before saving (DOC-912). It answers nil for a valid workflow.
func CheckDefinition(d workflow.Definition) *workflow.Problem {
	var p *workflow.Problem
	if err := workflow.Validate(d); errors.As(err, &p) {
		return p
	}
	return nil
}

func refuseOccupiedStates(ctx context.Context, tx pgx.Tx, id uuid.UUID, changes []workflow.Change) error {
	var removed []string
	for _, ch := range changes {
		if ch.Kind == workflow.StateRemoved {
			removed = append(removed, ch.ID)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	var occupied int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM page_control WHERE workflow_id = $1 AND state_id = ANY($2)", id, removed).Scan(&occupied); err != nil {
		return err
	}
	if occupied > 0 {
		return apperr.Conflict("state_occupied", fmt.Sprintf("%d pages are in a state this change removes. Move them on first.", occupied))
	}
	return nil
}

// ArchiveWorkflow takes a workflow out of use. One that governs a page, or is set on a
// space or a page, cannot be archived: control is removed first, with its preview.
func (s *Service) ArchiveWorkflow(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		w, err := scanWorkflow(tx.QueryRow(ctx, `SELECT `+workflowColumns+` FROM workflows w
			JOIN workflow_versions v ON v.workflow_id = w.id AND v.version = w.current_version WHERE w.id = $1 FOR NO KEY UPDATE OF w`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if err := s.perms.RequireSpace(ctx, tx, c.UserID, w.SpaceID, permissions.SpaceWorkflows); err != nil {
			return err
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM page_control WHERE workflow_id = $1)
			OR EXISTS (SELECT 1 FROM page_workflows WHERE workflow_id = $1)
			OR EXISTS (SELECT 1 FROM space_workflows WHERE workflow_id = $1)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return apperr.Conflict("workflow_in_use", "This workflow governs pages. Take it off them first.")
		}
		if _, err := tx.Exec(ctx, "UPDATE workflows SET archived_at = now() WHERE id = $1 AND archived_at IS NULL", id); err != nil {
			return err
		}
		return recordAudit(ctx, tx, c, "workflow.archived", "workflow", id, w.SpaceID, map[string]any{"name": w.Name})
	})
}

// WorkflowHistory lists every version of a workflow with what changed in it, newest first.
func (s *Service) WorkflowHistory(ctx context.Context, c identity.Caller, id uuid.UUID) ([]WorkflowVersion, error) {
	w, err := s.Workflow(ctx, c, id)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT v.version, v.definition, COALESCE(u.display_name, ''), v.saved_at
		FROM workflow_versions v LEFT JOIN users u ON u.id = v.saved_by WHERE v.workflow_id = $1 ORDER BY v.version LIMIT 1000`, w.ID)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (WorkflowVersion, error) {
		var v WorkflowVersion
		var raw []byte
		if err := r.Scan(&v.Version, &raw, &v.SavedBy, &v.SavedAt); err != nil {
			return v, err
		}
		return v, json.Unmarshal(raw, &v.Definition)
	})
	if err != nil {
		return nil, err
	}
	out := make([]WorkflowVersion, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		v := list[i]
		if i > 0 {
			v.Changes = workflow.Diff(list[i-1].Definition, v.Definition)
		} else {
			v.Changes = []workflow.Change{}
		}
		out = append(out, v)
	}
	return out, nil
}
