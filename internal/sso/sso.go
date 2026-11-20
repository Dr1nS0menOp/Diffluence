// SPDX-License-Identifier: Apache-2.0

// Package sso is OpenID Connect login (spec 004): the authorization code flow
// with PKCE, state and nonce, on top of the go-oidc library.
package sso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"

	"github.com/kenwerk/kenwerk/internal/apperr"
	"github.com/kenwerk/kenwerk/internal/identity"
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
	// AllowUnverifiedEmail lets AutoCreate take an address without email_verified.
	AllowUnverifiedEmail bool
	GroupsClaim          string // empty = do not sync groups
}

// Login failures users can be told about.
var (
	ErrNoAccount     = errors.New("sso: no account for this login")
	ErrAccountExists = errors.New("sso: a local account uses this e-mail address")
	ErrFailed        = errors.New("sso: login failed")
)

// errUnverifiedEmail ends a first login whose address the provider did not verify. The
// person is told what ErrNoAccount tells; the audit log names the reason.
var errUnverifiedEmail = fmt.Errorf("%w: the provider did not verify the address", ErrNoAccount)

// Provider runs logins against one identity provider.
type Provider struct {
	cfg      Config
	pool     *pgxpool.Pool
	identity *identity.Service

	// discoveryTimeout bounds one request to the provider; a field so tests can shorten it.
	discoveryTimeout time.Duration
	now              func() time.Time

	mu       sync.Mutex
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
	// inFlight is closed when the discovery that is running ends; nil when none runs.
	inFlight chan struct{}
	// failedAt and failure remember the last failed discovery for failurePause.
	failedAt time.Time
	failure  error
}

// discoveryTimeout is the longest one request to the provider may take. A provider that
// accepts a connection and then says nothing must not hold a login, or the logins behind it.
const discoveryTimeout = 10 * time.Second

// failurePause is how long a failed discovery is remembered, so a provider that is down
// is asked once in this time and not by every click on "Single sign-on".
const failurePause = 30 * time.Second

// New creates a provider. Discovery happens on first use, so a provider that is
// down cannot stop Kenwerk from starting.
func New(cfg Config, pool *pgxpool.Pool, id *identity.Service) *Provider {
	return &Provider{cfg: cfg, pool: pool, identity: id, discoveryTimeout: discoveryTimeout, now: time.Now}
}

// DisplayName is what the login button says.
func (p *Provider) DisplayName() string { return p.cfg.DisplayName }

// Issuer shows where logins go, for the admin console. It is not a secret.
func (p *Provider) Issuer() string { return p.cfg.Issuer }

// discover finds the provider's endpoints and keys once. The network call runs outside
// the lock, one at a time: logins that arrive meanwhile wait for its answer instead of
// queueing behind the lock, and a failure is remembered for failurePause.
func (p *Provider) discover(ctx context.Context) (*oidc.IDTokenVerifier, *oauth2.Config, error) {
	for {
		p.mu.Lock()
		switch {
		case p.verifier != nil:
			verifier, config := p.verifier, p.oauth
			p.mu.Unlock()
			return verifier, config, nil
		case p.failure != nil && p.now().Sub(p.failedAt) < failurePause:
			err := p.failure
			p.mu.Unlock()
			return nil, nil, err
		case p.inFlight != nil:
			wait := p.inFlight
			p.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, nil, fmt.Errorf("sso: discovery: %w", ctx.Err())
			}
		}
		done := make(chan struct{})
		p.inFlight = done
		p.mu.Unlock()

		verifier, config, err := p.fetch(ctx)

		p.mu.Lock()
		if err != nil {
			p.failure, p.failedAt = err, p.now()
		} else {
			p.verifier, p.oauth, p.failure = verifier, config, nil
		}
		p.inFlight = nil
		close(done)
		p.mu.Unlock()
		return verifier, config, err
	}
}

