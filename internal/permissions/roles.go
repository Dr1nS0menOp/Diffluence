// SPDX-License-Identifier: Apache-2.0

package permissions

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

// Scopes of a role (spec 036).
const (
	ScopeSpace    = "space"
	ScopeInstance = "instance"
)

const (
	maxRoleName        = 100
	maxRoleDescription = 500
)

// Role is a named set of permissions, for a space or for the instance. People and
// groups get roles; the grants that permissions.Evaluate reads are what their roles
// add up to, kept equal by the database (migration 0036).
type Role struct {
	ID          uuid.UUID `json:"id"`
	Scope       string    `json:"scope"`
	BuiltIn     string    `json:"built_in,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	// InUse is how many assignments hold the role.
	InUse int `json:"in_use"`
	// Unnamed marks a combination of single permissions that nobody named yet: its name is
	// the one the database gave it (UnnamedRoleName).
	Unnamed bool `json:"unnamed"`
}

// unnamedRolePattern matches the names rbac_unnamed_role_name gives (migration 0044).
var unnamedRolePattern = regexp.MustCompile(`^Custom combination ([0-9]+) \(([0-9]+) permissions?\)$`)

// UnnamedRoleName is the name of a combination nobody named: a number that stays with the
// role and how many permissions it holds, never cut off (ADR-040). The database gives the
// same name; a test holds the two equal.
func UnnamedRoleName(number, permissionCount int) string {
	word := "permissions"
	if permissionCount == 1 {
		word = "permission"
	}
	return fmt.Sprintf("Custom combination %d (%d %s)", number, permissionCount, word)
}

// unnamedNumber gives the number of an unnamed role's name, or false for any other name.
func unnamedNumber(name string) (int, bool) {
	m := unnamedRolePattern.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// RoleInput is what an administrator sets on a role.
type RoleInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// RoleImpact is what a change to a role would do to the people who hold it.
type RoleImpact struct {
	Assignments int      `json:"assignments"`
	Spaces      int      `json:"spaces"`
	People      int      `json:"people"`
	Gains       []string `json:"gains"`
	Loses       []string `json:"loses"`
}

// permissionsOf lists what a role of a scope may hold.
func permissionsOf(scope string) []string {
	if scope == ScopeInstance {
		return identity.AllGlobalPermissions
	}
	return AllSpacePermissions
}

// requireAdministrator holds the rule that only administrators make and change roles.
// A system caller, the command line, may as well.
func (s *Service) requireAdministrator(ctx context.Context, q db.Querier, c identity.Caller) error {
	if c.System != "" {
		return nil
	}
	u, err := s.LoadUser(ctx, q, c.UserID)
	if err != nil {
		return err
	}
	if !u.Active || !slices.Contains(u.Global, identity.PermAdministrator) {
		return apperr.Forbidden()
	}
	return nil
}

func (s *Service) isSystemAdministrator(ctx context.Context, q db.Querier, c identity.Caller) (bool, error) {
	if c.System != "" {
		return true, nil
	}
	u, err := s.LoadUser(ctx, q, c.UserID)
	if err != nil {
		return false, err
	}
	return u.Active && slices.Contains(u.Global, identity.PermSystemAdministrator), nil
}

// cleanRole checks a role's name, description and permissions, and returns the
// permissions sorted and without repeats.
func cleanRole(scope string, in RoleInput) (RoleInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxRoleName || strings.ContainsAny(in.Name, "\r\n\t\x00") {
		return in, apperr.Invalid("A role needs a name of at most 100 characters, on one line.")
	}
	if utf8.RuneCountInString(in.Description) > maxRoleDescription || strings.ContainsRune(in.Description, 0) {
		return in, apperr.Invalid("A description has at most 500 characters.")
	}
	allowed := permissionsOf(scope)
	for _, p := range in.Permissions {
		if !slices.Contains(allowed, p) {
			return in, apperr.Invalid("Unknown permission for this kind of role.")
		}
	}
	in.Permissions = slices.Compact(slices.Sorted(slices.Values(in.Permissions)))
	if len(in.Permissions) == 0 {
		return in, apperr.Invalid("A role needs at least one permission.")
	}
	return in, nil
}

// Roles lists the roles of a scope. Their names and permissions are not secret: anyone
// who uses the product may read them, so a space manager can choose one.
func (s *Service) Roles(ctx context.Context, c identity.Caller, scope string) ([]Role, error) {
	if scope != ScopeSpace && scope != ScopeInstance {
		return nil, apperr.Invalid("A role is for a space or for the instance.")
	}
	u, err := s.LoadUser(ctx, s.pool, c.UserID)
	if err != nil {
		return nil, err
	}
	if !u.Active || !slices.Contains(u.Global, globalUseProduct) {
		return nil, apperr.Forbidden()
	}
	return s.roles(ctx, s.pool, scope)
}

func (s *Service) roles(ctx context.Context, q db.Querier, scope string) ([]Role, error) {
	rows, err := q.Query(ctx, `SELECT r.id, r.scope, COALESCE(r.builtin, ''), r.name, r.description,
			ARRAY(SELECT p FROM unnest(r.permissions) p ORDER BY 1),
			(SELECT count(*) FROM role_assignments a WHERE a.role_id = r.id)
		FROM roles r WHERE r.scope = $1 ORDER BY r.builtin IS NULL, lower(r.name)`, scope)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, scanRole)
	if out == nil {
		out = []Role{}
	}
	return out, err
}

func scanRole(r pgx.CollectableRow) (Role, error) {
	var v Role
	err := r.Scan(&v.ID, &v.Scope, &v.BuiltIn, &v.Name, &v.Description, &v.Permissions, &v.InUse)
	_, v.Unnamed = unnamedNumber(v.Name)
	v.Unnamed = v.Unnamed && v.BuiltIn == ""
	return v, err
}

func (s *Service) roleForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (Role, error) {
	rows, err := tx.Query(ctx, `SELECT r.id, r.scope, COALESCE(r.builtin, ''), r.name, r.description,
			ARRAY(SELECT p FROM unnest(r.permissions) p ORDER BY 1),
			(SELECT count(*) FROM role_assignments a WHERE a.role_id = r.id)
		FROM roles r WHERE r.id = $1 FOR UPDATE`, id)
	if err != nil {
		return Role{}, err
	}
	v, err := pgx.CollectExactlyOneRow(rows, scanRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, apperr.NotFound()
	}
	return v, err
}

func roleNameTaken(err error) error {
	if db.IsUniqueViolation(err) {
		return apperr.Conflict("name_taken", "Another role already has this name.")
	}
	return err
}

// CreateRole makes a role. A role with system_administrator needs a system
// administrator, as that permission does (PRM-008).
func (s *Service) CreateRole(ctx context.Context, c identity.Caller, scope string, in RoleInput) (Role, error) {
	if scope != ScopeSpace && scope != ScopeInstance {
		return Role{}, apperr.Invalid("A role is for a space or for the instance.")
	}
	in, err := cleanRole(scope, in)
	if err != nil {
		return Role{}, err
	}
	out := Role{ID: ids.New(), Scope: scope, Name: in.Name, Description: in.Description, Permissions: in.Permissions}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireAdministrator(ctx, tx, c); err != nil {
			return err
		}
		if err := s.maySystemAdministrator(ctx, tx, c, nil, in.Permissions); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO roles (id, scope, name, description, permissions) VALUES ($1, $2, $3, $4, $5)`,
			out.ID, scope, in.Name, in.Description, in.Permissions); err != nil {
			return roleNameTaken(err)
		}
		return recordChange(ctx, tx, c, "role.created", "role", out.ID.String(), "", map[string]any{
			"scope": scope, "name": in.Name, "permissions": in.Permissions})
	})
	return out, err
}

