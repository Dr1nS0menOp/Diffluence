# Handoff 2026-11-05: the editor after the product owner's first test

## What was decided

- ADR-030: the page scales, Full width is a tool in the row, and the editor keeps the tree. This replaces the fixed column and "the sidebar goes" of ADR-021. The product owner asked for it.
- Spec 017 holds the study, 15 decisions and the plan.

## What was done

- `web/src/components/editor/table-frame.tsx`: the outline, the button with one menu, and the edge that stretches a table. Nothing for tables is left in the row of tools.
- `stretch` on `table`, a number from 1 to 100, through `Normalize`. Fixed columns scale with it.
- `--size-page` scales a page and the editor. `SpaceShell` with `focus` gives the editor its sidebar.
- `web/src/components/page-header.tsx` replaces the Page look dialog.
- All languages load on demand through `code-languages.ts`, in a file of their own of 743 kB. Guessing a language stays with the common ones.
- Leaving the editor saves first. When that fails, the text stays in the browser backup of wave D, and a message leads back to it.
- Only a change someone made marks the page as changed. The empty line after a last block arrives by itself on the first click, and it used to leave a draft on a page nobody touched.
- A menu that opens upwards now stops at the bars that stay in place. It used to slide under the row of tools, with the first item hidden.

## Unfinished, or seen along the way

- Handles on each row and column, and a plus between them, wait for wave E.
- A stretched table hides the contents list. That is a choice, not a limit of the code (D7).
- Between about 800 and 1000 pixels the edit bar wraps, and then covers the first row of the tools while the page is scrolled. This comes from `--size-editbar` being a fixed size for `.toolbar`, and is not fixed here. The table button does stay clear of both.
- With Page look gone, labels sit below the text in the editor (D20).
- The email half of the product owner's feedback and the green and red history are in the other feedback branch.

## What is next

Merge after waves C, D and F. The example space shows a stretched table and Rust.
