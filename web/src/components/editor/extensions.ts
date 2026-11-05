// SPDX-License-Identifier: Apache-2.0
import type { AnyExtension } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import { TableKit } from '@tiptap/extension-table';
import { AttachmentFile, AttachmentImage } from './attachments';
import { Column, Columns, Decision, Expand, Panel } from './blocks';
import { CodeBlock } from './code';
import { DateMark, Status } from './inline';
import { BlockFormat, Highlight, Subscript, Superscript, TextColor } from './marks';
import { Mention } from './people';
import { DocumentCell, DocumentHeaderCell } from './table';

/** A task has an id, so a tick in a list can find it in the document. */
const DocumentTaskItem = TaskItem.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-task'),
        renderHTML: (attrs: Record<string, string | null>) => (attrs.id ? { 'data-task': attrs.id } : {}),
      },
    };
  },
  addNodeView() {
    const parent = this.parent?.();
    if (!parent) return null;
    const canTick = this.options.onReadOnlyChecked !== undefined;
    return (props) => {
      const view = parent(props);
      // A checkbox that jumps back says nothing, so one that cannot change says so.
      if (!props.editor.isEditable && !canTick && view.dom instanceof HTMLElement) {
        const box = view.dom.querySelector('input');
        if (box) box.disabled = true;
      }
      return view;
    };
  },
});

/** Called with a task's id when a reader ticks it. */
export type TaskToggle = (id: string, done: boolean) => void;

/** Everything a document can hold, for reading and for writing. */
export function documentExtensions(onTask?: TaskToggle): AnyExtension[] {
  return [
    StarterKit.configure({
      codeBlock: false,
      link: {
        openOnClick: false,
        autolink: true,
        protocols: ['http', 'https', 'mailto'],
        HTMLAttributes: { rel: 'noopener noreferrer nofollow', target: '_blank' },
      },
    }),
    TaskList,
    DocumentTaskItem.configure({
      nested: true,
      onReadOnlyChecked: onTask
        ? (node, checked) => {
            const id = node.attrs.id as string | null;
            if (id) onTask(id, checked);
            return id !== null;
          }
        : undefined,
    }),
    TableKit.configure({ table: { resizable: true }, tableCell: false, tableHeader: false }),
    DocumentCell,
    DocumentHeaderCell,
    CodeBlock,
    AttachmentImage,
    AttachmentFile,
    Panel,
    Expand,
    Columns,
    Column,
    Decision,
    Status,
    DateMark,
    Mention,
    TextColor,
    Highlight,
    Subscript,
    Superscript,
    BlockFormat,
  ];
}
