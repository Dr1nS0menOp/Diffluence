# Spec 047: floating bars, near matches for names, and the storage limit everywhere

Status: built in wave ES (2026-11-10). The product owner answered its five questions on 2026-11-10; the decisions below include those answers, and the questions section records them. "What was built" at the end says where the build differs from the plan.

Phase: the third wave of ADR-038 (editor and search). Waves PM (spec 045, migration 0041) and SX (spec 046, migration 0042) come before it, so this wave takes migration 0043. Neither of them touches the editor bars, `internal/search` or the storage limit, so the three waves do not overlap.

This study was written from the code on `main` at `2965bf7`, without running anything. Every number below that is not quoted from `docs/operations/limits.md` is an estimate, and the build must replace it with a measurement.

## The decisions it builds on

- **ADR-038, editor:** a floating bar appears over selected text and over a selected image (EDT-006, EDT-080), beside the fixed bar.
- **ADR-038, files and search:** file names and titles are matched with trigrams (`pg_trgm`), so part of a word and a small typo also find them, with exact matches and word starts first. This widens ADR-037. The storage limit of a space counts everywhere: uploads, page copies, moves, imports and the trash.
- **ADR-035 and ADR-037:** search stays in PostgreSQL; finding pages and checking permissions stay apart in the code; within a tier the newest comes first, and nothing is left out because it is old.
- **ADR-036:** room for four times the database work; every feature states its load budget (ARC-238).
- **Spec 008, D3, and the design standard** said there are no floating bars in the editor. ADR-038 changes that, so `docs/design/README.md` changes with this wave.

## Part A: floating bars in the editor

### What exists

- The fixed row (`web/src/components/editor/toolbar.tsx`) holds every tool, is one Tab stop with arrow keys inside (`useRovingFocus`), and Escape returns to the text.
- The tools for what is selected appear at the end of that row (`selection-tools.tsx`, `SelectionTools`). `ImageTools` there already holds what EDT-080 asks for: size, alignment, wrap, border, into the line, describe, and edit for a drawing. Delete is the block menu's.
- A reader already has one floating row: `components/comments/selection-bar.tsx` (`SelectionBar`), placed from `window.getSelection()` with `position: fixed` and `transform: translate(-50%, -100%)`, styled by `.selection-bar` in `design/together.css`, and hidden on narrow screens. `Menu` with `floating` (`components/menu.tsx`, `placeBeside`) places a menu inside something that scrolls.
- **Packages.** `web/package-lock.json` already holds TipTap's bubble menu, as an optional dependency of `@tiptap/react` 3.32.2, so it is installed today but not listed in `web/package.json`:

  | Package | Version | Licence | Comes in through |
  |---|---|---|---|
  | `@tiptap/extension-bubble-menu` | 3.32.2 | MIT | optional dependency of `@tiptap/react` 3.32.2; its peers are `@tiptap/core` and `@tiptap/pm`, both exactly 3.32.2 |
  | `@floating-ui/dom` | 1.8.0 | MIT | dependency of the bubble menu (`^1.0.0`); the positioning library |
  | `@floating-ui/core` | 1.8.0 | MIT | dependency of `@floating-ui/dom` |
  | `@floating-ui/utils` | 0.2.12 | MIT | dependency of `@floating-ui/dom` and `@floating-ui/core` |

  The version matches every installed TipTap package (3.32.2). The bubble menu asks for exactly the same version of `@tiptap/core` and `@tiptap/pm`, so all TipTap packages must move together when Renovate updates them. `@tiptap/extension-floating-menu` 3.32.2 (MIT) comes in the same way and stays unused.

### Decisions

