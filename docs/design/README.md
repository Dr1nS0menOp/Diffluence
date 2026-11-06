# Design standard

This is how Diffluence looks and behaves, and how that stays true for every future change. It binds every build session: `CLAUDE.md` makes it a hard rule, and `make check` and the browser tests enforce most of it.

Status: agreed with the product owner, 2026-11-04 (ADR-021).

## The decision

After ten designs, three iterations and a final choice, the product owner decided:

| Part | Decision |
| --- | --- |
| Style and layout | **Helder**: calm, white and blue, a clear top bar, a sidebar per space, one centred reading column that scales with the window (ADR-030). |
| Editor | **Rail**: a strip with the page's fate at the top, the tools in a row below it, and an open sheet with no boxes. |
| Page tree | **Branches**: guide lines that show the structure, and every branch can be collapsed. |
| Font | The **system font**. A font can be imported in the settings. |

Rail kept the global navigation visible while editing. In Helder that means: **the top bar stays**. After his first test the product owner asked to keep the pages in view as well, so **the sidebar stays too**, as far as there is room (ADR-030).

The intuition of a familiar wiki is the aim, never a copy. The clean-room rules (`docs/blueprint/02`) apply to design as well: no logos, icons, copy or pixel-exact layouts from any other product. The logo and every icon are ours.

## How the standard holds

Words alone do not keep a design together, so the rules are checks. A failing check blocks the change.

| Rule | Enforced by |
| --- | --- |
| Colours, font sizes, families, weights, line heights, corners, shadows, layers and spacing come from `web/src/design/tokens.css` | `npm run design` (in `make check`) |
| Every token exists, and every token is used | `npm run design` |
| Text and controls have enough contrast, in light and in dark (WCAG AA) | `npm run design`, on the pairs in `check-design.mjs` |
| Layout is right-to-left ready: logical properties only | `npm run design` |
| No inline styles, no `!important`, no style sheets outside `web/src/design/` | `npm run design` |
| Every class in the code has a style | `npm run design` |
| No emoji or glyphs as icons; `<svg>` only in `components/icons.tsx` | `npm run design` |
| No `window.confirm`, `prompt` or `alert` | `npm run design`, and a browser test |
| Every screen has an approved picture | `npm run design`: each `…View` needs a test named `…View:` in `web/e2e/0-design.spec.ts` |
| Screens still look as approved, in light, dark, narrow and right-to-left | `web/e2e/0-design.spec.ts` against `web/e2e/design-baselines/` |
| Accessibility | axe in the browser tests |
| All words come from the translation files | `npm run strings` |

`web/scripts/check-design.test.mjs` proves that each rule fails on a violation.

**What no check can see:** whether the right component was chosen, whether a new screen feels like the others, and whether the words are good. That is what the checklist below is for. A picture only guards the state it shows: the first reading column looked fine with long text and collapsed with a short one. Look at a new screen with very little content as well as with a lot.

## Tokens

`web/src/design/tokens.css` is the only place with design values. The groups:

| Group | Prefix | Use |
| --- | --- | --- |
| Type | `--font-*`, `--text-*`, `--weight-*`, `--leading-*` | Interface text is `--text-ui`, documents `--text-prose` |
| Space | `--space-*` | All margins, paddings and gaps |
| Fixed sizes | `--size-*`, `--tree-*` | Controls, bars, columns |
| Corners, depth | `--radius*`, `--shadow-*`, `--layer-*`, `--motion-*` | 4px corners, 6px on cards and dialogs |
| Colour | `--color-*` | Both themes define the same names |

The colours that carry the look:

| Token | Light | Use |
| --- | --- | --- |
| `--color-ink` | `#1E2A3B` | Text |
| `--color-strong` | `#131C2B` | Headings |
| `--color-muted` | `#566278` | Secondary text |
| `--color-accent` | `#1D5FBF` | Links, primary buttons, the current item |
| `--color-accent-tint` | `#E4EDFA` | Background of the current item |
| `--color-side` | `#F5F7FA` | Sidebar, toolbar |
| `--color-line` | `#DDE2EA` | Dividers |
| `--color-guide` | `#B9C6D8` | Branches of the tree |
| `--color-lozenge` | `#FBEFD5` | Status marks such as Restricted |
| `--color-text-*` | 12 names | What a writer can choose for text. Each passes 4.5:1 on the page, in both themes |
| `--color-mark-*` | 6 names | Highlights, panels, status marks and table cells. Normal text on each passes 4.5:1 |

