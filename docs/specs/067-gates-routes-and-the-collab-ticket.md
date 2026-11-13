# Spec 067: pinned and scanned builds, a walk of every route that takes an id, and a ticket for the service for editing together

Wave RV2. Three parts: the build gates, a test that walks every route taking an id, and how the service for editing together learns who is connected.

## 1. Builds that are pinned, scanned and signed

**What is built.**

- **Actions pinned by commit.** Every action in `ci.yml` and `release.yml` is pinned by its commit SHA, with the version in a comment. The jobs use the same versions: `actions/checkout` v7.0.1, `actions/setup-go` v7.0.0 and `actions/setup-node` v7.1.0.
  - `renovate.json` extends `helpers:pinGitHubActionDigests`, so Renovate keeps the pins and comments current once the app is installed.
  - Renovate's own validator (`renovate-config-validator`) accepts the file.
- **Base images pinned by digest:** `postgres:16` in `compose.yaml`, in the backup stage of `Dockerfile` and in the CI services, and `node:22-slim` in `collab/Dockerfile`.
- **Three images scanned.** The `container` job builds the app image, the backup stage and the `collab` image. Trivy scans each and fails on a fixed HIGH or CRITICAL finding.
- **`npm audit` for the service.** The `web` job runs `npm audit --omit=dev --audit-level=high` for `collab` as it does for `web`.
- **Only what runs ships in the `collab` image.** It is built in two stages, and the runtime stage leaves out npm and corepack. Those bring their own packages into `node:22-slim`, and the service never runs them.
- **The backup stage takes Debian's updates at build time.** Its base is pinned by digest, so security updates published since then come in through `apt-get upgrade`. It also drops `gosu`: that tool serves the entrypoint of the postgres image, which the backup service replaces with its own.
- **Every image is released the same way.** `release.yml` builds and pushes the backup image and the `collab` image like the app image: for amd64 and arm64, with an SBOM and SLSA provenance, signed keylessly with cosign.

**Proof.**

- actionlint in a throw-away container reports nothing on the lines changed here.
- `renovate-config-validator` accepts `renovate.json`.
- All three images build locally under check tags.
- Trivy finds nothing HIGH or CRITICAL with a fix in any of them, using the settings of CI.
- The `collab` image starts and answers `/livez`.

**Not proven here:** the CI and release jobs themselves, which run only on GitHub.

## 2. Every route that takes an id is walked as a stranger

The `object:` rule of a route already proves access before the handler runs, and `TestEveryObjectRouteIsProtected` walks those routes. Other routes name an object by a path parameter or by ids in their body, and leave the decision to the module behind them. Examples are a saved block, an export or an approval round.

**What is built.**

- `idRoutes` in `internal/app/route_walk_test.go` is the one table of these routes. It names who each route lets through.
  - A route that any member may use on any object by design is marked open. Examples are a profile, or a list that holds only what the caller may view.
- `TestEveryRouteThatTakesAnIDIsProtected` sets up the walk:
  - It makes an owner, a colleague and a stranger.
  - It gives the owner one of every object these routes reach, each named with a marker.
  - It walks every route whose rule is `authenticated` (in any form) that has a path parameter or takes ids in its body. The walk fills the owner's ids in and acts as the stranger.
- The walk fails when:
  - a route is missing from the table;
  - a declared route is no longer walked;
  - the marker comes back in any answer;
  - a route not marked open answers below 400.
- At the end it checks that the owner's page, template, workflow and saved block are unchanged.

**What the walk found.**

- No route gave a stranger data.
- `POST /api/v1/pages/bulk-move` answers 200, with `moved: 0` and "Not found." for every page the caller may not move. It is declared open with that owner rule.

**Proof.**

- The test, which walks the 47 routes of the table.
- Two mutations, each removing one owner check behind a declared route: reading an export, and deleting a saved block.

## 3. The service for editing together holds a ticket, never the cookie

The browser opens a WebSocket on `/collab`, and the binary passes it on to the service (ARC-118). The service decides nothing itself: it asks the API (SEC-540, SEC-542).

**What is built.**

