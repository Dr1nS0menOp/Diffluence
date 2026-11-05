// SPDX-License-Identifier: Apache-2.0
// Images and files in a document. Both are attachments of the page, never outside addresses.
import { Node as DocumentNode } from '@tiptap/core';
import Image from '@tiptap/extension-image';
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from '@tiptap/react';
import type { Attachment } from '../../api';
import { IconDownload, IconPage } from '../icons';

const DOWNLOAD_ICON_SIZE = 18;

interface ImageAttrs {
  attachment_id: string;
  alt: string | null;
  caption: string | null;
  size: string | null;
  align: string | null;
}

function ImageView({ node }: ReactNodeViewProps) {
  const { attachment_id: id, alt, caption, size, align } = node.attrs as ImageAttrs;
  return (
    <NodeViewWrapper
      as="figure"
      className="doc-image"
      data-size={size ?? undefined}
      data-align={align ?? undefined}
    >
      <img src={`/api/v1/attachments/${id}/content`} alt={alt ?? ''} />
      {caption && <figcaption>{caption}</figcaption>}
    </NodeViewWrapper>
  );
}

const dataAttribute = (name: string) => ({
  default: null,
  parseHTML: (el: HTMLElement) => el.getAttribute(`data-${name}`),
  renderHTML: (attrs: Record<string, string | null>) =>
    attrs[name] ? { [`data-${name}`]: attrs[name] } : {},
});

/**
 * Images are attachments, never external addresses (spec 001). Pasted images
 * from elsewhere are dropped here, and the server refuses them as well.
 */
export const AttachmentImage = Image.extend({
  draggable: true,
  addAttributes() {
    return {
      attachment_id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-attachment-id'),
        renderHTML: (attrs: Record<string, string>) => ({
          'data-attachment-id': attrs.attachment_id,
          src: `/api/v1/attachments/${attrs.attachment_id}/content`,
        }),
      },
      alt: { default: null },
      title: { default: null },
      caption: dataAttribute('caption'),
      size: dataAttribute('size'),
      align: dataAttribute('align'),
    };
  },
  parseHTML() {
    return [{ tag: 'img[data-attachment-id]' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ImageView);
  },
});

/** A file in the document: a tile to download. While editing it is a block to move or remove. */
function FileTile({ node, editor }: ReactNodeViewProps) {
  const { attachment_id: id, filename } = node.attrs as { attachment_id: string; filename: string };
  const label = (
    <>
      <IconPage />
      <span className="file-name">{filename}</span>
    </>
  );
  return (
    <NodeViewWrapper className="file-block">
      {editor.isEditable ? (
        <span className="file-tile">{label}</span>
      ) : (
        <a className="file-tile" href={`/api/v1/attachments/${id}/content?download=1`} download={filename}>
          {label}
          <IconDownload size={DOWNLOAD_ICON_SIZE} />
        </a>
      )}
    </NodeViewWrapper>
  );
}

export const AttachmentFile = DocumentNode.create({
  name: 'file',
  group: 'block',
  atom: true,
  draggable: true,
  addAttributes() {
    return {
      attachment_id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-attachment-id'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-attachment-id': attrs.attachment_id }),
      },
      filename: {
        default: '',
        parseHTML: (el: HTMLElement) => el.getAttribute('data-filename'),
        renderHTML: (attrs: Record<string, string>) => ({ 'data-filename': attrs.filename }),
      },
    };
  },
  parseHTML() {
    return [{ tag: 'div[data-attachment-file]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['div', { ...HTMLAttributes, 'data-attachment-file': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(FileTile);
  },
});

/** What the server made of a file decides how it shows: only safe images show as images. */
export function attachmentNode(attachment: Attachment) {
  return attachment.inline
    ? { type: 'image', attrs: { attachment_id: attachment.id, alt: attachment.filename } }
    : { type: 'file', attrs: { attachment_id: attachment.id, filename: attachment.filename } };
}
