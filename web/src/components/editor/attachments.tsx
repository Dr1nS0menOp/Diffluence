// SPDX-License-Identifier: Apache-2.0
// Images and files in a document. Both are attachments of the page, never outside addresses.
import { useLayoutEffect, useRef, useState, type PointerEvent, type RefObject } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Node as DocumentNode } from '@tiptap/core';
import Image, { type ImageOptions } from '@tiptap/extension-image';
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from '@tiptap/react';
import { useTranslation } from 'react-i18next';
import { api, ApiError, type Attachment, type UploadWatch } from '../../api';
import { OpenInCraft } from '../craft-frame';
import { FilePreviewDialog, type PreviewFile } from '../file-preview';
import { IconDownload, IconPage, IconPicture } from '../icons';
import { Dialog, ErrorNote, formatSize, Loading } from '../ui';

const DOWNLOAD_ICON_SIZE = 18;
const MISSING_ICON_SIZE = 20;
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
  width: number | null;
  wrap: boolean;
  border: string | null;
  border_color: string | null;
  href: string | null;
}

const MIN_IMAGE_SHARE = 5;
const MAX_IMAGE_SHARE = 100;

/** A share of the column, from what a dragged corner reached. */
export function imageShare(imageWidth: number, columnWidth: number): number {
  const share = Math.round((imageWidth / Math.max(columnWidth, 1)) * MAX_IMAGE_SHARE);
  return Math.min(MAX_IMAGE_SHARE, Math.max(MIN_IMAGE_SHARE, share));
}

/** The corner that sizes an image. The width is a share, so the reader never gets more than the column. */
function ResizeCorner({
  figure,
  onDone,
  label,
}: {
  figure: RefObject<HTMLElement | null>;
  onDone: (share: number) => void;
  label: string;
}) {
  const resize = (start: PointerEvent<HTMLSpanElement>) => {
    const element = figure.current;
    const column = element?.parentElement;
    if (!element || !column) return;
    start.preventDefault();
    const corner = start.currentTarget;
    corner.setPointerCapture(start.pointerId);
    const left = element.getBoundingClientRect().left;
    const shareAt = (x: number) => imageShare(x - left, column.getBoundingClientRect().width);
    const follow = (e: globalThis.PointerEvent) => {
      element.style.inlineSize = `${shareAt(e.clientX)}%`;
    };
    const finish = (e: globalThis.PointerEvent) => {
      corner.removeEventListener('pointermove', follow);
      corner.removeEventListener('pointerup', finish);
      onDone(shareAt(e.clientX));
    };
    corner.addEventListener('pointermove', follow);
    corner.addEventListener('pointerup', finish);
  };
  return <span className="image-corner" title={label} contentEditable={false} onPointerDown={resize} />;
}

interface FileFacts extends PreviewFile {
  size: number;
  may_add_version: boolean;
}

/** Null means the file is gone or hidden, which look the same on purpose. */
export function useFileFacts(id: string | null, enabled: boolean) {
  return useQuery({
    queryKey: ['file-facts', id],
    queryFn: () => api.post<{ files: FileFacts[] }>('/attachments/lookup', { ids: [id] }),
    select: (found) => found.files[0] ?? null,
    enabled: enabled && !!id,
  });
}

