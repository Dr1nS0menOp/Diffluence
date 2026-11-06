// SPDX-License-Identifier: Apache-2.0
// Small items that sit in a line of text: status marks and dates.
import { mergeAttributes, Node as DocumentNode, nodeInputRule } from '@tiptap/core';

/** Today in the reader's own calendar, as stored: a date has no time zone. */
export function today(now = new Date()): string {
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${now.getFullYear()}-${month}-${day}`;
}

/** Reads a stored date in local time on purpose, so it never shifts by a day. */
export function formatStoredDate(value: string): string {
  return new Date(`${value}T00:00:00`).toLocaleDateString(undefined, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  });
}

export const Status = DocumentNode.create({
  name: 'status',
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return {
      text: {
        default: '',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-status'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-status': attrs.text }),
      },
      color: {
        default: 'grey',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-color'),
        renderHTML: (attrs: Record<string, string>) => ({
          'data-color': attrs.color,
          class: `status-mark mark-${attrs.color}`,
        }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-status]' }];
  },
  renderHTML({ HTMLAttributes, node }) {
    return ['span', mergeAttributes(HTMLAttributes), node.attrs.text as string];
  },
  renderText({ node }) {
    return node.attrs.text as string;
  },
});

/** A question in a template. The form that starts a page fills it in. */
export const TemplateField = DocumentNode.create({
  name: 'templateField',
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return {
      name: {
        default: '',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-field'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-field': attrs.name }),
      },
      kind: {
        default: 'line',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-kind'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-kind': attrs.kind }),
      },
      options: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-options'),
        renderHTML: (attrs: Record<string, string | null>) =>
          attrs.options ? { 'data-options': attrs.options } : {},
      },
      required: {
        default: null,
        parseHTML: (el: HTMLElement) => el.hasAttribute('data-required') || null,
        renderHTML: (attrs: Record<string, boolean | null>) =>
          attrs.required ? { 'data-required': '' } : {},
      },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-field]' }];
  },
  renderHTML({ HTMLAttributes, node }) {
    return ['span', mergeAttributes(HTMLAttributes, { class: 'template-field' }), node.attrs.name as string];
  },
  renderText({ node }) {
    return node.attrs.name as string;
  },
});

/** Help for the writer. Typing replaces it, and it never reaches a reader. */
export const Instruction = DocumentNode.create({
  name: 'instruction',
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return {
      text: {
        default: '',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-instruction'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-instruction': attrs.text }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-instruction]' }];
  },
  renderHTML({ HTMLAttributes, node }) {
    return ['span', mergeAttributes(HTMLAttributes, { class: 'instruction' }), node.attrs.text as string];
  },
  renderText() {
    return '';
  },
});

export const DateMark = DocumentNode.create({
  name: 'date',
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return {
      value: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('datetime'),
        renderHTML: (attrs: Record<string, string>) => ({ datetime: attrs.value }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'time[datetime]' }];
  },
  renderHTML({ HTMLAttributes, node }) {
    return [
      'time',
      mergeAttributes(HTMLAttributes, { class: 'date-mark' }),
      formatStoredDate(node.attrs.value as string),
    ];
  },
  renderText({ node }) {
    return node.attrs.value as string;
  },
  addInputRules() {
    return [
      nodeInputRule({
        find: /(?:^|\s)(\/\/)$/,
        type: this.type,
        getAttributes: () => ({ value: today() }),
      }),
    ];
  },
});
