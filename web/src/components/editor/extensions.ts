// SPDX-License-Identifier: Apache-2.0
import type { AnyExtension } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import { TaskItem, TaskList } from '@tiptap/extension-list';
import { TableKit } from '@tiptap/extension-table';
import { AttachmentFile, AttachmentImage, pageFileAddress, type FileAddress } from './attachments';
import { Column, Columns, Decision, Expand, Panel } from './blocks';
import { CodeBlock } from './code';
import { Dynamic } from './dynamic/dynamic';
import { Anchor, ChartNode, Excerpt, Media, PageFacts } from './dynamic/nodes';
import { DateMark, Instruction, Status, TemplateField } from './inline';
import { BlockFormat, Highlight, Subscript, Superscript, TextColor } from './marks';
import { Mention } from './people';
import { DocumentCell, DocumentHeaderCell, DocumentTable } from './table';

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
export interface DocumentOptions {
  fileAddress?: FileAddress;
  onTask?: TaskToggle;
  /** Editing together: undo comes from the shared document, per person. */
  shared?: boolean;
}

export function documentExtensions({
  fileAddress = pageFileAddress,
  onTask,
  shared = false,
}: DocumentOptions = {}): AnyExtension[] {
  return [
    StarterKit.configure({
      codeBlock: false,
      ...(shared ? { undoRedo: false as const } : {}),
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
    TableKit.configure({ table: false, tableCell: false, tableHeader: false }),
    DocumentTable,
    DocumentCell,
    DocumentHeaderCell,
    CodeBlock,
    AttachmentImage.configure({ fileAddress }),
    AttachmentFile.configure({ fileAddress }),
    Panel,
    Expand,
    Columns,
    Column,
    Decision,
    Dynamic,
    PageFacts,
    Excerpt,
    Anchor,
    ChartNode,
    Media,
    Status,
    DateMark,
    Mention,
    TemplateField,
    Instruction,
    TextColor,
    Highlight,
    Subscript,
    Superscript,
    BlockFormat,
  ];
}
