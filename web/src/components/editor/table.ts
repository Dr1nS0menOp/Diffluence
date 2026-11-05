// SPDX-License-Identifier: Apache-2.0
// Table cells with a named background, and sorting by a column.
import type { Editor } from '@tiptap/core';
import { TableCell, TableHeader } from '@tiptap/extension-table';
import { Fragment, type Node as ProseNode } from '@tiptap/pm/model';

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

function tableAround(editor: Editor) {
  const { $from } = editor.state.selection;
  for (let depth = $from.depth; depth > 0; depth--) {
    if ($from.node(depth).type.name === 'table') {
      return { table: $from.node(depth), pos: $from.before(depth), column: $from.index(depth + 1) };
    }
  }
  return null;
}

/** Sorting moves whole rows, which merged cells do not allow. */
export function canSortTable(editor: Editor): boolean {
  const found = tableAround(editor);
  if (!found) return false;
  let merged = false;
  found.table.descendants((node) => {
    const { colspan, rowspan } = node.attrs as { colspan?: number; rowspan?: number };
    if ((colspan ?? 1) > 1 || (rowspan ?? 1) > 1) merged = true;
    return !merged;
  });
  return !merged;
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