| | Decision | Why |
|---|---|---|
| D1 | **Decided by the product owner on 2026-11-10:** the bars use TipTap's `BubbleMenu`, the React component that `@tiptap/react` 3.32.2 provides on top of `@tiptap/extension-bubble-menu`. `@tiptap/extension-bubble-menu` is added to `web/package.json` at exactly `3.32.2`, the version already in the lock file, so it becomes a direct dependency. Positioning comes from `@floating-ui/dom`, which it pulls in. All four packages are MIT (table above), and `make licenses` checks them in CI. The build checks the component's import path and options against the installed types before using them | A maintained component that follows the editor's selection, scroll and resize, from the editor family we already use. It is a new dependency, approved in principle by the product owner |
| D2 | **Decided by the product owner on 2026-11-10:** the fixed row stays as it is, with every tool, and it cannot be hidden for now (the rest of EDT-006 waits). The bar holds a selection of the same `Tool` components, so a tool shows the same state in both places, and there is one source for what a tool does | ADR-038: "beside the fixed bar". Nothing is reachable only from the bar |
| D3 | **Decided by the product owner on 2026-11-10. Text bar:** bold, italic, link, inline code, text colour, highlight, Heading 1, Heading 2, Heading 3, bulleted list, numbered list, and Comment when comments are open. **Image bar:** the existing `ImageTools`, plus Delete from the block menu, which removes the image from the page and leaves the file (EDT-080). The headings and lists are toggles, each with its name and pressed state, like the matching tools of the row. At a width where the text bar does not fit on one line it wraps to a second line; it never folds tools into a menu of its own | The product owner's list. Wrapping keeps every tool one press away on a narrow screen |
| D4 | The bar shows for a text selection that is not empty, and for a selected image. It does not show for an empty cursor, a selection inside a code block (marks are refused there), a table cell selection (the table has its own menu, design README), any other node selection, while composing text with an input method, or while a dialog or menu is open | Each of these either has its own tools or would cover what is being typed |
| D5 | It sits above the selection, centred, and never covers the fixed strip and row: when there is no room above, it goes below. It stays inside the viewport at both ends, and follows on scroll and resize. On a touch screen (`pointer: coarse`) it goes below the selection, because the system's own menu and handles sit above it. **How:** `BubbleMenu` places through Floating UI, which follows scroll and resize itself. We pass its placement (`top`, or `bottom` on a coarse pointer), a flip to the other side, and a shift that keeps it inside the viewport, with the bottom of the sticky row as the top edge of the space it may use. These options come from one function of ours, `barPlacement(coarse, keepOut)`, so they can be tested without a browser | The fixed row is sticky, so "above" can run into it at the top of a page. Floating UI does the measuring; the rules stay ours |
| D6 | Keyboard: the bar is not a Tab stop of its own. Alt+F10 moves focus into the bar while it shows (into the fixed row when it does not), arrow keys move inside it, Escape returns to the text with the selection kept. It is `role="toolbar"` with a name ("Text tools", "Image tools"). It announces nothing when it appears. **How:** `BubbleMenu` only shows and places what we put in it; it adds no roles, names or keys. We put a `div role="toolbar"` inside it, with the roving focus the fixed row already uses (one hook for both), and the Alt+F10 and Escape keys on the editor | Tab must keep its meaning in the text. Alt+F10 is the key many rich-text editors use to reach their toolbar, so people who rely on the keyboard may know it already. Everything in the bar is also in the row, so a person who never uses the bar loses nothing |
| D7 | Right-to-left: centred on the selection, so it needs no mirror, and the shift of D5 keeps it inside the viewport on either side. Arrow keys follow the reading direction. **How:** Floating UI places by the box of the selection, which does not depend on direction, and the order of the tools follows `dir` through the flex row. The roving focus hook of the fixed row (`useRovingFocus` in `toolbar.tsx`) does not read the direction today: ArrowRight always moves to the next tool in the DOM, which on a right-to-left page is the one to the left. When it becomes the hook both bars use, it swaps the two arrows on a right-to-left page, and the fixed row gets that too | Arrows should move the way they point, in both bars |
| D8 | Co-editing: only one's own selection gets a bar. A change by someone else that moves the selection moves the bar, and a selection that disappears hides it | The carets of others are not ours to act on |
| D9 | The look follows `.selection-bar`: surface, line, radius and `--shadow-menu` tokens, the tool size of the row, and the `on` state of a tool in solid accent (EDT-004). Light and dark | One floating look for reader and writer |
| D10 | The design standard changes in two places: "There are no floating bars" (The editor) and "This is the only floating bar" (Working together). Blueprint lines EDT-006 and EDT-080 get their new words | ADR-038 |

