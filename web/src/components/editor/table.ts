// SPDX-License-Identifier: Apache-2.0
// Tables: how far they reach beyond the text, cells with a named background, and
// what the table menu does to rows and columns.
import type { Editor } from '@tiptap/core';
import { Table, TableCell, TableHeader, TableView } from '@tiptap/extension-table';
import { Fragment, type Node as ProseNode } from '@tiptap/pm/model';
import { TextSelection } from '@tiptap/pm/state';
import { CellSelection, moveTableColumn, moveTableRow, TableMap } from '@tiptap/pm/tables';
import type { EditorView } from '@tiptap/pm/view';
import { dataAttribute, switchAttribute } from './dynamic/nodes';

export const STRETCH_STEP = 5;
export const WIDE_STRETCH = 50;
export const FULL_STRETCH = 100;
const PERCENT = 100;

/** A table scrolls in its own frame. The width comes from a number, never from the document. */
class StretchedTableView extends TableView {
  constructor(node: ProseNode, cellMinWidth: number, view: EditorView, attrs?: Record<string, unknown>) {
    super(node, cellMinWidth, view, attrs);
    const { stretch, numbered, flexible, caption, caption_below: below } = node.attrs as TableSettings;
    if (stretch) {
      this.dom.dataset.stretch = String(stretch);
      this.dom.style.setProperty('--table-stretch', String(stretch));
    }
    if (numbered) this.dom.dataset.numbered = '';
    if (flexible) {
      this.dom.dataset.flexible = '';
      this.forgetWidth();
    }
    if (caption) this.addCaption(caption, below);
  }

  update(node: ProseNode) {
    const kept = super.update(node);
    if (kept && (node.attrs as TableSettings).flexible) this.forgetWidth();
    return kept;
  }

  /** The columns set a width in pixels. A flexible table follows the page instead. */
  private forgetWidth() {
    this.table.style.width = '';
    this.table.style.minWidth = '';
  }

  private addCaption(words: string, below: boolean) {
    const caption = document.createElement('caption');
    caption.textContent = words;
    caption.contentEditable = 'false';
    if (below) caption.className = 'below';
    this.table.prepend(caption);
  }
}

interface TableSettings {
  stretch: number | null;
  numbered: boolean;
  flexible: boolean;
  caption: string | null;
  caption_below: boolean;
}

export const DocumentTable = Table.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      stretch: {
        default: null,
        parseHTML: (el: HTMLElement) => Number(el.getAttribute('data-stretch')) || null,
        renderHTML: (attrs: { stretch?: number | null }) =>
          attrs.stretch ? { 'data-stretch': attrs.stretch } : {},
      },
      block_id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-block'),
        renderHTML: (attrs: { block_id?: string | null }) =>
          attrs.block_id ? { 'data-block': attrs.block_id } : {},
      },
      numbered: switchAttribute('numbered'),
      flexible: switchAttribute('flexible'),
      caption: dataAttribute('caption'),
      caption_below: switchAttribute('caption_below'),
    };
  },
}).configure({ resizable: true, View: StretchedTableView });

/** How wide a table is at a stretch: the column, plus that share of the room beyond it. */
export function stretchedWidth(column: number, room: number, stretch: number): number {
  return column + (Math.max(0, room - column) * stretch) / PERCENT;
}

/** The stretch that puts the edge of a centred table at this distance from the centre. */
export function stretchAt(fromCentre: number, column: number, room: number): number {
  const extra = room - column;
  if (extra <= 0) return 0;
  const share = ((2 * Math.abs(fromCentre) - column) / extra) * PERCENT;
  const stepped = Math.round(share / STRETCH_STEP) * STRETCH_STEP;
  return Math.min(FULL_STRETCH, Math.max(0, stepped));
}

/** Fixed columns keep their proportions when the table gets wider or narrower. */
export function scaledWidths(widths: number[], factor: number): number[] {
  return widths.map((width) => Math.max(1, Math.round(width * factor)));
}

const background = {
  background: {
    default: null,
    parseHTML: (el: HTMLElement) => el.getAttribute('data-background'),
    renderHTML: (attrs: Record<string, string | null>) =>
      attrs.background ? { 'data-background': attrs.background, class: `mark-${attrs.background}` } : {},
  },
};

export const DocumentCell = TableCell.extend({
  addAttributes() {
    return { ...this.parent?.(), ...background };
  },
});

export const DocumentHeaderCell = TableHeader.extend({
  addAttributes() {
    return { ...this.parent?.(), ...background };
  },
});

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

function childrenOf(node: ProseNode): ProseNode[] {
  const children: ProseNode[] = [];
  node.forEach((child) => children.push(child));
  return children;
}

const isHeaderRow = (row: ProseNode) => childrenOf(row).every((cell) => cell.type.name === 'tableHeader');

