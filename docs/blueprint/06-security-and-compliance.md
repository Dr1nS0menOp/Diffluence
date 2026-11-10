# 06: Security and compliance

A wiki holds what an organisation knows, including what it would rather keep inside. So this blueprint goes through Diffluence surface by surface: what can go wrong, what stops it today, and what is still needed. It then maps that to the standards customers ask about.

This file is the index. The detail is in the chapters below.

This is a design study of our own product. It comes from reading the code on `main` at commit `2970b0f`, the plans for waves C to H, and the [sources](06-security-and-compliance/sources.md). It is not an audit, and nobody outside has tested it. It is not legal advice either: a line marked *Lawyer* must be checked by a lawyer or data protection officer before anyone relies on it.

## How to read it

Each surface starts with **What can go wrong**, followed by numbered lines in the form `SEC-134 · status · text`. A number never changes and is never reused, so specs, tests and commits can point at it.

| Status | Means |
|---|---|
| **built** | In the code today |
| **partly** | Some of it is in the code. The line says what is missing |
| **wave X** | Planned in that wave's spec |
| **phase N** | Planned in that phase of the roadmap |
| **new** | Found by this study and in no plan yet. The line says where it belongs |
| **later** | Left for later on purpose |

Open lines can carry **High**, **Medium** or **Low**. Chapter 1.6 says what these mean. In short, a High line is fixed in the wave it touches, and always before 1.0.

Every ASVS reference is to version 5.0.0, so `ASVS 6.2.1` stands for `v5.0.0-6.2.1`. The aim is Level 2.

## Chapters

| Chapter | Covers | Lines |
|---|---|---|
| [01 Method and assets](06-security-and-compliance/01-method-and-assets.md) | The aim, what we protect, classes of data, attackers, trust boundaries, risk levels | 14 |
| [02 Accounts and login](06-security-and-compliance/02-accounts-and-login.md) | Passwords, authenticator and recovery codes, passkeys, OIDC, SCIM, pushing people to stronger login, sessions, the life of an account | 79 |
| [03 Access, the API and the browser](06-security-and-compliance/03-access-api-and-browser.md) | Access rules, the API, cross-site requests, browser headers and storage, too much of everything | 54 |
| [04 Content](06-security-and-compliance/04-content.md) | The document gate, links, files, previews, imports, exports, public sites | 58 |
| [05 Messages and outward calls](06-security-and-compliance/05-messages-and-outward.md) | Mentions, comments, notifications, mail, the outward client, webhooks, automation, analytics | 38 |
| [06 Optional services](06-security-and-compliance/06-optional-services.md) | Gotenberg, draw.io, office files, editing together, PdfCraft | 42 |
| [07 Admin and operations](06-security-and-compliance/07-admin-and-operations.md) | The console, deployment, backups, upgrades, noticing an attack | 36 |
| [08 Cryptography and secrets](06-security-and-compliance/08-cryptography-and-secrets.md) | Inventory, keys, secrets, encryption at rest and on the way | 22 |
| [09 The audit log and other logs](06-security-and-compliance/09-audit-and-logging.md) | What it holds, whether it can be trusted, what it should not hold, retention, the application log | 30 |
| [10 Privacy under the GDPR (AVG)](06-security-and-compliance/10-privacy.md) | Roles, what is stored about people, retention, rights, breaches, the works council | 27 |
| [11 Supply chain and disclosure](06-security-and-compliance/11-supply-chain-and-disclosure.md) | Dependencies, the build, checks, reporting a hole, the Cyber Resilience Act, customers' rules | 29 |
| [12 How this maps to standards](06-security-and-compliance/12-standards-mapping.md) | OWASP ASVS, OWASP Top 10, NIS2, BIO2 and ISO 27002, GDPR Article 32 | Table |
| [Sources](06-security-and-compliance/sources.md) | What was read, and what could not be read | |

## Where it stands

Counted again in spec 042, against the code and the tests. Lines still marked with the wave that planned them could not be proven line by line; spec 042 lists them.

