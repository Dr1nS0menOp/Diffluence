# 02: Clean-room and licensing

> Not legal advice. Have a lawyer check this before a public launch or before accepting outside funding.

## Why clean-room

We want behaviour that users recognise from Confluence (proprietary) and Docmost (AGPL-3.0), but we want to release under **Apache 2.0**. Ideas and functionality are not protected by copyright. Code, text, icons, illustrations and specific visual designs are. Atlassian's customer terms also forbid using their products to develop a competing product, and forbid reverse engineering. So we build only from **public descriptions of behaviour** and our **own design**.

## Allowed sources ("what it should do")

- Public product documentation, help pages, release notes and marketing pages of Confluence, Docmost, Plane and others, **read for behaviour only**
- Using a self-hosted **Docmost** instance as a user. AGPL allows use, and we look at behaviour, not code.
- Publicly documented file formats (for example the Confluence storage format and export structure) for the importer
- Sample Confluence exports **donated by third parties from their own data**, for importer testing
- General knowledge, open standards (OIDC, SCIM, WebDAV, CommonMark), and permissively licensed libraries (MIT, BSD, Apache 2.0, ISC)

## Forbidden

- Reading or copying source code of Docmost, Plane, OpenProject, Huly or any copyleft or proprietary wiki or issue tracker
- Including anything from Docmost's enterprise folders (`ee/`), which are under a separate proprietary licence
- Using an Atlassian product or account to develop Diffluence, including the Confluence of an employer and its exports (one exception, decided by the product owner: see "One exception" below)
- Non-public APIs, decompiling, or capturing network traffic of Atlassian products
- Copying documentation text, UI copy, icons, illustrations, templates or pixel-exact layouts
- Using "Confluence", "Atlassian" or "Jira" in our product name, logo or domain. Descriptive use ("Import from Confluence") is fine.
- Copyleft dependencies (GPL, AGPL, LGPL linked into our code). Calling a GPL tool as a **separate process** (for example Pandoc for exports) is allowed, documented, and optional.

## One exception: pictures of another product's screens (ADR-045)

On 2026-11-14 the product owner decided to use a set of 154 pictures of the screens of a running instance of another wiki product as a source for blueprint 03, and accepted the risk that this goes against the third rule under "Forbidden". The pictures were read for behaviour and for the arrangement of the screens only. These conditions hold for that study and for any later use of the pictures:

- The pictures, the inventories written from them and their file names never enter this repository, its history, its issues or its pull requests.
- Requirements say what a screen does and how it is arranged, in our own words: its regions, the order and grouping of its items, what stays in view and how a state is marked, as roles. Never literal on-screen text, icons, illustrations, colours, sizes, fonts or logos, and never the names of the other product, its vendor or its add-ons.
- No person, organisation, site, page title or figure from the pictures is named anywhere.
- The look stays our own: colours, icons, fonts and every value come from `web/src/design/tokens.css` (ADR-021). The behaviour decisions of 03 ("Where we deliberately differ") and of the decision log stand.
- The study is recorded in `03-functional/sources.md`. Every line it added has the status **new** or **later**, and a line that changes the design standard ends with "(changes the design standard, decided 2026-11-14)".

## The process

1. **Spec sessions.** The product owner and Claude read allowed public sources and write **functional specs in our own words** in `docs/specs/`. Each spec lists the public sources it is based on (a provenance log).
2. **Build sessions.** Claude implements from specs and ADRs only. The session instructions forbid fetching competitor source code.
3. **Checks in CI.** A licence scanner (for example ScanCode) on our code and dependencies, and an allow-list of licences for dependencies.
4. **Honest limitation.** A language model has seen public code during training. We reduce the risk by designing from our own specs, by never asking for "how does X do this", and by the scanning above.

## CLA vs DCO

| | **DCO** (Developer Certificate of Origin) | **CLA** (Contributor License Agreement) |
|---|---|---|
| What it is | The contributor adds `Signed-off-by: Name <email>` to each commit, confirming they have the right to submit the code | A signed legal agreement in which the contributor grants the project owner rights to their code |
| Effort | Very low, one flag: `git commit -s` | A bot plus an e-signature; scares some contributors away |
| What you can do later | Contributions stay Apache 2.0 ("inbound = outbound") | The owner can relicense, for example to sell a commercial edition |
| Used by | Linux kernel and many CNCF projects | Apache Software Foundation, many company-led projects |

**Decision: DCO.** Apache 2.0 already states (section 5) that contributions are submitted under the same licence, and it includes a patent grant. A CLA is only worth it if you want the option to relicense later, and that conflicts with the "open for everyone" goal. Enforce it with a DCO check on every pull request.

## Name

- **Diffluence** (working name): a real English word for a river splitting into branches, and a playful counterpart to Confluence. No software product by that name turned up in a quick search.
- **Before going public:** search EUIPO TMview and USPTO in classes 9 and 42, check the `.org`/`.io`/`.dev` domains and the GitHub organisation name, and design an original logo with no similarity to Atlassian's marks.

## Licence housekeeping

- `LICENSE` (Apache 2.0), a `NOTICE` file, SPDX headers in every source file
- `THIRD_PARTY_LICENSES` generated in CI from the dependency tree
- `CONTRIBUTING.md` explaining the DCO and the clean-room rules
