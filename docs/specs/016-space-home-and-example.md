# 016: The home of a space, and an example space

Status: built, waiting for the product owner's acceptance session
Phase: asked for by the product owner on 2026-11-05, between the waves. It changes a frame, which is their decision (ADR-027).

## Problem

Two things make a first visit harder than it should be.

**A person cannot see that they are on the home page.** The home page is the first row of the tree, with the same name as the space, and everything else hangs below it. It looks like any other page. The product owner wants the home page to be clearly the home page, above the tree and not part of it, with a strip above that for moving between spaces.

**A new instance is empty.** A new person sees one sentence and has to imagine what a wiki could be. The product owner wants the very first space to hold everything that is possible, as a first introduction.

## What the study found

**How people expect the home of a space to work.** In the wikis studied, a space has a fixed entry for its landing page at the top of the sidebar, separate from the page tree. Below it come shortcuts, then the tree of the pages under the landing page. People look for the landing page in the fixed entry, not in the tree. The help forums show the risk in the other direction: where the landing page hides the tree, people ask where their pages went. So the tree must stay in view on the home page, which ours does.

**Why empty hurts.** Guides on starting a wiki agree that an empty wiki stops people from starting, and that seeded pages which invite editing are the usual answer. The seeded page says it is an example, and that it may be changed or thrown away.

**What accessibility demands.**

| Source | Demand | What we do |
|---|---|---|
| WCAG 1.4.10 Reflow | A strip that scrolls sideways is fine when it scrolls on its own and the page does not. | Only the strip scrolls, in one direction. |
| axe `scrollable-region-focusable` | A keyboard user must be able to reach a scrolling region. | The strip holds links, so Tab reaches and scrolls it. |
| 2.4.7, 2.4.11 Focus | A focused link may not stay out of sight. | A browser scrolls a focused link into view inside the strip. |
| 2.5.8 Target size | At least 24 by 24 CSS pixels. | Each entry is as high as a control. |
| 1.4.1 Use of colour | Colour alone may not carry meaning. | `aria-current` and a heavier weight mark the current space and the Home row. |
| 4.1.2 Name | Every link has a name. | The name of the space is the visible text. |

**What the code allows.** `buildTree` already treats a page whose parent is not in the list as a root, so leaving the home page out of the list shows its children at the top. Rows at the top can now have two real parents, the home page or none, and each has its own positions. A drop in a gap must use the real parent of the row next to it, or the server refuses it because the rows are not siblings.

`user create-admin` runs in its own process, often beside a running server, and the embedded queue cannot be opened twice. The server's worker runs periodic tasks with a lease, so that is the safe place for the first run.

**Safety.** The example space is made through the public services with a real caller, so permissions, audit and outbox all apply. Its files go through the normal upload, and its documents through the normal gate. No outside address is fetched. A link to an outside site only shows as a link.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | The home page leaves the tree, the flat views and the filter, and becomes a fixed **Home** row | It is what the product owner asked for, and where people look. |
| D2 | Overview stays, with its own icon | They answer different questions: the page that welcomes you, and the facts about a space. |
| D3 | The strip shows a badge and a short name | Two spaces can share a first letter, and an overview needs names. |
| D4 | Strip order: favourites, recent, then the rest, as in the Spaces menu | One order everywhere. The menu and the strip share the code. |
| D5 | The current space is always in the strip, also when it is archived | A person must see where they are. |
| D6 | Rows at the top: children of the home page first, then pages without a parent | Each group keeps its own order, so nothing jumps. |
| D7 | The worker makes the example space, within seconds of the first administrator | The command line process cannot run the whole application safely beside a server. |
| D8 | It is made once. A mark in the settings remembers that, also when nothing was made | Deleting it must not bring it back, and an instance with spaces gets none. |
| D9 | An administrator can add it later, with a button in Settings | Instances that exist today, such as the product owner's, have spaces already. |
| D10 | A setting turns it off: `DIFFLUENCE_EXAMPLE_SPACE=false` | Some installations want to start empty, and the picture tests need an empty database. |
| D11 | All members can edit it | It is for trying things out. |
| D12 | One page has an edit restriction, not a view restriction | Everyone sees the lock and what it does, and nobody misses a page. |
| D13 | Each page has two versions: a short first one, and the full one | So History and Compare have something to show. |
| D14 | Content is plain JSON in `internal/example/content`, with a list of pages, and placeholders for files and links | A later wave adds a page by adding a file and one line. |
| D15 | A test fails when the document format gains something the example does not show | "Everything that is possible" would otherwise go stale without anyone noticing. |

