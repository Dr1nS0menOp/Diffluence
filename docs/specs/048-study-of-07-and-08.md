# Spec 048: the study of blueprints 07 and 08, and the build order

Status: study done on branch `research-78`. Nothing is built by this spec.

Phase: blueprints 07 (operations and upgrades) and 08 (roadmap). Wave O7 built part of 07 after a short study (spec 044); the product owner asked for the same depth of study as for blueprints 03 to 06 before any more work on 07 or 08 (2026-11-10). This spec is that plan.

## The study

The research is in two folders, one file per topic, each ending in its sources:

| Folder | Chapters |
|---|---|
| [`docs/blueprint/07-operations/`](../blueprint/07-operations/) | 1 upgrades and schema change · 2 backup and recovery · 3 high availability · 4 observability · 5 release engineering · 6 configuration · 7 running it yourself · 8 capacity and load testing · 9 review of wave O7 · sources |
| [`docs/blueprint/08-roadmap/`](../blueprint/08-roadmap/) | 1 the road to 1.0 · 2 enterprise document needs · 3 risks of the later phases · 4 keeping it going without a budget · sources |

It adds lines **OPS-070 to OPS-114** to blueprint 07 and **RMP-001 to RMP-033** to blueprint 08.

What the study found, in short:

1. **The upgrade promise rests on the right test, but too narrow a one.** Research on 123 real upgrade failures (Zhang et al., SOSP 2021) shows they come through stored data (60%) and messages (40%). Diffluence's upgrade test checks reads and logins only. It must also write, and every format that crosses versions needs its own test.
2. **The failures that hurt self-hosted installations are quiet ones**: a backup that stopped, a disk that filled, an installation that fell behind. Diffluence records the backup but shows it to nobody, and does not watch the disk. Making the product watch itself is the cheapest large gain.
3. **The rollback window is short.** Contract migrations run 10 minutes after every node knows them; a bad upgrade is often noticed the next day.
4. **Releases are not yet signed, packaged, reproducible or described by an SBOM**, and all of that can be done with free tools and no long-lived keys.
5. **What needs servers can largely be done on virtual machines** on one host: the HA topology, failures and partitions, restores of 100 GB, a 24-hour soak. What one host cannot prove is spread across real hosts and their network (07.8, section 8.4).
6. **For 08**: document control should come first of the enterprise features, derived from the standards regulated teams must meet (ISO 9001, 21 CFR Part 11, EU GMP Annex 11), not from any product. Each later phase has known hurdles with known answers, mostly "start smaller than the name suggests". Donations will not pay for a person (Overney et al., ICSE 2020), so the roadmap must keep the product maintainable through pauses.

## Review of wave O7

Chapter 9 checks each part. **All of it is kept.** Corrections, each placed in a wave below:

| What | Correction | Wave |
|---|---|---|
| Upgrade test | Add writes during the migration, and a test per format that crosses versions | 2 |
| Contract migrations after 10 minutes | 24 hours by default, a setting; a release rule that a contract migration only removes what the previous release already stopped reading | 2 |
| Backup check warns by default | Refuses by default with 24 hours (product owner, 2026-11-10) | 2 |
| Backup marker | The age is shown all the time, not only at upgrade | 1 |
| Migrations | `statement_timeout`, a check against table rewrites, migrations outside a transaction for concurrent indexes | 2 |
| Release checklist | Signing, SBOM, provenance, reproducibility, `config check` | 5 |

## Decided during the study

The product owner asked that hard questions be decided here, with the reasons written down, so each can be overruled.