func (s *Service) maySystemAdministrator(ctx context.Context, q db.Querier, c identity.Caller, before, after []string) error {
	if slices.Contains(before, identity.PermSystemAdministrator) == slices.Contains(after, identity.PermSystemAdministrator) {
		return nil
	}
	ok, err := s.isSystemAdministrator(ctx, q, c)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("needs_system_administrator", "Only a system administrator can change this permission.")
	}
	return nil
}

func difference(a, b []string) []string {
	out := []string{}
	for _, p := range a {
		if !slices.Contains(b, p) {
			out = append(out, p)
		}
	}
	return out
}

func (s *Service) impact(ctx context.Context, q db.Querier, r Role, perms []string) (RoleImpact, error) {
	out := RoleImpact{Gains: difference(perms, r.Permissions), Loses: difference(r.Permissions, perms)}
	// A group counts its members, and everyone of a kind counts every such person.
	err := q.QueryRow(ctx, `SELECT count(*), count(DISTINCT a.space_id), (
			SELECT count(DISTINCT u.id) FROM role_assignments x JOIN users u ON u.status = 'active' AND (
				(x.subject_type = 'user' AND u.id = x.subject_id)
				OR (x.subject_type = 'group' AND u.id IN (SELECT user_id FROM group_members WHERE group_id = x.subject_id))
				OR (x.subject_type = 'all_members' AND u.type = 'member')
				OR (x.subject_type = 'all_guests' AND u.type = 'guest'))
			WHERE x.role_id = $1)
		FROM role_assignments a WHERE a.role_id = $1`, r.ID).Scan(&out.Assignments, &out.Spaces, &out.People)
	return out, err
}

