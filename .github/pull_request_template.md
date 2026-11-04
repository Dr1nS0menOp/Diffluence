## What and why

<!-- One or two sentences. Link the issue and the spec in docs/specs/. -->

## How to test (plain language, for the product owner)

- [ ] Step 1 …
- [ ] Expected result …

## Checklist

- [ ] Tests written first, `make check` green
- [ ] New routes declare an access rule
- [ ] State changes write audit + outbox events in the same transaction
- [ ] Migrations are new files (no edits to merged ones) and keep the previous release working
- [ ] No new dependency, or its licence is MIT/BSD/Apache-2.0/ISC and the reason is given
- [ ] Docs and handoff note updated
- [ ] Clean-room: no competitor code or text was consulted
