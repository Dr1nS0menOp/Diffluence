# Spec 060: traces with OpenTelemetry

Wave 6 of spec 048. It builds OPS-088 (trace export over OTLP, off by default), lists the names an operator may build on (OPS-092), and writes down what checkmk needs (OPS-091), which can only be tried on the test server. The study is `docs/blueprint/07-operations/04-observability.md`; the decision is spec 048 D5: traces only, over OTLP, off by default, 10% sampled, no content in spans. Metrics stay Prometheus and logs stay `slog`. The product owner approved the OpenTelemetry library on 2026-11-10 (ADR-040).

## 1. What exists

- Every request gets an `X-Request-Id`, and the request log line carries `trace_id` when the request came with a valid W3C `traceparent` (spec 021 D5, `internal/httpapi/middleware.go`).
- `/metrics` counts requests by method and class of status, never by path (spec 021 D3).
- The pool already has a `pgx.QueryTracer`, `queryCounter`, for the query budgets (ARC-239).
- Events go from the outbox to NATS JetStream (`internal/queue`), and back to listeners.
- Jobs and periodic tasks run in `internal/jobs`; the audit sealer runs every second on every worker (ADR-041); the backup worker calls `diffluence backup next`, `report` and `restore-checked` (spec 058).

Nothing leaves the machine today, and that stays the default.

## 2. Decisions

| # | Decision | Why |
|---|---|---|
| D1 | One switch, `DIFFLUENCE_TRACING` (default `false`), and one ratio, `DIFFLUENCE_TRACING_SAMPLE` (default `0.1`, from 0 to 1), both through the configuration of spec 056, so they show in `config check` and may sit in the file. | The study asks for one setting to switch it on; a ratio outside 0 to 1 is a typing mistake and refuses the start. |
| D2 | Where spans go is the standard of OpenTelemetry: `OTEL_EXPORTER_OTLP_ENDPOINT` or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, `_HEADERS`, `_CERTIFICATE`, `_TIMEOUT`, `_COMPRESSION`, and `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES` for the resource. The SDK reads them. Without an endpoint the SDK sends to `localhost:4318`, the usual sidecar collector. | Every collector, Grafana Tempo, Jaeger and the hosted services document these names; inventing our own would make every set-up translate. |
| D3 | The exporter is OTLP over HTTP with protobuf (`otlptracehttp`), not gRPC. | Every collector takes both; HTTP passes plain proxies and needs no second protocol stack in our code. The module still links the gRPC stubs of the OTLP protocol package; that is how the official package is built. |
| D4 | Sampling is parent-based on a trace-id ratio: a request that arrives inside a sampled trace stays sampled, a new trace is sampled at `DIFFLUENCE_TRACING_SAMPLE`. | So a trace a proxy started is never cut in half, and the volume stays a set share (D5 of spec 048). |
| D5 | When tracing is off, nothing of it runs: no middleware in the chain, no query tracer on the pool, no headers on events, no wrapping of the log. The helpers check one flag first. | "Zero when off" is then true by construction, not by measurement only. The flag and the global tracer provider of OpenTelemetry are the one piece of process-wide state, set once at start before anything serves; passing a tracer through every module would touch forty constructors for the same effect. |
| D6 | Spans carry names and identifiers of kinds, never content: the route pattern (`GET /api/v1/pages/{id}`), never the path; the SQL operation and table (`SELECT pages`), never the statement or its values; the event type and the consumer, never the payload; the job kind and the task name. No user agent, client address, email, name, title or text. An error sets the span's status with the word `error` or the SQLSTATE class, never the error's message, because a database message can quote a value. | Spans leave the machine for a collector the operator runs, perhaps a hosted one; personal data and page text must not travel with them (SEC rules on personal data in logs apply to traces as well). The study allowed ids; this spec leaves them out too, because the id of a person, joined with the database, names that person, and the route pattern already says which kind of object a request touched. A trace is found from a log line by `trace_id`, and the log line holds the `request_id`. |
| D7 | A request whose route has no pattern (a 404, the static files) keeps the name `GET` and the attribute `http.route` is left out. | A path is unbounded and holds ids and tokens. |
| D8 | Trace context crosses the queue in NATS headers (`traceparent`, `tracestate`), with the W3C propagator. The outbox relay publishes in its own trace, not in the trace of the request that wrote the event: carrying the request's context through the outbox would need a column and a migration, and the relay's span already links publish and consume. | A listener's work shows under the publish that caused it, without a migration. |
| D9 | Log lines that are written with a context carry `trace_id` and `span_id` while tracing is on, so a line can be found from a trace and back. The request log line keeps `trace_id` from `traceparent` while tracing is off, as before. | The study (chapter 4) asks for the join between logs and traces; `slog`'s context is the place for it. |
| D10 | The backup worker's commands start tracing too when the switch is on, with one span per command, and flush before they exit. | The worker is a short process; without a flush its span would be lost. |
| D11 | No `otelhttp`, `otelpgx` or other contrib modules: the middleware, the query tracer and the NATS carrier are each a few lines of our own. | Spec 048 asks for the smallest set of modules; contrib modules bring their own attribute choices, which we would have to filter again for D6. |
| D12 | The restore commands of spec 059 (`backup restore-request`, `-prepare`, `-run`, `-abort`, `-way-back`) get no span in this wave; `next`, `report` and `restore-checked` keep theirs. Decided on the product owner's behalf on 2026-11-13, may be overruled. | A restore stops every node and replaces the database, so a trace of it would go to a collector that may itself be restored over; its record is the restored audit log. A later wave can add spans once wave 7 shows the collector side. |

