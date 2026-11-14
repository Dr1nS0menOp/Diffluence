# Spec 076: changes of the console that let more through

Wave RV6. Status: built. Phase: hardening before 1.0.

## What this spec covers

Most settings of the console are an administrator's to change. A few let more through than before when they change one way, and nothing when they change the other:

| Section | Lets more through | Stays an ordinary change |
|---|---|---|
| Backups (`backups`) | fewer backups kept, or backing up less often | more backups, or more often |
| Retention (`retention`) | a shorter audit retention | a longer one, or for ever |
| Access (`access`) | a weaker rule for a second factor, a longer idle time or none, a new domain for signing up, guest invitations turned on | the opposite of each |

From this release a change that lets more through is a system administrator's, with a fresh login and a second factor. Keeping fewer backups or backing up less often also needs the code the backup service writes to its own log, the mechanism of spec 070. A shorter audit retention also needs a reason, and holds only after a week. Two smaller changes go with it: taking back a restore is a system administrator's, as asking for one is; and the code that confirms a restore can be entered while maintenance mode is on.

## 1. A confirmed change

### Behaviour

- `PUT /api/v1/admin/settings/{section}` stays as it was for an administrator's session. A change that lets more through is refused there with `403 confirmation_required`, which names what it needs.
- `PUT /api/v1/admin/settings/{section}/confirmed` takes `{"settings": {...}, "code": "...", "reason": "..."}` for the sections `access`, `retention` and `backups`; any other section is 404.
  - The route asks for system administration and a login at most 10 minutes old (`Sensitive`), as starting a restore does. The console's dialog for logging in again answers `reauth_required` by itself.
  - The handler asks for a second factor, as a restore does: a code from the authenticator app, a recovery code, or a passkey confirmed in this session within those 10 minutes. Wrong codes count against the same limit per account as for a restore, and each is audited.
  - The section is checked before the code, so a wrong address costs no code.
- The audit event `settings.updated` of a confirmed change carries `confirmed: true`.
- **What counts.** The module of each section decides, inside the transaction that stores it, against the section in force, with its row locked:
  - access: a lower place in the order none, administrators after the grace, administrators, all members; an idle time that is off or longer, where one was set; guest invitations turned on; a sign-up domain not listed before (case does not matter);
  - retention: fewer days, where zero is for ever;
  - backups: fewer kept or a longer interval, with zero standing for the backup service's default. Before any backup service said its defaults, a change to or from zero cannot be judged, so it counts as fewer.
- The console shows the fields of a confirmed change only to a system administrator, after the refusal, and only then.

### The console starts from what holds

`GET /api/v1/admin/settings/{section}` read the section under the wrong key and so always answered the defaults. With a check that compares against what holds, a form that starts from the defaults would read as a change back to them. It now reads the section where it is stored.

## 2. Fewer backups need the code from the backup service's log

### What exists

The backup service asks `diffluence backup next` every round, at most a minute apart, and prunes to the `keep` it is told. The server's database role writes the backup section. So far, whatever that section said was what the backup service kept.

### Options for the backup service's own guard

The backup service must never remove more backups, nor back up less often, than it did before a lower value was confirmed with its code, whatever the server's role writes.

| Option | For | Against |
|---|---|---|
| A. The confirmed value in the database, with a MAC from the backup key | No new place to keep state | The server's role can put back any older row with its MAC, such as a low value confirmed a year ago and raised since |
| B. A table only the owner of the tables may write | The database keeps it | In a `compose.yaml` of before spec 070 the server still holds the owner's address; a restore rewinds it |
| **C. A small file in the backup folder** | The server never mounts that folder; nothing it writes reaches the file; a replay has nothing to replay | One more file the backup service keeps; a new backup folder starts again |

**Choice: C.** It is the simplest form that holds against everything the server's role can write. The file `/backups/.diffluence-held.json` holds the fewest backups the service keeps, the longest it waits between two, and the code it waits for, if any. It holds no key and no code: only a random part the code is made from.