Need a value that is not there? Use the nearest token. If none fits, add one to `tokens.css` for both themes, and add a contrast pair if it carries text.

## The frame of a screen

Every screen is one of these. Do not invent a fifth.

| Frame | Used for | Built from |
| --- | --- | --- |
| **Plain** | Start screen, search, your account | Top bar, `Plain` in `app.tsx` (wide, or `reading`) |
| **Space** | Everything inside a space | Top bar, `Frame` or `SpaceShell` in `components/frame.tsx`: sidebar and a centred `.column` |
| **Admin** | The admin console | Top bar, sidebar with `.side-link` items, `.column.wide` |
| **Focus** | The editor | Top bar, sidebar, `main.focus`: `SpaceShell` with `focus`, and no reading column around it |

The login page stands alone: one card on the sidebar colour.

**Top bar:** logo and name, Home, Spaces, Recent, Favourites, Create, then search, the bell, help and the account menu on the far side. Create adds a page in the current space, or a space when there is none, and a small menu offers both. The search box is a combobox with one panel below it, and a chip shows when it stays inside a space.

**A page:** breadcrumbs with Edit, Share, star, lock and More on one line; the title; the byline; the document; child pages; reactions; comments. It fills the reading column, however short the text is. Files sit in the document as tiles, and a page has no upload (ADR-022). Rarely used actions live in More, never as a row of buttons. A header image sits above all of it. The contents list stands beside the column when there is room, and the column keeps its width. The column scales: `--size-page` is at least `--size-reading`, 64 percent of the room, and at most `--size-reading-max`. Full width takes all the room. Details is one panel on the far side, on top of the page, and does not block reading. Comments use the same place, and only one of the two is open at a time. On a narrow screen comments become a sheet from the bottom, so the words stay above it.

## Components

Use these. Reuse beats a new variant.

| Need | Use |
| --- | --- |
| Main action, one per view | `<button className="primary">` |
| Normal action | `<button>` or `<a className="button">` |
| Destructive action | `className="danger"`, and ask first |
| Quiet action next to a primary one | `className="quiet"` |
| Action inside a table or list | `className="ghost"` |
| Icon only | `className="icon"` with `aria-label` and `title` |
| Asking before something hard to undo | `ConfirmButton`, or `ConfirmDialog` from a menu |
| A form or detail on top of a screen | `Dialog`, with buttons in `.dialog-actions` |
| A list of less used actions | `Menu`, `MenuItem`, `MenuLink`, `MenuDownload`, `MenuSeparator` |
| A labelled input | `Field` |
| Choosing a file outside the editor | `FilePicker` |
| A file in a document | `.file-tile`, through the `file` block in `components/editor.tsx` |
| A person | `Avatar` |
| A space | `SpaceBadge`, which shows its animal |
| Choosing the icon of a space | `AnimalChoice` |
| A person or group in a form | `SubjectPicker`, which shows people on focus |
| A comparison of versions | `DiffDocument` |
| A status | `.lozenge` |
| A person | `PersonName` for a name on someone's work, `PersonPicture`, and `PersonMark` for the words Guest and No longer active |
| Something the whole instance says | `.site-note` under the top bar, a static region. Never a dialog that opens by itself |
| A new password | `NewPasswordField`, with Show password. Paste always works |
| Tabs inside a screen | `Tabs`, with arrow keys |
| Sections of Your account | `AccountNav`, which holds real addresses |
| Numbers on a summary | `.tiles` |
| A block that fills itself | `.dynamic`, with `.dynamic-head` only while editing. Empty is one muted sentence |
| A chart | `Chart` in `components/chart.tsx`. Series differ by shape as well as colour, and the numbers are one press away |
| Something drawn | Only `components/icons.tsx`, `components/chart.tsx` and `components/drawing.tsx` may hold SVG (ADR-026) |
| A message in the flow | `.note`, `.note-error`, `ErrorNote` |
| A short message, maybe with Undo | `useToast`, and `useCopy` for copying |
| A time | `RelativeTime` |
| Switching between lists | `Tabs`, which brings the arrow keys |
| Writing a comment | `CommentBox` |
| Counting responses | `Reactions`, for a page or a comment |
| A menu in something that scrolls | `Menu` with `floating` |
| A page in a list | `PageMark`, which shows its emoji |

