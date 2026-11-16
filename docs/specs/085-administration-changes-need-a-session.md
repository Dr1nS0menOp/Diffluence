# 085: Administration changes need a signed-in session

Status: built
Phase: hardening before 1.0 (wave RV10)

## Problem

Spec 015 H9 gave personal access tokens four scopes, among them `admin`, so a script can read what Administration shows. Spec 013 D12 and AC-12 say a token never reaches account security or the console, and ARC-140 says `SessionOnly` keeps tokens away from the console. Both rules are kept route by route: a route below `/api/v1/admin/` takes a token unless whoever registered it wrapped its rule in `SessionOnly` or `Sensitive`.

That leaves the choice to every new route, and the answer to who reads the route table. Changing Administration is work for a person in a signed-in session, so the choice should not exist per route.

## Study

**What the route table says.** `docs/api/routes.txt` on 16 November lists 142 routes below `/api/v1/admin/`: 86 change something, 56 read. Most changes were already `session-only` or `sensitive` by their own rule; 13 followed only the `admin` scope.

**Why a rule and not edits per route.** Wrapping those routes one by one would settle today's list and leave the next route to the same choice. The default must be the safe one: a new administration route that changes something refuses a token, decided in the router, where it is visible, and checked by a test that walks every route.

**What a token keeps.** The `admin` scope stays useful for reading: a monitoring script can read analytics, insights, the health of mail and the features that are on. A read changes nothing, and every read route below `/api/v1/admin/` still needs the owner to be an administrator, as before.

**Scope of this wave.** The rule covers the prefix `/api/v1/admin/`. Routes outside it keep the rules they were registered with.

## Decision

Taken on the product owner's behalf on 16 November 2026, open to change (ADR-052):

- **D1.** Every route below `/api/v1/admin/` whose method is not `GET` or `HEAD` is session-only. The router applies this when the route is registered, in one place, so a route cannot forget it. A pattern without a method matches every method, so it counts as a change.
- **D2.** Read routes below `/api/v1/admin/` keep their rule: a token with the `admin` scope, owned by an administrator, still reads them, unless the route is already session-only or sensitive.
- **D3.** `routes.txt` and `openapi.json` show the rule as `session-only`, because they print the rule the router enforces.
- **D4.** A refused token gets `403` with the code `session_only`, the same answer as on every other session-only route.

## Behaviour

- `Router.Handle` makes the rule session-only for a change below `/api/v1/admin/` before it records and wraps the route. `SessionOnly` and `Sensitive` on such a route stay allowed, and change nothing more.
- `scopeFor` and the router name the prefix through one constant, so the two cannot drift apart.
- The help text on Your account, Tokens, says that the Administration scope reads Administration and only a signed-in session changes it; the scope is labelled "Administration, read only".

## Load budget

No change. The rule is decided once at start-up; a request costs one boolean check more, which was already there.

## Acceptance criteria

- [x] AC-1 A token with every scope, owned by an administrator, is refused with `session_only` on every route below `/api/v1/admin/` that is not a read. The test walks the real route table, so a route added later is covered the moment it exists.
- [x] AC-2 The same token still reads a read route below `/api/v1/admin/`.
- [x] AC-3 The router marks a change below `/api/v1/admin/` session-only whatever rule it was given, also for a pattern without a method, and leaves reads and routes outside the prefix as they were.
- [x] AC-4 `routes.txt` and `openapi.json` show every route below `/api/v1/admin/` that changes something as `session-only` or `sensitive`.
- [x] AC-5 A mutation that drops the rule is caught.

## How the product owner tests it

1. Sign in as an administrator, open Your account, Tokens, and make a token with every scope.
2. With the token, `GET /api/v1/admin/features`: the answer is the list of features.
3. With the token, `PUT /api/v1/admin/features` with the same body: `403`, code `session_only`.
4. In the browser, change a feature in Administration: it works as before.

## Provenance

Own design, from specs 013 and 015 and blueprint 04 (ARC-140).