// fetch asks the provider for its configuration.
func (p *Provider) fetch(ctx context.Context) (*oidc.IDTokenVerifier, *oauth2.Config, error) {
	// Keys are refreshed in the background with this context, so it must outlive the request;
	// the client's timeout bounds every request made with it, discovery and key refresh alike.
	// Discovery uses its own client, outside internal/outward, and follows redirects: the
	// issuer is an address the operator configured, not one a person types (SEC-434).
	client := &http.Client{Timeout: p.discoveryTimeout}
	provider, err := oidc.NewProvider(oidc.ClientContext(context.WithoutCancel(ctx), client), p.cfg.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("sso: discovery: %w", err)
	}
	scopes := []string{oidc.ScopeOpenID, "profile", "email"}
	if p.cfg.GroupsClaim != "" {
		scopes = append(scopes, "groups")
	}
	config := &oauth2.Config{ClientID: p.cfg.ClientID, ClientSecret: p.cfg.ClientSecret, RedirectURL: p.cfg.RedirectURL,
		Endpoint: provider.Endpoint(), Scopes: scopes}
	return provider.Verifier(&oidc.Config{ClientID: p.cfg.ClientID}), config, nil
}

type flow struct {
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Binding  string `json:"binding"`
	// FreshAfter is set for a step-up: the provider must say the person logged in after
	// this moment, in Unix seconds. Zero = an ordinary login.
	FreshAfter int64 `json:"fresh_after,omitempty"`
	// Signing is the token of a signing step this login confirms (DOC-731). Such a login
	// opens no session: it only proves who is at the keyboard now.
	Signing string `json:"signing,omitempty"`
}

// Start begins a login. It returns the URL to send the browser to, and a value
// for a cookie that ties the flow to this browser. That stops an attacker from
// slipping their own login into someone else's browser.
func (p *Provider) Start(ctx context.Context) (redirect, binding string, err error) {
	return p.start(ctx, false)
}

// StartFresh begins a login in which the provider must ask again, also when the
// person is still logged in there. It is the step-up of an account without a
// password in Kenwerk (spec 013, F6).
func (p *Provider) StartFresh(ctx context.Context) (redirect, binding string, err error) {
	// prompt=login asks the provider to ask again, and max_age=0 makes it say when it did
	// (auth_time), so Callback can check that it really asked: a provider may ignore prompt.
	return p.start(ctx, true, oauth2.SetAuthURLParam("prompt", "login"), oauth2.SetAuthURLParam("max_age", "0"))
}

// StartSigning begins a login that confirms a signing step (DOC-731): the provider must ask
// again, and say it did after the step began.
func (p *Provider) StartSigning(ctx context.Context, token string, began time.Time) (redirect, binding string, err error) {
	f := p.newFlow()
	f.FreshAfter, f.Signing = began.Add(-freshSkew).Unix(), token
	return p.startFlow(ctx, f, oauth2.SetAuthURLParam("prompt", "login"), oauth2.SetAuthURLParam("max_age", "0"))
}

func (p *Provider) newFlow() flow {
	return flow{Nonce: oauth2.GenerateVerifier(), Verifier: oauth2.GenerateVerifier(), Binding: oauth2.GenerateVerifier()}
}

func (p *Provider) start(ctx context.Context, fresh bool, extra ...oauth2.AuthCodeOption) (redirect, binding string, err error) {
	f := p.newFlow()
	if fresh {
		f.FreshAfter = time.Now().Add(-freshSkew).Unix()
	}
	return p.startFlow(ctx, f, extra...)
}

func (p *Provider) startFlow(ctx context.Context, f flow, extra ...oauth2.AuthCodeOption) (redirect, binding string, err error) {
	_, config, err := p.discover(ctx)
	if err != nil {
		return "", "", err
	}
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
	AuthTime          int64  `json:"auth_time"`
}

// freshSkew allows for clocks that differ a little between the provider and this server.
const freshSkew = time.Minute

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

// Result is how a return from the provider ended: a session for a login, or, for a login
// that confirmed a signing step, where the browser goes back to.
type Result struct {
	Session string
	// Signing says the login was for a signing step; SigningReturn is its page.
	Signing       bool
	SigningReturn string
}

