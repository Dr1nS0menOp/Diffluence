// SPDX-License-Identifier: Apache-2.0

// Package sso is OpenID Connect login (spec 004): the authorization code flow
// with PKCE, state and nonce, on top of the go-oidc library.
package sso

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/diffluence/diffluence/internal/apperr"
	"github.com/diffluence/diffluence/internal/identity"
)

const (
	challengeKind = "oidc"
	flowTTL       = 10 * time.Minute
)

// Config is the provider setup, from the environment.
type Config struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	DisplayName  string
	AutoCreate   bool
	GroupsClaim  string // empty = do not sync groups
}

// Login failures users can be told about.
var (
	ErrNoAccount     = errors.New("sso: no account for this login")
	ErrAccountExists = errors.New("sso: a local account uses this e-mail address")
	ErrFailed        = errors.New("sso: login failed")
)

// Provider runs logins against one identity provider.
type Provider struct {
	cfg      Config
	pool     *pgxpool.Pool
	identity *identity.Service

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// New creates a provider. Discovery happens on first use, so a provider that is
// down cannot stop Diffluence from starting.
func New(cfg Config, pool *pgxpool.Pool, id *identity.Service) *Provider {
	return &Provider{cfg: cfg, pool: pool, identity: id}
}

// DisplayName is what the login button says.
func (p *Provider) DisplayName() string { return p.cfg.DisplayName }

// Issuer shows where logins go, for the admin console. It is not a secret.
func (p *Provider) Issuer() string { return p.cfg.Issuer }

func (p *Provider) discover(ctx context.Context) (*oidc.IDTokenVerifier, *oauth2.Config, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.verifier != nil {
		return p.verifier, p.oauth, nil
	}
	// Keys are refreshed in the background with this context, so it must outlive the request.
	provider, err := oidc.NewProvider(context.WithoutCancel(ctx), p.cfg.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("sso: discovery: %w", err)
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if p.cfg.GroupsClaim != "" {
		scopes = append(scopes, "groups")
	}
	p.oauth = &oauth2.Config{ClientID: p.cfg.ClientID, ClientSecret: p.cfg.ClientSecret, RedirectURL: p.cfg.RedirectURL,
		Endpoint: provider.Endpoint(), Scopes: scopes}
	p.verifier = provider.Verifier(&oidc.Config{ClientID: p.cfg.ClientID})
	return p.verifier, p.oauth, nil
}

type flow struct {
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Binding  string `json:"binding"`
}

// Start begins a login. It returns the URL to send the browser to, and a value
// for a cookie that ties the flow to this browser. That stops an attacker from
// slipping their own login into someone else's browser.
func (p *Provider) Start(ctx context.Context) (redirect, binding string, err error) {
	return p.start(ctx)
}

// StartFresh begins a login in which the provider must ask again, also when the
// person is still logged in there. It is the step-up of an account without a
// password in Diffluence (spec 013, F6).
func (p *Provider) StartFresh(ctx context.Context) (redirect, binding string, err error) {
	return p.start(ctx, oauth2.SetAuthURLParam("prompt", "login"))
}

func (p *Provider) start(ctx context.Context, extra ...oauth2.AuthCodeOption) (redirect, binding string, err error) {
	_, config, err := p.discover(ctx)
	if err != nil {
		return "", "", err
	}
	f := flow{Nonce: oauth2.GenerateVerifier(), Verifier: oauth2.GenerateVerifier(), Binding: oauth2.GenerateVerifier()}
	state, err := p.identity.NewChallenge(ctx, p.pool, challengeKind, nil, f, flowTTL)
	if err != nil {
		return "", "", err
	}
	options := append([]oauth2.AuthCodeOption{oidc.Nonce(f.Nonce), oauth2.S256ChallengeOption(f.Verifier)}, extra...)
	return config.AuthCodeURL(state, options...), f.Binding, nil
}

type claims struct {
	Subject           string `json:"sub"`
	ObjectID          string `json:"oid"` // Entra ID: stable across applications
	Email             string `json:"email"`
	EmailVerified     *bool  `json:"email_verified"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

var providerError = regexp.MustCompile(`^[a-z_]{1,64}$`)

// Refused records that the provider answered with an error. The code is part of
// the OAuth standard and is what an operator needs to find the cause.
func (p *Provider) Refused(ctx context.Context, state, code string, meta identity.RequestMeta) error {
	if _, err := p.identity.TakeChallenge(ctx, p.pool, challengeKind, state, nil); err != nil && !errors.Is(err, identity.ErrInvalidCredentials) {
		return err
	}
	if !providerError.MatchString(code) {
		code = "unknown"
	}
	return p.identity.RecordLoginFailure(ctx, "oidc", "provider_error:"+code, meta)
}

// Callback finishes a login and opens a session.
func (p *Provider) Callback(ctx context.Context, state, code, binding string, meta identity.RequestMeta) (string, error) {
	fail := func(reason string, err error) (string, error) {
		if rerr := p.identity.RecordLoginFailure(ctx, "oidc", reason, meta); rerr != nil {
			return "", rerr
		}
		return "", err
	}
	var f flow
	if _, err := p.identity.TakeChallenge(ctx, p.pool, challengeKind, state, &f); err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			return fail("bad_state", ErrFailed)
		}
		return "", err
	}
	if binding == "" || binding != f.Binding {
		return fail("bad_binding", ErrFailed)
	}
	verifier, config, err := p.discover(ctx)
	if err != nil {
		return "", err
	}
	tok, err := config.Exchange(ctx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return fail("exchange_failed", ErrFailed)
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return fail("no_id_token", ErrFailed)
	}
	idToken, err := verifier.Verify(ctx, raw)
	if err != nil {
		return fail("bad_token", ErrFailed)
	}
	if idToken.Nonce != f.Nonce {
		return fail("bad_nonce", ErrFailed)
	}
	var c claims
	if err := idToken.Claims(&c); err != nil || c.Subject == "" {
		return fail("bad_claims", ErrFailed)
	}
	var all map[string]any
	if err := idToken.Claims(&all); err != nil {
		return fail("bad_claims", ErrFailed)
	}

	var session string
	err = pgx.BeginFunc(ctx, p.pool, func(tx pgx.Tx) error {
		userID, err := p.resolve(ctx, tx, c, meta)
		if err != nil {
			return err
		}
		if p.cfg.GroupsClaim != "" {
			caller := identity.SystemCaller("oidc")
			caller.Meta = meta
			if err := p.identity.SyncExternalGroups(ctx, tx, caller, userID, identity.SourceOIDC, stringList(all[p.cfg.GroupsClaim])); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, "UPDATE user_identities SET last_login_at = now() WHERE provider = $1 AND subject = $2", p.cfg.Issuer, c.Subject); err != nil {
			return err
		}
		session, _, err = p.identity.OpenSession(ctx, tx, userID, "oidc", meta)
		return err
	})
	switch {
	case errors.Is(err, ErrNoAccount):
		return fail("no_account", err)
	case errors.Is(err, ErrAccountExists):
		return fail("email_in_use", err)
	case errors.Is(err, identity.ErrInvalidCredentials):
		return fail("user_disabled", ErrFailed)
	}
	return session, err
}

func stringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// resolve finds or creates the account. Links are by provider and subject. An
// e-mail address alone never logs anyone in: many providers let users change it.
//
// An account the provider itself created through SCIM is linked on first login by
// the stable id both protocols share: SCIM's externalId equals the token's subject
// (Authentik) or object id (Entra ID, when externalId maps to objectId). Users
// cannot change that id. Providers that share no id can still link on an e-mail
// address they state is verified. Accounts with a password are never linked.
func (p *Provider) resolve(ctx context.Context, tx pgx.Tx, c claims, meta identity.RequestMeta) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, "SELECT user_id FROM user_identities WHERE provider = $1 AND subject = $2", p.cfg.Issuer, c.Subject).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	err = tx.QueryRow(ctx, `SELECT u.id FROM users u
		WHERE u.source = 'scim' AND u.external_id <> '' AND u.external_id IN ($1, $2)
		  AND NOT EXISTS (SELECT 1 FROM local_credentials l WHERE l.user_id = u.id)
		  AND NOT EXISTS (SELECT 1 FROM user_identities i WHERE i.user_id = u.id AND i.provider = $3)`,
		c.Subject, c.ObjectID, p.cfg.Issuer).Scan(&id)
	if err == nil {
		_, err = tx.Exec(ctx, "INSERT INTO user_identities (provider, subject, user_id) VALUES ($1, $2, $3)", p.cfg.Issuer, c.Subject, id)
		return id, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return id, err
	}
	email := strings.ToLower(strings.TrimSpace(c.Email))
	var source string
	var hasPassword bool
	err = tx.QueryRow(ctx, `SELECT u.id, u.source, EXISTS (SELECT 1 FROM local_credentials l WHERE l.user_id = u.id)
		FROM users u WHERE lower(u.email) = $1`, email).Scan(&id, &source, &hasPassword)
	switch {
	case err == nil:
		verified := c.EmailVerified != nil && *c.EmailVerified
		if source != identity.SourceSCIM || hasPassword || !verified {
			return uuid.Nil, ErrAccountExists
		}
	case errors.Is(err, pgx.ErrNoRows):
		if !p.cfg.AutoCreate {
			return uuid.Nil, ErrNoAccount
		}
		caller := identity.SystemCaller("oidc")
		caller.Meta = meta
		name := c.Name
		if name == "" {
			name = c.PreferredUsername
		}
		id, err = p.identity.CreateExternalUser(ctx, tx, caller, identity.External{Email: email, DisplayName: name,
			Username: c.PreferredUsername, Source: identity.SourceOIDC, Active: true})
		if e, ok := apperr.As(err); ok && e.Kind == apperr.KindInvalid {
			return uuid.Nil, ErrNoAccount
		}
		if err != nil {
			return uuid.Nil, err
		}
	default:
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, "INSERT INTO user_identities (provider, subject, user_id) VALUES ($1, $2, $3)", p.cfg.Issuer, c.Subject, id)
	return id, err
}
