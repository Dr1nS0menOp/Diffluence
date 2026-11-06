// SPDX-License-Identifier: Apache-2.0
// Images and files in a document. Both are attachments of the page, never outside addresses.
import { Node as DocumentNode } from '@tiptap/core';
import Image, { type ImageOptions } from '@tiptap/extension-image';
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from '@tiptap/react';
import { useTranslation } from 'react-i18next';
import { api, ApiError, type Attachment } from '../../api';
import { IconDownload, IconPage } from '../icons';
import { Dialog } from '../ui';

const DOWNLOAD_ICON_SIZE = 18;
const MAX_COPIES = 50;

/** Where a file of a document lives. A template serves its own files. */
export type FileAddress = (attachmentId: string) => string;

export const pageFileAddress: FileAddress = (id) => `/api/v1/attachments/${id}/content`;

interface ImageAttrs {
  attachment_id: string;
  alt: string | null;
  caption: string | null;
  size: string | null;
  align: string | null;
}

function ImageView({ node, editor, extension }: ReactNodeViewProps) {
  const { attachment_id: id, alt, caption, size, align } = node.attrs as ImageAttrs;
  const { fileAddress } = extension.options as { fileAddress: FileAddress };
  const image = <img src={fileAddress(id)} alt={alt ?? ''} />;
  return (
    <NodeViewWrapper
      as="figure"
      className="doc-image"
      data-size={size ?? undefined}
      data-align={align ?? undefined}
    >
      {/* A reader opens it large. The button gives the keyboard a way in. */}
      {editor.isEditable ? (
        image
      ) : (
        <button type="button" className="image-open">
          {image}
        </button>
      )}
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
export const AttachmentImage = Image.extend<ImageOptions & { fileAddress: FileAddress }>({
  draggable: true,
  addOptions() {
    return { ...(this.parent?.() as ImageOptions), fileAddress: pageFileAddress };
  },
  addAttributes() {
    const { fileAddress } = this.options;
    return {
      attachment_id: {
        default: null,
        parseHTML: (el: HTMLElement) => el.getAttribute('data-attachment-id'),
        renderHTML: (attrs: Record<string, string>) => ({
          'data-attachment-id': attrs.attachment_id,
          src: fileAddress(attrs.attachment_id ?? ''),
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
function FileTile({ node, editor, extension }: ReactNodeViewProps) {
  const { attachment_id: id, filename } = node.attrs as { attachment_id: string; filename: string };
  const { fileAddress } = extension.options as { fileAddress: FileAddress };
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
        <a className="file-tile" href={`${fileAddress(id)}?download=1`} download={filename}>
          {label}
          <IconDownload size={DOWNLOAD_ICON_SIZE} />
        </a>
      )}
    </NodeViewWrapper>
  );
}

export const AttachmentFile = DocumentNode.create<{ fileAddress: FileAddress }>({
  name: 'file',
  group: 'block',
  atom: true,
  draggable: true,
  addOptions() {
    return { fileAddress: pageFileAddress };
  },
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
  if (attachment.inline) {
    return { type: 'image', attrs: { attachment_id: attachment.id, alt: attachment.filename } };
  }
  const [kind] = attachment.content_type.split('/');
  if (kind === 'video' || kind === 'audio') {
    return { type: 'media', attrs: { attachment_id: attachment.id, kind } };
  }
  return { type: 'file', attrs: { attachment_id: attachment.id, filename: attachment.filename } };
}

export type NameChoice = 'version' | 'both' | 'cancel';

/** Keep both puts the number before the extension: a.txt becomes a (2).txt. */
export function copyName(filename: string, copy: number): string {
  const dot = filename.lastIndexOf('.');
  if (dot <= 0) return `${filename} (${copy})`;
  return `${filename.slice(0, dot)} (${copy})${filename.slice(dot)}`;
}

function isNameTaken(error: unknown): boolean {
  return error instanceof ApiError && error.code === 'name_taken';
}

/** Uploads a file and asks first when the page already has that name (FIL-012). */
export async function uploadAsking(
  pageId: string,
  file: File,
  ask: (filename: string) => Promise<NameChoice>,
): Promise<Attachment | null> {
  const send = (filename: string, query: string) =>
    api.upload<Attachment>(
      `/pages/${pageId}/attachments?filename=${encodeURIComponent(filename)}${query}`,
      file,
    );
  const asking = '&on_conflict=ask';
  try {
    return await send(file.name, asking);
  } catch (error) {
    if (!isNameTaken(error)) throw error;
  }
  const choice = await ask(file.name);
  if (choice === 'cancel') return null;
  if (choice === 'version') return send(file.name, '');
  for (let copy = 2; copy < MAX_COPIES; copy += 1) {
    try {
      return await send(copyName(file.name, copy), asking);
    } catch (error) {
      if (!isNameTaken(error)) throw error;
    }
  }
  return send(copyName(file.name, MAX_COPIES), asking);
}

export function NameTakenDialog({
  filename,
  onAnswer,
}: {
  filename: string;
  onAnswer: (choice: NameChoice) => void;
}) {
  const { t } = useTranslation();
  return (
    <Dialog title={t('files.nameTaken')} onClose={() => onAnswer('cancel')}>
      <p>{t('files.nameTakenHelp', { name: filename })}</p>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={() => onAnswer('cancel')}>
          {t('common.cancel')}
        </button>
        <button type="button" onClick={() => onAnswer('both')}>
          {t('files.keepBoth')}
        </button>
        <button type="button" className="primary" onClick={() => onAnswer('version')}>
          {t('files.newVersion')}
        </button>
      </div>
    </Dialog>
  );
}
