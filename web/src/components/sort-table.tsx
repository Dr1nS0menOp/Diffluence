// SPDX-License-Identifier: Apache-2.0
import { useMemo, useState, type ReactNode } from 'react';

export interface SortColumn<Row> {
  key: string;
  label: string;
  value: (row: Row) => string | number;
  show?: (row: Row) => ReactNode;
}

/** Sorts rows by one column. Text follows the language, numbers follow size. */
export function sortRows<Row>(rows: Row[], value: (row: Row) => string | number, ascending: boolean): Row[] {
  const direction = ascending ? 1 : -1;
  return [...rows].sort((a, b) => {
    const x = value(a);
    const y = value(b);
    const order = typeof x === 'number' && typeof y === 'number' ? x - y : String(x).localeCompare(String(y));
    return order * direction;
  });
}

/** A table with a button in each header, whose state is known to everyone (WCAG 1.3.1, 4.1.2). */
export function SortTable<Row>({
  caption,
  columns,
  rows,
  rowKey,
}: {
  caption: string;
  columns: SortColumn<Row>[];
  rows: Row[];
  rowKey: (row: Row) => string;
}) {
  const [sorted, setSorted] = useState<{ key: string; ascending: boolean } | null>(null);
  const shown = useMemo(() => {
    const column = columns.find((c) => c.key === sorted?.key);
    return column && sorted ? sortRows(rows, column.value, sorted.ascending) : rows;
  }, [columns, rows, sorted]);
  const stateOf = (key: string) => {
    if (sorted?.key !== key) return 'none';
    return sorted.ascending ? 'ascending' : 'descending';
  };
  return (
    <table>
      <caption className="visually-hidden">{caption}</caption>
      <thead>
        <tr>
          {columns.map((c) => (
            <th key={c.key} scope="col" aria-sort={stateOf(c.key)}>
              <button
                type="button"
                className="ghost"
                onClick={() =>
                  setSorted({ key: c.key, ascending: sorted?.key === c.key ? !sorted.ascending : true })
                }
              >
                {c.label}
              </button>
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {shown.map((row) => (
          <tr key={rowKey(row)}>
            {columns.map((c) => (
              <td key={c.key}>{c.show ? c.show(row) : c.value(row)}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
