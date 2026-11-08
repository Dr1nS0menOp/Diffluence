# 06.1: Method, assets and attackers

How to read this chapter: see `../06-security-and-compliance.md`. Every line has a number and a status: **built**, **partly**, **wave X**, **phase N**, **new** or **later**.

## 1.1 What we aim for

- **SEC-001** · partly · Diffluence aims for OWASP ASVS 5.0.0 Level 2 for the whole product. The standard says most applications should strive for this level. Level 3 items are taken where they are cheap or where a wiki holds company secrets. Chapter 12 shows where each requirement stands today.
- **SEC-002** · built · Deny by default: a route without an access rule cannot be registered, a person without a grant has no access, and a block or attribute that is not on the list does not get through.
- **SEC-003** · built · One rule, several places that cannot drift: the access rule lives in `permissions.Evaluate`, the same rule in SQL for lists, and a matrix test that compares the two with a literal transcription.
- **SEC-004** · built · Checks are the reviewer. The product owner does not read code, so every security rule has a test, and the mutation check proves that a test fails when the rule is broken on purpose.
- **SEC-005** · partly · Every new security rule gets a mutation line. 49 mutations run today. Rules from before that habit are not all covered, so a sweep through this document before 1.0 should add the rest.
- **SEC-006** · new · A security review of each wave before it goes to main, against this document: a fresh session that did not build the wave. `09-way-of-working.md` already names review sessions, and none has run yet. Belongs in the way of working.
- **SEC-007** · partly · This document and `docs/security/threat-model.md` are updated in the same change as what they describe. The threat model is at the level of phases 1 and 2, so it should become a short summary that points here.

## 1.2 What we protect

| Asset | Why it matters | Worst case |
|---|---|---|
| Page content, versions, drafts, comments | It is what a company keeps in a wiki: plans, procedures, sometimes passwords | Read by someone without access, or quietly changed |
| Attachments | The same, often with more detail | The same, and a hostile file running in the app |
| Credentials and sessions | Everything else depends on them | Account takeover, above all of an administrator |
| Permissions and restrictions | They are the promise of the product | A restriction lost without anyone noticing |
| Audit log | It proves what happened | Rewritten after an incident |
| Keys (`master.key`, secrets in the environment) | They open TOTP seeds and sign the audit log | Read together with a copy of the database |
| Personal data | Names, addresses, what people did and when | A breach, or monitoring nobody agreed to |
| The build and release chain | Everything we ship passes through it | A poisoned release reaches every installation |

## 1.3 Classes of data

ASVS 14.1.1 asks for classes with protection rules. These hold for every new table, log line and export.

| Class | Examples | Rules |
|---|---|---|
| **Secret** | Passwords, session and reset tokens, TOTP seeds, recovery codes, API and SCIM tokens, keys, webhook secrets | Never in logs, audit details, URLs that get logged, errors or exports. Stored hashed or sealed. Shown once at most |
| **Content** | Pages, versions, drafts, comments, attachments, titles of restricted pages | Only through the access rule. Never in mail by default, notifications, outbox events or logs |
| **Personal** | Names, e-mail addresses, IP addresses, user agents, login history, who read or did what | Only what a feature needs. Has a retention period. Covered by chapter 10 |
| **Operational** | Ids, counts, timings, versions | May be logged. Ids alone must not reveal content |
| **Public** | Public sites, login page text, the imported font, the instance name | Chosen on purpose, and reviewed before it goes out |

- **SEC-008** · new · Add the class to `docs/compliance/` as a table per data store, so each new column gets a class in review. Belongs in wave F, with the data download and anonymising work.
- **SEC-009** · built · Outbox events and queue messages hold ids, not text, so a message broker never holds Content.
- **SEC-010** · built · Notifications store ids and read titles when they are shown, so old text cannot leak later (spec 010, D10).

## 1.4 Who attacks

| Attacker | What they can do | Most likely goal |
|---|---|---|
| A logged-in member reaching beyond their rights | Calls the API directly, guesses ids, tries every route | Read a restricted page. **The most likely case** |
| A guest | The same with fewer rights | See more of the company than they were invited to |
| An outsider on the internet | Reaches the login page, public routes and public sites | Guess passwords, phish, find a known bug |
| Someone with a stolen session | Everything the victim can do, until it ends | Stay in: add a passkey, turn off a factor, export a space |
| A writer with hostile content | Writes documents, uploads files, imports archives | Run a script in a colleague's browser, or exhaust the server |
| An administrator overstepping | The console, the audit log, the host in small installs | Read content without leaving a trace |
| A compromised identity provider, or a stolen SCIM token | Creates users, moves them into groups | Become administrator |
| Someone with a copy of the database or a backup | Reads everything that is not sealed | Read content, crack passwords, rewrite the audit log |
| A poisoned dependency, image or action | Runs code in our build or in a browser | Reach every installation |

## 1.5 Trust boundaries

```
Browser ──TLS── reverse proxy ──http── diffluence (api, worker)
                                         │  │  │
                     PostgreSQL ─────────┘  │  └───── NATS (embedded, or external in HA)
                     Disk or object store ──┘
Optional, each its own container on an internal network:
  Gotenberg · draw.io · Collabora or LibreOffice · collab service
Outward, only where switched on:
  SMTP · identity provider · webhook targets
```

- **SEC-011** · built · Everything from the browser is untrusted, also from a logged-in administrator: the server decides again.
- **SEC-012** · built · The reverse proxy is trusted only through `DIFFLUENCE_TRUSTED_PROXIES`. Without it, `X-Forwarded-For` is ignored, so an address cannot be faked.
- **SEC-013** · wave E · An optional service is untrusted in both directions: it only gets what the person may see, and what comes back is treated like an upload (chapter 6).

## 1.6 How a risk is weighed

| Level | Meaning | Before |
|---|---|---|
| **High** | Crosses a permission, takes over an account, or breaks a promise of the product | The wave it touches, and always before 1.0 |
| **Medium** | Needs a stolen session, a mistake by an operator, or a second bug | Before 1.0 |
| **Low** | Defence in depth, or a small information leak | When it fits |

The levels are our own judgement, not a CVSS score. Lines that this study found open carry a level in the index.

- **SEC-014** · new · Security findings in a wave that is being built block its commit when they are High. Belongs in the way of working.
