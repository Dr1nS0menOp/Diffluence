# Beta criteria

Written for: operators who want to know when Kenwerk is worth trying on real work, and contributors who want to know what still stands between today and the first beta.

Kenwerk is **alpha**. A release is called **beta** only when every criterion below holds on the commit that is tagged, and the beta notes link the evidence of each one. This page answers RMP-001 of the [roadmap](blueprint/08-roadmap.md) and its [chapter 1](blueprint/08-roadmap/01-sequencing-to-1-0.md). What 1.0 then promises is in [promises.md](promises.md).

**What beta means here.** Beta is the first release meant for people who did not build Kenwerk, on data they can afford to restore from a backup. It claims what [ADR-043](blueprint/10-decision-log.md) claims and nothing more: **one node, a few hundred people, measured on your own hardware**. The stable parts below should not lose data or show anyone what they may not see; the experimental parts may still change or go. Beta is not 1.0: nothing is frozen yet, and no version is supported.

**Status today** is read from the repository on 2026-11-14, at main `fe000ca`. Each status says one of three things:

- **Met:** the repository shows it, with the evidence named.
- **Not met:** the repository shows that it does not hold yet.
- **Not recorded:** it may hold, but the repository holds no evidence. It counts as not met until the evidence is written down.

## 1. Features

The stable set is everything Kenwerk does that is not listed as experimental: spaces and the page tree, the editor, drafts, versions, compare and restore, attachments and files, search, the trash, comments, notifications and watching, labels and templates, document control, local accounts with a second factor, passkeys, OIDC login and SCIM, roles, groups, guests and restrictions, the audit log and its witness, import and export (Markdown, HTML, PDF, Word, OpenDocument), the admin console, backups, upgrades, and the optional service for editing together.

The experimental set is what the installation switches on part by part (ADR-043, spec 069): `automation`, `databases` (with sheets), `bundles`, `console_restore`, `publishing` (draft and published spaces, spec 073) and `guided_templates` (spec 074), all in `internal/features/experimental.go`, plus public sites and analytics, which keep switches of their own. All start off.

| # | Criterion | How it is checked | Evidence | Today |
|---|---|---|---|---|
| F1 | The feature freeze holds: every part added after ADR-043 is experimental and off on a new installation and after an upgrade | Read each spec since 069; every new part has a name in `internal/features/experimental.go` | Specs 069 to 077; spec 073 D1, spec 074 TPL-1401, spec 075 CSL-1218 | **Met.** Publishing and guided templates came in as experimental parts; spec 075 builds only the show-only wave CS1 before 1.0; 070, 071, 072, 076 and 077 harden what exists |
| F2 | An experimental part that is off answers its routes with 404 `experimental_off`, offers nothing on any screen, and keeps what exists | A test per part | `TestANewInstallationRefusesEveryExperimentalPart` and the proofs in spec 069 §1; PUB-1303 and PUB-1304 in spec 073; TPL-050 in spec 074 | **Met** |
| F3 | An upgrade keeps on what an installation already used | Migration 0054 and its tests | `TestAnUpgrade*`, spec 069 §1 | **Met**, with the limit spec 069 names: tested on prepared data, not on a copy of a real installation |
| F4 | No High line of blueprints 04 to 07 stays open for the stable set or for one node | Search the blueprints for High lines whose status is not *built* | `docs/blueprint/04-architecture/` to `07-operations/` | **Not met.** Open in blueprint 07: OPS-070 (partly: the editing service across two releases), OPS-078, OPS-083, OPS-094 (partly), OPS-109, OPS-110. OPS-084 is left out on purpose (decision 1) |

## 2. Security

Kenwerk has no budget for a penetration test (ADR-029). Until a volunteer or a donation makes one possible, three things stand in for it: the automated checks on every change, the self-test repeated on the candidate, and one outside reader before 1.0 (ADR-043).

| # | Criterion | How it is checked | Evidence | Today |
|---|---|---|---|---|
| S1 | No open security finding rated Medium, High or Critical, from any source: reports under `SECURITY.md`, reviews, self-tests, scanners | The product owner confirms it in the beta notes; GitHub's list of security advisories holds no open draft above Low | Beta notes; advisories once published | **Not met:** an open security item |
| S2 | The self-test of [known limits](operations/limits.md) ("Attacks that held") is run again on the candidate: every route as a stranger, a guest, a member of another space and a reader, and the hostile inputs listed there | Run against the tagged candidate; any new hole blocks the beta | A report in `docs/reports/` naming the commit, written only once nothing it found is open | **Not met.** The last full run was on the build of 6 November 2026 |
| S3 | The automated security checks run on every change: Semgrep, CodeQL, Trivy on the images, a ZAP baseline against the real image, nightly fuzzing of every format reader | `.github/workflows/ci.yml` | Jobs `semgrep`, `codeql`, `container`, `nightly` (ADR-038, SEC-351) | **Met.** That they are green on the candidate is C1 |
| S4 | The threat model and [the security risks](security/risks.md) are current: dated within 30 days of the tag, and with a line for every experimental part | Read both against `internal/features/experimental.go` | `docs/security/threat-model.md`, `docs/security/risks.md` | **Not met.** `risks.md` (13 November) has no line for publishing or guided templates |
| S5 | A public call for one outside reader of permissions, login and restore is open, with the scope written down. The reading itself is a 1.0 criterion (ADR-043) | A link to the call, an issue or discussion on the repository | Beta notes | **Not recorded** |
| S6 | Without a penetration test, the beta says plainly that none was done (ADR-029) | Read `risks.md` and the beta notes | `docs/security/risks.md`, "It has not been independently tested" | **Met** in `risks.md`; the beta notes must repeat it |
| S7 | Branch protection on `main` and on tags `v*`, with the CI gates required, before outside contributions are invited (ADR-029) | The repository settings on GitHub | A line in the beta notes stating the rules in force | **Not recorded** |

