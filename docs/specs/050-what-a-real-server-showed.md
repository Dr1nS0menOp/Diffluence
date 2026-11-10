# Spec 050: what a real server showed

Status: built on branch `wave-ld`.

Phase: the load work of ADR-035 and ADR-036 (blueprint 04, section 8.5), after the first test on a real server ([the stress test](../reports/2026-11-10-stress-test.md)). Wave FU ran at the same time (permissions and the Access screen), so this wave stays out of permission rules and role code: it changes how three lists ask the rule, never the rule.

## The study

Read for this wave: the stress test report, ADR-035 and ADR-036, spec 031 (an order of work under load), `docs/operations/limits.md`, the load budget rule ARC-238 and the query budgets of ARC-239, the code of the feed (`internal/feed`), the bell (`internal/notify/list.go`), file search (`internal/attachments/attachments.go`), the audit chain (`internal/audit/audit.go`), the pool (`internal/db`), admission (`internal/httpapi/admission.go`) and the passkey set-up.

### Requests and connections

Admission (spec 031) lets as many saves through as the pool has connections, on the idea that a handler holds one connection at a time. The study looked for code that holds a connection, in a transaction or as a querier handed in, and then asks the pool for another. A script listed every call made while a transaction is held that does not pass that transaction on, and each was read by hand. Found:

| Where | What took a second connection | On which path |
|---|---|---|
| `features.Start`, `features.ForSpace` | `settings.Read` of the instance's features | Creating a space; every check whether a feature is on |
| `notify` | `settings.Get` for mail | Every notification made, on every save that notifies |
| `comments.reactionsOn` | `settings.Get` | Reacting |
| `attachments.AddImported` | `settings.MaxUploadBytes` | Importing a bundle |
| `identity.needsSecondFactor` (login) | The second-factor rule and the age of the instance | Every login |
| `identity` account mail | The mail queue | A new device, a change of sign-in, an invitation |
| `identity.mayGrant` | `HasGlobalPermission` | Changing someone's global permissions |
| `sharing` invitations | The features section and `HasGlobalPermission` | Asking for and approving invitations |
| `sharing.checkMayRequest` | `MayAsk` with its page and features | Asking for access |
| `sites.requirePublisher` | `pages.SpaceOf` | Public links |
| `templates.requireManager`, `trash.authorize` | `HasGlobalPermission` | Templates of the instance, deleted spaces |

When as many of these run at once as the pool is large, each holds one connection and waits for a second that no one gives back. Nothing on the server side ended that wait.

A script finds only what it is written to find, so fixing the list is not enough on its own. The second half is a limit on every wait of a request for a connection, so anything the list missed ends as "busy" instead of waiting without end.

### Three queries

EXPLAIN on data of the stress test's size (`scripts/stress/bench-data.sql`, see below) showed the same three costs as the server:

- **The feed** joined `perm_visible_pages` (about 50,000 pages for a member of 250 spaces) to every version, sorted them per page, and only then kept 20.
- **The bell** joined `perm_visible_pages` to a person's unread rows. With a page tree of real depth the rule cost about 50 ms, every poll.
- **File search** asked `EXISTS (perm_listed_pages(...) WHERE page = this file's page)` for each candidate file. PostgreSQL ran the check of the person's spaces again for each one: `space_grants` was read 343 to 1,892 times in one search.

The code already has the answer to this in two places (`search/words.go`, `pages/branches.go`): pick the few candidate rows first, then ask the rule once with `v.page_id = ANY(<their page ids>)`, which uses the index of `pages`.

### The audit lock

`audit.Record` takes one advisory lock for the chain and holds it until the transaction commits, and a commit waits for the disk to sync. All audited writes go through that lock one at a time, so they run at the pace of the disk's sync: about 160 a second on the server's second volume, which explains 3.7 s for saving with 5,000 people at once. A shorter lock means another design of the chain, such as hashing after commit by one writer; that changes what ADR-013 and ADR-039 guarantee, and this wave cannot prove it safe. **Decided:** the lock stays, and the install guide and known limits say what it costs and that a disk that syncs fast takes the cost away.

### The public address

`identity.NewWebAuthn` uses the host of `DIFFLUENCE_PUBLIC_URL` as the passkey RP ID, and the library refuses an IP address. The app stopped at start, and Compose restarted it forever. **Decided:** the configuration refuses an IP address at start and asks for a host name, rather than starting without passkeys. Why: passkeys are a second factor and part of how login stays safe; an installation that quietly lacks them would be weaker without anyone choosing that, and a second way to run the app adds paths to test. The address never worked, so no working installation changes. `localhost` stays allowed, because browsers accept it as a name.

## What this wave builds, worst risk first

