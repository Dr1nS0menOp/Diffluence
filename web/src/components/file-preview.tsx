// SPDX-License-Identifier: Apache-2.0
// A look at a file without downloading it. Nothing in here runs what a file holds: an
// image is an image, text is text, and the rest goes to the viewers in their sandbox.
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api';
import { useCapabilities } from '../capabilities';
import { craftOf, OpenInCraft } from './craft-frame';
import { highlightFile, type HighlightedText } from './editor/code';
import { Dialog, ErrorNote, Loading } from './ui';

/** What a preview needs to know about a file. */
export interface PreviewFile {
  id: string;
  filename: string;
  content_type: string;
  inline: boolean;
  office: boolean;
  /** Whether the person may store a changed PDF as a new version (ARC-099). */
  may_add_version?: boolean;
}

const TEXT_FILE = /^text\/|json|xml|yaml/;

export const isTextFile = (contentType: string) => TEXT_FILE.test(contentType);

type HighlightedNode = HighlightedText['children'][number];

function renderNode(node: HighlightedNode, key: number): ReactNode {
  if (node.type === 'text') return node.value;
  if (node.type !== 'element') return null;
  const { className } = node.properties;
  return (
    <span key={key} className={Array.isArray(className) ? className.join(' ') : undefined}>
      {node.children.map(renderNode)}
    </span>
  );
}

/** Text with its colours, as data and never as markup (FIL-028). */
export function TextPreview({ file }: { file: { id: string; filename: string; version?: number } }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['file-text', file.id, file.version],
    queryFn: async () => {
      const body = await api.get<{ text: string; cut: boolean }>(`/attachments/${file.id}/text`);
      return { ...body, coloured: await highlightFile(file.filename, body.text) };
    },
  });
  return (
    <section aria-label={t('files.preview')}>
      <h3>{t('files.preview')}</h3>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && (
        <pre className="file-text">
          <code>{q.data.coloured ? q.data.coloured.children.map(renderNode) : q.data.text}</code>
        </pre>
      )}
      {q.data?.cut && <p className="muted">{t('files.cut')}</p>}
    </section>
  );
}

function PreviewBody({ file }: { file: PreviewFile }) {
  const { t } = useTranslation();
  const capabilities = useCapabilities();
  const address = `/api/v1/attachments/${file.id}/content`;
  if (file.inline) return <img className="file-preview-image" src={address} alt={file.filename} />;
  if (isTextFile(file.content_type)) return <TextPreview file={file} />;
  if (craftOf(file.filename, file.content_type)) {
    return (
      <OpenInCraft file={file} address={address} versionOf={file.may_add_version ? file.id : undefined} />
    );
  }
  if (file.office && capabilities.office_preview) {
    return (
      <OpenInCraft
        file={{ filename: `${file.filename}.pdf`, content_type: 'application/pdf' }}
        address={`/api/v1/attachments/${file.id}/preview.pdf`}
      />
    );
  }
  return <p className="muted">{t('files.noPreview')}</p>;
}

/** Opens on one file and steps through the others of the same list (FIL-027). */
export function FilePreviewDialog({
  files,
  startId,
  onClose,
}: {
  files: PreviewFile[];
  startId: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [index, setIndex] = useState(() =>
    Math.max(
      files.findIndex((file) => file.id === startId),
      0,
    ),
  );
  const file = files[index];
  if (!file) return null;
  return (
    <Dialog title={file.filename} onClose={onClose} wide>
      <PreviewBody key={file.id} file={file} />
      <div className="dialog-actions">
        <a className="button" href={`/api/v1/attachments/${file.id}/content?download=1`}>
          {t('attachments.download')}
        </a>
        {files.length > 1 && (
          <>
            <button type="button" disabled={index === 0} onClick={() => setIndex(index - 1)}>
              {t('files.previous')}
            </button>
            <button type="button" disabled={index === files.length - 1} onClick={() => setIndex(index + 1)}>
              {t('files.next')}
            </button>
          </>
        )}
      </div>
    </Dialog>
  );
}
