// SPDX-License-Identifier: Apache-2.0

// Package identity owns users, local credentials, sessions and global
// permissions (layer 1 of docs/blueprint/05).
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/keys"
	"github.com/diffluence/diffluence/internal/outbox"
)

// Global permissions.
const (
	PermUseProduct          = "use_product"
	PermCreateSpace         = "create_space"
	PermViewProfiles        = "view_profiles"
	PermAdministrator       = "administrator"
	PermSystemAdministrator = "system_administrator"
)

// AllGlobalPermissions lists every global permission.
var AllGlobalPermissions = []string{PermUseProduct, PermCreateSpace, PermViewProfiles, PermAdministrator, PermSystemAdministrator}

// User types and statuses.
const (
	TypeMember  = "member"
	TypeGuest   = "guest"
	TypeService = "service"

	StatusActive   = "active"
	StatusDisabled = "disabled"
)

var (
	ErrInvalidCredentials = errors.New("identity: invalid credentials")
	ErrEmailTaken         = errors.New("identity: email address already in use")
	ErrInvalidInput       = errors.New("identity: invalid input")
	ErrNoSession          = errors.New("identity: no valid session")
)

// User is an account.
type User struct {
	ID          uuid.UUID
	Type        string
	Status      string
	Email       string
	DisplayName string
	CreatedAt   time.Time
}

// Principal is an authenticated user plus the session used.
type Principal struct {
	User      User
	SessionID uuid.UUID
}

// RequestMeta describes where a request came from, for the audit log.
type RequestMeta struct {
	IP        string
	UserAgent string
	RequestID string
}

// Actor is who performs an action, for the audit log.
type Actor struct {
	Type string // audit.ActorUser, audit.ActorSystem, ...
	ID   string
}

// SystemActor is used for CLI and background actions.
func SystemActor(component string) Actor { return Actor{Type: audit.ActorSystem, ID: component} }

// Options configure the service.
type Options struct {
	Params             Params
	SessionIdleTimeout time.Duration
	SessionMaxAge      time.Duration
	Now                func() time.Time // for tests
	Keyring            *keys.Keyring
	WebAuthn           *webauthn.WebAuthn
	Issuer             string // name shown in authenticator apps
}

// Service implements identity operations.
type Service struct {
	pool       *pgxpool.Pool
	instanceID uuid.UUID
	params     Params
	idle       time.Duration
	maxAge     time.Duration
	now        func() time.Time
	keyring    *keys.Keyring
	webauthn   *webauthn.WebAuthn
	issuer     string

	dummyOnce sync.Once
	dummy     string
}

