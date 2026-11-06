// SPDX-License-Identifier: Apache-2.0
// What a reader can do with a table without changing the page (EDT-094, 099, 103). It all
// happens in this view of the document, which is never sent anywhere. Header cells hold
// real buttons, and aria-sort sits on the sorted column only, as the WAI-ARIA Authoring
// Practices ask for a sortable table.
import { Extension } from '@tiptap/core';
import { Fragment, type Node as ProseNode } from '@tiptap/pm/model';
import { Plugin, PluginKey, type EditorState } from '@tiptap/pm/state';
import { Decoration, DecorationSet, type EditorView } from '@tiptap/pm/view';
import i18n from '../../i18n';
import { refreshHighlights } from '../comments/anchor';
import { hasHeaderRow, hasMergedCells, sortRows } from './table';

const FILTER_FROM_ROWS = 6;
const LONG_TABLE_ROWS = 15;

export type SortDirection = 'up' | 'down';

interface ReaderChoice {
  filter: string;
  sort: { column: number; direction: SortDirection } | null;
}

interface ChoiceChange extends Partial<ReaderChoice> {
  table: number;
}

type ReaderChoices = Record<number, ReaderChoice>;

const NO_CHOICE: ReaderChoice = { filter: '', sort: null };

const readerTablesKey = new PluginKey<ReaderChoices>('readerTables');

/** The first press sorts up, the next one turns it around. */
export function nextDirection(current: ReaderChoice['sort'], column: number): SortDirection {
  return current?.column === column && current.direction === 'up' ? 'down' : 'up';
}

export const rowMatches = (rowText: string, filter: string) =>
  rowText.toLowerCase().includes(filter.trim().toLowerCase());

const isReadable = (table: ProseNode) => hasHeaderRow(table) && !hasMergedCells(table);

function bodyRows(table: ProseNode): ProseNode[] {
  const rows: ProseNode[] = [];
  table.forEach((row, _offset, index) => {
    if (index > 0) rows.push(row);
  });
  return rows;
}

const choiceOf = (state: EditorState, table: number) => readerTablesKey.getState(state)?.[table] ?? NO_CHOICE;

function sortColumn(view: EditorView, pos: number, column: number) {
  const { state } = view;
  const table = state.doc.nodeAt(pos);
  if (!table) return;
  const direction = nextDirection(choiceOf(state, pos).sort, column);
  const rows: ProseNode[] = [];
  table.forEach((row) => rows.push(row));
  const sorted = table.copy(Fragment.fromArray(sortRows(rows, column, direction)));
  const change: ChoiceChange = { table: pos, sort: { column, direction } };
  view.dispatch(
    state.tr
      .replaceWith(pos, pos + table.nodeSize, sorted)
      .setMeta(readerTablesKey, change)
      .setMeta(refreshHighlights, true)
      .setMeta('addToHistory', false),
  );
  // The table was drawn again, so the pressed button is a new one.
  view.dom.querySelector<HTMLElement>(`[data-sort-button="${pos}-${column}"]`)?.focus();
}

function sortButton(view: EditorView, pos: number, column: number, name: string, direction: string) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'table-sort';
  button.dataset.sortButton = `${pos}-${column}`;
  button.dataset.direction = direction;
  button.setAttribute('aria-label', i18n.t('readerTable.sortBy', { name }));
  button.addEventListener('click', () => sortColumn(view, pos, column));
  return button;
}

function filterBox(view: EditorView, pos: number) {
  const box = document.createElement('div');
  box.className = 'table-filter';
  const input = document.createElement('input');
  input.type = 'search';
  input.placeholder = i18n.t('readerTable.filter');
  input.setAttribute('aria-label', i18n.t('readerTable.filter'));
  const count = document.createElement('span');
  count.className = 'muted';
  count.setAttribute('role', 'status');
  input.addEventListener('input', () => {
    const change: ChoiceChange = { table: pos, filter: input.value };
    view.dispatch(view.state.tr.setMeta(readerTablesKey, change));
    const table = view.state.doc.nodeAt(pos);
    const rows = table ? bodyRows(table) : [];
    const shown = rows.filter((row) => rowMatches(row.textContent, input.value)).length;
    count.textContent = input.value ? i18n.t('readerTable.rows', { shown, total: rows.length }) : '';
  });
  box.append(input, count);
  return box;
}

function headerDecorations(table: ProseNode, pos: number, choice: ReaderChoice): Decoration[] {
  const found: Decoration[] = [];
  const firstCell = pos + 2;
  table.firstChild?.forEach((cell, offset, column) => {
    const start = firstCell + offset;
    const sorted = choice.sort?.column === column ? choice.sort.direction : null;
    if (sorted) {
      const value = sorted === 'up' ? 'ascending' : 'descending';
      found.push(Decoration.node(start, start + cell.nodeSize, { 'aria-sort': value }));
    }
    const direction = sorted ?? 'none';
    found.push(
      Decoration.widget(
        start + cell.nodeSize - 2,
        (view) => sortButton(view, pos, column, cell.textContent, direction),
        { side: 1, key: `sort-${pos}-${column}-${direction}`, stopEvent: () => true, ignoreSelection: true },
      ),
    );
  });
  return found;
}

function rowDecorations(table: ProseNode, pos: number, filter: string): Decoration[] {
  const found: Decoration[] = [];
  table.forEach((row, offset, index) => {
    if (index === 0 || rowMatches(row.textContent, filter)) return;
    const start = pos + 1 + offset;
    found.push(Decoration.node(start, start + row.nodeSize, { class: 'row-filtered' }));
  });
  return found;
}

function decorationsOf(state: EditorState): DecorationSet {
  const found: Decoration[] = [];
  state.doc.descendants((node, pos) => {
    if (node.type.name !== 'table') return true;
    if (!isReadable(node)) return false;
    const choice = choiceOf(state, pos);
    found.push(...headerDecorations(node, pos, choice), ...rowDecorations(node, pos, choice.filter));
    if (node.childCount >= LONG_TABLE_ROWS) {
      found.push(Decoration.node(pos, pos + node.nodeSize, { class: 'table-long' }));
    }
    if (node.childCount >= FILTER_FROM_ROWS) {
      found.push(
        Decoration.widget(pos, (view) => filterBox(view, pos), {
          side: -1,
          key: `filter-${pos}`,
          stopEvent: () => true,
          ignoreSelection: true,
        }),
      );
    }
    return false;
  });
  return DecorationSet.create(state.doc, found);
}

export const ReaderTables = Extension.create({
  name: 'readerTables',
  addProseMirrorPlugins() {
    return [
      new Plugin<ReaderChoices>({
        key: readerTablesKey,
        state: {
          init: () => ({}),
          apply(tr, choices) {
            const change = tr.getMeta(readerTablesKey) as ChoiceChange | undefined;
            if (!change) return choices;
            const { table, ...rest } = change;
            return { ...choices, [table]: { ...(choices[table] ?? NO_CHOICE), ...rest } };
          },
        },
        props: { decorations: decorationsOf },
      }),
    ];
  },
});
