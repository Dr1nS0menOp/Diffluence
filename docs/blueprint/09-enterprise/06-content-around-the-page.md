# 09.6: Comments, attachments, search, export, public sites, the API and templates

How to read this chapter: see [01-document-control.md](01-document-control.md).

A page is never alone: it has comments, files, a search index, exports, sometimes a public copy, an API, and the templates that make it. Each of these is a door through which a working version could leak to a reader. ISO 9001 7.5.3.2 asks for control of distribution and access, so every door applies the one rule of DOC-300.

## 6.1 Comments

- **DOC-500** · new · Comments belong to a **version**. Readers see the comments made on the approved version and its predecessors that are still current; authors and reviewers also see comments on the working version. *Why:* a comment about a sentence that only exists in a draft would leak the draft. *Edge cases:* when a working version is approved, its comments become visible to readers if they are page comments; inline comments anchored to text that changed stay with the old version, as today. *Proof:* visibility tests for page and inline comments across an approval.
- **DOC-501** · new · **Review comments** (DOC-251) are never shown to readers, also not after approval; they are part of the round's record. *Why:* review discussion is part of the controlled record, not of the published text. *Proof:* a test.
- **DOC-502** · new · Resolving or deleting a comment on a controlled page is audited like today; deleting a review comment is not possible after the round ended. *Why:* Part 11 §11.10(e): the record of a review must not lose its parts. *Proof:* a test.

## 6.2 Attachments

- **DOC-510** · new · An approved version **pins** the versions of the attachments it shows. Readers always get those versions, even after an author uploads a new version of the file. *Why:* an approved procedure with a changed attached form is no longer the approved procedure. *Edge cases:* an attachment shown on the page that is deleted is kept for the approved version until it is no longer pinned by any version that readers or history may see; the storage limit counts it (wave ES). *Proof:* a test that a reader downloads the pinned version after a new upload.
- **DOC-511** · new · Uploading a new version of a pinned attachment is a content change (DOC-310) and restarts review. *Proof:* a test.
- **DOC-512** · new · Attachment previews and the Files screen show readers only pinned versions of controlled pages. *Proof:* tests on the Files screen and on previews.

## 6.3 Search

- **DOC-520** · new · Search indexes a controlled page **twice**: the approved version for readers, and the working version for authors and reviewers. The query chooses per page which entry the person may match. A reader never finds a page by a word that exists only in the working version, and never sees an excerpt from it. *Why:* search is the most used door (40% of use, ADR-037) and the easiest to leak through. *Edge cases:* trigram search on titles and file names (wave ES) follows the same rule; a title changed only in the working version is not found by readers. *Proof:* tests with a word only in the working version, in the body, the title and a file name.
- **DOC-521** · new · A never-approved page has only the working entry, so readers never find it (DOC-301). *Proof:* a test.

## 6.4 Export

- **DOC-530** · new · Every export (PDF, Word, ODT, HTML, Markdown, bundle) of a controlled page or space contains the version the person may see. *Proof:* tests per format as reader and as author.
- **DOC-531** · new · PDF and Word exports of a controlled page print a **control block**: the state, the version, the approval date, and every signature of that version with the printed name, the date and time, and the meaning. *Why:* Part 11 §11.50(b): the signature's name, date and time, and meaning shall be included in any human-readable form of the record. *Edge cases:* an export of the working version by an author prints "not approved" in the block, on every page of the PDF. *Proof:* tests that read the generated PDF text and Word XML for the block.
- **DOC-532** · new · A **bundle** export carries the workflow, the rounds, the answers and the signatures, so that an import elsewhere keeps the record (09.13). *Proof:* a round-trip test.

## 6.5 Public sites

- **DOC-540** · new · A public site publishes only **approved** versions of controlled pages; a never-approved page is not published, and a new approval updates the site at the next publish of the site. *Why:* a public copy is the widest distribution there is. *Proof:* a test that a working version never reaches the site.

## 6.6 The API and webhooks

- **DOC-550** · new · Reading a controlled page through the API returns the version the caller may see, with the state, the version numbers of the approved and working versions (the latter only to those who may see it), and the signatures. *Proof:* API tests per kind of caller.
- **DOC-551** · new · The API can start a round, answer it, take transitions and read the record, with the same checks as the screens. It can **not sign** (decided, DOC-1413). *Why:* a signature needs a person present and a fresh second component (09.8); a token is not a person. *Proof:* a test that a signing transition through a token is refused.
- **DOC-552** · new · Webhook events for controlled pages say "a new version was approved" to subscribers who may only see approved content, and "a working version changed" only to subscriptions of owners with `page.edit`. *Why:* webhooks are a distribution channel too. *Proof:* tests per subscription.

## 6.7 Templates

- **DOC-560** · new · A template may name a workflow. A page made from it enters that workflow in its start state, unless the space has a space workflow, which wins (DOC-420). *Proof:* tests for both cases.
- **DOC-561** · new · A template itself is not a controlled document in this feature; approving templates for others' use belongs to guided templates (spec 048, later). *Why:* scope.

## 6.8 Labels, the feed, analytics and the tree

- **DOC-570** · new · The feed (wave PF) shows readers the event "a new version was approved" and never working changes of controlled pages. *Proof:* a feed test with the real-data benchmark.
- **DOC-571** · new · Page analytics count reads of the version the reader saw, so reports can show who read which approved version (useful for read and understood, 09.9). *Proof:* a test.
- **DOC-572** · new · The tree shows readers a controlled page under its approved title. *Proof:* a test with a title changed only in the working version.