## 3. Load

The claim is one node and a few hundred people. The test server of the stress tests is a server from 2012 with spinning disks ([10 November](reports/2026-11-10-stress-test.md), [12 November](reports/2026-11-12-stress-test-2.md)), so a result there is a floor, not a ceiling. Every run uses `scripts/stress/load.js` on the seeded data of those tests (5,000 people, 300 spaces, 60,000 pages), which is harsher than a few hundred people would make.

| # | Criterion | How it is checked | Evidence | Today |
|---|---|---|---|---|
| L1 | At the claim, 50 people active at once (a few hundred people, of whom one in ten is active, rounded up), on the test server with the candidate: reading a page and saving answered busy in under 1% of requests, page p95 at most 300 ms, publish p95 at most 500 ms, and no wrong answer | `SCENARIO=level` at 50 people, 4 minutes | A report in `docs/reports/` and a row in `limits.md` marked with the machine and the commit | **Not met.** On 12 November, at 50 people: 30% busy and page p95 3.0 s. The fixes since (spec 061, spec 064) were measured on a copy of that data in the development container, not on the server |
| L2 | Room for four times the work (ADR-036): at 200 active people nothing is lost or wrong, no save fails, search is turned away before reading, and everything recovers by itself when the load stops | `SCENARIO=level` at 200 people | As L1 | **Not met.** Last run on 10 November, before the changes of waves AC, LD and PF |
| L3 | A soak of 24 hours at the claim (OPS-110): memory of the app flat after the first hour, no job failed, the audit sealer never more than a minute behind, `kenwerk audit verify` passes at the end | `load.js` at 50 people for 24 hours | As L1 | **Not met.** Never run |
| L4 | The hot queries stay within their budgets on the real data set: the feed (recent 150 ms, for you 500 ms, popular 250 ms) and a controlled read at most 5 ms over an ordinary one | `TestTheFeedIsFastOnRealData`, `TestDocumentControlOnRealData`, against the dump, on the candidate | `limits.md`, "Measured on the real data set" | **Met** on 12 and 13 November; to repeat on the candidate |

Several nodes, PgBouncer, a NATS cluster and 5,000 people are **not** beta criteria (decision 1). They become criteria for any later claim of more than one node.

## 4. Upgrades, backup and restore

| # | Criterion | How it is checked | Evidence | Today |
|---|---|---|---|---|
| U1 | Every change passes the upgrade test: the previous build and the new one run side by side on one database, write and read each other's data, and the old one runs again after a rollback | CI job `upgrade`, `scripts/upgrade-test.sh` | [upgrade.md](operations/upgrade.md), "Proof"; OPS-060, OPS-071 | **Met** |
| U2 | Upgrades between real releases: at least two tagged pre-releases before the beta, and `scripts/upgrade-check.sh <previous tag>` passes from each to the next and from the last to the candidate | Run as step 3 of the [release checklist](operations/release.md) | The notes of each release | **Not met.** No release has been tagged |
| U3 | The release is signed with Sigstore and carries SLSA provenance and SBOMs, and a download verifies as `install.md` says (OPS-094) | Step 8 of the release checklist on a real tag | The release on GitHub | **Not met.** Built and run as a dry run; signing and provenance are only proven by a first tagged release |
| U4 | The restore drill runs every night on a fresh installation: page, file, search, login, audit chain and the roles of the database (OPS-054) | CI job `nightly`, `scripts/restore-drill.sh` | `.github/workflows/ci.yml` | **Met** as a gate; that it is green is C1 |
| U5 | The time to restore 10 GB is measured on the real disks of the test server, and written in `backup-restore.md` (part of OPS-078; 100 GB is a 1.0 criterion) | `scripts/restore-check.sh` on a backup of that size | `docs/operations/backup-restore.md` | **Not met.** Only a small set was timed (29 seconds) |
| U6 | A person who did not build Kenwerk installs it on a fresh machine, upgrades it between two tagged releases, makes a backup and restores it, with the guides alone. Every place a guide fell short is fixed before the beta | The person writes down each step and each stumble | A report in `docs/reports/` | **Not met.** The stress test of 10 November was installed by its builders from `install.md`; the three gaps it found are fixed (D5) |

## 5. Documentation

