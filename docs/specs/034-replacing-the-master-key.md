# 034: Replacing the master key

Phase: wave Y, after wave W encrypted every stored file under `master.key` (spec 032). Wave X builds the look and settings beside it, so this wave touches no screen.

## Problem

Since spec 032, `master.key` protects every stored file as well as TOTP seeds, webhook secrets and queued account mail. Before this wave there was no way to replace it: a new key made everything unreadable, a lost key lost everything, and nothing could prove that a restored backup opens with its key. The encryption job also walked the whole store from the start on every run, so a store of millions of files might never finish.

## The study

**What the key does today.** `internal/keys` seals each value with its own data key, wrapped by the KEK in `master.key` (ADR-011). The same 32 bytes also derive two other keys through HKDF: the key behind login address fingerprints, and the Ed25519 key that signs audit checkpoints and exchange bundles.

**Where sealed values live.** A search for every `Seal` call found five places:

| Place | Stored in | Context |
|---|---|---|
| TOTP seeds | `totp_credentials.secret_sealed` | `totp:<user id>` |
| Webhook secrets | `webhook_endpoints.secret` and `old_secret` | `webhook-secret` |
| Account mail | `mail_queue.sealed` | `account mail` |
| Stored files | the header of each file | `storage-file:<object key>` |
| Unsubscribe links | not stored: the token is in a mail | `mail-unsubscribe` |

`TestEverySealedColumnIsKnown` fails when code seals in a new place, so this list cannot fall behind.

**Ways to replace a key**, weighed:

1. *Decrypt and encrypt everything at once, with the instance stopped.* Simple, but it needs downtime, and a crash halfway leaves values under two keys with only one key file.
2. *Every node knows both keys during the change, and a job seals everything again.* No downtime; a crash leaves everything readable, because both keys stay until a check proves the old one unused. Chosen.
3. *A KMS that rotates for us.* Not available to a self-hosted install without a new dependency, and ADR-011 keeps it for later.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | A sealed value now starts with format 2 and an 8-byte key id. Values in format 1 still open: every loaded key is tried on them, and GCM refuses a wrong one | A node can pick the right key at once, and a check can count values per key without opening them |
| D2 | The key id is an HMAC of the fixed text `diffluence key id v1` under the key | It can be shown and logged; it gives nothing away about the key |
| D3 | Fingerprints and signing keys come from a **root** that stays when the KEK is replaced. A key file of 32 bytes is both; after the first change the file holds `DFK2`, the root and the new KEK | Login fingerprints, audit checkpoints and bundles keep their meaning. Replacing the root as well is a different job, see "Left" |
| D4 | Three steps, each safe to stop after and to run again: `key add` writes `master.key.next`, `key promote` makes it seal and keeps the old key in `master.key.previous`, `key retire` removes the old key | Every key that sealed anything is in a file at every moment |
| D5 | `key add` and `key promote` refuse while a previous change has not finished | Two changes at once could drop a key that is still in use |
| D6 | `key promote` saves the old key before it changes `master.key`, and writes key files whole: beside the old file, synced, renamed over it | A crash leaves the old or the new file, never half of one |
| D7 | `key reseal` seals every stored value and file again with the current key, beside a running instance. A value is replaced only when it still holds what was read (`... AND column = old`) | A value written meanwhile was sealed with the current key already, and must win |
| D8 | Each run of `key reseal` and of the encryption job stops after a budget, having finished at least one thing, and remembers the last file it finished in `storage_state` | A store of millions of files gets through over many runs; a run with a short budget still moves on |
| D9 | What does not open is skipped and counted, and `key check` names it by table and row, or by file name, never by content | The job must end; the operator must see what is wrong without seeing secrets |
| D10 | `key retire` runs the check first and refuses unless everything opens and uses the current key | Without that, a value of the old key would be lost for good |
| D11 | `key check --deep` reads every byte of every file; without it, the key and the end of each file are checked | A deep check finds damage inside a file, and costs a full read |
| D12 | The backup takes `master.key.next` and `master.key.previous` along when they exist, and `restore.sh` puts them back, then runs `key check` | A backup made halfway through a change must still restore everything |
| D13 | Every step writes an audit event naming the key id only: `keys.added`, `keys.promoted`, `keys.resealed`, `keys.retired` | The audit log says when the key changed, and never holds the key |