// Complete finishes a return from the provider: a login opens a session, and a login for a
// signing step confirms that step and opens nothing (DOC-731).
func (p *Provider) Complete(ctx context.Context, state, code, binding string, meta identity.RequestMeta) (Result, error) {
	var f flow
	if _, err := p.identity.TakeChallenge(ctx, p.pool, challengeKind, state, &f); err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			if rerr := p.identity.RecordLoginFailure(ctx, "oidc", "bad_state", meta); rerr != nil {
				return Result{}, rerr
			}
			return Result{}, ErrFailed
		}
		return Result{}, err
	}
	if f.Signing == "" {
		session, err := p.login(ctx, f, code, binding, meta)
		return Result{Session: session}, err
	}
	back, err := p.confirmSigning(ctx, f, code, binding, meta)
	if err != nil {
		// A refused step still goes back to its page, which says so.
		var lookup error
		if back, lookup = p.identity.SigningReturnPath(ctx, f.Signing); lookup != nil {
			return Result{}, lookup
		}
	}
	return Result{Signing: true, SigningReturn: back}, err
}

// confirmSigning checks the provider's answer for a signing step: the account it names must
// be the one that began the step, and its login must be fresh (DOC-731). An account is never
// made or linked here.
func (p *Provider) confirmSigning(ctx context.Context, f flow, code, binding string, meta identity.RequestMeta) (string, error) {
	c, _, reason, err := p.verify(ctx, f, code, binding)
	if err != nil {
		return "", err
	}
	if reason != "" {
		return "", p.recordSigningFailure(ctx, reason, meta)
	}
	var userID uuid.UUID
	err = p.pool.QueryRow(ctx, "SELECT user_id FROM user_identities WHERE provider = $1 AND subject = $2", p.cfg.Issuer, c.Subject).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", p.recordSigningFailure(ctx, "no_account", meta)
	}
	if err != nil {
		return "", err
	}
	back, err := p.identity.ConfirmSigningByProvider(ctx, f.Signing, userID, meta)
	if e, ok := apperr.As(err); ok && e.Kind == apperr.KindForbidden {
		return "", p.recordSigningFailure(ctx, "other_account", meta)
	}
	return back, err
}

func (p *Provider) recordSigningFailure(ctx context.Context, reason string, meta identity.RequestMeta) error {
	if err := p.identity.RecordLoginFailure(ctx, "oidc_signing", reason, meta); err != nil {
		return err
	}
	return ErrFailed
}

// verify checks the provider's answer to a flow. It answers the reason of a refusal, which
// the caller records, or an error it could not check.
func (p *Provider) verify(ctx context.Context, f flow, code, binding string) (claims, map[string]any, string, error) {
	var c claims
	if binding == "" || binding != f.Binding {
		return c, nil, "bad_binding", nil
	}
	verifier, config, err := p.discover(ctx)
	if err != nil {
		return c, nil, "", err
	}
	tok, err := config.Exchange(ctx, code, oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return c, nil, "exchange_failed", nil
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return c, nil, "no_id_token", nil
	}
	idToken, err := verifier.Verify(ctx, raw)
	if err != nil {
		return c, nil, "bad_token", nil
	}
	if idToken.Nonce != f.Nonce {
		return c, nil, "bad_nonce", nil
	}
	if err := idToken.Claims(&c); err != nil || c.Subject == "" {
		return c, nil, "bad_claims", nil
	}
	if f.FreshAfter != 0 && c.AuthTime < f.FreshAfter {
		return c, nil, "not_fresh", nil
	}
	var all map[string]any
	if err := idToken.Claims(&all); err != nil {
		return c, nil, "bad_claims", nil
	}
	return c, all, "", nil
}

// login finishes a login and opens a session.
func (p *Provider) login(ctx context.Context, f flow, code, binding string, meta identity.RequestMeta) (string, error) {
	fail := func(reason string, err error) (string, error) {
		if rerr := p.identity.RecordLoginFailure(ctx, "oidc", reason, meta); rerr != nil {
			return "", rerr
		}
		return "", err
	}
	c, all, reason, err := p.verify(ctx, f, code, binding)
	if err != nil {
		return "", err
	}
	if reason != "" {
		return fail(reason, ErrFailed)
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
	case errors.Is(err, errUnverifiedEmail):
		return fail("email_not_verified", ErrNoAccount)
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
		// The new account takes the address, so the provider must vouch for it: else anyone
		// could claim the address of a colleague who has not logged in yet.
		verified := c.EmailVerified != nil && *c.EmailVerified
		if !verified && !p.cfg.AllowUnverifiedEmail {
			return uuid.Nil, errUnverifiedEmail
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
