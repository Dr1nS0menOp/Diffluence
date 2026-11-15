# Handoff: blueprint 03 completed from pictures of another product's screens (15 November 2026)

## State

- **Done:** blueprint 03 was checked against 154 pictures of another product's screens and completed: 762 new requirement lines (588 new, 174 later) across all thirteen chapters, with new sections in ten of them. The report is `docs/reports/2026-11-15-screens-study.md`.
- **Decisions:** ADR-045 (the product owner accepts the risk of using the pictures, under the conditions in 02; for arrangement the pictures win over the design standard; the look stays ours). The exception and its conditions are in `docs/blueprint/02-clean-room-and-licensing.md`, "One exception".
- **Design standard:** `docs/design/README.md` has a new section, "The arrangement decided on 2026-11-14 (ADR-045)", that lists every rule a line replaces. Nothing is built yet: until a screen is rebuilt, the existing rule describes it, and the design pictures still guard it.
- **No code changed.** Every new line has the status new or later, so no status of a built line is wrong.

## For whoever builds next

- The lines that change built screens are the urgent part, because they describe built behaviour differently. Find them with `rg "changes the design standard, decided 2026-11-14" docs/blueprint/03-functional`. Most rewrite the frame (top bar, sidebar, start screen), the page header and byline, the editor's strip and insert dialog, the administration's sidebar and column, the settings of a space, and Your account.
- Every area still starts with its own study and spec (`docs/blueprint/09-way-of-working.md`). A sensible order, because each builds on the one before:
  1. **The frame:** top bar, the global part of the sidebar and its panels, the space's block in the sidebar, the start screen (chapter 1, sections 1.2, 1.3 and 1.10 to 1.15; chapter 2, section 2.4).
  2. **The page:** header row, byline, More, Share, the notification panel (chapter 3, section 3.11; chapter 7).
  3. **The editor:** strip, row of tools, insert dialog, tables (chapter 4, sections 4.1 to 4.3 and the table lines).
  4. **Settings:** the administration's sidebar, centred column, tables and notices (chapter 10, section 10.1), then the settings of a space (chapter 2, section 2.11), then Your account (chapter 9).
  5. **New features:** everything else, within the feature freeze of ADR-043, which keeps new parts for after 1.0 unless the product owner says otherwise.
- Each of these waves changes many design pictures. Show the product owner the before and after pictures, as the design standard asks.
- The study material (inventories, comparisons, maps per chapter) is outside the repository on purpose, under the conditions of 02. Never add pictures, inventories or their file names to the repository.

## For the product owner

**Built things these lines change; please confirm, because some reverse your own earlier decisions:**

- The strip of spaces and the fixed Home row leave the sidebar (ADR-027; NAV-135, SPC-097, SPC-106).
- Home, Spaces, Recent and Favourites leave the top bar for the sidebar (NAV-091, NAV-125); Administration moves from the account menu to a settings button (NAV-095, ADM-059).
- The start screen loses its greeting, the grid of spaces and the side column (NAV-103).
- The star and the lock leave the page header; the byline loses date and version; Watch becomes a sub-menu (PAG-141 to PAG-144, PAG-151).
- The administration gets a new sidebar and a centred column, replacing the "Look" item of ADR-038 (ADM-068 to ADM-078); the settings of a space get a new grouping, replacing spec 033's groups (SPC-135, SPC-145).
- Your account gets its own bar without search, the bell or Create (ACC-102, ACC-104).
- Tables lose the line under each row (ADM-107).
- A rule is built as a chain of cards instead of a form (PLT-129 to PLT-152).

**Questions only you can answer:**

- One owner per space or several (SPC-094)? A personal space shows its owner's picture instead of an animal (SPC-132, ADR-031)?
- Five or six suggested statuses (PAG-055, PAG-133)? Does the home page of a team space drop its byline too (PAG-146)?
- Classification levels may keep guests out, which the one permission rule then has to include (PAG-162, PAG-163). May a page be less strict than its space?
- ACC-017 promises login history "with place", ADR-026 says no place. Which stays?
- Instance permissions as an editable table of ticks, shaped as roles (ACC-150), or read-only as ADR-034 chose?
- A second factor for all members does not reach guests until login policies exist (ACC-015). A smaller fix sooner?
- An administrator may change someone's e-mail address when mail is not set up (ACC-087), against SEC-192 today.
- The instance name beside the logo: on by default as today, or off as pictured (ADM-134)? A backup without files (ADM-190)? Hiding the password form on the login page needs a study first (ADM-140).
- A reader who presses a rule button runs the rule as the rule's account (PLT-063). Should apps' entries carry a mark that they come from an app (PLT-090, PLT-097)?
- Links to other tools in two places, top bar and sidebar (NAV-099, NAV-130), or one? Short lists of settings: an add form above the table or an add row in it (EDT-207)?
- Watermarks: skip animated and very large images, or refuse them in a space that wants a watermark (FIL-068)? A mail archive per space (COL-129)?
- The security overview (ADM-125 to ADM-128) and the instance's header and footer (ADM-147 to ADM-149): after 1.0 under the feature freeze, or now?

**Decided on your behalf, may be overruled:** listed in the report, "Decided on the product owner's behalf".

## Seen, not changed (existing lines that need a repair of their own)

- `03-functional/03-pages.md`: PAG-016, PAG-017 and PAG-032 to PAG-036 have broken separators, and PAG-018 runs on in the line of PAG-017.
- NAV-020 puts search on the far side; ADR-035 and the design standard put it in the middle. NAV-001 speaks of four frames; the design standard has five.
- ADM-002 lists the old sections; ADM-035 says new while TPL-1000 says built; ADM-049 and ADM-057 are out of date since the console runs backups (OPS-115); ADM-008 does not say the user list filters by role and space.
- SEC-229 against ARC-140 and SEC-228 on whether a token with the admin scope reaches the administration.

## Checks run

- Every new line was checked for number clashes (none), duplicates and contradictions across chapters, and for quoted labels, product names, picture references, pixel sizes and colour values (none left).
- No code changed, so `make check` and the end-to-end tests were not run.