## Behaviour

```
diffluence key add                      # writes master.key.next; restart every node
diffluence key promote                  # the new key seals; the old one moves to master.key.previous; restart every node
diffluence key reseal                   # seals every value and file again, in runs of 2 minutes
diffluence key check --require-current  # proves everything opens and uses the new key
diffluence key retire                   # removes master.key.previous, only if the check passes
```

On several nodes, the key files are copied to every node after `add` and after `promote`, and each node is restarted before the next step. A node that has not loaded the new key yet still opens everything that the old key sealed, and nothing is sealed with the new key until every node knows it.

Where `master.key` comes from a read-only mount, such as a Kubernetes secret, the operator places the files: `master.key.next` beside it for step 1, then the new `master.key` and `master.key.previous` for step 2. `key reseal`, `key check` and `key retire` work the same way.

## Permissions and audit

The commands run on the server, with access to the data directory and the database, as the backup does. They need no account in Diffluence. Each step is audited as system actor `cli`, with the key id as object id. No key, value or file content appears in output, logs, errors or audit details.

## Load budget

Class: maintenance, run beside the instance, in the class of search and reports (ARC-233).

- `key check`: one query per 500 sealed values; one file open per stored file, which reads the header and the last chunk, or every chunk with `--deep`.
- `key reseal`: one query per 500 values, plus one `UPDATE` per value that uses an older key; one full read and one full write per file that uses an older key. Measured on one machine through a Windows mount in spec 032: about 226 MB/s encrypted, read and written. A store of 100 GB therefore needs roughly a quarter of an hour of disk time or more, spread over runs of 2 minutes. Measure it on your own store.
- The encryption job: one header read per file not done yet, nothing for files before its cursor.

## Acceptance criteria

- [x] A value naming a key that is not loaded fails with a clear error; a changed key id does not open (keys tests)
- [x] A key file of a wrong length, or a damaged next key, stops the start
- [x] Values from before key ids still open, and count as needing a new seal
- [x] The key can be replaced while every value stays readable; fingerprints and signing keys survive
- [x] Promote run again after a crash finishes the job; a second change waits for the first
- [x] A file is sealed again under the new key and reads the same; a file of a retired key does not open and is left as it was
- [x] A missing key file stops the key tools, which never create a key
- [x] A wrong key is named for every value and file, without showing them
- [x] A file copied under another name is named as unreadable
- [x] A deep check finds damage inside a file that a quick check does not
- [x] The old key cannot be retired before everything is sealed again
- [x] The key is replaced while the instance keeps serving, and the change is audited
- [x] A value changed while it is sealed again keeps the new value
- [x] Resealing skips what does not open and still finishes; the check names it
- [x] The encryption job goes on where the last run stopped
- [x] The restore drill starts a key change, backs up halfway, restores, proves everything opens, and finishes the change

## How the product owner tests it

On a test installation with a few pages, files and an authenticator app set up: run the five commands above in order, restarting Diffluence after `add` and after `promote`. After each step, open a page with a file and log in with the authenticator app. `key check` should report no problems, and the audit log should show the four key events.

## Left

- **Replacing the root as well**, for when `master.key` itself may have been stolen. That needs new signing keys, with old checkpoints still verifiable through the public keys stored beside them, and every login fingerprint worked out again. Today, a stolen key file lets a thief forge checkpoints and bundles even after the KEK is replaced.
- **Reloading keys without a restart.** Nodes load keys when they start.
- **Unsubscribe links** in mails sent before a change stop working once the old key is retired.
- **S3 storage** is not built (spec 032), so resealing works on the disk store only.
- **Listing** still reads the whole directory tree on every run; only the work per file is skipped before the cursor.

## Provenance

Own design, on the Go standard library (`crypto/aes`, `crypto/cipher`, `crypto/hmac`, `crypto/hkdf`). The three-step change with both keys loaded is the usual way to replace a key-encryption key without downtime; no product's text or code was read for it.
