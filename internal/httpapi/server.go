// SPDX-License-Identifier: Apache-2.0

// Package httpapi is the HTTP layer: the router with mandatory access rules,
// middleware, and the /api/v1 handlers.
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/diffluence/diffluence/internal/analytics"
	"github.com/diffluence/diffluence/internal/attachments"
	"github.com/diffluence/diffluence/internal/audit"
	"github.com/diffluence/diffluence/internal/automation"
	"github.com/diffluence/diffluence/internal/blocks"
	"github.com/diffluence/diffluence/internal/collab"
	"github.com/diffluence/diffluence/internal/comments"
	"github.com/diffluence/diffluence/internal/config"
	"github.com/diffluence/diffluence/internal/console"
	"github.com/diffluence/diffluence/internal/example"
	"github.com/diffluence/diffluence/internal/exchange"
	"github.com/diffluence/diffluence/internal/failures"
	"github.com/diffluence/diffluence/internal/features"
	"github.com/diffluence/diffluence/internal/feed"
	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/jobs"
	"github.com/diffluence/diffluence/internal/mail"
	"github.com/diffluence/diffluence/internal/metrics"
	"github.com/diffluence/diffluence/internal/notify"
	"github.com/diffluence/diffluence/internal/outward"
	"github.com/diffluence/diffluence/internal/pages"
	"github.com/diffluence/diffluence/internal/people"
	"github.com/diffluence/diffluence/internal/permissions"
	"github.com/diffluence/diffluence/internal/presence"
	"github.com/diffluence/diffluence/internal/savedblocks"
	"github.com/diffluence/diffluence/internal/scim"
	"github.com/diffluence/diffluence/internal/search"
	"github.com/diffluence/diffluence/internal/settings"
	"github.com/diffluence/diffluence/internal/setup"
	"github.com/diffluence/diffluence/internal/sharing"
	"github.com/diffluence/diffluence/internal/sites"
	"github.com/diffluence/diffluence/internal/spaces"
	"github.com/diffluence/diffluence/internal/sso"
	"github.com/diffluence/diffluence/internal/tasks"
	"github.com/diffluence/diffluence/internal/templates"
	"github.com/diffluence/diffluence/internal/trash"
	"github.com/diffluence/diffluence/internal/watches"
	"github.com/diffluence/diffluence/internal/webhooks"
)

// tokenCallsPerMinute limits one token, so a runaway script cannot take the server.
const tokenCallsPerMinute = 600

// Deps are what the API needs from the rest of the application.
type Deps struct {
	Config   config.Config
	Pool     *pgxpool.Pool
	Identity *identity.Service

	Permissions *permissions.Service
	Spaces      *spaces.Service
	Pages       *pages.Service
	Attachments *attachments.Service
	Blocks      *blocks.Service
	People      *people.Service
	Console     *console.Service
	Search      *search.Service
	Trash       *trash.Service
	Settings    *settings.Service
	Features    *features.Service
	Templates   *templates.Service
	Jobs        *jobs.Service
	Exchange    *exchange.Service
	Sites       *sites.Service
	Example     *example.Service
	SCIM        *scim.Service
	SSO         *sso.Provider // nil when no OIDC provider is configured
	Comments    *comments.Service
	Watches     *watches.Service
	Tasks       *tasks.Service
	Notify      *notify.Service
	Mail        *mail.Service
	Sharing     *sharing.Service
	SavedBlocks *savedblocks.Service
	Feed        *feed.Service
	Analytics   *analytics.Service
	Presence    *presence.Service
	Outward     *outward.Service
	Automation  *automation.Service
	Webhooks    *webhooks.Service
	Collab      *collab.Service
	Failures    *failures.Service
	Setup       *setup.Service

	// SystemStatus reports nodes, versions and queues for the admin console.
	SystemStatus func(ctx context.Context) (any, error)
	// HelpersDown names the optional helpers that did not answer the last check (ARC-072).
	HelpersDown func(ctx context.Context) (map[string]bool, error)
	// Gauges reads the numbers that /metrics shows beside the request counters.
	Gauges func(ctx context.Context) []metrics.Gauge
	// CheckAudit verifies the audit chain and the signed checkpoints.
	CheckAudit func(ctx context.Context) (audit.VerifyResult, error)

	InstanceID uuid.UUID
	Log        *slog.Logger
	// Ready returns nil when this node may receive traffic, or an error whose
	// message is a short reason code ("database", "queue", "migrations", "draining").
	Ready func(ctx context.Context) error
}

