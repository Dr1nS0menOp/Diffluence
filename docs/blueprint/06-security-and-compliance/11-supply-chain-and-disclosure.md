# 06.11: Supply chain, development and disclosure

## 11.1 Dependencies

**What can go wrong:** a package is poisoned, a known hole stays in, an abandoned library is never updated. OWASP moved supply chain failures up to third place in its Top 10:2025.

- **SEC-1000** · built · Few dependencies, by the product owner's rule, each listed with its reason in `docs/dependencies.md`. Licences are checked in CI.
- **SEC-1001** · built · `go.sum` and `package-lock.json` pin exact versions, and CI installs with `npm ci`.
- **SEC-1002** · built · `govulncheck`, `npm audit` for shipped packages, and Trivy on the image run on every push.
- **SEC-1003** · partly · `renovate.json` asks for weekly updates, and at once for known holes. No Renovate change has ever landed in the history, so it may not be switched on for the repository. Medium. Check it.
- **SEC-1004** · partly · Most actions in `ci.yml` use a moving tag such as `actions/checkout@v5`. Only Trivy is pinned to a commit. Renovate is set up to pin them, and that has not landed, so a poisoned tag would run in CI. Medium. `permissions: contents: read` limits the damage.
- **SEC-1005** · partly · Images use tags, not digests: `golang:1.26`, `node:22-bookworm-slim`, the distroless base, `postgres:16`. Pin by digest, and let Renovate update them. Low.
- **SEC-1006** · new · **Medium.** There is no SBOM. ASVS 15.1.2 asks for an inventory, and customers and the Cyber Resilience Act will too. Generate one per release, for Go, npm and the image. Belongs before the first beta.
- **SEC-1007** · new · ASVS 15.1.1 asks for time limits for fixing vulnerable components. Proposal: critical 7 days, high 30, medium 90, and a yearly check on abandoned packages.
- **SEC-1008** · new · A list of risky components (ASVS 15.1.4), which this document starts in chapter 6: anything that parses hostile files, and anything alpha.
- **SEC-1009** · built · The leaked password list is data from SecLists (MIT), with a generator so it can be rebuilt, and an entry in `docs/dependencies.md`.

## 11.2 Building and releasing

**What can go wrong:** a release that was not built from the code people read.

- **SEC-1015** · partly · Releases are not signed yet, and no provenance is published. The blueprint promises cosign and SLSA. GitHub's build provenance reaches SLSA Build Level 2 with little work. Belongs before the first beta.
- **SEC-1016** · built · The image build uses `-trimpath`, and `scripts/ci-local.sh` fails when a frontend build touches tracked files.
- **SEC-1017** · partly · `main` takes direct pushes in this phase, while `09-way-of-working.md` says pull requests with all checks required. The product owner chose direct pushes per wave, and CI runs after the push, so a bad commit is noticed, not stopped. The product owner decided (ADR-029): branch protection goes on before the first outside contribution.
- **SEC-1018** · partly · Commits have a DCO sign-off, and are not signed with a key, which the blueprint promised.

## 11.3 Checks on the code

- **SEC-1020** · built · `go vet`, staticcheck, ESLint, TypeScript in strict mode, the race detector, integration tests on real PostgreSQL, browser tests with axe.
- **SEC-1021** · built · The permission matrix, the route walker and the mutation check.
- **SEC-1022** · built · Fuzzing covers the Markdown reader and the document gate, nightly. Wave G adds every format reader. The SCIM filter parser has no target yet. Low. Built (spec 042): `FuzzParseFilter`, nightly. A filter on an attribute a route does not read is now refused.
- **SEC-1023** · built · `09-way-of-working.md` lists gates that are not in `ci.yml`: Semgrep, CodeQL and an OWASP ZAP baseline. Either add them or change the list. A check on paper is worse than none. CodeQL and a ZAP baseline are cheap. Medium. Built: the list in `09-way-of-working.md` now names what runs, and what is not in CI yet (spec 042). Since spec 046 all three run in `ci.yml` on every push and pull request, and a finding fails the build: Semgrep with the Go, TypeScript, React and secrets rules, CodeQL with its extended security queries for Go and TypeScript, and a ZAP baseline against the real image. Exceptions sit on the line they excuse, with the reason above them.
- **SEC-1024** · new · The code is written by AI sessions, and read by no person. The checks stand in for review, which covers what they know about. Fresh review sessions and the penetration test must cover the rest, and no review session has run yet: see SEC-006.
- **SEC-1025** · built · A test that scans all logs and audit details from a test run for planted secrets. Low, and cheap.

## 11.4 Reporting and fixing

- **SEC-1030** · built · `SECURITY.md` with private reporting through GitHub, acknowledgement within 3 working days, and critical fixes within 7 days.
- **SEC-1031** · partly · GitHub private reporting is named. Check that it is switched on for the repository.
- **SEC-1032** · new · A `security.txt` (RFC 9116), with `Contact` and an `Expires` less than a year ahead, on the project's site. An instance can serve one too, pointing at its operator, from a setting. Low.
- **SEC-1033** · partly · CVE assignment through GitHub is the plan, and has not been needed. *Pre-alpha* in `SECURITY.md` must change at the first beta.
- **SEC-1034** · phase 3 · An external penetration test before 1.0. Scope: this document, and above all chapters 3, 4 and 6.
- **SEC-1035** · later · One experienced outside Go reviewer at milestones, if a volunteer offers. There is no budget for it (ADR-029).

## 11.5 The Cyber Resilience Act

**This is not legal advice.** The Regulation (EU) 2024/2847 entered into force on 10 December 2024. Reporting of actively exploited vulnerabilities applies from 11 September 2026, and the main obligations from 11 December 2027.

The Act treats free and open-source software differently. A project that is not part of a commercial activity seems to fall outside it. A legal person that supports open-source software meant for commercial use can be an *open-source software steward*, with lighter duties: a public security policy, a contact point, cooperation with authorities, and reporting. A party that sells the product, or support for it, can be a *manufacturer*, with all duties.

So the answer depends on what the product owner does with Diffluence: keep it a free project, start a foundation, or sell hosting or support. Several sources also note that the Commission published guidance on 27 July 2026, which was not read for this study.

- **SEC-1040** · later · The role of the project under the Cyber Resilience Act. There is no budget for a lawyer and no paying customer (ADR-029). The product owner asks for donations. Whether donations change that role is unchecked, and worth a look if money ever comes in.
- **SEC-1041** · partly · Much of what the Act asks is here already: a vulnerability process, a contact point, security updates, a secure default. SBOM (SEC-1006) and documented support periods are missing.
- **SEC-1042** · new · A statement of how long a release gets security updates. Today none is supported before 1.0, and after that only the latest line. Check that against the Act before 1.0.

## 11.6 Rules customers work under

The Dutch Cyberbeveiligingswet, implementing NIS2, has been in force since 15 August 2026. It falls on organisations, not on software, but they will ask suppliers about security. Dutch government bodies work with BIO2, built on ISO/IEC 27001 and 27002. Chapter 12 maps our controls to these.
