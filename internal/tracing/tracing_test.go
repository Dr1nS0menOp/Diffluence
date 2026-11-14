// SPDX-License-Identifier: Apache-2.0

package tracing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const (
	incomingTrace  = "4bf92f3577b34da6a3ce929d0e0e4736"
	incomingParent = "00f067aa0ba902b7"
)

// record switches tracing on for one test, keeping every span, and switches it off after.
func record(t *testing.T, share float64) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	stop, err := Start(context.Background(), Options{Enabled: true, Sample: share, Exporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return exporter
}

func only(t *testing.T, exporter *tracetest.InMemoryExporter) tracetest.SpanStub {
	t.Helper()
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("%d spans, want 1: %v", len(spans), spans)
	}
	return spans[0]
}

func attrs(s tracetest.SpanStub) map[string]string {
	out := map[string]string{}
	for _, kv := range s.Attributes {
		out[string(kv.Key)] = kv.Value.String()
	}
	return out
}

// D5: off, nothing of tracing runs, and nothing allocates.
func TestOffDoesNothingAndChangesNothing(t *testing.T) {
	stop, err := Start(context.Background(), Options{Enabled: false, Sample: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if On() {
		t.Fatal("tracing is on after a start that was off")
	}
	ctx := context.Background()
	if got, _ := Span(ctx, "job", "x"); got != ctx {
		t.Error("Span changed the context while off")
	}
	if QueryTracer() != nil {
		t.Error("a query tracer while off")
	}
	h := slog.NewJSONHandler(&bytes.Buffer{}, nil)
	if LogHandler(h) != slog.Handler(h) {
		t.Error("the log handler is wrapped while off")
	}
	headers := map[string][]string{}
	_, end := Publish(ctx, "diffluence.events.page.created", headers)
	end(nil)
	if len(headers) != 0 {
		t.Errorf("headers written while off: %v", headers)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		_, end := Span(ctx, "job", "page.export")
		end(nil)
	}); allocs != 0 {
		t.Errorf("Span allocates %v times while off", allocs)
	}
}

// D6, D7: a request's span is named after its route pattern and carries no path, query, id,
// address or agent; it continues the trace that arrived in traceparent.
func TestARequestSpanIsNamedByItsRouteAndHoldsNoPath(t *testing.T) {
	exporter := record(t, 1)
	const id = "0193a2b4-1c2d-7e8f-9a0b-1c2d3e4f5a6b"
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		NameRoute(r.Context(), "GET /api/v1/pages/{id}")
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest("GET", "/api/v1/pages/"+id+"?q=private-words", nil)
	req.Header.Set("traceparent", "00-"+incomingTrace+"-"+incomingParent+"-01")
	req.Header.Set("User-Agent", "agent-of-someone")
	req.RemoteAddr = "192.0.2.7:1234"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	span := only(t, exporter)
	if span.Name != "GET /api/v1/pages/{id}" || span.SpanKind != trace.SpanKindServer {
		t.Fatalf("name %q kind %v", span.Name, span.SpanKind)
	}
	if span.SpanContext.TraceID().String() != incomingTrace || span.Parent.SpanID().String() != incomingParent {
		t.Errorf("the trace of traceparent was not continued: %v under %v", span.SpanContext.TraceID(), span.Parent.SpanID())
	}
	got := attrs(span)
	if got["http.route"] != "/api/v1/pages/{id}" || got["http.request.method"] != "GET" || got["http.response.status_code"] != "204" {
		t.Errorf("attributes %v", got)
	}
	for key, value := range got {
		for _, private := range []string{id, "private-words", "agent-of-someone", "192.0.2.7"} {
			if strings.Contains(value, private) {
				t.Errorf("%s holds %q", key, private)
			}
		}
	}
}

// D7: a request without a route keeps the method as its name, and an unknown method cannot
// make names without limit.
func TestARequestWithoutARouteKeepsItsMethod(t *testing.T) {
	exporter := record(t, 1)
	handler := Middleware(http.NotFoundHandler())
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/no/such/place", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("BREW", "/pot", nil))
	spans := exporter.GetSpans()
	if len(spans) != 2 || spans[0].Name != "GET" || spans[1].Name != "_OTHER" {
		t.Fatalf("spans %v", spans)
	}
	if _, ok := attrs(spans[0])["http.route"]; ok {
		t.Error("a request without a route has a route")
	}
}

// A server error marks the span, without a message.
func TestAServerErrorMarksTheSpan(t *testing.T) {
	exporter := record(t, 1)
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "the database said: duplicate key x@example.org", http.StatusInternalServerError)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/x", nil))
	span := only(t, exporter)
	if span.Status.Code != codes.Error || span.Status.Description != "error" {
		t.Fatalf("status %v", span.Status)
	}
}

