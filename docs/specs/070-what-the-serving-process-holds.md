# Spec 070: migrations, checkpoint signing and restore orders move to the backup service

Wave RV3b. Status: built. Phase: hardening before 1.0.

## Problem

The reference installation runs two long-lived processes of the same binary: the server, which answers the network, and the backup service, which already holds the address of the owner of the tables, the data folder and the backup key. Until now the server also held the owner's address, ran every migration itself, signed the audit checkpoints with a key derived from `master.key`, and the backup service carried out any restore it found in the schema `ops`.

This spec moves three duties to the backup service, and gives the server only what it needs to serve:

1. **Migrations.** The backup service runs the expand migrations of a new release and, after the grace period, its contract migrations. The server container gets only the address of the application role.
2. **Checkpoint signing.** The backup service signs the checkpoints of the audit chain with a key derived from the backup key, which the server never reads. Verifying needs only public keys.
3. **Restore orders.** The backup service carries out a restore only with a one-time confirmation code it wrote to its own log, which an operator types into the console.

Each section sets out the options, the choice, and the way to a stronger form later.

## What each process holds afterwards

| | Server container (`diffluence`) | Backup service (`backup`) |
|---|---|---|
| Application role address | yes | yes |
| Owner address | **no** (was yes) | yes |
| `master.key` | yes (it encrypts files and secrets) | yes, read-only through `DAC_OVERRIDE`, as before |
| Backup key | no | yes |
| Checkpoint key | no (it held the derived one) | yes, derived from the backup key |
| Restore confirmation key | no | yes, derived from the backup key |

What the application role may still do is unchanged, except that it may only read the new table of checkpoint keys (D6).

## 1. Migrations run in the backup service

### Options

| Option | For | Against |
|---|---|---|
| A. A one-shot `migrate` service, with the server waiting on `service_completed_successfully` | Standard Compose pattern; the owner address lives only seconds | A refused upgrade fails `docker compose up` instead of showing the waiting page of ADR-042; a third container to build and keep; contract migrations still need a long-lived place |
| **B. The backup service** | Already holds the owner address and runs on every Compose install by default; one place for expand and contract migrations; the server keeps its waiting page | The backup service becomes a little more than backups |
| C. The operator runs `diffluence migrate` by hand | No new code | Breaks "upgrades have no manual steps" (blueprint 07) |

**Choice: B.** The server stays in charge of what people see: while migrations wait, it shows the waiting page with the reason and the command that helps.

### Behaviour

- **Server.** `compose.yaml` gives the server only `DIFFLUENCE_DATABASE_URL_FILE` and sets `DIFFLUENCE_MIGRATE_ON_START=false`.
  - A node that does not migrate on start refuses a database whose expand migrations of its release have not run. It does not exit: it serves the waiting page with the new kind `migrations`.
    - The page names the fix: `docker compose up -d --build backup`, and `docker compose logs backup` for the reason.
    - When the backup check would refuse the upgrade, the page names that kind too.
    - While `migrations` is the only kind, the node looks again every 5 seconds instead of every minute, so a new installation is up within seconds of the backup service migrating it.
  - A node that has no owner address and does not migrate on start leaves contract migrations to whoever migrates.
- **Backup service.** Every round of its loop, at most a minute apart, it first runs `diffluence worker migrate`:
  - with expand migrations pending, the checks of `diffluence migrate` (the backup rule of ADR-040 and ADR-042, and the upgrade marker), then the migrations, the session defaults and the grants of the application role;
  - with contract migrations pending, the rule the server's worker used until now: every live node knows them, and the newest has run for `DIFFLUENCE_CONTRACT_GRACE`. Each is recorded as `system.contract_migrated`;
  - otherwise nothing, in two indexed reads.
  - On a refusal it logs the reason and tries again next round; the backups go on.
