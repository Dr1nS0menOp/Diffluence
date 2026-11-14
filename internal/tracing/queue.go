// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// Publish starts the span of sending an event and writes its trace into the headers, which
// have the type of NATS headers, a map of lists (D8). The subject names the event type and
// holds no id. Off, it returns ctx, does nothing to the headers and nothing at the end.
func Publish(ctx context.Context, subject string, headers map[string][]string) (context.Context, End) {
	if !On() {
		return ctx, noEnd
	}
	ctx, span := tracer().Start(ctx, "queue publish "+subject, trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject)))
	propagator.Inject(ctx, propagation.HeaderCarrier(http.Header(headers)))
	return ctx, endSpan(span)
}

// Consume starts the span of handling an event, inside the trace its headers carry.
func Consume(ctx context.Context, subject, consumer string, headers map[string][]string) (context.Context, End) {
	if !On() {
		return ctx, noEnd
	}
	ctx = propagator.Extract(ctx, propagation.HeaderCarrier(http.Header(headers)))
	ctx, span := tracer().Start(ctx, "queue consume "+consumer, trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", subject),
			attribute.String("messaging.consumer.group.name", consumer)))
	return ctx, endSpan(span)
}

// traceparentKey is the W3C header that holds the trace and the span of a request.
const traceparentKey = "traceparent"

// Traceparent is the W3C traceparent of the span in ctx, which an event stored in
// PostgreSQL keeps so that its consumer works inside the trace of the request that saved
// it (spec 068, 4.1). It is empty when tracing is off or ctx holds no span.
func Traceparent(ctx context.Context) string {
	if !On() {
		return ""
	}
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	return carrier.Get(traceparentKey)
}

// ConsumeStored starts the span of handling an event taken from the event log, inside the
// trace its traceparent names; an empty traceparent starts a trace of its own.
func ConsumeStored(ctx context.Context, eventType, consumer, traceparent string) (context.Context, End) {
	if !On() {
		return ctx, noEnd
	}
	// An empty or broken traceparent extracts nothing, and the span starts a trace of its own.
	ctx = propagator.Extract(ctx, propagation.MapCarrier{traceparentKey: traceparent})
	ctx, span := tracer().Start(ctx, "queue consume "+consumer, trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("messaging.system", "postgresql"),
			attribute.String("messaging.destination.name", eventType),
			attribute.String("messaging.consumer.group.name", consumer)))
	return ctx, endSpan(span)
}

func endSpan(span trace.Span) End {
	return func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, "error")
		}
		span.End()
	}
}
