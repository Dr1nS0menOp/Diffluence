// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// The load budget of spec 060 (ARC-238): what a request and a query cost with tracing off,
// at the default share and with every trace kept. Run with:
//   go test ./internal/tracing -run XXX -bench . -benchmem

func benchStart(b *testing.B, share float64) {
	b.Helper()
	if share < 0 {
		return
	}
	stop, err := Start(context.Background(), Options{Enabled: true, Sample: share, Exporter: tracetest.NewNoopExporter()})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = stop(context.Background()) })
}

var noContent = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

func benchRequest(b *testing.B, share float64) {
	benchStart(b, share)
	handler := http.Handler(noContent)
	if On() {
		handler = Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			NameRoute(r.Context(), "GET /api/v1/pages/{id}")
			noContent(w, r)
		}))
	}
	req := httptest.NewRequest("GET", "/api/v1/pages/x", nil)
	b.ReportAllocs()
	for b.Loop() {
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkRequestTracingOff(b *testing.B)      { benchRequest(b, -1) }
func BenchmarkRequestTracingTenth(b *testing.B)    { benchRequest(b, 0.1) }
func BenchmarkRequestTracingEveryOne(b *testing.B) { benchRequest(b, 1) }

func benchQuery(b *testing.B, share float64) {
	benchStart(b, share)
	tracer := QueryTracer()
	data := pgx.TraceQueryStartData{SQL: "SELECT id, title FROM pages WHERE id = $1"}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if tracer != nil {
			end := tracer.TraceQueryStart(ctx, nil, data)
			tracer.TraceQueryEnd(end, nil, pgx.TraceQueryEndData{})
		}
	}
}

// A query of a request whose trace is not kept, as nine in ten are at the default share.
func BenchmarkQueryInATraceNotKept(b *testing.B) {
	benchStart(b, 0.1)
	tracer := QueryTracer()
	data := pgx.TraceQueryStartData{SQL: "SELECT id, title FROM pages WHERE id = $1"}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	b.ReportAllocs()
	for b.Loop() {
		end := tracer.TraceQueryStart(ctx, nil, data)
		tracer.TraceQueryEnd(end, nil, pgx.TraceQueryEndData{})
	}
}

func BenchmarkQueryTracingOff(b *testing.B)      { benchQuery(b, -1) }
func BenchmarkQueryTracingTenth(b *testing.B)    { benchQuery(b, 0.1) }
func BenchmarkQueryTracingEveryOne(b *testing.B) { benchQuery(b, 1) }
