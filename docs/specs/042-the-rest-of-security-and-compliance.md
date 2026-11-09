# Spec 042: the rest of security and compliance

Status: built on branch `wave-s6`.

Phase: blueprint 06, the lines that do not wait for the product owner. Wave N2 ran at the same time (a new version notice, and the comments and pages modules), so this wave stays out of those screens.

## The study

Blueprint 06 held 145 lines that were not `built`: 68 new, 43 partly, 14 still marked with the wave that planned them, 5 planned in a phase and 15 later. Each was checked against the code and the tests in three ways, as in spec 035:

1. **A test proves it.** The test is named in the line.
2. **The code holds it, and a test of this wave proves it**, failing test first.
3. **A guide or the way of working says it**, for lines that ask for a text.

Lines without one of these stay as they were. A line is left out of this wave when it needs a new dependency, real servers, a lawyer, money, an independent tester, a decision of the product owner, or a screen that wave N2 changes.

## What this wave builds, worst risk first

| Line | What | Load budget (ARC-238) |
|---|---|---|
| SEC-546, SEC-309 (High) | Every browser that edits together takes out of the shared document what its schema and link rules refuse: unknown nodes, unknown attributes, unknown marks and addresses with an unsafe scheme. It does so on load and after every change from elsewhere, in one transaction with its own origin, so the service and every colleague get the cleaned state. A test speaks the protocol of the service with a real session, sends a script, a frame, an image with `onerror` and a `javascript:` link, and checks the colleague's page, the shared state and the published page | Browser only: one walk of the document per batch of changes from others, none for its own typing. No query |
| SEC-544 | The service sets the person in every cursor to the one the session proved (`collab/presence.mjs`, on `beforeHandleAwareness`). The same hostile client poses under another name, and the colleague never sees it | Service only, per cursor message. No query |
| SEC-127 | One mail when a password changes (by the person, by an administrator or by a reset link), when a second factor is turned on or off, and when a passkey is added or removed | One query and one queued mail per change |
| SEC-190 | At most 3 reset or sign-up mails per recipient in 15 minutes, from any address. The answer and its timing stay the same, so nothing tells whether the address has an account | No query; one counter in memory per node |
| SEC-113 | Turning off TOTP shares the count of every route that checks a password | No query |
| SEC-257, SEC-256 | A warning at start, and an item in the admin console, when the public address is plain http on any host but this machine | No query |
| SEC-627 | Compose publishes the app on `127.0.0.1` unless `DIFFLUENCE_BIND` says otherwise | None |
| SEC-153 | The cookie of the login flow is `__Secure-diffluence_oidc` on https | None |
| SEC-351, SEC-1022 | New fuzz targets for the HTML import, import archives and SCIM filters, and the search language joins `make fuzz`. A SCIM filter on an attribute the route does not read is now refused, where it was ignored | Nightly only |
| SEC-548 (partly) | A test proves that text removed is gone from the state the service keeps, stores and sends, and that it would stay without garbage collection | Test only |
| SEC-159, SEC-161, SEC-179, SEC-423, SEC-625, SEC-728, SEC-843, SEC-905 | `docs/operations/identity-providers.md` and `install.md` | Documents |
| SEC-308, SEC-1023 | The definition of done, and a list of gates in `09-way-of-working.md` that matches CI | Documents |
| SEC-434 | A comment at OIDC discovery says why it stays outside `internal/outward` | None |

The mutation check gets a third kind, `mutate_collab`, for the service's own tests. CI installs its packages in the mutation job.

## Built before, proven now

SEC-124, SEC-164, SEC-222, SEC-260, SEC-266, SEC-273, SEC-353, SEC-363, SEC-383, SEC-407, SEC-416, SEC-443, SEC-523, SEC-606, SEC-608, SEC-807, SEC-821, SEC-822, SEC-831, SEC-911, SEC-943. Each line in blueprint 06 names its test. SEC-807 keeps a changed promise: a read through a token leaves a `token.used` event at most once a minute per token, not one per call.

## What stays open, and why

- **Waits for a decision of the product owner:** SEC-006, SEC-014 and SEC-1024 (review sessions per wave), SEC-110 with SEC-111 (a lasting count of 100 failures, against lockout by anyone who knows an address), SEC-176, SEC-333, SEC-648, SEC-1007, SEC-1017, SEC-1018, SEC-1003 and SEC-1031 (settings of the repository), and the audit tail.
- **A lawyer:** SEC-533, SEC-901, SEC-916, SEC-918, SEC-927, SEC-932, SEC-933, SEC-941, SEC-1042.
- **Real servers, phase 3 or money:** SEC-109, SEC-231, SEC-232, SEC-274, SEC-331, SEC-525, SEC-532, SEC-624, SEC-639, SEC-647, SEC-656, SEC-712, SEC-729, SEC-816, SEC-1034, SEC-1035, SEC-1040.
- **A new dependency:** SEC-722 (a secret scanner), SEC-1006 and SEC-1041 (an SBOM tool), SEC-1015 (signing), SEC-1004 and SEC-1005 (pins need lookups in outside registries).
- **A feature that does not exist yet, so its rules wait for it:** SEC-556 to SEC-559 (a PDF editor), SEC-535 (WOPI), SEC-513 (office conversion beside PDF export).
- **Too large for this wave, each a feature of its own:** SEC-116 (a reset link from the admin screen), SEC-148 (back-channel logout), SEC-192 and SEC-921 (changing a local address), SEC-926 (who could see a page on a date), SEC-913, SEC-919, SEC-151, SEC-210, SEC-008, SEC-705 (a key policy), SEC-1008.
- **Small, and not reached in this wave:** SEC-136, SEC-177 (the button shows only after removing a factor), SEC-243, SEC-255, SEC-319, SEC-320, SEC-334, SEC-355, SEC-384, SEC-618 (the console's design picture would change), SEC-655 (a notification needs a new kind with its mail and screens), SEC-805, SEC-1032, SEC-106.
- **No proof found, so still marked with their wave:** SEC-013, SEC-138, SEC-275, SEC-915.
- **Ongoing until 1.0:** SEC-001, SEC-005, SEC-007, SEC-714, SEC-1033.
- **Later by design:** SEC-137, SEC-147, SEC-181, SEC-234, SEC-318, SEC-354, SEC-715, SEC-727, SEC-730, SEC-806, SEC-832.
- **SEC-548** stays partly: a browser that had the page open keeps its own copy until logout clears it.

## Counts

| | Built | Partly | Marked with a wave | Phase | New | Later | Total |
|---|---|---|---|---|---|---|---|
| 06 before | 284 | 43 | 14 | 5 | 68 | 15 | 429 |
| 06 after | 328 | 33 | 4 | 5 | 44 | 15 | 429 |
