# 09.9: Periodic review, and read and understood

How to read this chapter: see [01-document-control.md](01-document-control.md).

Two duties follow a document after it is approved. First, it must stay right: EU GMP Annex 11 clause 11 asks for periodic evaluation, and quality systems set a next review date on every controlled document so that procedures do not quietly go stale. Second, the people it applies to must know it: auditors ask for evidence that staff read the current version of the procedures they follow. Neither is in today's Diffluence.

## 9.1 Periodic review

- **DOC-800** · new · A state of kind *approved* that is **timed** (DOC-103) sets a **review period** for documents in it, for example 12 months. Each approved version gets a **next review date**: the approval date plus the period. *Why:* Annex 11 clause 11; ISO 9001 7.5.2 c (review for suitability). *Edge cases:* an approved version that is replaced resets the date from the new approval. A page can have its own shorter period, set by those with `page.workflow` or `space.workflows`, never longer than the state's. *Proof:* tests of the dates.
- **DOC-801** · new · The page shows "next review" to everyone who sees it, so readers know how fresh the document is. *Proof:* an e2e picture.
- **DOC-802** · new · The **page owner** is reminded a set time before the date (default 30 days) and on the date. When the date passes, the workflow does what the state says: stay approved and keep reminding, or move to a review state, which starts a round on the current text. Readers keep seeing the approved version during that round. *Why:* a review that finds nothing to change must still be recorded. *Proof:* tests with a fixed clock.
- **DOC-803** · new · A periodic review that finds no change ends with a **confirmation** transition: the reviewers confirm the document is still suitable, optionally with a signature of meaning "reviewed". The version stays the same, the next review date moves on, and the record shows the review. *Why:* an unchanged document must not need a new version to be reviewed. *Proof:* a test that the version number does not change and the date does.
- **DOC-804** · new · A **report** of documents by review state (overdue, due within 30 days, current) is available to those with `space.audit` for a space, and to instance administrators across spaces. *Proof:* a report test.
- **DOC-805** · new · A page owner who leaves (disabled) makes the space's owners the reminder target, and the report marks the page "owner gone". *Proof:* a test.

## 9.2 Read and understood

- **DOC-820** · new · An approved state can ask a **group** to confirm they **read and understood** the new version. Members are notified, and the page shows them a confirmation button with the version. *Why:* evidence of awareness is a common audit request; ISO 9001 7.3 asks that people are aware of relevant documented information. *Edge cases:* the group is resolved when the version is approved, like reviewers (DOC-201); people who join the group later are asked when they join (decided, DOC-1418). *Proof:* tests of resolution and of a late joiner.
- **DOC-821** · new · Confirming is a deliberate act with an audit entry naming the person, the version and the time. It can require a signature of meaning "read and understood" if the state says so. *Proof:* tests.
- **DOC-822** · new · A **new approved version** asks again; earlier confirmations stay on record for their versions. *Proof:* a test.
- **DOC-823** · new · A **list** shows who confirmed which version and who did not, to those with `space.audit`, with reminders to those who have not. It exports with the auditor's export. *Proof:* a test.
- **DOC-824** · new · The confirmation button is only shown when the person has actually opened the version in this session; it is not a guarantee of reading, and the guide says plainly that it records an acknowledgement, not comprehension. *Why:* honest claims. *Proof:* a test that the button appears after opening.
- **DOC-825** · later · Short quizzes before confirming. *Why:* asked for by training teams, but a different feature.
