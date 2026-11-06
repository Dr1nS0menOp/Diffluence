// SPDX-License-Identifier: Apache-2.0
// What the editor enforces around tables while writing: no table inside a table (EDT-101),
// and text with tabs lands as cells (EDT-100).
import { Extension } from '@tiptap/core';
import { Fragment, Slice, type Node as ProseNode, type ResolvedPos, type Schema } from '@tiptap/pm/model';
import { Plugin, type Transaction } from '@tiptap/pm/state';
import type { ReplaceStep } from '@tiptap/pm/transform';

const MAX_PASTED_ROWS = 1000;
const MAX_PASTED_COLUMNS = 100;
// Editing together: the other person's step already happened, and refusing it splits the document.
const FROM_ANOTHER_PERSON = 'y-sync$';

function holdsTable(fragment: Fragment): boolean {
  let found = false;
  fragment.descendants((node) => {
    if (node.type.name === 'table') found = true;
    return !found;
  });
  return found;
}

/** How many tables sit inside another table. */
export function nestedTables(doc: ProseNode): number {
  let count = 0;
  doc.descendants((node, _pos, parent) => {
    if (node.type.name !== 'table') return true;
    let inner = 0;
    node.descendants((child) => {
      if (child.type.name === 'table') inner += 1;
      return true;
    });
    count += inner;
    return parent?.type.name === 'doc' ? false : true;
  });
  return count;
}

/** Only a step that brings in a table can make things worse, so the rest costs nothing. */
export function addsNestedTable(tr: Transaction, before: ProseNode): boolean {
  if (!tr.docChanged || tr.getMeta(FROM_ANOTHER_PERSON)) return false;
  const inserts = tr.steps.some((step) => {
    const content = (step as Partial<ReplaceStep>).slice?.content;
    return content !== undefined && holdsTable(content);
  });
  return inserts && nestedTables(tr.doc) > nestedTables(before);
}

/** The cells in copied text. Text that is no grid gets no answer, so it stays text. */
export function cellsInText(text: string): string[][] | null {
  const lines = text.replace(/\r\n?/g, '\n').replace(/\n$/, '').split('\n');
  const rows = lines.map((line) => line.split('\t'));
  const width = rows[0]?.length ?? 0;
  const isGrid = width >= 2 && rows.every((row) => row.length === width);
  if (!isGrid || rows.length > MAX_PASTED_ROWS || width > MAX_PASTED_COLUMNS) return null;
  return rows;
}

export function tableOfCells(schema: Schema, rows: string[][]): ProseNode {
  const { table, tableRow, tableCell, paragraph } = schema.nodes;
  if (!table || !tableRow || !tableCell || !paragraph) throw new Error('the schema has no tables');
  const cell = (text: string) =>
    tableCell.create(null, paragraph.create(null, text ? schema.text(text) : null));
  return table.create(
    null,
    rows.map((row) => tableRow.create(null, row.map(cell))),
  );
}

function parseCells(text: string, $context: ResolvedPos, plain: boolean, schema: Schema): Slice | null {
  if (plain || $context.parent.type.spec.code) return null;
  const rows = cellsInText(text);
  return rows ? new Slice(Fragment.from(tableOfCells(schema, rows)), 0, 0) : null;
}

export const TableRules = Extension.create<{ onNested: () => void }>({
  name: 'tableRules',
  addOptions() {
    return { onNested: () => undefined };
  },
  addProseMirrorPlugins() {
    const { onNested } = this.options;
    return [
      new Plugin({
        filterTransaction(tr, state) {
          if (!addsNestedTable(tr, state.doc)) return true;
          onNested();
          return false;
        },
        props: {
          // The library's types ask for a slice, while the editor takes no answer as "use the text".
          clipboardTextParser: (text, $context, plain, view) =>
            parseCells(text, $context, plain, view.state.schema) as Slice,
        },
      }),
    ];
  },
});
