// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/outbox"
	"github.com/diffluence/diffluence/internal/plaintext"
)

// Sources of an account.
const (
	SourceLocal = "local"
	SourceOIDC  = "oidc"
	SourceSCIM  = "scim"
)

// UserDetail is a user as administrators see it.
type UserDetail struct {
	ID          uuid.UUID `json:"id"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Username    string    `json:"username"`
	ExternalID  string    `json:"external_id"`
	Source      string    `json:"source"`
	HasPassword bool      `json:"has_password"`
	TOTPEnabled bool      `json:"totp_enabled"`
	// PasswordLocked says that password login stopped after too many wrong passwords.
	PasswordLocked    bool      `json:"password_locked"`
	Passkeys          int       `json:"passkeys"`
	GlobalPermissions []string  `json:"global_permissions"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	// ExpiresAt is when the account ends, and LastLoginAt when it was last used.
	ExpiresAt   *time.Time `json:"expires_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
	// Spaces are the keys of the spaces a guest can open now. Members leave it empty,
	// because most of them reach most spaces and the list would say nothing (ACC-040).
	Spaces []string `json:"spaces"`
}

const userDetailSQL = `SELECT u.id, u.type, u.status, u.email, u.display_name, u.username, u.external_id, u.source,
		EXISTS (SELECT 1 FROM local_credentials c WHERE c.user_id = u.id),
		EXISTS (SELECT 1 FROM totp_credentials t WHERE t.user_id = u.id AND t.confirmed_at IS NOT NULL),
		EXISTS (SELECT 1 FROM local_credentials c WHERE c.user_id = u.id AND c.password_locked_at IS NOT NULL),
		(SELECT count(*) FROM webauthn_credentials w WHERE w.user_id = u.id),
		COALESCE((SELECT array_agg(g.permission ORDER BY g.permission) FROM global_grants g
			WHERE g.subject_type = 'user' AND g.subject_id = u.id), '{}'),
		u.created_at, u.updated_at, u.expires_at, u.last_login_at,
		CASE WHEN u.type = 'guest' THEN COALESCE((SELECT array_agg(s.key ORDER BY s.key)
			FROM perm_spaces(u.id, 'space.view') ps JOIN spaces s ON s.id = ps.space_id), '{}') ELSE '{}' END
	FROM users u`

func scanUserDetail(r pgx.Row) (UserDetail, error) {
	var u UserDetail
	err := r.Scan(&u.ID, &u.Type, &u.Status, &u.Email, &u.DisplayName, &u.Username, &u.ExternalID, &u.Source,
		&u.HasPassword, &u.TOTPEnabled, &u.PasswordLocked, &u.Passkeys, &u.GlobalPermissions, &u.CreatedAt, &u.UpdatedAt, &u.ExpiresAt, &u.LastLoginAt,
		&u.Spaces)
	return u, err
}

// UserFilter selects users. Empty fields do not filter.
type UserFilter struct {
	Text       string
	Email      string // exact, ignoring case
	Username   string // exact, ignoring case
	ExternalID string // exact
	Type       string
	Status     string
	Source     string
	GroupID    uuid.UUID
	// IdleDays keeps people who have not logged in for this many days.
	IdleDays int
	// Role keeps people who hold this global permission, directly or through a group.
	Role string
	// SpaceID keeps people who can view this space.
	SpaceID uuid.UUID
	Limit   int
	Offset  int
}

// FilterRoles are the global permissions the user list can filter on.
var FilterRoles = []string{PermAdministrator, PermSystemAdministrator, PermCreateSpace, PermInvitePeople}

// ListUsers returns users, and how many match in total.
func (s *Service) ListUsers(ctx context.Context, f UserFilter) ([]UserDetail, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	const where = ` WHERE ($1 = '' OR position(lower($1) IN lower(u.email)) > 0 OR position(lower($1) IN lower(u.display_name)) > 0)
		AND ($2 = '' OR lower(u.email) = lower($2))
		AND ($3 = '' OR lower(u.username) = lower($3))
		AND ($4 = '' OR u.external_id = $4)
		AND ($5 = '' OR u.type = $5)
		AND ($6 = '' OR u.status = $6)
		AND ($7 = '' OR u.source = $7)
		AND ($8::uuid IS NULL OR u.id IN (SELECT m.user_id FROM group_members m WHERE m.group_id = $8))
		AND ($9 = 0 OR u.last_login_at IS NULL OR u.last_login_at < now() - make_interval(days => $9))
		AND ($10 = '' OR perm_has_global(u.id, $10))
		AND ($11::uuid IS NULL OR $11 IN (SELECT ps.space_id FROM perm_spaces(u.id, 'space.view') ps))`
	var group *uuid.UUID
	if f.GroupID != uuid.Nil {
		group = &f.GroupID
	}
	var space *uuid.UUID
	if f.SpaceID != uuid.Nil {
		space = &f.SpaceID
	}
	args := []any{f.Text, f.Email, f.Username, f.ExternalID, f.Type, f.Status, f.Source, group, f.IdleDays, f.Role, space}
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM users u"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, userDetailSQL+where+" ORDER BY lower(u.display_name), u.id LIMIT $12 OFFSET $13",
		append(args, f.Limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (UserDetail, error) { return scanUserDetail(r) })
	if out == nil {
		out = []UserDetail{}
	}
	return out, total, err
}

// GetUser returns one user.
func (s *Service) GetUser(ctx context.Context, q db.Querier, id uuid.UUID) (UserDetail, error) {
	u, err := scanUserDetail(q.QueryRow(ctx, userDetailSQL+" WHERE u.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return u, apperr.NotFound()
	}
	return u, err
}

// Explain turns an error of creating an account into one a person can act on.
func Explain(err error) error { return toInvalid(err) }

func toInvalid(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInvalidInput) {
		return apperr.Invalid(strings.TrimPrefix(err.Error(), ErrInvalidInput.Error()+": "))
	}
	if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooLong) {
		return apperr.Invalid(strings.TrimPrefix(err.Error(), "identity: "))
	}
	if errors.Is(err, ErrEmailTaken) {
		return apperr.Conflict("email_taken", "This e-mail address is already in use.")
	}
	return err
}

// CreateUserAdmin creates a local account on behalf of an administrator.
func (s *Service) CreateUserAdmin(ctx context.Context, c Caller, in NewUser) (UserDetail, error) {
	if len(in.GlobalPermissions) == 0 {
		in.GlobalPermissions = defaultPermissions(in.Type)
	}
	if err := s.mayGrant(ctx, s.pool, c, in.GlobalPermissions, nil); err != nil {
		return UserDetail{}, err
	}
	u, err := s.CreateLocalUser(ctx, in, c.Actor(), c.Meta)
	if err != nil {
		return UserDetail{}, toInvalid(err)
	}
	return s.GetUser(ctx, s.pool, u.ID)
}

func defaultPermissions(userType string) []string {
	if userType == TypeGuest {
		return []string{PermUseProduct}
	}
	return []string{PermUseProduct, PermViewProfiles, PermPersonalSpace}
}

// mayGrant keeps the highest permission in the hands of those who already hold it.
func (s *Service) mayGrant(ctx context.Context, q db.Querier, c Caller, after, before []string) error {
	if c.System != "" || slices.Contains(after, PermSystemAdministrator) == slices.Contains(before, PermSystemAdministrator) {
		return nil
	}
	ok, err := s.HasGlobalPermissionIn(ctx, q, c.UserID, PermSystemAdministrator)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("needs_system_administrator", "Only a system administrator can change this permission.")
	}
	return nil
}

// mayActOn keeps an account that holds system administration in the hands of those who
// hold it too: resetting its sign-in, disabling it, ending it or changing its groups is
// as strong as granting the permission itself, which mayGrant already guards.
func (s *Service) mayActOn(ctx context.Context, q db.Querier, c Caller, target uuid.UUID) error {
	if c.System != "" {
		return nil
	}
	held, err := s.HasGlobalPermissionIn(ctx, q, target, PermSystemAdministrator)
	if err != nil || !held {
		return err
	}
	return s.callerIsSystemAdministrator(ctx, q, c)
}

// mayActOnGroup applies the same rule to a group that holds system administration: a
// member added to it would get the permission.
func (s *Service) mayActOnGroup(ctx context.Context, q db.Querier, c Caller, groupID uuid.UUID) error {
	if c.System != "" {
		return nil
	}
	var held bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM global_grants
		WHERE subject_type = 'group' AND subject_id = $1 AND permission = $2)`, groupID, PermSystemAdministrator).Scan(&held)
	if err != nil || !held {
		return err
	}
	return s.callerIsSystemAdministrator(ctx, q, c)
}

