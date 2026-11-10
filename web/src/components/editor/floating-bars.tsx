// SPDX-License-Identifier: Apache-2.0
// Bars over selected text and over a selected image, beside the fixed row, which keeps
// every tool (ADR-038, spec 047, part A). TipTap's BubbleMenu shows and places them; what
// it does not do, the keys, the names and the rules of placement, is ours.
import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useEditorState, type Editor } from '@tiptap/react';
import { BubbleMenu } from '@tiptap/react/menus';
import { CellSelection } from '@tiptap/pm/tables';
import { NodeSelection } from '@tiptap/pm/state';
import {
  IconBold,
  IconBullet,
  IconCode,
  IconComment,
  IconHeading,
  IconItalic,
  IconLink,
  IconNumbered,
  IconTrash,
} from '../icons';
import { toggleList } from './block-commands';
import type { EditorActions } from './keys';
import { setTextStyle } from './marks';
import { firstControlOf, useRovingFocus } from './roving-focus';
import { ImageTools } from './selection-tools';
import { Tool, TOOL_ICON_SIZE } from './tool';
import { HighlightMenu, TextColorMenu } from './toolbar';

/** What decides whether a bar shows, read from the editor (spec 047, D4). */
export interface SelectionFacts {
  empty: boolean;
  /** The type of a selected node, or null for a text selection. */
  node: string | null;
  inCode: boolean;
  cells: boolean;
  composing: boolean;
  /** A dialog or an open menu covers the text. */
  covered: boolean;
}

export type BarKind = 'text' | 'image';

/** Which bar shows for a selection, if any. */
export function barFor(facts: SelectionFacts): BarKind | null {
  if (facts.composing || facts.covered || facts.cells) return null;
  if (facts.node !== null) return facts.node === 'image' ? 'image' : null;
  if (facts.empty || facts.inCode) return null;
  return 'text';
}

function factsOf(editor: Editor): SelectionFacts {
  const { selection } = editor.state;
  return {
    empty: selection.empty,
    node: selection instanceof NodeSelection ? selection.node.type.name : null,
    inCode: selection.$from.parent.type.spec.code === true || editor.isActive('code'),
    cells: selection instanceof CellSelection,
    composing: editor.view.composing,
    // A menu opened from a bar belongs to it; anything else open covers the text.
    covered: Array.from(document.querySelectorAll('[role=dialog], [role=menu]')).some(
      (open) => !open.closest('.floating-tools'),
    ),
  };
}

/** Space kept between the bar and the selection, in pixels. */
const GAP = 8;
/** Space kept between the bar and the sides of the window. */
const EDGE = 8;

/**
 * The options BubbleMenu hands to Floating UI (spec 047, D5): above the selection, below on
 * a touch screen, flipped when there is no room, and never over the sticky row, whose
 * bottom is keepOut pixels from the top of the window.
 */
export function barPlacement(coarse: boolean, keepOut: number) {
  const padding = { top: keepOut + EDGE, right: EDGE, bottom: EDGE, left: EDGE };
  return {
    // Absolute, from the real offset parent: an ancestor with a transform would move a fixed bar.
    strategy: 'absolute' as const,
    placement: coarse ? ('bottom' as const) : ('top' as const),
    offset: GAP,
    flip: { padding },
    shift: { padding },
    hide: true,
  };
}

type Placement = ReturnType<typeof barPlacement>;

/**
 * Moves the edge the bar keeps clear of to the bottom of the sticky row, as it is now. The
 * row moves while the page scrolls, and BubbleMenu reads the padding each time it places
 * the bar, so the one padding object both middlewares share is changed in place.
 */
export function keepClearOfRow(options: Placement, keepOut: number) {
  options.flip.padding.top = keepOut + EDGE;
}

function rowBottom(): number {
  const row = document.querySelector('.editor-area .toolbar');
  return row ? Math.max(0, row.getBoundingClientRect().bottom) : 0;
}
const isCoarse = () => window.matchMedia('(pointer: coarse)').matches;

function HeadingTool({ editor, level }: { editor: Editor; level: 1 | 2 | 3 }) {
  const { t } = useTranslation();
  const on = useEditorState({ editor, selector: (s) => s.editor.isActive('heading', { level }) });
  return (
    <Tool
      editor={editor}
      label={t(`editor.h${level}`)}
      pressed={on}
      run={() => setTextStyle(editor, on ? null : level)}
    >
      <IconHeading size={TOOL_ICON_SIZE} />
      <span className="tool-level">{level}</span>
    </Tool>
  );
}

