# 07: Operations and upgrades

## Deployment targets

| Target | What we ship |
|---|---|
| **Docker Compose** | The reference installation. One `compose.yaml` and one `.env` file. |
| **Kubernetes** | A Helm chart (phase 3). An operator later, if there is demand. |
| **LXC on Proxmox** | The single binary as a `.deb` plus a systemd unit, an install script, and an Ansible role. No Docker needed. |
| Bare Linux | `.deb` and `.rpm` packages built by GoReleaser |

Configuration is one YAML file and/or environment variables. Secrets come from files (Docker/Kubernetes secrets), never from the command line.

## Topologies

**Small (up to a few hundred users):** one node with `role=all`, embedded NATS, local Postgres, local disk storage.

**HA (from phase 3):**
- 2 or more `api` nodes and 2 or more `worker` nodes behind a load balancer
- A NATS JetStream cluster of 3 nodes
- PostgreSQL with Patroni (3 nodes), PgBouncer and HAProxy. Reads can go to replicas.
- S3-compatible storage with versioning

**Rough sizing, worked out:** 5,000 users × 10% active at the same time = 500 active users. Each makes about 1 request every 10 seconds: 500 ÷ 10 = **50 requests per second**. One API node handles well over 1,000 per second, so **3 nodes** is for redundancy, not capacity. The k6 load test in CI checks this.

## Monitoring

- `/livez` (process is alive) and `/readyz` (ready for traffic: database, NATS and storage reachable, migrations compatible)
- Prometheus metrics at `/metrics` (requests, latency, queue depth, job failures, migration progress). checkmk can read them with its Prometheus integration.
- OpenTelemetry traces and structured JSON logs (`slog`), ready for Graylog or Loki

## Upgrades: the most important promise

**Goal:** after 1.0, releases come monthly and older versions go out of support quickly. So an upgrade must be **one command** (`docker compose pull && docker compose up -d`, `helm upgrade`, or `apt upgrade`), and it must cause **no downtime** while people are working.

**Lessons from large vendors**
- **GitLab** splits migrations into *regular* ones (additive, before new code runs), *post-deployment* ones (removals, only after all nodes run the new version) and *batched background* ones (big data changes in small chunks). It keeps the number of required upgrade stops low.
- **Atlassian Data Center** rolling upgrades: nodes are upgraded one by one while the cluster temporarily runs **two versions side by side**.
- **Stripe** online migrations: dual-write to old and new, move reads over, move writes over, then remove the old. Never one big switch.
- **Kubernetes** version-skew policy: components must work with the version just before and after them.
- **PostgreSQL** practice: `CREATE INDEX CONCURRENTLY`, a short `lock_timeout` on every migration, and no full table rewrites during working hours.

**Our rules**
1. **Expand → migrate → contract.** A release only *adds* (new columns, tables, indexes). Data is moved by **background jobs via NATS**, with progress shown in the admin console. Removal of old structures happens in a **later** release.
2. **Version N and N+1 must run together.** Every release works with both the old and new schema, so rolling restarts are safe. CI tests this: the old version keeps serving traffic while the new one migrates.
3. **No manual steps, ever.** If an upgrade needs a human, it is a bug.
4. **Preflight checks** before migrating: disk space, Postgres version, NATS health, backup recency. If one fails, the upgrade refuses to start and says why.
5. **Feature flags** for risky features, so code can ship dark and be switched on later.
6. **Roll forward, plus a safety net.** Database changes are additive, so rolling back to version N keeps working. A pre-upgrade backup marker makes a full restore possible.
7. **Skipping versions:** jumps are allowed. The binary runs all migrations in order. Zero downtime is **guaranteed between consecutive releases**. A larger jump may need a short maintenance window, which the preflight check announces up front.

## Backup and restore (RPO and RTO at most 4 hours)

- **PostgreSQL:** pgBackRest with continuous WAL archiving and a daily full or differential backup. In practice that gives an RPO of **minutes**.
- **Object storage:** versioning plus replication to a second location, or an encrypted nightly sync
- **Configuration and keys:** a documented export. The KEK is backed up **separately** from the data.
- **Automatic restore test:** a weekly job restores to a scratch environment and verifies hash-chain integrity and page counts, and alerts on failure
- **Runbook:** a step-by-step restore guide, practised at least every phase. RTO is measured in that practice and must stay under 4 hours for 100 GB of data.

## Releases

- Before 1.0: no support, breaking changes allowed, but the upgrade machinery is already used for every build
- From 1.0: monthly releases (`1.x.0`), with security fixes as patch releases (`1.x.y`). Only the latest release line is supported.
- Release notes in plain language, with an "upgrade impact" line (expected none)
