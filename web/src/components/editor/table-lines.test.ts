// SPDX-License-Identifier: Apache-2.0
// Spec 023: rows and columns as things that can be selected and moved.
import { describe, expect, it } from 'vitest';
import { Editor } from '@tiptap/core';
import '../../i18n';
import { documentExtensions } from './extensions';
import { cellOfPosition, lineCell, moveLines, selectedLines, selectLine, setTableSettings } from './table';

const cell = (text: string, attrs: object = {}) => ({
  type: 'tableCell',
  attrs,
  content: [{ type: 'paragraph', content: [{ type: 'text', text }] }],
});

function editorWith(rows: object[][]) {
  return new Editor({
    extensions: documentExtensions(),
    content: {
      type: 'doc',
      content: [{ type: 'table', content: rows.map((content) => ({ type: 'tableRow', content })) }],
    },
  });
}

const threeByTwo = () =>
  editorWith([
    [cell('a1'), cell('a2')],
    [cell('b1'), cell('b2')],
    [cell('c1'), cell('c2')],
  ]);

function grid(editor: Editor): string[][] {
  const rows: string[][] = [];
  editor.state.doc.firstChild?.forEach((row) => {
    const cells: string[] = [];
    row.forEach((one) => cells.push(one.textContent));
    rows.push(cells);
  });
  return rows;
}

describe('a place in the grid', () => {
  it('goes from grid to document and back', () => {
    const editor = threeByTwo();
    const start = lineCell(editor, 0, { row: 2, column: 1 });
    expect(start).not.toBeNull();
    expect(cellOfPosition(editor, 0, (start ?? 0) + 1)).toEqual({ row: 2, column: 1 });
  });

  it('stays inside the table when a line is gone', () => {
    const editor = threeByTwo();
    expect(lineCell(editor, 0, { row: 9, column: 9 })).toBe(lineCell(editor, 0, { row: 2, column: 1 }));
  });

  it('has no answer for a table that is gone', () => {
    const editor = threeByTwo();
    expect(lineCell(editor, 1, { row: 0, column: 0 })).toBeNull();
  });
});

describe('selecting lines', () => {
  it('selects one row', () => {
    const editor = threeByTwo();
    selectLine(editor, 0, 'row', { row: 1, column: 0 }, false);
    expect(selectedLines(editor, 0, 'row')).toEqual({ first: 1, last: 1 });
    expect(selectedLines(editor, 0, 'column')).toBeNull();
  });

  it('reaches from the first line with shift', () => {
    const editor = threeByTwo();
    selectLine(editor, 0, 'row', { row: 0, column: 0 }, false);
    selectLine(editor, 0, 'row', { row: 2, column: 0 }, true);
    expect(selectedLines(editor, 0, 'row')).toEqual({ first: 0, last: 2 });
  });

  it('selects a column', () => {
    const editor = threeByTwo();
    selectLine(editor, 0, 'column', { row: 0, column: 1 }, false);
    expect(selectedLines(editor, 0, 'column')).toEqual({ first: 1, last: 1 });
  });
});

describe('moving lines', () => {
  it('moves a row down and up', () => {
    const editor = threeByTwo();
    expect(moveLines(editor, 0, 'row', { first: 0, last: 0 }, 2)).toBe(true);
    expect(grid(editor).map((row) => row[0])).toEqual(['b1', 'c1', 'a1']);
    expect(moveLines(editor, 0, 'row', { first: 2, last: 2 }, 0)).toBe(true);
    expect(grid(editor).map((row) => row[0])).toEqual(['a1', 'b1', 'c1']);
  });

  it('moves a column', () => {
    const editor = threeByTwo();
    expect(moveLines(editor, 0, 'column', { first: 0, last: 0 }, 1)).toBe(true);
    expect(grid(editor)[0]).toEqual(['a2', 'a1']);
  });

  it('moves several rows and keeps their order', () => {
    const editor = editorWith([
      [cell('a'), cell('.')],
      [cell('b'), cell('.')],
      [cell('c'), cell('.')],
      [cell('d'), cell('.')],
    ]);
    expect(moveLines(editor, 0, 'row', { first: 0, last: 1 }, 3)).toBe(true);
    expect(grid(editor).map((row) => row[0])).toEqual(['c', 'd', 'a', 'b']);
    expect(moveLines(editor, 0, 'row', { first: 2, last: 3 }, 0)).toBe(true);
    expect(grid(editor).map((row) => row[0])).toEqual(['a', 'b', 'c', 'd']);
  });

  it('does nothing past the edge, or onto itself', () => {
    const editor = threeByTwo();
    const before = grid(editor);
    expect(moveLines(editor, 0, 'row', { first: 0, last: 0 }, -1)).toBe(true);
    expect(moveLines(editor, 0, 'row', { first: 2, last: 2 }, 3)).toBe(true);
    expect(moveLines(editor, 0, 'row', { first: 1, last: 1 }, 1)).toBe(true);
    expect(grid(editor)).toEqual(before);
  });

  it('moves merged rows as one, and the table stays whole', () => {
    const editor = editorWith([
      [cell('a1', { rowspan: 2 }), cell('a2')],
      [cell('b2')],
      [cell('c1'), cell('c2')],
    ]);
    expect(moveLines(editor, 0, 'row', { first: 1, last: 1 }, 2)).toBe(true);
    expect(() => {
      editor.state.doc.check();
    }).not.toThrow();
    expect(grid(editor)[0]).toEqual(['c1', 'c2']);
    expect(grid(editor)[1]).toEqual(['a1', 'a2']);
  });
});

describe('table settings', () => {
  it('keeps the other settings', () => {
    const editor = threeByTwo();
    setTableSettings(editor, 0, { numbered: true });
    setTableSettings(editor, 0, { caption: 'Costs', caption_below: true });
    expect(editor.state.doc.firstChild?.attrs).toMatchObject({
      numbered: true,
      caption: 'Costs',
      caption_below: true,
    });
  });

  it('comes through a save', () => {
    const editor = threeByTwo();
    setTableSettings(editor, 0, { flexible: true, caption: 'Costs' });
    const attrs = editor.getJSON().content?.[0]?.attrs;
    expect(attrs).toMatchObject({ flexible: true, caption: 'Costs' });
  });
});
