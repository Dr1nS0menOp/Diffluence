# Wave ES study: editor and search (ADR-038, third wave)

The study behind spec 047, written from the code on `main` at `2965bf7` without running anything: the development containers were not available that night. Every number in the spec that is not quoted from `docs/operations/limits.md` is an estimate for the build to measure.

## What was read

- The editor: `toolbar.tsx`, `selection-tools.tsx`, `tool.tsx`, the reader's `comments/selection-bar.tsx`, `menu.tsx` (`placeBeside`) and `design/together.css` (`.selection-bar`).
- `web/package.json` and `web/package-lock.json`: `@tiptap/extension-bubble-menu` 3.32.2 is an optional dependency of `@tiptap/react` 3.32.2, so it is installed but not ours. It pulls in `@floating-ui/dom` 1.8.0, `@floating-ui/core` 1.8.0 and `@floating-ui/utils` 0.2.12. All four are MIT.
- Search: `internal/search/search.go` and `words.go` (the tiers, `rowScope`, `notHidden`, `pickedRows`), migration 0028 (`search_pages`, `attachments_name_words_idx`), and `internal/attachments` (`Find`, `SpaceFiles`, `fileWords`, `fileNameWords`).
- The images: every PostgreSQL in Compose, CI and the IdP test is `postgres:16`, which ships `pg_trgm`. No migration creates an extension yet.
- The storage limit: `attachments/limits.go`, every `store.Put`, every insert into `attachments` and `attachment_versions`, and every caller of `CopyAll`, `MoveToSpace` and `AddImported`.
- The specs of waves PM (045) and SX (046), to keep migration numbers and files apart: this wave takes 0043 and touches none of their files.

## Findings worth keeping

- The study first advised our own bar, because placing one already exists twice (the reader's bar and floating menus). The product owner chose TipTap's `BubbleMenu` on 2026-11-10. It places the bar, but adds no keyboard access, names or right-to-left arrows, so those are built around it. The fixed row's roving focus does not read the direction today, and is fixed for both bars.
- `ImageTools` already holds every image option EDT-080 lists; only the place is new.
- Near matches fit the existing tiers as two more steps; picking and the permission check stay apart, as ADR-037 asks.
- The count behind the storage limit sums every version row. `MakeCurrent` adds a row that points at an object already stored, so the Files screen total counts that object twice. Spec 047, D19, counts each stored object once.
- Six ways bytes come into a space do not yet pass the limit; ADR-038 already said these must count. They are listed in spec 047, part C, with one shared check to add.

## The product owner's answers (2026-11-10)

The fixed row stays and cannot be hidden. The text bar also gets Heading 1 to 3 and both lists. The words of the text rank before titles with a typo. No grace period when the trash starts to count. TipTap's `BubbleMenu` instead of our own bar. Spec 047 holds the details.

## Not proven

Everything in this study is read, not run. The load budgets of parts B and C are estimates until step B6 measures them.
