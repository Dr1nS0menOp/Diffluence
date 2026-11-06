// SPDX-License-Identifier: Apache-2.0
// The data of a database: columns and rows, as the document holds them. Every change
// gives a new sheet, so undo is a list of old ones.

export type ColumnType = 'text' | 'number' | 'date' | 'check' | 'choice' | 'link';

export const COLUMN_TYPES: ColumnType[] = ['text', 'number', 'date', 'check', 'choice', 'link'];

export interface SheetColumn {
  id: string;
  name: string;
  type: ColumnType;
  options?: string[];
}

export interface SheetRow {
  id: string;
  cells: Record<string, string>;
}

export interface Sheet {
  columns: SheetColumn[];
  rows: SheetRow[];
}

export const MAX_COLUMNS = 100;
export const MAX_ROWS = 5000;

const ID_BYTES = 6;
const SAFE_LINK = /^(https?:\/\/|mailto:|\/(?!\/))/i;
const DATE = /^\d{4}-\d{2}-\d{2}$/;
const FORMULA_START = '=';

/** A new id for a row or a column. */
export function newId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(ID_BYTES));
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null;
}

function columnOf(value: unknown): SheetColumn | null {
  if (!isRecord(value) || typeof value.id !== 'string' || typeof value.name !== 'string') return null;
  const type = COLUMN_TYPES.find((name) => name === value.type) ?? 'text';
  const options = Array.isArray(value.options)
    ? value.options.filter((o): o is string => typeof o === 'string')
    : undefined;
  return { id: value.id, name: value.name, type, ...(options ? { options } : {}) };
}

function rowOf(value: unknown): SheetRow | null {
  if (!isRecord(value) || typeof value.id !== 'string') return null;
  const cells: Record<string, string> = {};
  if (isRecord(value.cells)) {
    for (const [id, cell] of Object.entries(value.cells)) {
      if (typeof cell === 'string') cells[id] = cell;
    }
  }
  return { id: value.id, cells };
}

/** Reads the attributes of a sheet block. Anything unreadable is left out. */
export function sheetOfAttrs(attrs: unknown): Sheet {
  if (!isRecord(attrs)) return { columns: [], rows: [] };
  const columns = Array.isArray(attrs.columns) ? attrs.columns : [];
  const rows = Array.isArray(attrs.rows) ? attrs.rows : [];
  return {
    columns: columns.map(columnOf).filter((c) => c !== null),
    rows: rows.map(rowOf).filter((r) => r !== null),
  };
}

/** The sheet of a stored document. */
export function sheetOfDocument(document: unknown): Sheet {
  if (!isRecord(document) || !Array.isArray(document.content)) return { columns: [], rows: [] };
  const block: unknown = document.content[0];
  return sheetOfAttrs(isRecord(block) ? block.attrs : null);
}

/** The document that holds a sheet. */
export function documentOfSheet(sheet: Sheet) {
  return { type: 'doc', content: [{ type: 'sheet', attrs: { columns: sheet.columns, rows: sheet.rows } }] };
}

/** Column letters as a spreadsheet has them: A to Z, then AA. */
export function columnLetter(index: number): string {
  const letters = 26;
  const first = 'A'.charCodeAt(0);
  let rest = index;
  let name = '';
  do {
    name = String.fromCharCode(first + (rest % letters)) + name;
    rest = Math.floor(rest / letters) - 1;
  } while (rest >= 0);
  return name;
}

export function isFormula(value: string): boolean {
  return value.startsWith(FORMULA_START);
}

/** Whether a link can be shown as a link. The server holds the real rule. */
export function isSafeLink(value: string): boolean {
  return SAFE_LINK.test(value);
}

function isRealDate(value: string): boolean {
  return DATE.test(value) && !Number.isNaN(Date.parse(`${value}T00:00:00Z`));
}

/** Whether a value fits its column, as the server will check. */
export function fitsColumn(column: SheetColumn, value: string): boolean {
  if (value === '') return true;
  if (column.type === 'link') return isSafeLink(value);
  if (isFormula(value)) return true;
  if (column.type === 'date') return isRealDate(value);
  if (column.type === 'check') return value === 'true';
  return true;
}

export function setCell(sheet: Sheet, rowId: string, columnId: string, value: string): Sheet {
  const rows = sheet.rows.map((row) => {
    if (row.id !== rowId) return row;
    const cells = { ...row.cells };
    if (value === '') delete cells[columnId];
    else cells[columnId] = value;
    return { ...row, cells };
  });
  return { ...sheet, rows };
}

export function addRow(sheet: Sheet, at: number): Sheet {
  if (sheet.rows.length >= MAX_ROWS) return sheet;
  const rows = [...sheet.rows];
  rows.splice(at, 0, { id: newId(), cells: {} });
  return { ...sheet, rows };
}

export function removeRow(sheet: Sheet, rowId: string): Sheet {
  return { ...sheet, rows: sheet.rows.filter((row) => row.id !== rowId) };
}

function moved<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length) return list;
  const out = [...list];
  const [item] = out.splice(from, 1);
  if (item !== undefined) out.splice(to, 0, item);
  return out;
}

export function moveRow(sheet: Sheet, rowId: string, by: number): Sheet {
  const from = sheet.rows.findIndex((row) => row.id === rowId);
  return { ...sheet, rows: moved(sheet.rows, from, from + by) };
}

export function addColumn(sheet: Sheet, at: number, name: string): Sheet {
  if (sheet.columns.length >= MAX_COLUMNS) return sheet;
  const columns = [...sheet.columns];
  columns.splice(at, 0, { id: newId(), name, type: 'text' });
  return { ...sheet, columns };
}

/** Removes a column and what it held. The last column stays: a sheet needs one. */
export function removeColumn(sheet: Sheet, columnId: string): Sheet {
  if (sheet.columns.length <= 1) return sheet;
  const rows = sheet.rows.map((row) => {
    const cells = { ...row.cells };
    delete cells[columnId];
    return { ...row, cells };
  });
  return { columns: sheet.columns.filter((column) => column.id !== columnId), rows };
}

export function moveColumn(sheet: Sheet, columnId: string, by: number): Sheet {
  const from = sheet.columns.findIndex((column) => column.id === columnId);
  return { ...sheet, columns: moved(sheet.columns, from, from + by) };
}

/** Changes a column. Values that no longer fit its type are cleared, or the server refuses the sheet. */
export function changeColumn(sheet: Sheet, changed: SheetColumn): Sheet {
  const columns = sheet.columns.map((column) => (column.id === changed.id ? changed : column));
  const rows = sheet.rows.map((row) => {
    const value = row.cells[changed.id];
    if (value === undefined || fitsColumn(changed, value)) return row;
    const cells = { ...row.cells };
    delete cells[changed.id];
    return { ...row, cells };
  });
  return { columns, rows };
}

/** Puts a block of pasted text in the sheet from a cell, with new rows when needed. */
export function pasteBlock(
  sheet: Sheet,
  block: string[][],
  from: { row: number; column: number },
): { sheet: Sheet; refused: number } {
  let next = sheet;
  let refused = 0;
  while (next.rows.length < Math.min(from.row + block.length, MAX_ROWS)) {
    next = addRow(next, next.rows.length);
  }
  block.forEach((line, r) => {
    const row = next.rows[from.row + r];
    line.forEach((value, c) => {
      const column = next.columns[from.column + c];
      if (!row || !column || !fitsColumn(column, value)) {
        refused += 1;
        return;
      }
      next = setCell(next, row.id, column.id, value);
    });
  });
  return { sheet: next, refused };
}
