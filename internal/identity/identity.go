// SPDX-License-Identifier: Apache-2.0

// Package identity owns users, local credentials, sessions and global
// permissions (layer 1 of docs/blueprint/05).
package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	PermPersonalSpace       = "create_personal_space"
)

// AllGlobalPermissions lists every global permission.
var AllGlobalPermissions = []string{PermUseProduct, PermCreateSpace, PermViewProfiles, PermAdministrator, PermSystemAdministrator, PermPersonalSpace}

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

// Principal is an authenticated user plus the session or token used.
type Principal struct {
	User      User
	SessionID uuid.UUID
	// ViaToken says a personal access token was used. CanWrite is its scope.
	ViaToken bool
	CanWrite bool
	TokenID  uuid.UUID
	Scopes   []string
	// ConfirmedAt is when the person last proved who they are in this session.
	ConfirmedAt time.Time
	// NeedsSecondFactor limits the session to setting up a second factor.
	NeedsSecondFactor bool
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
	// PasswordBlocklist holds known bad passwords, in lower case.
	PasswordBlocklist map[string]bool
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

	idleSetting      func(context.Context) time.Duration
	secondFactorRule func(context.Context) string
	mailer           Mailer
	origin           string
	blocklist        map[string]bool

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
		keyring: opts.Keyring, webauthn: opts.WebAuthn, issuer: opts.Issuer, blocklist: opts.PasswordBlocklist}
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
	if err := s.validateNewPassword(in.Password, in.Email, in.DisplayName); err != nil {
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
	// An account past its end date is treated like a disabled one.
	err := s.pool.QueryRow(ctx, `SELECT u.id, u.type,
			CASE WHEN u.expires_at IS NOT NULL AND u.expires_at <= now() THEN 'disabled' ELSE u.status END,
			u.email, u.display_name, u.created_at, c.password_hash
		FROM users u JOIN local_credentials c ON c.user_id = u.id WHERE lower(u.email) = $1 AND u.type <> 'service'`, email).
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
		if err := s.recordFailedLogin(ctx, failedLogin{reason: reason, email: email, account: u, known: found}, meta); err != nil {
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
			token, p.SessionID, err = s.createSession(ctx, tx, sessionStart{UserID: u.ID, Method: "password", Meta: meta})
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

// failedLogin is what is known of a login that did not work.
type failedLogin struct {
	reason  string
	email   string
	account User
	known   bool
}

// recordFailedLogin never stores what was typed. People sometimes type a password in
// the address field, and an audit row cannot be changed afterwards (SEC-820). A known
// account gets its id. An unknown address gets a keyed hash, which still shows
// credential stuffing: many tries with one address.
func (s *Service) recordFailedLogin(ctx context.Context, f failedLogin, meta RequestMeta) error {
	details := map[string]any{"reason": f.reason, "method": "password"}
	objectID := ""
	if f.known {
		objectID = f.account.ID.String()
	} else if s.keyring != nil {
		sum, err := s.keyring.Fingerprint("login address", []byte(strings.ToLower(strings.TrimSpace(f.email))))
		if err != nil {
			return err
		}
		details["address_hash"] = hex.EncodeToString(sum)
	}
	return audit.RecordStandalone(ctx, s.pool, event(meta, Actor{Type: audit.ActorAnonymous}, "auth.login", "user", objectID,
		audit.ResultFailure, details))
}

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

func (s *Service) createSession(ctx context.Context, tx pgx.Tx, in sessionStart) (string, uuid.UUID, error) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := base64.RawURLEncoding.EncodeToString(b)
	id := ids.New()
	now := s.now()
	needsFactor, err := s.needsSecondFactor(ctx, tx, in.UserID)
	if err != nil {
		return "", id, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO sessions (id, token_hash, user_id, created_at, last_seen_at, expires_at, ip, user_agent, needs_second_factor)
		VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8)`,
		id, hashToken(token), in.UserID, now, now.Add(s.maxAge), in.Meta.IP, truncate(in.Meta.UserAgent, 512), needsFactor)
	if err != nil {
		return "", id, err
	}
	return token, id, s.noteLogin(ctx, tx, in)
}

// session resolves a session token and removes it when it has ended. Only a hash
// of the token is stored, so a database leak does not leak sessions.
func (s *Service) session(ctx context.Context, token string) (Principal, time.Time, error) {
	if token == "" || len(token) > 128 {
		return Principal{}, time.Time{}, ErrNoSession
	}
	var p Principal
	var lastSeen, created, expires time.Time
	var confirmed, accountEnds *time.Time
	err := s.pool.QueryRow(ctx, `SELECT s.id, s.last_seen_at, s.created_at, s.expires_at, s.reauth_at, s.needs_second_factor,
			u.id, u.type, u.status, u.email, u.display_name, u.created_at, u.expires_at
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1`, hashToken(token)).
		Scan(&p.SessionID, &lastSeen, &created, &expires, &confirmed, &p.NeedsSecondFactor,
			&p.User.ID, &p.User.Type, &p.User.Status, &p.User.Email, &p.User.DisplayName, &p.User.CreatedAt, &accountEnds)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, time.Time{}, ErrNoSession
	}
	if err != nil {
		return Principal{}, time.Time{}, err
	}
	now := s.now()
	accountEnded := accountEnds != nil && !now.Before(*accountEnds)
	if p.User.Status != StatusActive || accountEnded || !now.Before(expires) || !now.Before(lastSeen.Add(s.idleLimit(ctx))) {
		_, _ = s.pool.Exec(ctx, "DELETE FROM sessions WHERE id = $1", p.SessionID)
		return Principal{}, time.Time{}, ErrNoSession
	}
	p.CanWrite = true
	p.ConfirmedAt = created
	if confirmed != nil && confirmed.After(created) {
		p.ConfirmedAt = *confirmed
	}
	return p, lastSeen, nil
}

// idleLimit is the idle time an administrator chose, or the configured one.
func (s *Service) idleLimit(ctx context.Context) time.Duration {
	if s.idleSetting != nil {
		if d := s.idleSetting(ctx); d > 0 {
			return d
		}
	}
	return s.idle
}

// AuthenticateSession resolves a session token, and counts as activity.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (Principal, error) {
	p, lastSeen, err := s.session(ctx, token)
	if err != nil {
		return p, err
	}
	// Refresh at most once a minute to avoid a write on every request.
	if now := s.now(); now.Sub(lastSeen) > time.Minute {
		_, _ = s.pool.Exec(ctx, "UPDATE sessions SET last_seen_at = $2 WHERE id = $1", p.SessionID, now)
	}
	return p, nil
}

// PeekSession resolves a session token without counting as activity. Polling in an
// open tab must not keep a session alive for ever (spec 013, D8).
func (s *Service) PeekSession(ctx context.Context, token string) (Principal, error) {
	p, _, err := s.session(ctx, token)
	return p, err
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
