# Spec 032: Files at rest, derived files, cursors, and other open lines of blueprint 04

Phase: wave W, blueprint 04 outside chapter 8. Wave V builds load protection beside it (ARC-233 to 239, the pool, `/readyz`, the login limiter), so this wave stays out of those.

## The study

| Line | Before | Decision |
|---|---|---|
| ARC-194, SEC-332, SEC-725 | Files were plain on disk. `internal/keys` could seal small values only | Encrypt every stored file, in chunks, with its own key. Build first |
| ARC-195, ARC-093, ARC-196 | An office file was converted on every view | Keep the PDF under `previews/`, named by the hash of the source |
| ARC-133 | The feed and notifications paged by time only | Add the id to the cursor |
| ARC-027 | One known exception | Scan for all of them, record each |
| ARC-071 | What is on reached the frontend through several answers | One route |
| ARC-099, ARC-111 | A changed PDF comes back as a download; two frame modules | Left, see below |
| ARC-007, ARC-218 | Drills of spec 025 on one node | Left, see below |

## Files at rest (ARC-194, SEC-725)

**Layout.** A stored file starts with `DFE1`, the length of the sealed data key, the data key sealed by `master.key` through `keys.Seal` with the context `storage-file:<object key>`, and 8 random bytes. Then come chunks of 64 KiB, each sealed with AES-256-GCM. The nonce is the 8 random bytes and the chunk number. The extra data of each chunk is its number and whether it is the last one.

**Why this way.**
- **Its own key per file:** a key that leaks reveals one file.
- **Bound to the object key:** a file moved under another name does not open.
- **Chunks:** memory stays at one chunk, and a range read decrypts only the chunks it touches.
- **The last-chunk mark:** a file cut at a chunk border does not open. Without it, a cut file would pass as a shorter, valid one.

**Failing clearly.** `Open` unwraps the key and opens the last chunk before it returns. So a wrong or missing key, a file cut short or a changed last chunk fail before a single byte is served, while a handler can still answer with an error. A byte changed elsewhere fails when its chunk is read, and nothing of that chunk is handed out. GCM only releases a chunk that is whole.

**Old files.** Files written before this release are plain. The job `encrypt-stored-files` runs every 10 minutes and rewrites each plain file encrypted under the same name, at most 2 minutes per run, so a large store takes several runs and the worker stays free. The new file replaces the old one in one step, so a reader sees the whole old or the whole new file. Until a whole run finds no plain file, plain files are still read. After that run, `storage_state.encrypted_all_at` is set, an audit event `storage.encrypted_all` is written, and from then on a plain file is refused. So a file swapped for a plain one cannot be served. Other nodes learn it within a minute.

**What is not built, and why it matters.**
- **Rotating `master.key`:** every file key would have to be wrapped again, and no tool does that yet. Today, a new `master.key` makes every file unreadable.
- **Losing `master.key`:** every file is lost, as are TOTP seeds. The backup keeps it encrypted apart from the data already (SEC-636), and the guide says to keep it apart.
- **A delete during the job:** if a file is deleted between the job reading and rewriting it, the rewrite can leave it behind. It is then a file nothing points at, which the care report lists. It is not served, because no row points at it.
- **`List` sizes:** the stored size is a little larger than the file, so the care report shows the stored size.
- **`encryption_key_id`:** stays empty, because the key sits in the file's header. A later contract migration may drop it.

**Measured.** Storing and reading 8 MiB through the store, in the development container on this machine: 414 MB/s plain, 226 MB/s encrypted. That is one machine through a Windows mount, so the ratio matters more than the numbers.

**Load budget.** No query on writing or reading an encrypted file. Reading a plain file costs one query a minute per node, until the job is done. The job is one listing and one read of the first bytes per file.

## Derived files (ARC-093, ARC-195, ARC-196)

The PDF of an office file is kept as `previews/<sha256 of the source>-pdf`. The first view converts it and keeps it; later views and copies of the same content read the kept file. Whoever asks has already been allowed to read the source, and the PDF holds nothing the source does not. The job `purge-previews` runs daily and removes kept PDFs whose hash no version of any file has, in one query for all of them. A preview is made on the first view rather than in a job: the result is the same once per version, without a queue.

**Load budget.** A view of a kept PDF is one file read. The purge is one listing and one query a day.

## Cursors (ARC-133)

The audit log already pages by `before_seq`. The feed and notifications now also take `before_id`, the id of the last item seen, next to `before`, its time. Two items with the same time are then never skipped. `before` alone works as it did; `before_id` without `before` is refused.

**Load budget.** The same query, with one more condition.

## Modules and tables (ARC-027)

A scan of every `INSERT`, `UPDATE` and `DELETE` outside tests found 14 tables written by more than one package:

| Table | Owner | Also written by | Why |
|---|---|---|---|
| `access_requests`, `follows`, `invitation_requests`, `notifications` | sharing, watches, notify | identity | Erasing a person removes their rows in one transaction (`identity/lifecycle.go`) |
| `space_grants` | permissions | identity, exchange | A deleted group loses its grants in the same transaction; an imported bundle restores grants |
| `comments`, `page_labels`, `page_restrictions`, `spaces` | comments, pages, permissions, spaces | exchange | Importing a bundle restores a whole space in one transaction |
| `page_restrictions` | permissions | pages | A move or copy carries restrictions inside the page transaction |
| `pages` | pages | comments, templates | Closing comments sets a column on the page; a template is a page and its delete goes with it |
| `spaces` | spaces | pages | Space settings for approval, fixed labels, free stage and review months live on `spaces` |
| `settings` | settings | analytics, automation | Their own settings keys, written in their own transaction |
| `storage_deletions` | storage | attachments, spaces | Each module that owns files queues them in the transaction that removes them, in one query |
| `trash_items` | trash | pages | A move between spaces carries the trash rows of the moved branch |
| `user_preferences` | identity | notify | The notification settings column |

The ones that must happen in one transaction with the owner's change stay. The small ones (`comments` closing comments, `analytics` and `automation` writing `settings`, `pages` writing space settings) can move behind their owner later.

## One route for what is on (ARC-071)

`GET /api/v1/system/capabilities`, for anyone who may use the product, answers `pdf_export`, `office_preview`, `drawing` and `collaboration` as true or false. It never says where a helper runs. The frontend still reads the answers it read before; switching it is left.

## Left

- **ARC-099:** a changed PDF coming back as a new version needs the viewer to post the PDF back, through the frame module of ARC-111.
- **ARC-111:** one frame module for drawing and the PDF viewer. Both need a browser test of the frames, which spec 025 also did not run.
- **ARC-007 and ARC-218:** killing an API node while people work, and the drills of the large shape, need two or more nodes, a shared file store (S3) and a load balancer. Spec 025 drilled the app, the worker, PostgreSQL and a full disk on one node. On one machine nothing more can be proven honestly.

## Sources

Go standard library `crypto/cipher` (AES-GCM), the existing `internal/keys` (ADR-011), and NIST SP 800-38D for the rule that a GCM nonce must never repeat under one key: each file has its own key, and the nonce counts chunks.
