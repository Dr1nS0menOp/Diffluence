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
import { DocumentCell, DocumentHeaderCell } from './table';

/** Everything a document can hold, for reading and for writing. */
export function documentExtensions(): AnyExtension[] {
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
    TaskItem.configure({ nested: true }),
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
    TextColor,
    Highlight,
    Subscript,
    Superscript,
    BlockFormat,
  ];
}