### Plan

**A0. The dependency.** Add `@tiptap/extension-bubble-menu` at exactly `3.32.2` to `web/package.json`. The lock file may change only by listing it as a direct dependency: no other package may be added or change version. `make licenses` must pass. Renovate groups it with the other TipTap packages, because its peers ask for their exact version.

**A1. Placement, failure paths first.** A pure function `barPlacement(coarse, keepOut)` returns the options `BubbleMenu` passes to Floating UI: placement, flip, and a shift that keeps the bar inside the viewport below `keepOut` (the bottom of the sticky row). Unit tests first: a coarse pointer gets `bottom`; the flip is always there; the top edge of the shift is `keepOut`. Browser tests, because the measuring happens in Floating UI: no room above puts the bar below; the first line of a page never puts it over the row; at 320 px it stays inside both edges; a selection scrolled out of view hides it. Mutations: drop the flip, drop the shift, drop `keepOut`, place a coarse pointer on top.

**A2. When it shows.** A pure function `barFor(state)` returns `'text'`, `'image'` or nothing, following D4, and is what `BubbleMenu`'s `shouldShow` asks. Unit tests first for every case where it must not show (empty, code block, cell selection, other node, composing, dialog open), then the two where it does. Mutations: show in a code block; show on an empty selection.

**A3. The component.** `FloatingBars` in `components/editor/floating-bars.tsx`: two `BubbleMenu`s, one for text and one for an image, each holding a `div role="toolbar"` with the same `Tool`, `TextStyle`, list tools, `TextColorMenu`, `HighlightMenu` and `ImageTools` the row uses. Roving focus is factored out of `toolbar.tsx` into one hook for both bars, and learns the direction (D7). `onMouseDown` prevents the loss of the selection, as the reader bar does. The bar is styled by our tokens (D9), not by any style of the package.

**A4. Keyboard and screen readers.** Alt+F10, arrow keys (both directions, also on a right-to-left page), Escape. Browser tests: the bar is reached and left by keyboard only; axe passes with the bar open; a person who never presses Alt+F10 meets no extra Tab stop; the headings and lists in the bar report their pressed state.

**A5. Browser tests and pictures.** Select words, press Bold in the bar, and the row shows Bold on (one state). Turn a line into Heading 2 and into a numbered list from the bar. Select an image, change its size in the bar. No bar in a code block, in a table cell selection, or for a reader. RTL page. Touch emulation puts it below. At 320 px the text bar wraps to two lines. New pictures: `floating-text`, `floating-image`, `floating-text-dark`, `floating-rtl`, `floating-text-narrow`. Every picture changed by this wave is looked at before it is kept.

**A6. Words.** New strings in `en.json` for the bar names and Delete image. Design README and blueprint 03 lines updated. The changelog names the new dependency and its licence.

**Load budget (ARC-238):** none on the server. In the browser one placement per selection change, scroll or resize, done by Floating UI. The download grows by the bubble menu and Floating UI, a few tens of kilobytes before compression (estimate; the build measures the bundle before and after).

## Part B: near matches for titles and file names

### What exists