- **Command line.** In a container without the owner address, `diffluence migrate` and `diffluence user create-admin` do not try to migrate as the application role.
  - `migrate` says in one sentence where to run it: in the backup service.
  - `create-admin` refuses a database that is not migrated yet, and says to wait for the backup service.

### An installation that keeps its old `compose.yaml`

The server there still holds both addresses and `DIFFLUENCE_MIGRATE_ON_START` is unset, so it migrates as before, beside the backup service; the migration lock lets them take turns. Nothing fails. The log says, at the moment the server migrates, that this layout gives the server the owner address and that the new `compose.yaml` moves migrations to the backup service. With a new `compose.yaml` and a backup image from before this release, the server waits on the page that names `docker compose up -d --build backup`.

### Upgrade path

- Helm, the LXC script and the packages keep the previous layout: one process with both addresses. Each gets the same split in its own wave: a Kubernetes Job before the Deployment, and a systemd unit for the worker.
- Option A stays open for a platform where the backup service is not wanted.

## 2. Checkpoints are signed in the backup service

### What exists

The checkpoints of ADR-039 are Ed25519 signatures. The private key is derived with HKDF from the root of `master.key`, which the server needs anyway to open files and secrets. Each checkpoint row and witness line carries its public key, but verification uses the one key derived from `master.key`.

### Options for the new key

| Option | For | Against |
|---|---|---|
| A. A separate key file made by `setup-secrets.sh` | Purpose kept apart from the backup key | Every existing installation must run the script again before anything changes; one more file to keep safe and back up |
| **B. Derived with HKDF from the backup key** | Held only by the backup service already; kept in a password safe already, as the guide asks; no step for the operator; Go's standard library only | Whoever has the backup key can also sign checkpoints. Both are already one secret of the same holder |
| C. A KMS or hardware key | Strongest | A dependency and an outside service, which ADR-029's "no budget" and the single-node target rule out for now |

**Choice: B.** HKDF with its own label gives an independent key from one secret, as `internal/keys` already does for the fingerprint and checkpoint keys of `master.key`.

### A chain of keys, and the point where the key changes

- A new table `audit_signing_keys` lists every key after the first: from which position of the chain it signs (`from_seq`), its public key, and an **endorsement**: a signature over `from_seq` and the public key, made with the installation's own checkpoint key, the one derived from `master.key`.
- The key that signs a checkpoint at position `s` is the newest key whose `from_seq` is at most `s`; before the first entry it is the installation's key. So existing checkpoints keep verifying unchanged.
- **The change point.** The first time the backup service runs `diffluence worker checkpoint` with a backup key, it:
  - takes the lock of the chain, so nothing is sealed meanwhile;
  - writes the new key with `from_seq` one past both the head of the chain and the newest checkpoint;
  - records `audit.checkpoint_key_changed`, with the position and the public key;
  - signs a checkpoint with the new key at once.
- When the backup key is replaced, the derived key changes, and the next round makes another change point the same way. A lost key therefore stops nothing.
- A trigger on `audit_checkpoints` refuses a checkpoint whose public key is not the key in force at its position. The application role may only read `audit_signing_keys`.
- **Verification**, in the console, the daily job, `diffluence audit verify` and the check after a restore:
  - checks each endorsement against the installation's public key;
  - checks each checkpoint against the key in force at its position;
  - accepts an anchor of a prune signed by the installation's key or by an endorsed key.
  - The witness comparison uses the same rule.
- The server's own checkpoint job stops by itself once a newer key is in force, so the two never race. An installation without the backup service, or without a backup key, keeps signing in the server as before.
- The backup service writes each witness line to its own log. `docker compose logs --no-log-prefix backup | diffluence audit verify --witness -` compares them.

### What each control holds against (D9)

- The server's own process can no longer sign a checkpoint that verifies after the change point, and its database role cannot add a key or change a checkpoint.
- Someone with the owner's address but neither key cannot endorse a key, so a rewritten chain stops verifying.
- Someone who holds both `master.key` and the owner's address can endorse a key of their own and rewrite the chain. The witness lines, in a log the database owner cannot change, still show it.

