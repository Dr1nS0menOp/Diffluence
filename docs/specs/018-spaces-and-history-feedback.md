# 018: Animals for spaces, changes in the page, and people in pickers

Status: built, waiting for the product owner's acceptance session
Phase: The product owner's feedback after their first test of waves A and B, 2026-11-05. The editor half of their list is spec 017.

## Problem

The product owner tested the product and asked for four things:

1. Every space gets a fun animal: cartoonish, with character, and preferably no standard emoji.
2. History shows in green what was added or changed, and in red what was removed. Today a comparison is a list of plain text lines that looks nothing like the page.
3. A mail about a change to a watched page must show those changes too.
4. Standing in a field for restrictions must show people at once. Today nothing shows until a person types.

## What the study found

**The animals.** Kenney's *Animal Pack Remastered* holds 30 animals, each in eight styles, with real character: a moose with antlers, a narwhal, a sloth. Its `License.txt` says Creative Commons Zero, and the asset page says the same. Credit is welcome and not required. The round PNG style has 30 files of about 4 KB, 115 KB in all, at 128 pixels or more. That is sharp at our largest badge of 40 pixels on a high-density screen. The vector files are one sheet per style, with no file per animal, so PNG is simpler and smaller here. We tested no other set: this one meets every wish, and the licence is the most open there is.

**Where the files live.** Vite inlines files under 4 KB into the JavaScript, which would add about 160 KB to every page load. Files in `web/public/` are copied as they are, served by the existing static handler, and only load when shown. The policy already allows `img-src 'self'`.

**Colour alone is not enough.** WCAG 1.4.1 says colour may not be the only visual means of showing information. Technique G14 makes it available in text, and G182 adds a visual cue. So a change gets a word as well as a colour, and removed text is struck through.

**The right elements.** `ins` and `del` carry the meaning in the markup, and `del` has the role `deletion`. MDN gives no promise of how screen readers announce them, which varies, so we do not lean on them alone: every changed block has a label in real text.

**Showing changes in the document.** The editor renders a document through ProseMirror, which offers decorations: an inline one can use another element, such as `ins`, a node one can add a class to a block, and a widget can place an element at a position. So a comparison can be the real page with marks on top, and no document changes. Words can only be marked inside plain text blocks. Lists, tables and panels are shown as removed and then added as a whole.

**Mail.** Mail clients ignore style sheets in many cases, so a mail needs inline styles and its own fixed colours. Every piece of text in it comes from a page, so it must be escaped. Whether a mail may hold text at all is a setting of wave C (COL-076).

**Pickers.** The combobox examples open from keys, and the text does not rule out opening on focus. Spec 009 already opens quick search on focus, and the product owner asks for this here. The rest of the pattern stays: focus stays in the field, arrows move and wrap, Enter picks, Escape closes. An empty search on `/directory` already gives the first people and groups, and it needs *view profiles*.

## Decisions

| # | Decision | Why |
|---|---|---|
| D1 | The round PNGs of Kenney's pack, in `web/public/animals/` | CC0, 115 KB, with character. They load only when shown |
| D2 | The animal is stored in the existing `icon` field by its name. An empty icon means it follows from the key | No migration, so existing spaces get an animal at once and the change to shared files stays small |
| D3 | A character stays possible: any other text in `icon` shows as it did | The product owner allowed it, and existing spaces with an emoji keep their look |
| D4 | The key picks through a small hash, not the sum of its letters | Keys that look alike should not get the same animal |
| D5 | A comparison sends the blocks themselves as well as their text | The page can then show the real thing |
| D6 | Words are marked in paragraphs and headings. Other blocks show as removed and added | Mapping words into a table or a list would be fragile |
| D7 | If the words do not fit the text, a block falls back to removed and added | Inline marks such as a status have no plain text place. An honest block beats a wrong mark |
| D8 | Opening a version shows its changes against the version before it, with a button for the clean page | That is what the product owner asked for. Version 1 has nothing to compare with |
| D9 | `internal/document` makes the text and HTML for mail, and `internal/notify` compares when the mail is sent | Access is checked at that moment, so a person who lost it gets nothing |
| D10 | Pickers open on focus | The product owner's request, for every picker alike |

## Scope

**In:** SPC-010, PAG-090, PAG-092 as far as blocks go, and new lines below.

**Out:** an uploaded picture as icon (SPC-011) stays new. Comparing per table cell (PAG-092) stays open. Sending the mail itself is wave C.

