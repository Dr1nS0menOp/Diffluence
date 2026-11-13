# 07.4: Observability: metrics, logs, traces and alerts

An operator of Diffluence has to know three things without reading code: is it working for people, is it about to stop, and why did it go wrong. This chapter sets out how the industry answers them, and what Diffluence should ship so a small team gets good answers with little effort.

## 4.1 What the research and practice say

- **Measure what people feel.** The Google SRE book names four golden signals for a service: latency, traffic, errors and saturation. Tom Wilkie's RED method (rate, errors, duration) is the same for requests, and Brendan Gregg's USE method (utilisation, saturation, errors) is the same for resources. Diffluence already exports most of these.
- **Tracing explains slow requests.** Google's Dapper paper (Sigelman et al., 2010) showed that following one request through every service, with low overhead and sampling, finds problems that logs and averages hide. OpenTelemetry is the vendor-neutral standard that grew from that line of work, and W3C Trace Context is the header (`traceparent`) that carries a trace across services.
- **Tails matter more than averages.** Dean and Barroso, *The Tail at Scale* (CACM 2013): when one page needs many requests, the slowest of them decides the page. So targets are percentiles (p95, p99), never means.
- **Alert on symptoms, against an objective.** A service level objective (SLO) says what share of requests must be good, such as 99.5% under 300 ms over 30 days. The remaining 0.5% is the error budget. The SRE workbook's chapter on alerting on SLOs recommends alerts on how fast the budget burns, checked over a long and a short window at once, so an alert fires quickly for a real outage and not at all for a short blip. That is the industry's main answer to **alert fatigue**: pages for symptoms people feel, tickets for causes.
- **Getting an alert to the right person is itself costly.** Chen et al. studied incident triage in 20 large online services at Microsoft (ICSE-SEIP 2019): between 4% and 92% of incidents were first sent to the wrong team, and each reassignment made handling up to ten times slower. For a small team the same lesson holds in a simpler form: every alert must say what is wrong and what to do, so the one person who reads it can act without searching.

## 4.2 OpenTelemetry today

The OpenTelemetry project rates its Go SDK by signal: **traces stable, metrics stable, logs beta**. Most self-hosted projects therefore keep Prometheus for metrics, keep their own structured logs, and add OpenTelemetry for traces, exported with OTLP to a collector of the operator's choice (Jaeger, Tempo, or a vendor). Export is off until the operator gives an address, so nothing leaves the machine by surprise.

## 4.3 Where Diffluence stands

| Practice | State | Where |
|---|---|---|
| `/livez`, `/readyz` with reasons, `busy` and `draining` | Built | OPS-020, OPS-021 |
| Prometheus metrics: requests, latency, pool, outbox, jobs, mail, failures, migrations | Built, behind a token | `internal/metrics/metrics.go`, OPS-022 |
| JSON logs through `slog`, with a `trace_id` from `traceparent` | Built | OPS-025, OPS-024 |
| Trace export | Not built | OPS-024 |
| Example alert rules, an SLO, a dashboard | Not shipped | |
| checkmk reading the metrics | Not tried | OPS-023 |

## 4.4 Recommendations

- **OPS-088** · built (spec 060; spans hold no ids either) · **Medium.** Export traces with the OpenTelemetry Go SDK over OTLP, off by default, switched on by one setting with the collector's address. Spans for each request, each database query group, each job and each event handler, carrying the existing `trace_id`. A sampling rate setting (default 10%), and spans never carry page text, titles, names or addresses, only ids. Approved as a dependency by the product owner on 2026-11-10. This closes OPS-024.
- **OPS-089** · new · **Medium.** Ship `deploy/monitoring/` with Prometheus recording and alert rules: an availability SLO and a latency SLO (default 99.5% of page requests under 300 ms over 30 days, the blueprint 01 target), burn-rate alerts over two windows, and plain alerts for causes that need a person soon: disk of the data folder above 85%, backup older than 26 hours (OPS-077), failed jobs growing, outbox not draining, pending contract migrations older than a day. Each alert says in one line what to do.
- **OPS-090** · new · **Low.** Ship one dashboard as a JSON file for Grafana, built only on the metrics above. It is a file, not a dependency.
- **OPS-091** · new · **Low.** Try the metrics with checkmk's Prometheus integration on the test machine and write the steps down (OPS-023).
- **OPS-092** · built (spec 060) · **Low.** A page in the operations guide listing every metric and every log event an operator may build on, with the promise that names change only with a note in the changelog. Log and metric names are part of the interface an operator scripts against.

## Sources

- Beyer et al. (eds.): [Site Reliability Engineering, chapter 6, Monitoring Distributed Systems](https://sre.google/sre-book/monitoring-distributed-systems/), O'Reilly 2016.
- Beyer et al. (eds.): [The Site Reliability Workbook, chapter 5, Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/), O'Reilly 2018.
- Sigelman et al.: [Dapper, a Large-Scale Distributed Systems Tracing Infrastructure](https://research.google/pubs/dapper-a-large-scale-distributed-systems-tracing-infrastructure/), Google Technical Report, 2010.
- Dean, Barroso: [The Tail at Scale](https://doi.org/10.1145/2408776.2408794), Communications of the ACM 56(2), 2013.
- Chen et al.: [An Empirical Investigation of Incident Triage for Online Service Systems](https://doi.org/10.1109/ICSE-SEIP.2019.00020), ICSE-SEIP 2019.
- Gregg: [The USE Method](https://www.brendangregg.com/usemethod.html).
- OpenTelemetry: [Go, status of each signal](https://opentelemetry.io/docs/languages/go/), [OTLP specification](https://opentelemetry.io/docs/specs/otlp/).
- W3C: [Trace Context](https://www.w3.org/TR/trace-context/), Recommendation.
- Prometheus: [Metric and label naming](https://prometheus.io/docs/practices/naming/), [Alerting](https://prometheus.io/docs/practices/alerting/).

All sources of this study are also in [sources.md](sources.md).
