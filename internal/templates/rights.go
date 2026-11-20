// SPDX-License-Identifier: Apache-2.0

package templates

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/db"
	"github.com/kenwerk/kenwerk/internal/features"
	"github.com/kenwerk/kenwerk/internal/identity"
	"github.com/kenwerk/kenwerk/internal/permissions"
	"github.com/kenwerk/kenwerk/internal/spaces"
)

// maxListed is how many people and groups one audience list holds (TPL-500).
const maxListed = 50

// Rights are the audience lists of a template (TPL-500) and, for a template of all spaces,
// where it is offered (TPL-502). An empty list adds no limit.
type Rights struct {
	Use  []permissions.Subject `json:"use"`
	Edit []permissions.Subject `json:"edit"`
	// Chosen says the template of all spaces is offered only in Spaces; otherwise everywhere.
	Chosen bool        `json:"chosen"`
	Spaces []uuid.UUID `json:"spaces"`
}

// RightsView is the lists as the Who can use it section shows them, with today's names.
type RightsView struct {
	Rights
	Names map[string]string `json:"names"`
	// AllSpaces says the template is of all spaces, so it has spaces to choose.
	AllSpaces bool `json:"all_spaces"`
}

// rightsOf reads the lists of a template in one query, and its spaces in a second only when
// they were chosen.
func rightsOf(ctx context.Context, q db.Querier, templateID uuid.UUID) (Rights, error) {
	out := Rights{Use: []permissions.Subject{}, Edit: []permissions.Subject{}, Spaces: []uuid.UUID{}}
	rows, err := q.Query(ctx, `SELECT r.kind, r.subject_type, r.subject_id, t.offered_in_chosen
		FROM templates t LEFT JOIN template_rights r ON r.template_id = t.id
		WHERE t.id = $1 ORDER BY r.kind, r.subject_type, r.subject_id`, templateID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, typ *string
		var id *uuid.UUID
		if err := rows.Scan(&kind, &typ, &id, &out.Chosen); err != nil {
			return out, err
		}
		if kind == nil {
			continue
		}
		sub := permissions.Subject{Type: *typ, ID: *id}
		if *kind == RightUse {
			out.Use = append(out.Use, sub)
		} else {
			out.Edit = append(out.Edit, sub)
		}
	}
	if err := rows.Err(); err != nil || !out.Chosen {
		return out, err
	}
	rows, err = q.Query(ctx, "SELECT space_id FROM template_spaces WHERE template_id = $1 ORDER BY space_id", templateID)
	if err != nil {
		return out, err
	}
	out.Spaces, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	return out, err
}

// RightsOf gives the lists of a template to whoever looks after it.
func (s *Service) RightsOf(ctx context.Context, c identity.Caller, id uuid.UUID) (RightsView, error) {
	t, err := load(ctx, s.pool, c.UserID, id)
	if err != nil {
		return RightsView{}, err
	}
	st, err := s.requireTo(ctx, s.pool, c, t, standing.mayLook)
	if err != nil {
		return RightsView{}, err
	}
	view := RightsView{Rights: st.rights, AllSpaces: t.SpaceID == nil}
	view.Names, err = s.subjectNames(ctx, append(slices.Clone(st.rights.Use), st.rights.Edit...))
	return view, err
}

// subjectNames names people and groups for the screen, by id.
func (s *Service) subjectNames(ctx context.Context, subjects []permissions.Subject) (map[string]string, error) {
	people := []uuid.UUID{}
	for _, sub := range subjects {
		if sub.Type == permissions.SubjectUser {
			people = append(people, sub.ID)
		}
	}
	names := map[string]string{}
	byID, err := identity.DisplayNames(ctx, s.pool, people)
	if err != nil {
		return nil, err
	}
	for id, name := range byID {
		names[id.String()] = name
	}
	for _, sub := range subjects {
		if sub.Type != permissions.SubjectGroup {
			continue
		}
		g, err := s.identity.GetGroup(ctx, s.pool, sub.ID)
		if apperr.IsNotFound(err) {
			continue // a group that is gone has no name; the screen says so
		}
		if err != nil {
			return nil, err
		}
		names[sub.ID.String()] = g.Name
	}
	return names, nil
}

// checkSubjects refuses a list that is too long, names something other than people and
// groups, or names one that does not exist (TPL-500).
func (s *Service) checkSubjects(ctx context.Context, q db.Querier, list []permissions.Subject) ([]permissions.Subject, error) {
	if len(list) > maxListed {
		return nil, apperr.Invalid("A list of who may use or edit a template holds at most 50 people and groups.")
	}
	out := []permissions.Subject{}
	for _, sub := range list {
		if (sub.Type != permissions.SubjectUser && sub.Type != permissions.SubjectGroup) || sub.ID == uuid.Nil {
			return nil, apperr.Invalid("A list of who may use or edit a template holds only people and groups.")
		}
		if !slices.Contains(out, sub) {
			out = append(out, sub)
		}
	}
	gone, err := s.perms.MissingSubjects(ctx, q, out)
	if err != nil {
		return nil, err
	}
	if len(gone) > 0 {
		return nil, apperr.Invalid("A person or group on the list no longer exists.")
	}
	return out, nil
}