function ImageView({ node, editor, extension, updateAttributes, selected }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const figure = useRef<HTMLElement>(null);
  const share = (node.attrs as ImageAttrs).width;
  // A number has no class of its own, so our code sets it, as for the width of a table.
  useLayoutEffect(() => {
    if (figure.current) figure.current.style.inlineSize = share ? `${share}%` : '';
  }, [share]);
  const {
    attachment_id: id,
    alt,
    caption,
    size,
    align,
    wrap,
    border,
    border_color: borderColor,
    href,
  } = node.attrs as ImageAttrs;
  const { fileAddress } = extension.options as { fileAddress: FileAddress };
  const [isMissing, setIsMissing] = useState(false);
  if (isMissing) {
    return (
      <NodeViewWrapper as="figure" className="doc-image">
        <span className="file-missing">
          <IconPicture size={MISSING_ICON_SIZE} /> {alt ? `${alt}: ` : ''}
          {t('files.imageGone')}
        </span>
      </NodeViewWrapper>
    );
  }
  const image = <img src={fileAddress(id)} alt={alt ?? ''} onError={() => setIsMissing(true)} />;
  return (
    <NodeViewWrapper
      ref={figure}
      as="figure"
      className="doc-image"
      data-size={size ?? undefined}
      data-align={align ?? undefined}
      data-wrap={wrap || undefined}
      data-border={border ?? undefined}
      data-line={borderColor ?? undefined}
    >
      {/* A reader opens it large. The button gives the keyboard a way in. */}
      {editor.isEditable ? (
        <>
          {image}
          {selected && (
            <ResizeCorner
              figure={figure}
              label={t('editor.imageResize')}
              onDone={(share) => updateAttributes({ width: share, size: null })}
            />
          )}
        </>
      ) : href ? (
        <a className="image-link" href={href} rel="noopener noreferrer">
          {image}
        </a>
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
      width: {
        default: null,
        parseHTML: (el: HTMLElement) => Number(el.getAttribute('data-width')) || null,
        renderHTML: (attrs: Record<string, number | null>) =>
          attrs.width ? { 'data-width': attrs.width } : {},
      },
      wrap: {
        default: false,
        parseHTML: (el: HTMLElement) => el.hasAttribute('data-wrap'),
        renderHTML: (attrs: Record<string, boolean>) => (attrs.wrap ? { 'data-wrap': '' } : {}),
      },
      border: dataAttribute('border'),
      border_color: dataAttribute('border_color'),
      href: dataAttribute('href'),
    };
  },
  parseHTML() {
    return [{ tag: 'img[data-attachment-id]' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(ImageView);
  },
});

function InlineImageView({ node, extension }: ReactNodeViewProps) {
  const { attachment_id: id, alt } = node.attrs as { attachment_id: string; alt: string | null };
  const { fileAddress } = extension.options as { fileAddress: FileAddress };
  return (
    <NodeViewWrapper as="span" className="inline-image">
      <img src={fileAddress(id)} alt={alt ?? ''} />
    </NodeViewWrapper>
  );
}

/** A small image inside a line of text, such as an icon (EDT-078). */
export const InlineImage = DocumentNode.create<{ fileAddress: FileAddress }>({
  name: 'inlineImage',
  group: 'inline',
  inline: true,
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
      alt: { default: null },
    };
  },
  parseHTML() {
    return [{ tag: 'span[data-inline-image]' }];
  },
  renderHTML({ HTMLAttributes }) {
    return ['span', { ...HTMLAttributes, 'data-inline-image': '' }];
  },
  addNodeView() {
    return ReactNodeViewRenderer(InlineImageView);
  },
});

/** The type as people name it, from the end of the file name. */
export function fileKind(filename: string): string {
  const dot = filename.lastIndexOf('.');
  return dot > 0 ? filename.slice(dot + 1).toUpperCase() : '';
}

/** The files a document shows, in order, for stepping through them in a preview. */
function shownFileIds(doc: ReactNodeViewProps['editor']['state']['doc']): string[] {
  const found = new Set<string>();
  doc.descendants((node) => {
    const { attachment_id: id } = node.attrs as { attachment_id?: string };
    if (id && (node.type.name === 'file' || node.type.name === 'image')) found.add(id);
    return true;
  });
  return [...found];
}

function PreviewOfDocument({
  startId,
  ids,
  onClose,
}: {
  startId: string;
  ids: string[];
  onClose: () => void;
}) {
  const q = useQuery({
    queryKey: ['file-facts-of', ids],
    queryFn: () => api.post<{ files: FileFacts[] }>('/attachments/lookup', { ids }),
  });
  if (!q.data) return null;
  // The server leaves out what is hidden or gone, in any order.
  const files = ids.flatMap((id) => q.data.files.filter((file) => file.id === id));
  return <FilePreviewDialog files={files} startId={startId} onClose={onClose} />;
}