// NewService creates the service.
func NewService(pool *pgxpool.Pool, instanceID uuid.UUID, opts Options) *Service {
	if opts.Params == (Params{}) {
		opts.Params = DefaultParams
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Issuer == "" {
		opts.Issuer = "Diffluence"
	}
	return &Service{pool: pool, instanceID: instanceID, params: opts.Params,
		idle: opts.SessionIdleTimeout, maxAge: opts.SessionMaxAge, now: opts.Now,
		keyring: opts.Keyring, webauthn: opts.WebAuthn, issuer: opts.Issuer}
}

// dummyHash is verified against when a user does not exist, so the response
// time does not reveal which email addresses have accounts.
func (s *Service) dummyHash() string {
	s.dummyOnce.Do(func() {
		s.dummy, _ = HashPassword("diffluence-timing-equaliser", s.params)
	})
	return s.dummy
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func validEmail(e string) bool {
	if len(e) > 254 || strings.ContainsAny(e, " \t\r\n<>") {
		return false
	}
	a, err := mail.ParseAddress(e)
	return err == nil && a.Address == e && strings.Contains(e[strings.LastIndex(e, "@")+1:], ".")
}

func event(meta RequestMeta, actor Actor, action, objType, objID, result string, details map[string]any) audit.Event {
	return audit.Event{ActorType: actor.Type, ActorID: actor.ID, Action: action, ObjectType: objType,
		ObjectID: objID, Result: result, IP: meta.IP, UserAgent: meta.UserAgent, RequestID: meta.RequestID,
		Details: details}
}

// NewUser is the input for CreateLocalUser.
type NewUser struct {
	Email             string
	DisplayName       string
	Type              string
	Password          string
	GlobalPermissions []string
}

// CreateLocalUser creates an account with a password and global permissions.
func (s *Service) CreateLocalUser(ctx context.Context, in NewUser, actor Actor, meta RequestMeta) (User, error) {
	in.Email = normalizeEmail(in.Email)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Type == "" {
		in.Type = TypeMember
	}
	switch {
	case !validEmail(in.Email):
		return User{}, fmt.Errorf("%w: invalid email address", ErrInvalidInput)
	case in.DisplayName == "" || utf8.RuneCountInString(in.DisplayName) > 200:
		return User{}, fmt.Errorf("%w: display name must be 1 to 200 characters", ErrInvalidInput)
	case in.Type != TypeMember && in.Type != TypeGuest:
		return User{}, fmt.Errorf("%w: type must be member or guest", ErrInvalidInput)
	}
	for _, p := range in.GlobalPermissions {
		if !slices.Contains(AllGlobalPermissions, p) {
			return User{}, fmt.Errorf("%w: unknown permission %q", ErrInvalidInput, p)
		}
	}
	if err := ValidatePassword(in.Password); err != nil {
		return User{}, err
	}
	hash, err := HashPassword(in.Password, s.params)
	if err != nil {
		return User{}, err
	}

	u := User{ID: ids.New(), Type: in.Type, Status: StatusActive, Email: in.Email, DisplayName: in.DisplayName}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO users (id, origin_instance_id, type, status, email, display_name)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at`,
			u.ID, s.instanceID, u.Type, u.Status, u.Email, u.DisplayName).Scan(&u.CreatedAt)
		if db.IsUniqueViolation(err) {
			return ErrEmailTaken
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO local_credentials (user_id, password_hash) VALUES ($1, $2)", u.ID, hash); err != nil {
			return err
		}
		if _, err := audit.Record(ctx, tx, event(meta, actor, "user.created", "user", u.ID.String(), audit.ResultSuccess,
			map[string]any{"email": u.Email, "type": u.Type, "auth": "local"})); err != nil {
			return err
		}
		for _, p := range in.GlobalPermissions {
			if _, err := tx.Exec(ctx, "INSERT INTO global_grants (subject_type, subject_id, permission) VALUES ('user', $1, $2)", u.ID, p); err != nil {
				return err
			}
			if _, err := audit.Record(ctx, tx, event(meta, actor, "permission.global.granted", "user", u.ID.String(), audit.ResultSuccess,
				map[string]any{"permission": p})); err != nil {
				return err
			}
		}
		_, err = outbox.Enqueue(ctx, tx, "user.created", map[string]any{"user_id": u.ID, "type": u.Type})
		return err
	})
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// Login checks a local password and opens a session. It returns the session
// token, which the caller puts in a cookie. All outcomes are audited.
func (s *Service) Login(ctx context.Context, email, password string, meta RequestMeta) (string, Principal, error) {
	email = normalizeEmail(email)
	var u User
	var hash string
	err := s.pool.QueryRow(ctx, `SELECT u.id, u.type, u.status, u.email, u.display_name, u.created_at, c.password_hash
		FROM users u JOIN local_credentials c ON c.user_id = u.id WHERE lower(u.email) = $1`, email).
		Scan(&u.ID, &u.Type, &u.Status, &u.Email, &u.DisplayName, &u.CreatedAt, &hash)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", Principal{}, err
	}
	if !found {
		hash = s.dummyHash()
	}
	ok, rehash, verr := VerifyPassword(hash, password, s.params)

	reason := ""
	switch {
	case !found:
		reason = "unknown_user"
	case verr != nil:
		reason = "unusable_password_hash"
	case u.Status != StatusActive:
		reason = "user_disabled"
	case !ok:
		reason = "bad_password"
	}
	if reason != "" {
		actor := Actor{Type: audit.ActorAnonymous}
		objID := ""
		if found {
			objID = u.ID.String()
		}
		// Attempted email is recorded to detect credential-stuffing; never the password.
		e := event(meta, actor, "auth.login", "user", objID, audit.ResultFailure,
			map[string]any{"reason": reason, "email": truncate(email, 254), "method": "password"})
		if err := audit.RecordStandalone(ctx, s.pool, e); err != nil {
			return "", Principal{}, err
		}
		return "", Principal{}, ErrInvalidCredentials
	}

	mfa, err := s.hasTOTP(ctx, s.pool, u.ID)
	if err != nil {
		return "", Principal{}, err
	}
	var token string
	var p Principal
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		if !mfa {
			token, p.SessionID, err = s.createSession(ctx, tx, u.ID, meta)
			if err != nil {
				return err
			}
		}
		if rehash {
			newHash, err := HashPassword(password, s.params)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "UPDATE local_credentials SET password_hash = $2, updated_at = now() WHERE user_id = $1", u.ID, newHash); err != nil {
				return err
			}
		}
		if mfa {
			token, err = s.NewChallenge(ctx, tx, challengeMFA, &u.ID, map[string]any{}, mfaTTL)
			return err
		}
		actor := Actor{Type: audit.ActorUser, ID: u.ID.String()}
		if _, err := audit.Record(ctx, tx, event(meta, actor, "auth.login", "session", p.SessionID.String(), audit.ResultSuccess,
			map[string]any{"method": "password", "password_rehashed": rehash})); err != nil {
			return err
		}
		_, err = outbox.Enqueue(ctx, tx, "auth.login_succeeded", map[string]any{"user_id": u.ID, "session_id": p.SessionID})
		return err
	})
	if err != nil {
		return "", Principal{}, err
	}
	if mfa {
		return "", Principal{}, &MFARequiredError{Token: token}
	}
	p.User = u
	return token, p, nil
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, userID uuid.UUID, meta RequestMeta) (string, uuid.UUID, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	id := ids.New()
	now := s.now()
	_, err := tx.Exec(ctx, `INSERT INTO sessions (id, token_hash, user_id, created_at, last_seen_at, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $4, $5, $6, $7)`,
		id, hashToken(token), userID, now, now.Add(s.maxAge), meta.IP, truncate(meta.UserAgent, 512))
	return token, id, err
}

// AuthenticateSession resolves a session token. Expired sessions are removed.
// Only a hash of the token is stored, so a database leak does not leak sessions.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (Principal, error) {
	if token == "" || len(token) > 128 {
		return Principal{}, ErrNoSession
	}
	var p Principal
	var lastSeen, expires time.Time
	err := s.pool.QueryRow(ctx, `SELECT s.id, s.last_seen_at, s.expires_at,
			u.id, u.type, u.status, u.email, u.display_name, u.created_at
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1`, hashToken(token)).
		Scan(&p.SessionID, &lastSeen, &expires, &p.User.ID, &p.User.Type, &p.User.Status, &p.User.Email,
			&p.User.DisplayName, &p.User.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrNoSession
	}
	if err != nil {
		return Principal{}, err
	}
	now := s.now()
	if p.User.Status != StatusActive || !now.Before(expires) || !now.Before(lastSeen.Add(s.idle)) {
		_, _ = s.pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", p.SessionID)
		return Principal{}, ErrNoSession
	}
	// Refresh at most once a minute to avoid a write on every request.
	if now.Sub(lastSeen) > time.Minute {
		_, _ = s.pool.Exec(ctx, "UPDATE sessions SET last_seen_at = $2 WHERE id = $1", p.SessionID, now)
	}
	return p, nil
}

// Logout ends the principal's current session.
func (s *Service) Logout(ctx context.Context, p Principal, meta RequestMeta) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM sessions WHERE id = $1", p.SessionID); err != nil {
			return err
		}
		_, err := audit.Record(ctx, tx, event(meta, Actor{Type: audit.ActorUser, ID: p.User.ID.String()},
			"auth.logout", "session", p.SessionID.String(), audit.ResultSuccess, nil))
		return err
	})
}

// HasGlobalPermission reports whether the user holds perm, directly or through
// a group.
func (s *Service) HasGlobalPermission(ctx context.Context, userID uuid.UUID, perm string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, "SELECT perm_has_global($1, $2)", userID, perm).Scan(&ok)
	return ok, err
}

// GlobalPermissions lists the user's global permissions.
func (s *Service) GlobalPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT permission FROM global_grants
		WHERE (subject_type = 'user' AND subject_id = $1)
		   OR (subject_type = 'group' AND subject_id IN (SELECT group_id FROM group_members WHERE user_id = $1))
		ORDER BY permission`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// RecordAccessDenied audits a request that was refused for lack of permission.
func (s *Service) RecordAccessDenied(ctx context.Context, p Principal, permission, method, path string, meta RequestMeta) error {
	return audit.RecordStandalone(ctx, s.pool, event(meta, Actor{Type: audit.ActorUser, ID: p.User.ID.String()},
		"access.denied", "route", method+" "+truncate(path, 300), audit.ResultDenied,
		map[string]any{"permission": permission}))
}

// RecordObjectDenied audits a refused action on an object.
func (s *Service) RecordObjectDenied(ctx context.Context, p Principal, action, objectType, objectID, spaceID string, meta RequestMeta) error {
	e := event(meta, Actor{Type: audit.ActorUser, ID: p.User.ID.String()}, "access.denied", objectType, objectID, audit.ResultDenied,
		map[string]any{"permission": action})
	e.SpaceID = spaceID
	return audit.RecordStandalone(ctx, s.pool, e)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Cut on a rune boundary.
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
