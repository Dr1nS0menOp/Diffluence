# 039: The open lines of blueprint 04 that need only code

Status: built on branch `wave-a4`
Phase: 3 (blueprint 04, chapters 2, 4, 7 and 8)

## Problem

Blueprint 04 had lines that were `partly` or planned for phase 3 where the missing part is code, not a server: screens offered helpers that were off or broken, nobody saw a broken helper, a PDF changed in the viewer could only leave as a download, two frames each checked their own messages, nothing in the code said which reads could later go to a standby, and two modules still wrote tables of another module. This wave closes those parts and leaves what needs real servers.

The lines that make pages and counts faster and keep groups and grants in memory (ARC-205, ARC-206, ARC-231) are not in this wave. They work in the permission code and its matrix test, and a wave for blueprint 05 takes them.

## The study

Each line was read against the code as it is on main.

- **ARC-071.** `GET /system/capabilities` existed (spec 032), but screens asked other answers: the file preview read `office_preview` from `/me`, the PDF button its own flag, and the slash menu, the insert dialog and the block browser offered a drawing even with draw.io off.
- **ARC-072.** `/readyz` already ignored helpers. There was no check of a helper at all, so a PDF helper that stopped answering was found by the first person whose export failed.
- **ARC-099.** PdfCraft saves the way most browser apps do: it makes a blob, gives it an address with `URL.createObjectURL`, and clicks a link with a `download` name. Its own JavaScript bindings show exactly those three steps. The frame is sandboxed without its own origin and cannot reach the session, so the bytes have to come to the page, and the page has to store them through the normal upload of a new version (`POST /attachments/{id}/versions`, which checks `attachment.add`).
- **ARC-111.** The drawing frame and the viewer frame each had their own listener and their own checks. Both checked the origin; how they checked the window differed.
- **ARC-228.** No read said whether it could lag. Before a standby can help, the code has to say which reads may go there, and the reads that decide access must be shown never to go.
- **ARC-186.** The line said search is not pluggable and left a move to a standby open. That move is ARC-229. Once a search is marked as a read that may lag, nothing in ARC-186 itself is open.
- **ARC-027.** A scan for writes to another module's table found two small ones still open: `comments` wrote `pages.comments_closed`, and `analytics` and `automation` wrote rows of `settings` directly. The larger ones (erasing a person, importing a bundle, the file deletion queue, space settings written by `pages`) keep their recorded reasons.

## Behaviour

1. **What is on.** Every screen asks one hook for what is on. A drawing, a PDF export, an office preview or a format of the export screen is offered only when its helper is configured and, for the helpers the server can reach, answered at the last check.
2. **A helper that does not answer.** Every minute the worker asks each configured helper if it is alive: the PDF helper at `/health`, the office helper at `/hosting/discovery` (or through the PDF helper when they are one), the editing service at `/livez`. The answer is kept per helper, with a short reason in our own words and never an address. While one does not answer, administrators see a warning in the console and the system screen shows it by name. The wiki stays ready. A helper that is switched off is forgotten, not shown as broken.
3. **A changed PDF.** When someone saves in the viewer, the browser still downloads the file as before. If that person may add files to the page, a line under the viewer says a changed version was saved, with a button to keep it as a new version. Kept, it is stored through the normal upload, with every check of an upload. Bytes that do not start as a PDF are refused with a message, and nothing is stored. Someone who may only read sees no button.
4. **Frames.** Every message from a frame is read only when it came from that frame's window and from the origin expected. A message the page itself posts, or another window posts, is ignored.
5. **Reads that may lag.** Search, reports, exports, analytics and the other heavy reads, the feed, the people list and the recent list are marked as reads that may lag. Sessions and permission facts are always read current, also inside such a read. Writes are never marked. Until a standby exists, this only counts.
6. **Owners write their own tables.** Closing comments on a page is done by `pages`. `analytics` and `automation` store their choices through `settings`.

## Permissions and audit