| # | Decision | Why |
|---|---|---|
| D1 | Contract migrations wait **24 hours** by default before they run (a setting, `DIFFLUENCE_CONTRACT_GRACE`), instead of 10 minutes | Problems after an upgrade are often found the next working day; a waiting contract migration costs nothing |
| D2 | A contract migration may only remove what the **previous** release already stopped reading | Then rolling back one release is safe even after it ran, which is what "removal in a later release" in blueprint 07 rule 1 means |
| D3 | An installation up to **six months** old may upgrade to the newest release in one step, with a short stop allowed; consecutive releases keep zero downtime. CI proves it from 1.0 | Monthly releases with only the newest line supported (blueprint 07) are fair only if falling behind a little is cheap. Six months covers a missed half year without the cost of hard stops |
| D4 | The configuration file is YAML, read with `go.yaml.in/yaml/v3` in strict mode; order default, file, environment; a secret written in the file only if the file is readable by its owner alone | The maintained successor of the archived `gopkg.in/yaml.v3`, MIT and Apache-2.0; strictness avoids YAML's known traps; the order lets container platforms override |
| D5 | OpenTelemetry for **traces only**, exported over OTLP, off by default, 10% sampled, no content in spans; metrics stay Prometheus, logs stay `slog` | Traces and metrics are stable in the Go SDK, logs are beta; Prometheus and JSON logs already work; nothing leaves the machine unless the operator asks |
| D6 | Packages are release files with checksums and signatures; a signed `apt` and `yum` repository comes later | A repository needs hosting and a key; the files give the same install with `apt install ./diffluence.deb` |
| D7 | Releases are signed **keylessly** with Sigstore and GitHub's attestations; no long-lived signing key | No key to lose or guard, which suits a project of one maintainer; signatures sit in a public log |
| D8 | **Nothing phones home.** The console warns when the running release is more than 60 days old, from its own build date; a check of the public release feed is opt-in | Privacy by default for a self-hosted product; the build date needs no network |
| D9 | Low disk: a warning below 10% free in the data folder; below 2%, uploads, imports and exports are refused while reading and saving text go on | Text is what people lose most; large writes are what fill a disk |
| D10 | An optional backup service in Compose, every 4 hours by default; a warning when the last backup is older than 26 hours | Matches the RPO of 4 hours; 26 hours tolerates a daily schedule that runs a little late |
| D11 | Default SLOs in the shipped rules: 99.5% of page requests succeed and 99.5% take under 300 ms, over 30 days | The p95 target of blueprint 01, made into an objective an operator can change |
| D12 | Load tests use the **open model** only (arrival-rate executors), and k6 runs on its own VM | The closed model hides slowness (Schroeder et al., NSDI 2006); a shared machine falsifies results |
| D13 | The reference HA set-up: Patroni with 3 etcd members and the watchdog on; NATS with 3 replicas per stream and a sync policy chosen by measurement in wave 7 | The watchdog fences a hung primary; the sync policy is a trade-off that must be measured, not guessed (Jepsen 2025) |
| D14 | Of the enterprise features: **document control first**, then draft and published spaces, then guided templates | Document control serves the strictest needs, builds on approvals that exist, and gives the "readers see the approved version" core the second one reuses |
| D15 | After 1.0, each phase starts smaller than its name: AI off by default and filtered by permissions at retrieval; the platform with scopes before a marketplace; linked instances as one-way copies; the work module as integration; mobile as a progressive web app; migration in one direction with a dry run | The known hurdles of each (08.3) all point that way, and a small start keeps the project maintainable |

## Open for the product owner

1. **Donations and anonymity (RMP-030).** GitHub Sponsors and Open Collective pay out to a person or a fiscal host, and grants need a named applicant who signs. Which route, if any, fits the wish to stay anonymous?
2. **The S3 client (wave 9).** A new dependency. The candidates are `minio-go` (Apache-2.0, small) and the AWS SDK for Go v2 (Apache-2.0, many modules). The study leans to `minio-go`; the choice is the product owner's when the wave starts.
3. **A cluster for Helm (wave 10).** Is a small Kubernetes (k3s) on virtual machines of the test host enough to call OPS-002 and OPS-031 proven, or must it be a cluster on separate hosts?

## The waves, in build order

Each wave starts with its own short study of the lines it builds, as the way of working asks, and ends with the usual gates. "Servers" says what needs more than the development machine.

