# Which versions may meet

Written for: operators, and for whoever writes a release.

During an upgrade, two releases share one database, one queue and one storage folder for a while. This page says which parts may differ by how many releases, for how long, and what proves it (OPS-075). It follows the way Kubernetes writes its version-skew policy, and the research on upgrade failures behind blueprint 07: most of them come through stored data and messages, not through reads (07.1).

## The rules

| Part | May differ by | For how long | Proven by |
|---|---|---|---|
| Nodes (`api`, `worker`, `all`) | One release: N and N+1 side by side | Until the last old node stops. Contract migrations of N+1 wait until every live node knows them, plus `DIFFLUENCE_CONTRACT_GRACE` (a day by default) | `scripts/upgrade-test.sh` in CI, on every change |
| Rolling back | One release: N starts again on a database N+1 migrated | Until N+1's contract migrations ran; after that N refuses to start and names the migration | The rollback step of the upgrade test; `TestOlderNodeRefusesADatabaseAfterAContractMigration` |
| Jobs in the jobs table | One release | A job of a kind a node does not know waits for a node that does | `TestAJobOfANewerReleaseWaitsForANodeThatKnowsIt` |
| Events in the outbox and the queue | One release | Every listener ignores a type or a field it does not know | `TestEveryListenerLetsAnEventOfANewerReleasePass`; the upgrade test checks that no event is set aside |
| Settings | One release | Saving a section keeps the fields a newer release stored | `TestSavingSettingsKeepsFieldsOfANewerRelease` |
| Page documents | Any older version | A document is upgraded step by step when read, never rewritten; a newer one is left as it is | `internal/document/upgrade.go` and its tests |
| Export bundles | Any older format (1 to 5 today) | A newer bundle is refused with a reason | the bundle tests in `internal/exchange` |
| Files | Any | The store keeps bytes and a version per file; nothing in the format depends on the release | the storage tests |
| The editing service (`collab`) | Same release as the nodes; one behind only during an upgrade | It never writes a page version itself: it asks the API who may do what and hands the shared document to the API to save | Not tested across releases yet |

## Jumping releases

Zero downtime is promised between consecutive releases only. An installation up to **six months** old may jump straight to the newest release, with a short stop: stop every old node, then start the new ones, which run every migration in order (spec 048 D3). Older than that, upgrade in steps of at most six months. CI proves the six-month jump from 1.0 on (OPS-076); before 1.0 there is no release that old.

## What a release must keep

- **Only add, then remove later.** A migration of release N+1 only adds (expand). A contract migration only removes what the **previous** release already stopped reading and writing (spec 048 D2), so the release before it never needs what is gone, also after a rollback.
- **Read what the previous release wrote.** A new field gets a default for rows written without it. A new event type, job kind or settings field must be harmless to a node that does not know it.
- **Write what the previous release can read.** A change of a stored format that the previous release cannot read needs two releases: first one that reads both, then one that writes the new one.
- **Never rewrite a busy table in a migration.** See the rewrite check at the top of `internal/db/migrate.go`.