## Scope

**In:** the strip, the Home row, the home page out of the tree, breadcrumbs that say Home, the example space and its button. New requirements: SPC-082 to SPC-086 and NAV-083 to NAV-087.

**Out, and where it goes:**

| Item | Goes to |
|---|---|
| Comments, reactions, a mention and a task with a person in the example | Wave C adds them |
| Labels, a blog post, a page status, a template in the example | Wave D adds them |
| Dynamic blocks in the example | Wave E adds them |
| A guided tour on top of the example | Wave F, with NAV-070 |
| Translating the example | Later: English is the only language so far |

## Feature plan

### H1. The strip of spaces

At the very top of the sidebar, above the name of the space.

- **Shows:** a chip per space, with its badge and name. The name is cut off with dots past a fixed width, and `title` shows it in full.
- **Order:** see D4. A link at the far end goes to All spaces.
- **Current:** a tint, a heavier weight and `aria-current="true"`. It is scrolled into view when the sidebar opens, without moving anything else.
- **Mouse:** a thin scrollbar shows when the chips do not fit, and the wheel scrolls sideways over the strip.
- **Keyboard:** Tab goes through the links, and a browser keeps the focused one in view.
- **Accessibility:** a `nav` named Your spaces, with a list.
- **Right-to-left:** the strip starts on the other side, by itself.
- **Edge cases:** a person with one space sees one chip and the link. Loading shows the current space only, so the sidebar does not jump. A failed request does the same, because the strip is not worth a message.

### H2. The Home row

- **Place:** right below the name of the space, above Overview and the shortcuts.
- **Shows:** a house and the word Home, whatever the page is called.
- **On the home page:** the row has the tint of a current item and `aria-current="page"`.
- **Breadcrumbs on the home page:** the space, then **Home**, marked as the current step.
- **Edge cases:** a space without a home page has no row.

### H3. The tree without the home page

- **Rows:** the home page is gone from the tree, the flat views and the filter. Its children are the top rows.
- **Show the current page:** this button is absent on the home page, because it has no row.
- **The add button by Pages:** on a screen without a page it adds below the home page, so a new page lands in the tree at the top.
- **Dragging:** a drop in a gap uses the real parent of the row next to it. Dropping a row on another row works as before.
- **Move dialog:** the home page stays a choice there, under its own title.

### H4. The example space

**When:** see D7 and D8. The task looks every 10 seconds until it has made a decision, and then only reads one setting.

**Who owns it:** the first active administrator who may use the product. The audit log shows the system as the actor, and the administrator as its owner.

**What it holds:**

| Page | Shows |
|---|---|
| Home | A welcome, what a space is, links to the rest, and a note that all of it may be changed or deleted |
| Start here | The sidebar, search, shortcuts, favourites, a task list to try |
| Writing a page | Headings 1 to 6, marks, colours, highlights, sub and superscript, alignment, indent, links, lists, quotes, rules |
| Blocks | Panels of all five kinds, collapsible sections, columns, decisions, status marks, dates, emoji |
| Tables | Merged cells, header row and column, cell colours, column widths |
| Images and files | Image sizes, alignment and captions, a file tile, a header image on the page itself |
| Code | Language, line numbers, a title, wrapping |
| A wide page | Full width and a title in the centre |
| A long read | Enough headings for the contents beside the page, heading links and the read time |
| Only the owner edits this | An edit restriction, and what a lock means |