// PreviewRole says what changing a role's permissions would do, without doing it.
func (s *Service) PreviewRole(ctx context.Context, c identity.Caller, id uuid.UUID, perms []string) (RoleImpact, error) {
	var out RoleImpact
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireAdministrator(ctx, tx, c); err != nil {
			return err
		}
		r, err := s.roleForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		in, err := cleanRole(r.Scope, RoleInput{Name: r.Name, Permissions: perms})
		if err != nil {
			return err
		}
		out, err = s.impact(ctx, tx, r, in.Permissions)
		return err
	})
	return out, err
}

// UpdateRole renames a role or changes its permissions. A built-in role keeps its
// meaning. A change that touches people who hold the role needs confirm, after a
// preview, so nobody widens or narrows access without seeing who it reaches. A role
// in use cannot gain space.administer: who administers a space is chosen per space.
func (s *Service) UpdateRole(ctx context.Context, c identity.Caller, id uuid.UUID, in RoleInput, confirm bool) (Role, error) {
	var out Role
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireAdministrator(ctx, tx, c); err != nil {
			return err
		}
		r, err := s.roleForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if r.BuiltIn != "" {
			return apperr.Conflict("built_in", "A built-in role keeps its meaning. Make a new role instead.")
		}
		in, err := cleanRole(r.Scope, in)
		if err != nil {
			return err
		}
		if err := s.maySystemAdministrator(ctx, tx, c, r.Permissions, in.Permissions); err != nil {
			return err
		}
		changed := !slices.Equal(r.Permissions, in.Permissions)
		// An unnamed combination that keeps its given name keeps telling how many
		// permissions it holds.
		if n, unnamed := unnamedNumber(r.Name); unnamed && changed && in.Name == r.Name {
			in.Name = UnnamedRoleName(n, len(in.Permissions))
		}
		if changed && r.InUse > 0 {
			if r.Scope == ScopeSpace && slices.Contains(difference(in.Permissions, r.Permissions), SpaceAdminister) {
				return apperr.Conflict("administer_in_use", "A role in use cannot gain space administration. Make someone a space admin in that space instead.")
			}
			if !confirm {
				return apperr.Conflict("confirm_needed", "This role is in use. Look at the preview, then confirm.")
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE roles SET name = $2, description = $3, permissions = $4, updated_at = now() WHERE id = $1`,
			id, in.Name, in.Description, in.Permissions); err != nil {
			return roleNameTaken(err)
		}
		if changed {
			if err := s.keepsAdminsAfterRoleChange(ctx, tx, c, r); err != nil {
				return err
			}
		}
		_, unnamed := unnamedNumber(in.Name)
		out = Role{ID: id, Scope: r.Scope, Name: in.Name, Description: in.Description, Permissions: in.Permissions, InUse: r.InUse,
			Unnamed: unnamed}
		return recordChange(ctx, tx, c, "role.changed", "role", id.String(), "", map[string]any{
			"before":  map[string]any{"name": r.Name, "permissions": r.Permissions},
			"after":   map[string]any{"name": in.Name, "permissions": in.Permissions},
			"held_by": r.InUse,
		})
	})
	return out, err
}

// keepsAdminsAfterRoleChange stops a role change from leaving a space without an
// administrator, or the instance without one.
func (s *Service) keepsAdminsAfterRoleChange(ctx context.Context, tx pgx.Tx, c identity.Caller, r Role) error {
	if r.Scope == ScopeInstance {
		return keepsAnAdministrator(ctx, tx, c)
	}
	rows, err := tx.Query(ctx, "SELECT DISTINCT space_id FROM role_assignments WHERE role_id = $1", r.ID)
	if err != nil {
		return err
	}
	spaces, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, sp := range spaces {
		if err := s.keepsAnAdmin(ctx, tx, sp); err != nil {
			return err
		}
	}
	return nil
}

// keepsAnAdministrator holds PRM-007 for roles: one active administrator remains.
func keepsAnAdministrator(ctx context.Context, tx pgx.Tx, c identity.Caller) error {
	if c.System != "" {
		return nil
	}
	var ok bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users u
		WHERE u.status = 'active' AND perm_has_global(u.id, 'administrator'))`).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("last_administrator", "At least one active administrator must remain.")
	}
	return nil
}