- Capabilities are readable by anyone signed in, as before. They say what is on, never where it runs.
- The health of helpers is on the system screen, which needs administrator. The attention item `helpers_down` is in the console.
- Keeping a changed PDF is `attachment.add` on the page, checked by the existing upload. The file details now say `may_add_version` per file, from the same check, so the button is not offered to someone the upload would refuse.
- Closing comments still needs `page.edit`, and still writes the audit event and outbox event `page.comments_closed`. The audit details now also hold the title, as every other page event does.
- The health check writes no audit event: it is not a change by a person.

## Load budget

| Line | Load (ARC-238) |
|---|---|
| ARC-071 | One query added to `/system/capabilities` (the helpers that did not answer). The frontend keeps the answer for 60 seconds |
| ARC-072 | Periodic, once a minute: one HTTP request per configured helper (at most three, five seconds each at most), and one transaction with one row per helper and one delete. Nothing on a request |
| ARC-099 | The file details ask one permission check per page among the files asked, not per file. Keeping a version is one upload, the existing saving class |
| ARC-111 | Browser only |
| ARC-228 | No query. One context lookup per query, measured below |
| ARC-186 | Documents |
| ARC-027 | The same queries as before, moved to their owner |

No main screen in `internal/app/query_budget_test.go` changed its budget.

## Measurements

On the development machine, in the test database, one person with one space and twenty pages, the same mix of reads before and after: 50 page views, 20 trees, 10 searches, 10 feeds, 10 notification lists and 5 people lists.

| | Queries | Marked as may lag | Share |
|---|---|---|---|
| Before | 1,675 | 0 (no mark existed) | 0 % |
| After | 1,675 | 170 | 10.1 % |

The mark costs 6.1 ns per query (`FreshnessOf` on a marked context, Go benchmark), against roughly 0.3 ms for an average query in that mix. The share is low because page views and trees are most of the mix and must stay current; in a busy instance search and reports weigh more, and the gauge `diffluence_db_queries_may_lag_total` shows the real share.

## Acceptance criteria

- [x] A helper that does not answer is no longer offered, shows in the console and on the system screen without its address, and `/readyz` stays ready. When it answers again it is offered again; when it is switched off it is forgotten (`TestAHelperThatDoesNotAnswerIsShownAndNoLongerOffered`).
- [x] Only someone who may add files is offered to save a changed PDF; a reader is refused by the upload as well (`TestOnlySomeoneWhoMayAddFilesIsOfferedToSaveAChangedPDF`).
- [x] In the browser, a changed PDF becomes version 2; a message posted by the page is ignored; bytes that are not a PDF are refused; a reader sees no button (`a PDF changed in the viewer is kept as a new version, and only what the frame sends counts`).
- [x] A message from another window or another origin is refused (`frame-messages.test.ts`); only bytes that start as a PDF are kept (`craft-frame.test.ts`).
- [x] Only reads, and only the named lists and heavy reads, may lag (`TestOnlyListsAndHeavyReadsMayLag`); the count separates them (`TestTheCounterSeparatesReadsThatMayLag`); inside a read that may lag, a current mark wins (`TestAReadIsCurrentUnlessMarkedAndCurrentWinsInside`).
- [x] Screens offer only what is on (`capabilities.test.ts`).
- [x] A reader cannot close comments, an editor can, and closed comments refuse new ones (`TestACommentIsCheckedAndStaysBehindItsPage`).
- [x] Each rule above has a mutation in `scripts/mutation-check.sh`, and each is caught.

## How the product owner tests it

1. Start Diffluence with the PDF helper, open the system screen as administrator: the PDF helper is shown as answering.
2. Stop the PDF helper and wait a minute. The console warns that a helper does not answer; the PDF button and the PDF export format are gone. The wiki still works.
3. Start the helper again and wait a minute: the warning goes and the PDF options come back.
4. As an editor, open a PDF on a page in PdfCraft, change something and save it there. The browser downloads it, and under the viewer a line offers "Keep it as a new version". Keep it, and the file shows a second version.
5. As someone who may only read the page, do the same: the download happens, and no button appears.
6. With draw.io off, type `/` in the editor: no drawing is offered.

## Provenance

Own design. The way PdfCraft saves was read from its own shipped bindings in `web/public/vendor/pdfcraft/`. `postMessage` and its `source` and `origin` fields as described by the HTML standard (WHATWG, "Cross-document messaging", read 2026-11-09).