## 3. Plan

1. **Configuration** (`internal/config`): `TRACING` and `TRACING_SAMPLE`, with a reader for a fraction; a ratio outside 0 to 1 or not a number refuses the start. Failure paths first.
2. **`internal/tracing`**: `Start` builds the provider with the OTLP exporter, the resource and the sampler, sets the W3C propagator and the flag, and returns a shutdown that flushes; when off it returns at once and changes nothing. `Span` starts a span or, when off, returns the context and a no-op. `Middleware` makes the server span and continues an incoming `traceparent`; `NameRoute` names it by the pattern. `QueryTracer` makes one span per query, named by operation and table. `Inject` and `Extract` carry the context in NATS headers. `LogHandler` adds `trace_id` and `span_id`.
3. **Wiring**: `diffluence serve` starts tracing before the app, wraps the logger and flushes on exit; `httpapi.New` puts the middleware first when on; the router names the span; `db.Open` adds the query tracer when on; the queue injects on publish and extracts on delivery; jobs, periodic tasks and the sealer get a span each; the backup commands get one span each.
4. **Tests**:
   - the switch and the ratio, and their refusals;
   - off means no span, no middleware, no change to the pool and no headers;
   - a request makes one span named by its route, with no path, no query and no ids;
   - a query span names operation and table and holds no SQL and no values;
   - an error status holds no message;
   - an event carries the trace from publish to delivery;
   - the log line carries the ids of its span;
   - a test through the whole application with a recording exporter that creates a person with an email, a page with text and a token, reads and searches, and fails if any span name or attribute holds the email, the text, the token or a UUID.
5. **Mutations** for each of these checks.
6. **Docs**: the monitoring guide (traces, every log event and metric an operator may build on), `install.md`, the example configuration file, the changelog, `dependencies.md`, and the OPS lines.

## 4. Load budget (ARC-238)

Off: no work at all (D5), proven by a benchmark of the helpers with zero allocations and by tests that the chain and the pool are unchanged. On: one span per request, one per query and one per job or event, sampled at 10%; an unsampled span is a few hundred nanoseconds and no export. The numbers measured are in the report and in `docs/operations/limits.md`. Spans are sent in batches by the SDK in the background; when the collector is down the SDK drops spans after its queue of 2,048 and never blocks a request.

## 5. What one machine cannot prove

- A real collector and a tracing back end (Tempo, Jaeger): the tests use the SDK's in-memory exporter and an HTTP test server that takes OTLP. Trying one on the test server is part of wave 7.
- checkmk (OPS-091): needs the test server.
- The overhead under real load: wave 7 measures it.
