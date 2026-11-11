// SPDX-License-Identifier: Apache-2.0
// A handle and a plus for the row and the column in use (EDT-087, 088, 091). Only those two
// lines have them, so a long table does not get a button per row (spec 023, D10).
import { useLayoutEffect, useRef, type PointerEvent, type ReactNode, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { useEditorState, type Editor } from '@tiptap/react';
import {
  IconColumnAdd,
  IconColumnDelete,
  IconGrip,
  IconMoveDown,
  IconMoveUp,
  IconPlus,
  IconRowAdd,
  IconRowDelete,
} from '../icons';
import { Menu, MenuItem } from '../menu';
import { useToast } from '../toast';
import {
  linesInUse,
  lineCell,
  moveLines,
  selectLine,
  selectedLines,
  tableAt,
  type LineKind,
  type TableCell,
} from './table';

const LINE_ICON_SIZE = 16;
const DRAG_START_PX = 5;

function cellElement(editor: Editor, pos: number, line: TableCell): HTMLElement | null {
  const start = lineCell(editor, pos, line);
  if (start === null) return null;
  const element = editor.view.nodeDOM(start);
  return element instanceof HTMLElement ? element : null;
}

/** The boxes of every line of a kind, for finding the line under the pointer. */
function lineBoxes(frame: HTMLElement | null, cell: HTMLElement, kind: LineKind): DOMRect[] {
  const table = cell.closest('table');
  if (!frame || !table) return [];
  if (kind === 'row') return Array.from(table.rows, (row) => row.getBoundingClientRect());
  return Array.from(table.rows[0]?.cells ?? [], (one) => one.getBoundingClientRect());
}

function lineUnderPointer(boxes: DOMRect[], kind: LineKind, x: number, y: number): number {
  const at = kind === 'row' ? y : x;
  const found = boxes.findIndex((box) =>
    kind === 'row' ? at >= box.top && at <= box.bottom : at >= box.left && at <= box.right,
  );
  if (found >= 0) return found;
  const first = boxes[0];
  if (!first) return 0;
  return at < (kind === 'row' ? first.top : Math.min(...boxes.map((box) => box.left))) ? 0 : boxes.length - 1;
}

function place(element: HTMLElement | null, left: number, top: number) {
  if (!element) return;
  element.style.left = `${left}px`;
  element.style.top = `${top}px`;
}

function LineMenu({
  editor,
  kind,
  pos,
  onMoved,
}: {
  editor: Editor;
  kind: LineKind;
  pos: number;
  onMoved: () => void;
}) {
  const { t } = useTranslation();
  const toast = useToast();
  const chain = () => editor.chain();
  const move = (step: number) => {
    const lines = selectedLines(editor, pos, kind);
    if (!lines) return;
    const to = step < 0 ? lines.first - 1 : lines.last + 1;
    if (moveLines(editor, pos, kind, lines, to)) onMoved();
    else toast({ message: t('table.cannotMove') });
  };
  if (kind === 'row') {
    return (
      <>
        <MenuItem onClick={() => chain().addRowBefore().run()}>
          <IconRowAdd size={LINE_ICON_SIZE} /> {t('table.rowAbove')}
        </MenuItem>
        <MenuItem onClick={() => chain().addRowAfter().run()}>
          <IconRowAdd size={LINE_ICON_SIZE} /> {t('table.rowBelow')}
        </MenuItem>
        <MenuItem onClick={() => move(-1)}>
          <IconMoveUp size={LINE_ICON_SIZE} /> {t('table.rowUp')}
        </MenuItem>
        <MenuItem onClick={() => move(1)}>
          <IconMoveDown size={LINE_ICON_SIZE} /> {t('table.rowDown')}
        </MenuItem>
        <MenuItem className="danger" onClick={() => chain().deleteRow().run()}>
          <IconRowDelete size={LINE_ICON_SIZE} /> {t('table.rowDelete')}
        </MenuItem>
      </>
    );
  }
  return (
    <>
      <MenuItem onClick={() => chain().addColumnBefore().run()}>
        <IconColumnAdd size={LINE_ICON_SIZE} /> {t('table.columnBefore')}
      </MenuItem>
      <MenuItem onClick={() => chain().addColumnAfter().run()}>
        <IconColumnAdd size={LINE_ICON_SIZE} /> {t('table.columnAfter')}
      </MenuItem>
      <MenuItem onClick={() => move(-1)}>
        <IconMoveUp size={LINE_ICON_SIZE} /> {t('table.columnEarlier')}
      </MenuItem>
      <MenuItem onClick={() => move(1)}>
        <IconMoveDown size={LINE_ICON_SIZE} /> {t('table.columnLater')}
      </MenuItem>
      <MenuItem className="danger" onClick={() => chain().deleteColumn().run()}>
        <IconColumnDelete size={LINE_ICON_SIZE} /> {t('table.columnDelete')}
      </MenuItem>
    </>
  );
}

interface LineProps {
  editor: Editor;
  pos: number;
  kind: LineKind;
  line: TableCell;
  frame: RefObject<HTMLDivElement | null>;
  /** Changes whenever the table moved or changed size. */
  layout: unknown;
  /** The handles go along with the line, which is now where the selection is. */
  onMoved: () => void;
}

/** Dragging starts past a small distance, or a click on the handle could never open its menu. */
function useLineDrag(
  { editor, pos, kind, line, frame, onMoved }: LineProps,
  drop: RefObject<HTMLDivElement | null>,
) {
  const { t } = useTranslation();
  const toast = useToast();
  const start = useRef<{ x: number; y: number } | null>(null);
  const dragged = useRef(false);
  const index = kind === 'row' ? line.row : line.column;

  const boxes = () => {
    const cell = cellElement(editor, pos, line);
    return cell ? lineBoxes(frame.current, cell, kind) : [];
  };
  const showDrop = (target: DOMRect | undefined) => {
    const outer = frame.current?.getBoundingClientRect();
    if (!target || !outer || !drop.current) return;
    drop.current.hidden = false;
    place(
      drop.current,
      kind === 'row' ? 0 : target.left - outer.left,
      kind === 'row' ? target.top - outer.top : 0,
    );
  };
  return {
    onPointerDown: (e: PointerEvent<HTMLDivElement>) => {
      dragged.current = false;
      start.current = { x: e.clientX, y: e.clientY };
    },
    onPointerMove: (e: PointerEvent<HTMLDivElement>) => {
      if (!start.current) return;
      const distance = Math.hypot(e.clientX - start.current.x, e.clientY - start.current.y);
      if (!dragged.current && distance < DRAG_START_PX) return;
      if (!dragged.current) e.currentTarget.setPointerCapture(e.pointerId);
      dragged.current = true;
      const all = boxes();
      showDrop(all[lineUnderPointer(all, kind, e.clientX, e.clientY)]);
    },
    onPointerUp: (e: PointerEvent<HTMLDivElement>) => {
      start.current = null;
      if (drop.current) drop.current.hidden = true;
      if (!dragged.current) return;
      const to = lineUnderPointer(boxes(), kind, e.clientX, e.clientY);
      const lines = selectedLines(editor, pos, kind) ?? { first: index, last: index };
      const chosen = index >= lines.first && index <= lines.last ? lines : { first: index, last: index };
      if (moveLines(editor, pos, kind, chosen, to)) onMoved();
      else toast({ message: t('table.cannotMove') });
    },
    wasDragged: () => dragged.current,
  };
}

function LineHandle(props: LineProps) {
  const { editor, pos, kind, line, frame, layout, onMoved } = props;
  const { t } = useTranslation();
  const handle = useRef<HTMLDivElement>(null);
  const plus = useRef<HTMLButtonElement>(null);
  const drop = useRef<HTMLDivElement>(null);
  const { wasDragged, ...dragging } = useLineDrag(props, drop);

  useLayoutEffect(() => {
    const cell = cellElement(editor, pos, line);
    const outer = frame.current?.getBoundingClientRect();
    if (!cell || !outer) return;
    const box = cell.closest(kind === 'row' ? 'tr' : 'td,th')?.getBoundingClientRect();
    if (!box) return;
    const inView = kind === 'row' || (box.right > outer.left && box.left < outer.right);
    if (handle.current) handle.current.hidden = !inView;
    if (plus.current) plus.current.hidden = !inView;
    if (kind === 'row') {
      place(handle.current, 0, box.top - outer.top + box.height / 2);
      place(plus.current, outer.width, box.bottom - outer.top);
    } else {
      place(handle.current, box.left - outer.left + box.width / 2, 0);
      place(plus.current, box.right - outer.left, outer.height);
    }
  }, [editor, pos, kind, line, frame, layout]);

  const label = kind === 'row' ? t('table.rowMenu') : t('table.columnMenu');
  const addLabel = kind === 'row' ? t('table.rowBelow') : t('table.columnAfter');
  return (
    <>
      <div
        ref={handle}
        className={`table-line ${kind}-line`}
        {...dragging}
        onClickCapture={(e) => {
          if (wasDragged()) {
            e.stopPropagation();
            return;
          }
          selectLine(editor, pos, kind, line, e.shiftKey);
          // Shift only selects, so several lines can be chosen before the menu opens.
          if (e.shiftKey) e.stopPropagation();
        }}
      >
        <Menu
          label={label}
          trigger={<IconGrip size={LINE_ICON_SIZE} />}
          className="table-handle"
          keepOpen
          align="start"
        >
          <LineMenu editor={editor} kind={kind} pos={pos} onMoved={onMoved} />
        </Menu>
      </div>
      <button
        ref={plus}
        type="button"
        className={`table-plus ${kind}-line`}
        aria-label={addLabel}
        title={addLabel}
        onClick={() => {
          selectLine(editor, pos, kind, line, false);
          if (kind === 'row') editor.chain().focus().addRowAfter().run();
          else editor.chain().focus().addColumnAfter().run();
        }}
      >
        <IconPlus size={LINE_ICON_SIZE} />
      </button>
      {/* Not "row" or "column": those are layout classes whose display would beat hidden. */}
      <div ref={drop} className={`table-drop ${kind}-line`} hidden />
    </>
  );
}

export function TableLines({
  editor,
  pos,
  hovered,
  frame,
  layout,
  onMoved,
}: {
  editor: Editor;
  pos: number;
  hovered: TableCell | null;
  frame: RefObject<HTMLDivElement | null>;
  layout: unknown;
  onMoved: () => void;
}): ReactNode {
  const fromCursor = useEditorState({ editor, selector: (s) => linesInUse(s.editor, pos) });
  const line = hovered ?? fromCursor;
  if (!line || !tableAt(editor, pos)) return null;
  const shared = { editor, pos, line, frame, layout, onMoved };
  return (
    <div className="table-lines">
      <LineHandle {...shared} kind="row" />
      <LineHandle {...shared} kind="column" />
    </div>
  );
}
