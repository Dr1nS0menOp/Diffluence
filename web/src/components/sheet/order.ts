// SPDX-License-Identifier: Apache-2.0
// Sort and filter are a reader's view: they give row numbers, and change nothing stored.
import type { Sheet } from './model';

export interface SheetSort {
  columnId: string;
  descending: boolean;
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });

function compareValues(a: string, b: string): number {
  if (a === '' || b === '') return Number(a === '') - Number(b === '');
  const x = Number(a);
  const y = Number(b);
  if (!Number.isNaN(x) && !Number.isNaN(y)) return x - y;
  return collator.compare(a, b);
}

/** The stored indexes of the rows to show, in the order to show them. Empty cells go last. */
export function orderedRows(
  sheet: Sheet,
  read: (row: number, column: number) => string,
  sort: SheetSort | null,
  filter: string,
): number[] {
  const words = filter.trim().toLowerCase();
  const columns = sheet.columns.map((_, i) => i);
  const indexes = sheet.rows
    .map((_, i) => i)
    .filter((r) => words === '' || columns.some((c) => read(r, c).toLowerCase().includes(words)));
  if (!sort) return indexes;
  const column = sheet.columns.findIndex((c) => c.id === sort.columnId);
  if (column < 0) return indexes;
  const direction = sort.descending ? -1 : 1;
  return indexes.sort((a, b) => {
    const x = read(a, column);
    const y = read(b, column);
    if (x === '' || y === '') return compareValues(x, y);
    return compareValues(x, y) * direction;
  });
}
