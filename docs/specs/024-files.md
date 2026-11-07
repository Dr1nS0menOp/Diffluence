# 024: The rest of files

Status: built on branch `wave-n`, waiting for the product owner's acceptance session
Phase: blueprint 03, chapter 6, and ORG-042. Spec 012 planned most of this in wave E and left it open.

## Problem

A file can be added, versioned, moved and listed, but the daily work around it stops short. A large upload shows no progress and cannot be stopped. Nobody can see who added a file or whether the page still uses it. Files cannot carry labels, and the Files screen only filters by name. A space can fill the disk without a limit. The page history is silent about files.

## What the study found

**Old versions cannot really be downloaded.** FIL-037 says they can, and the list of versions shows them, but no route serves one. `GET /attachments/{id}/content` only gives the current version. FIL-042 needs that route, so FIL-037 gets it too.

**Renaming wipes the comment.** `attachments.comment` is stored and never sent, so the form opens empty and saves an empty comment. FIL-039 is not true until the comment comes back.

**The space limit has a column and nothing else.** `spaces.storage_limit_mib` came with migration 0016, and no code reads it.

**`attachment_refs` and `attachment_events` were planned and never made.** DAT-011 and DAT-094 name them. FIL-017 needs to know what no version uses, across pages, because a moved file leaves tiles behind on its old page. Reading every document for that would not hold up, so the derived table is the right answer.

**"No version uses it" is not the same as "nothing uses it".** Four other things hold a file: an open draft, a comment, the header of a page, and an upload in an editor that has not saved yet. Offering any of those for clearing up would break something a person can see.

**Fetch cannot report upload progress.** Only `XMLHttpRequest` can. `api.ts` wraps everything in `Response`, so the rest of the error handling, including a second try after a request for proof, can stay as it is.

**Space administrators do not see everything.** The Files screen goes through `perm_visible_pages`, so a restricted page stays hidden from them. Bulk delete has to hold to the same rule, and an edit restriction counts too.

**A first-page picture needs a renderer.** Images are not tiles. PDF and office files would need a server-side renderer, or PdfCraft running inside every tile. Neither is here, and a new dependency is the product owner's call. FIL-026 stays partly built.

**Highlighting needs nothing new.** The code block already has `lowlight`, and highlight.js knows the file extensions as language aliases (`py`, `yml`, `rs`). Its output is a tree, which turns into React elements with no raw HTML, so the Trusted Types policy is not touched.

**Dropping an archive already does not unpack it (FIL-014).** It becomes a tile like any other file. Only a test was missing.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | `attachment_refs` holds which version of which page shows which file. It is written in the transaction that makes a version, and migration 0024 fills it from all versions. It follows `page_versions` by foreign key, so removing a version removes its rows | DAT-011: a derived table never runs ahead of or behind a version |
| D2 | A file can be cleared up when no version of any page shows it, no draft or comment on its page holds it, no page has it as header, and it is at least a day old | Everything else breaks something visible. The day covers an editor that has not saved yet, as the console does for stored files (`unusedMinAge`) |
| D3 | Clearing up sends files to the trash, one person's rights at a time. Files the person may not delete are left alone and counted as skipped | It stays undoable, and it asks nothing new of the rights |
| D4 | `attachment_events` holds added, new version, removed, restored, moved here and moved away. Purging a file removes its events | Purge means gone. History starts with this release, because the audit log is not for readers |
| D5 | A move away shows the name of the file and never the other page | The reader of this page may not see that page |
| D6 | Anyone who can view the page reads its file history, as they can read the list of files | Same object, same rule |
| D7 | A file label uses the rules of a page label, and the fixed list of the space. `internal/labels` holds those rules, because `attachments` cannot use `pages` | One word, one rule. Changing a label needs `attachment.add`, as renaming does |
| D8 | The label filter of search also finds pages that hold an active file with that label. Trashed files do not count | Search finds pages, and access follows the page, so nothing new can leak |
| D9 | The limit is set by an administrator of the instance, never by a space administrator, under `PUT /admin/spaces/{id}/storage-limit`. Zero means no limit | A limit that the limited person can raise is no limit (FIL-020) |
| D10 | The limit counts what the storage total of the space counts: all versions of active files | The number in the refusal is the number on the screen |
| D11 | The limit is checked before storing, so a full space stops at once, and again in the transaction with the space row locked, so two uploads at once cannot both take the last room | The first check saves a long upload, and the second one is the real one |
| D12 | A limit counts for upload, new version, copy of a file, and a move of a file into the space. It does not count for page copy, page move or import. Those are named in the handoff | Stopping half of a page move or an import is worse than going over, and an administrator sees the total |
| D13 | A new version by drop keeps the name of the tile. The dropped name is still checked against blocked types | Tiles and links stay right, and a blocked file cannot come in under a good name |
| D14 | Only a drop of one file on a tile is a new version. Several files land as new tiles | There is no sensible meaning for several versions at once |
| D15 | One version has its own address, `/attachments/{id}/versions/{number}/content`, always as a download. It checks view on the page on every request | A link is an address and never a right (FIL-042) |
| D16 | Bulk delete takes up to 200 files of one space, needs `space.administer`, and checks delete on each page. Unknown, hidden and refused files all count as skipped | The answer cannot tell hidden from unknown |
| D17 | Filters on the Files screen are offered from what the person can see. The kinds are image, video and sound, PDF, text, office and other | A list of labels must not tell what is on a hidden page |
| D18 | A tile is `small` by default, or `large` as a card. The attribute is `display` | Old documents stay as they are |
| D19 | The preview opens from a tile and from the list of files, and moves through the files in that same list. It shows images and text itself, and hands PDF, spreadsheets and office files to the viewer we have | Previews run no code from the file, and one viewer is enough |
| D20 | Highlighting stops at 200,000 characters, and the text shows plain above that | It runs in the browser of the reader, and must not freeze it |
| D21 | Progress shows for files of 1 MiB and more. Cancelling stops that file and the others go on, like a failed file (FIL-009) | A small file is gone before anyone reads the number |

## User stories

- As a writer, I watch a large upload run, and stop it when I picked the wrong file.
- As a writer, I drop a new copy of a file on its tile, and the tile points at the new version.
- As a writer, I see who added a file, and whether the page still uses it.
- As a writer, I clear up what a discarded draft left behind.
- As a reader, I look into a file without downloading it, and step to the next one.
- As a reader, I keep a link to one version of a file.
- As a space administrator, I find files by type, label and person, and delete many at once.
- As an administrator, I give a space a storage limit.

## Tests first

Failure paths come first:

- A reader without access gets 404 on a version address, on labels, on file history, and on a link to a version of a restricted page.
- A label cannot be changed with view only. A fixed list refuses other labels.
- Search does not find a hidden page by the label of its file, or a page by the label of a trashed file.
- The filters of the Files screen hold no label or person from a hidden page.
- Bulk delete skips hidden, foreign and refused files, and needs `space.administer`.
- Clearing up leaves files held by a draft, a comment, a header, an old version or a young upload.
- A full space refuses an upload, a new version, a copy and a move, and leaves nothing behind.
- A space administrator cannot set a limit.
- A blocked name dropped on a tile is refused.
- History of files never names the page a file went to.

## Not in this wave

- A preview image of the first page (FIL-026): needs a renderer, so a decision by the product owner.
- File labels in bundles and template exports. They stay on the page they were set on.
- Labels on a copied file: a copy starts without them.
- Next and previous inside the PDF viewer: it is its own window.
