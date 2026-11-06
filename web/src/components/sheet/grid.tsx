// SPDX-License-Identifier: Apache-2.0
// The grid of a database, by the WAI-ARIA grid pattern with the keys of a spreadsheet.
// Without onChange it is for reading: sort and filter still work, nothing is stored.
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ClipboardEvent,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { IconChevronDown, IconTick } from '../icons';
import { parsePasted } from './csv';
import { CYCLE_VALUE, ERROR_VALUE, evaluator } from './formula';
import {
  addRow,
  columnLetter,
  fitsColumn,
  isSafeLink,
  pasteBlock,
  setCell,
  type Sheet,
  type SheetColumn,
} from './model';
import { orderedRows, type SheetSort } from './order';

const ROWS_PER_STEP = 200;
const PAGE_ROWS = 20;
const UNDO_STEPS = 100;
const HELP_ID_SUFFIX = '-help';

interface Position {
  row: number;
  column: number;
}

function CellValue({ column, value }: { column: SheetColumn; value: string }) {
  const { t } = useTranslation();
  if (column.type === 'check') {
    return value === 'true' ? (
      <span role="img" aria-label={t('sheet.ticked')}>
        <IconTick size={16} />
      </span>
    ) : null;
  }
  if (column.type === 'link' && isSafeLink(value)) {
    return (
      <a href={value} target="_blank" rel="noopener noreferrer nofollow" tabIndex={-1}>
        {value}
      </a>
    );
  }
  if (column.type === 'choice' && value) return <span className="lozenge">{value}</span>;
  return <>{value}</>;
}

function CellInput({
  column,
  value,
  label,
  invalid,
  onInput,
  onKey,
  onBlur,
}: {
  column: SheetColumn;
  value: string;
  label: string;
  invalid: boolean;
  onInput: (value: string) => void;
  onKey: (e: KeyboardEvent) => void;
  onBlur: () => void;
}) {
  const ref = useRef<HTMLInputElement & HTMLSelectElement>(null);
  useEffect(() => ref.current?.focus(), []);
  if (column.type === 'choice') {
    return (
      <select
        ref={ref}
        aria-label={label}
        value={value}
        onChange={(e) => onInput(e.target.value)}
        onKeyDown={onKey}
        onBlur={onBlur}
      >
        <option value="" />
        {column.options?.map((option) => (
          <option key={option}>{option}</option>
        ))}
      </select>
    );
  }
  return (
    <input
      ref={ref}
      aria-label={label}
      aria-invalid={invalid}
      value={value}
      onChange={(e) => onInput(e.target.value)}
      onKeyDown={onKey}
      onBlur={onBlur}
    />
  );
}

/** Tells the grid's owner which cell has focus, for the buttons above it. */
export interface ActiveCell {
  rowId: string | null;
  columnId: string | null;
}

