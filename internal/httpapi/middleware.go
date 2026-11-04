// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/diffluence/diffluence/internal/identity"
	"github.com/diffluence/diffluence/internal/ids"
)

type ctxKey int

const (
	ctxPrincipal ctxKey = iota
	ctxRequestID
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
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
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

// withLogAndRecover logs every request (path only, never the query string,
// which may hold tokens) and turns panics into 500 responses.
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
				if r.URL.Path == "/livez" || r.URL.Path == "/readyz" {
					level = slog.LevelDebug
				}
				log.Log(r.Context(), level, "http request", "method", r.Method, "path", r.URL.Path,
					"status", rec.status, "duration_ms", time.Since(start).Milliseconds(),
					"ip", ClientIPFrom(r.Context()), "request_id", RequestIDFrom(r.Context()))
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

// withSession loads the principal from the session cookie, if any.
func withSession(svc *identity.Service, cookieName string, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err == nil && c.Value != "" {
				p, err := svc.AuthenticateSession(r.Context(), c.Value)
				switch {
				case err == nil:
					r = r.WithContext(context.WithValue(r.Context(), ctxPrincipal, &p))
				case err != identity.ErrNoSession:
					log.Error("session lookup failed", "error", err, "request_id", RequestIDFrom(r.Context()))
					writeError(w, http.StatusServiceUnavailable, "unavailable", "Please try again shortly.")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
