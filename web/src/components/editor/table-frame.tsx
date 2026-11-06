// SPDX-License-Identifier: Apache-2.0
// What a table has around it while writing: an outline, a button with everything for
// tables, and an edge to stretch it. It stays out of the row of tools (spec 017, D2).
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type MouseEvent,
  type PointerEvent,
  type ReactNode,
  type RefObject,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useEditorState, type Editor } from '@tiptap/react';
import {
  IconColumnAdd,
  IconColumnDelete,
  IconCopy,
  IconEqual,
  IconFullWidth,
  IconHeaderColumn,
  IconHeaderRow,
  IconMerge,
  IconRowAdd,
  IconRowDelete,
  IconSortDown,
  IconSortUp,
  IconTableMenu,
  IconTrash,
} from '../icons';
import { FIXED_BARS, Menu, MenuItem } from '../menu';
import { duplicateBlock } from './block-commands';
import { MARK_COLORS } from './palette';
import {
  canSortTable,
  enterTable,
  equalColumns,
  FULL_STRETCH,
  hasHeaderColumn,
  hasHeaderRow,
  isTopLevel,
  paintLine,
  setStretch,
  sortTable,
  STRETCH_STEP,
  stretchAt,
  stretchedWidth,
  tableAround,
  tableAt,
  WIDE_STRETCH,
} from './table';

const MENU_ICON_SIZE = 18;

interface Box {
  pos: number;
  top: number;
  left: number;
  width: number;
  height: number;
}

function tableBox(editor: Editor, pos: number, sheet: HTMLElement): Box | null {
  if (!tableAt(editor, pos)) return null;
  const wrapper = editor.view.nodeDOM(pos);
  if (!(wrapper instanceof HTMLElement)) return null;
  const outer = sheet.getBoundingClientRect();
  const box = wrapper.getBoundingClientRect();
  return {
    pos,
    top: box.top - outer.top,
    left: box.left - outer.left,
    width: box.width,
    height: box.height,
  };
}

/** How far down the bars reach once the page has scrolled under them. Bars can wrap or stop sticking. */
function barsReach(): number {
  const reaches = Array.from(document.querySelectorAll<HTMLElement>(FIXED_BARS), (bar) => {
    const style = getComputedStyle(bar);
    return style.position === 'sticky' ? parseFloat(style.insetBlockStart) + bar.offsetHeight : 0;
  });
  return Math.max(0, ...reaches);
}

/** The table around this element, as a position in the document. */
function tableOf(editor: Editor, element: Element): number | null {
  const wrapper = element.closest('.tableWrapper');
  if (!wrapper) return null;
  const inside = editor.view.posAtDOM(wrapper, 0);
  const $pos = editor.state.doc.resolve(inside);
  for (let depth = $pos.depth; depth >= 0; depth--) {
    if ($pos.node(depth).type.name === 'table') return $pos.before(depth);
  }
  const after = $pos.nodeAfter;
  return after?.type.name === 'table' ? inside : null;
}

/** Which table has the frame: the one with the cursor, or else the one under the pointer. */
export function useTableTracking(editor: Editor, sheet: RefObject<HTMLDivElement | null>) {
  const root = useRef<HTMLDivElement>(null);
  const [hovered, setHovered] = useState<number | null>(null);
  const [box, setBox] = useState<Box | null>(null);
  const inTable = useEditorState({ editor, selector: (s) => tableAround(s.editor)?.pos ?? null });
  const pos = inTable ?? hovered;

  const measure = useCallback(() => {
    sheet.current?.style.setProperty('--size-bars', `${barsReach()}px`);
    setBox(pos !== null && sheet.current ? tableBox(editor, pos, sheet.current) : null);
  }, [editor, pos, sheet]);

  useLayoutEffect(measure, [measure]);
  useEffect(() => {
    editor.on('transaction', measure);
    window.addEventListener('resize', measure);
    // Putting the sidebar away changes the room, and so the bars, without resizing the window.
    const bars = new ResizeObserver(measure);
    document.querySelectorAll(FIXED_BARS).forEach((bar) => bars.observe(bar));
    return () => {
      editor.off('transaction', measure);
      window.removeEventListener('resize', measure);
      bars.disconnect();
    };
  }, [editor, measure]);

  const isInUse = () => root.current?.contains(document.activeElement) ?? false;
  return {
    root,
    box,
    active: inTable !== null,
    measure,
    track: (e: MouseEvent<HTMLElement>) => {
      const target = e.target as Element;
      if (root.current?.contains(target) || isInUse()) return;
      setHovered(tableOf(editor, target));
    },
    leave: () => {
      if (!isInUse()) setHovered(null);
    },
    open: () => {
      root.current?.querySelector<HTMLElement>('[aria-haspopup]')?.click();
    },
  };
}