export function SheetGrid({
  id,
  label,
  sheet,
  onChange,
  onActive,
  tools,
}: {
  id: string;
  label: string;
  sheet: Sheet;
  onChange?: (next: Sheet) => void;
  onActive?: (active: ActiveCell) => void;
  tools?: ReactNode;
}) {
  const { t } = useTranslation();
  const [active, setActive] = useState<Position>({ row: 0, column: 0 });
  const [draft, setDraft] = useState<string | null>(null);
  const [sort, setSort] = useState<SheetSort | null>(null);
  const [filter, setFilter] = useState('');
  const [shownRows, setShownRows] = useState(ROWS_PER_STEP);
  const [note, setNote] = useState('');
  const hasFocus = useRef(false);
  const table = useRef<HTMLTableElement>(null);
  const undone = useRef<Sheet[]>([]);
  const past = useRef<Sheet[]>([]);

  const read = useMemo(() => evaluator(sheet), [sheet]);
  const order = useMemo(() => orderedRows(sheet, read, sort, filter), [sheet, read, sort, filter]);
  const shown = order.slice(0, shownRows);
  const canEdit = onChange !== undefined;
  const row = Math.min(active.row, Math.max(order.length - 1, 0));
  const column = Math.min(active.column, Math.max(sheet.columns.length - 1, 0));
  const activeRowIndex = order[row];
  const activeRow = activeRowIndex === undefined ? undefined : sheet.rows[activeRowIndex];
  const activeColumn = sheet.columns[column];
  const isEditing = draft !== null;

  useEffect(() => {
    onActive?.({ rowId: activeRow?.id ?? null, columnId: activeColumn?.id ?? null });
  }, [onActive, activeRow?.id, activeColumn?.id]);

  // Focus follows the active cell, but a grid that was not in use never takes it.
  useEffect(() => {
    if (!hasFocus.current || isEditing) return;
    table.current?.querySelector<HTMLElement>('[data-active="true"]')?.focus();
  }, [row, column, isEditing]);

  const change = (next: Sheet) => {
    past.current = [...past.current.slice(-UNDO_STEPS), sheet];
    undone.current = [];
    onChange?.(next);
  };

  const undo = () => {
    const last = past.current.at(-1);
    if (!last) return;
    past.current = past.current.slice(0, -1);
    undone.current = [...undone.current, sheet];
    onChange?.(last);
  };

  const redo = () => {
    const last = undone.current.at(-1);
    if (!last) return;
    undone.current = undone.current.slice(0, -1);
    past.current = [...past.current, sheet];
    onChange?.(last);
  };

  const moveTo = (nextRow: number, nextColumn: number) => {
    const lastRow = Math.max(Math.min(order.length, shownRows) - 1, 0);
    setActive({
      row: Math.min(Math.max(nextRow, 0), lastRow),
      column: Math.min(Math.max(nextColumn, 0), sheet.columns.length - 1),
    });
  };

  const rawValue = () => (activeRow && activeColumn ? (activeRow.cells[activeColumn.id] ?? '') : '');

  /** The sheet with what was typed, or null when it does not fit, so the cell stays open. */
  const typedSheet = (): Sheet | null => {
    if (draft === null || !activeRow || !activeColumn || draft === rawValue()) return sheet;
    if (fitsColumn(activeColumn, draft)) return setCell(sheet, activeRow.id, activeColumn.id, draft);
    setNote(t(`sheet.notFit_${activeColumn.type}`));
    return null;
  };

  const commit = (): boolean => {
    const next = typedSheet();
    if (!next) return false;
    setNote('');
    setDraft(null);
    if (next !== sheet) change(next);
    return true;
  };

  /** Enter on the last row goes on to a new one, as a spreadsheet never ends. */
  const commitAndGrow = () => {
    const next = typedSheet();
    if (!next) return;
    setNote('');
    setDraft(null);
    change(addRow(next, next.rows.length));
    setActive({ row: row + 1, column });
  };

  const tick = () => {
    if (!activeRow || !activeColumn) return;
    change(setCell(sheet, activeRow.id, activeColumn.id, rawValue() === 'true' ? '' : 'true'));
  };

  const startEditing = (value: string) => {
    if (!canEdit || !activeColumn) return;
    if (activeColumn.type === 'check') tick();
    else setDraft(value);
  };

  const onEditKey = (e: KeyboardEvent) => {
    e.stopPropagation();
    if (e.key === 'Escape') {
      e.preventDefault();
      setDraft(null);
      setNote('');
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault();
      const isLastRow = row === sheet.rows.length - 1 && order.length === sheet.rows.length && !sort;
      if (e.key === 'Enter' && !e.shiftKey && isLastRow) {
        commitAndGrow();
        return;
      }
      if (!commit()) return;
      if (e.key === 'Enter') moveTo(row + (e.shiftKey ? -1 : 1), column);
      else moveTo(row, column + (e.shiftKey ? -1 : 1));
    }
  };

  const shortcut = (e: KeyboardEvent): boolean => {
    const key = e.key.toLowerCase();
    if (key === 'z' && canEdit) {
      if (e.shiftKey) redo();
      else undo();
    } else if (key === 'y' && canEdit) redo();
    else if (e.key === 'Home') moveTo(0, 0);
    else if (e.key === 'End') moveTo(order.length - 1, sheet.columns.length - 1);
    else return false;
    return true;
  };

  /** Tab goes right as in a spreadsheet, and leaves at the edge, so the keyboard is never stuck. */
  const onTab = (e: KeyboardEvent) => {
    if (!canEdit) return;
    const next = column + (e.shiftKey ? -1 : 1);
    if (next < 0 || next >= sheet.columns.length) return;
    e.preventDefault();
    moveTo(row, next);
  };

  const moves: Record<string, () => void> = {
    ArrowUp: () => moveTo(row - 1, column),
    ArrowDown: () => moveTo(row + 1, column),
    ArrowLeft: () => moveTo(row, column - 1),
    ArrowRight: () => moveTo(row, column + 1),
    Home: () => moveTo(row, 0),
    End: () => moveTo(row, sheet.columns.length - 1),
    PageUp: () => moveTo(row - PAGE_ROWS, column),
    PageDown: () => moveTo(row + PAGE_ROWS, column),
    Enter: () => startEditing(rawValue()),
    F2: () => startEditing(rawValue()),
    ' ': () => (activeColumn?.type === 'check' && canEdit ? tick() : undefined),
    Delete: () => {
      if (canEdit && activeRow && activeColumn) change(setCell(sheet, activeRow.id, activeColumn.id, ''));
    },
    Backspace: () => startEditing(''),
  };

  const onKey = (e: KeyboardEvent) => {
    if (isEditing) return;
    if (e.key === 'Tab') {
      onTab(e);
      return;
    }
    if ((e.ctrlKey || e.metaKey) && shortcut(e)) {
      e.preventDefault();
      return;
    }
    const move = moves[e.key];
    if (move && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      move();
    } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey && canEdit) {
      e.preventDefault();
      startEditing(e.key);
    }
  };

  const onPaste = (e: ClipboardEvent) => {
    if (!canEdit || isEditing || activeRowIndex === undefined) return;
    e.preventDefault();
    const block = parsePasted(e.clipboardData.getData('text/plain'));
    const pasted = pasteBlock(sheet, block, { row: activeRowIndex, column });
    change(pasted.sheet);
    setNote(pasted.refused > 0 ? t('sheet.pasteRefused', { count: pasted.refused }) : '');
  };

  const onCopy = (e: ClipboardEvent) => {
    if (isEditing || activeRowIndex === undefined) return;
    e.preventDefault();
    e.clipboardData.setData('text/plain', read(activeRowIndex, column));
  };

  const changeSort = (columnId: string) => {
    if (sort?.columnId !== columnId) setSort({ columnId, descending: false });
    else if (!sort.descending) setSort({ columnId, descending: true });
    else setSort(null);
  };

  const ariaSort = (columnId: string) => {
    if (sort?.columnId !== columnId) return 'none';
    return sort.descending ? 'descending' : 'ascending';
  };

  return (
    <div className="sheet">
      <div className="sheet-tools">
        <input
          aria-label={t('sheet.filter')}
          placeholder={t('sheet.filter')}
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
            setActive({ row: 0, column });
          }}
        />
        {tools}
      </div>
      <p id={`${id}${HELP_ID_SUFFIX}`} className="visually-hidden">
        {t(canEdit ? 'sheet.helpEdit' : 'sheet.helpRead')}
      </p>
      <div className="sheet-scroll">
        <table
          ref={table}
          role="grid"
          aria-label={label}
          aria-describedby={`${id}${HELP_ID_SUFFIX}`}
          aria-readonly={!canEdit}
          aria-rowcount={order.length + 1}
          onKeyDown={onKey}
          onPaste={onPaste}
          onCopy={onCopy}
          onFocus={() => {
            hasFocus.current = true;
          }}
          onBlur={(e) => {
            hasFocus.current = table.current?.contains(e.relatedTarget) ?? false;
          }}
        >
          <thead>
            <tr>
              <th scope="col" className="sheet-corner">
                <span className="visually-hidden">{t('sheet.rowNumber')}</span>
              </th>
              {sheet.columns.map((c, i) => (
                <th key={c.id} scope="col" aria-sort={ariaSort(c.id)} data-type={c.type}>
                  <button type="button" className="sheet-sort" tabIndex={-1} onClick={() => changeSort(c.id)}>
                    <span className="muted">{columnLetter(i)}</span> {c.name}
                    {sort?.columnId === c.id && (
                      <span className={sort.descending ? undefined : 'sheet-up'}>
                        <IconChevronDown size={14} />
                      </span>
                    )}
                  </button>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {shown.map((rowIndex, r) => {
              const stored = sheet.rows[rowIndex];
              if (!stored) return null;
              return (
                <tr key={stored.id} aria-rowindex={r + 2}>
                  <th scope="row" className="sheet-number">
                    {rowIndex + 1}
                  </th>
                  {sheet.columns.map((c, i) => {
                    const isActive = r === row && i === column;
                    const value = read(rowIndex, i);
                    const isBad = value === ERROR_VALUE || value === CYCLE_VALUE;
                    return (
                      <td
                        key={c.id}
                        role="gridcell"
                        tabIndex={isActive && !isEditing ? 0 : -1}
                        data-active={isActive}
                        data-type={c.type}
                        data-bad={isBad || undefined}
                        onClick={() => {
                          if (!isActive && !commit()) return;
                          setActive({ row: r, column: i });
                        }}
                        onDoubleClick={() => startEditing(stored.cells[c.id] ?? '')}
                      >
                        {isActive && draft !== null ? (
                          <CellInput
                            column={c}
                            value={draft}
                            label={`${c.name} ${rowIndex + 1}`}
                            invalid={!fitsColumn(c, draft)}
                            onInput={setDraft}
                            onKey={onEditKey}
                            onBlur={() => void commit()}
                          />
                        ) : (
                          <CellValue column={c} value={value} />
                        )}
                      </td>
                    );
                  })}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {order.length === 0 && <p className="muted">{t('sheet.noRows')}</p>}
      {order.length > shownRows && (
        <button type="button" className="quiet" onClick={() => setShownRows(shownRows + ROWS_PER_STEP)}>
          {t('sheet.more', { count: order.length - shownRows })}
        </button>
      )}
      <p role="status" className="muted">
        {note}
      </p>
    </div>
  );
}
