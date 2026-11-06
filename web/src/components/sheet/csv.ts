// SPDX-License-Identifier: Apache-2.0
// A sheet to CSV and back. A cell that a spreadsheet would run as a formula is made
// harmless on the way out: OWASP's advice, and it warns no way works everywhere.
import { evaluator } from './formula';
import { MAX_COLUMNS, MAX_ROWS, newId, type Sheet, type SheetColumn } from './model';

const DANGEROUS_START = /^[=+\-@\t\r\n＝＋－＠]/;
const PLAIN_NUMBER = /^[-+]?\d+(\.\d+)?$/;

/** Quotes one value, and puts ' before a start that could run. A real number stays a number. */
export function csvField(value: string): string {
  const guarded = DANGEROUS_START.test(value) && !PLAIN_NUMBER.test(value) ? `'${value}` : value;
  return `"${guarded.replaceAll('"', '""')}"`;
}

/** The file of a sheet: what its cells show, so no formula leaves. */
export function sheetToCsv(sheet: Sheet): string {
  const read = evaluator(sheet);
  const header = sheet.columns.map((column) => csvField(column.name)).join(',');
  const lines = sheet.rows.map((_, r) => sheet.columns.map((__, c) => csvField(read(r, c))).join(','));
  return `${[header, ...lines].join('\r\n')}\r\n`;
}

/** Splits CSV text into lines of fields, by the rules of RFC 4180. */
export function parseCsv(text: string, separator = ','): string[][] {
  const lines: string[][] = [];
  let line: string[] = [];
  let field = '';
  let quoted = false;
  const endField = () => {
    line.push(field);
    field = '';
  };
  const endLine = () => {
    endField();
    lines.push(line);
    line = [];
  };
  for (let i = 0; i < text.length; i += 1) {
    const c = text.charAt(i);
    if (quoted) {
      if (c === '"' && text.charAt(i + 1) === '"') {
        field += '"';
        i += 1;
      } else if (c === '"') quoted = false;
      else field += c;
    } else if (c === '"' && field === '') quoted = true;
    else if (c === separator) endField();
    else if (c === '\n') endLine();
    else if (c !== '\r') field += c;
  }
  if (field !== '' || line.length > 0) endLine();
  return lines;
}

/** The separator of a file: Excel in a Dutch setting writes semicolons. */
function separatorOf(text: string): string {
  const firstLine = text.split('\n', 1)[0] ?? '';
  return firstLine.split(';').length > firstLine.split(',').length ? ';' : ',';
}

function typeOf(values: string[]): SheetColumn['type'] {
  const filled = values.filter((value) => value !== '');
  return filled.length > 0 && filled.every((value) => PLAIN_NUMBER.test(value)) ? 'number' : 'text';
}

/** A new sheet from a file: the first line names the columns. A formula stays text. */
export function csvToSheet(text: string): Sheet | null {
  const [header, ...lines] = parseCsv(text.replace(/^﻿/, ''), separatorOf(text));
  if (!header || header.length === 0 || header.length > MAX_COLUMNS || lines.length > MAX_ROWS) return null;
  const columns: SheetColumn[] = header.map((name, i) => ({
    id: newId(),
    name: name.trim() || `Column ${i + 1}`,
    type: typeOf(lines.map((line) => line[i] ?? '')),
  }));
  const rows = lines.map((line) => {
    const cells: Record<string, string> = {};
    columns.forEach((column, i) => {
      const value = (line[i] ?? '').replace(/^=+/, '');
      if (value !== '') cells[column.id] = value;
    });
    return { id: newId(), cells };
  });
  return { columns, rows };
}

/** A block copied from a spreadsheet: tabs and new lines. */
export function parsePasted(text: string): string[][] {
  return parseCsv(text.replace(/\r?\n$/, ''), '\t');
}
