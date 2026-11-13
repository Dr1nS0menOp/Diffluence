// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// knownMethods are the methods a span may name. Anything else is _OTHER, so a client
// cannot make names without limit.
var knownMethods = map[string]bool{
	http.MethodGet: true, http.MethodHead: true, http.MethodPost: true, http.MethodPut: true,
	http.MethodPatch: true, http.MethodDelete: true, http.MethodOptions: true,
}

func methodName(m string) string {
	if knownMethods[m] {
		return m
	}
	return "_OTHER"
}

// Middleware makes the server span of every request, continuing a trace that arrived in
// traceparent. Its name is the method until the router names the route (NameRoute); a
// request without a route keeps it (D7). It carries no path, query, address or agent (D6).
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := methodName(r.Method)
		ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer().Start(ctx, method, trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", method)))
		rec := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			span.SetAttributes(attribute.Int("http.response.status_code", rec.status))
			if rec.status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, "error")
			}
			span.End()
		}()
		next.ServeHTTP(rec, r.WithContext(ctx))
	})
}

// NameRoute names the server span of a request after the pattern of its route, as in
// "GET /api/v1/pages/{id}". The pattern holds no id, unlike the path.
func NameRoute(ctx context.Context, pattern string) {
	if !On() {
		return
	}
	span := trace.SpanFromContext(ctx)
	span.SetName(pattern)
	if _, route, found := strings.Cut(pattern, " "); found {
		span.SetAttributes(attribute.String("http.route", route))
	} else {
		span.SetAttributes(attribute.String("http.route", pattern))
	}
}

// statusWriter remembers the status a handler sent.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status, s.wrote = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the writer below, for flushing and deadlines.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }
