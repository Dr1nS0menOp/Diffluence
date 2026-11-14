# 11.12: What it will not do

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

Each limit has a reason. A limit can be revisited by a new decision, never quietly.

- **CSL-1000** · not · **Start, stop or restart containers, pods or services.** The console would need the Docker socket, the Kubernetes API with rights to scale, or root over systemd, and whoever holds that holds the host. Nodes reload what they can by themselves (the key, the log level, maintenance), and a restore stops nodes by having them wait, not by stopping them (spec 059).
- **CSL-1001** · not · **Upgrade Diffluence from the browser.** Replacing the binary or the image needs the same rights as CSL-1000, and a failed upgrade started from the console leaves no console to see it. The upgrade page shows and explains (CSL-560 to CSL-562). Two products of chapter 04 do it; their own documentation prefers the command line for larger installations.
- **CSL-1002** · not · **Store secrets in the database**: the mail password, the identity provider's secret, tokens of helpers, an S3 key. Every backup and every database copy would carry them (CSL-304).
- **CSL-1003** · not · **Change what the server needs to start**: database addresses, the public address, the role, the data folder, the key's place, trusted proxies. A wrong value would make the console unreachable to correct it (CSL-303).
- **CSL-1004** · not · **Write the configuration file, `.env`, `compose.yaml` or the chart's values.** The server would need write access to its own configuration, which its read-only container does not have and should not; and a file written by the web process can be changed by a stolen session for the next start.
- **CSL-1005** · not · **Waive a safety rule**: the backup before an upgrade (`--without-backup`, `UPGRADE_BACKUP_MAX_AGE=off`), the grace by setting it to zero for good, the code of tier 3. Each exists so that a single session cannot do lasting harm.
- **CSL-1006** · not · **Compare the audit witness in the console.** The witness is useful only where the console and the database owner cannot change it (ADR-039); the console shows a result reported from there (CSL-532).
- **CSL-1007** · not · **Download or upload a backup through the browser** (CSL-1220).
- **CSL-1008** · not · **Restore a single space or page from a backup.** It is a different feature, closer to the trash (roadmap, "Restoring one space from a backup"; spec 059).
- **CSL-1009** · not · **Manage PostgreSQL itself**: its settings, extensions, replication, pgBackRest, point-in-time recovery, vacuum. PostgreSQL has its own tools and its own administrator; Diffluence's owner role is not a superuser. The upgrade page shows the PostgreSQL version, nothing more.
- **CSL-1010** · not · **Run a shell command, SQL or a script** chosen in the console. Every kind of work is a named kind with validated parameters (CSL-409).
- **CSL-1011** · not · **Make the first administrator, or a setup code.** They exist for when nobody can log in (CSL-105, CSL-106).
- **CSL-1012** · not · **Add or remove nodes, or change their role.** That is the platform's work: Compose's file, the chart's replicas, another machine.
- **CSL-1013** · not · **Move the key out of a read-only mount** to make the guided replacement work on Kubernetes. The Secret stays the operator's (CSL-556).
- **CSL-1014** · not · **Reach the host's files outside what the worker mounts.** The targets are what the worker reports; a path typed in the console is never used.
- **CSL-1015** · not · **Let a token do any of it.** Every route of these areas is `SessionOnly` or `Sensitive`, as the console's routes that change the installation are today.
- **CSL-1016** · later · **S3 as a backup target** waits for wave 9 of spec 048, with `minio-go` (ADR-040). The target design of CSL-503 is ready for it: a target with a name, reported by the worker, its credentials only in the worker's environment.