## The page tree

- A branch has a chevron and can be collapsed; a page without children has a page icon.
- Guide lines show the branches. They are the signature of this design: keep them.
- The two levels nearest the top start open, so the shape of a space shows at once.
- What a reader collapses stays collapsed, per space, across pages and reloads.
- Going to a page reveals it, also when it sits in a collapsed branch.
- The current page has the tint, and shows the add button. Other rows show it on hover or focus.
- The sidebar itself can be put away, and starts hidden on narrow screens. Its width is a preference.
- The sidebar from top to bottom: the strip of spaces, the name of the space, Home, Overview, People, the shortcuts of the space, then a filter, a choice of view and the tree. The flat views have no guide lines, and say that the tree is where pages are arranged.
- The home page is not a row of the tree. It has the fixed Home row, which is marked while it is open, and its children are the top rows. On the home page the breadcrumbs end in Home.
- The strip of spaces scrolls sideways on its own, in the order of the Spaces menu, with the current space marked.
- The tree is a `nav` with links and the keys of the tree pattern, so it is one Tab stop plus the buttons of the focused row.

## The editor

- Top strip: Close on the left with the save status beside it; on the right the summary, Discard draft and Publish.
- Below it one row of tools: text style, marks, More formatting, colour, highlight; alignment and indent; lists and blocks; link, image or file, table, Insert, Find; undo and redo. It is one Tab stop: arrow keys move inside it, Escape returns to the text.
- The tools for what is selected appear at the end of the same row: image, panel, section, columns, status mark, date, code block, table. There are no floating bars. Text fields come last, because they need the arrow keys themselves.
- `/` opens a list at the cursor, and Insert opens the same list in a dialog.
- A block has a grip and a menu in the margin, outside the text. Everything the grip does is also in the menu and on the keyboard.
- A file can be dropped anywhere below the strip. The sheet shows a dashed outline while it is above the editor.
- An active tool is solid accent, so its state is clear without reading it.
- The sheet has no border, and scales like a page. The title is a large plain input.
- Above the title: Add emoji, Add header image and Page options, quiet until hover or focus, and always there on touch screens.
- A table has one button on its outline, with one menu for everything about tables, and nothing for tables appears in the row. It has an outline in `--color-guide` and a grid in `--color-line`, and the table with the cursor has an accent outline. Its end edge stretches it.
- Both bars stay visible while scrolling, except on narrow screens.

## The blocks of a document

`web/src/design/blocks.css` holds them. A writer picks names, never values: a document can only say `red`, and each name has a light and a dark value in the tokens. Meaning never rests on colour alone: a panel has an icon and a name, a status mark has words, a match in Find has an outline.

| Block | Look |
|---|---|
| Panel | A tinted box with an icon: info blue, note purple, success green, warning yellow, error red |
| Collapsible section | A thin frame. Readers get the browser's own `details`. Writers see it open, with its title as a field |
| Columns | No frame for readers, a dashed outline for writers. They stack on narrow screens |
| Decision | A green line on the start side, with an icon |
| Status mark, date | A soft box in the line of text |
| Code block | A header with title, language and Copy for readers. Always left-to-right |
| Image | A `figure` with a caption, in three sizes or its own |

## Changes between versions

A comparison looks like the page. What was added sits on green, what was removed on red and struck through, and new words are underlined. Each changed block has the word Added, Removed or Changed beside it, so colour never stands alone. Words can be marked in paragraphs and headings. Other blocks show as they were and as they are.
## What organises a page

