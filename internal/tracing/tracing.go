// SPDX-License-Identifier: Apache-2.0

// Package tracing exports traces with OpenTelemetry over OTLP when the operator switches it
// on (spec 060). It is off by default, and then nothing of it runs: no middleware, no query
// tracer, no headers on events, no wrapping of the log (D5).
//
// Spans carry names and identifiers of kinds, never content (D6): the route pattern and
// never the path, the SQL operation and table and never the statement, the event type and
// never the payload. An error sets the status without its message.
package tracing

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// on says whether tracing runs in this process. Start sets it once, before anything serves;
// it and OpenTelemetry's global tracer provider are the one process-wide state of tracing,
// because handing a tracer to every module would touch every constructor for the same
// effect (spec 060 D5).
var on atomic.Bool

// On reports whether tracing runs. Every helper asks it first, so off costs nothing.
func On() bool { return on.Load() }

// tracerName is the instrumentation scope of every span.
const tracerName = "github.com/diffluence/diffluence"

func tracer() trace.Tracer { return otel.Tracer(tracerName) }

// propagator reads and writes W3C traceparent and tracestate (D8).
var propagator = propagation.TraceContext{}

// Options says how to trace. The address of the collector and its headers come from the
// standard OTEL_EXPORTER_OTLP_* variables, which the exporter reads itself (D2).
type Options struct {
	Enabled bool
	// Sample is the share of new traces kept, from 0 to 1 (D4).
	Sample  float64
	Version string
	Role    string
	// Log receives export failures, at most once a minute.
	Log *slog.Logger
	// Exporter replaces OTLP over HTTP and sends every span at once. Tests use it.
	Exporter sdktrace.SpanExporter
}

// Start switches tracing on when the options ask for it, and returns a function that flushes
// what waits and switches it off again. Off, it changes nothing and the function does nothing.
func Start(ctx context.Context, o Options) (func(context.Context) error, error) {
	if !o.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	processor, err := spanProcessor(ctx, o.Exporter)
	if err != nil {
		return nil, err
	}
	// The variables come last, so OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES win (D2).
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", "diffluence"),
			attribute.String("service.version", o.Version),
			attribute.String("diffluence.role", o.Role)),
		resource.WithFromEnv())
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(processor),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(o.Sample))))
	if o.Log != nil {
		otel.SetErrorHandler(&quietErrors{log: o.Log, every: time.Minute})
	}
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagator)
	on.Store(true)
	return func(ctx context.Context) error {
		on.Store(false)
		otel.SetTracerProvider(noop.NewTracerProvider())
		return provider.Shutdown(ctx)
	}, nil
}

// spanProcessor sends spans in batches to the OTLP collector, or one by one to the exporter
// a test gave.
func spanProcessor(ctx context.Context, exporter sdktrace.SpanExporter) (sdktrace.SpanProcessor, error) {
	if exporter != nil {
		return sdktrace.NewSimpleSpanProcessor(exporter), nil
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	// The batcher drops spans when its queue is full and never blocks a request.
	return sdktrace.NewBatchSpanProcessor(exp), nil
}

// quietErrors logs what OpenTelemetry could not do, at most once per period, so a collector
// that is down does not fill the log.
type quietErrors struct {
	log   *slog.Logger
	every time.Duration
	mu    sync.Mutex
	last  time.Time
}

func (q *quietErrors) Handle(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if now := time.Now(); now.Sub(q.last) >= q.every {
		q.last = now
		q.log.Warn("tracing: spans could not be sent", "error", err)
	}
}

// End finishes a span. A failure sets the span's status without the error's message, which
// can quote a value (D6).
type End func(err error)

func noEnd(error) {}

// Span starts a span named what, or what and which, as in "job page.export". Off, it
// returns ctx and does nothing, without allocating.
func Span(ctx context.Context, what, which string) (context.Context, End) {
	if !On() {
		return ctx, noEnd
	}
	name := what
	if which != "" {
		name = what + " " + which
	}
	ctx, span := tracer().Start(ctx, name)
	return ctx, func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, "error")
		}
		span.End()
	}
}

// Count records a number on the current span, such as how many audit events were sealed.
func Count(ctx context.Context, key string, n int) {
	if !On() {
		return
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int(key, n))
}
