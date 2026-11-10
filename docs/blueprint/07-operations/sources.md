# 07: Sources

Read on 2026-11-10 for this study, in our own words. Diffluence's own code, specs and ADRs are the first source. Each chapter ends with the sources it leans on; this page holds them all, by subject. No product's code, text or screens were copied; other products are named only for public facts about how they upgrade, back up or release.

## Upgrades and schema change

- Zhang, Yang, Jin, Sethi, Rodrigues, Lu, Yuan: [Understanding and Detecting Software Upgrade Failures in Distributed Systems](https://doi.org/10.1145/3477132.3483577), SOSP 2021 ([PDF](https://www.cs.purdue.edu/homes/yonglezh/pub/upgrade-sosp21.pdf)).
- Dumitraș, Narasimhan: [Why Do Upgrades Fail and What Can We Do about It?](https://doi.org/10.1007/978-3-642-10445-9_18), Middleware 2009.
- Gunawi et al.: [Why Does the Cloud Stop Computing? Lessons from Hundreds of Service Outages](https://doi.org/10.1145/2987550.2987583), SoCC 2016.
- De Jong, van Deursen, Cleve: [Zero-Downtime SQL Database Schema Evolution for Continuous Deployment](https://doi.org/10.1109/ICSE-SEIP.2017.5), ICSE-SEIP 2017 ([PDF](https://pure.tudelft.nl/ws/files/11926973/TUD_SERG_2017_005.pdf)).
- Curino, Moon, Zaniolo: [Graceful Database Schema Evolution: the PRISM Workbench](https://www.vldb.org/pvldb/vol1/1453939.pdf), PVLDB 1(1), 2008.
- Fowler: [Parallel Change](https://martinfowler.com/bliki/ParallelChange.html).
- Stripe: [Online migrations at scale](https://stripe.com/blog/online-migrations).
- GitLab: [Migration style guide](https://docs.gitlab.com/development/migration_style_guide/), [Post-deployment migrations](https://docs.gitlab.com/development/database/post_deployment_migrations/), [Plan your upgrade path](https://docs.gitlab.com/update/upgrade_paths/), [Upgrade path tool](https://gitlab-com.gitlab.io/support/toolbox/upgrade-path/).
- Kubernetes: [Version skew policy](https://kubernetes.io/releases/version-skew-policy/).
- Nextcloud: [How to upgrade](https://docs.nextcloud.com/server/latest/admin_manual/maintenance/upgrade.html).
- Sentry: [Self-hosted releases and upgrading](https://develop.sentry.dev/self-hosted/releases/).
- PostgreSQL 16: [Explicit locking](https://www.postgresql.org/docs/16/explicit-locking.html), [Client connection defaults: lock_timeout, statement_timeout](https://www.postgresql.org/docs/16/runtime-config-client.html), [CREATE INDEX CONCURRENTLY](https://www.postgresql.org/docs/16/sql-createindex.html#SQL-CREATEINDEX-CONCURRENTLY), [ALTER TABLE](https://www.postgresql.org/docs/16/sql-altertable.html).
- Xata: [Schema changes and the Postgres lock queue](https://xata.io/blog/migrations-and-exclusive-locks).

## Backup and recovery

- NIST: [SP 800-34 Rev. 1, Contingency Planning Guide for Federal Information Systems](https://csrc.nist.gov/pubs/sp/800/34/r1/upd1/final).
- GitLab: [Postmortem of database outage of January 31](https://about.gitlab.com/blog/postmortem-of-database-outage-of-january-31/), 2017.
- Beyer, Jones, Petoff, Murphy (eds.): [Site Reliability Engineering](https://sre.google/sre-book/table-of-contents/), O'Reilly 2016: [chapter 26, Data Integrity](https://sre.google/sre-book/data-integrity/).
- US-CERT: [Data Backup Options](https://www.cisa.gov/sites/default/files/publications/data_backup_options.pdf), 2012.
- PostgreSQL 16: [Backup and restore](https://www.postgresql.org/docs/16/backup.html), [Continuous archiving and point-in-time recovery](https://www.postgresql.org/docs/16/continuous-archiving.html), [WAL configuration](https://www.postgresql.org/docs/16/wal-configuration.html).
- PostgreSQL 17: [pg_basebackup](https://www.postgresql.org/docs/17/app-pgbasebackup.html), [pg_combinebackup](https://www.postgresql.org/docs/17/app-pgcombinebackup.html).
- pgBackRest: [User guide](https://pgbackrest.org/user-guide.html), [Configuration reference](https://pgbackrest.org/configuration.html).

## High availability

- Ongaro, Ousterhout: [In Search of an Understandable Consensus Algorithm](https://www.usenix.org/conference/atc14/technical-sessions/presentation/ongaro), USENIX ATC 2014.
- Gilbert, Lynch: [Brewer's Conjecture and the Feasibility of Consistent, Available, Partition-Tolerant Web Services](https://doi.org/10.1145/564585.564601), SIGACT News 33(2), 2002.
- Kleppmann: [Designing Data-Intensive Applications](https://dataintensive.net/), O'Reilly 2017.
- GitHub: [October 21 post-incident analysis](https://github.blog/2018-10-30-oct21-post-incident-analysis/), 2018.
- Jepsen: [NATS 2.12.1](https://jepsen.io/analyses/nats-2.12.1), 2025.
- NATS: [JetStream clustering](https://docs.nats.io/running-a-nats-service/configuration/clustering/jetstream_clustering), [JetStream concepts](https://docs.nats.io/nats-concepts/jetstream).
- Patroni: [Documentation](https://patroni.readthedocs.io/en/latest/), [Watchdog support](https://patroni.readthedocs.io/en/latest/watchdog.html).
- PgBouncer: [Features](https://www.pgbouncer.org/features.html).
- PostgreSQL 16: [High availability, load balancing and replication](https://www.postgresql.org/docs/16/high-availability.html).

## Observability

- Beyer et al. (eds.): [Site Reliability Engineering, chapter 6, Monitoring Distributed Systems](https://sre.google/sre-book/monitoring-distributed-systems/).
- Beyer et al. (eds.): [The Site Reliability Workbook, chapter 5, Alerting on SLOs](https://sre.google/workbook/alerting-on-slos/), O'Reilly 2018.
- Sigelman et al.: [Dapper, a Large-Scale Distributed Systems Tracing Infrastructure](https://research.google/pubs/dapper-a-large-scale-distributed-systems-tracing-infrastructure/), 2010.
- Dean, Barroso: [The Tail at Scale](https://doi.org/10.1145/2408776.2408794), CACM 56(2), 2013.
- Chen et al.: [An Empirical Investigation of Incident Triage for Online Service Systems](https://doi.org/10.1109/ICSE-SEIP.2019.00020), ICSE-SEIP 2019.
- Gregg: [The USE Method](https://www.brendangregg.com/usemethod.html).
- OpenTelemetry: [Go](https://opentelemetry.io/docs/languages/go/), [OTLP specification](https://opentelemetry.io/docs/specs/otlp/).
- W3C: [Trace Context](https://www.w3.org/TR/trace-context/).
- Prometheus: [Metric and label naming](https://prometheus.io/docs/practices/naming/), [Alerting](https://prometheus.io/docs/practices/alerting/).

## Release engineering and the supply chain

- [Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html).
- Michlmayr, Fitzgerald, Stol: [Why and How Should Open Source Projects Adopt Time-Based Releases?](https://doi.org/10.1109/MS.2015.55), IEEE Software 32(2), 2015.
- Rahman, Querel, Rigby, Adams: [Feature Toggles: Practitioner Practices and a Case Study](https://doi.org/10.1145/2901739.2901745), MSR 2016.
- Ladisa, Plate, Martinez, Barais: [SoK: Taxonomy of Attacks on Open-Source Software Supply Chains](https://doi.org/10.1109/SP46215.2023.10179304), IEEE S&P 2023.
- Newman, Meyers, Torres-Arias: [Sigstore: Software Signing for Everybody](https://doi.org/10.1145/3548606.3560596), ACM CCS 2022.
- Torres-Arias, Afzali, Kuppusamy, Curtmola, Cappos: [in-toto](https://www.usenix.org/conference/usenixsecurity19/presentation/torres-arias), USENIX Security 2019.
- Lamb, Zacchiroli: [Reproducible Builds: Increasing the Integrity of Software Supply Chains](https://arxiv.org/abs/2104.06020), IEEE Software 39(2), 2022.
- Cox: [Perfectly Reproducible, Verified Go Toolchains](https://go.dev/blog/rebuild), 2023.
- [SLSA 1.0 levels](https://slsa.dev/spec/v1.0/levels).
- NTIA: [The Minimum Elements For a Software Bill of Materials](https://www.ntia.gov/report/2021/minimum-elements-software-bill-materials-sbom), 2021.
- [CycloneDX](https://cyclonedx.org/specification/overview/), [SPDX](https://spdx.dev/).
- GitHub: [Artifact attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds).
- GoReleaser: [nFPM](https://goreleaser.com/customization/nfpm/).
- Support policies: [PostgreSQL](https://www.postgresql.org/support/versioning/), [Kubernetes](https://kubernetes.io/releases/), [GitLab](https://docs.gitlab.com/policy/maintenance/), [Node.js](https://nodejs.org/en/about/previous-releases), [Nextcloud](https://github.com/nextcloud/server/wiki/Maintenance-and-Release-Schedule).

## Configuration

- Yin, Ma, Xu, Rouf, Pasupathy, Bairavasundaram: [An Empirical Study on Configuration Errors in Commercial and Open Source Systems](https://doi.org/10.1145/2043556.2043572), SOSP 2011.
- Xu, Jin, Huang, Zhou, Lu, Jin, Pasupathy: [Early Detection of Configuration Errors to Reduce Failure Damage](https://www.usenix.org/conference/osdi16/technical-sessions/presentation/xu), OSDI 2016.
- Xu et al.: [Hey, You Have Given Me Too Many Knobs!](https://doi.org/10.1145/2786805.2786852), ESEC/FSE 2015.
- [The Twelve-Factor App, Config](https://12factor.net/config).
- OWASP: [Secrets Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Secrets_Management_Cheat_Sheet.html).
- systemd: [Credentials](https://systemd.io/CREDENTIALS/).
- [YAML 1.2.2](https://yaml.org/spec/1.2.2/).
- [go-yaml (archived)](https://github.com/go-yaml/yaml), [yaml/go-yaml](https://github.com/yaml/go-yaml).

## Self-hosted operations

- Raman, Joglekar, De Cristofaro, Sastry, Tyson: [Challenges in the Decentralised Web: The Mastodon Case](https://doi.org/10.1145/3355369.3355572), IMC 2019.
- The upgrade guides of Nextcloud, GitLab and Sentry, above.

## Capacity and load testing

- Schroeder, Wierman, Harchol-Balter: [Open Versus Closed: A Cautionary Tale](https://www.usenix.org/conference/nsdi-06/open-versus-closed-cautionary-tale), NSDI 2006.
- Tene: [How NOT to Measure Latency](https://www.infoq.com/presentations/latency-response-time/), 2015.
- Little: [A Proof for the Queuing Formula: L = λW](https://doi.org/10.1287/opre.9.3.383), Operations Research 9(3), 1961.
- Gunther: [Guerrilla Capacity Planning](https://doi.org/10.1007/978-3-540-31010-5), Springer 2007.
- Jain: The Art of Computer Systems Performance Analysis, Wiley 1991.
- Grafana k6: [Executors](https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/), [Open and closed models](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/open-vs-closed/), [Dropped iterations](https://grafana.com/docs/k6/latest/using-k6/scenarios/concepts/dropped-iterations/).
- PostgreSQL 16: [pg_test_fsync](https://www.postgresql.org/docs/16/pgtestfsync.html), [pgbench](https://www.postgresql.org/docs/16/pgbench.html).
