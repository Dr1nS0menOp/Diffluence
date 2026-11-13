// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
)

// Group is a set of users that can hold permissions.
type Group struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Source            string    `json:"source"`
	ExternalID        string    `json:"external_id"`
	Members           int       `json:"members"`
	GlobalPermissions []string  `json:"global_permissions"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

const groupSQL = `SELECT g.id, g.name, g.description, g.source, g.external_id,
		(SELECT count(*) FROM group_members m WHERE m.group_id = g.id),
		COALESCE((SELECT array_agg(p.permission ORDER BY p.permission) FROM global_grants p
			WHERE p.subject_type = 'group' AND p.subject_id = g.id), '{}'),
		g.created_at, g.updated_at
	FROM groups g`

func scanGroup(r pgx.Row) (Group, error) {
	var g Group
	err := r.Scan(&g.ID, &g.Name, &g.Description, &g.Source, &g.ExternalID, &g.Members, &g.GlobalPermissions, &g.CreatedAt, &g.UpdatedAt)
	return g, err
}

// GroupFilter selects groups. Empty fields do not filter.
type GroupFilter struct {
	Name       string // exact, ignoring case
	ExternalID string
	Limit      int
	Offset     int
}

// ListGroups returns groups, and how many match in total.
func (s *Service) ListGroups(ctx context.Context, f GroupFilter) ([]Group, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	const where = ` WHERE ($1 = '' OR lower(g.name) = lower($1)) AND ($2 = '' OR g.external_id = $2)`
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM groups g"+where, f.Name, f.ExternalID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, groupSQL+where+" ORDER BY lower(g.name), g.id LIMIT $3 OFFSET $4", f.Name, f.ExternalID, f.Limit, max(f.Offset, 0))
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Group, error) { return scanGroup(r) })
	if out == nil {
		out = []Group{}
	}
	return out, total, err
}

// GetGroup returns one group.
func (s *Service) GetGroup(ctx context.Context, q db.Querier, id uuid.UUID) (Group, error) {
	g, err := scanGroup(q.QueryRow(ctx, groupSQL+" WHERE g.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, apperr.NotFound()
	}
	return g, err
}

func cleanGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 200 || strings.ContainsAny(name, "\r\n\x00") {
		return "", apperr.Invalid("A group needs a name of at most 200 characters.")
	}
	return name, nil
}

// NewGroup is the input for CreateGroup.
type NewGroup struct {
	Name        string
	Description string
	Source      string
	ExternalID  string
}

// CreateGroup adds a group.
func (s *Service) CreateGroup(ctx context.Context, c Caller, in NewGroup) (Group, error) {
	name, err := cleanGroupName(in.Name)
	if err != nil {
		return Group{}, err
	}
	if in.Source == "" {
		in.Source = SourceLocal
	}
	id := ids.New()
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO groups (id, origin_instance_id, name, description, source, external_id)
			VALUES ($1, $2, $3, $4, $5, $6)`, id, s.instanceID, name, strings.TrimSpace(in.Description), in.Source, in.ExternalID)
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("name_taken", "A group with this name already exists.")
		}
		if err != nil {
			return err
		}
		return s.record(ctx, tx, c, "group.created", "group", id.String(), map[string]any{"name": name, "source": in.Source})
	})
	if err != nil {
		return Group{}, err
	}
	return s.GetGroup(ctx, s.pool, id)
}

// managedGuard stops people from changing groups that an identity provider owns:
// the next sync would undo it, and it would look as if it had worked.
func (s *Service) managedGuard(ctx context.Context, q db.Querier, c Caller, id uuid.UUID) error {
	g, err := s.GetGroup(ctx, q, id)
	if err != nil {
		return err
	}
	if c.System == "" && g.Source != SourceLocal {
		return apperr.Conflict("group_managed", "This group is managed by your identity provider.")
	}
	return nil
}

// UpdateGroup changes a group's name and description.
func (s *Service) UpdateGroup(ctx context.Context, c Caller, id uuid.UUID, in NewGroup) (Group, error) {
	name, err := cleanGroupName(in.Name)
	if err != nil {
		return Group{}, err
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.managedGuard(ctx, tx, c, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE groups SET name = $2, description = $3, external_id = COALESCE(NULLIF($4, ''), external_id), updated_at = now() WHERE id = $1",
			id, name, strings.TrimSpace(in.Description), in.ExternalID)
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("name_taken", "A group with this name already exists.")
		}
		if err != nil {
			return err
		}
		return s.record(ctx, tx, c, "group.updated", "group", id.String(), map[string]any{"name": name})
	})
	if err != nil {
		return Group{}, err
	}
	return s.GetGroup(ctx, s.pool, id)
}

