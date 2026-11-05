# 03.10: Administration

## 10.1 The console

- **ADM-001** · built · **Administration** in the account menu opens the console, with a vertical menu.
- **ADM-002** · built · The sections are users, groups, global permissions, spaces, settings, SCIM tokens, audit log and system health.
- **ADM-003** · new · The console has a search over settings, so an operator finds things by name.
- **ADM-004** · new · The first page is a summary: number of users, spaces, pages and files, storage used, failed jobs, and things that need attention.
- **ADM-005** · new · Sensitive changes ask for the password or passkey again, even in a running session.

## 10.2 Users

- **ADM-006** · built · An administrator lists users with search, opens one, and changes name, type, permissions and status.
- **ADM-007** · built · Disabling a user ends their sessions at once. They can no longer log in by any method.
- **ADM-008** · new · The list filters by type, status, group, last login and how they log in.
- **ADM-009** · new · Bulk actions: disable, add to group. Import of users from CSV for instances without SCIM.
- **ADM-010** · new · A user's screen shows groups, spaces, last login, and pages they own, and offers to hand over ownership.
- **ADM-011** · new · A list of people who have not logged in for a chosen time helps to clean up.
- **ADM-012** · built · Administrators cannot read content through user screens.

## 10.3 Look of the instance

- **ADM-013** · new · An administrator sets the instance name and logo, shown in the top bar and on the login page. The file is checked like any image.
- **ADM-014** · new · An administrator chooses an accent colour. The app checks contrast in light and dark, and refuses a colour that fails.
- **ADM-015** · built · An administrator imports a WOFF2 or WOFF font, which replaces the system font, and removes it again. It is served from our own address.
- **ADM-016** · new · An administrator writes the login page's welcome text and a link to terms.
- **ADM-017** · new · A site-wide banner shows a short message, with a level and an end date, for maintenance or news. People can dismiss it.
- **ADM-018** · new · A reset returns everything to the defaults in one action.

## 10.4 Rules for the whole instance

- **ADM-019** · built · Trash retention period, default 30 days.
- **ADM-020** · built · Maximum upload size, default 100 MiB.
- **ADM-021** · new · Which features are on: comments, reactions, blogs, invitations, personal spaces, self sign-up, analytics, automation.
- **ADM-022** · new · Default language and time zone for new users.
- **ADM-023** · new · The default set of permissions for new spaces.
- **ADM-024** · new · Allowed outside addresses for link cards and embeds (EDT-067), empty by default.
- **ADM-025** · new · Audit retention: at least one year by default, adjustable by a system administrator.
- **ADM-026** · new · Draft retention (PAG-019).
- **ADM-027** · new · Whether notifications may contain text from pages (COL-076).

## 10.5 E-mail

- **ADM-028** · partly · An administrator sets the mail server, sender and reply-to address, with a Send test button. The password comes from a file or the environment, never from the screen. Built: settings from the environment, and Send test. New: a screen for the rest (wave F).
- **ADM-029** · built · A queue view shows sent and failed mails, with no content.

## 10.6 Spaces

- **ADM-030** · built · An administrator lists all spaces, also ones they cannot read, with key, name, status and owners.
- **ADM-031** · built · An administrator recovers access to a space.
- **ADM-032** · built · An administrator restores or purges spaces from the trash.
- **ADM-033** · new · The list shows the number of pages, storage used and last activity, and can be filtered by them.
- **ADM-034** · new · A list shows spaces with no active administrator.

## 10.7 Templates and content defaults

- **ADM-035** · new · The console manages instance-wide templates, with the starter set (ORG-053).
- **ADM-036** · new · The console manages the list of statuses that new spaces start with.
- **ADM-037** · new · The console manages space categories.

## 10.8 Audit

- **ADM-038** · built · Every user and admin action is recorded with who, what, when and from where.
- **ADM-039** · built · The log is append-only and hash-chained, and the application's database role cannot change it.
- **ADM-040** · built · An administrator filters the log and exports it as CSV.
- **ADM-041** · built · A daily job verifies the chain and signs a checkpoint.
- **ADM-042** · built · Page views are not in the audit log (decision of 2026-11-03).
- **ADM-043** · new · The log filters by person, action, object, space, result and time, and the filters are part of the address.
- **ADM-044** · new · A line opens to show all its details, and links to the object.
- **ADM-045** · new · A space administrator sees the audit log of their own space.
- **ADM-046** · later · The log streams to an outside system, through syslog or a webhook, for a SIEM.

## 10.9 System health

- **ADM-047** · built · A screen shows nodes, versions, migrations, the queue, storage and failed jobs.
- **ADM-048** · new · Jobs can be retried or cancelled from the screen.
- **ADM-049** · new · A check lists what needs attention: mail not set, backup not seen, a node with an old version, disk nearly full.
- **ADM-050** · new · Diffluence tells an administrator when a new release exists, only when they allow it to check, because a check reveals an installation.
- **ADM-051** · built · Liveness and readiness endpoints serve load balancers.
- **ADM-052** · phase 3 · Metrics for Prometheus and traces for OpenTelemetry.
- **ADM-053** · new · **Maintenance mode:** a system administrator makes the instance read-only with a banner, for a backup or migration.
- **ADM-054** · built · Upgrades need one command, and contract migrations run by themselves once every node knows them.

## 10.10 Data care

- **ADM-055** · new · A report shows the largest spaces, pages and files.
- **ADM-056** · new · A job finds stored files that no attachment uses, and reports them before removal.
- **ADM-057** · built · Backup and restore follow the guide in `docs/operations/backup-restore.md`.
- **ADM-058** · phase 3 · An automatic restore test proves that a backup works.