// DeleteRole removes a role nobody holds. A role in use is refused rather than taken
// from people quietly: take it away from them first, where they can be seen.
func (s *Service) DeleteRole(ctx context.Context, c identity.Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireAdministrator(ctx, tx, c); err != nil {
			return err
		}
		r, err := s.roleForUpdate(ctx, tx, id)
		if err != nil {
			return err
		}
		if r.BuiltIn != "" {
			return apperr.Conflict("built_in", "A built-in role cannot be deleted.")
		}
		if r.InUse > 0 {
			return apperr.Conflict("role_in_use", "This role is in use. Give the people who hold it another role first.")
		}
		if _, err := tx.Exec(ctx, "DELETE FROM roles WHERE id = $1", id); err != nil {
			return err
		}
		return recordChange(ctx, tx, c, "role.deleted", "role", id.String(), "", map[string]any{
			"name": r.Name, "scope": r.Scope, "permissions": r.Permissions})
	})
}

// RoleRef names a role someone holds.
type RoleRef struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// Assignment is who holds which roles, in a space or for the instance.
type Assignment struct {
	Subject Subject   `json:"subject"`
	Name    string    `json:"name"`
	Roles   []RoleRef `json:"roles"`
}

func (s *Service) assignments(ctx context.Context, q db.Querier, spaceID *uuid.UUID) ([]Assignment, error) {
	rows, err := q.Query(ctx, `SELECT a.subject_type, a.subject_id, COALESCE(u.display_name, gr.name, ''),
			array_agg(r.id ORDER BY lower(r.name)), array_agg(r.name ORDER BY lower(r.name))
		FROM role_assignments a JOIN roles r ON r.id = a.role_id
		LEFT JOIN users u ON a.subject_type = 'user' AND u.id = a.subject_id
		LEFT JOIN groups gr ON a.subject_type = 'group' AND gr.id = a.subject_id
		WHERE a.space_id IS NOT DISTINCT FROM $1
		GROUP BY a.subject_type, a.subject_id, u.display_name, gr.name
		ORDER BY a.subject_type, 3`, spaceID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Assignment, error) {
		var v Assignment
		var idList []uuid.UUID
		var names []string
		if err := r.Scan(&v.Subject.Type, &v.Subject.ID, &v.Name, &idList, &names); err != nil {
			return v, err
		}
		for i := range idList {
			v.Roles = append(v.Roles, RoleRef{ID: idList[i], Name: names[i]})
		}
		return v, nil
	})
	if out == nil {
		out = []Assignment{}
	}
	return out, err
}

// maxHoldersListed caps the list of holders of one role; the total says how many more
// there are, so a role held in thousands of spaces stays one cheap page (ARC-238).
const maxHoldersListed = 200

// RoleHolder is someone who holds a role, and the space they hold it in. SpaceID is nil
// for an instance role.
type RoleHolder struct {
	Subject   Subject    `json:"subject"`
	Name      string     `json:"name"`
	SpaceID   *uuid.UUID `json:"space_id,omitempty"`
	SpaceName string     `json:"space_name,omitempty"`
}