| Step | What | Load budget (ARC-238) |
|---|---|---|
| 1 | Every place in the table above reads through the connection it holds: `settings.GetIn`, `ReadIn`, `ReadSectionIn`, `identity.HasGlobalPermissionIn`, `features.instanceIn`, `sharing.mayAskIn`, and the mail queue and second-factor rule take the querier of the caller. A refused invitation is audited after its transaction, as before but no longer inside it | Fewer connections, the same queries |
| 1 | A request waits at most 10 seconds for a database connection. `db.WithAcquireLimit` marks the context of a request, the pool's acquire tracer applies the limit to each wait and notes when it ran out, and the server answers such a request 503 "busy" with `Retry-After`, as admission does. Background work keeps waiting | A timer per wait for a connection |
| 2 | The feed reads versions newest first, and asks for each whether it is the newest that was not silent and whether its page is visible, until it has enough. The popular tab takes as candidates only pages with a reaction or comment in its days | About 20 versions read for 20 items, instead of every visible page |
| 3 | The bell counts unread rows first, and asks the rule only about their pages | A few page lookups per poll |
| 4 | File search picks the files that match first, then asks `perm_listed_pages` once about their pages | One check of the person's spaces per search |
| 5 | The audit lock: no change, see above | |
| 6 | An IP address as public address is refused at start, with the reason. The install guide gives PostgreSQL memory settings by machine size, `pg_stat_statements` with `CREATE EXTENSION`, and what the audit lock costs on a slow disk | |
| 7 | `docs/operations/limits.md` gets the server's numbers and this wave's changes, and says plainly that the load test has not been repeated on the server | |

The notification list was measured too: it was already 3.8 ms, as PostgreSQL picks the person's rows first there, so it stays as it was.

## How it is proven

- `TestManySpacesCreatedAtOnceAllFinish`: 12 creations on a pool of 3 connections. Before: the creations hung until the client gave up after 30 seconds. After: all 12 in 0.6 s.
- `TestARequestOnAFullPoolIsToldBusyInTime`: every connection held, as stuck work would; a request answers 503 with `Retry-After` after 10 seconds.
- `TestAWaitForAConnectionGivesUpAtTheLimitOfItsRequest`, `TestAWaitWithoutALimitIsNotCountedAsRunningOut`, `TestAWaitWithinItsLimitGetsTheConnection`, on a pool of one connection.
- `TestTheFeedGivesWhatTheFormerQueryGave`: the feed through the API, a few items at a time with the cursor, against the former query kept in the test, for an owner, a reader and an outsider, for recent, for you and popular, over restricted branches, silent versions, drafts and other spaces.
- The existing tests of visibility: `TestTheFeedShowsOnlyWhatAPersonMaySee`, `TestWatchersHearOnceAndLosingAccessEndsIt` (a restricted page leaves the bell and comes back), `TestQuickSearchShowsGroupsWithoutRevealingAnything`.
- `TestAnIPAddressIsRefusedAsThePublicAddress`.
- Twelve mutations, one for each new check: in `scripts/mutation-check.sh` under "Spec 050".

### Measured

`scripts/stress/bench-data.sql` fills an empty, migrated database in under a minute, by writing the tables directly: 5,000 people in 40 groups, 300 spaces of which 250 open to all members, 60,000 pages in trees four children wide, 120,000 versions with a few silent, a restriction on one page in 200, 30,000 comments, 12,000 files and 250,000 notifications. EXPLAIN ANALYZE in PostgreSQL 16 in the development container, for a member who sees 250 spaces, one warm run each:

| Query | Before | After |
|---|---|---|
| The feed, recent | 379 ms | 20 ms |
| The count on the bell | 55 ms | 5.8 ms |
| File names, a common word | 268 ms | 41 ms |
| File names, a rare word | 277 ms | 18 ms |
| File names, near a typo | 1,363 ms | 78 ms |

## How the product owner tests it

1. Open the start screen, the bell and the quick box with a file name: they show what they showed before, only sooner.
2. Set `DIFFLUENCE_PUBLIC_URL=http://192.0.2.10:8080` (any address) and start: the app stops at once and the log asks for a host name. With `http://localhost:8080` it starts.
3. On the test server, run the stress test again (`scripts/stress/README.md`) and compare with the report of 10 November.

## What one machine cannot prove

- The full load test with these changes, on the server: the numbers above are single queries in a container. The re-run on the server is next.
- That no other place holds a connection and asks for a second one. The script reads one pattern, a deeper call chain can hide one, and the 10-second limit is what makes that safe.
- How the audit lock behaves on fast disks.

## Open for the product owner

- **The audit lock (ARC-203).** The blueprint already names the way out: write events without a hash in the transaction, and let one worker chain them seconds later, so the log stays complete and tamper-evident with a short tail that is not chained yet. It changes ADR-013, so it waits for a measurement and the product owner's agreement. The measurement now exists: with all 5,000 people at once, saving waited 3.7 s (p50) on a disk that syncs about 160 times a second. On a fast disk the lock costs far less, so the question is whether Diffluence should carry that load on slow disks.

Decided in this wave, with the reasons above: the lock stays for now (step 5), and an IP address is refused rather than started without passkeys (step 6).