function TextBar({
  editor,
  actions,
  onComment,
}: {
  editor: Editor;
  actions: EditorActions;
  onComment?: () => void;
}) {
  const { t } = useTranslation();
  const chain = () => editor.chain().focus();
  return (
    <>
      <Tool editor={editor} label={t('editor.bold')} active="bold" run={() => chain().toggleBold().run()}>
        <IconBold size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.italic')}
        active="italic"
        run={() => chain().toggleItalic().run()}
      >
        <IconItalic size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.link')} active="link" run={actions.openLink}>
        <IconLink size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool editor={editor} label={t('editor.code')} active="code" run={() => chain().toggleCode().run()}>
        <IconCode size={TOOL_ICON_SIZE} />
      </Tool>
      <TextColorMenu editor={editor} />
      <HighlightMenu editor={editor} />
      <HeadingTool editor={editor} level={1} />
      <HeadingTool editor={editor} level={2} />
      <HeadingTool editor={editor} level={3} />
      <Tool
        editor={editor}
        label={t('editor.bullet')}
        active="bulletList"
        run={() => toggleList(editor, 'bulletList')}
      >
        <IconBullet size={TOOL_ICON_SIZE} />
      </Tool>
      <Tool
        editor={editor}
        label={t('editor.ordered')}
        active="orderedList"
        run={() => toggleList(editor, 'orderedList')}
      >
        <IconNumbered size={TOOL_ICON_SIZE} />
      </Tool>
      {onComment && (
        <Tool editor={editor} label={t('comments.comment')} run={onComment}>
          <IconComment size={TOOL_ICON_SIZE} />
        </Tool>
      )}
    </>
  );
}

function ImageBar({ editor, actions }: { editor: Editor; actions: EditorActions }) {
  const { t } = useTranslation();
  return (
    <>
      <ImageTools editor={editor} actions={actions} />
      {/* Takes the image out of the page; the file stays on the page's list (EDT-080). */}
      <Tool
        editor={editor}
        label={t('editor.deleteImage')}
        run={() => editor.chain().focus().deleteSelection().run()}
      >
        <IconTrash size={TOOL_ICON_SIZE} />
      </Tool>
    </>
  );
}

function Bar({
  editor,
  kind,
  label,
  children,
}: {
  editor: Editor;
  kind: BarKind;
  label: string;
  children: ReactNode;
}) {
  const row = useRef<HTMLDivElement>(null);
  const roving = useRovingFocus(row, () => editor.commands.focus());
  // One object for the life of the bar: a new one would make BubbleMenu set itself up again.
  const [options] = useState(() => barPlacement(isCoarse(), rowBottom()));
  return (
    <BubbleMenu
      editor={editor}
      pluginKey={`floating-${kind}`}
      className="floating-bar"
      options={options}
      shouldShow={({ editor: shown }) => {
        if (!shown.isEditable || barFor(factsOf(shown)) !== kind) return false;
        keepClearOfRow(options, rowBottom());
        return true;
      }}
    >
      <div
        ref={row}
        role="toolbar"
        aria-label={label}
        className="floating-tools"
        data-bar={kind}
        onFocus={roving.onFocus}
        onKeyDown={roving.onKeyDown}
      >
        {children}
      </div>
    </BubbleMenu>
  );
}

/** Alt+F10 moves into the bar that shows, or into the fixed row when none does (D6). */
function useAltF10(editor: Editor) {
  useEffect(() => {
    // On the document and in the capture phase, so no handler in the editor can keep it.
    const onKey = (e: KeyboardEvent) => {
      if (!(e.altKey && e.key === 'F10') || !editor.view.hasFocus()) return;
      e.preventDefault();
      const dom = editor.view.dom;
      // BubbleMenu takes a hidden bar out of the page, so a bar found here is the one that shows.
      const shown = dom.parentElement?.querySelector<HTMLElement>('.floating-tools');
      const target = shown ?? dom.closest('.editor-area')?.querySelector<HTMLElement>('.toolbar');
      firstControlOf(target ?? null)?.focus();
    };
    document.addEventListener('keydown', onKey, true);
    return () => document.removeEventListener('keydown', onKey, true);
  }, [editor]);
}

export function FloatingBars({
  editor,
  actions,
  onComment,
}: {
  editor: Editor;
  actions: EditorActions;
  onComment?: () => void;
}) {
  const { t } = useTranslation();
  useAltF10(editor);
  return (
    <>
      <Bar editor={editor} kind="text" label={t('editor.textBar')}>
        <TextBar editor={editor} actions={actions} onComment={onComment} />
      </Bar>
      <Bar editor={editor} kind="image" label={t('editor.imageBar')}>
        <ImageBar editor={editor} actions={actions} />
      </Bar>
    </>
  );
}
