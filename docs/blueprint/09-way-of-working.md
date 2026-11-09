# 09: Way of working

## Roles

| Who | Role |
|---|---|
| **The product owner** | Product owner (decides *what* and *why*), tester and operator. Reviews **behaviour**, not code. |
| **Claude (build sessions)** | Developer: writes code, tests and documentation from specs and ADRs |
| **Claude (review sessions)** | A fresh session that has **not** seen the build. Reviews a pull request or module against the specs, the security rules and this document. |
| Volunteers (welcome, never assumed) | A penetration tester, or an experienced Go developer who reviews at milestones. There is no budget for paid help (ADR-029). |

Because the product owner does not read code, **the automated checks are the code reviewer**. They must be stricter than in a normal team.

## GitHub setup

- `main` is protected: changes only via pull request, every check must pass, no force-push
- Signed commits, DCO sign-off, CODEOWNERS
- Renovate for dependency updates, grouped weekly
- GitHub secret scanning, push protection, CodeQL, private vulnerability reporting
- Issues with templates: feature (links to the spec), bug (steps to reproduce), security (private)

## Quality gates (every pull request)

| Gate | Tools |
|---|---|
| Formatting and lint | `gofmt`, `golangci-lint` (incl. `gosec`, `staticcheck`), ESLint, Prettier, TypeScript strict mode |
| Unit tests | `go test -race`, Vitest. Coverage must not go down. |
| Integration tests | Real PostgreSQL and NATS in containers (Testcontainers) |
| Permission matrix | Generated tests from 05. Must be 100% green. |
| End-to-end | Playwright on the built app, including accessibility checks (axe) |
| Design | `docs/design/README.md`: tokens, contrast, icons and dialogs through `npm run design`; approved pictures of every screen in the end-to-end run |
| Upgrade test | Start the previous release with traffic, upgrade to this build, no failed requests allowed |
| Security | In CI today: `govulncheck`, Trivy (image and dependencies), `npm audit` of shipped packages, licence scan, mutation check (nightly). Not in CI yet, each would add a new tool: Semgrep, CodeQL, OWASP ZAP baseline (SEC-1023) |
| Fuzzing | Go native fuzzing on parsers (Markdown, imports, sanitiser), nightly |
| Load | k6 smoke test per pull request, full 5,000-user test nightly |
| API contract | The OpenAPI spec matches the code, and every endpoint has a permission annotation |

## Every phase starts with a study (product owner, 2026-11-05)

The product owner: this is one of the most important ways to make the product as complete as possible. No phase or wave is built from a short list. Before the first line of code:

1. **Read widely.** Public help pages of other products, for behaviour only (`02`). W3C, MDN and library documentation for how it should work. Not one source: several, and in detail.
2. **Write what was found.** What people expect, what accessibility demands, what the libraries allow, what is risky.
3. **Plan every feature in detail** in `docs/specs/`: behaviour, mouse, keyboard, accessibility, document format, export, search, edge cases and limits.
4. **Decide and write it down:** a table of decisions with reasons, what is in and what waits.
5. **Plan the build:** an order of work, a test plan per level, acceptance criteria, risks, and how the product owner tests it.
6. **Log the sources** in the spec, and update `03-functional` when the study finds something new.

Only then does building start. If a build has started without this, stop and do the study first. Spec 008 is the example to follow.

## How a build session works

1. **One issue, one branch, one pull request.** Keep it small: one feature or fix.
2. At the start Claude reads `CLAUDE.md`, the relevant spec and ADRs, and the latest handoff note.
3. Claude writes the tests **first** from the spec's acceptance criteria, then the code.
4. Claude runs the full check suite locally before opening the pull request.
5. The pull request description contains a **"How to test" checklist in plain language** for the product owner, for example "Log in as the guest user → open space X → you should NOT see page Y".
6. At the end Claude updates the changelog, the relevant docs, any new ADR, and writes a **handoff note** (`docs/handoff/`) so the next session knows where things stand.

## The product owner's review sessions ("as a colleague")

- **When:** at least once per phase milestone, and for every pull request touching permissions, authentication, encryption or upgrades
- **Where:** the staging environment, which is redeployed automatically from the pull request or `main`
- **How:** Claude prepares a script of scenarios. The product owner performs them and reports what they see. Claude fixes the deviations, and each deviation becomes a new automated test.
- **The product owner's fixed checks per milestone:** upgrade while a colleague is editing, kill a node, restore from backup, try to reach a restricted page as a guest

## Definition of done

The feature matches its spec and acceptance criteria, all gates are green, permissions and audit events are covered, the UI text goes through translation files, the screen follows the design standard, the docs are updated, the upgrade test passes, and the product owner has ticked the "How to test" checklist. A change that makes the document gate stricter also says what happens to stored versions: a pass over them, or the same check when they are read, because stored documents are trusted as they are (SEC-308).

## Working within Claude Max limits

- Plan work in **small, independent issues**, so a session can always finish something
- Use parallel cloud sessions only for tasks that don't touch the same files
- Spec sessions and review sessions are cheap. Big refactors are expensive and are avoided by deciding early (ADRs).
- When a limit is reached, the handoff note makes it safe to stop at any time

## Documentation language

English, because the project is public. These blueprint documents are the start of `docs/`.
