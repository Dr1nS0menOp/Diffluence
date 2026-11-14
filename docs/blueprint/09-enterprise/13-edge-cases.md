# 09.13: Edge cases and failure paths

How to read this chapter: see [01-document-control.md](01-document-control.md).

Document control is only as strong as its weakest path. Each line here is a situation where a page could slip out of control, a record could be lost, or a reader could see what they should not. Each one is built and tested before the happy path of its wave, following the rule "failure paths first".

## 13.1 Moving, copying and deleting

- **DOC-1200** · new · **Moving** a controlled page to another space takes the target's rule. If the target has a space workflow, the page enters its start state there, with its approved version kept as approved only if the target's workflow is the same workflow (it never is across spaces, DOC-430), so in practice the page needs a new approval in the target. Readers in the target see nothing until then, and the move dialog says so before the move. *Why:* approval under one space's procedure is not approval under another's. *Edge cases:* moving a branch moves each page this way; moving into a space without control makes ordinary pages at their approved versions. *Proof:* tests for each target kind.
- **DOC-1201** · new · **Moving within** a space keeps the workflow, state and record, unless the page leaves or enters a branch workflow (DOC-410). *Proof:* a test.
- **DOC-1202** · new · **Copying** a controlled page makes a new page without approvals, rounds or signatures. In a controlled target it enters the start state; elsewhere it is an ordinary page. The copy holds the content the copier may see. *Why:* Part 11 §11.70: signatures must not be copied with content. *Proof:* tests that no signature or approval is copied, and that a reader's copy holds the approved text.
- **DOC-1203** · new · **Deleting** a controlled page moves it to the trash; restoring it from the trash returns it with its state and record. Purging keeps signed versions and their records for the space's retention (DOC-770). *Proof:* tests.
- **DOC-1204** · new · **Archiving** a controlled page keeps its record and hides it from readers like any archived page; unarchiving returns it in the state it had. *Proof:* a test.

## 13.2 Restores and imports

- **DOC-1210** · new · A **version restore** is a change and goes through review (DOC-312). *Proof:* as in DOC-312.
- **DOC-1211** · new · A **full restore** of the installation (wave RS) returns every workflow, round, answer and signature to the moment of the backup. Rounds that were open at the backup are open again, with their due dates in the past, and escalate at once by DOC-231. The restore is in the restored audit log (wave RS). *Proof:* a step in the restore drill that restores a backup with an open round and checks it escalates, and that signatures verify after the restore.
- **DOC-1212** · new · **Importing** a bundle (spec 018 and later) that carries document control creates the workflows in the target space as new workflows, and the pages' records as **imported records**, marked with the source instance and time. Imported signatures keep their records and verify against their content hash, and are shown as "signed in another installation". They do not count as approvals in the target until a round there approves (decided, DOC-1421). *Why:* trust is local to an installation's audit chain. *Proof:* a round-trip test that imported signatures verify and do not approve.
- **DOC-1213** · new · Importing Markdown or other formats into a controlled space creates working versions that need approval. *Proof:* a test.

## 13.3 People and groups

- **DOC-1220** · new · A **reviewer who leaves** (disabled) is dropped from open rounds with an audit entry; the rule is recalculated, and the round escalates if it can no longer pass (DOC-242). *Proof:* a test.
- **DOC-1221** · built · A **group that changes** while a round runs does not change the round (DOC-201); a group deleted while a round runs leaves the resolved reviewers in place. *Proof:* tests.
- **DOC-1222** · built · A **role** removed from the space leaves running rounds as they are; new rounds resolve the role to nobody and are refused (DOC-200). *Proof:* a test.
- **DOC-1223** · new · A **page owner** who leaves makes the space owners the target of review reminders (DOC-805). *Proof:* a test.
- **DOC-1224** · built · A **guest** can never be a reviewer or signer (decided, DOC-1422), because guests are outside the organisation whose procedure it is. *Proof:* a test that a guest cannot be added and cannot answer.
- **DOC-1225** · new · A person renamed after signing keeps the printed name of the moment in the signature record (DOC-710); the page shows that name. *Proof:* a test.

## 13.4 Several nodes and concurrency

- **DOC-1230** · built · Two people answering the last two answers of a round at the same moment on two nodes produce exactly one transition: the round row is locked (`SELECT … FOR UPDATE`) while answering. *Proof:* a concurrency test with two goroutines and two connections.
- **DOC-1231** · built · A publish and a final approval at the same moment: whichever commits first wins, and the other sees the new state (the approval of a superseded text is refused, DOC-330). *Proof:* a concurrency test.
- **DOC-1232** · new · The due-date job (DOC-230) claims due rounds with `FOR UPDATE SKIP LOCKED`, so on several workers each reminder and escalation happens once. *Proof:* a test with two workers.
- **DOC-1233** · built · The visibility row (`page_control`, DOC-1300) is updated in the same transaction as the transition, so no node can serve a reader a version that is not approved, even for a moment. *Proof:* a test that reads from another connection during a transition.

## 13.5 Time

- **DOC-1240** · new · All times are taken from the database server (`now()` in the transaction), not from the application node, so nodes with drifting clocks agree on order and due dates. *Why:* Part 11 §11.10(e) asks for time-stamped trails; one clock source avoids contradictions. *Proof:* a test that a node with a skewed clock writes the database time.
- **DOC-1241** · new · A clock change on the database server (daylight saving is not one, since times are stored in UTC) that moves time backwards does not repeat reminders, because each reminder records that it was sent. *Proof:* a test.
- **DOC-1242** · new · Due periods in working days use the instance's time zone and working week; a change of those settings applies to new rounds only. *Proof:* a test.

## 13.6 Failures

- **DOC-1250** · built · A failure while taking a transition (database error, a node dying) leaves the page in its previous state, with no partial answer, signature or visibility change, because it is one transaction. *Proof:* fault-injection tests.
- **DOC-1251** · new · A failure while assigning a workflow to a large space as background work leaves each page either fully assigned or untouched, and the job resumes where it stopped. *Proof:* a test that kills the job halfway and resumes.
- **DOC-1252** · new · A notification that fails to send is retried by the existing outbox and never blocks a transition. *Proof:* a test with a failing mail sender.
- **DOC-1253** · new · Search indexing of a newly approved version runs after the transition; until it is done, readers find the previous approved version, never the working one. *Proof:* a test that delays indexing.
