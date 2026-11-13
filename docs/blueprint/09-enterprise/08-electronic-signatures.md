# 09.8: Electronic signatures

How to read this chapter: see [01-document-control.md](01-document-control.md).

An electronic signature in regulated work is a person's legally binding statement that they reviewed, approved or released a specific record. The US FDA's 21 CFR Part 11 and the EU's GMP Annex 11 set what a system must do so that such a signature can stand in for a handwritten one. eIDAS defines electronic, advanced and qualified electronic signatures in the EU; what this chapter builds is an electronic signature in the eIDAS sense, with several properties of an advanced one inside a single organisation's system, and it is not a qualified signature (which needs a qualified trust service provider).

Diffluence cannot be "Part 11 compliant" on its own: compliance belongs to a customer's validated use of a system, with their procedures and training (FDA guidance on scope and application, 2003). What Diffluence can do is provide the technical controls those rules ask of a system, and say plainly which remain the customer's (DOC-790).

## 8.1 Part 11 and Annex 11, point by point

| Clause | What it asks | How Diffluence meets it | Line |
|---|---|---|---|
| Part 11 §11.10(a) | Validation of the system | The customer validates; Diffluence supplies a validation pack: the requirement lines of this chapter, the tests that prove each, and release notes | DOC-790 |
| §11.10(b) | Accurate and complete copies, human-readable and electronic | PDF and Word with the control block; the auditor's export in a machine-readable file | DOC-531, DOC-1010 |
| §11.10(c) | Protection of records for their retention period | Signed versions and their signature records cannot be deleted while retention applies | DOC-770 |
| §11.10(d) | Limiting system access to authorised individuals | The permission rule, roles, and the signing right | DOC-300, DOC-760 |
| §11.10(e) | Secure, computer-generated, time-stamped audit trails that do not obscure earlier values | The hash-chained, sealed and witnessed audit log (ADR-013, ADR-039, ADR-041) | DOC-1000 |
| §11.10(f) | Operational checks enforcing the sequence of steps | The workflow's transitions and conditions | 09.2 |
| §11.10(g) | Authority checks | Who may take each transition; the signing right | DOC-760 |
| §11.10(h) | Device checks | Passkeys bind to authenticators; sessions to their device | DOC-731 |
| §11.10(i) | Training of people | The customer's duty; the guide names it | DOC-790 |
| §11.10(j) | Written policies holding people accountable for their signatures | The customer's duty; Diffluence shows the meaning before signing | DOC-720, DOC-790 |
| §11.10(k) | Control over system documentation | Diffluence's documentation is versioned in its repository | DOC-790 |
| §11.50(a) | The signature shows the printed name, date and time, and meaning | The signature record and its display | DOC-710, DOC-720 |
| §11.50(b) | Those items are in every human-readable form of the record | Page, PDF, Word, the auditor's export | DOC-531, DOC-740 |
| §11.70 | Signatures are linked to their records so they cannot be excised, copied or transferred | The content hash in the signature record, checked on every display | DOC-711, DOC-741 |
| §11.100(a) | Each signature is unique to one person and never reused by or reassigned to anyone else | Signatures belong to an account; accounts are never reused for another person | DOC-750 |
| §11.100(b) | Identity is verified before a signature is set up | The administrator who enables signing for a person confirms their identity, recorded | DOC-751 |
| §11.200(a)(1) | Two distinct components, such as an id and a password; both at the first signing of a session, and at least one at later signings in a continuous session | Decided stricter: a fresh second component at every signing | DOC-730 |
| §11.200(a)(2) | Used only by its genuine owner | Fresh confirmation, rate limits, lockout after failures (wave SX) | DOC-730, DOC-732 |
| §11.200(a)(3) | Use by anyone else would need two or more people to collaborate | Passwords are hashed with Argon2id and never shown; a second factor or passkey is personal | DOC-730 |
| §11.300 | Controls for identification codes and passwords | Existing password, second factor, lockout and session controls | DOC-732 |
| Annex 11 clause 9 | Audit trails for changes to GMP data, with the reason | Every transition and change is audited; a reason is required where the workflow says | DOC-1000 |
| Annex 11 clause 14 | E-signatures have the same impact as handwritten ones within the company, are permanently linked to their record, and include the time and date | Same as Part 11 §11.50 and §11.70 | DOC-711, DOC-720 |

## 8.2 What a signature is in Diffluence

- **DOC-700** · new · A transition can **require a signature**. Taking it then needs the signing step (8.3) and produces a signature record. A workflow can require signatures from all reviewers of a round, or only from the person taking the transition. *Why:* Part 11 and Annex 11 tie signatures to the step that makes a record official. *Proof:* tests that a signing transition cannot complete without a signature.
- **DOC-710** · new · A signature record holds the signer's account id, the **printed name at the moment of signing** (kept even if the person changes their name later), the meaning, the server time (UTC, shown in the reader's time zone), the page, the version, the workflow version, the transition, and the method of confirmation (password and second factor, passkey, or identity provider). *Why:* Part 11 §11.50(a); the method lets an auditor see how identity was confirmed. *Proof:* a test of each field.
- **DOC-711** · new · The record holds the **hash of the signed content**: SHA-256 over a canonical form of the title, the document, and the hashes of the pinned attachment versions. *Why:* Part 11 §11.70; a signature that names a hash cannot be moved to other content without failing verification. *Edge cases:* the canonical form is versioned, so a future change of the document format can still verify old signatures; the format version is stored with the hash. *Proof:* a test that any change to title, body or an attachment changes the hash; a test that verification fails when a record is copied to another version.
- **DOC-712** · new · The signature record and the transition are written in **one transaction**, and the record's audit entry includes the hash. The audit chain then protects the signature as well (ADR-041). *Proof:* a test that a failure in the transition leaves no signature.