- **Pages.** `search_pages` (migration 0028) holds the title of every page with `words`, a `simple` tsvector of the title, and a btree on `lower(title)`. The tiers in `internal/search/words.go` are: the exact title (`exactTier`), every word in the title with the last word also as a prefix (`titleTier`, or `languageTitleTier` with search syntax), the words in the text (`textTier`), then pages found through a mentioned name. Within a tier the newest comes first. The quick box (`Query.Quick`) reads titles only.
- **Permissions.** Picking only narrows: `rowScope` keeps the spaces the person may view and `notHidden` drops hidden pages, and every shown row passes `perm_visible_pages` in `pickedRows`, at the moment of the search. A near match must go through exactly the same steps.
- **Files.** The quick box (`attachments.Find`) and the Files screen (`SpaceFiles`) both match the words of a name, split at `-_.()[]`, with the last word as a prefix (`fileWords`, `fileNameWords`, ADR-037), on the index `attachments_name_words_idx`. A label matches whole. The quick box looks at the newest 2,000 files when a word is common.
- **Spelling.** When nothing is found, `spelling()` compares the typed words with up to 5,000 titles the person may see, by edit distance in Go, and suggests one. It stays as it is in this wave.
- **The extension.** No migration creates an extension yet. Every image used, in `compose.yaml`, `compose.dev.yaml`, the CI services and the IdP test, is `postgres:16`, the official image, which ships the contrib modules, `pg_trgm` among them. Since PostgreSQL 13, `pg_trgm` is a trusted extension: a role with the CREATE right on the database can create it without being a superuser. The migrating role owns the tables and creates the database objects today, so it has that right on a Compose install.

### Decisions

| | Decision | Why |
|---|---|---|
| D11 | Migration `0043_trigram_names.sql`, phase expand: `CREATE EXTENSION IF NOT EXISTS pg_trgm`, a GIN index `gin_trgm_ops` on `lower(search_pages.title)`, and one on `lower(attachments.filename)` | Expand only: release N ignores the extension and both indexes, and a rollback to N keeps working (OPS-040) |
| D12 | The checks before migrating (OPS-038, wave O7) gain one: `pg_trgm` must be in `pg_available_extensions`, and the migrating role must be able to create it. A failure names both, in words an operator can act on, and migrates nothing. The install guide says that a managed PostgreSQL must allow `pg_trgm` | An install without contrib must fail before it changes anything, not halfway through |
| D13 | **Order for titles.** Quick box: the exact title, then the words and word starts (as today), then **titles that contain the typed text** (3 characters or more), then **titles near the typed words** (a typo, by `word_similarity`). Full search: the exact title, the words of the title, titles that contain the typed text, the words of the text, then titles near the typed words, then mentions. Within each tier, newest first, as ADR-035 asks. The near tier sorts by closeness first, then newest. **Decided by the product owner on 2026-11-10:** this order stands; the words of the text rank before titles with a typo | ADR-038: exact and word starts first. A real word in the text is a surer find than a title with a typo, so the typo tier stands after the text tier in a full search |
| D14 | **Order for file names.** The quick box and the Files screen use one function in `internal/attachments`: the words and word starts (as today), then names that contain the typed text, then names near the typed words. Labels stay exact. A name filter on the Files screen orders by tier, then newest, instead of newest alone | ADR-037: a name is found the same way everywhere |
| D15 | The near tiers run only for typed text of 3 to 64 characters, and only while the earlier tiers have not filled the page. A text of 1 or 2 characters has too few trigrams to mean anything, and one longer than 64 is not a name | Bounds the cost and keeps short typing quiet |
| D16 | The typo threshold is a named constant set per statement with `SET LOCAL pg_trgm.word_similarity_threshold`, starting at 0.5 and settled by a test set of real-looking titles and typos | The default of 0.6 may miss one wrong letter in a short word. The number comes from the test set, not from taste |
| D17 | Trigrams only pick candidate ids. Pages still go through `rowScope`, `notHidden` and `perm_visible_pages`; files keep their permission joins. Typed text only ever travels as an argument, and `%` and `_` in it are escaped before `LIKE` | A near match must never show a title or name the reader may not see, also right after a change |
| D18 | The search syntax (`title:`, quotes, wildcards, NOT) keeps its present meaning and gets no near tier. Near matches are for plain typing | An exact query asked for exactly that |

### Plan

**B1. Migration, failure first.** A test that a database without `pg_trgm` available is refused by the checks before migrating, with the reason named and nothing changed (fake the availability through the check's query, as wave O7's tests fake disk space). Then the migration, the upgrade test from `main` (OPS-036) and the rollback. Mutations: drop the new check; let it pass when the extension is missing.

