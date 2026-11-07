// SPDX-License-Identifier: Apache-2.0
// The handle beside a block: a grip to drag it, and a menu. TipTap's own handle needs
// the whole collaboration stack, so this small one is ours (spec 008, A4).
import { useEffect, useLayoutEffect, useRef, useState, type DragEvent, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import { NodeSelection } from '@tiptap/pm/state';
import {
  IconCopy,
  IconGrip,
  IconLink,
  IconMore,
  IconMoveDown,
  IconMoveUp,
  IconPlus,
  IconTrash,
  IconUnwrap,
} from '../icons';
import { Menu, MenuItem, MenuSeparator } from '../menu';
import {
  anchorOfBlock,
  blockAtCursor,
  blockRange,
  canLinkToBlock,
  deleteBlock,
  duplicateBlock,
  moveBlock,
  selectBlock,
  selectBlocks,
  unwrapBlock,
} from './block-commands';
import { turnIntoItems } from './insert';
import type { EditorActions } from './keys';

const MENU_ICON_SIZE = 18;
const HANDLE_ICON_SIZE = 16;
const ITEM_GRIP_SIZE = 12;
const TURNABLE = new Set([
  'paragraph',
  'heading',
  'bulletList',
  'orderedList',
  'taskList',
  'blockquote',
  'codeBlock',
]);
const FRAMES = new Set(['panel', 'expand', 'spotlight']);
const LIST_ITEMS = new Set(['listItem', 'taskItem']);

/** The top-level block at this height. The middle of the text avoids the margins. */
function blockAtHeight(editor: Editor, y: number): number | null {
  const { view } = editor;
  const box = view.dom.getBoundingClientRect();
  const found = view.posAtCoords({ left: box.left + box.width / 2, top: y });
  if (!found) return null;
  const inside = found.inside >= 0 ? found.inside : found.pos;
  const $pos = view.state.doc.resolve(inside);
  const pos = $pos.depth > 0 ? $pos.before(1) : inside;
  return view.state.doc.nodeAt(pos) ? pos : null;
}

/** The innermost list item at this height: dragging it takes everything nested below it along. */
function listItemAtHeight(editor: Editor, y: number): number | null {
  const { view } = editor;
  const box = view.dom.getBoundingClientRect();
  const found = view.posAtCoords({ left: box.left + box.width / 2, top: y });
  if (!found) return null;
  const $pos = view.state.doc.resolve(found.inside >= 0 ? found.inside + 1 : found.pos);
  for (let depth = $pos.depth; depth > 0; depth -= 1) {
    if (LIST_ITEMS.has($pos.node(depth).type.name)) return $pos.before(depth);
  }
  return null;
}

/** A remembered position can be stale: the text may have become shorter since. */
function isBlockStart(editor: Editor, pos: number): boolean {
  const { doc } = editor.state;
  return pos <= doc.content.size && doc.resolve(pos).depth === 0 && doc.nodeAt(pos) !== null;
}

/** How far a block starts from the start of the sheet, whichever way the text runs. */
function startOffset(box: DOMRect, sheet: HTMLElement): number {
  const outer = sheet.getBoundingClientRect();
  return getComputedStyle(sheet).direction === 'rtl' ? outer.right - box.right : box.left - outer.left;
}

/** Which block the handle belongs to. It stays put while its menu is open. */
export function useBlockTracking(editor: Editor) {
  const [pos, setPos] = useState<number | null>(null);
  const [itemPos, setItemPos] = useState<number | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const held = useRef(false);
  const isInUse = () => held.current || (root.current?.contains(document.activeElement) ?? false);

  // An edit moves blocks, so the handle goes until the pointer moves again.
  useEffect(() => {
    const forget = () => {
      setPos(null);
      setItemPos(null);
    };
    editor.on('update', forget);
    return () => {
      editor.off('update', forget);
    };
  }, [editor]);

  return {
    pos,
    itemPos,
    root,
    hold: () => {
      held.current = true;
    },
    release: () => {
      held.current = false;
    },
    track: (e: MouseEvent<HTMLElement>) => {
      if (isInUse()) return;
      setPos(blockAtHeight(editor, e.clientY));
      setItemPos(listItemAtHeight(editor, e.clientY));
    },
    leave: () => {
      if (isInUse()) return;
      setPos(null);
      setItemPos(null);
    },
    openAtCursor: () => {
      setPos(blockAtCursor(editor));
      requestAnimationFrame(() => root.current?.querySelector<HTMLElement>('[aria-haspopup]')?.click());
    },
  };
}

/** Starts dragging one node. The block rides along in `dragging`: changing the selection now
 * would change the page, and Chrome ends a drag when the page changes while it starts. */
function startDragOf(editor: Editor, pos: number, e: DragEvent<HTMLSpanElement>) {
  const { view, state } = editor;
  const selection = NodeSelection.create(state.doc, pos);
  const slice = selection.content();
  const { dom, text } = view.serializeForClipboard(slice);
  const block = view.nodeDOM(pos);
  if (block instanceof HTMLElement) e.dataTransfer.setDragImage(block, 0, 0);
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/html', dom.innerHTML);
  e.dataTransfer.setData('text/plain', text);
  const dragging = { slice, move: true, node: selection };
  view.dragging = dragging;
}

/** The small grip beside a list item. */
function ItemGrip({ editor, tracker }: { editor: Editor; tracker: ReturnType<typeof useBlockTracking> }) {
  const { t } = useTranslation();
  const grip = useRef<HTMLSpanElement>(null);
  const { itemPos } = tracker;
  useLayoutEffect(() => {
    const element = grip.current;
    const sheet = element?.parentElement;
    if (!element || !sheet || itemPos === null) return;
    const item = editor.view.nodeDOM(itemPos);
    if (!(item instanceof HTMLElement)) return;
    const outer = sheet.getBoundingClientRect();
    const box = item.getBoundingClientRect();
    element.style.insetBlockStart = `${box.top - outer.top}px`;
    element.style.insetInlineStart = `${startOffset(box, sheet)}px`;
  }, [editor, itemPos]);
  if (itemPos === null || itemPos > editor.state.doc.content.size) return null;
  const item = editor.state.doc.nodeAt(itemPos);
  if (!item || !LIST_ITEMS.has(item.type.name)) return null;
  return (
    <span
      ref={grip}
      className="item-grip"
      title={t('editor.dragItem')}
      draggable
      onMouseDown={tracker.hold}
      onMouseUp={tracker.release}
      onDragStart={(e) => startDragOf(editor, itemPos, e)}
      onDragEnd={() => {
        tracker.release();
        editor.view.dragging = null;
      }}
    >
      <IconGrip size={ITEM_GRIP_SIZE} />
    </span>
  );
}

export function BlockHandle({
  editor,
  tracker,
  actions,
}: {
  editor: Editor;
  tracker: ReturnType<typeof useBlockTracking>;
  actions: EditorActions;
}) {
  const { t } = useTranslation();
  const { pos, root } = tracker;

  useLayoutEffect(() => {
    const handle = root.current;
    if (!handle || pos === null) return;
    const block = editor.view.nodeDOM(pos);
    const sheet = handle.parentElement;
    if (!(block instanceof HTMLElement) || !sheet) return;
    const box = block.getBoundingClientRect();
    handle.style.insetBlockStart = `${box.top - sheet.getBoundingClientRect().top}px`;
    // A stretched table reaches past the text, so beside the text the handle would sit on its cells.
    const reachesPastText = box.width > editor.view.dom.getBoundingClientRect().width + 1;
    handle.classList.toggle('on-corner', reachesPastText);
    handle.style.insetInlineStart = reachesPastText ? `${startOffset(box, sheet)}px` : '';
  }, [editor, pos, root]);

  if (pos === null || !isBlockStart(editor, pos)) return null;
  const node = editor.state.doc.nodeAt(pos);
  if (!node) return null;
  const index = editor.state.doc.resolve(pos).index(0);
  const act = (run: () => void) => {
    run();
    editor.commands.focus();
  };
  const range = blockRange(editor, pos);
  const lastIndex = range ? editor.state.doc.resolve(range.to).index(0) - 1 : index;
  const firstIndex = range ? editor.state.doc.resolve(range.from).index(0) : index;
  const isEmptyLine = node.type.name === 'paragraph' && node.content.size === 0;
  const extendSelection = (e: MouseEvent<HTMLSpanElement>) => {
    const start = blockAtCursor(editor);
    if (e.shiftKey && start !== null) selectBlocks(editor, start, pos);
  };
  const copyLink = () => {
    const anchor = anchorOfBlock(editor, pos);
    if (anchor) actions.copyBlockLink(anchor);
  };
  const insertHere = () => {
    selectBlock(editor, pos);
    editor.chain().focus().insertContent({ type: 'text', text: '/' }).run();
  };
  return (
    <>
      <ItemGrip editor={editor} tracker={tracker} />
      <div className="block-handle" ref={root}>
        {isEmptyLine && (
          <button
            type="button"
            className="block-plus"
            aria-label={t('editor.insertHere')}
            title={t('editor.insertHere')}
            onClick={insertHere}
          >
            <IconPlus size={HANDLE_ICON_SIZE} />
          </button>
        )}
        <span
          className="block-grip"
          title={t('editor.dragBlock')}
          draggable
          onMouseDown={tracker.hold}
          onMouseUp={tracker.release}
          onClick={extendSelection}
          onDragStart={(e) => startDragOf(editor, pos, e)}
          onDragEnd={() => {
            tracker.release();
            editor.view.dragging = null;
          }}
        >
          <IconGrip size={HANDLE_ICON_SIZE} />
        </span>
        <Menu
          label={t('editor.blockMenu')}
          trigger={<IconMore size={HANDLE_ICON_SIZE} />}
          align="start"
          className="block-menu"
        >
          <MenuItem disabled={firstIndex === 0} onClick={() => act(() => moveBlock(editor, pos, 'up'))}>
            <IconMoveUp size={MENU_ICON_SIZE} /> {t('editor.moveUp')}
          </MenuItem>
          <MenuItem
            disabled={lastIndex === editor.state.doc.childCount - 1}
            onClick={() => act(() => moveBlock(editor, pos, 'down'))}
          >
            <IconMoveDown size={MENU_ICON_SIZE} /> {t('editor.moveDown')}
          </MenuItem>
          <MenuItem onClick={() => act(() => duplicateBlock(editor, pos))}>
            <IconCopy size={MENU_ICON_SIZE} /> {t('editor.duplicate')}
          </MenuItem>
          {canLinkToBlock(node, pos) && (
            <MenuItem onClick={() => act(copyLink)}>
              <IconLink size={MENU_ICON_SIZE} /> {t('editor.copyBlockLink')}
            </MenuItem>
          )}
          <MenuItem
            onClick={() => {
              // Several selected blocks stay selected, and are saved together.
              if (range?.to === pos + node.nodeSize && range.from === pos) selectBlock(editor, pos);
              actions.openSaveBlock();
            }}
          >
            <IconCopy size={MENU_ICON_SIZE} /> {t('editor.saveBlock')}
          </MenuItem>
          <MenuItem className="danger" onClick={() => act(() => deleteBlock(editor, pos))}>
            <IconTrash size={MENU_ICON_SIZE} /> {t('common.delete')}
          </MenuItem>
          {FRAMES.has(node.type.name) && (
            <>
              <MenuSeparator />
              <MenuItem
                onClick={() =>
                  act(() => {
                    selectBlock(editor, pos);
                    unwrapBlock(editor, node.type.name as 'panel' | 'expand' | 'spotlight');
                  })
                }
              >
                <IconUnwrap size={MENU_ICON_SIZE} /> {t('editor.backToText')}
              </MenuItem>
            </>
          )}
          {node.type.name === 'decision' && (
            <>
              <MenuSeparator />
              <MenuItem
                onClick={() => {
                  selectBlock(editor, pos);
                  editor.chain().focus().setParagraph().run();
                }}
              >
                <IconUnwrap size={MENU_ICON_SIZE} /> {t('editor.backToText')}
              </MenuItem>
            </>
          )}
          {TURNABLE.has(node.type.name) && (
            <>
              <MenuSeparator />
              {turnIntoItems(node.type.name).map((item) => (
                <MenuItem
                  key={item.id}
                  onClick={() => {
                    // A selection of several blocks stays, so all of them turn at once.
                    if (range?.from === pos && range.to === pos + node.nodeSize) selectBlock(editor, pos);
                    item.run(editor, actions);
                  }}
                >
                  <item.icon size={MENU_ICON_SIZE} /> {t(`insert.${item.id}.name`)}
                </MenuItem>
              ))}
            </>
          )}
        </Menu>
      </div>
    </>
  );
}
