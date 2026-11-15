# Regulated use: electronic records and signatures

This guide is for an organisation that uses Diffluence under 21 CFR Part 11, EU GMP Annex 11 or a quality system such as ISO 9001. It says what Diffluence provides, how an auditor verifies a signature outside Diffluence, and what remains the organisation's own work (RMP-015, DOC-790). The requirements behind it are blueprint chapter 09.8; spec 064 holds the decisions.

Diffluence is not "Part 11 compliant" on its own. Compliance belongs to a validated use of a system, with procedures and training. Diffluence supplies the technical controls those rules ask of a system, and the evidence that they work.

## What Diffluence provides

| Clause | What Diffluence does |
|---|---|
| Part 11 §11.10(b), Annex 11 clause 8 | Every export holds the version a person reads, with a control line that names its state, version, approval and signatures. The auditor's export (below) is the machine-readable copy. |
| §11.10(c) | Signatures, the versions they sign and the files those versions show cannot be deleted or changed, by administrators either. A signed page goes to the trash as before, but is never purged; pruning the audit log stops where the records of document control begin. |
| §11.10(d), (g) | Each step of a workflow names who may take it. Signing is enabled per person, never through a role. |
| §11.10(e), Annex 11 clause 9 | Every step, answer and signature is in the hash-chained, sealed audit log, which a witness can check outside the installation. |
| §11.10(f) | Workflows enforce their sequence of states; nobody steps around the reviewers. |
| §11.50, Annex 11 clause 14 | A signature shows the printed name of the moment of signing, the date and time, and its meaning, on the page, in its history and in every export. |
| §11.70 | A signature is bound to a SHA-256 hash of the exact content it signs: the title, the document and the files the version shows. A record moved, copied or edited shows "does not match". |
| §11.100(a) | A signature belongs to one account. Accounts are disabled when people leave, never given to someone else. |
| §11.200(a) | Each signature needs a fresh confirmation made for it alone, which counts once and for two minutes: the password with a code of an authenticator app, a passkey with user verification, or a new login at the identity provider. Wrong passwords and codes count toward the stop after 100 wrong guesses. Tokens, automation rules and the system can never sign. |

## What remains yours

- **Validation** of your use of Diffluence (§11.10(a)). Each release comes with its validation pack, `docs/operations/validation-pack.md`, which names the test that proves each requirement; the results are the run of that release.
- **Training** of the people who sign (§11.10(i)).
- **Written policies** that hold people accountable for what they sign (§11.10(j)). The signing dialog shows the meaning before anything else, and your policy says what each meaning commits to.
- **Confirming who a person is** before an administrator enables signing for them (§11.100(b)). Diffluence records who enabled it and how the person was identified, as a choice of four ways; it never stores the number of an identity document.
- **Certification to the FDA** that your electronic signatures are the legally binding equivalent of handwritten ones, where you need it (§11.100(c)).
- **Periodic review** of the system and of who may sign.
- **Backups and restores** as part of your procedure. A restore brings back the signatures of the backup and loses those made after it; run `diffluence signatures verify` after every restore.
- **Retention periods.** Diffluence keeps signed records without end. A space that must let them go after a period needs a decision of the product owner first.

## Review dates, reading and absence

- **Periodic review** (Annex 11 clause 11). A timed approved state gives each approved version a next review date, shown to everyone who sees the page. The page owner is reminded before it; on the date the workflow starts a review of the same text, and passing it confirms the version without a new one.
- **Read and understood.** An approved state can ask groups to confirm that they read each new version. The button appears only after the person opened that version, in the same session. It records an acknowledgement: that the person says they read it. It is not a test of understanding, and Diffluence makes no claim beyond that.
- **Due dates** count working days of the working week set under **Administration, Settings, General**, in its time zone. Holidays are not known: set a longer period around them.
- **Delegation and absence.** A reviewer can hand a review only to someone the workflow names for it in the same way, and someone who looks after the workflows can choose another person who may review. A stand-in takes a review only when the workflow names them too. Nobody signs for someone else.
- **Bundles from another installation** bring their signatures as records of that installation. They verify against the content here, but they are not signatures of this installation, and they approve nothing here.

