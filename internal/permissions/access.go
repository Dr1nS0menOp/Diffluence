// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"context"
	"encoding/csv"
	"errors"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

const (
	maxAccessRows  = 200
	maxSetName     = 100
	roleCustom     = "custom"
	sourceDirect   = "direct"
	sourceGroup    = "group"
	sourceEveryone = "everyone"
)

// RoleOf names a set of permissions: one of the built-in sets, or custom.
func RoleOf(perms []string) string {
	sorted := slices.Sorted(slices.Values(perms))
	for _, name := range []string{TemplateViewer, TemplateEditor, TemplateManager, TemplateSpaceAdmin} {
		if slices.Equal(slices.Sorted(slices.Values(Templates[name])), sorted) {
			return name
		}
	}
	return roleCustom
}

// Holder is who holds access to a space, and how.
type Holder struct {
	Subject Subject `json:"subject"`
	Name    string  `json:"name"`
	Role    string  `json:"role"`
	// Source says how: directly, through a group, or as everyone of a kind.
	Source string `json:"source"`
	// Via is the name of the group.
	Via string `json:"via"`
}

// Holders lists who holds access to a space, for anyone who can view it (SPC-058).
// "All members" stays one line: it does not name every person.
func (s *Service) Holders(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Holder, error) {
	if err := s.RequireSpace(ctx, s.pool, c.UserID, spaceID, SpaceView); err != nil {
		return nil, err
	}
	return s.holders(ctx, spaceID)
}

func (s *Service) holders(ctx context.Context, spaceID uuid.UUID) ([]Holder, error) {
	entries, err := s.grants(ctx, s.pool, spaceID)
	if err != nil {
		return nil, err
	}
	out := make([]Holder, 0, len(entries))
	for _, e := range entries {
		if !slices.Contains(e.Permissions, SpaceView) {
			continue
		}
		source := sourceDirect
		switch e.Subject.Type {
		case SubjectGroup:
			source = sourceGroup
		case SubjectAllMembers, SubjectAllGuests:
			source = sourceEveryone
		}
		out = append(out, Holder{Subject: e.Subject, Name: e.Name, Role: RoleOf(e.Permissions), Source: source})
	}
	return out, nil
}

// AccessDetail lists each person with access and how they got it, for those who
// manage access. Groups are opened up, up to a limit.
func (s *Service) AccessDetail(ctx context.Context, c identity.Caller, spaceID uuid.UUID, text string) ([]Holder, error) {
	if err := s.RequireSpace(ctx, s.pool, c.UserID, spaceID, SpaceManageAccess); err != nil {
		return nil, err
	}
	entries, err := s.holders(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	wanted := strings.ToLower(strings.TrimSpace(text))
	out := []Holder{}
	for _, e := range entries {
		if e.Source != sourceGroup {
			if strings.Contains(strings.ToLower(e.Name), wanted) {
				out = append(out, e)
			}
			continue
		}
		rows, err := s.pool.Query(ctx, `SELECT u.id, u.display_name FROM group_members m JOIN users u ON u.id = m.user_id
			WHERE m.group_id = $1 AND ($2 = '' OR position($2 IN lower(u.display_name)) > 0)
			ORDER BY lower(u.display_name) LIMIT $3`, e.Subject.ID, wanted, maxAccessRows)
		if err != nil {
			return nil, err
		}
		members, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Holder, error) {
			h := Holder{Subject: Subject{Type: SubjectUser}, Role: e.Role, Source: sourceGroup, Via: e.Name}
			err := r.Scan(&h.Subject.ID, &h.Name)
			return h, err
		})
		if err != nil {
			return nil, err
		}
		out = append(out, members...)
	}
	return out, nil
}

// neutralise keeps a spreadsheet from running a name as a formula.
func neutralise(cell string) string {
	if cell != "" && strings.ContainsRune("=+-@\t\r", rune(cell[0])) {
		return "'" + cell
	}
	return cell
}

// WriteAccessCSV writes who has access, with formulas neutralised, and audits it.
func (s *Service) WriteAccessCSV(ctx context.Context, c identity.Caller, spaceID uuid.UUID, w io.Writer) error {
	list, err := s.AccessDetail(ctx, c, spaceID, "")
	if err != nil {
		return err
	}
	if err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return recordChange(ctx, tx, c, "space.access_exported", "space", spaceID.String(), spaceID.String(), nil)
	}); err != nil {
		return err
	}
	out := csv.NewWriter(w)
	if err := out.Write([]string{"name", "kind", "role", "source", "via"}); err != nil {
		return err
	}
	for _, h := range list {
		if err := out.Write([]string{neutralise(h.Name), h.Subject.Type, h.Role, h.Source, neutralise(h.Via)}); err != nil {
			return err
		}
	}
	out.Flush()
	return out.Error()
}

// GrantPreview is what a change of permissions would do.
type GrantPreview struct {
	Gains []string `json:"gains"`
	Loses []string `json:"loses"`
	// People is how many it touches: a group counts its members.
	People int `json:"people"`
	// LosesView asks for a confirmation, because someone loses the space.
	LosesView bool `json:"loses_view"`
}

