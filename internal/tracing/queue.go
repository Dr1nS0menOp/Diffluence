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

func endSpan(span trace.Span) End {
	return func(err error) {
		if err != nil {
			span.SetStatus(codes.Error, "error")
		}
		span.End()
	}
}
