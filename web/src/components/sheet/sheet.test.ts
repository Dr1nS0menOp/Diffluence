// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { csvField, csvToSheet, parseCsv, parsePasted, sheetToCsv } from './csv';
import { CYCLE_VALUE, ERROR_VALUE, evaluator } from './formula';
import {
  addRow,
  changeColumn,
  columnLetter,
  documentOfSheet,
  fitsColumn,
  isSafeLink,
  moveRow,
  pasteBlock,
  removeColumn,
  sheetOfDocument,
  type Sheet,
} from './model';

function sheetOf(lines: string[][]): Sheet {
  const columns = (lines[0] ?? []).map((_, i) => ({ id: `c${i}`, name: `C${i}`, type: 'text' as const }));
  const rows = lines.map((line, r) => {
    const cells: Record<string, string> = {};
    line.forEach((value, c) => {
      if (value !== '') cells[`c${c}`] = value;
    });
    return { id: `r${r}`, cells };
  });
  return { columns, rows };
}

const worked = (
  formula: string,
  lines: string[][] = [
    ['1', '2'],
    ['3', 'text'],
  ],
) => {
  const sheet = sheetOf([...lines, [formula, '']]);
  return evaluator(sheet)(lines.length, 0);
};

describe('formulas', () => {
  it('refuses what is not a formula, without running it', () => {
    expect(worked('=alert(1)')).toBe(ERROR_VALUE);
    expect(worked('=constructor')).toBe(ERROR_VALUE);
    expect(worked('=A1;B1')).toBe(ERROR_VALUE);
    expect(worked('=(1+')).toBe(ERROR_VALUE);
    expect(worked('=1 2')).toBe(ERROR_VALUE);
    expect(worked('=A1$')).toBe(ERROR_VALUE);
    expect(worked(`=${'('.repeat(5000)}1`)).toBe(ERROR_VALUE);
  });

  it('shows a loop, directly and through other cells', () => {
    expect(evaluator(sheetOf([['=A1']]))(0, 0)).toBe(CYCLE_VALUE);
    const round = evaluator(sheetOf([['=B1', '=A1']]));
    expect(round(0, 0)).toBe(CYCLE_VALUE);
  });

  it('stops a very long chain of cells with an error, not a crash', () => {
    const lines = Array.from({ length: 3000 }, (_, i) => [`=A${i + 2}`]);
    expect(evaluator(sheetOf(lines))(0, 0)).toBe(ERROR_VALUE);
  });

  it('does not divide by nothing', () => {
    expect(worked('=1/0')).toBe(ERROR_VALUE);
    expect(worked('=AVERAGE(D1:D2)')).toBe(ERROR_VALUE);
  });

  it('passes an error on to the cells that use it', () => {
    expect(evaluator(sheetOf([['=1/0', '=A1+1']]))(0, 1)).toBe(ERROR_VALUE);
  });

  it('adds, multiplies and keeps the order of operations', () => {
    expect(worked('=1+2*3')).toBe('7');
    expect(worked('=(1+2)*3')).toBe('9');
    expect(worked('=-A1+A2')).toBe('2');
    expect(worked('=0.1+0.2')).toBe('0.3');
  });

  it('counts an empty cell as nothing, and text not at all in a range', () => {
    expect(worked('=A1+D9')).toBe('1');
    expect(worked('=SUM(A1:B2)')).toBe('6');
    expect(worked('=COUNT(A1:B2)')).toBe('3');
  });

  it('knows the functions people use, with commas or semicolons', () => {
    expect(worked('=sum(A1,A2)')).toBe('4');
    expect(worked('=SUM(A1;A2)')).toBe('4');
    expect(worked('=AVERAGE(A1:A2)')).toBe('2');
    expect(worked('=MIN(A1:B2)')).toBe('1');
    expect(worked('=MAX(A1:B2)')).toBe('3');
    expect(worked('=ROUND(2.345,2)')).toBe('2.35');
  });
});