**B2. Permissions first.** Before any ranking: a hidden page whose title is one letter from the typed word never appears, in the quick box or in full search; a page in a space the person may not view never appears; a page hidden a moment ago is gone at once; a file on a hidden page never appears on the Files screen or in the quick box. These run against the real database with the existing test helpers. Mutations: take `notHidden` out of a near tier; take the permission join out of the file query.

**B3. Contains.** The contains tier for titles and names, with `LIKE` on the trigram index and the typed text escaped. Tests: `port` finds "Quarterly report" and `report.pdf`; `%` and `_` typed are taken as letters; 2 characters do not use the tier. Mutation: drop the escaping.

**B4. Near.** The typo tier, with the test set of D16 in `internal/search/testdata`: at least 40 pairs of a title and a typo (a letter dropped, doubled, swapped, wrong) and 20 typed words that must find nothing. Tests: an exact title still stands first; a word start stands before a contains match, and both before a typo; within a tier the newest first; the threshold finds the pairs and leaves the decoys. Mutations: swap two tiers; drop the threshold; sort the near tier by date alone.

**B5. Files.** One function for the quick box and the Files screen (D14), so the two cannot drift. Tests on both routes with the same names. Mutation: change the order on one route only.

**B6. Measure.** The load script of spec 028 at 50,000 pages and 52,000 files: the model of 40 quick searches and 10 result pages a second, then four times that (ADR-036), each with typed text that fills no earlier tier, so every search reaches the near tiers. Results go in `docs/operations/limits.md` beside the existing rows.

**Load budget (ARC-238):** the two trigram indexes are a few MB at 50,000 titles and 52,000 names (estimate: trigram GIN indexes run at two to three times the size of the text). Writes: one more index entry per title change and per file name, on rows that already change. Reads: a near tier runs only when the earlier tiers leave room (D15), inside the search share of one connection in four and the 3 second statement time. Target, to be measured in B6: with the near tiers, the quick box stays at p95 under 50 ms in the database at the model on 2.5 CPUs, and the share of searches told "busy" at four times the model does not grow by more than a tenth of what spec 028 measured. If it does, the near tier for full search moves behind a pause in typing, and the measurement says so.

## Part C: the storage limit everywhere

### What exists

A space has `storage_limit_mib`, zero for none (migration 0016). `spaceBytesSQL` in `internal/attachments/limits.go` sums the size of every version of every active file in the space, and is the same number the Files screen shows (spec 024, D10). `requireRoom` locks the space row, so two writes take turns for the last room (spec 024, D11).

Every way bytes come into a space or move between spaces, and how the limit counts it since this wave:

| Way in | Code | Counted |
|---|---|---|
| Upload a file | `attachments.Upload` | Yes |
| A new version of a file | `attachments.go` (new version) | Yes |
| Copy a file to another page | `moves.go`, `Copy`, through `Upload` | Yes |
| Move a file to a page in another space | `moves.go`, `Move` | Yes |
| Import Markdown, HTML or a plan, and Word with its pictures | `exchange/plan.go`, `exchange/wordinto.go`, through `Upload` | Yes, file by file |
| The example space | `example.go`, through `Upload` | Yes |
| Copy a page or a branch, also a large copy in the background | `pages/tree.go` and `content.go`, through `attachments.CopyAll` | Yes: `CopyAll` checks before it stores anything; a large copy reports a page that does not fit |
| A new page from a template | `pages/templates.go`, through `CopyAll` | Yes, through `CopyAll` |
| Copy the files of a page into a template | `CopyFilesToTemplate`, through `CopyAll` | Yes, and checked before the template is made |
| Move a page or a branch to another space | `pages/move.go`, through `attachments.MoveToSpace` | Yes: every stored object of the branch, in the trash too |
| Import a bundle | `exchange/bundlein.go`, through `AddImported` | Nothing to count: a bundle always makes a new space, which has no limit; the upload limit per file holds |
| Make an older version current | `versions.go`, `MakeCurrent` | Needs no room: it stores nothing new, and each stored object counts once |
| The trash | Files in the trash (`status = 'trashed'`) | Yes, until they are purged |