// Server is the API handler.
type Server struct {
	deps       Deps
	router     *Router
	handler    http.Handler
	cookieName string
	loginByIP  *limiter
	loginByAcc *limiter
	// loginByAllIPs counts tries on one account from every address together.
	loginByAllIPs *limiter
	tokenCalls    *limiter
	// setupTries limits tries at the first setup per address (spec 038).
	setupTries *limiter
	requests   *metrics.Requests
	// markdownBusy holds a place for every paste being converted.
	markdownBusy chan struct{}
	// admission keeps the order of work when the database is full (ARC-233).
	admission *admission
}

// New builds the API.
func New(d Deps) (*Server, error) {
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	s := &Server{
		deps:       d,
		cookieName: "diffluence_session",
		loginByIP:  newLimiter(50, loginWindow),
		loginByAcc: newLimiter(10, loginWindow),
		tokenCalls: newLimiter(tokenCallsPerMinute, time.Minute),
		requests:   metrics.NewRequests(),

		loginByAllIPs: newLimiter(100, loginWindow),
		setupTries:    newLimiter(setupTriesPerAddress, setupWindow),
		markdownBusy:  make(chan struct{}, markdownSlots),
		admission:     newAdmission(poolSize(d)),
	}
	if d.Config.SecureCookies() {
		// The __Host- prefix makes browsers refuse the cookie unless it is
		// Secure, has Path=/ and no Domain, which blocks subdomain overrides.
		s.cookieName = "__Host-diffluence_session"
	}
	s.router = newRouter(s.enforce)
	s.registerRoutes()

	cop := http.NewCrossOriginProtection()
	if err := cop.AddTrustedOrigin(d.Config.PublicOrigin()); err != nil {
		return nil, err
	}
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "cross_origin_blocked", "Cross-origin requests are not allowed.")
	}))

	s.handler = chain(s.router,
		withRequestID,
		withMetrics(s.requests),
		withClientIP(d.Config.TrustedProxies),
		withLogAndRecover(d.Log),
		s.admission.middleware,
		withSecurityHeaders(d.Config.SecureCookies()),
		cop.Handler,
		s.withAuthentication,
		withPermissionMemo,
		// After authentication, so a session is always read current (ARC-228).
		withReadFreshness,
		s.withMaintenance,
	)
	return s, nil
}

// Handler returns the complete HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Routes lists every registered route with its access rule.
func (s *Server) Routes() []Route { return s.router.Routes() }

// enforce wraps a handler with its access rule.
func (s *Server) enforce(a Access, h http.Handler) http.Handler {
	base := s.enforceKind(a, h)
	if a.kind == accessPublic || a.kind == accessSCIM {
		return base
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := PrincipalFrom(r.Context()); p != nil && !s.allowedByCredential(w, r, a, p) {
			return
		}
		base.ServeHTTP(w, r)
	})
}

// secondFactorRoutes stay open to a person who still has to set up a second factor.
var secondFactorRoutes = []string{"/api/v1/me", "/api/v1/auth/"}

// allowedByCredential checks what the kind of credential allows, before the rule
// of the route. It answers the request when it refuses.
func (s *Server) allowedByCredential(w http.ResponseWriter, r *http.Request, a Access, p *identity.Principal) bool {
	switch {
	case p.ViaToken && a.sessionOnly:
		writeError(w, http.StatusForbidden, "session_only", "This needs a login in the browser, not a token.")
	case p.ViaToken && !s.tokenMayGoOn(w, r, p):
		return false
	case p.NeedsSecondFactor && !slices.ContainsFunc(secondFactorRoutes, func(prefix string) bool { return strings.HasPrefix(r.URL.Path, prefix) }):
		writeError(w, http.StatusForbidden, "second_factor_required", "Set up a second factor first.")
	case a.sensitive && time.Since(p.ConfirmedAt) > identity.ReauthWindow:
		writeError(w, http.StatusForbidden, "reauth_required", "Confirm who you are to do this.")
	default:
		return true
	}
	return false
}