type Tracker = ReturnType<typeof useTableTracking>;

function Swatches({
  label,
  current,
  onPick,
}: {
  label: string;
  current?: string | null;
  onPick: (color: string | null) => void;
}) {
  const { t } = useTranslation();
  // A whole row or column has no one colour, so those are plain actions.
  const checks = current !== undefined;
  const role = checks ? 'menuitemradio' : 'menuitem';
  return (
    <div role="group" aria-label={label} className="menu-swatches">
      <span className="menu-swatches-label" aria-hidden="true">
        {label}
      </span>
      <MenuItem
        role={role}
        aria-checked={checks ? !current : undefined}
        aria-label={`${label}: ${t('editor.noColor')}`}
        title={t('editor.noColor')}
        onClick={() => onPick(null)}
      >
        <span className="swatch" />
      </MenuItem>
      {MARK_COLORS.map((color) => (
        <MenuItem
          key={color}
          role={role}
          aria-checked={checks ? current === color : undefined}
          aria-label={`${label}: ${t(`editor.colors.${color}`)}`}
          title={t(`editor.colors.${color}`)}
          onClick={() => onPick(color)}
        >
          <span className={`swatch mark-${color}`} />
        </MenuItem>
      ))}
    </div>
  );
}

function Group({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} className="menu-group">
      <div className="menu-group-title" aria-hidden="true">
        {label}
      </div>
      {children}
    </div>
  );
}

/** How wide a table can get on this screen, for the sums of a stretch. */
function roomFor(editor: Editor): { column: number; room: number } {
  const text = editor.view.dom;
  const frame = text.closest<HTMLElement>('.focus');
  const padding = parseFloat(getComputedStyle(text.closest('.editor-sheet') ?? text).paddingInlineStart);
  return { column: text.clientWidth, room: (frame?.clientWidth ?? text.clientWidth) - 2 * padding };
}

function commitStretch(editor: Editor, pos: number, next: number) {
  const before = (tableAt(editor, pos)?.attrs.stretch as number | null) ?? 0;
  const { column, room } = roomFor(editor);
  const factor = stretchedWidth(column, room, next) / stretchedWidth(column, room, before);
  setStretch(editor, pos, next, factor);
}

