// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
	"github.com/diffluence/diffluence/internal/metrics"
	"github.com/diffluence/diffluence/internal/settings"
)

type ctxKey int

const (
	ctxPrincipal ctxKey = iota
	ctxRequestID
	ctxTraceID
	ctxClientIP
	ctxSCIMCaller
)

// PrincipalFrom returns the authenticated principal, or nil.
func PrincipalFrom(ctx context.Context) *identity.Principal {
	p, _ := ctx.Value(ctxPrincipal).(*identity.Principal)
	return p
}

// RequestIDFrom returns the request id.
func RequestIDFrom(ctx context.Context) string { s, _ := ctx.Value(ctxRequestID).(string); return s }

// ClientIPFrom returns the client IP (after trusted-proxy handling).
func ClientIPFrom(ctx context.Context) string { s, _ := ctx.Value(ctxClientIP).(string); return s }

func callerFromSCIM(r *http.Request) identity.Caller {
	c, _ := r.Context().Value(ctxSCIMCaller).(identity.Caller)
	return c
}

func metaFrom(r *http.Request) identity.RequestMeta {
	ua := r.UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	return identity.RequestMeta{IP: ClientIPFrom(r.Context()), UserAgent: ua, RequestID: RequestIDFrom(r.Context())}
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ids.New().String()
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), ctxRequestID, id)
		ctx = context.WithValue(ctx, ctxTraceID, traceIDFrom(r.Header.Get("traceparent")))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// isLowerHex reports whether s is lower-case hex with at least one digit that is
// not zero, which is what W3C Trace Context asks of an id.
func isLowerHex(s string) bool {
	hasValue := false
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
		hasValue = hasValue || c != '0'
	}
	return hasValue
}

// traceIDFrom reads the trace id of a W3C traceparent header. The header comes from
// outside, so anything that does not fit gives no id and no error (spec 021 D5).
func traceIDFrom(header string) string {
	parts := strings.Split(header, "-")
	if len(parts) < 4 {
		return ""
	}
	version, traceID, parentID, flags := parts[0], parts[1], parts[2], parts[3]
	validVersion := len(version) == 2 && version != "ff" && (isLowerHex(version) || version == "00")
	if !validVersion || (version == "00" && len(parts) != 4) {
		return ""
	}
	if len(traceID) != 32 || !isLowerHex(traceID) || len(parentID) != 16 || !isLowerHex(parentID) {
		return ""
	}
	if len(flags) != 2 || (!isLowerHex(flags) && flags != "00") {
		return ""
	}
	return traceID
}

// withMetrics counts every request, by method and class of status only: a path
// would give a label without limit, and holds ids (spec 021 D3).
func withMetrics(requests *metrics.Requests) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			defer func() { requests.Observe(r.Method, rec.finalStatus(), time.Since(start)) }()
			next.ServeHTTP(rec, r)
		})
	}
}

// withClientIP takes the client address from X-Forwarded-For only when the
// direct peer is a trusted proxy, walking right-to-left past trusted hops.
func withClientIP(trusted []netip.Prefix) func(http.Handler) http.Handler {
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a.Unmap()) {
				return true
			}
		}
		return false
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := peerAddr(r.RemoteAddr)
			if ip.IsValid() && isTrusted(ip) {
				hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
				for i := len(hops) - 1; i >= 0; i-- {
					a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
					if err != nil {
						break
					}
					ip = a
					if !isTrusted(a) {
						break
					}
				}
			}
			s := ""
			if ip.IsValid() {
				s = ip.Unmap().String()
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxClientIP, s)))
		})
	}
}