- **The browser names the page.** It opens `/collab?page=<id>`.
- **The API checks the upgrade itself, before passing it on.** It checks the origin as before, then the session cookie and the right to edit that page.
  - With no session it answers 401.
  - A page the person may not edit, a page that does not exist, a token or a session still waiting for its second factor gets 404.
- **The cookie stays with the API.** It removes the `Cookie` and `Authorization` headers. In their place it sets `X-Diffluence-Collab-Ticket`, and a header of that name from the browser is replaced.
- **The ticket** names the session, the person, the page and an expiry two minutes ahead.
  - It is signed with HMAC-SHA-256.
  - The key comes from the key file through `Keyring.Fingerprint`, under its own purpose (`collab ticket v1`).
- **The service asks with the ticket.** It reads only the ticket from the socket. It sends `/collab/authorize` the page and the ticket.
- **The API answers yes only when everything holds:**
  - the signature checks;
  - the ticket has not expired and names this page;
  - the session it names still lives and belongs to that person;
  - the person may still edit the page.
- **The check does not count as activity.** The API reads the session by its id through `PeekSessionByID`, so an open editor does not keep a session alive. The session ends at its idle limit, and the next check closes the socket.
- **The ticket is renewed at every recheck.** With every yes comes a new ticket, which the service keeps for the next check. It asks every minute, and at once after a change of rights.
  - A no, an error or no answer closes the connection, as before.
- **Where it lives.** The service's part is in `collab/access.mjs`, apart from `server.mjs`, so it is tested without a server.

**Proof.** Written first, and seen failing before the change:

- `TestTheServiceGetsATicketAndNeverTheCookie`
- `TestOnlySomeoneWhoMayEditCanConnect`
- `TestASocketNeedsAPageItMayEdit`
- `TestATokenDoesNotOpenASocket`
- `TestATicketOpensOnlyItsOwnPage`
- `TestAForgedTicketIsRefused`
- `TestAnOpenEditorDoesNotKeepTheSessionAlive`
- `TestATicketEndsWithItsSession`
- `TestATicketEndsWhenTheRightToEditEnds`

Further proof:

- Unit tests of the ticket in `internal/collab/ticket_test.go`: expiry, another person, another key, malformed tickets and fields.
- Eight tests in `collab/access.test.mjs`.
- An e2e test through the real service: a socket opened for one page cannot join the document of another.
- Mutations of each check.

## Load

- One more session query per open editor per minute, by primary key, replacing the one the cookie made. No new table, no migration.
- A ticket costs one HMAC to make and one to check.

## Decided on the product owner's behalf on 2026-11-13, may be overruled

- **D1: a signed ticket instead of a stored one.** An HMAC from a key derived from the key file needs no table and no new dependency. The key comes from the root of the key file, which stays when the KEK is replaced, so a key change does not close every socket.
- **D2: two minutes, renewed at each check.** The service checks every minute, so a ticket outlives one missed check and no more. A stolen ticket is worth two minutes on one page, and only while its session lives.
- **D3: the ticket names the session, not its secret.** The API can then check the session again without the service ever holding anything that opens the API.
  - Opening the socket goes through the normal sign-in check, as any request of the person does.
  - The rechecks of an open editor do not count as activity.
- **D4: 404 for a page the person may not edit.** The socket says nothing about whether a page exists. A token never opens a socket: editing together is for people in a browser.
- **D5: Trivy fails on fixed HIGH and CRITICAL findings.** An unfixed finding cannot be acted on and would block every build. `npm audit` for the service uses the same level as for the web app.
- **D6: the `collab` image leaves npm out.** Patching npm's own packages would wait for a new `node` image. The service never uses them.
- **D9: the backup stage upgrades its Debian packages when built.** This makes the stage depend on the day it is built. That was already so, because it installs `gnupg` from the archive. In exchange, a fix does not wait for a new digest of `postgres:16`.
- **D7: bulk-move is declared open.** It reports pages the caller may not move as not found, and moves nothing of theirs. The walk and the check of the owner's page at the end prove it.
- **D8: GitHub settings are not changed from here.** The repository's settings, such as rules for the main branch and the Renovate app, are the product owner's to set. The builder hands over the list.
