# 038: The first setup screen

Status: built on branch `wave-f2`. Asked for by the product owner: whoever installs Diffluence names the company or person it is for, which the search box then shows as "Search in {name}". Until now that was only possible on the command line or under Administration, Look (spec 033).

Also in this wave, from the open points of wave X, both approved by the product owner: the folded state of the settings groups is kept per person, and the results page drops its own space select in favour of the `space:` words of the search panel.

## The danger, first

A screen that creates the first administrator is the most valuable target an install has. A fresh server is often reachable before its owner finishes: whoever reaches the screen first would own the instance, its later content and its audit log. The design starts from that.

| Threat | Answer |
|---|---|
| A stranger reaches a fresh install first | The screen needs a one-time code that only appears in the server's log and in `diffluence setup-code`. Reading either means running the server already. |
| Guessing the code | 80 random bits. 10 wrong tries end a code, and one address gets 10 tries in 15 minutes. |
| A code that lies around | A code lasts an hour. A new code replaces every older one. |
| Reusing a code, or a setup after the setup | A code works once. The screen is closed as soon as any administrator grant exists, or any code was ever used. Closed routes answer exactly as an unknown route does. |
| Two people at once | Using the code and creating the administrator are one transaction under one advisory lock. The second finds the setup closed. A test runs two at the same moment. |
| Reading the code from the database | Only a SHA-256 digest of the code is stored, with a fixed prefix. |
| Timing | The given code is digested and compared to the stored digest with `subtle.ConstantTimeCompare`; both have the same length. |
| Learning the form's rules without the code | The code is checked before any other field, so a stranger gets the same refusal whatever else they send. |
| Leaking the code in the audit log | Every try is audited, as `setup.refused` or `setup.completed`, with address and request, never with the code. A test plants codes and searches the whole audit log for them. |
| A mistake in the form burns the code | A refused field rolls the whole transaction back: the code stays usable. |

Logging the code is on purpose and is the one exception to "no secrets in logs": it is short-lived, works once, and the log of a server is read by whoever runs it, which is exactly who should set it up. It is written at level warn, once per start, while the setup is open.

## What was built

| Part | Where |
|---|---|
| Table `setup_codes`: digest, expiry, use, wrong tries | migration 0038 |
| `setup.Service`: `Open`, `Issue`, `Check`, `Complete`, the code format | `internal/setup` |
| One step: create the administrator inside the transaction that uses the code | `identity.CreateLocalUserIn`; `CreateLocalUser` now wraps it, unchanged in behaviour |
| Organisation name and default time zone, checked before anything is created | `settings.CheckFirstSetup`, `settings.ApplyFirstSetup` |
| `GET /api/v1/setup` (open or 404), `POST /api/v1/setup` | `internal/httpapi/handlers_setup.go`; both public, with the reason in the code and in blueprint 04.5 |
| The server logs a fresh code at start while the setup is open | `internal/app/setup.go` |
| `diffluence setup-code` | `cmd/diffluence/setup.go` |
| The screen: code, organisation, the first administrator, language and time zone | `web/src/views/setup.tsx` |
| The login page sends a visitor of an open install to `/setup` | `web/src/views/login.tsx` |
| After the setup the new administrator is signed in and lands on Security with a note to add a passkey | `web/src/views/profile.tsx` |
| Folded settings groups follow the person | preference `settings_shut`; `web/src/components/settings-frame.tsx` |
| No space select on the results page | `web/src/views/search.tsx` |

The language list holds English only, as the rest of the product does; the field is there so the next language needs no new screen.

Versions must be contiguous, so 0037, which spec 037 did not use, is a `SELECT 1` placeholder between wave R's 0036 and this 0038.

### The settings area at the width of a laptop

Found by the product owner in the same wave: at about 1150 px the Users screen ran past the window, with the Source filter and the Email column out of reach. An address that cannot wrap made the table wider than the room, and nothing kept it inside.

| Part | Where |
|---|---|
| The area starts beside the sidebar, not in the middle of the room; its parts may shrink, and a select or field is never wider than the room | `.settings-area` in `web/src/design/layout.css` |
| Every table of the app sits in a box that scrolls it sideways; only while it scrolls is the box a region named "Table, scrolls sideways" and a Tab stop | `TableBox` in `web/src/components/table-box.tsx`, `.table-box` in `controls.css`, used by `SortTable` and every bare table in `web/src/views` |
| An address in a table breaks where it must, in pieces of at least 16 characters; a select in a table fills its column and cuts a long choice short, so neither can widen the table | `td.email` and `td select` in `controls.css` |
| Six admin and settings screens at 1024, 1150, 1280 and 1600 px: the area starts at the sidebar's edge plus the padding, nothing outside a table box passes the edge, and the page never scrolls sideways | `the settings area at the widths of a laptop` in `web/e2e/0-design.spec.ts`, which failed before the fix |
| On a narrow screen the table of people scrolls in its box: the box is a named region, the keyboard reaches it, an arrow key scrolls it, the page does not move | same block |
| Pictures: Users at 1150 px light and dark, Spaces at 1150 px, Users on a narrow screen | `admin-users-1150-*.png`, `admin-spaces-1150.png`, `admin-users-narrow.png` |

## Tests

Failure paths first, in `internal/app/setup_test.go`: no code, a wrong code, a wrong code with a wrong form, an expired code, an old code after a new one, too many wrong tries, too many tries from one address, an administrator already present, the setup closed for good after use even when no administrator is left, two people at once, a wrong field that leaves the code usable, and no code in the audit log or the table. Then the way that works: the administrator, the organisation, the time zone and a working session.

`cmd/diffluence/setup_test.go` covers `setup-code` before and after an administrator exists, and `create-admin --organisation`, which had no test.

Seven new mutations break each guard: any code works, codes never expire, codes can be guessed for ever, a used code stays unused, the setup stays open with an administrator, the form is read before the code, and one address may guess for ever. Each is caught.

## Load budget (ARC-238)

| Route | Queries | Writes | Class |
|---|---|---|---|
| `GET /api/v1/setup` | 1 | 0 | reading |
| `POST /api/v1/setup`, refused | 3 to 4 | 1 counter, 1 audit row | saving |
| `POST /api/v1/setup`, accepted | about 15, once in the life of an install | user, grants, audit, settings | saving |
| Server start | 1 to 3 | 1 code, 1 audit row, only while open | none |

The login page asks `GET /api/v1/setup` once per visit before anyone logs in: one indexed `EXISTS` on two small tables.

## Not built, or not proven

- A real end-to-end run of the screen in the browser: the test instance already has an administrator, so the browser test shows the form with a stubbed "open" answer, and the rules are proven in Go.
- The log line when the server starts is not covered by a test, because the tests do not run `serve`; the text it uses is.
- More than one language: the field shows English only.
- The settings area in Firefox and Safari: the tests run Chromium only. In Chromium the exact picture of the report, with the area starting in the middle of the window, did not appear with the test data; the overflow did, and is fixed.
- With several API nodes, each start makes a new code and the newest wins; the install guide says to use the newest line or `diffluence setup-code`.
