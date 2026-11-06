// SPDX-License-Identifier: Apache-2.0
// Spec 023: what a table refuses, and what a spreadsheet leaves on the clipboard.
import { describe, expect, it } from 'vitest';
import { getSchema } from '@tiptap/core';
import { EditorState } from '@tiptap/pm/state';
import '../../i18n';
import { documentExtensions } from './extensions';
import { addsNestedTable, cellsInText, nestedTables, tableOfCells } from './table-rules';

const schema = getSchema(documentExtensions());
const paragraph = { type: 'paragraph', content: [{ type: 'text', text: 'x' }] };
const table = (cell: object) => ({
  type: 'table',
  content: [{ type: 'tableRow', content: [{ type: 'tableCell', content: [cell] }] }],
});
const docOf = (...content: object[]) => schema.nodeFromJSON({ type: 'doc', content });

describe('a table inside a table', () => {
  it('counts a table in a cell, also below a panel', () => {
    expect(nestedTables(docOf(table(paragraph)))).toBe(0);
    expect(nestedTables(docOf(table(table(paragraph))))).toBe(1);
    expect(
      nestedTables(docOf(table({ type: 'panel', attrs: { kind: 'info' }, content: [table(paragraph)] }))),
    ).toBe(1);
  });

  it('does not count a table in a panel, which is no table', () => {
    expect(nestedTables(docOf({ type: 'panel', attrs: { kind: 'info' }, content: [table(paragraph)] }))).toBe(
      0,
    );
  });

  const stateWithTable = () => EditorState.create({ schema, doc: docOf(table(paragraph)) });
  const insideTheCell = 4;

  it('refuses a step that puts a table in a cell', () => {
    const state = stateWithTable();
    const tr = state.tr.insert(insideTheCell, schema.nodeFromJSON(table(paragraph)));
    expect(addsNestedTable(tr, state.doc)).toBe(true);
  });

  it('lets a table through below another table', () => {
    const state = stateWithTable();
    const tr = state.tr.insert(state.doc.content.size, schema.nodeFromJSON(table(paragraph)));
    expect(addsNestedTable(tr, state.doc)).toBe(false);
  });

  it('leaves a document alone that already had one', () => {
    const state = EditorState.create({ schema, doc: docOf(table(table(paragraph))) });
    const tr = state.tr.insertText('more', 1);
    expect(addsNestedTable(tr, state.doc)).toBe(false);
  });

  it('never refuses what another person already did', () => {
    const state = stateWithTable();
    const tr = state.tr.insert(insideTheCell, schema.nodeFromJSON(table(paragraph))).setMeta('y-sync$', {});
    expect(addsNestedTable(tr, state.doc)).toBe(false);
  });
});

describe('cells in copied text', () => {
  it('reads rows and columns', () => {
    expect(cellsInText('a\tb\nc\td\n')).toEqual([
      ['a', 'b'],
      ['c', 'd'],
    ]);
  });

  it('copes with the line ends of Windows', () => {
    expect(cellsInText('a\tb\r\nc\td')).toEqual([
      ['a', 'b'],
      ['c', 'd'],
    ]);
  });

  it('keeps an empty cell', () => {
    expect(cellsInText('a\t\n\tb')).toEqual([
      ['a', ''],
      ['', 'b'],
    ]);
  });

  it.each([
    ['plain text', 'just some words'],
    ['lines without tabs', 'one\ntwo'],
    ['lines of different lengths', 'a\tb\nc'],
    ['a table too wide', Array.from({ length: 101 }, () => 'x').join('\t')],
    ['a table too long', Array.from({ length: 1001 }, () => 'a\tb').join('\n')],
  ])('leaves %s alone', (_name, text) => {
    expect(cellsInText(text)).toBeNull();
  });

  it('makes a table the schema accepts, with empty cells too', () => {
    const made = tableOfCells(schema, [
      ['a', ''],
      ['', 'b'],
    ]);
    expect(() => {
      made.check();
    }).not.toThrow();
    expect(made.childCount).toBe(2);
    expect(made.textContent).toBe('ab');
  });
});
