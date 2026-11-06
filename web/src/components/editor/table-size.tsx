// SPDX-License-Identifier: Apache-2.0
// How big a new table is (EDT-086). 3 by 3 is ready, so Enter is as fast as a plain insert.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import { Dialog, Field } from '../ui';

const DEFAULT_SIZE = 3;
const GRID_SIZE = 8;
const MAX_SIZE = 50;
const GRID_STEPS = Array.from({ length: GRID_SIZE }, (_, i) => i + 1);

export function TableSizeDialog({ editor, onClose }: { editor: Editor; onClose: () => void }) {
  const { t } = useTranslation();
  const [rows, setRows] = useState(DEFAULT_SIZE);
  const [columns, setColumns] = useState(DEFAULT_SIZE);
  const [withHeader, setWithHeader] = useState(true);
  const insert = (rowCount: number, columnCount: number) => {
    onClose();
    editor
      .chain()
      .focus()
      .insertTable({ rows: rowCount, cols: columnCount, withHeaderRow: withHeader })
      .run();
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    insert(rows, columns);
  };
  return (
    <Dialog title={t('table.newTitle')} onClose={onClose}>
      <form onSubmit={submit}>
        {/* A faster way for a mouse. The fields below do the same for everyone. */}
        <div className="size-grid" aria-hidden="true">
          {GRID_STEPS.flatMap((row) =>
            GRID_STEPS.map((column) => (
              <button
                key={`${row}-${column}`}
                type="button"
                tabIndex={-1}
                className={row <= rows && column <= columns ? 'size-cell on' : 'size-cell'}
                onMouseEnter={() => {
                  setRows(row);
                  setColumns(column);
                }}
                onClick={() => insert(row, column)}
              />
            )),
          )}
        </div>
        <div className="row">
          <Field label={t('table.rows')}>
            <input
              autoFocus
              type="number"
              min={1}
              max={MAX_SIZE}
              value={rows}
              onChange={(e) => setRows(Number(e.target.value))}
              required
            />
          </Field>
          <Field label={t('table.columns')}>
            <input
              type="number"
              min={1}
              max={MAX_SIZE}
              value={columns}
              onChange={(e) => setColumns(Number(e.target.value))}
              required
            />
          </Field>
        </div>
        <label className="check">
          <input type="checkbox" checked={withHeader} onChange={(e) => setWithHeader(e.target.checked)} />
          {t('table.headerRow')}
        </label>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('table.insert')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