// scopeFor maps a request to the scope a token needs (spec 015, H9).
func scopeFor(r *http.Request) string {
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/v1/admin/"):
		return identity.ScopeAdmin
	case strings.Contains(r.URL.Path, "/export"):
		return identity.ScopeExport
	case isRead(r):
		return identity.ScopeRead
	}
	return identity.ScopeWrite
}

// tokenMayGoOn applies the limit per minute and the scope of a token, and answers
// the request when it refuses. A refusal is audited: a script has gone wrong, or
// someone is trying a stolen token.
func (s *Server) tokenMayGoOn(w http.ResponseWriter, r *http.Request, p *identity.Principal) bool {
	if !s.tokenCalls.allow(p.TokenID.String()) {
		writeError(w, http.StatusTooManyRequests, "too_many_requests", "This token made too many calls. Wait a minute.")
		return false
	}
	scope := scopeFor(r)
	if slices.Contains(p.Scopes, scope) {
		return true
	}
	if err := s.deps.Identity.RecordTokenRefused(r.Context(), s.caller(r), p.TokenID, scope); err != nil {
		s.fail(w, r, err)
		return false
	}
	if scope == identity.ScopeWrite {
		writeError(w, http.StatusForbidden, "token_read_only", "This token can only read.")
		return false
	}
	writeError(w, http.StatusForbidden, "token_scope", "This token does not have the scope "+scope+".")
	return false
}

func isRead(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions
}

func (s *Server) enforceKind(a Access, h http.Handler) http.Handler {
	switch a.kind {
	case accessPublic:
		return h
	case accessAuthenticated, accessObject:
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if PrincipalFrom(r.Context()) == nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated", "Please log in.")
				return
			}
			h.ServeHTTP(w, r)
		})
	case accessPermission:
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFrom(r.Context())
			if p == nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated", "Please log in.")
				return
			}
			ok, err := s.deps.Identity.HasGlobalPermission(r.Context(), p.User.ID, a.permission)
			if err != nil {
				s.internalError(w, r, err)
				return
			}
			if !ok {
				if err := s.deps.Identity.RecordAccessDenied(r.Context(), *p, a.permission, r.Method, r.URL.Path, metaFrom(r)); err != nil {
					s.internalError(w, r, err)
					return
				}
				writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to do this.")
				return
			}
			h.ServeHTTP(w, r)
		})
	case accessSCIM:
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := s.deps.SCIM.Authenticate(r)
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer")
				scim.WriteError(w, http.StatusUnauthorized, "", "A valid bearer token is required.")
				return
			}
			c.Meta = metaFrom(r)
			h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxSCIMCaller, c)))
		})
	default:
		panic("httpapi: unknown access kind")
	}
}

func (s *Server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.deps.Log.Error("request failed", "error", err, "request_id", RequestIDFrom(r.Context()))
	writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong.")
}

// defaultPoolSize is used when there is no pool to measure, as in some tests.
const defaultPoolSize = 20

func poolSize(d Deps) int {
	if d.Pool == nil {
		return defaultPoolSize
	}
	return int(d.Pool.Config().MaxConns)
}

// ready adds the queues of this node to the checks of the app: a node with a long
// queue says busy, so a load balancer sends people elsewhere (ARC-235).
func (s *Server) ready(ctx context.Context) error {
	if s.admission.overloaded() {
		return ReadyError("busy")
	}
	return s.deps.Ready(ctx)
}

// HealthOnly serves just /livez and /readyz, for processes with role=worker.
func HealthOnly(ready func(context.Context) error, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", handleLivez)
	mux.HandleFunc("GET /readyz", readyzHandler(ready, log))
	return mux
}

func handleLivez(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func readyzHandler(ready func(context.Context) error, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ready(r.Context()); err != nil {
			reason := "unknown"
			var re ReadyError
			if errors.As(err, &re) {
				reason = string(re)
			} else if log != nil {
				log.Warn("readiness check failed", "error", err)
			}
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "reason": reason})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// ReadyError is a short, safe-to-expose readiness reason.
type ReadyError string

func (e ReadyError) Error() string { return string(e) }