func peerAddr(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	a, _ := netip.ParseAddr(host)
	return a
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status, s.wrote = http.StatusOK, true
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// finalStatus is the status sent. A handler that wrote nothing sent a 200.
func (s *statusRecorder) finalStatus() int {
	if !s.wrote {
		return http.StatusOK
	}
	return s.status
}

// tokenPaths are paths that end in a secret. A proxy or a log server must not get it.
var tokenPaths = []string{"/api/v1/mail/unsubscribe/", "/unsubscribe/", "/hooks/"}

// loggedPath is the path for the log: it keeps the route, and leaves out a token.
func loggedPath(path string) string {
	for _, prefix := range tokenPaths {
		if strings.HasPrefix(path, prefix) {
			return prefix + "{token}"
		}
	}
	return path
}

// withLogAndRecover logs every request (path only, never the query string, and never a
// token in the path) and turns panics into 500 responses.
func withLogAndRecover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					log.Error("panic in handler", "panic", v, "stack", string(debug.Stack()), "request_id", RequestIDFrom(r.Context()))
					if !rec.wrote {
						writeError(rec, http.StatusInternalServerError, "internal_error", "Something went wrong.")
					}
				}
				level := slog.LevelInfo
				if r.URL.Path == "/livez" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
					level = slog.LevelDebug
				}
				attrs := []any{"method", r.Method, "path", loggedPath(r.URL.Path),
					"status", rec.status, "duration_ms", time.Since(start).Milliseconds(),
					"ip", ClientIPFrom(r.Context()), "request_id", RequestIDFrom(r.Context())}
				if traceID, _ := r.Context().Value(ctxTraceID).(string); traceID != "" {
					attrs = append(attrs, "trace_id", traceID)
				}
				// The handler writes JSON, which escapes a line break in the path, so a path
				// cannot fake a log line (SEC-840).
				// codeql[go/log-injection]
				log.Log(r.Context(), level, "http request", attrs...)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// withSecurityHeaders sets defensive headers on every response. The API serves
// JSON only, so the CSP forbids everything; the frontend will get its own policy.
func withSecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
			h.Set("Cache-Control", "no-store")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// backgroundHeader marks a request an open tab makes by itself. It does not count
// as activity, or polling would keep every open tab logged in for ever (D8).
const backgroundHeader = "X-Background"

func (s *Server) principalFromCookie(r *http.Request) (identity.Principal, error) {
	c, err := r.Cookie(s.cookieName)
	if err != nil || c.Value == "" {
		return identity.Principal{}, identity.ErrNoSession
	}
	if r.Header.Get(backgroundHeader) != "" {
		return s.deps.Identity.PeekSession(r.Context(), c.Value)
	}
	return s.deps.Identity.AuthenticateSession(r.Context(), c.Value)
}

func (s *Server) principalFromToken(r *http.Request) (identity.Principal, error) {
	secret, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || !identity.IsToken(secret) {
		return identity.Principal{}, identity.ErrNoSession
	}
	features := settings.DefaultFeatures
	if err := s.deps.Settings.ReadSection(r.Context(), settings.SectionFeatures, &features); err != nil {
		return identity.Principal{}, err
	}
	if !features.Tokens {
		return identity.Principal{}, identity.ErrNoSession
	}
	return s.deps.Identity.AuthenticateToken(r.Context(), secret)
}

// withAuthentication loads the principal from the session cookie or a token.
func (s *Server) withAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.principalFromCookie(r)
		if errors.Is(err, identity.ErrNoSession) {
			p, err = s.principalFromToken(r)
		}
		switch {
		case err == nil:
			r = r.WithContext(context.WithValue(r.Context(), ctxPrincipal, &p))
		case !errors.Is(err, identity.ErrNoSession):
			s.deps.Log.Error("session lookup failed", "error", err, "request_id", RequestIDFrom(r.Context()))
			writeError(w, http.StatusServiceUnavailable, "unavailable", "Please try again shortly.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// openDuringMaintenance lists what still works: logging in and out, and switching
// maintenance mode off again.
var openDuringMaintenance = []string{
	"/api/v1/auth/login", "/api/v1/auth/login/totp", "/api/v1/auth/logout", "/api/v1/auth/reauth",
	"/api/v1/auth/reauth/passkey/begin", "/api/v1/auth/reauth/passkey/finish",
	"/api/v1/auth/passkey/begin", "/api/v1/auth/passkey/finish", "/api/v1/admin/maintenance",
}

// withMaintenance refuses every write during maintenance mode. It is a check here
// and not a rule in Evaluate: it concerns every write, also from SCIM, and not a
// person's rights (spec 013, D17).
func (s *Server) withMaintenance(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isRead(r) || slices.Contains(openDuringMaintenance, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		state, err := s.deps.Settings.Maintenance(r.Context())
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		if state.On {
			writeError(w, http.StatusServiceUnavailable, "maintenance", "Diffluence is in maintenance. Your text stays here. Try again shortly.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