| # | Criterion | How it is checked | Evidence | Today |
|---|---|---|---|---|
| D1 | A guide exists for each task of an operator: install, upgrade, backup and restore, identity providers, monitoring, incidents, limits, version skew, public sites, release | List `docs/operations/` | The files of `docs/operations/` | **Met** |
| D2 | The experimental parts carry the same names in the code, the README, `risks.md`, `upgrade.md` and this page | Compare with `internal/features/experimental.go` | Those files | **Not met.** The README and `risks.md` do not name publishing and guided templates |
| D3 | `version-skew.md` matches the code for every format that crosses versions | Compare each row with the code it names | [version-skew.md](operations/version-skew.md) | **Not met.** It gives bundle formats 1 to 5; the code writes version 7 and reads 1 to 7 |
| D4 | Every entry in `CHANGELOG.md` has an "Upgrade impact" line, and what an operator must do comes first | The release checklist, step 4 | `CHANGELOG.md`; OPS-062 | **Met** |
| D5 | The install guide holds what the stress test found missing: a host name for the public address, PostgreSQL's memory settings, `pg_stat_statements` | Read `install.md` | `docs/operations/install.md` | **Met** |
| D6 | The beta criteria and the [1.0 promises](promises.md) are written and linked from the README (RMP-001, RMP-002) | Read the README | This page | **Met** with this page |
| D7 | On the beta commit, the README, `SECURITY.md` and `risks.md` say "beta", repeat the claim, and say what was not checked (S5, S6) | Read them on the tagged commit | Those files | **Not met.** They say alpha, which is right today |

## 6. CI

| # | Criterion | How it is checked | Evidence | Today |
|---|---|---|---|---|
| C1 | `main` is green for **14 days in a row** before the tag: every run on a push and every nightly run (fuzzing, the full mutation check, the restore drill) passed | The GitHub Actions history of `main` | A link to that history in the beta notes | **Not recorded** |
| C2 | The nightly run holds the **full** mutation check (`make mutation`, without `MUTATION_CHANGED`), and every mutation is caught | `.github/workflows/ci.yml`, job `nightly` | [faster mutations](reports/2026-11-09-faster-mutations.md): the shortcut never replaces the full run in CI | **Met** as a gate; its results are C1 |
| C3 | Two builds of the release are the same byte for byte, and the `.deb` and `.rpm` install (OPS-095) | CI job `release-dry-run` | `.github/workflows/ci.yml` | **Met** as a gate |

## Summary today

Met: F1, F2, F3, S3, S6, L4, U1, U4, D1, D4, D5, D6, C2, C3.
Not met: F4, S1, S2, S4, L1, L2, L3, U2, U3, U5, U6, D2, D3, D7.
Not recorded: S5, S7, C1.

The beta is announced when all of them are met, in the order RMP-003 gives: the stable parts first, then the measurements that need the test server, then the tags.

## Decisions

Each was decided on the product owner's behalf on 2026-11-14, may be overruled.

1. **One node is enough for the beta.** RMP-001 and the exit of phase 3 asked for the HA topology on several machines (OPS-084) and a load test of 5,000 people. ADR-043 later narrowed the claim to one node and a few hundred people, so the beta is measured against that claim. OPS-084 becomes a criterion for any claim of more than one node.
2. **The load numbers.** "A few hundred people" is read as up to 500, with one in ten active: 50 at once (L1), and four times that for the headroom of ADR-036 (L2). The limits of 300 ms and 500 ms are about ten times what reading and saving cost on that server when it was not full.
3. **Medium and above blocks the beta** (S1). The roadmap's exit check stops at High; a beta invites strangers, so a Medium finding waits for its fix first.
4. **The outside reader** is called for at the beta and reads before 1.0 (S5), as ADR-043 places the reading at 1.0.
5. **The stranger** (U6) is a volunteer. If none comes forward within 30 days of the other criteria being met, the product owner does it on a fresh machine with the guides alone, and the beta notes say who did it.
6. **Two tagged pre-releases** before the beta (U2), so the upgrade, the signing and the release checklist are proven on real tags before strangers rely on them.
7. **14 days of green CI** (C1): two weeks of nightly runs is long enough to catch a test that fails now and then, and short enough for one maintainer.
8. **Restore time:** 10 GB before the beta, 100 GB before 1.0 (U5).

## Sources

Read on 2026-11-14.

- Mattermost, [feature labels](https://docs.mattermost.com/administration-guide/manage/feature-labels.html): what *experimental*, *beta* and *generally available* mean, and that beta follows a security review.
- Discourse, [Understanding Discourse release channels](https://meta.discourse.org/t/understanding-discourse-release-channels/264400): the `beta` tag is frozen; releases and extended support releases take its place.
- Gitea, [Upgrade from an old Gitea](https://docs.gitea.com/installation/upgrade-from-gitea): a backup before every upgrade, and no way back without one.
- Nextcloud, [Upgrade manual](https://docs.nextcloud.com/server/latest/admin_manual/maintenance/upgrade.html): major releases cannot be skipped.
- PostgreSQL, [Versioning policy](https://www.postgresql.org/support/versioning/).
- Blueprint 08 chapter 1 and its sources: Semantic Versioning, Fogel's *Producing Open Source Software*, and the research on time-based releases.