/** Natural order, so 2 comes before 10. Header rows stay on top. */
export function sortRows(rows: ProseNode[], column: number, direction: 'up' | 'down'): ProseNode[] {
  const sign = direction === 'up' ? 1 : -1;
  const textOf = (row: ProseNode) => (column < row.childCount ? row.child(column).textContent : '');
  const body = rows.filter((row) => !isHeaderRow(row));
  const sorted = [...body].sort((a, b) => sign * collator.compare(textOf(a), textOf(b)));
  return [...rows.filter(isHeaderRow), ...sorted];
}

/** The table at a remembered position, which may be gone by now. */
export function tableAt(editor: Editor, pos: number): ProseNode | null {
  const { doc } = editor.state;
  if (pos > doc.content.size) return null;
  const node = doc.nodeAt(pos);
  return node?.type.name === 'table' ? node : null;
}

export const isTopLevel = (editor: Editor, pos: number) =>
  pos <= editor.state.doc.content.size && editor.state.doc.resolve(pos).depth === 0;

export function tableAround(editor: Editor) {
  const { $from } = editor.state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === 'table') {
      return {
        table: $from.node(depth),
        pos: $from.before(depth),
        column: $from.index(depth + 1),
        cell: $from.before(depth + 2),
      };
    }
  }
  return null;
}

/** The menu acts on the table with the cursor, so a table under the pointer gets it first. */
export function enterTable(editor: Editor, pos: number) {
  if (tableAround(editor)?.pos === pos || !tableAt(editor, pos)) return;
  const { tr, doc } = editor.state;
  editor.view.dispatch(tr.setSelection(TextSelection.near(doc.resolve(pos + 1))));
}

const isHeader = (cell: ProseNode | null | undefined) => cell?.type.name === 'tableHeader';

export function hasHeaderRow(table: ProseNode): boolean {
  return table.firstChild ? isHeaderRow(table.firstChild) : false;
}

export function hasHeaderColumn(table: ProseNode): boolean {
  return childrenOf(table).every((row) => isHeader(row.firstChild));
}

/** Gives every cell of the current row or column a colour, in one step. */
export function paintLine(editor: Editor, line: 'row' | 'column', background: string | null) {
  const found = tableAround(editor);
  if (!found) return;
  const { table, pos, cell } = found;
  const start = pos + 1;
  const map = TableMap.get(table);
  const here = map.findCell(cell - start);
  const rect =
    line === 'row'
      ? { left: 0, right: map.width, top: here.top, bottom: here.bottom }
      : { left: here.left, right: here.right, top: 0, bottom: map.height };
  const { tr } = editor.state;
  map.cellsInRect(rect).forEach((offset) => {
    const node = table.nodeAt(offset);
    if (node) tr.setNodeMarkup(start + offset, undefined, { ...node.attrs, background });
  });
  editor.view.dispatch(tr);
}

/** Sets how far a table reaches, and keeps fixed columns in proportion, as one Undo. */
export function setStretch(editor: Editor, pos: number, stretch: number, factor: number) {
  const { tr } = editor.state;
  const table = tableAt(editor, pos);
  if (!table) return;
  table.descendants((node, offset) => {
    const widths = node.attrs.colwidth as number[] | null | undefined;
    if (widths) {
      tr.setNodeMarkup(pos + 1 + offset, undefined, {
        ...node.attrs,
        colwidth: scaledWidths(widths, factor),
      });
    }
    return true;
  });
  tr.setNodeMarkup(pos, undefined, { ...table.attrs, stretch: stretch || null });
  editor.view.dispatch(tr);
}

export function hasMergedCells(table: ProseNode): boolean {
  let merged = false;
  table.descendants((node) => {
    const { colspan, rowspan } = node.attrs as { colspan?: number; rowspan?: number };
    if ((colspan ?? 1) > 1 || (rowspan ?? 1) > 1) merged = true;
    return !merged;
  });
  return merged;
}

/** Sorting moves whole rows, which merged cells do not allow. */
export function canSortTable(editor: Editor): boolean {
  const found = tableAround(editor);
  return found !== null && !hasMergedCells(found.table);
}

export function sortTable(editor: Editor, direction: 'up' | 'down') {
  const found = tableAround(editor);
  if (!found) return;
  const { table, pos, column } = found;
  const rows = sortRows(childrenOf(table), column, direction);
  const sorted = table.copy(Fragment.fromArray(rows));
  editor.view.dispatch(editor.state.tr.replaceWith(pos, pos + table.nodeSize, sorted));
}

