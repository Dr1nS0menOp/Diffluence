// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/workflow"
)

// ExportPage is a page as an export sees it.
type ExportPage struct {
	ID       uuid.UUID
	ParentID *uuid.UUID
	Title    string
	Document json.RawMessage
	// Control is the state of a controlled page, for the control block of a file; nil for an
	// ordinary page (spec 064, DOC-531).
	Control *ExportControl
}

// ExportControl is what a file says about the version of a controlled page it holds.
type ExportControl struct {
	StateName string
	// Version is the version in the file.
	Version int
	// Working says the file holds a working version, which is not approved.
	Working         bool
	ApprovedVersion *int
	ApprovedAt      *time.Time
	ApprovedBy      string
	// History says the person may see the versions before, as an author may (DOC-321).
	History bool
	// Signatures are those of the version in the file, oldest first (DOC-740, §11.50(b)).
	Signatures []SignatureView
}

// ForExport returns the published pages the user may view, parents first. With
// root set, only that page and what is below it. It goes through the same SQL
// rule as every list, so an export can never contain more than the tree shows. A
// controlled page holds the version the person reads: the approved one, or the working
// one for its authors (DOC-530).
func (s *Service) ForExport(ctx context.Context, c identity.Caller, spaceID uuid.UUID, root *uuid.UUID) ([]ExportPage, error) {
	if err := s.perms.RequireSpace(ctx, s.pool, c.UserID, spaceID, permissions.SpaceExport); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.id, p.parent_id, p.title, v.document
		FROM pages p
		JOIN perm_visible_pages($1) vis ON vis.page_id = p.id
		JOIN page_versions v ON v.page_id = p.id AND v.number = p.current_version
		LEFT JOIN page_tree d ON d.descendant_id = p.id AND d.ancestor_id = $3
		WHERE p.space_id = $2 AND p.status = 'published' AND ($3::uuid IS NULL OR d.ancestor_id IS NOT NULL)
		ORDER BY (SELECT count(*) FROM page_tree a WHERE a.descendant_id = p.id), p.position, p.id`, c.UserID, spaceID, root)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExportPage, error) {
		var p ExportPage
		err := r.Scan(&p.ID, &p.ParentID, &p.Title, &p.Document)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	if err := s.exportControl(ctx, c, out); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Document, err = s.withNames(ctx, out[i].Document); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// exportControl fills in the control of the controlled pages of an export, and puts the
// working version in place of the approved one for whoever may see it.
func (s *Service) exportControl(ctx context.Context, c identity.Caller, list []ExportPage) error {
	at := make(map[uuid.UUID]int, len(list))
	ids := make([]uuid.UUID, len(list))
	for i, p := range list {
		at[p.ID], ids[i] = i, p.ID
	}
	rows, err := s.pool.Query(ctx, `SELECT pc.page_id, pc.state_id, COALESCE(pc.approved_state_id, ''), v.definition, pc.approved_version, pc.approved_at,
			pc.approved_by, pc.head_version, p.current_version
		FROM page_control pc
		JOIN pages p ON p.id = pc.page_id
		JOIN workflow_versions v ON v.workflow_id = pc.workflow_id AND v.version = pc.workflow_version
		WHERE pc.page_id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	type found struct {
		id                          uuid.UUID
		control                     ExportControl
		head, current               int
		workingState, approvedState string
		approver                    *uuid.UUID
	}
	controlled, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (found, error) {
		var f found
		var state, approvedState string
		var raw []byte
		if err := r.Scan(&f.id, &state, &approvedState, &raw, &f.control.ApprovedVersion, &f.control.ApprovedAt, &f.approver, &f.head, &f.current); err != nil {
			return f, err
		}
		var def workflow.Definition
		if err := json.Unmarshal(raw, &def); err != nil {
			return f, fmt.Errorf("pages: read workflow of page %s: %v", f.id, err)
		}
		f.control.Version = f.current
		f.control.Working = f.control.ApprovedVersion == nil
		f.workingState, f.approvedState = stateName(def, state), stateName(def, approvedState)
		return f, nil
	})
	if err != nil {
		return err
	}
	approvers := []uuid.UUID{}
	for _, f := range controlled {
		if f.approver != nil {
			approvers = append(approvers, *f.approver)
		}
	}
	names, err := identity.DisplayNames(ctx, s.pool, approvers)
	if err != nil {
		return err
	}
	for _, f := range controlled {
		p := &list[at[f.id]]
		control := f.control
		if f.approver != nil {
			control.ApprovedBy = names[*f.approver]
		}
		control.StateName = f.approvedState
		if control.Working {
			control.StateName = f.workingState
		}
		working, err := s.perms.ReadsWorking(ctx, s.pool, c.UserID, f.id)
		if err != nil {
			return err
		}
		control.History = working
		if working && f.head > f.current {
			if err := s.pool.QueryRow(ctx, "SELECT title, document FROM page_versions WHERE page_id = $1 AND number = $2", f.id, f.head).
				Scan(&p.Title, &p.Document); err != nil {
				return err
			}
			control.Version, control.Working, control.StateName = f.head, true, f.workingState
		}
		if control.Signatures, err = signaturesOf(ctx, s.pool, f.id, []int{control.Version}); err != nil {
			return err
		}
		p.Control = &control
	}
	return nil
}

// SpaceOf returns the space of a page the user may view.
func (s *Service) SpaceOf(ctx context.Context, c identity.Caller, id uuid.UUID) (uuid.UUID, error) {
	if err := s.perms.RequirePage(ctx, s.pool, c.UserID, id, permissions.ActionPageView, uuid.Nil); err != nil {
		return uuid.Nil, err
	}
	p, err := load(ctx, s.pool, id)
	return p.SpaceID, err
}

// stateName is the name of a state of a workflow, or nothing for a state it does not have.
func stateName(def workflow.Definition, id string) string {
	st, _ := def.State(id)
	return st.Name
}
