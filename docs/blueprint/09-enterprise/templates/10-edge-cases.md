# 09.T10: Edge cases and failure paths

How to read this chapter: see [01-guided-templates.md](01-guided-templates.md).

A guided template touches many parts at once: the tree, rights, files, labels, workflows and people. Each line here is a way a set could be made wrong, made halfway, or reveal what it should not. Following "failure paths first", each is built and tested before the happy path of its wave.

## 10.1 A missing space or place

- **TPL-800** · built · The **target space is gone** or archived between opening the form and submitting it. The submit is refused with "This space no longer exists, or you cannot add pages to it". The form stays open with every answer, so the person can copy what they wrote. *Why:* nothing typed is lost (TPL-014), and the message does not tell apart "deleted" from "no access" (the design standard's "does not exist, or you cannot see it"). *Proof:* tests for a deleted, an archived and a forbidden space.
- **TPL-801** · built · The **template's own space** is deleted. Its templates go with it, as today (`ON DELETE CASCADE`), and pages made from them keep their origin record with "template deleted" (TPL-612). *Proof:* a test.
- **TPL-802** · built · The **default parent** is gone or out of the creator's reach. The root lands at the top of the space, as `UsableParent` does today, and the check step says so before anything is made. *Proof:* a test.
- **TPL-803** · built · The **chosen parent** is deleted, moved to another space, or restricted against the creator before submit. The submit is refused, naming the parent, and nothing is made. *Proof:* a test for each.
- **TPL-804** · built · A template **of all spaces** is used in a space it is no longer offered in (TPL-502). The submit is refused as "not found". *Proof:* a test.

## 10.2 No rights on part of the set

- **TPL-810** · built · The creator lacks a right that **part** of the set needs:
  - `restriction.manage` for a restricted entry (TPL-130);
  - `page.workflow` for an entry with a workflow (TPL-152);
  - `page.create` under a parent with an edit restriction.

  The whole set is refused **before** anything is made. The message lists every entry that cannot be made and why, so the person can ask for the right once. A set is never made without the parts the person cannot make (decided, TPL-1417). *Why:* a set missing its restricted page, or holding it unrestricted, is wrong in a way nobody would notice. *Proof:* a test per right; a test that the message lists two missing rights at once; a mutation that skips the entry instead of refusing must be caught.
- **TPL-811** · built · The dry run (TPL-341) runs the same checks, so the check step already shows "You cannot make this set: …" before the person presses Create. *Proof:* a test that the dry run and the real run refuse alike.
- **TPL-812** · built · A **guest** may use a template only where they hold `page.create`, which guests rarely have. A guest never sees templates of all spaces unless they are offered in a space the guest may create pages in. *Proof:* matrix cases.

## 10.3 Half-created sets

- **TPL-820** · built · A failure **during** making a set, of any kind, leaves nothing behind. Kinds of failure include a database error, a node dying, the storage limit, a label refused, the audit write, or a timeout. "Nothing behind" covers:
  - no page and no draft;
  - no file copy;
  - no label;
  - no origin row;
  - no audit event, apart from `template.set_failed` for background sets, written after the rollback.

  This holds because it is one transaction (TPL-120). *Proof:* fault-injection tests at each step; a test that reads from a second connection during a slow set and sees none of it.
- **TPL-821** · built · **Stored files** do not take part in the transaction. `CopyAll` writes each copy to storage with `store.Put` while the transaction is open, and a rollback removes the rows but not the stored copy. This is already true of a single page made from a template today. The stored files are only deleted through `storage_deletions`, which is fed when pages are purged. A set copies many more files, so GT1 closes the gap:
  - before copying, the set writes each new object key to a small table of **pending copies**, in its own short transaction;
  - the set's transaction removes those rows when it commits;
  - a periodic task moves pending copies older than one hour that no `attachment_versions` row references into `storage_deletions`.

  *Why:* a failed set must leave nothing behind (TPL-017), in storage as in the database, and the storage limit must not count files nobody can reach. *Proof:* a test that counts stored objects before a set that fails after copying files, and again after the task has run.
- **TPL-822** · built · Two people making sets in the **same space** at the same moment wait for each other on the tree lock. Both sets are complete, in the order they committed. *Proof:* a concurrency test with two connections.
- **TPL-823** · built · A person **double-submits** (TPL-123): one set. *Proof:* see TPL-123.

## 10.4 Large sets

- **TPL-825** · built · The template's editor refuses more than 100 entries, more than 6 levels, or more than 40 fields (TPL-102, TPL-312). Its total text, all entries together, must stay under 5 MiB, and its files under the space's storage room. *Proof:* validator tests at each bound.
- **TPL-826** · built · Sets above 25 pages run as background work (TPL-121). The tree lock of the space is held for the length of that one transaction. Its budget is in TPL-902. A set that runs past 60 seconds is cancelled and rolled back, with `template.set_failed`. *Why:* a lock held longer would stall every other page created or moved in that space. *Proof:* a test with a set made slow on purpose.
- **TPL-827** · built · At most **two** background sets run at once per space, and one per person. Further requests wait in the job queue, and the person sees "waiting". *Why:* a burst of large sets must not take every worker. *Proof:* a test with three requests.

## 10.5 Templates with forbidden content

- **TPL-840** · partly · A template is checked on every save, on import and again at approval (TPL-525). It is refused if it holds:
  - an unknown block, as import does today;
  - a link with a scheme other than `http`, `https` or `mailto`;
  - a `templateValue` that points to an entry that does not exist;
  - a condition that does not validate (TPL-432);
  - a label rule naming a label off the space's fixed list;
  - more than the limits of TPL-825.

  The message names the entry and the place. *Why:* a template is copied into many pages; whatever slips through once slips through every time. *Proof:* a test per rule.
- **TPL-841** · built · The text of a template can hold **mentions** and **page links** written by its author. When a page is made, each mention and link is kept as written. A reader without access to a linked page sees it as any link they cannot follow, never its title (existing rule for page links). A mention of a person who has since left becomes their name as plain text. *Proof:* tests for both.
- **TPL-842** · built · **Answers** can never carry markup: they become text, dates, mentions or page links built by the server from checked values (TPL-212). An answer that looks like a block, a link or a variable stays text. *Proof:* tests with an answer holding JSON of a block, `{space}`, and a `javascript:` address in a line field.
- **TPL-843** · new · Dynamic blocks in a template run with the **reader's** rights in the page made from it, as everywhere. A report block in an onboarding template therefore never shows a new colleague what they may not see. *Proof:* covered by the dynamic-block tests; one test for a page from a set.
- **TPL-844** · new · Whether a template's **words** are acceptable (personal data, secrets, offensive text) is not something software can judge. It is what approval is for (TPL-522). The approval screen reminds the approver of it in one sentence. *Proof:* nothing to test.

## 10.6 Moves, copies, imports and people

- **TPL-850** · built · **Moving** a page of a set keeps its origin record. Moving it to another space keeps the record too, but the set's root link in Details shows only what the reader may see. *Proof:* a test.
- **TPL-851** · built · **Copying** a page of a set does not copy the origin record: the copy was not made from a template. *Proof:* a test.
- **TPL-852** · new · **Exporting** a template (ORG-059) includes all entries, fields, steps, conditions and rules of its approved version, and their files. The audience lists, the state and the history are left out, because people and groups differ between installations. **Importing** makes a template in **draft**, owned by the importer, which follows the approval rule of its scope (TPL-511). *Why:* an imported template has not been checked here. *Edge cases:* an import naming groups that do not exist here keeps the restriction but refuses approval until the groups are chosen again (TPL-525). *Proof:* a round-trip test; a test of the missing group.
- **TPL-853** · new · A **template's owner who leaves** keeps ownership on record. Their drafts stay, visible to the managers of the scope, who can delete or take them over (audited). *Proof:* a test.
- **TPL-854** · new · A **full restore** returns templates, versions, approvals and origin records to the moment of the backup. Background sets that were waiting are lost, because the jobs table is restored with them. The creators see nothing, and the set does not exist. *Proof:* a restore drill step with a waiting set.

## 10.7 Several nodes and concurrency

- **TPL-855** · new · Two approvers decide on the **same version** at the same moment. The version row is locked (`SELECT … FOR UPDATE`), so one decision wins. The other sees "already decided by *person*". *Proof:* a concurrency test.
- **TPL-856** · new · An author saves a new version **while** it is being approved. Whichever commits first wins. An approval of a version that is no longer the newest proposed one is refused. *Proof:* a concurrency test.
- **TPL-857** · built · All times are the database's (`now()`), as in DOC-1240. *Proof:* covered by TPL-201.