// RoleHolders lists who holds a role and where, for administrators, with the total.
func (s *Service) RoleHolders(ctx context.Context, c identity.Caller, roleID uuid.UUID) ([]RoleHolder, int, error) {
	if err := s.requireAdministrator(ctx, s.pool, c); err != nil {
		return nil, 0, err
	}
	var total int
	err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM role_assignments WHERE role_id = $1)
		WHERE EXISTS (SELECT 1 FROM roles WHERE id = $1)`, roleID).Scan(&total)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, apperr.NotFound()
	}
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, `SELECT a.subject_type, a.subject_id, COALESCE(u.display_name, gr.name, ''),
			a.space_id, COALESCE(sp.name, '')
		FROM role_assignments a
		LEFT JOIN users u ON a.subject_type = 'user' AND u.id = a.subject_id
		LEFT JOIN groups gr ON a.subject_type = 'group' AND gr.id = a.subject_id
		LEFT JOIN spaces sp ON sp.id = a.space_id
		WHERE a.role_id = $1
		ORDER BY lower(COALESCE(sp.name, '')), a.subject_type, lower(COALESCE(u.display_name, gr.name, ''))
		LIMIT $2`, roleID, maxHoldersListed)
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (RoleHolder, error) {
		var h RoleHolder
		err := r.Scan(&h.Subject.Type, &h.Subject.ID, &h.Name, &h.SpaceID, &h.SpaceName)
		return h, err
	})
	if out == nil {
		out = []RoleHolder{}
	}
	return out, total, err
}

// SpaceAssignments lists who holds which roles in a space, for those who manage access.
func (s *Service) SpaceAssignments(ctx context.Context, c identity.Caller, spaceID uuid.UUID) ([]Assignment, error) {
	if err := s.RequireSpace(ctx, s.pool, c.UserID, spaceID, SpaceManageAccess); err != nil {
		return nil, err
	}
	return s.assignments(ctx, s.pool, &spaceID)
}

// InstanceAssignments lists who holds which instance roles, for administrators.
func (s *Service) InstanceAssignments(ctx context.Context, c identity.Caller) ([]Assignment, error) {
	if err := s.requireAdministrator(ctx, s.pool, c); err != nil {
		return nil, err
	}
	return s.assignments(ctx, s.pool, nil)
}

// rolesByID loads the roles asked for, each of the scope, and fails on any other.
func rolesByID(ctx context.Context, tx pgx.Tx, scope string, roleIDs []uuid.UUID) ([]Role, []string, error) {
	roleIDs = slices.Compact(slices.SortedFunc(slices.Values(roleIDs), func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) }))
	rows, err := tx.Query(ctx, `SELECT id, scope, COALESCE(builtin, ''), name, description,
			ARRAY(SELECT p FROM unnest(permissions) p ORDER BY 1), 0
		FROM roles WHERE id = ANY($1) AND scope = $2`, roleIDs, scope)
	if err != nil {
		return nil, nil, err
	}
	found, err := pgx.CollectRows(rows, scanRole)
	if err != nil {
		return nil, nil, err
	}
	if len(found) != len(roleIDs) {
		return nil, nil, apperr.Invalid("Unknown role for this place.")
	}
	perms := []string{}
	for _, r := range found {
		perms = append(perms, r.Permissions...)
	}
	return found, slices.Compact(slices.Sorted(slices.Values(perms))), nil
}

func replaceAssignments(ctx context.Context, tx pgx.Tx, spaceID *uuid.UUID, sub Subject, roleIDs []uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM role_assignments WHERE space_id IS NOT DISTINCT FROM $1
		AND subject_type = $2 AND subject_id = $3 AND NOT (role_id = ANY($4))`, spaceID, sub.Type, sub.ID, roleIDs); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO role_assignments (role_id, space_id, subject_type, subject_id)
		SELECT unnest($4::uuid[]), $1, $2, $3 ON CONFLICT DO NOTHING`, spaceID, sub.Type, sub.ID, roleIDs)
	return err
}

func roleNames(roles []Role) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Name)
	}
	return out
}

func currentRoleNames(ctx context.Context, tx pgx.Tx, spaceID *uuid.UUID, sub Subject) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT r.name FROM role_assignments a JOIN roles r ON r.id = a.role_id
		WHERE a.space_id IS NOT DISTINCT FROM $1 AND a.subject_type = $2 AND a.subject_id = $3 ORDER BY lower(r.name)`,
		spaceID, sub.Type, sub.ID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// SetSpaceRoles gives a subject exactly these roles in a space. No roles removes the