// D4: a new trace is kept at the share; a trace that arrives sampled stays sampled.
func TestATraceThatArrivesSampledStaysSampled(t *testing.T) {
	exporter := record(t, 0)
	handler := Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if n := len(exporter.GetSpans()); n != 0 {
		t.Fatalf("a share of 0 kept %d new traces", n)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("traceparent", "00-"+incomingTrace+"-"+incomingParent+"-01")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if n := len(exporter.GetSpans()); n != 1 {
		t.Fatalf("a sampled trace that arrived was cut: %d spans", n)
	}
}

func TestAStatementIsNamedByOperationAndTableOnly(t *testing.T) {
	for sql, want := range map[string][2]string{
		"SELECT id, title FROM pages WHERE id = $1":                             {"SELECT", "pages"},
		"select id from Pages where title = 'secret from people'":               {"SELECT", "pages"},
		"INSERT INTO audit_pending (id, data) VALUES ($1, $2)":                  {"INSERT", "audit_pending"},
		"UPDATE jobs SET status = 'running' WHERE id = $1":                      {"UPDATE", "jobs"},
		"DELETE FROM sessions WHERE expires_at < now()":                         {"DELETE", "sessions"},
		"WITH visible AS (SELECT page_id FROM perm_visible_pages($1)) SELECT 1": {"WITH", "perm_visible_pages"},
		"SELECT now()":                  {"SELECT", ""},
		"SELECT * FROM (SELECT 1) AS x": {"SELECT", ""},
		"  \n\tBEGIN":                   {"BEGIN", ""},
		"'x@example.org' SELECT":        {"OTHER", ""},
		"EXPLAIN SELECT * FROM pages":   {"OTHER", ""},
		"":                              {"OTHER", ""},
		"SELECT 1 FROM " + strings.Repeat("a", 64): {"SELECT", ""},
		"SELECT 1 FROM 9pages":                     {"SELECT", ""},
		"SELECT count(*) FROM pages p JOIN spaces s ON s.id = p.space_id WHERE true": {"SELECT", "pages"},
	} {
		op, table := Statement(sql)
		if op != want[0] || table != want[1] {
			t.Errorf("%q: got %q %q, want %q %q", sql, op, table, want[0], want[1])
		}
	}
}

// D6: a database error is described by its SQLSTATE code, never its message, which can
// quote the value that broke a rule.
func TestAnErrorIsDescribedByItsKindOnly(t *testing.T) {
	duplicate := &pgconn.PgError{Code: "23505", Message: "duplicate key", Detail: "Key (email)=(x@example.org) already exists."}
	for err, want := range map[error]string{
		duplicate:                          "sqlstate 23505",
		fmt.Errorf("pages: %w", duplicate): "sqlstate 23505",
		&pgconn.PgError{Code: "bad"}:       "error",
		errors.New("x@example.org is odd"): "error",
		context.Canceled:                   "canceled",
		context.DeadlineExceeded:           "canceled",
	} {
		if got := ErrorClass(err); got != want {
			t.Errorf("%v: got %q, want %q", err, got, want)
		}
	}
}

func TestAQuerySpanHoldsNoStatementAndNoValue(t *testing.T) {
	exporter := record(t, 1)
	tracer := QueryTracer()
	sql := "SELECT id FROM people WHERE email = 'x@example.org'"
	ctx := tracer.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: sql, Args: []any{"x@example.org"}})
	tracer.TraceQueryEnd(ctx, nil, pgx.TraceQueryEndData{Err: &pgconn.PgError{Code: "23505", Detail: "x@example.org"}})
	span := only(t, exporter)
	if span.Name != "SELECT people" || span.SpanKind != trace.SpanKindClient {
		t.Fatalf("name %q kind %v", span.Name, span.SpanKind)
	}
	got := attrs(span)
	if got["db.operation.name"] != "SELECT" || got["db.collection.name"] != "people" || got["db.system.name"] != "postgresql" {
		t.Errorf("attributes %v", got)
	}
	if span.Status.Description != "sqlstate 23505" {
		t.Errorf("status %v", span.Status)
	}
	for key, value := range got {
		if strings.Contains(value, "example.org") || strings.Contains(value, "email") {
			t.Errorf("%s holds part of the statement: %q", key, value)
		}
	}
}