- **Status** is always a word and a colour, in the style of a status mark in a document. It sits at the end of the byline.
- **Verified and Approved** have an icon and a word.
- **Labels** sit at the bottom of a page, as small quiet tags. Each has its own remove button, of at least 24 pixels.
- **Notes on a page**, such as archived, scheduled and approval, use `.note` under the byline. Past review is a muted line, for editors only.
- **The publish arrow** sits beside Publish, in the same colour, with a small gap.
- **Fields and instruction text** in a template: a field has a dashed border, instruction text is muted and italic.
- **Selected rows** have the accent tint and a real checkbox, so state never rests on colour.
- A restricted item in the trash shows the words "A restricted page", with no title.

## Working together

`web/src/design/together.css` holds comments, reactions, mentions, the bell and the feed.

- A comment on words is a soft tint with an underline, so it does not rest on colour alone. It is a `button`, and its name says who wrote what.
- A selection in a page shows one small floating row. This is the only floating bar, because a reader has no row of tools.
- A person mentioned is a soft box with `@`. An overdue task date is red and says "overdue" in words.
- The bell carries a number capped at 99+. A hidden `role="status"` line only changes when the number goes up.
- Six common reactions come from `components/comments/common-reactions.json`. They are content, not icons.

## States

Every screen handles all of these:

| State | How |
| --- | --- |
| Loading | `Loading` |
| Failed | `ErrorNote`, or `.note-error` with a clear sentence |
| Empty | A muted sentence that says what will appear here, not a blank |
| No permission | The control is absent, not disabled. Pages say "does not exist, or you cannot see it" |
| Busy | The button is disabled while the action runs |

## Icons and logo

`web/src/components/icons.tsx` is the only file that draws. Icons are our own line drawings on a 24 by 24 grid, stroke 1.8, round ends, `currentColor`, hidden from screen readers. To add one, add a line to that file. An icon needs words beside it, or an `aria-label` and `title` on its button.

The logo is a river that splits in two, which is what the word diffluence means. User marks use four tints, chosen by the name, so they never change by themselves.

A space has a cartoon animal (ADR-031), picked from its key until someone chooses, and it can choose a character instead, which uses the tints. The animals are pictures from `web/public/animals/`, the one exception to our own line drawings, because they carry character and not meaning. They are always next to the name of the space.

## Words

UI text is English, in sentence case, through `web/src/locales/en.json`. Name things plainly: "Discard draft", not "Abort". Errors say what happened and what to do. Buttons say what they do; a confirmation repeats the action on its button instead of saying OK.

## Accessibility, dark mode, right-to-left, small screens

- WCAG 2.2 AA is the goal. Everything works from the keyboard, focus is always visible, and menus follow arrow keys and Escape.
- Dark mode follows the operating system, or a person's choice in Preferences. A token holds both colours in `light-dark()`, so nothing needs a dark variant of its own.
- Only logical properties, so a right-to-left language needs no new styles. Icons with a direction are mirrored in `layout.css`. Code stays left-to-right.
- Below 50rem the top bar wraps, the sticky bars let go, and the sidebar starts hidden.
- Motion is short and stops with `prefers-reduced-motion`.

## Fonts

The default is the system font: nothing to ship, license or download, and it fits each person's device. An administrator can import a font under **Administration, Settings, Appearance** (spec 006). `web/src/appearance.ts` is the only code that applies it, by replacing `--font-sans`. The system stack stays behind it, so a font that fails to load changes nothing. The licence of an imported font is the responsibility of whoever imports it.

## Adding or changing a screen

1. Pick one of the four frames, and look at the nearest existing screen.
2. Use the components above. Need something new? Add it once in `components/` and to the table in this document.
3. Style with classes in `web/src/design/`, using tokens only.
4. Handle every state in the table above. All words go through `en.json`.
5. Add a test named `YourView: …` to `web/e2e/0-design.spec.ts`, with stable data, and mask what changes by itself such as dates.
6. Run `scripts/e2e design.spec.ts --update-snapshots`, then **look at each new picture**: compare it with the screens around it, also in dark and narrow if it matters.
7. Run `scripts/dev make check` and `scripts/e2e`.
8. Commit the pictures with the change. In the pull request, say which pictures changed and why.

Never update a picture just to get past a failure. A failing picture means the screen changed: either that was intended and you can say why, or it is a bug.

## Changing the standard

The look is the product owner's decision. Changing a token value, a frame or a rule in this document needs his agreement and a new ADR. Ask him with the before and after pictures.
