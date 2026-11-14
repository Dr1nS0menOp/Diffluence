# 09.P10: Audit and the release history

How to read this chapter: see [01-draft-and-published-spaces.md](01-draft-and-published-spaces.md).

The question an auditor asks of a published space is: what did readers have on a given date, where did it come from, and who decided it. ISO 9001 7.5.3.2 asks for control of distribution, access, changes (version control) and retention; where the pair holds controlled documents, Part 11 §11.10(e) asks for secure, computer-generated, time-stamped audit trails. Diffluence's audit log is hash-chained, sealed after saving (ADR-041) and witnessed outside the database (ADR-039); this chapter says what the pair writes to it, and how the history of releases is read.

## 10.1 What is audited

- **PUB-900** · new · Every step of this feature writes an audit entry: a pair made (with both spaces), its settings changed (old and new), ended, made again; a run previewed (with its counts, not its lists), started, each batch done, cancelled, stopped (with the reason), finished; each page released (draft page and version, twin and version, the aspects, and whether it was made, changed, moved or placed higher); each page taken off; each page left out and included (with the reason); the experimental switch turned on or off; and each verification of a twin (PUB-732) with its outcome. *Why:* ISO 9001 7.5.3.2; Part 11 §11.10(e) for controlled content. *Proof:* a test per event that it is written with the right fields; the audit chain tests of ADR-041 cover integrity.
- **PUB-901** · new · Entries about the pair are written in the **draft space** with both sides. In the **published space**, the same changes appear as `page.released`, `page.taken_off` and their versions, with the twin's ids only and the releaser as the actor (or "approval" with the approver as cause). *Why:* the published space's administrators audit their space without learning draft names (PUB-108). *Proof:* tests that the published space's audit holds no draft id, key or title.
- **PUB-902** · new · System releases (approval, PUB-711; withdrawal, PUB-720) are recorded with the actor "approval" or "workflow" and the person whose act caused them as `caused_by`, as DC1 records its system steps (DOC-105). *Why:* Part 11 §11.10(e): the trail does not attribute acts to people who did not take them. *Proof:* tests.
- **PUB-903** · new · The release note of each run is in its audit entry. *Proof:* a test.
- **PUB-904** · new · Audit entries of a pair are kept as long as the audit retention of the draft space; where the draft space is under document control, the retention of DOC-1002 applies. *Proof:* a test that `audit prune` keeps them under control.

## 10.2 The release history

- **PUB-910** · new · The release screen has a **history** of runs, newest first: when, who or which approval, the scope, the counts, the note and the report. *Proof:* an e2e picture.
- **PUB-911** · new · A draft page's history shows, beside each version that was released, **"Released as version N of the published page"**, with the date and the run, to those who may see the release screen. *Proof:* a test.
- **PUB-912** · new · **What readers had on a date**: for a page or the whole published space, the release history answers which twin version was current at a chosen moment, and which draft version it came from. It is the twin's own history read at a time, plus the page pairs. *Why:* the auditor's first question. *Proof:* a test with three runs and four dates.
- **PUB-913** · new · An **export of the release history** of a pair for a period, as CSV for people and JSON Lines for tools, with each run, each page, both sides' versions and content hashes, and the audit entries' chain positions. It needs `space.audit` in the draft space; making it is audited. Under document control it is part of the auditor's export (DOC-1020). *Why:* Part 11 §11.10(b), accurate and complete copies. *Proof:* a test that the export of a seeded pair holds every expected item.

## 10.3 Verification

- **PUB-920** · new · Verifying a twin (PUB-732) recomputes the draft version's content hash and compares it with the twin after mapping links back; a space-wide verification runs as background work and reports per page: "the same as draft version N", "differs", or "draft version purged". *Why:* a published copy whose text was changed in the database, or by a bug, must be found. *Proof:* tests for each outcome.
- **PUB-921** · new · The consistency check of PUB-343 also checks that every twin's current version is the result of a recorded release, and reports a twin version without one. *Why:* the managed rule (PUB-220) and the trigger (PUB-224) should make this impossible; the check proves it stayed so. *Proof:* a test that inserts a version with the trigger off and sees the report.
- **PUB-922** · new · The audit entries of releases name the **content hash** of each released draft version and of each twin version, so the audit chain itself binds what was released. *Proof:* a test.
- **PUB-923** · new · No audit entry of this feature holds page text, file content or the names of people other than the actor and the cause. *Why:* the audit log keeps personal data apart so it can be erased (ADR-041). *Proof:* a test over the entries.