/** Makes selected columns equal by forgetting their widths: the browser then shares the space. */
export function equalColumns(editor: Editor) {
  const found = tableAround(editor);
  if (!found) return;
  const { tr } = editor.state;
  found.table.descendants((node, offset) => {
    if (node.attrs.colwidth) {
      tr.setNodeMarkup(found.pos + 1 + offset, undefined, { ...node.attrs, colwidth: null });
    }
    return true;
  });
  editor.view.dispatch(tr);
}

/** Changes a setting of the table, as one Undo. */
export function setTableSettings(editor: Editor, pos: number, settings: Partial<TableSettings>) {
  const table = tableAt(editor, pos);
  if (!table) return;
  editor.view.dispatch(editor.state.tr.setNodeMarkup(pos, undefined, { ...table.attrs, ...settings }));
}

export type LineKind = 'row' | 'column';

/** A cell by its place in the grid. */
export interface TableCell {
  row: number;
  column: number;
}

export interface LineRange {
  first: number;
  last: number;
}

/** Where a cell of the grid starts in the document. */
export function lineCell(editor: Editor, pos: number, { row, column }: TableCell): number | null {
  const table = tableAt(editor, pos);
  if (!table) return null;
  const map = TableMap.get(table);
  const offset = map.map[Math.min(row, map.height - 1) * map.width + Math.min(column, map.width - 1)];
  return offset === undefined ? null : pos + 1 + offset;
}

/** The grid place of a position inside a table. */
export function cellOfPosition(editor: Editor, pos: number, inside: number): TableCell | null {
  const table = tableAt(editor, pos);
  if (!table) return null;
  const $inside = editor.state.doc.resolve(inside);
  for (let depth = $inside.depth; depth > 0; depth--) {
    const role = $inside.node(depth).type.spec.tableRole as string | undefined;
    if ((role === 'cell' || role === 'header_cell') && $inside.node(depth - 2) === table) {
      const { left, top } = TableMap.get(table).findCell($inside.before(depth) - pos - 1);
      return { row: top, column: left };
    }
  }
  return null;
}

/** The row and column of the cursor, when the cursor is in this table. */
export function linesInUse(editor: Editor, pos: number): TableCell | null {
  const found = tableAround(editor);
  if (found?.pos !== pos) return null;
  return cellOfPosition(editor, pos, found.cell + 1);
}

/** Selects a whole row or column. With extend it reaches from the line selected before. */
export function selectLine(editor: Editor, pos: number, kind: LineKind, line: TableCell, extend: boolean) {
  const start = lineCell(
    editor,
    pos,
    kind === 'row' ? { row: line.row, column: 0 } : { row: 0, column: line.column },
  );
  if (start === null) return;
  const { doc, selection, tr } = editor.state;
  const $head = doc.resolve(start);
  const $anchor = extend && selection instanceof CellSelection ? selection.$anchorCell : $head;
  const make = kind === 'row' ? CellSelection.rowSelection : CellSelection.colSelection;
  editor.view.dispatch(tr.setSelection(make($anchor, $head)));
}

/** The whole lines that are selected, if any. */
export function selectedLines(editor: Editor, pos: number, kind: LineKind): LineRange | null {
  const { selection } = editor.state;
  const table = tableAt(editor, pos);
  if (!table || !(selection instanceof CellSelection)) return null;
  const whole = kind === 'row' ? selection.isRowSelection() : selection.isColSelection();
  if (!whole) return null;
  const map = TableMap.get(table);
  const start = pos + 1;
  const { left, right, top, bottom } = map.rectBetween(
    selection.$anchorCell.pos - start,
    selection.$headCell.pos - start,
  );
  return kind === 'row' ? { first: top, last: bottom - 1 } : { first: left, last: right - 1 };
}

/** Moves whole lines so that they land on `to`. Says no when merged cells stand in the way. */
export function moveLines(
  editor: Editor,
  pos: number,
  kind: LineKind,
  lines: LineRange,
  to: number,
): boolean {
  const table = tableAt(editor, pos);
  if (!table) return false;
  const map = TableMap.get(table);
  const end = (kind === 'row' ? map.height : map.width) - 1;
  if (to < 0 || to > end || (to >= lines.first && to <= lines.last)) return true;
  const move = kind === 'row' ? moveTableRow : moveTableColumn;
  const count = lines.last - lines.first + 1;
  const downwards = to > lines.last;
  for (let step = 0; step < count; step++) {
    const from = downwards ? lines.first : lines.first + step;
    const target = downwards ? to : to + step;
    const moved = move({ from, to: target, pos: pos + 1, select: false })(editor.state, editor.view.dispatch);
    if (!moved) return false;
  }
  const landed = downwards ? to - count + 1 : to;
  selectLine(editor, pos, kind, { row: landed, column: landed }, false);
  if (count > 1) selectLine(editor, pos, kind, { row: landed + count - 1, column: landed + count - 1 }, true);
  return true;
}