## Who may sign

1. Set up an authenticator app or a passkey under **Your account**. A password alone never signs.
2. An administrator opens the person under **Administration, People**, confirms who they are, chooses how, and enables signing. An administrator cannot enable their own signing.
3. A space adds its own meanings of signatures beside the four every space has (reviewed, approved, released, verified), in the workflow settings of the space. A meaning is never removed or changed.
4. A step of a workflow requires a signature by naming a meaning. On an author's step, its author signs; on the step after an approved review, every reviewer who approves signs. A signed approval cannot be changed into a request for changes.

## Checking signatures

- **On the page:** the band names each signature of the version on display, with its mark. The mark compares the record with the hash its version keeps.
- **In the history:** each version lists its signatures, and **Verify signatures** recomputes the hash from the content and checks the audit entry and its place in the chain.
- **For the whole installation:** `diffluence signatures verify` checks every signature and exits 1 when one does not match.
- **Control history:** whoever reads the audit of a space sees, under the history of a page, every audit entry about it with its place in the chain, and the verification of each signature.

## The auditor's export

`GET /api/v1/spaces/{id}/auditor-export` gives the record of a whole space, or of one page (`page=`) and its branch (`branch=true`), for a period (`since=`, `until=`, as `2026-01-31T00:00:00Z`). It needs the right to read the audit of the space, and making one is itself audited. It holds at most 1000 pages; choose a branch or a period for more.

The file is JSON Lines: one object per line, each with a `type`:

| Type | What it holds |
|---|---|
| `export` | The first line: format 1, the canonical forms it uses, the installation, scope and period |
| `page` | Each page: its state, approved version, approver and workflow version. A page the exporter may not read has a censored title and no content |
| `version` | Each version of the period with its content hash; a signed version carries its `content` |
| `round`, `answer`, `reviewer`, `review_comment` | The rounds of review and everything said in them |
| `signature` | Each signature: its whole `record` and its `verification` |
| `workflow_version` | Each version of a workflow the pages followed or were signed under |
| `audit_entry` | Each audit entry about the pages: `seq`, `payload`, `prev_hash`, `hash` |
| `end` | The last line, with the number of lines before it |

## Verifying a signature outside Diffluence

`scripts/verify-signatures.mjs` follows these steps with Node.js and nothing else: `node scripts/verify-signatures.mjs auditor-export.jsonl`. They can also be followed by hand.

1. **Rebuild the canonical form of the signed version (form 1)** from the `content` of its `version` line. It is UTF-8 bytes: the line `diffluence-signed-content`, a newline, the line `1`, a newline, then a sequence of fields. Each field is its name, a space, the length of its value in bytes, a newline, the value, and a newline. The fields are, in order: `title`; `schema` (the document format version, as a number in decimal); `document` (the document as canonical JSON, exactly as the export gives it); `files` (how many files, in decimal); then for each file, sorted by its id as text: `file` (the id), `version`, `name`, `sha256` (the hash of that file version's bytes, in lower-case hex).
2. **Hash it** with SHA-256. The lower-case hex must equal the version's `content_hash` and the record's `content_hash`. A record that names another hash was moved, copied or edited; content that gives another hash was changed.
3. **Find the audit entry** whose `id` is the record's `audit_event_id`. Its `payload` is JSON with `action` = `page.signed`, `object_id` = the page, `actor_id` = the signer, and `details` naming the signature id, version, content hash, canonical form and meaning of the record.
4. **Check its place in the chain:** SHA-256 over the bytes of `prev_hash` (from hex) followed by the UTF-8 bytes of `payload` must give its `hash`, and the entry with the next `seq` in the export must name that hash as its `prev_hash`.
5. **Tie the chain to the installation:** compare the hashes with the signed checkpoints of the audit witness (spec 070), which are kept outside the installation. That shows the chain in the export is the chain the installation wrote.

An audit entry that waits to be sealed has no `seq` yet; export again a moment later.