## Feature plan

### F1. Animals

- **Showing:** `SpaceBadge` shows the animal. It is a picture without words, because the name is always beside it.
- **Choosing:** Space settings and the New space dialog have a grid of the 30 animals, with their names for screen readers, and one choice "From the key". A field below takes a character instead.
- **Keyboard:** the grid is a row of buttons with `aria-pressed`.
- **Dark mode:** the pictures have their own colours and a dark outline, and work on both themes.
- **Edge cases:** a name that is not an animal shows as a character. A key with only non-Latin letters still gets an animal.

### F2. Changes in the page

- **Comparing:** the page itself, with what was added on green, what was removed on red and struck through, and a label beside each block: Added, Removed or Changed.
- **Words:** in a changed paragraph or heading the new words are `ins` and the old ones `del`, in place.
- **Long pages:** long runs without changes still fold, as spec 009 planned.
- **A new title** shows in a line above the page.
- **Opening a version** in History shows its changes. **Show the clean version** shows the page as it was, and **Show the changes** goes back.
- **Accessibility:** colour is never alone: there are words, underlines and strikethrough.
- **Edge cases:** version 1 shows clean, with no button. Files and images in old versions show while the file exists.

### F3. Changes for mail

`document.ChangesText` and `document.ChangesHTML` turn a comparison into text and safe HTML with inline styles. They leave out what did not change, stop after 20 changes with a line that says how many more there are, and escape everything. Words come from the caller, in the person's language.

### F4. Pickers

`SubjectPicker` becomes a combobox with a list. Focus or a click shows the first people and groups. Typing narrows them. Down and Up move and wrap, Enter picks, Escape closes the list and not the dialog. Clicking a row picks it. A person without *view profiles* gets the message they got before, and no list.

## API

| Route | Rule | What |
|---|---|---|
| `GET /pages/{id}/compare` | `page.view` | Blocks now also carry `node` and `previous` |

## Data

No migration. `spaces.icon` takes up to 12 characters, so *crocodile* fits.

## Build order

1. Go: a comparison with blocks, the mail functions, the length of an icon.
2. Picker.
3. Animals.
4. Changes in the page.
5. Pictures, docs, handoff.

## Test plan

- **Go:** blocks in a comparison, escaping, the cap, a long icon name.
- **Mutation:** mail text that is not escaped.
- **Vitest:** animal from the key, a name that is no animal, words to ranges, the fallback, and display blocks.
- **Browser:** animals load and can be chosen, a character stays, changes in the page, clean and changes, version 1, a picker by keyboard, a guest gets no people.
- **Pictures:** every screen with a badge, and compare and history.

## Acceptance criteria

- [x] AC-1 Every space shows an animal without anyone doing anything, also existing ones.
- [x] AC-2 A space administrator can pick another animal or a character, and back.
- [x] AC-3 A comparison looks like the page, with green, red and words beside them.
- [x] AC-4 Changed words show in place, and a change in a table shows as removed and added.
- [x] AC-5 Opening a version shows what changed, and can show the clean page.
- [x] AC-6 The functions for mail escape everything, and stop after 20 changes.
- [x] AC-7 Every picker shows people on focus, works by keyboard, and shows none without *view profiles*.
- [x] AC-8 Everything passes axe, and every changed picture has been looked at.

## Risks

| Risk | Answer |
|---|---|
| Two animals can land on one key | Thirty animals for many spaces: it will happen, and a person can pick another |
| Comparison answers get larger | Both versions are already read to compare them, and the size of a document is limited |
| Several editors on one compare screen | Only runs that are shown exist, and long runs fold |

## How the product owner tests it

1. Open the start screen: every space has an animal.
2. Open Space settings, pick the moose and save.
3. Change a sentence in a page and publish. Open History, and View the newest version: new words are green, old ones red. Press Show the clean version.
4. Pick two versions and compare them.
5. Open a page's restrictions and click in the field: people show at once. Use the arrows and Enter.

## Provenance

Own text from these public sources, read for behaviour and licences only:

- Kenney, *Animal Pack Remastered* (`kenney.nl/assets/animal-pack-remastered`), with its `License.txt`: CC0.
- W3C, Understanding WCAG 2.2 success criterion 1.4.1 Use of Color.
- MDN, the `del` element.
- W3C WAI-ARIA Authoring Practices, select-only combobox example, and the patterns read for spec 009.
- ProseMirror reference manual, `prosemirror-view` decorations.