func (s *Service) callerIsSystemAdministrator(ctx context.Context, q db.Querier, c Caller) error {
	ok, err := s.HasGlobalPermissionIn(ctx, q, c.UserID, PermSystemAdministrator)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("needs_system_administrator", "Only a system administrator can do this to an account or group that holds system administration.")
	}
	return nil
}

// keepsAnAdministrator stops people from removing the last administrator by
// accident. It does not apply to identity providers: they are leading, and a sync
// that fails here would lock users out of logging in. If a provider removes the
// last administrator, "diffluence user create-admin" is the way back.
func (s *Service) keepsAnAdministrator(ctx context.Context, tx pgx.Tx, c Caller) error {
	if c.System != "" {
		return nil
	}
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users u
		WHERE u.status = 'active' AND perm_has_global(u.id, 'administrator'))`).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("last_administrator", "At least one active administrator must remain.")
	}
	return nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, c Caller, action, objType, objID string, details map[string]any) error {
	if _, err := audit.Record(ctx, tx, c.Event(action, objType, objID, "", details)); err != nil {
		return err
	}
	_, err := outbox.Enqueue(ctx, tx, action, map[string]any{"object_id": objID})
	return err
}

// UpdateUser is what can be changed about an account.
type UpdateUser struct {
	DisplayName string
	Type        string
}

// UpdateUserDetails changes a name or user type. The route requires administrator.
func (s *Service) UpdateUserDetails(ctx context.Context, c Caller, id uuid.UUID, in UpdateUser) (UserDetail, error) {
	in.DisplayName = strings.TrimSpace(plaintext.WithoutDirectionMarks(in.DisplayName))
	if in.DisplayName == "" || utf8.RuneCountInString(in.DisplayName) > 200 {
		return UserDetail{}, apperr.Invalid("A display name has 1 to 200 characters.")
	}
	if in.Type != TypeMember && in.Type != TypeGuest {
		return UserDetail{}, apperr.Invalid("A user is a member or a guest.")
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.mayActOn(ctx, tx, c, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE users SET display_name = $2, type = $3, updated_at = now() WHERE id = $1", id, in.DisplayName, in.Type)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return s.record(ctx, tx, c, "user.updated", "user", id.String(), map[string]any{"type": in.Type})
	})
	if err != nil {
		return UserDetail{}, err
	}
	return s.GetUser(ctx, s.pool, id)
}

// SetUserStatus enables or disables an account. Disabling ends its sessions at once.
func (s *Service) SetUserStatus(ctx context.Context, c Caller, id uuid.UUID, status string) error {
	if c.System == "" && c.UserID == id && status == StatusDisabled {
		return apperr.Conflict("cannot_disable_self", "You cannot disable your own account.")
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.mayActOn(ctx, tx, c, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "UPDATE users SET status = $2, updated_at = now() WHERE id = $1 AND status <> $2", id, status)
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperr.NotFound()
		}
		if tag.RowsAffected() == 0 {
			return nil // already in this state
		}
		action := "user.enabled"
		if status == StatusDisabled {
			action = "user.disabled"
			if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", id); err != nil {
				return err
			}
			if err := s.keepsAnAdministrator(ctx, tx, c); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, c, action, "user", id.String(), nil)
	})
}

// SetGlobalPermissions replaces what a user or group holds at the global layer.
func (s *Service) SetGlobalPermissions(ctx context.Context, c Caller, subjectType string, subjectID uuid.UUID, perms []string) error {
	if subjectType != "user" && subjectType != "group" {
		return apperr.Invalid("Permissions go to a user or a group.")
	}
	for _, p := range perms {
		if !slices.Contains(AllGlobalPermissions, p) {
			return apperr.Invalid("Unknown permission.")
		}
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		table := map[string]string{"user": "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", "group": "SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1)"}
		var exists bool
		if err := tx.QueryRow(ctx, table[subjectType], subjectID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperr.NotFound()
		}
		rows, err := tx.Query(ctx, "SELECT permission FROM global_grants WHERE subject_type = $1 AND subject_id = $2", subjectType, subjectID)
		if err != nil {
			return err
		}
		before, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if err := s.mayGrant(ctx, tx, c, perms, before); err != nil {
			return err
		}
		// The set becomes one instance role; grants follow from it (spec 036).
		if _, err := tx.Exec(ctx, "SELECT rbac_set_exact(NULL, $1, $2, $3::text[])", subjectType, subjectID, perms); err != nil {
			return err
		}
		if err := s.keepsAnAdministrator(ctx, tx, c); err != nil {
			return err
		}
		for _, p := range perms {
			if !slices.Contains(before, p) {
				if err := s.record(ctx, tx, c, "permission.global.granted", subjectType, subjectID.String(), map[string]any{"permission": p}); err != nil {
					return err
				}
			}
		}
		for _, p := range before {
			if !slices.Contains(perms, p) {
				if err := s.record(ctx, tx, c, "permission.global.revoked", subjectType, subjectID.String(), map[string]any{"permission": p}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Service) storePassword(ctx context.Context, tx pgx.Tx, userID uuid.UUID, password string) error {
	hash, err := HashPassword(ctx, password, s.params)
	if err != nil {
		return err
	}
	// A new password ends a stop after wrong ones: it was chosen by the person through a
	// reset link or a session (ADR-038).
	_, err = tx.Exec(ctx, `INSERT INTO local_credentials (user_id, password_hash) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET password_hash = EXCLUDED.password_hash, updated_at = now(),
			failed_logins_in_row = 0, password_locked_at = NULL`, userID, hash)
	return err
}

// LiftPasswordLock lets password login work again for an account that was stopped after
// too many wrong passwords. The route requires administrator.
func (s *Service) LiftPasswordLock(ctx context.Context, c Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.mayActOn(ctx, tx, c, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE local_credentials SET failed_logins_in_row = 0, password_locked_at = NULL
			WHERE user_id = $1 AND password_locked_at IS NOT NULL`, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("not_locked", "Password login is not stopped for this person.")
		}
		return s.record(ctx, tx, c, "auth.password_unlocked", "user", id.String(), nil)
	})
}

