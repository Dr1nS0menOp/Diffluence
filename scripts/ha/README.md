# HA test topology (spec 062)

Scripts that build the topology of OPS-084 on virtual machines of one test host, and the
tests that break it. Nothing here runs in CI; spec 062 is the plan and the pass criteria.

| File | What it does | Runs on |
|---|---|---|
| `topology.env` | VM ids, sizes and roles | everywhere (sourced) |
| `ha.local.env` | Addresses and secrets of one test run; never committed (see below) | operator |
| `host-create-vms.sh` | Creates the VMs from the cloud image with `qm` | the test host |
| `cloud-init/common.yaml` | Base packages, time sync, the operator's key | each VM at first boot |
| `roles/db.sh` | PostgreSQL 16, Patroni, etcd, the softdog watchdog | db1 to db3 |
| `roles/nats.sh` | A NATS JetStream cluster member | nats1 to nats3 |
| `roles/edge.sh` | HAProxy (PostgreSQL leader and app) and PgBouncer | edge |
| `roles/app.sh` | The `diffluence` package as `api` or `worker` | api1, api2, work1, work2 |
| `conf/` | Templates the role scripts fill in | |
| `tests/` | The fault tests and the write ledger | the load VM and the host |
| `validate.sh` | Checks every script and config without a host | a development machine |

`ha.local.env` holds what may not be committed: the addresses on the test network, the
database passwords, `DIFFLUENCE_COLLAB_TOKEN` if used, and the path of the shared
`master.key`. Copy `ha.local.env.example` and fill it in.

Run `bash scripts/ha/validate.sh` before a session: it checks the shell scripts and runs
the validators of HAProxy, NATS and Patroni in throw-away containers.
