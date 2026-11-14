# 11.6: A stolen session

How to read this chapter: see [01-console-runs-the-installation.md](01-console-runs-the-installation.md).

Every class C item is judged against one question: **what could someone do who holds an administrator's session in a browser, without the administrator's password, second factor or access to the machine?** The threat model names this "a logged-in user reaching beyond their rights" and "a compromised identity provider account"; a stolen session is the console's version of both. The answer decides the guard, and the guard must keep true what the threat model's table "What each control holds against" says today.

## 6.1 Three tiers

- **CSL-450** · new · **Tier 1: an administrator with a session** (`SessionOnly(Permission(administrator))`), audited. For what is harmless, bounded, or undone by itself: back up now, a more frequent schedule, keeping more backups, the log level for an hour, the disk thresholds within bounds, a restore test, a key check, reading anything in the areas. *Holds against:* a member and a token (`session_only`). A stolen session can make work, within the limits of CSL-408, and cannot remove anything.
- **CSL-451** · new · **Tier 2: a system administrator with a fresh login** (`Sensitive(Permission(system_administrator))`), audited and mailed to every system administrator. For what is reversible but changes how the installation runs: maintenance mode (built), the events switch, the first key steps (add, promote, reseal), raising audit retention, choosing another target that keeps every backup. *Holds against:* a stolen session without the password, and an administrator without system administration (spec 066). The fresh login is the check of `/api/v1/auth/reauth`, a password or a passkey.
- **CSL-452** · new · **Tier 3: a system administrator with a fresh login, a second factor or passkey confirmed for this request, and the code from the worker's log**, mailed to every system administrator when asked, and recorded at each step. For what removes history or a way back: a restore over the live installation (built), retiring a key, running contract migrations before the grace ends, lowering audit retention, lowering the number of backups kept, and moving to a target that drops older backups. *Holds against:* a stolen session even with the password; a compromised app process (it can write the request, but cannot make the code or the seal, CSL-431). Not against someone who also reads the worker's log; that person already holds the host.
- **CSL-453** · new · **The tier is fixed in the code per kind and per direction of a change**, in one table the route, the worker and the tests read. A change of a setting is judged by what it does: raising audit retention is tier 2, lowering it tier 3; raising the number of backups kept is tier 1, lowering it tier 3. *Why:* the harm is in the direction. *Proof:* a table test per kind and direction; a mutation that lowers a tier fails it.
- **CSL-454** · new · **Every tier-2 and tier-3 action is mailed to every system administrator** at the moment it is asked, with who, when, from which address (as the login mail does), and a link to the area, where a tier-3 request can be cancelled until it runs. *Why:* one stolen session faces every other administrator. *Proof:* a test of the queued mails.
- **CSL-455** · new · **A tier-3 request that is cancelled, refused or expired leaves nothing changed**, and its record stays. *Proof:* a test per kind.

## 6.2 Each class C item

| Line | Item | Tier | A stolen session without the tier could | With the guard |
|---|---|---|---|---|
| CSL-460 | Back up now | 1 | Start backups, at most 4 an hour | Load is bounded (CSL-408, spec 058 D5) |
| CSL-461 | Schedule, more often or the same | 1 | Make backups more often, at most one an hour | Bounded |
| CSL-462 | Schedule, less often | 2 | Leave the installation with older backups | Mailed; health warns when the newest backup is older than interval plus two hours |
| CSL-463 | Number kept, higher | 1 | Use more disk | The worker reports free space; the target's room is checked before a backup |
| CSL-464 | Number kept, lower; target that drops backups | 3 | Remove every older backup | Code from the worker's log |
| CSL-465 | Restore test now | 1 | Start one a day | Bounded; it never touches the live database |
| CSL-466 | Restore over the live installation | 3 | Rewind every account and grant | Built (specs 059, 066, 070) |
| CSL-467 | Audit retention, higher | 2 | Keep rows longer, with a reason | Mailed |
| CSL-468 | Audit retention, lower, and the prune | 3 | Erase the record of what it did | Code; takes effect after seven days, cancellable (CSL-535) |
| CSL-469 | Key add, promote, reseal | 2 | Start a key change, costing load | Mailed; reversible until retire; nothing becomes unreadable |
| CSL-470 | Key retire | 3 | Make files sealed only with the old key unreadable | Code; refused while `key check --require-current` finds anything |
| CSL-471 | Contract migrations now | 3 | End the window for a rollback | Code; only when every live node knows them |
| CSL-472 | Log level, disk thresholds, events switch, end sessions | 1, 1, 2, built | Fill logs for an hour; warn earlier or later within bounds; switch transports and back; log everyone out | Bounded and undone by itself; reversible; built with a fresh login |

## 6.3 Against the threat model

The table "What each control holds against" of `docs/security/threat-model.md` keeps every cell it has today. The rows this study adds, written for that table when spec 075 lands:

| Control | A member | An administrator | SQL injection | A compromised app process | The database owner |
|---|---|---|---|---|---|
| Tasks of the worker (tiers 1 and 2) | Holds: administrators only | Holds for tier 2 unless system administrator with a fresh login | Holds | No: it can write a request of tier 1 or 2 | No |
| Tasks of the worker (tier 3) | Holds | Holds unless system administrator, fresh login, second factor and the code | Holds | Holds: the code and the seal come from the backup key | No: it can change what the worker reads |
| Audit retention and prune | Holds | Holds: lowering is tier 3 and waits seven days | Holds | Holds: the prune runs only with the code | No: as today, the witness shows it |
| Key replacement | Holds | Holds: retire is tier 3 | Holds | Partly: it can start a change, not retire a key | No |

None of these rows lowers what an existing row holds. The "Accepted for now" line on Helm, the LXC script and the packages goes away when wave CS6 gives each its worker.