function TableMenu({ editor, pos }: { editor: Editor; pos: number }) {
  const { t } = useTranslation();
  const state = useEditorState({
    editor,
    selector: (s) => {
      const table = tableAt(s.editor, pos);
      return {
        canSort: canSortTable(s.editor),
        canStretch: isTopLevel(s.editor, pos),
        headerRow: table ? hasHeaderRow(table) : false,
        headerColumn: table ? hasHeaderColumn(table) : false,
        stretch: (table?.attrs.stretch as number | null) ?? 0,
        background:
          (s.editor.getAttributes('tableHeader').background as string | null | undefined) ??
          (s.editor.getAttributes('tableCell').background as string | null | undefined) ??
          null,
      };
    },
  });
  // These leave the focus in the menu, which stays open for the next step.
  const chain = () => editor.chain();
  const widths = [
    { value: 0, label: t('table.widthText') },
    { value: WIDE_STRETCH, label: t('table.widthWide') },
    { value: FULL_STRETCH, label: t('table.widthFull') },
  ];
  return (
    <Menu
      label={t('table.menu')}
      trigger={<IconTableMenu size={MENU_ICON_SIZE} />}
      className="table-button"
      keepOpen
      onLeave={() => editor.commands.focus()}
    >
      <Group label={t('table.rows')}>
        <MenuItem onClick={() => chain().addRowBefore().run()}>
          <IconRowAdd size={MENU_ICON_SIZE} /> {t('table.rowAbove')}
        </MenuItem>
        <MenuItem onClick={() => chain().addRowAfter().run()}>
          <IconRowAdd size={MENU_ICON_SIZE} /> {t('table.rowBelow')}
        </MenuItem>
        <MenuItem onClick={() => chain().deleteRow().run()}>
          <IconRowDelete size={MENU_ICON_SIZE} /> {t('table.rowDelete')}
        </MenuItem>
        <MenuItem
          role="menuitemcheckbox"
          aria-checked={state.headerRow}
          onClick={() => chain().toggleHeaderRow().run()}
        >
          <IconHeaderRow size={MENU_ICON_SIZE} /> {t('table.headerRow')}
        </MenuItem>
      </Group>
      <Group label={t('table.columns')}>
        <MenuItem onClick={() => chain().addColumnBefore().run()}>
          <IconColumnAdd size={MENU_ICON_SIZE} /> {t('table.columnBefore')}
        </MenuItem>
        <MenuItem onClick={() => chain().addColumnAfter().run()}>
          <IconColumnAdd size={MENU_ICON_SIZE} /> {t('table.columnAfter')}
        </MenuItem>
        <MenuItem onClick={() => chain().deleteColumn().run()}>
          <IconColumnDelete size={MENU_ICON_SIZE} /> {t('table.columnDelete')}
        </MenuItem>
        <MenuItem
          role="menuitemcheckbox"
          aria-checked={state.headerColumn}
          onClick={() => chain().toggleHeaderColumn().run()}
        >
          <IconHeaderColumn size={MENU_ICON_SIZE} /> {t('table.headerColumn')}
        </MenuItem>
        {state.canSort && (
          <>
            <MenuItem onClick={() => sortTable(editor, 'up')}>
              <IconSortUp size={MENU_ICON_SIZE} /> {t('editor.sortUp')}
            </MenuItem>
            <MenuItem onClick={() => sortTable(editor, 'down')}>
              <IconSortDown size={MENU_ICON_SIZE} /> {t('editor.sortDown')}
            </MenuItem>
          </>
        )}
      </Group>
      <Group label={t('table.cells')}>
        <MenuItem onClick={() => chain().mergeOrSplit().run()}>
          <IconMerge size={MENU_ICON_SIZE} /> {t('table.merge')}
        </MenuItem>
      </Group>
      <Group label={t('table.colour')}>
        <Swatches
          label={t('table.colourCell')}
          current={state.background}
          onPick={(color) => chain().setCellAttribute('background', color).run()}
        />
        <Swatches label={t('table.colourRow')} onPick={(color) => paintLine(editor, 'row', color)} />
        <Swatches label={t('table.colourColumn')} onPick={(color) => paintLine(editor, 'column', color)} />
      </Group>
      <Group label={t('table.table')}>
        {state.canStretch &&
          widths.map((width) => (
            <MenuItem
              key={width.value}
              role="menuitemradio"
              aria-checked={state.stretch === width.value}
              onClick={() => commitStretch(editor, pos, width.value)}
            >
              <IconFullWidth size={MENU_ICON_SIZE} /> {width.label}
            </MenuItem>
          ))}
        <MenuItem onClick={() => equalColumns(editor)}>
          <IconEqual size={MENU_ICON_SIZE} /> {t('editor.tableEqual')}
        </MenuItem>
        <MenuItem onClick={() => duplicateBlock(editor, pos)}>
          <IconCopy size={MENU_ICON_SIZE} /> {t('table.duplicate')}
        </MenuItem>
        <MenuItem
          className="danger"
          onClick={() => {
            chain().deleteTable().run();
            editor.commands.focus();
          }}
        >
          <IconTrash size={MENU_ICON_SIZE} /> {t('editor.tableDelete')}
        </MenuItem>
      </Group>
    </Menu>
  );
}