// DeleteGroup removes a group and its grants. Page restrictions keep the entry on
// purpose: a page restricted to only this group must stay closed, not fall open.
// A space admin can replace the entry, and an administrator can recover access.
func (s *Service) DeleteGroup(ctx context.Context, c Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.managedGuard(ctx, tx, c, id); err != nil {
			return err
		}
		if err := s.mayActOnGroup(ctx, tx, c, id); err != nil {
			return err
		}
		// Its roles go, everywhere, and with them every grant they added (spec 036).
		if _, err := tx.Exec(ctx, "DELETE FROM role_assignments WHERE subject_type = 'group' AND subject_id = $1", id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM groups WHERE id = $1", id); err != nil {
			return err
		}
		if err := s.keepsAnAdministrator(ctx, tx, c); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "group.deleted", "group", id.String(), nil)
	})
}

// MemberGroup is a group as its member sees it.
type MemberGroup struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Owner bool      `json:"owner"`
}

// GroupsOf lists the groups a person is in or owns, for their own profile.
func (s *Service) GroupsOf(ctx context.Context, userID uuid.UUID) ([]MemberGroup, error) {
	rows, err := s.pool.Query(ctx, `SELECT g.id, g.name, EXISTS (SELECT 1 FROM group_owners o WHERE o.group_id = g.id AND o.user_id = $1)
		FROM groups g
		WHERE EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = g.id AND m.user_id = $1)
		   OR EXISTS (SELECT 1 FROM group_owners o WHERE o.group_id = g.id AND o.user_id = $1)
		ORDER BY lower(g.name)`, userID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[MemberGroup])
	if out == nil {
		out = []MemberGroup{}
	}
	return out, err
}

// SetGroupOwners chooses who may manage a group's members. The route requires
// administrator.
func (s *Service) SetGroupOwners(ctx context.Context, c Caller, id uuid.UUID, owners []uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := s.GetGroup(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM group_owners WHERE group_id = $1", id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO group_owners (group_id, user_id)
			SELECT $1, u.id FROM users u WHERE u.id = ANY($2)`, id, owners); err != nil {
			return err
		}
		return s.record(ctx, tx, c, "group.owners_changed", "group", id.String(), map[string]any{"owners": len(owners)})
	})
}

// requireOwner answers "not found" for anyone who does not own the group, so the
// route cannot be used to test which groups exist.
func (s *Service) requireOwner(ctx context.Context, q db.Querier, c Caller, id uuid.UUID) error {
	var ok bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM group_owners WHERE group_id = $1 AND user_id = $2)", id, c.UserID).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.NotFound()
	}
	return nil
}

// OwnedGroup is a group as its owner sees it.
func (s *Service) OwnedGroup(ctx context.Context, c Caller, id uuid.UUID) (Group, []GroupMember, error) {
	if err := s.requireOwner(ctx, s.pool, c, id); err != nil {
		return Group{}, nil, err
	}
	g, err := s.GetGroup(ctx, s.pool, id)
	if err != nil {
		return g, nil, err
	}
	members, err := s.GroupMembers(ctx, id)
	return g, members, err
}

// OwnerAddMember adds a member, as the owner of a group. A group from an identity
// provider stays read-only for them too: the provider is the source of truth.
func (s *Service) OwnerAddMember(ctx context.Context, c Caller, groupID, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireOwner(ctx, tx, c, groupID); err != nil {
			return err
		}
		if err := s.managedGuard(ctx, tx, c, groupID); err != nil {
			return err
		}
		return s.addMember(ctx, tx, c, groupID, userID)
	})
}

// OwnerRemoveMember removes a member, as the owner of a group.
func (s *Service) OwnerRemoveMember(ctx context.Context, c Caller, groupID, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.requireOwner(ctx, tx, c, groupID); err != nil {
			return err
		}
		if err := s.managedGuard(ctx, tx, c, groupID); err != nil {
			return err
		}
		return s.removeMember(ctx, tx, c, groupID, userID)
	})
}

// GroupMember is a member of a group.
type GroupMember struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
}

// GroupMembers lists a group's members.
func (s *Service) GroupMembers(ctx context.Context, id uuid.UUID) ([]GroupMember, error) {
	if _, err := s.GetGroup(ctx, s.pool, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.email, u.display_name FROM group_members m
		JOIN users u ON u.id = m.user_id WHERE m.group_id = $1 ORDER BY lower(u.display_name)`, id)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (GroupMember, error) {
		var m GroupMember
		err := r.Scan(&m.ID, &m.Email, &m.DisplayName)
		return m, err
	})
	if out == nil {
		out = []GroupMember{}
	}
	return out, err
}