// errEditorsLockout refuses an editors list that leaves out the person setting it, which
// could leave nobody able to change the template (TPL-501).
var errEditorsLockout = &apperr.Error{Kind: apperr.KindInvalid, Code: "editors_lockout",
	Message: "Put yourself on the list of who may edit this template, by name or through a group, so it is never left without an editor."}

// SetRights replaces the lists of a template and, for one of all spaces, where it is offered.
// They change who finds and changes it, not what it makes, so they are no version (TPL-600).
// Whoever manages the template sets them. A holder of space.administer may always clear them,
// so a template is never locked away for good (TPL-501).
func (s *Service) SetRights(ctx context.Context, c identity.Caller, id uuid.UUID, in Rights) (RightsView, error) {
	if err := s.holdForOlderRelease(ctx); err != nil {
		return RightsView{}, err
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		t, err := lockTemplate(ctx, tx, c.UserID, id)
		if err != nil {
			return err
		}
		clearing := len(in.Use) == 0 && len(in.Edit) == 0 && !in.Chosen
		st, err := s.requireTo(ctx, tx, c, t, func(st standing) bool {
			return st.manages() || (clearing && st.administer)
		})
		if err != nil {
			return err
		}
		next, err := s.cleanRights(ctx, tx, c, st, in)
		if err != nil {
			return err
		}
		return s.storeRights(ctx, tx, c, st, next)
	})
	if err != nil {
		return RightsView{}, err
	}
	return s.RightsOf(ctx, c, id)
}

// cleanRights checks what a person asks the lists to become.
func (s *Service) cleanRights(ctx context.Context, tx pgx.Tx, c identity.Caller, st standing, in Rights) (Rights, error) {
	use, err := s.checkSubjects(ctx, tx, in.Use)
	if err != nil {
		return Rights{}, err
	}
	edit, err := s.checkSubjects(ctx, tx, in.Edit)
	if err != nil {
		return Rights{}, err
	}
	if len(edit) > 0 && !matches(c.UserID, st.groups, edit) {
		return Rights{}, errEditorsLockout
	}
	next := Rights{Use: use, Edit: edit, Spaces: []uuid.UUID{}}
	if st.t.SpaceID != nil {
		if in.Chosen || len(in.Spaces) > 0 {
			return Rights{}, apperr.Invalid("Only a template of all spaces is offered in chosen spaces.")
		}
		return next, nil
	}
	next.Chosen = in.Chosen
	for _, space := range in.Spaces {
		if slices.Contains(next.Spaces, space) {
			continue
		}
		if _, err := spaces.LoadIn(ctx, tx, space); apperr.IsNotFound(err) {
			return Rights{}, apperr.Invalid("A chosen space no longer exists.")
		} else if err != nil {
			return Rights{}, err
		}
		next.Spaces = append(next.Spaces, space)
	}
	return next, nil
}

// storeRights writes the lists and records the change with the lists before and after, as ids
// (TPL-711). A template with a list counts as one that uses this part, so it is not offered
// while the part is off (columns, TPL-050).
func (s *Service) storeRights(ctx context.Context, tx pgx.Tx, c identity.Caller, st standing, next Rights) error {
	id := st.t.ID
	if _, err := tx.Exec(ctx, "DELETE FROM template_rights WHERE template_id = $1", id); err != nil {
		return err
	}
	for kind, list := range map[string][]permissions.Subject{RightUse: next.Use, RightEdit: next.Edit} {
		for _, sub := range list {
			if _, err := tx.Exec(ctx, `INSERT INTO template_rights (template_id, kind, subject_type, subject_id) VALUES ($1, $2, $3, $4)`,
				id, kind, sub.Type, sub.ID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM template_spaces WHERE template_id = $1", id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO template_spaces (template_id, space_id) SELECT $1, unnest($2::uuid[])`, id, next.Spaces); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE templates SET offered_in_chosen = $2 WHERE id = $1", id, next.Chosen); err != nil {
		return err
	}
	details := map[string]any{"before": st.rights, "after": next}
	if !st.manages() {
		details["cleared_by_space_administrator"] = true
	}
	return auditTemplateDetails(ctx, tx, c, "template.rights_changed", st.t, details)
}

// holdForOlderRelease refuses what a node of the previous release would misread while one
// still runs: a list, a draft and the four-eyes setting (TPL-1103).
func (s *Service) holdForOlderRelease(ctx context.Context) error {
	return s.features.RefuseWhileOlderReleaseRuns(ctx, features.ExperimentalGuidedTemplates)
}