// subject. The limits of managing access hold as before: only a space administrator
// can make or unmake an administrator, a space keeps one, and the owner of a personal
// space keeps theirs.
func (s *Service) SetSpaceRoles(ctx context.Context, c identity.Caller, spaceID uuid.UUID, sub Subject, roleIDs []uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Access first, so a stranger learns nothing about the space from the answer.
		if err := s.RequireSpace(ctx, tx, c.UserID, spaceID, SpaceManageAccess); err != nil {
			return err
		}
		if !validSpaceSubject(sub) {
			return apperr.Invalid("Choose a user, a group, all members or all guests.")
		}
		if err := s.subjectExists(ctx, tx, sub); err != nil {
			return err
		}
		roles, after, err := rolesByID(ctx, tx, ScopeSpace, roleIDs)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT permission FROM space_grants
			WHERE space_id = $1 AND subject_type = $2 AND subject_id = $3`, spaceID, sub.Type, sub.ID)
		if err != nil {
			return err
		}
		before, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if err := s.mayChangeSubject(ctx, tx, c, spaceID, sub, before, after); err != nil {
			return err
		}
		beforeRoles, err := currentRoleNames(ctx, tx, &spaceID, sub)
		if err != nil {
			return err
		}
		if err := replaceAssignments(ctx, tx, &spaceID, sub, idsOf(roles)); err != nil {
			return err
		}
		if err := s.keepsAnAdmin(ctx, tx, spaceID); err != nil {
			return err
		}
		return recordChange(ctx, tx, c, "role.assigned", sub.Type, sub.ID.String(), spaceID.String(), map[string]any{
			"before": map[string]any{"roles": beforeRoles, "permissions": slices.Sorted(slices.Values(before))},
			"after":  map[string]any{"roles": roleNames(roles), "permissions": after},
		})
	})
}

// PreviewSpaceRoles says what SetSpaceRoles would do, without doing it. A role holds
// exactly its permissions, so unlike PreviewGrant nothing is filled in.
func (s *Service) PreviewSpaceRoles(ctx context.Context, c identity.Caller, spaceID uuid.UUID, sub Subject, roleIDs []uuid.UUID) (GrantPreview, error) {
	var out GrantPreview
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Access first, as in SetSpaceRoles, so a stranger learns nothing from the answer.
		if err := s.RequireSpace(ctx, tx, c.UserID, spaceID, SpaceManageAccess); err != nil {
			return err
		}
		if !validSpaceSubject(sub) {
			return apperr.Invalid("Choose a user, a group, all members or all guests.")
		}
		_, after, err := rolesByID(ctx, tx, ScopeSpace, roleIDs)
		if err != nil {
			return err
		}
		before, err := grantsOf(ctx, tx, spaceID, sub)
		if err != nil {
			return err
		}
		// The limits of SetSpaceRoles hold here too, so a preview never shows a change that cannot happen.
		if err := s.mayChangeSubject(ctx, tx, c, spaceID, sub, before, after); err != nil {
			return err
		}
		out, err = previewAgainst(ctx, tx, sub, before, after)
		return err
	})
	return out, err
}

// SetInstanceRoles gives a user or group exactly these instance roles. Administrators
// do this; system_administrator needs a system administrator, and one active
// administrator remains.
func (s *Service) SetInstanceRoles(ctx context.Context, c identity.Caller, sub Subject, roleIDs []uuid.UUID) error {
	if (sub.Type != SubjectUser && sub.Type != SubjectGroup) || sub.ID == uuid.Nil {
		return apperr.Invalid("Instance roles go to a user or a group.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireAdministrator(ctx, tx, c); err != nil {
			return err
		}
		if err := s.subjectExists(ctx, tx, sub); err != nil {
			return err
		}
		roles, after, err := rolesByID(ctx, tx, ScopeInstance, roleIDs)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT permission FROM global_grants WHERE subject_type = $1 AND subject_id = $2", sub.Type, sub.ID)
		if err != nil {
			return err
		}
		before, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if err := s.maySystemAdministrator(ctx, tx, c, before, after); err != nil {
			return err
		}
		beforeRoles, err := currentRoleNames(ctx, tx, nil, sub)
		if err != nil {
			return err
		}
		if err := replaceAssignments(ctx, tx, nil, sub, idsOf(roles)); err != nil {
			return err
		}
		if err := keepsAnAdministrator(ctx, tx, c); err != nil {
			return err
		}
		return recordChange(ctx, tx, c, "role.assigned", sub.Type, sub.ID.String(), "", map[string]any{
			"before": map[string]any{"roles": beforeRoles, "permissions": slices.Sorted(slices.Values(before))},
			"after":  map[string]any{"roles": roleNames(roles), "permissions": after},
		})
	})
}

func idsOf(roles []Role) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.ID)
	}
	return out
}