describe('csv', () => {
  it('makes a start that a spreadsheet would run harmless', () => {
    for (const value of [
      '=1+1',
      '+cmd',
      '-cmd',
      '@SUM(A1)',
      '\tx',
      '\rx',
      '\nx',
      '＝1',
      '＋1',
      '－1',
      '＠1',
    ]) {
      expect(csvField(value).startsWith(`"'`), value).toBe(true);
    }
  });

  it('leaves a real number and plain text as they are', () => {
    expect(csvField('-5.5')).toBe('"-5.5"');
    expect(csvField('Hello')).toBe('"Hello"');
  });

  it('quotes every field, and doubles a quote inside', () => {
    expect(csvField('a"b,c')).toBe('"a""b,c"');
  });

  it('writes what a cell shows, so no formula leaves', () => {
    const file = sheetToCsv(sheetOf([['2', '=A1*2']]));
    expect(file).toBe('"C0","C1"\r\n"2","4"\r\n');
  });

  it('reads quotes, separators and line breaks inside a field', () => {
    expect(parseCsv('a,"b,""c""\nd"\r\n1,2\n')).toEqual([
      ['a', 'b,"c"\nd'],
      ['1', '2'],
    ]);
  });

  it('refuses a file that is too large for a sheet', () => {
    const wide = Array.from({ length: 101 }, (_, i) => `c${i}`).join(',');
    expect(csvToSheet(wide)).toBeNull();
    expect(csvToSheet(`a\n${'1\n'.repeat(5001)}`)).toBeNull();
    expect(csvToSheet('')).toBeNull();
  });

  it('never makes a formula of a value in a file', () => {
    const sheet = csvToSheet('a\n=1+1\n');
    const column = sheet?.columns[0]?.id ?? '';
    expect(sheet?.rows[0]?.cells[column]).toBe('1+1');
  });

  it('takes the first line as columns, finds numbers and knows semicolons', () => {
    const sheet = csvToSheet('﻿Name;Sum\nTea;2\nCake;3.5\n');
    expect(sheet?.columns.map((c) => [c.name, c.type])).toEqual([
      ['Name', 'text'],
      ['Sum', 'number'],
    ]);
    expect(sheet?.rows).toHaveLength(2);
  });

  it('reads a block copied from Excel', () => {
    expect(parsePasted('a\tb\r\n1\t2\r\n')).toEqual([
      ['a', 'b'],
      ['1', '2'],
    ]);
  });
});

describe('the sheet', () => {
  const link = { id: 'l', name: 'L', type: 'link' as const };
  const date = { id: 'd', name: 'D', type: 'date' as const };
  const check = { id: 'k', name: 'K', type: 'check' as const };

  it('refuses what the server would refuse', () => {
    expect(fitsColumn(link, 'javascript:alert(1)')).toBe(false);
    expect(fitsColumn(link, '=A1')).toBe(false);
    expect(fitsColumn(link, '//elsewhere.example')).toBe(false);
    expect(fitsColumn(date, '6-11-2026')).toBe(false);
    expect(fitsColumn(date, '2026-13-01')).toBe(false);
    expect(fitsColumn(check, 'yes')).toBe(false);
  });

  it('accepts what fits, and an empty cell always', () => {
    expect(fitsColumn(link, 'https://example.com')).toBe(true);
    expect(fitsColumn(date, '2026-11-06')).toBe(true);
    expect(fitsColumn(date, '=A1')).toBe(true);
    expect(fitsColumn(check, '')).toBe(true);
    expect(isSafeLink('/pages/1')).toBe(true);
  });

  it('clears values that no longer fit a changed column', () => {
    const sheet = sheetOf([['a'], ['2026-11-06']]);
    const first = sheet.columns[0];
    if (!first) throw new Error('no column');
    const changed = changeColumn(sheet, { ...first, type: 'date' });
    expect(changed.rows[0]?.cells.c0).toBeUndefined();
    expect(changed.rows[1]?.cells.c0).toBe('2026-11-06');
  });

  it('keeps the last column', () => {
    expect(removeColumn(sheetOf([['a']]), 'c0').columns).toHaveLength(1);
  });

  it('does not move a row off the sheet', () => {
    const sheet = sheetOf([['a'], ['b']]);
    expect(moveRow(sheet, 'r0', -1).rows[0]?.id).toBe('r0');
    expect(moveRow(sheet, 'r0', 1).rows[0]?.id).toBe('r1');
  });

  it('adds rows for a paste, and counts what did not fit', () => {
    const sheet: Sheet = { columns: [date], rows: [] };
    const pasted = pasteBlock(sheet, [['2026-11-06', 'extra'], ['nonsense']], { row: 0, column: 0 });
    expect(pasted.sheet.rows).toHaveLength(2);
    expect(pasted.refused).toBe(2);
  });

  it('stops at the row limit', () => {
    const full: Sheet = {
      columns: [date],
      rows: Array.from({ length: 5000 }, (_, i) => ({ id: `r${i}`, cells: {} })),
    };
    expect(addRow(full, 0).rows).toHaveLength(5000);
  });

  it('names columns as a spreadsheet does', () => {
    expect(columnLetter(0)).toBe('A');
    expect(columnLetter(25)).toBe('Z');
    expect(columnLetter(26)).toBe('AA');
  });

  it('comes back unchanged from a document', () => {
    const sheet = sheetOf([['a', 'b']]);
    expect(sheetOfDocument(documentOfSheet(sheet))).toEqual(sheet);
  });

  it('reads a broken document as an empty sheet', () => {
    expect(sheetOfDocument(null)).toEqual({ columns: [], rows: [] });
    expect(sheetOfDocument({ content: [{ attrs: { columns: 'x', rows: [1] } }] })).toEqual({
      columns: [],
      rows: [],
    });
  });
});