/** The edge of the table. Dragging has the menu and the arrow keys beside it (WCAG 2.5.7). */
function StretchEdge({ editor, pos, onMove }: { editor: Editor; pos: number; onMove: () => void }) {
  const { t } = useTranslation();
  const stored = useEditorState({
    editor,
    selector: (s) => (tableAt(s.editor, pos)?.attrs.stretch as number | null) ?? 0,
  });
  const [dragged, setDragged] = useState<number | null>(null);
  const shown = dragged ?? stored;

  const wrapper = () => editor.view.nodeDOM(pos) as HTMLElement | null;
  const preview = (next: number) => {
    setDragged(next);
    wrapper()?.style.setProperty('--table-stretch', String(next));
    onMove();
  };
  const stretchFor = (e: PointerEvent<HTMLDivElement>) => {
    const text = editor.view.dom.getBoundingClientRect();
    const { column, room } = roomFor(editor);
    return stretchAt(e.clientX - (text.left + text.width / 2), column, room);
  };
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    const rtl = getComputedStyle(e.currentTarget).direction === 'rtl';
    const steps: Record<string, number> = {
      ArrowRight: stored + (rtl ? -STRETCH_STEP : STRETCH_STEP),
      ArrowLeft: stored + (rtl ? STRETCH_STEP : -STRETCH_STEP),
      Home: 0,
      End: FULL_STRETCH,
    };
    const next = steps[e.key];
    if (next === undefined) return;
    e.preventDefault();
    commitStretch(editor, pos, Math.min(FULL_STRETCH, Math.max(0, next)));
  };
  return (
    <div
      role="separator"
      tabIndex={0}
      className="table-edge"
      aria-orientation="vertical"
      aria-label={t('table.width')}
      aria-valuemin={0}
      aria-valuemax={FULL_STRETCH}
      aria-valuenow={shown}
      onKeyDown={keys}
      onPointerDown={(e) => {
        e.preventDefault();
        e.currentTarget.setPointerCapture(e.pointerId);
        setDragged(stored);
      }}
      onPointerMove={(e) => {
        if (dragged !== null) preview(stretchFor(e));
      }}
      onPointerUp={(e) => {
        if (dragged === null) return;
        const next = stretchFor(e);
        setDragged(null);
        commitStretch(editor, pos, next);
      }}
    />
  );
}

export function TableFrame({ editor, tracker }: { editor: Editor; tracker: Tracker }) {
  const { box, root, active, measure } = tracker;
  const canStretch = useEditorState({
    editor,
    selector: (s) => (box ? isTopLevel(s.editor, box.pos) : false),
  });

  useLayoutEffect(() => {
    const frame = root.current;
    if (!frame || !box) return;
    frame.style.top = `${box.top}px`;
    frame.style.left = `${box.left}px`;
    frame.style.width = `${box.width}px`;
    frame.style.height = `${box.height}px`;
  }, [box, root]);

  if (!box) return null;
  return (
    <div
      ref={root}
      className={active ? 'table-frame active' : 'table-frame'}
      onClickCapture={() => enterTable(editor, box.pos)}
    >
      <TableMenu editor={editor} pos={box.pos} />
      {canStretch && <StretchEdge editor={editor} pos={box.pos} onMove={measure} />}
    </div>
  );
}