### Behaviour

1. **Raising is free.** Every round, what the console asks raises what the service holds at once: more backups, or a shorter interval.
2. **Lowering waits.** A system administrator saves a lower value through the confirmed route. The section is stored, and an ask is opened in `backup_schedule_requests`. Only one ask is open; a new one replaces the one before.
3. **The code.** Within a minute the backup service sees the ask, and only an ask for exactly what the section holds now. It draws 16 random bytes, keeps them in its file, and writes one line to its log:
   `To keep the newest 7 backups and back up every 168 hours, enter this confirmation code in the console: 12345-67890 (valid 30 minutes)`.
   The code is an HMAC, with a key derived from the backup key under its own purpose, over the random part, the ask, the number kept and the interval. It is 10 digits, shown as `12345-67890`.
4. **Entering it.** The Backups page shows that a code waits, with `docker compose logs backup` and a field. A system administrator enters it (`POST /api/v1/admin/backups/schedule/code`); `backup.schedule_code_entered` is recorded without the code.
5. **Comparing.** The backup service compares it in constant time on its next round:
   - **right:** it lowers what it holds to, records `backup.schedule_confirmed`, and prunes to the new number from then on;
   - **wrong:** it records `backup.schedule_code_wrong` and clears the field; after the third, the ask ends and `backup.schedule_refused` is recorded;
   - **none within 30 minutes:** the ask ends, recorded the same way.
   The count of wrong codes and the time the code was shown live in the service's file, so the server cannot reset them.
6. **Until then** the backup service keeps what it held, and the Backups page says so: "The backup service keeps 42 backups, and backs up at least every 4 hours, until the code from its log is entered." The backup service reports what it holds to in `backup_worker` every round, for the page.
7. **Without a backup key** the service can make no code: it ends the ask at once and keeps what it holds.
8. **A section changed past the console** asks for nothing: the service holds what it held, and the page shows it.

### The first round

A backup service that holds nothing yet starts from whatever keeps the most: the console's choice or its own defaults (`DIFFLUENCE_BACKUP_KEEP`, `DIFFLUENCE_BACKUP_INTERVAL`). An installation that kept fewer through the console before this release confirms that once with a code.

### Removing old backups

The shell of the backup service asks `diffluence backup held` for the number held before it removes anything, and never keeps fewer, also for `docker compose run --rm backup once`. When the file cannot be read, nothing is removed and the log says why. `diffluence backup held` reads only the file, so it works while the database cannot be reached.

## 3. A shorter audit retention

- It needs a confirmed change and a reason, one line of at most 500 characters.
- It holds 7 days after it was saved (`RetentionLoweringDelay`). Until then the longer time holds, for `diffluence audit prune` as for the console.
- The Retention form shows "From …, audit rows are kept for 90 days, because: …", and a button "Keep the longer time", which any administrator may press: it keeps more, never less (`DELETE /api/v1/admin/settings/retention/lowering`, audited as `settings.retention_lowering_cancelled`).
- An ordinary save keeps a waiting time as it is. A form cannot set one: the server ignores the `lowering` a client sends.
- A longer time stays as before: an administrator's change, with a reason beyond a year.

## 4. A restore

- `DELETE /api/v1/admin/backups/restore` asks for system administration, as starting a restore does. The Backups page shows the button only to a system administrator.
- `POST /api/v1/admin/backups/restore/code` is open during maintenance mode, beside logging in and switching maintenance off. Its route lets only a system administrator in, as before. Every other write still waits for the end of maintenance.

## Limits

- The code for fewer backups has 10 digits and three tries per ask; each ask needs a system administrator with a fresh login and a second factor, and each code is shown in the backup service's log. Every wrong code is audited.
- A backup folder that is emptied, or a new one, starts the file again, from the more keeping of the console's choice and the service's defaults.
- The console guard rests on the server; the backup service's own guard rests on the backup key and the backup folder, which the server never holds.

