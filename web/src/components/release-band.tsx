// SPDX-License-Identifier: Apache-2.0
// Draft and published spaces on a page (spec 073, PUB-1221 to PUB-1231): the release status of
// a draft page under its byline, the quiet line of a page that was released, and the dialog
// that releases one page after its preview.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PageDetail, type ReleasePreview, type ReleaseStatus } from '../api';
import { useCapabilities } from '../capabilities';
import { Link } from '../router';
import { Dialog, ErrorNote, Field, formatSize, Loading, RelativeTime } from './ui';
import { useToast } from './toast';

/** The status of a page in a pair. Nothing is asked of a page outside a pair. */
export function useReleaseStatus(page: PageDetail | undefined, pairRole: string | undefined) {
  const capabilities = useCapabilities();
  return useQuery({
    queryKey: ['release', page?.id],
    queryFn: () => api.get<ReleaseStatus>(`/pages/${page?.id}/release`),
    enabled: capabilities.publishing && !!pairRole && !!page && page.current_version > 0,
  });
}

function aspectWords(t: (key: string) => string, aspects: string[]): string {
  return aspects.map((a) => t(`publishing.aspect_${a}`)).join(', ');
}

/** The band under the byline: on a draft page its release status, for writers and release
 * managers; on a twin a quiet line that it was released, and for whoever may see the draft a
 * link to it. Never colour alone: every state is words. */
export function ReleaseBand({ status }: { status: ReleaseStatus | undefined }) {
  const { t } = useTranslation();
  if (!status || status.side === '') return null;
  if (status.side === 'published') {
    if (!status.released_at) return null;
    return (
      <p className="muted release-line">
        {t('publishing.twinLine')} <RelativeTime value={status.released_at} />
        {status.twin_version ? ` · ${t('publishing.twinVersion', { number: status.twin_version })}` : ''}
        {status.draft_page_id && (
          <>
            {' · '}
            <Link href={`/pages/${status.draft_page_id}`}>{t('publishing.openDraft')}</Link>
          </>
        )}
      </p>
    );
  }
  let line: string;
  switch (status.status) {
    case 'released':
      return (
        <p className="muted release-line">
          {t('publishing.statusReleased')}
          {status.twin_page_id && (
            <>
              {' · '}
              <Link href={`/pages/${status.twin_page_id}`}>{t('publishing.openTwin')}</Link>
            </>
          )}
        </p>
      );
    case 'changed':
      line = t('publishing.statusChanged', { aspects: aspectWords(t, status.aspects) });
      break;
    case 'cannot_release':
      line = t('publishing.statusCannot', { reason: t(`publishing.reason_${status.reason ?? 'kind'}`) });
      break;
    default:
      line = t('publishing.statusNotReleased');
  }
  return (
    <div role="status" className="note release-band">
      <p>
        {status.released_version ? `${t('publishing.releasedAs', { number: status.twin_version })} · ` : ''}
        {status.released_at && (
          <>
            <RelativeTime value={status.released_at} />
            {' · '}
          </>
        )}
        {line}
      </p>
      {status.twin_page_id && (
        <p>
          <Link href={`/pages/${status.twin_page_id}`}>{t('publishing.openTwin')}</Link>
        </p>
      )}
    </div>
  );
}

/** Releases one page: a preview first, then a note for readers, then the release itself. */
export function ReleaseDialog({ page, onClose }: { page: PageDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const toast = useToast();
  const [note, setNote] = useState('');
  const [tell, setTell] = useState(true);
  const preview = useQuery({
    queryKey: ['release-preview', page.id],
    queryFn: () => api.post<ReleasePreview>(`/pages/${page.id}/release/preview`),
    gcTime: 0,
  });
  const run = useMutation({
    mutationFn: () =>
      api.post(`/pages/${page.id}/release`, { preview_id: preview.data?.id, note, silent: !tell }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['release', page.id] });
      toast({ message: t('publishing.released') });
      onClose();
    },
  });
  const p = preview.data;
  return (
    <Dialog title={t('publishing.releaseTitle')} onClose={onClose}>
      <ErrorNote error={preview.error ?? run.error} />
      {preview.isLoading && <Loading />}
      {p && (
        <>
          <p>
            {p.action === 'make' && t('publishing.previewMake', { version: p.version })}
            {p.action === 'change' &&
              t('publishing.previewChange', { version: p.version, aspects: aspectWords(t, p.aspects) })}
            {p.action === 'update' && t('publishing.previewUpdate', { aspects: aspectWords(t, p.aspects) })}
            {p.action === 'none' && t('publishing.previewNone')}
          </p>
          {p.files > 0 && (
            <p>{t('publishing.previewFiles', { count: p.files, size: formatSize(p.bytes) })}</p>
          )}
          {p.placed_higher && <p className="note">{t('publishing.previewHigher')}</p>}
          {p.links_as_text > 0 && (
            <p className="note">{t('publishing.previewLinks', { count: p.links_as_text })}</p>
          )}
          <Field label={t('publishing.note')} help={t('publishing.noteHelp')}>
            <textarea value={note} maxLength={2000} rows={3} onChange={(e) => setNote(e.target.value)} />
          </Field>
          <label className="check">
            <input type="checkbox" checked={tell} onChange={(e) => setTell(e.target.checked)} />{' '}
            {t('publishing.tellWatchers')}
          </label>
        </>
      )}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" disabled={!p || run.isPending} onClick={() => run.mutate()}>
          {t('publishing.confirm')}
        </button>
      </div>
    </Dialog>
  );
}