// The load budget: a query in a trace that is not kept makes no span, and the end of a query
// never ends the span around it.
func TestAQueryInATraceNotKeptMakesNoSpan(t *testing.T) {
	exporter := record(t, 1)
	tracer := QueryTracer()
	unsampled := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}})
	ctx := trace.ContextWithSpanContext(context.Background(), unsampled)
	end := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1 FROM pages"})
	// A span that is not kept is still work; the load budget says none is started at all.
	if end != ctx {
		t.Error("a query in a trace that is not kept started a span of its own")
	}
	tracer.TraceQueryEnd(end, nil, pgx.TraceQueryEndData{})
	if n := len(exporter.GetSpans()); n != 0 {
		t.Fatalf("%d spans for a trace that is not kept", n)
	}
	outer, finish := Span(context.Background(), "job", "x")
	tracer.TraceQueryEnd(outer, nil, pgx.TraceQueryEndData{Err: errors.New("x")})
	if n := len(exporter.GetSpans()); n != 0 {
		t.Fatal("the end of a query ended the span around it")
	}
	finish(nil)
	if s := only(t, exporter); s.Status.Code == codes.Error {
		t.Error("the end of a query marked the span around it")
	}
}

// D8: an event carries its trace from publish to delivery in its headers.
func TestAnEventCarriesItsTraceThroughTheQueue(t *testing.T) {
	exporter := record(t, 1)
	headers := map[string][]string{}
	_, end := Publish(context.Background(), "diffluence.events.page.created", headers)
	end(nil)
	if len(headers) == 0 {
		t.Fatal("no headers written")
	}
	_, done := Consume(context.Background(), "diffluence.events.page.created", "notify", headers)
	done(errors.New("the listener failed"))
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("%d spans", len(spans))
	}
	publish, consume := spans[0], spans[1]
	if publish.Name != "queue publish diffluence.events.page.created" || consume.Name != "queue consume notify" {
		t.Fatalf("names %q %q", publish.Name, consume.Name)
	}
	if consume.SpanContext.TraceID() != publish.SpanContext.TraceID() || consume.Parent.SpanID() != publish.SpanContext.SpanID() {
		t.Error("the delivery is not in the trace of the publish")
	}
	if consume.Status.Code != codes.Error || consume.Status.Description != "error" {
		t.Errorf("status %v", consume.Status)
	}
}

// Spec 068: an event stored in PostgreSQL keeps the trace of the request that saved it, and
// its consumer works inside that trace.
func TestAStoredEventCarriesItsTraceToItsConsumer(t *testing.T) {
	exporter := record(t, 1)
	saved, end := Span(context.Background(), "request", "")
	stored := Traceparent(saved)
	end(nil)
	if !strings.HasPrefix(stored, "00-") {
		t.Fatalf("stored %q, want a W3C traceparent", stored)
	}
	_, done := ConsumeStored(context.Background(), "page.published", "webhooks", stored)
	done(errors.New("the listener failed"))
	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("%d spans", len(spans))
	}
	request, consume := spans[0], spans[1]
	if consume.Name != "queue consume webhooks" || consume.SpanKind != trace.SpanKindConsumer {
		t.Fatalf("name %q kind %v", consume.Name, consume.SpanKind)
	}
	if consume.SpanContext.TraceID() != request.SpanContext.TraceID() || consume.Parent.SpanID() != request.SpanContext.SpanID() {
		t.Error("the consumer is not in the trace of the request that saved the event")
	}
	if got := attrs(consume); got["messaging.system"] != "postgresql" || got["messaging.destination.name"] != "page.published" {
		t.Errorf("attributes %v", got)
	}
	if consume.Status.Code != codes.Error || consume.Status.Description != "error" {
		t.Errorf("status %v", consume.Status)
	}
}