/** A file in the document: a tile to download. While editing it is a block to move or remove. */
function FileTile({ node, editor, extension }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const {
    attachment_id: id,
    filename,
    display,
  } = node.attrs as { attachment_id: string; filename: string; display: string | null };
  const { fileAddress } = extension.options as { fileAddress: FileAddress };
  const [previewing, setPreviewing] = useState<string[] | null>(null);
  // A template or a public page serves its own files, and has nothing to ask.
  const facts = useFileFacts(id, fileAddress === pageFileAddress);
  const isGone = facts.data === null;
  const label = (
    <>
      <IconPage />
      <span className="file-name">{filename}</span>
      {facts.data && (
        <span className="muted file-facts">
          {fileKind(filename)} · {formatSize(facts.data.size)}
        </span>
      )}
      {isGone && <span className="muted file-facts">{t('files.fileGone')}</span>}
    </>
  );
  return (
    <NodeViewWrapper className="file-block" data-file-tile={id} data-display={display ?? undefined}>
      {editor.isEditable || isGone ? (
        <span className="file-tile">{label}</span>
      ) : (
        <>
          <a className="file-tile" href={`${fileAddress(id)}?download=1`} download={filename}>
            {label}
            <IconDownload size={DOWNLOAD_ICON_SIZE} />
          </a>
          <OpenInCraft
            file={{ filename }}
            address={fileAddress(id)}
            versionOf={facts.data?.may_add_version ? id : undefined}
          />
          {facts.data && (
            <button
              type="button"
              className="ghost"
              onClick={() => setPreviewing(shownFileIds(editor.state.doc))}
            >
              {t('files.preview')}
            </button>
          )}
          {previewing && (
            <PreviewOfDocument startId={id} ids={previewing} onClose={() => setPreviewing(null)} />
          )}
        </>
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
      display: dataAttribute('display'),
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

/** Adds what the page already holds, so one file can show in several places (FIL-015). */
export function PageFilesDialog({
  pageId,
  onPick,
  onClose,
}: {
  pageId: string;
  onPick: (attachment: Attachment) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['attachments', pageId],
    queryFn: () => api.get<{ attachments: Attachment[] }>(`/pages/${pageId}/attachments`),
  });
  return (
    <Dialog title={t('insert.pageFile.name')} onClose={onClose}>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data?.attachments.length === 0 && <p className="muted">{t('attachments.none')}</p>}
      <ul className="plain-list insert-list">
        {q.data?.attachments.map((attachment) => (
          <li key={attachment.id}>
            <button type="button" className="suggestion" onClick={() => onPick(attachment)}>
              <IconPage size={DOWNLOAD_ICON_SIZE} />
              <span className="suggestion-name">{attachment.filename}</span>
              <span className="muted suggestion-hint">{formatSize(attachment.size)}</span>
            </button>
          </li>
        ))}
      </ul>
    </Dialog>
  );
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
  watch?: UploadWatch,
): Promise<Attachment | null> {
  const send = (filename: string, query: string) =>
    api.upload<Attachment>(
      `/pages/${pageId}/attachments?filename=${encodeURIComponent(filename)}${query}`,
      file,
      watch,
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

/** A file dropped on a tile becomes its next version, under the name of the tile (FIL-013). */
export const uploadNewVersion = (id: string, file: File, watch: UploadWatch) =>
  api.upload<Attachment>(
    `/attachments/${id}/versions?filename=${encodeURIComponent(file.name)}`,
    file,
    watch,
  );

/** The id of the tile under a drag, if there is one. */
export function tileUnder(target: EventTarget | null): string | null {
  if (!(target instanceof Element)) return null;
  return target.closest<HTMLElement>('[data-file-tile]')?.dataset.fileTile ?? null;
}

/** Marks the tile a drop would land on. It is only a look, so it stays out of React. */
export function markDropTile(area: HTMLElement, target: EventTarget | null) {
  const id = tileUnder(target);
  area.querySelectorAll<HTMLElement>('[data-file-tile]').forEach((tile) => {
    tile.classList.toggle('drop-target', tile.dataset.fileTile === id);
  });
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
