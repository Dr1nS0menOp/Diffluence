// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/db"
	"github.com/diffluence/diffluence/internal/ids"
)

const (
	challengePasskeyRegister = "passkey_register"
	challengePasskeyLogin    = "passkey_login"
	passkeyTTL               = 5 * time.Minute
)

// NewWebAuthn builds the relying party from the public URL of this installation.
func NewWebAuthn(instanceName, host, origin string) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{RPDisplayName: instanceName, RPID: host, RPOrigins: []string{origin}})
}

// passkeyUser adapts an account to the webauthn library.
type passkeyUser struct {
	id          uuid.UUID
	email       string
	displayName string
	credentials []webauthn.Credential
}

func (u *passkeyUser) WebAuthnID() []byte                         { return u.id[:] }
func (u *passkeyUser) WebAuthnName() string                       { return u.email }
func (u *passkeyUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *Service) loadPasskeyUser(ctx context.Context, q db.Querier, id uuid.UUID) (*passkeyUser, error) {
	u := &passkeyUser{id: id}
	err := q.QueryRow(ctx, "SELECT email, display_name FROM users WHERE id = $1", id).Scan(&u.email, &u.displayName)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, "SELECT credential FROM webauthn_credentials WHERE user_id = $1", id)
	if err != nil {
		return nil, err
	}
	raw, err := pgx.CollectRows(rows, pgx.RowTo[[]byte])
	if err != nil {
		return nil, err
	}
	for _, b := range raw {
		var c webauthn.Credential
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		u.credentials = append(u.credentials, c)
	}
	return u, nil
}

// BeginPasskey starts registering a passkey. It returns the options for the
// browser and a token for FinishPasskey.
func (s *Service) BeginPasskey(ctx context.Context, c Caller) (any, string, error) {
	u, err := s.loadPasskeyUser(ctx, s.pool, c.UserID)
	if err != nil {
		return nil, "", err
	}
	exclude := make([]protocol.CredentialDescriptor, 0, len(u.credentials))
	for _, cred := range u.credentials {
		exclude = append(exclude, cred.Descriptor())
	}
	options, session, err := s.webauthn.BeginRegistration(u,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclude))
	if err != nil {
		return nil, "", err
	}
	token, err := s.NewChallenge(ctx, s.pool, challengePasskeyRegister, &c.UserID, session, passkeyTTL)
	return options, token, err
}

// FinishPasskey stores a new passkey.
func (s *Service) FinishPasskey(ctx context.Context, c Caller, token, name string, response []byte) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return apperr.Invalid("Give the passkey a name of at most 100 characters.")
	}
	var session webauthn.SessionData
	owner, err := s.TakeChallenge(ctx, s.pool, challengePasskeyRegister, token, &session)
	if err != nil || owner == nil || *owner != c.UserID {
		return apperr.Invalid("The passkey setup expired. Try again.")
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(response))
	if err != nil {
		return apperr.Invalid("The browser sent an invalid passkey.")
	}
	u, err := s.loadPasskeyUser(ctx, s.pool, c.UserID)
	if err != nil {
		return err
	}
	cred, err := s.webauthn.CreateCredential(u, session, parsed)
	if err != nil {
		return apperr.Invalid("The passkey could not be verified.")
	}
	body, err := json.Marshal(cred)
	if err != nil {
		return err
	}
	id := ids.New()
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "INSERT INTO webauthn_credentials (id, user_id, credential_id, credential, name) VALUES ($1, $2, $3, $4, $5)",
			id, c.UserID, cred.ID, body, name)
		if db.IsUniqueViolation(err) {
			return apperr.Conflict("passkey_exists", "This passkey is already registered.")
		}
		if err != nil {
			return err
		}
		return s.record(ctx, tx, c, "auth.passkey_added", "passkey", id.String(), map[string]any{"name": name})
	})
}

// PasskeyInfo describes a registered passkey.
type PasskeyInfo struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// Passkeys lists a user's own passkeys.
func (s *Service) Passkeys(ctx context.Context, userID uuid.UUID) ([]PasskeyInfo, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, created_at, last_used_at FROM webauthn_credentials WHERE user_id = $1 ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (PasskeyInfo, error) {
		var p PasskeyInfo
		err := r.Scan(&p.ID, &p.Name, &p.CreatedAt, &p.LastUsedAt)
		return p, err
	})
	if out == nil {
		out = []PasskeyInfo{}
	}
	return out, err
}

// RemovePasskey deletes one of the caller's passkeys.
func (s *Service) RemovePasskey(ctx context.Context, c Caller, id uuid.UUID) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2", id, c.UserID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound()
		}
		return s.record(ctx, tx, c, "auth.passkey_removed", "passkey", id.String(), nil)
	})
}

// BeginPasskeyLogin starts a login without a user name: the browser offers the
// passkeys it has for this site.
func (s *Service) BeginPasskeyLogin(ctx context.Context) (any, string, error) {
	options, session, err := s.webauthn.BeginDiscoverableLogin()
	if err != nil {
		return nil, "", err
	}
	token, err := s.NewChallenge(ctx, s.pool, challengePasskeyLogin, nil, session, passkeyTTL)
	return options, token, err
}

// FinishPasskeyLogin verifies the assertion and opens a session. A passkey needs
// no second factor: it already is possession plus a local check on the device.
func (s *Service) FinishPasskeyLogin(ctx context.Context, token string, response []byte, meta RequestMeta) (string, Principal, error) {
	fail := func(reason string) (string, Principal, error) {
		if err := s.RecordLoginFailure(ctx, "passkey", reason, meta); err != nil {
			return "", Principal{}, err
		}
		return "", Principal{}, ErrInvalidCredentials
	}
	var session webauthn.SessionData
	if _, err := s.TakeChallenge(ctx, s.pool, challengePasskeyLogin, token, &session); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return fail("expired_challenge")
		}
		return "", Principal{}, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
	if err != nil {
		return fail("bad_response")
	}
	var found *passkeyUser
	handler := func(_, userHandle []byte) (webauthn.User, error) {
		id, err := uuid.FromBytes(userHandle)
		if err != nil {
			return nil, err
		}
		found, err = s.loadPasskeyUser(ctx, s.pool, id)
		return found, err
	}
	cred, err := s.webauthn.ValidateDiscoverableLogin(handler, session, parsed)
	if err != nil || found == nil {
		return fail("bad_assertion")
	}
	updated, err := json.Marshal(cred)
	if err != nil {
		return "", Principal{}, err
	}
	var sessionToken string
	var p Principal
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE webauthn_credentials SET credential = $2, last_used_at = now() WHERE credential_id = $1", cred.ID, updated); err != nil {
			return err
		}
		var err error
		sessionToken, p, err = s.OpenSession(ctx, tx, found.id, "passkey", meta)
		return err
	})
	if errors.Is(err, ErrInvalidCredentials) {
		return fail("user_disabled")
	}
	return sessionToken, p, err
}
