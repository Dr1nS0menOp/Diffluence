# Monitoring files

Two files for an operator who runs Prometheus and Grafana (spec 051, OPS-089, OPS-090):

- `diffluence-rules.yml`: recording and alert rules. Two objectives over 30 days, 99.5% of requests succeed and 99.5% take less than 300 ms, with burn-rate alerts over two windows, and alerts for the causes that need a person: a full disk, an old backup, failing jobs, an outbox that does not drain, a waiting contract migration, the queue, mail, and an old release. Each alert says in one line what to do.
- `diffluence-dashboard.json`: one Grafana dashboard. Import it and choose your Prometheus.

Both use only metrics that `/metrics` sends; a test checks that. The full list of metrics, alerts and what grows is in [`docs/operations/monitoring.md`](../../docs/operations/monitoring.md).
