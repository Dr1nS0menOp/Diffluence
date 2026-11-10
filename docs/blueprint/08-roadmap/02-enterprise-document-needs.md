# 08.2: What enterprise teams need first: document control, publishing and templates

ADR-038 puts three enterprise document features before 1.0: document control, draft and published spaces, and guided templates. Blueprint 08 names the products whose *behaviour* is meant; this is a clean-room project, so this chapter does not study those products. It studies what the **standards** that regulated teams must meet ask for, because those requirements are why such teams buy document control at all. Each feature gets its own detailed study before it is built (ADR-024).

## 2.1 What the standards ask

| Source | What it asks of documents | What that means for a wiki |
|---|---|---|
| **ISO 9001:2015, clause 7.5** (quality management) | Documented information is identified, reviewed and approved before use, available where needed, protected, distributed, versioned, retained and disposed of under control; obsolete versions are kept from unintended use | Approval before readers see it; versions; a clear "current" version; withdrawal of old ones; retention |
| **US FDA 21 CFR Part 11** (electronic records and signatures) | Secure, computer-generated, time-stamped audit trails that do not hide earlier values (§11.10(e)); validation of the system (§11.10(a)); a signature shows the printed name, date and time, and its **meaning**, such as review or approval (§11.50); signatures are linked to their record so they cannot be copied to another (§11.70); a signature needs two components, such as user id and password, and both again when the session was not continuous (§11.200) | Signatures with a stated meaning, re-authentication at the moment of signing, the signature visible on the page and in exports, bound to that exact version, and an audit trail of every change |
| **EU GMP Annex 11** (computerised systems in medicine production) | Similar to Part 11: audit trails, electronic signatures with the same effect as handwritten ones, periodic review, and data integrity over the whole retention period | The same, plus a review date on each controlled document |
| **ISO/IEC 27001:2022, Annex A 5.37** | Operating procedures are documented and made available to those who need them | A space of procedures that people can rely on to be current |

Two further needs show up in every regulated setting, because auditors ask about them: **periodic review** (each controlled document has a next review date, and owners are reminded), and **read and understood** (people confirm they read a new version of a procedure, with a record).

## 2.2 Where Diffluence stands

| Need | State | Where |
|---|---|---|
| Approvals with fixed approvers per space, the record of past rounds, page status | Built | PAG-062 to PAG-069 |
| Versions, compare, restore | Built | blueprint 03 |
| Tamper-evident audit log; a witness outside the database was decided in ADR-038 | Built, the witness in progress | ADR-013, ADR-038 |
| Re-authentication for sensitive steps | Built | `ReauthWindow` in `internal/identity/tokens.go` |
| Restrictions on who reads a page | Built | blueprint 05 |
| Public sites on a separate host, copying pages | Built | blueprint 03 |
| Templates with labels, a title pattern and guided pages | Built | ORG-045 to ORG-061 |
| Workflows of own states; readers held on the approved version; signatures with a meaning; review dates; read and understood | Not built | blueprint 08 |
| Two linked spaces, draft and published, with what is out of step | Not built | blueprint 08 |
| A set of pages from one template, conditions, rights per template | Not built | blueprint 08 |

## 2.3 Recommendations

- **RMP-010** · new · **High.** Build **document control first**. It serves the teams with the strictest needs, it builds on approvals that exist, and the two other features reuse its core idea (readers see the approved version). Its study derives each requirement from the table in 2.1, not from any product.
- **RMP-011** · new · **High.** Document control includes, as a minimum: workflows with own states and transitions per space or page; readers see the last approved version while authors and reviewers see the draft; any change to approved content starts a new review; electronic signatures with a meaning (reviewed, approved), re-authentication at the moment of signing, and the name, time and meaning shown on the page and in PDF and Word exports, bound to that version's hash; periodic review dates with reminders; and an export of the full history of a document for an auditor.
- **RMP-012** · new · **Medium.** Read and understood: a controlled page can ask a group to confirm they read the current version; the record is part of the audit log and exportable; a new approved version asks again.
- **RMP-013** · new · **Medium.** Then **draft and published spaces**, built on the same "readers see the approved version" core: a link between two spaces in one instance, what is out of step, publishing one page or all, leaving pages out, and publishing through a workflow.
- **RMP-014** · new · **Medium.** Then **guided templates**: one action that creates a set of linked pages with variables, forms with more field types, content chosen by conditions, rights per template, and administrator approval of templates others may use.
- **RMP-015** · new · **Medium.** State plainly what the software can and cannot claim. A product cannot be "Part 11 compliant" by itself; the customer validates its use. The guide lists which controls the software provides and which remain the customer's (validation, procedures, training), as responsible vendors do.

## Sources

- ISO: [ISO 9001:2015, Quality management systems, Requirements](https://www.iso.org/standard/62085.html), clause 7.5.
- US FDA: [21 CFR Part 11, Electronic Records; Electronic Signatures](https://www.ecfr.gov/current/title-21/chapter-I/subchapter-A/part-11), and [Guidance for Industry, Part 11 Scope and Application](https://www.fda.gov/regulatory-information/search-fda-guidance-documents/part-11-electronic-records-electronic-signatures-scope-and-application), 2003.
- European Commission: [EudraLex Volume 4, Annex 11, Computerised Systems](https://health.ec.europa.eu/system/files/2016-11/annex11_01-2011_en_0.pdf), 2011.
- ISO: [ISO/IEC 27001:2022](https://www.iso.org/standard/27001), Annex A 5.37.
- Diffluence: blueprint 08, section "Enterprise document features", and `03-functional/sources.md`, for which public behaviour pages were read.

All sources of this study are also in [sources.md](sources.md).