| Status | Lines |
|---|---|
| built | 332 |
| partly | 32 |
| still marked with a wave | 4 |
| planned in a phase | 5 |
| new | 41 |
| later | 15 |
| **Total** | **429** |

Chapter 12 lists what is still open at ASVS Level 1 and 2.

## What matters most

### High

Every High line of the first study is closed. The last, SEC-546, closed in spec 042: every browser takes out of the shared document what its rules refuse, and a test sends hostile changes through the real service.

### Medium, still open

- **Accounts:** SEC-148 back-channel logout. SEC-110 and SEC-111 closed in spec 046: 100 wrong passwords in a row stop password login, while passkeys and single sign-on keep working.
- **The browser:** SEC-557 checks on `postMessage` for the PDF editor, which does not exist yet.
- **Content:** SEC-548 removed text stays in a browser that had the page open, until logout.
- **Operations:** SEC-655 a failed chain check reaches the console, not yet a notification.
- **Services:** SEC-513 one Gotenberg for PDF export and hostile office files. SEC-532 Collabora's own security settings.
- **The project itself:** SEC-1003 Renovate may not be running. SEC-1004 most actions use moving tags. SEC-1006 there is no SBOM. Since spec 046, Semgrep, CodeQL and a ZAP baseline run in CI.
## Where this differs from the earlier blueprint

The earlier version of this file promised some things that were then built differently, or not yet. Its other facts moved into the chapters.

| The earlier blueprint said | Today | Line |
|---|---|---|
| Content Security Policy with nonces | A stricter policy with `'self'` and no inline script, so no nonces are needed | SEC-250 |
| Attachments from a separate origin | Same origin, locked down with headers, until phase 3 | SEC-331 |
| SQL through sqlc | Hand-written parameterised queries with pgx | SEC-220 |
| A CSRF token on changes | Go's cross-origin protection | SEC-240 |
| OIDC client secrets sealed in the database | In the environment or a file, on purpose | SEC-720 |
| TLS 1.3 with post-quantum key exchange at the edge | TLS ends at the operator's proxy, so that depends on the proxy | SEC-728 |
| Backups encrypted before they leave the host | Not built | SEC-636 |
| Every API call with a token in the audit log | A `token.used` event at most once a minute per token | SEC-807 |

`docs/security/threat-model.md` is shorter than this document and should become a summary that points here. This study did not change it (SEC-007).

## For the product owner to decide

| Line | Question | Advice |
|---|---|---|
| SEC-014 | Does a High finding block a wave? | Yes. That means SEC-134, SEC-135 and SEC-820 go into wave F before it is merged |
| SEC-176 | Keep sessions at 8 hours idle and 7 days, or move towards NIST's 1 hour and 24 hours? | Keep them as the default, write down why, and name the stricter values in the guide for sensitive installs |
| SEC-164 | Must administrators have a second factor on a new instance? | Yes, after a grace period. It follows from your choice to push admins to passkeys and an identity provider |
| SEC-606 | May administrators see titles of restricted pages in the audit screen? | No. Hide them on the screen and in exports, as spec 013 already does in a space's audit |
| SEC-821 | Change the audit format, so personal data can be erased while the chain still holds? | Yes, and early, because every event written before the change stays as it is |
| SEC-831 | How long does the audit log stay by default? | 12 months, as a setting, and longer only with a stated reason |
| SEC-333 | Scan files for malware? ASVS asks for it at Level 2 | Yes, as an optional scanner that runs as a separate program |
| SEC-547 | Editing together means one shared draft per page, which changes the promise of private drafts. Accept that? | Yes, as long as people can clearly see that a draft is shared |
| SEC-1017 | When does branch protection go on? | Before the first outside contribution |
| SEC-1040 | What role does the project have under the Cyber Resilience Act? | Ask a lawyer before a first paid customer. *Lawyer* |
| SEC-1035 | Pay an experienced outside Go reviewer? | Yes, at least once before 1.0, next to the penetration test |
| SEC-648 | How do operators learn about security updates? | A release feed they subscribe to, so there is no call home |