// checkPassword verifies a user's current password. It returns a forbidden error
// and not a login error: the user is logged in and is only confirming who they are.
func (s *Service) checkPassword(ctx context.Context, q db.Querier, userID uuid.UUID, password string) error {
	var hash string
	err := q.QueryRow(ctx, "SELECT password_hash FROM local_credentials WHERE user_id = $1", userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Conflict("no_password", "This account has no password.")
	}
	if err != nil {
		return err
	}
	ok, _, err := VerifyPassword(ctx, hash, password, s.params)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.Conflict("wrong_password", "The current password is not correct.")
	}
	return nil
}

// ChangePassword changes a user's own password and ends their other sessions.
func (s *Service) ChangePassword(ctx context.Context, c Caller, sessionID uuid.UUID, current, next string) error {
	u, err := s.GetUser(ctx, s.pool, c.UserID)
	if err != nil {
		return err
	}
	if err := s.validateNewPassword(next, u.Email, u.DisplayName); err != nil {
		return err
	}
	if err := s.checkPassword(ctx, s.pool, c.UserID, current); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.storePassword(ctx, tx, c.UserID, next); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1 AND id <> $2", c.UserID, sessionID); err != nil {
			return err
		}
		if err := s.record(ctx, tx, c, "auth.password_changed", "user", c.UserID.String(), nil); err != nil {
			return err
		}
		return s.tellOfChange(ctx, tx, c.UserID)
	})
}