### Upgrade path

A key pinned out of band, such as a public key the server reads from a read-only mount, would also stop the third case without the witness. A KMS can hold the backup service's key later without changing the table.

## 3. The backup service carries out a restore only with its own code

### What exists

The console writes a request into `ops.restores`, after a fresh login, the installation's name typed out and a second factor (specs 059 and 066). The backup service, which runs as root with the owner's address, takes every request in state `requested`. The application role may write the whole schema `ops`.

### Options

| Option | For | Against |
|---|---|---|
| **A. A one-time code in the backup service's log, typed into the console** | The server never holds what makes the code; the person stays in the console; a log collector can show the code to someone without a shell | Reading the log needs access to the host or to the log collector |
| B. A confirmation command run in the backup service | Same strength, no console step | Takes the restore out of the console, which the product owner chose to keep |
| C. A request signed on the administrator's device, such as by a passkey | Ties the order to a person | A WebAuthn verifier in the worker, a stored credential it trusts, and much more code |

**Choice: A.** The command line keeps a road as strong as B: `diffluence backup restore-request` run inside the backup service holds the key, so it fills in the code itself.

### Behaviour

- **The code.** It is made from an HMAC over the restore's id and backup name, with a key derived from the backup key. It is 10 digits, shown as `12345-67890`; spaces and dashes are ignored when typed.
- **Asking and confirming.**
  1. The console asks for a restore as before. The request waits in state `requested`.
  2. The backup service sees it within a minute, writes one line to its log, and notes that it did:
     `To restore diffluence-20261113T080000Z, enter this confirmation code in the console: 12345-67890 (valid 30 minutes)`.
  3. The console shows "Enter the confirmation code from the backup service's log", with the command `docker compose logs backup` and a field.
  4. A system administrator enters it. The console stores it beside the request and records `backup.restore_code_entered`, without the code.
  5. The backup service compares it in constant time on its next round:
     - **right:** it records `backup.restore_confirmed`, seals the request with a MAC over its id and backup name, and goes on as before;
     - **wrong:** it records `backup.restore_code_wrong`, clears the field and allows another try; after the third, the restore ends `refused` and `backup.restore_refused` is recorded;
     - **no code within 30 minutes:** the restore ends `refused`, recorded the same way.
- **Every later step checks the seal**: preparing, the safety backup and the restore itself. A restore whose backup name changed after it was confirmed, or that never went through the code, ends `refused`, or `failed` if it had changed something, with `backup.restore_refused` recorded.
- **The way back** restores only a safety backup the backup service sealed when it made it. A failed restore inserted by hand is refused. The waiting page and the console still name the way back, as before.
- **Without a backup key** the backup service cannot make codes. It refuses every request at once and says so in the restore's step.

The application role may still insert or change rows in `ops.restores`, but nothing it writes makes the backup service change the installation.

### Limits

- The code is checked at most once a round, and the console asks only after a fresh login and a second factor. Ten digits leave 10^10 codes; an attacker who writes guesses straight into the table, resetting the count each time, needs on average thousands of years at one try a minute.
- Every wrong code is in the audit log.

## 4. The wording

- **ADR-044** records the three moves. ADRs are never edited, so ADR-019 stands and ADR-044 states what it now covers: the owner's address is held by the backup service alone in Compose.
  - ADR-043 is left for wave RV3a, which runs beside this one.
- **Threat model:** the rows for the audit log and for restore name each control and what it holds against.
- **`docs/security/risks.md`:** the line on `master.key` and checkpoints changes to the new rule.

## Load

- **Backup service, each round, at most once a minute:**
  - two indexed reads of `schema_migrations` and `nodes`;
  - one read of the newest checkpoint time, and a signature every five minutes when something happened, as the server did;
  - the restore query it already ran.
