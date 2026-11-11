# Spec 055: the line through the first row of a table

Status: built (wave TB, 2026-11-11).

## What was wrong

While a table was in use in the editor, a thin accent line ran across the table about
12 px below its top edge, through the header row, and a second one ran down its start
side, 12 px past its end. The product owner reported it as "the edit line comes through
the first row".

## Cause

The lines are the drop markers that show where a dragged row or column lands
(`.table-drop`, spec 023). They carry the `hidden` attribute until a drag starts. Their
second class named the kind, `row` or `column`, and those are also the layout classes of
`base.css` and `layout.css`, which set `display: flex`. An author `display` beats the
browser's own `[hidden] { display: none }`, so both markers showed all the time, placed
at their resting spot: the top of the first row and the start of the first column.

The handles and plus buttons of the row and column in use (`.table-line`, `.table-plus`)
had the same classes, so they too stayed visible when their column was scrolled out of
view and should have hidden.

## Fix

The kind classes are now `row-line` and `column-line`, which no layout rule uses, so
`hidden` hides again. A comment at the marker says why the names differ.

## Proof

- `web/e2e/links-tables.spec.ts`, "the drop line shows only while a row is dragged":
  no marker is visible while a cell is in use, one shows during a drag, none after. It
  failed before the fix with two visible markers.
- The existing drag, selection and table tests pass with the new class names.
- Pictures taken in light and dark, right to left, a wide table scrolled sideways and a
  narrow window show no stray line.

The fix is a class name in a component and in CSS; the browser test above is its check.
The mutation check has no browser runner, so no mutation covers it.

## Seen and left

- The block handle of the paragraph above a table can overlap the table's top corner on a
  narrow window. It is the general block handle, not part of table editing.
- The row handle sits half on the table's start edge by design (spec 023), and can touch
  the first letters of a cell on a narrow window.