## 8.3 The signing step

- **DOC-720** · new · Signing shows, before anything else, the **meaning** in plain words ("You sign that you **approve** version 7 of 'Cleaning procedure B'"), the version, and a link to the exact text. The button carries the meaning ("Sign as approved"). *Why:* Part 11 §11.50 and §11.10(j): a person must know what they put their name to. *Proof:* an e2e test and an accessibility check that the heading and button name the meaning.
- **DOC-721** · new · The meanings are defined in the workflow from a list that a space can extend ("reviewed", "approved", "released", "verified"), each with a description shown when signing. *Proof:* a validator test.
- **DOC-730** · new · Signing needs a **fresh confirmation of identity for that one signature**: the password together with the second factor, or a passkey with user verification. A confirmation counts for exactly one signature and expires after two minutes (decided, DOC-1416). This is stricter than Part 11 asks within a continuous session and separate from `ReauthWindow`. *Why:* Part 11 §11.200(a)(1); stricter because a browser session is hard to prove continuous. *Edge cases:* a person without a second factor cannot sign with a password alone; the dialog tells them to set one up. *Proof:* tests that a reused confirmation, an expired confirmation, a password without a second factor, and a passkey without user verification are all refused.
- **DOC-731** · new · A person who signs in only through the **identity provider** confirms by a passkey registered in Diffluence, or by a fresh login at the provider (`prompt=login` with `max_age=0`); the provider's response must show an authentication time after the signing step began. The method is recorded. *Why:* the provider must prove identity at that moment, not hours ago. *Proof:* a test against the test provider of the IdP suite that an old authentication time is refused.
- **DOC-732** · new · Failed signing confirmations count toward the password-guess limits and the lockout after 100 (wave SX). *Proof:* a test.
- **DOC-733** · new · **No signing** through API tokens, automation, impersonation, support sessions or the setup screen. *Why:* a signature is a person's act. *Proof:* a test per route.

## 8.4 Showing and verifying

- **DOC-740** · new · The page shows the signatures of the version on display: name, meaning, date and time, and a verification mark. The history shows the signatures of each version. *Why:* Part 11 §11.50(b). *Proof:* an e2e picture.
- **DOC-741** · new · **Verification** recomputes the content hash of the version and compares it with the record; it also checks the audit entry of the signature is in the chain. The mark says "valid" or "does not match", with the reason. *Proof:* tests that tamper with a version's content directly in the database and check the mark turns to "does not match".
- **DOC-742** · new · The auditor's export (09.11) includes, for each signature, the record, the hash, the canonical form version, the audit entry and the chain position, so an auditor can verify outside Diffluence with a documented procedure. *Proof:* a test that the documented procedure, run by a script, verifies an export.

## 8.5 Who may sign

- **DOC-750** · new · A signature belongs to one account. Accounts are never renamed to another person; when a person leaves, their account is disabled, not reassigned. *Why:* Part 11 §11.100(a). *Proof:* existing account tests, plus a test that a disabled account's signatures still verify.
- **DOC-751** · new · Signing is **enabled per person** by an administrator who confirms the person's identity, with an audit entry naming the administrator and the method of identification. *Why:* Part 11 §11.100(b). *Proof:* a test that a person without signing enabled cannot sign.
- **DOC-760** · new · Taking a signing transition also needs the right to take that transition (09.3); signing rights do not give review rights. *Proof:* a matrix test.

## 8.6 Keeping signatures

- **DOC-770** · new · Signatures and signed versions **cannot be deleted or edited** by anyone, including administrators. Deleting a controlled page moves it to the trash as today, but its signed versions and their records stay for the retention period of the space, and purging the trash keeps them until it ends. *Why:* Part 11 §11.10(c) and Annex 11 clause 7 on data storage. *Proof:* tests that delete, purge and version removal all keep signed versions.
- **DOC-771** · new · An **erasure request** for a person who signed in a controlled space keeps the signature records, because retention duties win in regulated work (decided, DOC-1417); the privacy page says so, and the person's other data is erased as usual. *Edge cases:* a space that is not controlled has no signatures, so nothing changes there. *Proof:* a test of erasure with and without signatures.
- **DOC-772** · new · Restoring a backup (wave RS) brings back signatures as they were in the backup; signatures made after the backup are lost with everything else, and the restored audit log records the restore (wave RS). The guide warns that a restore of a controlled installation must be part of the customer's procedure. *Proof:* covered by RS's drill plus a signature check after restore.

## 8.7 What remains the customer's

- **DOC-790** · new · The guide `docs/operations/regulated-use.md` lists what Diffluence provides (the lines of this chapter and their tests) and what remains the customer's: validation of their use (§11.10(a)), training (§11.10(i)), written policies (§11.10(j)), the identity check before enabling signing (§11.100(b)), certification to the FDA where required (§11.100(c)), periodic review of the system, and their own backup and restore procedure. Diffluence supplies a **validation pack** per release: the requirement lines, the test names that prove each, and the results of that release's run. *Why:* RMP-015; responsible vendors say what they do and do not cover. *Proof:* a check that every DOC line in this chapter is named in the pack.
- **DOC-791** · later · Qualified electronic signatures under eIDAS through an outside trust service provider. *Why:* needs a provider and a contract, which only the product owner can decide.