## Load

- **Backup service, each round:** one indexed read of the open ask, one row update of `backup_worker`, and one write of a file of under a kilobyte. While a code waits, one row update a round.
- **Server:** a confirmed change is one more locked read of the section, and for backups one read of `backup_worker` and one insert. The Backups page reads one more row.
- Nothing on saving pages, reading or search. Negligible against the ARC-233 budget.

## Acceptance criteria

- [x] An administrator without system administration cannot keep fewer backups, back up less often, shorten the audit retention, or weaken the rule for logging in; raising each stays theirs.
- [x] A system administrator needs a fresh login and a second factor for each; a wrong second factor is refused, counted and audited.
- [x] Fewer backups take effect only after the right code from the backup service's log: not before it, not with a wrong code, not with an old one, not with the code of another ask, not with a section or ask written straight into the database, and not without a backup key.
- [x] The backup service never removes more backups than it holds to; a file it cannot read is never a fresh start.
- [x] A shorter audit retention needs a reason, waits 7 days, and any administrator can take it back.
- [x] Only a system administrator takes a restore back.
- [x] The code that confirms a restore is taken during maintenance mode, from a system administrator only.

## How the product owner tests it

1. Log in as an administrator without system administration. Under Backups, lower "Backups to keep": the page says a system administrator must confirm it.
2. Log in as a system administrator with an authenticator app. Lower it again, and enter a code from the app. The page says the backup service keeps the old number until the code from its log is entered.
3. Within a minute, `docker compose logs backup` shows the code. Enter a wrong one: the page says so. Enter the right one: within a minute the page no longer says the old number.
4. Under Rules, Retention, lower the days: the form asks for a reason and a code, and shows when the shorter time holds. Press "Keep the longer time".
5. Under Rules, Access, choose "Nobody" for the second factor: the form asks for a code.

## Upgrade path

- The guard of the backup service also covers a platform without Compose once it runs `diffluence backup next --held` with a file the server cannot reach.
- The console's attention list could name a shorter audit retention that waits; the Retention form shows it today.

## Decided on the product owner's behalf on 2026-11-14, may be overruled

- **D1: the backup service holds its own minimum in a file in the backup folder (option C).** It is the simplest form that holds against a replay of an older confirmed value; the server never reaches that folder.
- **D2: the backup interval is held the same way as the number kept.** Backing up less often thins backups as surely as keeping fewer.
- **D3: the first round holds the more keeping of the console's choice and the service's defaults.** Taking whatever the database says at the first round would let a value written just before an upgrade through. The cost is one code for an installation that kept fewer than its defaults on purpose.
- **D4: a shorter audit retention waits 7 days, and any administrator can take it back.** The wait gives other administrators time to see it on the Retention form before `diffluence audit prune` acts on it; taking it back keeps more, so it needs no more than an administrator. The console study proposed the same.
- **D5: a retention lowered is not confirmed with a code from the backup service's log.** The backup service does not prune the audit log; an operator runs `diffluence audit prune` by hand, so the person at the host already decides when rows go. The wait and the reason take the place of the code.
- **D6: a new sign-up domain and guest invitations turned on count as weaker.** Both let more people in, which is what a weaker rule for logging in does. Self sign-up in the Features section stays an ordinary change, because it admits only the listed domains, which this guard covers.
- **D7: the code entered for fewer backups needs system administration but no second login.** As for a restore: the change before it already asked for a fresh login and a second factor, and the code from the log is the proof.
- **D8: no backup "target" field exists today.** The section has only the interval and the number kept. A field added later, such as where backups go, must be judged in `keepsFewer` and held by the backup service before the console offers it.

## Provenance

Own design, on the mechanism of spec 070. Go standard library documentation for `crypto/hmac`, `crypto/rand` and `os.Rename` (pkg.go.dev, 2026-11-14). PostgreSQL 16 documentation on `jsonb_set` and row locks (postgresql.org, 2026-11-14).
