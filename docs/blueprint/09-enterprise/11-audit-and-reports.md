# 09.11: Audit and reports for auditors

How to read this chapter: see [01-document-control.md](01-document-control.md).

When an auditor visits, they ask for the history of a document: every version, who changed it, who reviewed and approved it, when, with what signatures, and how the procedure itself was controlled. Part 11 §11.10(e) asks for secure, computer-generated, time-stamped audit trails that record the date and time of operator entries and actions, do not obscure earlier information, and are kept as long as the records. Annex 11 clause 9 adds that the reason for a change should be recorded where it matters. Diffluence's audit log is already hash-chained, sealed after saving (ADR-041) and witnessed outside the database (ADR-039); this chapter says what document control adds to it and how an auditor gets it out.

## 11.1 What is audited

- **DOC-1000** · new · Every step of document control writes an audit entry: workflow created, changed (with the version) or archived; workflow assigned to or removed from a space, branch or page; a round started, superseded, passed or ended; each answer and each change of an answer; reviewers added, removed, delegated or replaced by a stand-in; due dates, reminders and escalations; each transition, by a person or the system with its cause; each signature with its hash; each read-and-understood confirmation; periodic review confirmations; and enabling or disabling signing for a person. *Why:* Part 11 §11.10(e); Annex 11 clause 9. *Proof:* a test per event that it is written with the right fields; the audit chain tests of ADR-041 cover integrity.
- **DOC-1001** · new · Transitions that the workflow marks as needing a **reason** (for example "withdraw" or "reject") require a comment, and the comment is in the audit entry. *Why:* Annex 11 clause 9. *Proof:* a test.
- **DOC-1002** · new · Audit entries of document control are kept at least as long as the retention of the controlled space; audit retention cannot prune them earlier, and the retention screen says so. *Why:* Part 11 §11.10(e), "retained for a period at least as long as that required for the subject electronic records". *Proof:* a test that `audit prune` keeps them.

## 11.2 On the page

- **DOC-1010** · new · The page's history gains, for those with `space.audit`, a **control history** view: versions, rounds, answers, signatures and transitions in one time line, with the audit entry of each and its verification mark. *Proof:* an e2e picture.

## 11.3 The auditor's export

- **DOC-1020** · new · An **auditor's export** of one page, a branch or a whole space, for a chosen period, gives: every version with its content hash; the approved and obsolete versions with their dates; every round with reviewers, rule, answers, comments and times; every signature with its record and verification; every transition; the workflow versions that applied; read-and-understood records; and the audit entries of all of these with their chain positions. It comes as a **PDF report** for people and a **machine-readable file** (JSON Lines) for tools. *Why:* Part 11 §11.10(b): accurate and complete copies in human-readable and electronic form. *Edge cases:* a space of thousands of pages runs the export as background work and mails a link when ready; the link needs `space.audit` and expires. *Proof:* a test that the export of a seeded space contains every expected item; a test that the JSON Lines file verifies with the documented procedure (DOC-742).
- **DOC-1021** · new · The export needs `space.audit`, and making one is itself audited, with the scope and the period. *Proof:* a test.

## 11.4 Reports

- **DOC-1030** · new · Reports for a space and across spaces: documents by state; rounds overdue; documents due for periodic review; read-and-understood completion per document; and approvals per person in a period. Each can be exported as CSV. *Proof:* report tests on the real-data benchmark of wave PF for speed (09.14).
- **DOC-1031** · new · Reports show only spaces and pages the person may see with `space.audit`. *Proof:* a matrix test.