// PreviewGrant says what SetGrants would do, without doing it.
func (s *Service) PreviewGrant(ctx context.Context, c identity.Caller, spaceID uuid.UUID, sub Subject, perms []string) (GrantPreview, error) {
	if err := s.RequireSpace(ctx, s.pool, c.UserID, spaceID, SpaceManageAccess); err != nil {
		return GrantPreview{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT permission FROM space_grants
		WHERE space_id = $1 AND subject_type = $2 AND subject_id = $3`, spaceID, sub.Type, sub.ID)
	if err != nil {
		return GrantPreview{}, err
	}
	before, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return GrantPreview{}, err
	}
	out := GrantPreview{Gains: []string{}, Loses: []string{}, People: 1}
	for _, p := range perms {
		if !slices.Contains(before, p) {
			out.Gains = append(out.Gains, p)
		}
	}
	for _, p := range before {
		if !slices.Contains(perms, p) {
			out.Loses = append(out.Loses, p)
		}
	}
	out.LosesView = slices.Contains(out.Loses, SpaceView)
	if sub.Type == SubjectGroup {
		err = s.pool.QueryRow(ctx, "SELECT count(*) FROM group_members WHERE group_id = $1", sub.ID).Scan(&out.People)
	}
	return out, err
}

// PermissionSet is a named set of space permissions for the whole instance. It
// only fills in boxes, like the built-in ones, and adds no rule.
type PermissionSet struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Permissions []string  `json:"permissions"`
}

// PermissionSets lists the sets administrators made.
func (s *Service) PermissionSets(ctx context.Context) ([]PermissionSet, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, permissions FROM permission_sets ORDER BY lower(name)")
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PermissionSet, error) {
		var v PermissionSet
		err := r.Scan(&v.ID, &v.Name, &v.Permissions)
		return v, err
	})
	if out == nil {
		out = []PermissionSet{}
	}
	return out, err
}

// SavePermissionSet adds a set, or replaces the one with the same name.
func (s *Service) SavePermissionSet(ctx context.Context, c identity.Caller, name string, perms []string) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxSetName || strings.ContainsAny(name, "\r\n\x00") {
		return apperr.Invalid("A set needs a name of at most 100 characters.")
	}
	for _, p := range perms {
		if !slices.Contains(AllSpacePermissions, p) {
			return apperr.Invalid("Unknown permission.")
		}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "UPDATE permission_sets SET permissions = $2 WHERE lower(name) = lower($1)", name, perms)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx, "INSERT INTO permission_sets (id, name, permissions) VALUES ($1, $2, $3)", ids.New(), name, perms); err != nil {
				return err
			}
		}
		return recordChange(ctx, tx, c, "permission_set.saved", "permission_set", name, "", nil)
	})
}

// DeletePermissionSet removes a set. No grant changes: a set only filled in boxes.
func (s *Service) DeletePermissionSet(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM permission_sets WHERE id = $1", id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return recordChange(ctx, tx, c, "permission_set.deleted", "permission_set", id.String(), "", nil)
	})
}

// GlobalHolder is who holds a global permission, and how.
type GlobalHolder struct {
	Permission string `json:"permission"`
	Type       string `json:"type"`
	Name       string `json:"name"`
}

// GlobalHolders lists who holds each global permission, directly or as a group.
func (s *Service) GlobalHolders(ctx context.Context) ([]GlobalHolder, error) {
	rows, err := s.pool.Query(ctx, `SELECT g.permission, g.subject_type, COALESCE(u.display_name, gr.name, '')
		FROM global_grants g
		LEFT JOIN users u ON g.subject_type = 'user' AND u.id = g.subject_id
		LEFT JOIN groups gr ON g.subject_type = 'group' AND gr.id = g.subject_id
		ORDER BY g.permission, 3 LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (GlobalHolder, error) {
		var h GlobalHolder
		err := r.Scan(&h.Permission, &h.Type, &h.Name)
		return h, err
	})
	if out == nil {
		out = []GlobalHolder{}
	}
	return out, err
}

// GrantGuest gives a person the viewer set on one space, for an accepted invitation.
func GrantGuest(ctx context.Context, tx pgx.Tx, spaceID, userID uuid.UUID) error {
	return ApplyTemplateTx(ctx, tx, spaceID, Subject{Type: SubjectUser, ID: userID}, TemplateViewer)
}

// GroupImpact says what deleting a group touches, and whether it would leave a
// space with nobody who administers it.
func (s *Service) GroupImpact(ctx context.Context, q db.Querier, groupID uuid.UUID) (GroupImpact, error) {
	var out GroupImpact
	rows, err := q.Query(ctx, `SELECT DISTINCT sp.name FROM space_grants g JOIN spaces sp ON sp.id = g.space_id
		WHERE g.subject_type = 'group' AND g.subject_id = $1 ORDER BY sp.name LIMIT 200`, groupID)
	if err != nil {
		return out, err
	}
	if out.Spaces, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return out, err
	}
	if out.Spaces == nil {
		out.Spaces = []string{}
	}
	if err := q.QueryRow(ctx, "SELECT count(*) FROM page_restrictions WHERE subject_type = 'group' AND subject_id = $1", groupID).Scan(&out.Restrictions); err != nil {
		return out, err
	}
	err = q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM space_grants g WHERE g.subject_type = 'group' AND g.subject_id = $1
			AND g.permission = 'space.administer' AND NOT EXISTS (SELECT 1 FROM space_grants o WHERE o.space_id = g.space_id
				AND o.permission = 'space.administer' AND NOT (o.subject_type = 'group' AND o.subject_id = $1)))`, groupID).Scan(&out.LeavesSpaceWithoutAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	return out, err
}

// GroupImpact is what deleting a group touches.
type GroupImpact struct {
	Spaces                  []string `json:"spaces"`
	Restrictions            int      `json:"restrictions"`
	LeavesSpaceWithoutAdmin bool     `json:"leaves_space_without_admin"`
}
