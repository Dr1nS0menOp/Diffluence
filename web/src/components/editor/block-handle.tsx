// SPDX-License-Identifier: Apache-2.0
// The handle beside a block: a grip to drag it, and a menu. TipTap's own handle needs
// the whole collaboration stack, so this small one is ours (spec 008, A4).
import { useEffect, useLayoutEffect, useRef, useState, type DragEvent, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';
import { NodeSelection } from '@tiptap/pm/state';
import { IconCopy, IconGrip, IconMore, IconMoveDown, IconMoveUp, IconTrash } from '../icons';
import { Menu, MenuItem, MenuSeparator } from '../menu';
import { blockAtCursor, deleteBlock, duplicateBlock, moveBlock, selectBlock } from './block-commands';
import { TURN_INTO_ITEMS } from './insert';
import type { EditorActions } from './keys';

const MENU_ICON_SIZE = 18;
const HANDLE_ICON_SIZE = 16;
const TURNABLE = new Set([
  'paragraph',
  'heading',
  'bulletList',
  'orderedList',
  'taskList',
  'blockquote',
  'codeBlock',
]);

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

/** A remembered position can be stale: the text may have become shorter since. */
function isBlockStart(editor: Editor, pos: number): boolean {
  const { doc } = editor.state;
  return pos <= doc.content.size && doc.resolve(pos).depth === 0 && doc.nodeAt(pos) !== null;
}

/** Which block the handle belongs to. It stays put while its menu is open. */
export function useBlockTracking(editor: Editor) {
  const [pos, setPos] = useState<number | null>(null);
  const root = useRef<HTMLDivElement>(null);
  const held = useRef(false);
  const isInUse = () => held.current || (root.current?.contains(document.activeElement) ?? false);

  // An edit moves blocks, so the handle goes until the pointer moves again.
  useEffect(() => {
    const forget = () => setPos(null);
    editor.on('update', forget);
    return () => {
      editor.off('update', forget);
    };
  }, [editor]);

  return {
    pos,
    root,
    hold: () => {
      held.current = true;
    },
    release: () => {
      held.current = false;
    },
    track: (e: MouseEvent<HTMLElement>) => {
      if (!isInUse()) setPos(blockAtHeight(editor, e.clientY));
    },
    leave: () => {
      if (!isInUse()) setPos(null);
    },
    openAtCursor: () => {
      setPos(blockAtCursor(editor));
      requestAnimationFrame(() => root.current?.querySelector<HTMLElement>('[aria-haspopup]')?.click());
    },
  };
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
    handle.style.insetBlockStart = `${block.getBoundingClientRect().top - sheet.getBoundingClientRect().top}px`;
  }, [editor, pos, root]);

  if (pos === null || !isBlockStart(editor, pos)) return null;
  const node = editor.state.doc.nodeAt(pos);
  if (!node) return null;
  const index = editor.state.doc.resolve(pos).index(0);
  const act = (run: () => void) => {
    run();
    editor.commands.focus();
  };
  const startDrag = (e: DragEvent<HTMLSpanElement>) => {
    const { view, state } = editor;
    const selection = NodeSelection.create(state.doc, pos);
    const slice = selection.content();
    const { dom, text } = view.serializeForClipboard(slice);
    const block = view.nodeDOM(pos);
    if (block instanceof HTMLElement) e.dataTransfer.setDragImage(block, 0, 0);
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/html', dom.innerHTML);
    e.dataTransfer.setData('text/plain', text);
    // The block rides along here. Changing the selection now would change the page,
    // and Chrome ends a drag when the page changes while it starts.
    const dragging = { slice, move: true, node: selection };
    view.dragging = dragging;
  };
  return (
    <div className="block-handle" ref={root}>
      <span
        className="block-grip"
        title={t('editor.dragBlock')}
        draggable
        onMouseDown={tracker.hold}
        onMouseUp={tracker.release}
        onDragStart={startDrag}
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
        <MenuItem disabled={index === 0} onClick={() => act(() => moveBlock(editor, pos, 'up'))}>
          <IconMoveUp size={MENU_ICON_SIZE} /> {t('editor.moveUp')}
        </MenuItem>
        <MenuItem
          disabled={index === editor.state.doc.childCount - 1}
          onClick={() => act(() => moveBlock(editor, pos, 'down'))}
        >
          <IconMoveDown size={MENU_ICON_SIZE} /> {t('editor.moveDown')}
        </MenuItem>
        <MenuItem onClick={() => act(() => duplicateBlock(editor, pos))}>
          <IconCopy size={MENU_ICON_SIZE} /> {t('editor.duplicate')}
        </MenuItem>
        <MenuItem className="danger" onClick={() => act(() => deleteBlock(editor, pos))}>
          <IconTrash size={MENU_ICON_SIZE} /> {t('common.delete')}
        </MenuItem>
        {TURNABLE.has(node.type.name) && (
          <>
            <MenuSeparator />
            {TURN_INTO_ITEMS.map((item) => (
              <MenuItem
                key={item.id}
                onClick={() => {
                  selectBlock(editor, pos);
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
  );
}