Shortcuts in the sidebar: one to a page, and one to the project's source. The tree has two levels, and links between pages feed Details.

**Words:** own text, written for this product. It explains Diffluence by doing what it describes. Nothing comes from a sample space of another product.

### H5. The button in Administration

Settings has an **Example space** section with a line of explanation and an **Add example space** button. It makes the same space with the administrator as owner, and goes there. If the key is taken, it says so.

## API

| Route | Rule | What |
|---|---|---|
| `POST /admin/example-space` | permission `administrator` | Make the example space |

## Data

No migration. The mark is a row in `settings`.

## Build order

1. Spec, ADR-027.
2. Go: the check that the example is complete, the package, the task, the route.
3. Frontend: tree, Home row, breadcrumbs, strip, button.
4. The content, with its images.
5. Pictures, docs, status lines, handoff.

## Test plan

- **Go unit:** every document of the example passes the gate, and together they use every node type and mark, every panel kind and every text colour.
- **Go integration:** the task waits for an administrator, makes the space once, leaves an instance with spaces alone, does not return after a delete, and does nothing when turned off. A member can read and edit, but not the restricted page. Each page has two versions. The route refuses a member and a second attempt.
- **Mutation:** the setting is ignored, the space comes back, any member can add it, an instance with spaces gets one.
- **Vitest:** order of the top rows, order of the strip.
- **Browser:** the Home row and breadcrumbs, no home page in the tree, a drop in a top gap, the strip with many spaces, by keyboard, the example through the button, with every page opened and axe on each.
- **Pictures:** every screen in a space changes. Look at them all, with a short and a long strip.

## Acceptance criteria

- [x] AC-1 On the home page the Home row is marked, and the breadcrumbs say Home.
- [x] AC-2 The home page is not in the tree, a flat view or the filter, and its children are the top rows.
- [x] AC-3 A drop in a gap between top rows works, whichever parent the rows have.
- [x] AC-4 The strip lists spaces in the order of the Spaces menu, marks the current one, and scrolls on its own.
- [x] AC-5 The strip works from the keyboard, and passes axe.
- [x] AC-6 A new instance gets the example space shortly after its first administrator, once.
- [x] AC-7 An instance with spaces gets none by itself, a deleted example stays gone, and the setting turns it off.
- [x] AC-8 An administrator can add it from Settings, and no one else can.
- [x] AC-9 The example uses every block and mark the format has, and a test keeps it that way.
- [x] AC-10 Every page of it opens without a script or policy error, and passes axe.
- [x] AC-11 All changed pictures have been looked at.

## Risks

| Risk | Answer |
|---|---|
| A new block makes the completeness test fail for someone who did not expect it | That is the point: the message says which file to change |
| Creating the example fails halfway | It is many small steps. A failure is logged, and the space that exists stops a second attempt. It can be deleted, and added with the button |
| The strip takes height on small screens | One row, and the sidebar starts hidden there |
| Older installations keep a page tree that people knew | The children simply move up one level |

## How the product owner tests it

1. Open a space. Home is marked, and the tree starts with the pages below it.
2. Open a page, and press Home. The breadcrumbs say Home.
3. Make enough spaces to fill the strip. Scroll it with the wheel, and Tab through it.
4. Drag a row between two top rows.
5. Go to Administration, Settings, and press Add example space.
6. Read through it, and try the task list on Start here.
7. On a new installation: create the administrator, log in, and the example is there.

## Provenance

Own text from these public sources, read for behaviour only:

- W3C WCAG 2.2 Understanding 1.4.10 Reflow.
- Deque axe rule `scrollable-region-focusable`.
- Community and support pages about the sidebar and home page of a commercial wiki: where people look for the landing page, and the confusion when the tree hides.
- ONES guide on seeding a new wiki, and the MeatballWiki page SeedPosting: why an empty wiki stops people.