### Decisions

| | Decision | Why |
|---|---|---|
| D19 | The count becomes the bytes a space really stores: every version of every file of the space, **in the trash too**, each stored object counted once (`DISTINCT object_key`). A purge frees the room; a restore needs none, because the file never left the count | ADR-038 names the trash. Counting once fixes `MakeCurrent`, which adds a version row that points at an object already stored |
| D20 | `requireRoom` gets one sibling, `requireRoomFor(tx, spaceID, bytes)`, used by every way in that the table marks No. Each way works out its bytes first: the current versions it copies, all versions of the files it moves, the files of a bundle. One refusal message for all, the one uploads use | One check, one message, one lock, so the ways cannot drift |
| D21 | A move of a branch to another space checks the room of the target for all files of the branch, in the transaction of the move, before anything changes. A move inside one space checks nothing | Nothing is added inside one space |
| D22 | A copy small enough to wait for checks the whole copy at once and refuses it whole. A large copy in the background (PAG-050) checks before it starts, and again per page, because the space can fill while it runs: a page that does not fit is reported in its result as not copied, with the reason, like any other page that fails | The background job already reports failures per page (`BulkFailure`) |
| D23 | A bundle import checks the total of its files from the bundle's own list before it stores any file, and again per file in the transaction | It refuses before writing gigabytes it will throw away |
| D24 | `CopyImpact` and the copy and move dialogs say before the action when it will not fit: "This needs 1.2 GB and the space has 300 MB left." No figure is shown to someone who may not see the target's Files screen | People find out before, not after |
| D25 | **Decided by the product owner on 2026-11-10:** no grace period. Spaces over their limit once the trash counts are not touched: nothing is deleted, and reading, editing and deleting keep working. Adding is refused at once until there is room, by emptying the trash or raising the limit, as today for uploads. The changelog's upgrade line says so, so an administrator can check the Files screens before upgrading | The count can rise when the trash starts to count. Refusing to add is the existing behaviour at the limit |

### Plan

**C1. The count, failure first.** Tests: a file in the trash counts; a purge frees its room; a restore over a full space succeeds because the room was never freed; `MakeCurrent` does not count twice; the Files screen total and the refusal agree. Mutations: drop `DISTINCT`; drop the trash from the count.

**C2. Each way in.** One test per No row of the table, each starting with a space that has 1 MiB left: the action is refused whole and leaves nothing behind (no rows, no stored objects, which the existing `TestAFullSpaceRefusesEveryWayInAndLeavesNothingBehind` already checks for uploads and is extended with these cases). Then the same action with room succeeds. Mutations: drop the check from each way, one mutation per way.

**C3. The background copy.** A test that fills the space while the job runs: the job stops adding files once full, reports the pages it did not copy, and leaves no half-copied page. Mutation: drop the per-page check.

**C4. Before the action.** `CopyImpact` and a new field on `MoveImpact` give the bytes needed and whether they fit, for someone who may see the target's files. Browser test of the dialog text. Mutation: report "fits" when it does not.

**C5. Two at once.** Two copies into a nearly full space at the same time: only one fits, and the other is refused. This is the lock of spec 024, D11, now used by every way in.

**Load budget (ARC-238):** one count per way in, as uploads pay today. The count reads the files of one space through the index on `space_id`; at 52,000 files in one space an estimate of a few milliseconds, to measure in B6's run. A large copy pays one count per page it copies; at the PAG-050 bound that is a few hundred counts over the life of a job, on the worker, not on a request. A running total kept in `spaces` would make the count free, and is left out until the measurement asks for it.

## What cannot be proven on one machine

- Four times the database work at the size of the model, on real servers: the big server of ADR-037 replaces the estimates in B6.
- A managed PostgreSQL that refuses `pg_trgm`. The check of D12 is tested by faking what the server answers.
- Touch screens: the browser tests emulate touch; a real phone and tablet are checked by hand.
- Screen readers: axe and keyboard tests run in CI; a real screen reader is checked by hand.

## Order of the build

