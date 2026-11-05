// SPDX-License-Identifier: Apache-2.0
// Marks and block attributes with names from short lists. The packages for these
// store free values, which our documents do not allow (spec 008, D2).
import { Extension, Mark, mergeAttributes, type Editor } from '@tiptap/core';

export const MAX_INDENT = 6;

export const TextColor = Mark.create({
  name: 'textColor',
  addAttributes() {
    return {
      color: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-text-color'),
        renderHTML: (attrs: Record<string, string>) => ({
          'data-text-color': attrs.color,
          class: `text-${attrs.color}`,
        }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-text-color]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['span', mergeAttributes(HTMLAttributes), 0];
  },
});

export const Highlight = Mark.create({
  name: 'highlight',
  addAttributes() {
    return {
      color: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-highlight'),
        renderHTML: (attrs: Record<string, string>) => ({
          'data-highlight': attrs.color,
          class: `mark-${attrs.color}`,
        }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'mark[data-highlight]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['mark', mergeAttributes(HTMLAttributes), 0];
  },
  addKeyboardShortcuts() {
    return { 'Mod-Shift-h': () => this.editor.commands.toggleMark(this.name, { color: 'yellow' }) };
  },
});

export const Subscript = Mark.create({
  name: 'subscript',
  excludes: 'superscript',
  parseHTML() {
    return [{ tag: 'sub' }];
  },
  renderHTML() {
    return ['sub', 0];
  },
  addKeyboardShortcuts() {
    return { 'Mod-,': () => this.editor.commands.toggleMark(this.name) };
  },
});

export const Superscript = Mark.create({
  name: 'superscript',
  excludes: 'subscript',
  parseHTML() {
    return [{ tag: 'sup' }];
  },
  renderHTML() {
    return ['sup', 0];
  },
  addKeyboardShortcuts() {
    return { 'Mod-.': () => this.editor.commands.toggleMark(this.name) };
  },
});

type Alignment = 'center' | 'end' | null;

/** Changes a text block between normal text and a heading, and keeps its alignment and indent. */
export function setTextStyle(editor: Editor, level: number | null) {
  const current = editor.isActive('heading') ? 'heading' : 'paragraph';
  const { align, indent } = editor.getAttributes(current) as { align?: string; indent?: number };
  const chain = editor.chain().focus();
  if (level) chain.setNode('heading', { level, align, indent }).run();
  else chain.setNode('paragraph', { align, indent }).run();
}

export function setAlignment(editor: Editor, align: Alignment) {
  editor
    .chain()
    .focus()
    .updateAttributes('paragraph', { align })
    .updateAttributes('heading', { align })
    .run();
}

/** Alignment and indent of text blocks, stored as data attributes for the style sheet. */
export const BlockFormat = Extension.create({
  name: 'blockFormat',
  addGlobalAttributes() {
    return [
      {
        types: ['paragraph', 'heading'],
        attributes: {
          align: {
            default: null,
            parseHTML: (el: HTMLElement) => el.getAttribute('data-align'),
            renderHTML: (attrs: Record<string, unknown>) =>
              attrs.align ? { 'data-align': attrs.align } : {},
          },
          indent: {
            default: null,
            parseHTML: (el: HTMLElement) => Number(el.getAttribute('data-indent')) || null,
            renderHTML: (attrs: Record<string, unknown>) =>
              attrs.indent ? { 'data-indent': attrs.indent } : {},
          },
        },
      },
    ];
  },
  addKeyboardShortcuts() {
    return {
      'Mod-Shift-l': () => {
        setAlignment(this.editor, null);
        return true;
      },
      'Mod-Shift-e': () => {
        setAlignment(this.editor, 'center');
        return true;
      },
      'Mod-Shift-r': () => {
        setAlignment(this.editor, 'end');
        return true;
      },
    };
  },
});

/** One step in or out: a list nests, and any other text gets an indent. */
export function indentBlock(editor: Editor, direction: 'in' | 'out') {
  const chain = editor.chain().focus();
  const item = ['listItem', 'taskItem'].find((name) => editor.isActive(name));
  if (item) {
    (direction === 'in' ? chain.sinkListItem(item) : chain.liftListItem(item)).run();
    return;
  }
  const block = editor.isActive('heading') ? 'heading' : 'paragraph';
  const current = (editor.getAttributes(block).indent as number | null) ?? 0;
  const next = Math.min(MAX_INDENT, Math.max(0, current + (direction === 'in' ? 1 : -1)));
  chain.updateAttributes(block, { indent: next || null }).run();
}