- **Server:**
  - one read of the newest row of `audit_signing_keys` every five minutes;
  - at start, the read of `schema_migrations` it already made.
- **Per checkpoint insert:** the trigger costs one indexed lookup in a table of a handful of rows.
- **Per restore:** one row update per round while a code is awaited.
- No change to saving, reading or search. Negligible against the ARC-233 budget.

## Acceptance criteria

- [x] A server with only the application role's address and `MIGRATE_ON_START=false` waits on an unmigrated database with the kind `migrations`, and serves once the backup service migrated it.
- [x] `diffluence worker migrate` applies expand migrations with the backup check and the grants, and contract migrations after the grace period, recording each.
- [x] A server without the owner's address runs no contract migration.
- [x] An old layout, where the server holds both addresses, keeps migrating as before.
- [x] Checkpoints verify across the change point. A checkpoint signed with the installation's key after it is refused by the database and fails verification. An endorsement by another key fails verification.
- [x] The witness comparison and a prune anchor follow the key in force.
- [x] The application role cannot write `audit_signing_keys`.
- [x] A restore request written straight into `ops.restores` is refused and recorded:
  - without the code;
  - with a wrong code three times;
  - in a later state without the seal;
  - after its backup name was changed;
  - as a failed restore offered to the way back.
- [x] A restore with the right code from the console runs to the end, as before.
- [x] The restore drill passes end to end with the code, on its own image tags.

## How the product owner tests it

1. On a test installation, pull this release, `docker compose up -d --build`, and watch `docker compose logs backup`: it migrates, then backs up and signs checkpoints.
2. `docker compose config diffluence` shows the server with only the secret `database_url`, and `DIFFLUENCE_MIGRATE_ON_START: "false"`.
3. In the console, ask for a restore. Within a minute the backup service's log shows a code. Type a wrong one: the console says so. Type the right one: the restore runs.
4. `docker compose exec diffluence diffluence audit verify` reports the chain whole, with checkpoints before and after the change of key.

## Decided on the product owner's behalf on 2026-11-13, may be overruled

- **D1: the backup service migrates, not a one-shot service.** It already holds the owner address, and the server keeps its waiting page (section 1).
- **D2: Compose only in this wave.** Helm, the LXC script and the packages keep one process with both addresses, which still works. Their split is listed as upgrade path.
- **D3: an old `compose.yaml` keeps working.** The server there still migrates, and logs that the new layout moves migrations to the backup service. Refusing to start would stop installations that changed nothing; a silent loss of the split is avoided by the log line and the changelog.
- **D4: the checkpoint key is derived from the backup key.** No new file, no step for the operator, the same holder (section 2).
- **D5: a chain of endorsed keys, not a key per row trusted as found.** Existing checkpoints keep their key; each later key is signed by the installation's key at a recorded position; a replaced backup key just makes the next change point.
- **D6: the database refuses a checkpoint signed with a key not in force.** This avoids a false alarm when an older server runs beside a newer backup service during an upgrade, and refuses a checkpoint written by the server after the change.
- **D7: the restore code is 10 digits, lasts 30 minutes, and allows 3 tries.** Long enough not to be guessed at one try a round, short enough to type. Three tries forgive a typing error without making a new request, which the limit of one a day would block.
- **D8: the code and the seal are derived from the backup key too.** The backup service needs that key for any restore anyway.
- **D9: the threat model states the third case plainly.** Whoever holds both `master.key` and the owner address can still rewrite the chain; the witness shows it.
- **D10: ADR-044, not ADR-043.** Wave RV3a runs beside this one and may need the next number; a gap in the numbers costs less than two ADRs with one number.

## Provenance

Own design. Go standard library documentation for `crypto/ed25519`, `crypto/hkdf` and `crypto/hmac` (pkg.go.dev, 2026-11-13). PostgreSQL 16 documentation on `CREATE TRIGGER` and privileges (postgresql.org, 2026-11-13).
