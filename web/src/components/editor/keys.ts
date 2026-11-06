// SPDX-License-Identifier: Apache-2.0
// Keyboard shortcuts of the editor that are not tied to one block (spec 008, A18).
import { Extension, InputRule } from '@tiptap/core';
import { blockAtCursor, deleteBlock, duplicateBlock, moveBlock } from './block-commands';

const LINK_SHORTHAND = /\[([^\]]+)\]\(((?:https?:\/\/|mailto:|\/)[^\s)]+)\)$/;

/** What the editor asks of the screen around it. */
export interface EditorActions {
  pickFiles: () => void;
  openLink: () => void;
  openFind: () => void;
  openBlockMenu: () => void;
  openTableMenu: () => void;
  openTemplateItem: (what: 'field' | 'instruction') => void;
  publish: () => void;
  save: () => void;
  mentionNoAccess: (name: string) => void;
}

export const noActions: EditorActions = {
  pickFiles: () => undefined,
  openLink: () => undefined,
  openFind: () => undefined,
  openBlockMenu: () => undefined,
  openTableMenu: () => undefined,
  openTemplateItem: () => undefined,
  publish: () => undefined,
  save: () => undefined,
  mentionNoAccess: () => undefined,
};

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
    const run = (action: Exclude<keyof EditorActions, 'mentionNoAccess' | 'openTemplateItem'>) => () => {
      this.options.actions()[action]();
      return true;
    };
    const onBlock = (act: (pos: number) => void) => () => {
      const pos = blockAtCursor(editor());
      if (pos === null) return false;
      act(pos);
      return true;
    };
    return {
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
