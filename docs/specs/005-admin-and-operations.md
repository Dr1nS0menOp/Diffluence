# 005: Admin console, background jobs, upgrades

Status: built, waiting for the product owner's acceptance session (written by Claude from the blueprint)
Phase: 1 and 2

## Problem
An operator must be able to run Diffluence without touching the database, and upgrades must need no manual steps.

## Behaviour

**Admin console.** Administrators manage users, groups, global permissions, all spaces (including recover access), settings, SCIM tokens, the audit log, and see system health: nodes, versions, migrations, queue, storage and failed jobs.

**Settings.** Trash retention and maximum upload size can be changed at runtime. So can the font (spec 006). Authentication providers are configured by the environment, because they hold secrets, and are shown read-only.

**Background jobs** run in the worker and only once across nodes: remove expired sessions and challenges, remove old published outbox rows, purge expired trash, delete stored files of purged items, verify the audit chain daily, sign a daily audit checkpoint with Ed25519, and run imports.

**Upgrades.** Every node registers itself with its version and the highest migration it knows. When every live node knows a pending contract migration, the worker applies it automatically after a grace period, so no human step is needed. `diffluence migrate --contract` stays as a manual override.

**Upgrade test.** A script starts the previous build with traffic, starts this build next to it on the same database, stops the old node, and fails on any failed request.

## Permissions and audit
All admin routes need *administrator*. Events: `settings.updated`, `settings.font_changed`, `audit.checkpoint_signed`, `audit.verified`, `system.contract_migrated`.

## Acceptance criteria
- [x] Two workers never run the same periodic job at the same moment
- [x] A contract migration waits while an older node is alive and runs once it is gone
- [x] A tampered audit row is reported by the daily verification
- [x] Checkpoints verify against the instance public key
- [x] The upgrade script passes from the walking skeleton to this build

## How the product owner tests it
Open the admin console and walk through every tab. Change the retention to 1 day. Follow the upgrade and restore guides in `docs/operations/`.

## Provenance
Own design from docs/blueprint/06, 07.
