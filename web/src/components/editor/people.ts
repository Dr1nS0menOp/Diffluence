// SPDX-License-Identifier: Apache-2.0
// People in a document: a mention, and a task with its date.
import { Extension, mergeAttributes, Node as DocumentNode } from '@tiptap/core';
import type { Node as ProseNode } from '@tiptap/pm/model';
import { Plugin, PluginKey } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import i18n from '../../i18n';
import { today } from './inline';

const SOON_DAYS = 7;
const MS_PER_DAY = 86_400_000;

const shownName = (label: unknown) => `@${(label as string) || i18n.t('mentions.unknown')}`;

/** Stored with an id alone. The server fills in the name of today when a document is read. */
export const Mention = DocumentNode.create({
  name: 'mention',
  group: 'inline',
  inline: true,
  atom: true,
  addAttributes() {
    return {
      id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-mention'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-mention': attrs.id }),
      },
      kind: {
        default: 'user',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-kind'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-kind': attrs.kind }),
      },
      label: {
        default: '',
        parseHTML: (el: HTMLElement) => el.textContent?.replace(/^@/, '') ?? '',
        renderHTML: () => ({}),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-mention]' }];
  },
  renderHTML({ HTMLAttributes, node }) {
    return ['span', mergeAttributes(HTMLAttributes, { class: 'mention' }), shownName(node.attrs.label)];
  },
  renderText({ node }) {
    return shownName(node.attrs.label);
  },
});

export type DueState = 'overdue' | 'soon' | 'later';

/** How near a task's date is. A date has no time zone: this compares calendar days. */
export function dueState(value: string, now = new Date()): DueState {
  const days = (Date.parse(`${value}T00:00:00Z`) - Date.parse(`${today(now)}T00:00:00Z`)) / MS_PER_DAY;
  if (days < 0) return 'overdue';
  return days <= SOON_DAYS ? 'soon' : 'later';
}

function dueMarks(doc: ProseNode, overdueWord: string): DecorationSet {
  const marks: Decoration[] = [];
  doc.descendants((node, pos) => {
    if (node.type.name !== 'taskItem') return true;
    if (node.attrs.checked) return true;
    const firstLine = node.firstChild;
    let found = false;
    firstLine?.descendants((child, offset) => {
      if (found || child.type.name !== 'date') return !found;
      found = true;
      const at = pos + 2 + offset;
      const state = dueState(child.attrs.value as string);
      if (state === 'later') return false;
      marks.push(Decoration.node(at, at + child.nodeSize, { class: `date-mark due-${state}` }));
      if (state === 'overdue') {
        marks.push(
          Decoration.widget(at + child.nodeSize, () => {
            const note = document.createElement('span');
            note.className = 'due-note';
            note.textContent = overdueWord;
            return note;
          }),
        );
      }
      return false;
    });
    return true;
  });
  return DecorationSet.create(doc, marks);
}

/** Colour alone may not carry meaning, so an overdue date also says so (WCAG 1.4.1). */
export const TaskDates = Extension.create<{ overdueWord: string }>({
  name: 'taskDates',
  addOptions() {
    return { overdueWord: '' };
  },
  addProseMirrorPlugins() {
    const { overdueWord } = this.options;
    return [
      new Plugin({
        key: new PluginKey('taskDates'),
        props: { decorations: (state) => dueMarks(state.doc, overdueWord) },
      }),
    ];
  },
});
