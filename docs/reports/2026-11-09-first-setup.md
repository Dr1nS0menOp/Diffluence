# Report: the first setup screen, a mutation runner that tests one snapshot, and settings that fit a laptop (wave F2)

Written for later build sessions. Spec 038 holds the design; this holds what was found and proven.

## The setup screen

The danger was the whole design: a screen that makes the first administrator is the most valuable target of a fresh install. Seven guards, each with a test and a mutation:

| Guard | Test | Mutation |
|---|---|---|
| A code is needed | TestTheSetupScreenRefusesAnyoneWithoutTheCode | any setup code works |
| A code expires after an hour | TestAnExpiredSetupCodeIsRefused | a setup code never expires |
| 10 wrong tries end a code | TestASetupCodeEndsAfterTooManyWrongTries | a setup code can be guessed for ever |
| A used code closes the setup for good | TestTheSetupStaysClosedEvenIfNoAdministratorIsLeft | a used setup code stays unused |
| An administrator closes the setup | TestTheSetupIsGoneOnceAnAdministratorExists | the setup stays open with an administrator |
| The code is checked before the form | TestTheSetupScreenRefusesAnyoneWithoutTheCode | the setup form is read before the code |
| One address gets 10 tries per 15 minutes | TestOneAddressCannotKeepGuessingSetupCodes | one address may guess setup codes for ever |

Also proven by tests: two people at once make one administrator; a wrong field leaves the code usable; no code appears in the audit log or the table; a new code makes the old one useless; `diffluence setup-code` refuses once an administrator exists; `create-admin --organisation` stores the name and refuses markup.

Two findings while building:

- A refusal inside the transaction that uses the code would have counted a wrong try twice: once in the rolled-back transaction, once in the separate count. The transaction now rolls back on a refusal, and only the separate check counts.
- Wave R moved global grants behind roles (`rbac_add_permissions`). On rebase, creating the first administrator inside the setup transaction follows that path; `global_grants` remains as what roles add up to, so "an administrator exists" still reads it.

Not proven: the screen end to end in a browser (the test instance already has an administrator, so the picture uses a stubbed "open" answer), and the log line at server start, which no test runs.

## The mutation runner

The faster runner of spec 037 copied the working tree when each mutation started. When the tree changed during a run, later mutations tested different code and could report a false BROKEN; this happened on main.

The runner now copies the code once into a snapshot when the run starts, and every mutation copies from that snapshot. The list of mutations is unchanged (396 `mutate` lines before and after), and so are the output, its order and the exit codes.

Proof, on a copy of this branch in its container, with `MUTATION_JOBS=1` and only the mutations of `internal/setup/setup.go`. Twenty seconds into each run, a line that a later mutation needs was changed in the working tree.

The runner before the snapshot, with the same list:

```
caught  any setup code works: --- FAIL: TestTheSetupScreenRefusesAnyoneWithoutTheCode
caught  a setup code never expires: --- FAIL: TestAnExpiredSetupCodeIsRefused
BROKEN  a setup code can be guessed for ever (the mutation no longer applies; update this script)
caught  a used setup code stays unused: --- FAIL: TestTheSetupStaysClosedEvenIfNoAdministratorIsLeft
caught  the setup stays open with an administrator: --- FAIL: TestTheSetupIsGoneOnceAnAdministratorExists
1 mutation(s) were not caught
exit 1
```

The runner with the snapshot:

```
caught  any setup code works: --- FAIL: TestTheSetupScreenRefusesAnyoneWithoutTheCode
caught  a setup code never expires: --- FAIL: TestAnExpiredSetupCodeIsRefused
caught  a setup code can be guessed for ever: --- FAIL: TestASetupCodeEndsAfterTooManyWrongTries
caught  a used setup code stays unused: --- FAIL: TestTheSetupStaysClosedEvenIfNoAdministratorIsLeft
caught  the setup stays open with an administrator: --- FAIL: TestTheSetupIsGoneOnceAnAdministratorExists
all mutations were caught
exit 0
```

The installed packages of the web app are still linked from the working tree, not copied: they are large and do not change within a run. A change to them during a run would still reach later web mutations.

## The settings area at the width of a laptop

The report: at about 1150 px, with the sidebar open, the Users screen started in the middle of the window and ran past its right edge, hiding the Source filter and the Email column.

What was measured first, in Chromium, on all 22 admin tabs and 18 space settings sections at 1024, 1150 and 1280 px: with the test data every screen started 40 px (the padding) after the sidebar and nothing passed the edge. Long names that can wrap, of a space, a group and a person, changed nothing either. An e-mail address without a hyphen did it: it cannot wrap, so the table of people became wider than the room, and the whole page scrolled sideways at 1024 px. The new layout test failed on exactly that before the fix:

```
Error: /admin/users at 1024 px
-   "pageScrolls": false,
+   "pageScrolls": true,
```

Three causes, three fixes:

- Two things in a cell set a table's least width without any limit: an address, which has no place to wrap, and a select, which is as wide as its longest choice. The Access screen of a space already ran past its column for the second reason: a person with a custom role has a choice that lists every permission, and the Actions column sat beyond the edge of the page column in the old picture. An address in a table now breaks where it must, in pieces of at least 16 characters, and a select in a table has a width in percent, which a browser counts as narrow when it shares out the columns. A first try with `max-inline-size` on the select changed nothing in Chromium; `inline-size: 100%` does it.
- Nothing kept a table inside its room. Every table of the app now sits in `TableBox`, which scrolls it sideways in its own box. While it scrolls, the box is a region named "Table, scrolls sideways" and a Tab stop, so the keyboard can scroll it; otherwise it adds nothing for a screen reader or the Tab key. The tables inside a page already had this (`.prose .tableWrapper`).
- The area was centred in the room beside the sidebar (`.column` with `margin-inline: auto`). Below about 1490 px (the sidebar, the padding and the widest the area may be) the room is narrower than the area, so it did not show; on a wider screen it leaves an empty band beside the sidebar. The settings area now starts at the sidebar.

With only the box, the table of people at 1150 px scrolled, and the browser squeezed every other column to its narrowest word, so rows grew tall and empty. That is why the address and the select are fixed at the source as well: at 1150 px every column now fits, and the box is the safety net for a table that truly has too many columns, as on a narrow screen.

The pictures were all looked at: Users at 1150 px light and dark, Spaces at 1150 px, Users on a narrow screen, and the Access screen of a space, which changed because its Actions column is now in view.

Two existing pictures failed now and then: All spaces and Recent. Both mask a relative time, but "now" and "2 minutes ago" differ in width and move what sits beside them, depending on how long the run has taken. Because the design pictures run in one serial block, such a failure also skips every later picture. Both tests now give the time a fixed text before the picture, as History already did, and so do the new pictures with a login time or a last activity. A run with `--update-snapshots` hides this kind of flake, because it rewrites the picture; the full e2e run, which does not, found both.

The middle start in the report did not appear in Chromium with any data tried. It may come from another browser or a zoom level, which the tests do not run; with the area no longer centred and nothing allowed past the edge, there is no rule left that could place it there.
