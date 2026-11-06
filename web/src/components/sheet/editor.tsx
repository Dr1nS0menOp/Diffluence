// SPDX-License-Identifier: Apache-2.0
// A database up close: the grid, with the tools for its rows and columns, and the
// way out to a file. SheetEditor changes it, SheetReader only looks.
import { useCallback, useId, useMemo, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { CraftDialog } from '../craft-frame';
import { ConfirmDialog, Dialog, ErrorNote, Field } from '../ui';
import { csvToSheet, sheetToCsv } from './csv';
import { SheetGrid, type ActiveCell } from './grid';
import {
  addColumn,
  addRow,
  changeColumn,
  COLUMN_TYPES,
  documentOfSheet,
  moveColumn,
  moveRow,
  removeColumn,
  removeRow,
  sheetOfDocument,
  type ColumnType,
  type Sheet,
  type SheetColumn,
} from './model';

const CSV_TYPE = 'text/csv';
const BYTE_ORDER_MARK = '﻿';

function csvBytes(sheet: Sheet): ArrayBuffer {
  const bytes = new TextEncoder().encode(`${BYTE_ORDER_MARK}${sheetToCsv(sheet)}`);
  return bytes.buffer.slice(0, bytes.byteLength) as ArrayBuffer;
}

function downloadCsv(sheet: Sheet, title: string) {
  const url = URL.createObjectURL(new Blob([csvBytes(sheet)], { type: CSV_TYPE }));
  const link = document.createElement('a');
  link.href = url;
  link.download = `${title}.csv`;
  link.click();
  URL.revokeObjectURL(url);
}

/** Export CSV and Open in GridCraft. Both hand out the data, so both need the right to export. */
function FileTools({ sheet, title }: { sheet: Sheet; title: string }) {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const close = useCallback(() => setIsOpen(false), []);
  const file = useMemo(
    () => ({
      name: `${title}.csv`,
      type: CSV_TYPE,
      load: () => Promise.resolve(csvBytes(sheet)),
    }),
    [sheet, title],
  );
  return (
    <>
      <button type="button" className="quiet" onClick={() => downloadCsv(sheet, title)}>
        {t('sheet.export')}
      </button>
      <button type="button" className="quiet" onClick={() => setIsOpen(true)}>
        {t('craft.open.gridcraft')}
      </button>
      {isOpen && <CraftDialog craft="gridcraft" file={file} onClose={close} />}
    </>
  );
}

function ColumnDialog({
  column,
  onSave,
  onMove,
  onRemove,
  onClose,
}: {
  column: SheetColumn;
  onSave: (changed: SheetColumn) => void;
  onMove: (by: number) => void;
  onRemove: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(column.name);
  const [type, setType] = useState<ColumnType>(column.type);
  const [options, setOptions] = useState((column.options ?? []).join('\n'));
  const submit = (e: FormEvent) => {
    e.preventDefault();
    const list = options
      .split('\n')
      .map((option) => option.trim())
      .filter(Boolean);
    onSave({ id: column.id, name: name.trim(), type, ...(type === 'choice' ? { options: list } : {}) });
    onClose();
  };
  return (
    <Dialog title={t('sheet.column')} onClose={onClose}>
      <form onSubmit={submit}>
        <Field label={t('sheet.columnName')}>
          <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={100} />
        </Field>
        <Field label={t('sheet.columnType')}>
          <select
            value={type}
            onChange={(e) => setType(COLUMN_TYPES.find((c) => c === e.target.value) ?? 'text')}
          >
            {COLUMN_TYPES.map((name) => (
              <option key={name} value={name}>
                {t(`sheet.type_${name}`)}
              </option>
            ))}
          </select>
        </Field>
        {type === 'choice' && (
          <Field label={t('sheet.columnOptions')}>
            <textarea value={options} onChange={(e) => setOptions(e.target.value)} rows={5} />
          </Field>
        )}
        <div className="dialog-actions">
          <button type="button" className="quiet" onClick={() => onMove(-1)}>
            {t('sheet.moveLeft')}
          </button>
          <button type="button" className="quiet" onClick={() => onMove(1)}>
            {t('sheet.moveRight')}
          </button>
          <button
            type="button"
            className="danger"
            onClick={() => {
              onRemove();
              onClose();
            }}
          >
            {t('sheet.removeColumn')}
          </button>
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** The tools that change a sheet, working on the cell that has focus. */
function EditTools({
  sheet,
  active,
  onChange,
}: {
  sheet: Sheet;
  active: ActiveCell;
  onChange: (next: Sheet) => void;
}) {
  const { t } = useTranslation();
  const [editingColumn, setEditingColumn] = useState(false);
  const [imported, setImported] = useState<Sheet | null>(null);
  const [error, setError] = useState<string | null>(null);
  const rowIndex = sheet.rows.findIndex((row) => row.id === active.rowId);
  const columnIndex = sheet.columns.findIndex((column) => column.id === active.columnId);
  const column = sheet.columns[columnIndex];

  const importFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    const read = csvToSheet(await file.text());
    setError(read ? null : t('sheet.importRefused'));
    setImported(read);
  };

  return (
    <>
      <button
        type="button"
        className="quiet"
        onClick={() => onChange(addRow(sheet, rowIndex < 0 ? sheet.rows.length : rowIndex + 1))}
      >
        {t('sheet.addRow')}
      </button>
      <button
        type="button"
        className="quiet"
        onClick={() =>
          onChange(
            addColumn(sheet, columnIndex + 1, t('sheet.newColumn', { number: sheet.columns.length + 1 })),
          )
        }
      >
        {t('sheet.addColumn')}
      </button>
      <button type="button" className="quiet" disabled={!column} onClick={() => setEditingColumn(true)}>
        {t('sheet.column')}
      </button>
      <button
        type="button"
        className="quiet"
        disabled={rowIndex < 0}
        onClick={() => active.rowId && onChange(removeRow(sheet, active.rowId))}
      >
        {t('sheet.removeRow')}
      </button>
      <button
        type="button"
        className="quiet"
        disabled={rowIndex <= 0}
        onClick={() => active.rowId && onChange(moveRow(sheet, active.rowId, -1))}
      >
        {t('sheet.moveUp')}
      </button>
      <button
        type="button"
        className="quiet"
        disabled={rowIndex < 0 || rowIndex >= sheet.rows.length - 1}
        onClick={() => active.rowId && onChange(moveRow(sheet, active.rowId, 1))}
      >
        {t('sheet.moveDown')}
      </button>
      <label className="button quiet">
        {t('sheet.import')}
        <input
          type="file"
          className="visually-hidden"
          accept=".csv,text/csv"
          onChange={(e) => void importFile(e)}
        />
      </label>
      <ErrorNote error={error ? new Error(error) : null} />
      {editingColumn && column && (
        <ColumnDialog
          column={column}
          onSave={(changed) => onChange(changeColumn(sheet, changed))}
          onMove={(by) => onChange(moveColumn(sheet, column.id, by))}
          onRemove={() => onChange(removeColumn(sheet, column.id))}
          onClose={() => setEditingColumn(false)}
        />
      )}
      {imported && (
        <ConfirmDialog
          title={t('sheet.import')}
          message={t('sheet.importConfirm', { count: imported.rows.length })}
          confirmLabel={t('sheet.importReplace')}
          onConfirm={() => onChange(imported)}
          onClose={() => setImported(null)}
        />
      )}
    </>
  );
}

export function SheetReader({
  document,
  title,
  canExport,
}: {
  document: unknown;
  title: string;
  canExport: boolean;
}) {
  const id = useId();
  const sheet = useMemo(() => sheetOfDocument(document), [document]);
  return (
    <SheetGrid
      id={id}
      label={title}
      sheet={sheet}
      tools={canExport ? <FileTools sheet={sheet} title={title} /> : undefined}
    />
  );
}

export function SheetEditor({
  initial,
  title,
  onChange,
}: {
  initial: unknown;
  title: string;
  onChange: (document: ReturnType<typeof documentOfSheet>) => void;
}) {
  const id = useId();
  const [sheet, setSheet] = useState(() => sheetOfDocument(initial));
  const [active, setActive] = useState<ActiveCell>({ rowId: null, columnId: null });
  const report = useRef(onChange);
  report.current = onChange;
  const change = useCallback((next: Sheet) => {
    setSheet(next);
    report.current(documentOfSheet(next));
  }, []);
  return (
    <SheetGrid
      id={id}
      label={title}
      sheet={sheet}
      onChange={change}
      onActive={setActive}
      tools={<EditTools sheet={sheet} active={active} onChange={change} />}
    />
  );
}