// AddGroupMember adds a user to a group.
func (s *Service) AddGroupMember(ctx context.Context, c Caller, groupID, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.managedGuard(ctx, tx, c, groupID); err != nil {
			return err
		}
		return s.addMember(ctx, tx, c, groupID, userID)
	})
}

func (s *Service) addMember(ctx context.Context, tx pgx.Tx, c Caller, groupID, userID uuid.UUID) error {
	if err := s.mayActOnGroup(ctx, tx, c, groupID); err != nil {
		return err
	}
	if _, err := s.GetUser(ctx, tx, userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", groupID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	return s.record(ctx, tx, c, "group.member_added", "group", groupID.String(), map[string]any{"user_id": userID})
}

// RemoveGroupMember removes a user from a group.
func (s *Service) RemoveGroupMember(ctx context.Context, c Caller, groupID, userID uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.managedGuard(ctx, tx, c, groupID); err != nil {
			return err
		}
		return s.removeMember(ctx, tx, c, groupID, userID)
	})
}

func (s *Service) removeMember(ctx context.Context, tx pgx.Tx, c Caller, groupID, userID uuid.UUID) error {
	// Every way out of a group, by an administrator or by its owner, passes here. Taking
	// someone out can take their access away, so it counts as acting on their account.
	// That also covers a group that holds system administration: each of its members
	// holds it through the group. Adding only gives, so addMember checks the group.
	if err := s.mayActOn(ctx, tx, c, userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "DELETE FROM group_members WHERE group_id = $1 AND user_id = $2", groupID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	if err := s.keepsAnAdministrator(ctx, tx, c); err != nil {
		return err
	}
	return s.record(ctx, tx, c, "group.member_removed", "group", groupID.String(), map[string]any{"user_id": userID})
}

// SetGroupMembers makes the membership exactly this list.
func (s *Service) SetGroupMembers(ctx context.Context, c Caller, groupID uuid.UUID, want []uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.managedGuard(ctx, tx, c, groupID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT user_id FROM group_members WHERE group_id = $1", groupID)
		if err != nil {
			return err
		}
		current, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return err
		}
		keep := map[uuid.UUID]bool{}
		for _, id := range want {
			keep[id] = true
			if err := s.addMember(ctx, tx, c, groupID, id); err != nil {
				return err
			}
		}
		for _, id := range current {
			if !keep[id] {
				if err := s.removeMember(ctx, tx, c, groupID, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// SyncExternalGroups makes a user's membership of groups from source match the
// names given, creating groups when needed. Groups from other sources stay as
// they are. Used for the OIDC groups claim.
func (s *Service) SyncExternalGroups(ctx context.Context, tx pgx.Tx, c Caller, userID uuid.UUID, source string, names []string) error {
	want := map[uuid.UUID]bool{}
	for _, raw := range names {
		name, err := cleanGroupName(raw)
		if err != nil {
			continue // a strange claim value must not block the login
		}
		var id uuid.UUID
		var groupSource string
		err = tx.QueryRow(ctx, "SELECT id, source FROM groups WHERE lower(name) = lower($1)", name).Scan(&id, &groupSource)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			id = ids.New()
			if _, err := tx.Exec(ctx, `INSERT INTO groups (id, origin_instance_id, name, source) VALUES ($1, $2, $3, $4)`,
				id, s.instanceID, name, source); err != nil {
				return err
			}
			if err := s.record(ctx, tx, c, "group.created", "group", id.String(), map[string]any{"name": name, "source": source}); err != nil {
				return err
			}
		case err != nil:
			return err
		case groupSource != source:
			continue // a claim may not add people to a group someone else manages
		}
		want[id] = true
		if err := s.addMember(ctx, tx, c, id, userID); err != nil {
			return err
		}
	}
	rows, err := tx.Query(ctx, `SELECT m.group_id FROM group_members m JOIN groups g ON g.id = m.group_id
		WHERE m.user_id = $1 AND g.source = $2`, userID, source)
	if err != nil {
		return err
	}
	current, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, id := range current {
		if !want[id] {
			if err := s.removeMember(ctx, tx, c, id, userID); err != nil {
				return err
			}
		}
	}
	return nil
}