A0 to A6, B1 to B6 and C1 to C5 can run in that order in one wave. Part C changes no screen except the two dialogs, Part B changes no screen except the order of results, and Part A changes no server code, so a failure in one part does not hold up the others. The gates are those of every wave: `make check`, `scripts/e2e`, the full mutation check, and a look at every changed picture.

## Questions for the product owner, and the answers

All five were decided by the product owner on 2026-11-10. The decision log records them in a later ADR.

1. **The rest of EDT-006:** should people also be able to hide the fixed row? **Decided:** no, the fixed row stays and cannot be hidden for now (D2). EDT-006's hiding stays open.
2. **What goes in the text bar:** **Decided:** bold, italic, link, inline code, text colour, highlight and Comment, plus Heading 1 to 3, bulleted list and numbered list (D3).
3. **Where a typo in a title ranks:** **Decided:** as planned. In a full search the words of the text rank before titles with a typo; in the quick box a typo comes right after word starts and contains (D13).
4. **Spaces over the limit when the trash starts to count:** **Decided:** no grace period. Adding is refused until there is room; nothing else changes (D25).
5. **Our own bar or TipTap's bubble menu:** **Decided:** TipTap's `BubbleMenu`, as a new direct dependency approved in principle (D1). It and the packages it pulls in are MIT, at the version of the installed TipTap packages. What it does not do, keyboard access, names, right-to-left arrows and the touch placement, we add around it (D5 to D7).

## What was built

Built in wave ES on 2026-11-10. Where the build differs from the plan above, this section counts.

**Part A.** `components/editor/floating-bars.tsx` holds two `BubbleMenu`s, one for words and one for an image, each with a `role="toolbar"` inside. `barFor` decides when a bar shows (D4) and `barPlacement` gives the options for Floating UI (D5). Two things differ from the plan:
- **Placement is `absolute`, not `fixed`.** A fixed bar was moved off by an ancestor of the editor that makes its own containing block, so the bar is placed from its real offset parent.
- **The sticky row is measured when the bar is about to show.** BubbleMenu reads the padding each time it places the bar, so the one padding object both flip and shift share is set to the bottom of the row at that moment (`keepClearOfRow`).

BubbleMenu gives the bar the width of its content. The tools sit in a box at most as wide as the window less 16 pixels, so on a narrow screen they wrap. The heading tools show the heading icon and a small number, so what is seen is part of the name ("Heading 1"). Roving focus moved to `roving-focus.ts`, which the fixed row now uses too, and its arrows follow the reading direction. Alt+F10 is caught on the document in the capture phase, only while the editor has focus.

**Part B.** As planned, with one number settled by data: the typo threshold is **0.4**, not 0.5. On the 45 typos and 26 decoys in `internal/search/testdata/typos.txt`, 0.4 finds 42 typos and no decoy comes above 0.25; 0.5 found 37. The three typos it leaves out swap letters at the start of a short word, where the spelling suggestion still helps. **Typos are looked for from five letters, not three** (D15 changed): at 0.4, four typed letters that share only their first two with a word already count as near, so "week" found "Welcome pack.pdf". Every typo in the test set has five letters or more. Names that hold the typed text are still found from three letters. The check before migrating reads `pg_available_extensions`, whether the extension is already there, the right to create objects in the database and whether the role is a superuser.

**Part C.** As planned. A bundle import needs no check: a bundle always makes a new space, and a new space has no limit. The dialogs get `fits` only, no figures, so a dialog never tells how much a space holds. Copy checks the page itself; the copy checks again for every page it copies.

**Query budget.** A search whose words leave room on the page costs two more queries, one per near tier, so the budget of search in `query_budget_test.go` goes from 17 to 19 (ARC-238, ARC-239).

**Measured.** See `docs/operations/limits.md`, "Part of a word and small typos". The full load model of B6 was not run; it waits for the stress server.

**Not proven.** A real touch screen: the test browser is started with a fine pointer, so touch placement is proven by a unit test only. A real screen reader. A managed PostgreSQL without `pg_trgm`. B6 at the model on 2.5 CPUs.