| # | Wave | Lines | Proven by | Servers |
|---|---|---|---|---|
| 1 | **It watches itself**: the health page shows disk, database growth, backup age, migrations, work, mail, login and version age; low disk warns and then refuses large writes; everything that grows has a cleaner; Prometheus rules and one dashboard as files; `diffluence doctor` | OPS-077, OPS-089, OPS-090, OPS-103 to OPS-107 | Go tests per check and threshold, failure paths first; a test that fills the disk of a temporary folder; e2e of the health page; mutations of each threshold | None |
| 2 | **Upgrades, stronger**: writes during the upgrade test; a test per format that crosses versions; `statement_timeout`; a check against table rewrites; migrations outside a transaction; contract grace 24 hours (D1); backup check refuses by default with 24 hours; the version-skew policy; the release rule D2 | OPS-070 to OPS-075; OPS-076 once 1.0 exists | The upgrade test in CI with writes; cross-version tests of documents, bundles, events, jobs and settings; a migration that rewrites a table is refused in CI; mutations of every guard | None |
| 3 | **Backups that run and restore**: optional backup service in Compose; pgBackRest set-up and guide; weekly restore of a real installation as a script; the 3-2-1 note | OPS-079 to OPS-082 | The restore drill against pgBackRest; the weekly-restore script run in CI on the drill's installation | pgBackRest timing on the test VM |
| 4 | **The configuration file**: YAML (D4), `config check`, settings for later use checked at start, the list of settings shortened | OPS-099 to OPS-102 | Tests of unknown keys, duplicate keys, wrong types, YAML 1.1 traps, file modes with secrets, the order of sources; mutations | None |
| 5 | **Releases**: GoReleaser with archives, `.deb`, `.rpm` and the image; SBOM, keyless signature, provenance; a reproducibility check; base images by digest; named toggles; the support policy | OPS-093 to OPS-098 | A release run on a test tag in a fork or a dry run; installing the `.deb` and `.rpm` in clean containers; the build made twice and compared | None for building; installing the `.deb` on a real Debian VM |
| 6 | **Traces**: OpenTelemetry over OTLP (D5), checkmk tried, metric and log names listed | OPS-088, OPS-091, OPS-092 | A test collector in the dev containers receives spans with no content in them; a test that no span attribute carries text | checkmk on the test VM |
| 7 | **Measured on one host**: the load series to four times the model and past it, a 24-hour soak, a hard reset under load to prove the queue's settings (D13), restore of 10 GB and 100 GB, CPU per person and the sizing table, a small load test in CI | OPS-078, OPS-083, OPS-108 to OPS-113 | k6 results with every condition of 07.8 section 8.4, written into `limits.md` | The test host: one VM for Diffluence and PostgreSQL, one for k6 |
| 8 | **Several nodes on VMs**: 2 `api`, 2 `worker`, 3 NATS, 3 PostgreSQL with Patroni and etcd, PgBouncer, HAProxy; failures, pauses and partitions under load; the suite under PgBouncer; a rolling upgrade under load; the page on what HA protects | OPS-084 to OPS-087, OPS-114, and OPS-011 to OPS-013, OPS-033 | No acknowledged write lost, no two truths, no failed request during the rolling upgrade, recorded per run | Ten or more VMs on the test host |
| 9 | **S3 storage** with versioning, after the client is chosen (open question 2) | OPS-014, OPS-052 | The storage interface's tests run against an S3-compatible test server; backup and restore with S3 | An S3-compatible server on a VM |
| 10 | **Helm and LXC on real hosts** | OPS-002, OPS-004, OPS-031 | Install, upgrade and restore with the chart on a small cluster, and with the LXC script on a container host | A cluster (open question 3) and an LXC host |

Waves 1 to 6 need no servers and can be built one after the other, with at most two builders at once. Wave 7 can run beside them on the test host, because it only measures. Waves 8 to 10 follow wave 7.

**Before 1.0, after these waves** (RMP-003): the enterprise document features in the order of D14, each with its own study; then the beta when the criteria of RMP-001 hold; then the release candidate; then 1.0 with the promises of RMP-002.

## Load budget (ARC-238)

| Wave | Cost at runtime |
|---|---|
| 1 | One `statfs` and three cheap queries per health check (cached for 30 seconds); one query for database size per hour |
| 2 | None at runtime; CI time for the longer upgrade test |
| 3 | The backup itself, outside working hours by the operator's choice |
| 4 | Reading one file at start |
| 6 | With tracing on, about one span per request and query group, sampled at 10%; nothing when off |
| 7 to 10 | None in the product; they measure |

## What one machine cannot prove

The test host shows how Diffluence behaves: where it gives way, how it fails, what a reset or a partition between its VMs does. It cannot show fail-over across separate hosts, partitions of a real network, or how the HA topology scales when its parts do not share one machine. Those wait for real hosts, and every result names the machine it came from.
