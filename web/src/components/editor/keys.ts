// SPDX-License-Identifier: Apache-2.0
// Keyboard shortcuts of the editor that are not tied to one block (spec 008, A18).
import { Extension, InputRule } from '@tiptap/core';
import type { EditorState } from '@tiptap/pm/state';
import {
  blockAtCursor,
  deleteBlock,
  duplicateBlock,
  moveBlock,
  setListKind,
  type ListKind,
} from './block-commands';
import type { DrawingFile } from './drawing';
import { ruleUndoPlugin, undoLastRule } from './rule-undo';

const LINK_SHORTHAND = /\[([^\]]+)\]\(((?:https?:\/\/|mailto:|\/)[^\s)]+)\)$/;

/** What the editor asks of the screen around it. */
export interface EditorActions {
  pickFiles: () => void;
  pickWord: () => void;
  openLink: () => void;
  openFind: () => void;
  openDrawing: (existing?: DrawingFile) => void;
  openPageFiles: () => void;
  openBlockMenu: () => void;
  openTableMenu: () => void;
  openTemplateItem: (what: 'field' | 'instruction') => void;
  publish: () => void;
  save: () => void;
  mentionNoAccess: (name: string) => void;
  /** Says whether there is a title to go to. */
  focusTitle: () => boolean;
}

export const noActions: EditorActions = {
  pickFiles: () => undefined,
  pickWord: () => undefined,
  openLink: () => undefined,
  openFind: () => undefined,
  openDrawing: () => undefined,
  openPageFiles: () => undefined,
  openBlockMenu: () => undefined,
  openTableMenu: () => undefined,
  openTemplateItem: () => undefined,
  publish: () => undefined,
  save: () => undefined,
  mentionNoAccess: () => undefined,
  focusTitle: () => false,
};

/** The cursor stands before everything, in a first block that is not inside another one. */
function atStartOfBody({ selection, doc }: EditorState): boolean {
  const { empty, $from } = selection;
  return empty && $from.depth === 1 && $from.parentOffset === 0 && $from.start(1) === 1 && doc.childCount > 0;
}

/** Moves to the row of tools. It has one Tab stop, the tool that had focus last. */
export function focusTools() {
  document.querySelector<HTMLElement>('[role=toolbar] [tabindex="0"]')?.focus();
}

export const EditorKeys = Extension.create<{ actions: () => EditorActions }>({
  name: 'editorKeys',
  // Before the keys of the library, which uses Ctrl Enter for a line break.
  priority: 1000,
  addOptions() {
    return { actions: () => noActions };
  },
  addKeyboardShortcuts() {
    const editor = () => this.editor;
    const run =
      (
        action: Exclude<
          keyof EditorActions,
          'mentionNoAccess' | 'openTemplateItem' | 'focusTitle' | 'openDrawing'
        >,
      ) =>
      () => {
        this.options.actions()[action]();
        return true;
      };
    const toList = (kind: ListKind) => () => setListKind(editor(), kind);
    const onBlock = (act: (pos: number) => void) => () => {
      const pos = blockAtCursor(editor());
      if (pos === null) return false;
      act(pos);
      return true;
    };
    return {
      Backspace: () =>
        undoLastRule(editor().view) || (atStartOfBody(editor().state) && this.options.actions().focusTitle()),
      // The keys of the library would lose the nesting. It still starts a new list.
      'Mod-Shift-7': toList('orderedList'),
      'Mod-Shift-8': toList('bulletList'),
      'Mod-Shift-9': toList('taskList'),
      'Mod-k': run('openLink'),
      'Mod-Enter': run('publish'),
      'Mod-s': run('save'),
      'Mod-Shift-.': () => run(editor().isActive('table') ? 'openTableMenu' : 'openBlockMenu')(),
      'Mod-\\': () => editor().chain().focus().unsetAllMarks().run(),
      'Mod-d': onBlock((pos) => duplicateBlock(editor(), pos)),
      'Mod-Shift-Backspace': onBlock((pos) => deleteBlock(editor(), pos)),
      'Alt-ArrowUp': onBlock((pos) => moveBlock(editor(), pos, 'up')),
      'Alt-ArrowDown': onBlock((pos) => moveBlock(editor(), pos, 'down')),
      'Alt-F10': () => {
        focusTools();
        return true;
      },
      Escape: () => {
        focusTools();
        return true;
      },
    };
  },
  addProseMirrorPlugins() {
    return [ruleUndoPlugin()];
  },
  addInputRules() {
    return [
      new InputRule({
        find: LINK_SHORTHAND,
        handler: ({ state, range, match }) => {
          const [, text, href] = match;
          const link = state.schema.marks.link;
          if (!text || !href || !link) return null;
          state.tr.replaceWith(range.from, range.to, state.schema.text(text, [link.create({ href })]));
          return undefined;
        },
      }),
    ];
  },
});
