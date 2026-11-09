# Handoff: the first setup screen (wave F2, spec 038)

## Done

- A new install opens on `/setup`: the organisation name, the first administrator and the default time zone, then Security with a note to add a passkey.
- The screen is guarded by a one-time code from the server's log or `diffluence setup-code`: an hour, once, 10 wrong tries, 10 tries per address per 15 minutes, checked before any other field, used in the same transaction as creating the administrator, closed for good after use. Every try is audited without the code.
- `create-admin --organisation` has a test now.
- Folded settings groups are kept per person (`settings_shut`).
- The results page has no space select of its own; spaces come from `space:` words.
- The settings area starts beside the sidebar instead of in the middle of the room, and nothing in it runs past the window. Every table of the app sits in `TableBox` (`components/table-box.tsx`), which scrolls a wide table in its own box and is a named Tab stop only while it scrolls. An address in a table breaks where it must and a select in a table fills its column, so neither can widen a table; the Access screen of a space shows its Actions column again. A test in `0-design.spec.ts` checks six admin and settings screens at 1024, 1150, 1280 and 1600 px, and the box on a narrow screen with the keyboard.
- `scripts/mutation-check.sh` copies the tree once into a snapshot, and every mutation starts from that copy, so a change to the tree during a run no longer gives a false BROKEN.

## On landing

- Migration 0036 is wave R's `0036_roles.sql`. `0037_reserved.sql` stays: spec 037 had no migration and versions must be contiguous. This branch removes wave A4's placeholder `0038_reserved_wave_f2.sql`, because `0038_first_setup.sql` takes the number; A4's `0039_helper_health.sql` follows it.
- The branch is rebased onto wave A4 (77d8fca). Two conflicts, both resolved by keeping both sides: the deps in `internal/app/app.go` (`HelpersDown` and `Setup`) and the end of the mutation list (A4's five and this wave's seven; 399 in all, 392 on main).
- A new table of the app goes inside `TableBox`; the design standard says so. Nothing checks it automatically yet.

## Next

- A screen reader pass of the setup screen.
- A second language for the language field, when the product has one.
- A check in the design test that flags a bare `<table>` in `web/src/views`.
- Give every `<time>` a fixed text inside `picture()` in `0-design.spec.ts`. A masked relative time still changes width with the length of the run; All spaces and Recent flaked on it and now fix their own times, other pictures with a time may follow. Note that a run with `--update-snapshots` hides such a flake, because it rewrites the picture.

## Not proven

- The setup screen end to end in a browser: the test instance has an administrator, so the picture uses a stubbed answer; the rules are proven in Go.
- The log line at server start is not run by a test.
- The exact screen the product owner saw at 1150 px, with the area starting in the middle, did not appear in Chromium with the test data. The overflow did appear, from an e-mail address that cannot wrap (the layout test failed on it before the fix), and is fixed. Firefox and Safari were not run.
