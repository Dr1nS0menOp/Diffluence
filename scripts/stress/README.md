# Stress test

The scripts of the stress test in `docs/reports/2026-11-10-stress-test.md`. Run them against a test instance on machines of their own, never against a live instance.

| File | Runs on | Does |
|---|---|---|
| `app-vm.sh` | the app machine | Installs Docker and Diffluence as the install guide says, with `compose.stress.yaml` as the override |
| `compose.stress.yaml` | the app machine | PostgreSQL settings for a 32 GB machine, and `pg_stat_statements` |
| `load-vm.sh` | the load machine | Installs k6 from its official release |
| `seed.py` | the load machine | Fills the instance with 5,000 people, 300 spaces, 60,000 pages and more, through the API; `--resume` adds what is missing |
| `load.js` | the load machine | The k6 scenarios: `smoke`, `mix`, `level` (with `VUS`), `step`, `search`, `worst`, `samepage` |
| `summarise.py` | the load machine | A table per route from a k6 summary |
| `monitor.sh` | the app machine | Samples CPU, memory, disk, database connections and `/readyz` every few seconds |
| `break.sh` | the app machine | Restarts PostgreSQL, kills the app, fills the disk, checks the audit chain |
| `slow-clients.py`, `huge-page.py` | the load machine | Slow connections, and a page just under the size limit |

The app's public address must be a host name, not an IP address: passkeys need a domain. Put the name in `/etc/hosts` on both machines when there is no DNS.