// SessionInfo describes one session of a user.
type SessionInfo struct {
	ID         uuid.UUID `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	Current    bool      `json:"current"`
}

// Sessions lists a user's own sessions.
func (s *Service) Sessions(ctx context.Context, userID, current uuid.UUID) ([]SessionInfo, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, created_at, last_seen_at, ip, user_agent FROM sessions
		WHERE user_id = $1 ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (SessionInfo, error) {
		var v SessionInfo
		err := r.Scan(&v.ID, &v.CreatedAt, &v.LastSeenAt, &v.IP, &v.UserAgent)
		v.Current = v.ID == current
		return v, err
	})
}

// RevokeSession ends one of the caller's own sessions.
func (s *Service) RevokeSession(ctx context.Context, c Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM sessions WHERE id = $1 AND user_id = $2", id, c.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return s.record(ctx, tx, c, "auth.sessions_revoked", "session", id.String(), nil)
	})
}

// DirectoryEntry is a user or group for pickers.
type DirectoryEntry struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	// Detail is the e-mail address of a user. It stays empty for groups.
	Detail string `json:"detail"`
}

// Directory searches users and groups, for people pickers. The route requires
// "view profiles", which guests do not have by default.
func (s *Service) Directory(ctx context.Context, text string) ([]DirectoryEntry, error) {
	rows, err := s.pool.Query(ctx, `(SELECT 'user', id, display_name, email FROM users
			WHERE status = 'active' AND type <> 'service' AND anonymised_at IS NULL
			  AND ($1 = '' OR position(lower($1) IN lower(display_name)) > 0 OR position(lower($1) IN lower(email)) > 0)
			ORDER BY lower(display_name) LIMIT 20)
		UNION ALL
		(SELECT 'group', id, name, '' FROM groups
			WHERE $1 = '' OR position(lower($1) IN lower(name)) > 0
			ORDER BY lower(name) LIMIT 20)`, text)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (DirectoryEntry, error) {
		var e DirectoryEntry
		err := r.Scan(&e.Type, &e.ID, &e.Name, &e.Detail)
		return e, err
	})
	if out == nil {
		out = []DirectoryEntry{}
	}
	return out, err
}

// PeopleByName finds people by the words of their name, through the index. It is for the
// search box, which runs on every pause in typing: Directory reads every person.
func (s *Service) PeopleByName(ctx context.Context, words string, limit int) ([]DirectoryEntry, error) {
	rows, err := s.pool.Query(ctx, `SELECT 'user', id, display_name, email FROM users
		WHERE to_tsvector('simple', display_name) @@ to_tsquery('simple', $1)
		  AND status = 'active' AND type <> 'service' AND anonymised_at IS NULL
		ORDER BY lower(display_name) LIMIT $2`, words, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (DirectoryEntry, error) {
		var e DirectoryEntry
		err := r.Scan(&e.Type, &e.ID, &e.Name, &e.Detail)
		return e, err
	})
}

// DisplayNames gives today's names of people and groups, for a mention.
func DisplayNames(ctx context.Context, q db.Querier, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := map[uuid.UUID]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT id, display_name FROM users WHERE id = ANY($1)
		UNION ALL SELECT id, name FROM groups WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// FirstAdministrator finds the oldest active administrator who may use the
// product, to own what the system sets up. Found is false while there is none.
func (s *Service) FirstAdministrator(ctx context.Context) (id uuid.UUID, found bool, err error) {
	err = s.pool.QueryRow(ctx, `SELECT id FROM users
		WHERE perm_has_global(id, $1) AND perm_has_global(id, $2)
		ORDER BY created_at, id LIMIT 1`, PermAdministrator, PermUseProduct).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}

// External describes an account that an identity provider manages.
type External struct {
	Email       string
	DisplayName string
	Username    string
	ExternalID  string
	Source      string
	Active      bool
}

// CreateExternalUser creates an account without a local password, for OIDC and SCIM.
func (s *Service) CreateExternalUser(ctx context.Context, tx pgx.Tx, c Caller, in External) (uuid.UUID, error) {
	in.Email = normalizeEmail(in.Email)
	in.DisplayName = strings.TrimSpace(plaintext.WithoutDirectionMarks(in.DisplayName))
	if in.DisplayName == "" {
		in.DisplayName = in.Email
	}
	if !validEmail(in.Email) {
		return uuid.Nil, apperr.Invalid("The account has no valid e-mail address.")
	}
	if utf8.RuneCountInString(in.DisplayName) > 200 {
		return uuid.Nil, apperr.Invalid("The display name is too long.")
	}
	status := StatusActive
	if !in.Active {
		status = StatusDisabled
	}
	id := ids.New()
	_, err := tx.Exec(ctx, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name, username, external_id, source)
		VALUES ($1, $2, 'member', $3, $4, $5, $6, $7, $8)`,
		id, s.instanceID, status, in.Email, in.DisplayName, in.Username, in.ExternalID, in.Source)
	if db.IsUniqueViolation(err) {
		return uuid.Nil, apperr.Conflict("email_taken", "This e-mail address is already in use.")
	}
	if err != nil {
		return uuid.Nil, err
	}
	for _, p := range defaultPermissions(TypeMember) {
		if _, err := tx.Exec(ctx, "SELECT rbac_add_permissions(NULL, 'user', $1, ARRAY[$2::text])", id, p); err != nil {
			return uuid.Nil, err
		}
	}
	return id, s.record(ctx, tx, c, "user.created", "user", id.String(), map[string]any{"email": in.Email, "auth": in.Source})
}

