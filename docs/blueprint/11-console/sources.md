# Sources of the study of the console that runs the installation

All outside sources were consulted on **2026-11-14**, in their public documentation, for how each product splits work between its web console and its command line or configuration. No product's source code was read, and no text, layout or wording was taken (`docs/blueprint/02-clean-room-and-licensing.md`). "Read" means the page itself was opened; "partly read" means only the first part or the relevant sections; "through search" means only a search engine's summary was available. Where a finding rests on a community forum rather than a product's own documentation, chapter 04 says so.

## GitLab (self-managed)

| Source | Read | Used for |
|---|---|---|
| [Back up and restore GitLab](https://docs.gitlab.com/administration/backup_restore/) | Read | Backup and restore by command only |
| [Back up GitLab](https://docs.gitlab.com/administration/backup_restore/backup_gitlab/) | Read | The secrets file kept apart from the backup |
| [GitLab Maintenance Mode](https://docs.gitlab.com/administration/maintenance_mode/) | Read | Maintenance mode in the Admin Area; what it blocks |
| [Background migrations](https://docs.gitlab.com/update/background_migrations/) | Read | Migrations shown in the Admin Area, controlled by commands |
| [Sign-in restrictions](https://docs.gitlab.com/administration/settings/sign_in_restrictions/) | Read | Admin mode: a login again before admin rights, for a limited time |
| [GitLab Admin area settings](https://docs.gitlab.com/administration/settings/) | Read | Settings stored in the database |
| [Configuration options for Linux package installations](https://docs.gitlab.com/omnibus/settings/configuration/) | Partly read | Settings fixed in `gitlab.rb` are locked in the Admin Area, naming who manages them |
| [Application settings API](https://docs.gitlab.com/api/settings/) | Through search | The same settings through the API |
| [Active sessions](https://docs.gitlab.com/user/profile/active_sessions/) | Through search | A person ends their own sessions |

## Mattermost

| Source | Read | Used for |
|---|---|---|
| [Environment configuration settings](https://docs.mattermost.com/administration-guide/configure/environment-configuration-settings.html) | Partly read | Console path, file key and variable per setting; logging settings |
| [Environment variables](https://docs.mattermost.com/administration-guide/configure/environment-variables.html) | Read | A variable wins and locks the console field |
| [Store configuration in your database](https://docs.mattermost.com/administration-guide/configure/configuration-in-your-database.html) | Read | Configuration moved into the database |
| [Experimental configuration settings](https://docs.mattermost.com/administration-guide/configure/experimental-configuration-settings) | Through search | A setting that hides settings from system administrators |
| [Backup and disaster recovery](https://docs.mattermost.com/deployment-guide/backup-disaster-recovery.html) | Read | Backups with the database's own tools |
| [Upgrade Mattermost Server](https://docs.mattermost.com/administration-guide/upgrade/upgrading-mattermost-server.html) | Read | Upgrades by command |
| [User management configuration settings](https://docs.mattermost.com/administration-guide/configure/user-management-configuration-settings.html) | Read | Ending a person's sessions from the console |

## Nextcloud

| Source | Read | Used for |
|---|---|---|
| [Backup](https://docs.nextcloud.com/server/latest/admin_manual/maintenance/backup.html) | Read | Backups by command, with maintenance mode |
| [Upgrade via built-in updater](https://docs.nextcloud.com/server/latest/admin_manual/maintenance/update.html) | Read | The web updater and its limits; the command line preferred for larger installations |
| [Configuration Parameters](https://docs.nextcloud.com/server/latest/admin_manual/configuration_server/config_sample_php_parameters.html) | Partly read | `config.php`, extra configuration files, a read-only configuration |
| [System and maintenance commands](https://docs.nextcloud.com/server/stable/admin_manual/occ_system.html) | Read | Maintenance mode, upgrade and log level by command |
| [User and group commands](https://docs.nextcloud.com/server/stable/admin_manual/occ_users.html) | Read | Ending sessions by command; a disabled account's sessions end later |

## Discourse

| Source | Read | Used for |
|---|---|---|
| [Create, download, and restore a backup of your Discourse database](https://meta.discourse.org/t/create-download-and-restore-a-backup-of-your-discourse-database/122710) | Read | Backups and restore in the admin pages; restore off until allowed; it logs everyone out |
| [Using Hidden Site Settings](https://meta.discourse.org/t/using-hidden-site-settings/308773) | Read | Settings outside the admin pages, with a warning |
| [Settings order of precedence with ENV vs DB?](https://meta.discourse.org/t/settings-order-of-precedence-with-env-vs-db/248965) | Read (a community answer) | Environment values winning over the database |
| [Read Only Modes in Discourse](https://meta.discourse.org/t/read-only-modes-in-discourse/296976) | Through search | A read-only mode that also applies to administrators |
| [When is upgrade actually possible from admin dashboard?](https://meta.discourse.org/t/when-is-upgrade-actually-possible-from-admin-dashboard/322894) | Through search (a forum thread) | The upgrade page and its limits |

## Gitea

| Source | Read | Used for |
|---|---|---|
| [Backup and Restore](https://docs.gitea.com/administration/backup-and-restore) | Read | A dump command; restore by hand |
| [Configuration Cheat Sheet](https://docs.gitea.com/administration/config-cheat-sheet) | Partly read | `app.ini` and environment variables; a restart applies a change |
| [Gitea Command Line](https://docs.gitea.com/administration/command-line) | Read | Log outputs and levels changed at run time by a command |
| [Upgrade from an old Gitea](https://docs.gitea.com/installation/upgrade-from-gitea) | Read | Upgrades by replacing the binary; migrations at start |

## Diffluence's own documents and code

Read on 2026-11-14 in main `4b8a343`:

- Blueprint: `08-roadmap.md` ("Running the installation from the admin screen"), `10-decision-log.md` (ADR-019, ADR-036, ADR-039 to ADR-044), `07-operations/` for the earlier study of operations.
- Specs read in full: 058, 064 (for the form of a spec), 068, 069, 070. Specs 054, 056, 059, 065 and 066 as the decision log, the operations guides and the code describe them.
- Security: `docs/security/threat-model.md`, `docs/security/risks.md`.
- Operations: `install.md`, `upgrade.md`, `backup-restore.md`, `monitoring.md`, `incidents.md`, `limits.md`.
- Design: `docs/design/README.md`.
- Code: `cmd/diffluence/` (`main.go`, `key.go`, `client.go`, `worker.go`), `internal/config/`, `internal/settings/sections.go`, `internal/health/health.go`, `internal/httpapi/handlers_running.go` and the routes of the other admin handlers, `internal/httpapi/router.go`, `web/src/views/admin.tsx` and `admin-running.tsx`, `compose.yaml`, `.env.example`, `deploy/` and `scripts/`.