// Off, an event stores no trace and its consumer starts nothing; an event saved outside a
// trace, or stored by a release without the column, starts a trace of its own.
func TestAStoredTraceIsEmptyWhenOff(t *testing.T) {
	ctx := context.Background()
	// Even a context that carries a trace from elsewhere stores none while tracing is off.
	elsewhere := trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{1}, TraceFlags: trace.FlagsSampled}))
	if got := Traceparent(elsewhere); got != "" {
		t.Fatalf("off, stored %q", got)
	}
	if got, _ := ConsumeStored(ctx, "page.published", "webhooks", ""); got != ctx {
		t.Fatal("off, the consumer changed the context")
	}
	exporter := record(t, 1)
	if got := Traceparent(ctx); got != "" {
		t.Fatalf("outside a span, stored %q", got)
	}
	_, done := ConsumeStored(ctx, "page.published", "webhooks", "")
	done(nil)
	if span := only(t, exporter); span.Parent.IsValid() {
		t.Errorf("an event without a trace got a parent: %v", span.Parent)
	}
}

// D9: a log line written inside a span names the span.
func TestALogLineNamesItsSpan(t *testing.T) {
	record(t, 1)
	var out bytes.Buffer
	log := slog.New(LogHandler(slog.NewJSONHandler(&out, nil))).With("node", "a")
	ctx, end := Span(context.Background(), "job", "page.export")
	sc := trace.SpanContextFromContext(ctx)
	log.InfoContext(ctx, "inside")
	end(nil)
	log.Info("outside")
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines %q", lines)
	}
	if !strings.Contains(lines[0], `"trace_id":"`+sc.TraceID().String()+`"`) || !strings.Contains(lines[0], `"span_id":"`+sc.SpanID().String()+`"`) {
		t.Errorf("the line inside does not name its span: %s", lines[0])
	}
	if strings.Contains(lines[1], "trace_id") {
		t.Errorf("a line outside a span names one: %s", lines[1])
	}
}

func TestSpanRecordsAFailureWithoutItsMessageAndACount(t *testing.T) {
	exporter := record(t, 1)
	ctx, end := Span(context.Background(), "audit seal", "")
	Count(ctx, "diffluence.audit.sealed", 3)
	end(errors.New("x@example.org"))
	span := only(t, exporter)
	if span.Name != "audit seal" || span.Status.Code != codes.Error || span.Status.Description != "error" {
		t.Fatalf("span %q status %v", span.Name, span.Status)
	}
	if attrs(span)["diffluence.audit.sealed"] != "3" {
		t.Errorf("attributes %v", attrs(span))
	}
}

// A collector that is down does not fill the log.
func TestExportFailuresAreLoggedAtMostOncePerPeriod(t *testing.T) {
	var out bytes.Buffer
	q := &quietErrors{log: slog.New(slog.NewJSONHandler(&out, nil)), every: time.Hour}
	q.Handle(errors.New("first"))
	q.Handle(errors.New("second"))
	if n := strings.Count(out.String(), "spans could not be sent"); n != 1 {
		t.Fatalf("%d lines: %s", n, out.String())
	}
}

// The resource names the service, its version and its role.
func TestTheResourceNamesTheService(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	stop, err := Start(context.Background(), Options{Enabled: true, Sample: 1, Version: "1.2.3", Role: "api", Exporter: exporter})
	if err != nil {
		t.Fatal(err)
	}
	_, end := Span(context.Background(), "x", "")
	end(nil)
	spans := exporter.GetSpans() // before the stop, which empties the exporter
	if err := stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatalf("%d spans", len(spans))
	}
	got := map[string]string{}
	for _, kv := range spans[0].Resource.Attributes() {
		got[string(kv.Key)] = kv.Value.String()
	}
	if got["service.name"] != "diffluence" || got["service.version"] != "1.2.3" || got["diffluence.role"] != "api" {
		t.Errorf("resource %v", got)
	}
	if On() {
		t.Error("tracing still on after its stop")
	}
}

var _ sdktrace.SpanExporter = (*tracetest.InMemoryExporter)(nil)