// UpdateExternalUser applies changes from an identity provider. Going inactive
// ends the sessions at once.
func (s *Service) UpdateExternalUser(ctx context.Context, c Caller, id uuid.UUID, in External) error {
	in.Email = normalizeEmail(in.Email)
	if !validEmail(in.Email) {
		return apperr.Invalid("The account has no valid e-mail address.")
	}
	in.DisplayName = strings.TrimSpace(plaintext.WithoutDirectionMarks(in.DisplayName))
	if in.DisplayName == "" {
		in.DisplayName = in.Email
	}
	status := StatusActive
	if !in.Active {
		status = StatusDisabled
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var before string
		err := tx.QueryRow(ctx, "SELECT status FROM users WHERE id = $1 FOR UPDATE", id).Scan(&before)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE users SET email = $2, display_name = $3, username = $4, external_id = $5, status = $6, updated_at = now()
			WHERE id = $1`, id, in.Email, in.DisplayName, in.Username, in.ExternalID, status)
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("email_taken", "This e-mail address is already in use.")
		}
		if err != nil {
			return err
		}
		if status == StatusDisabled {
			if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id = $1", id); err != nil {
				return err
			}
		}
		action := "user.updated"
		switch {
		case before == StatusActive && status == StatusDisabled:
			action = "user.disabled"
		case before == StatusDisabled && status == StatusActive:
			action = "user.enabled"
		}
		return s.record(ctx, tx, c, action, "user", id.String(), map[string]any{"source": fmt.Sprint(c.System)})
	})
}
