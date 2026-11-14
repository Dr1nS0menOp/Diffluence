// SPDX-License-Identifier: Apache-2.0
// The dialogs of releases of more than one page (spec 073, PUB-1213, PUB-420, PUB-440): the
// preview of a run with its note and its choice to tell watchers, and leaving a page out.
// Escape closes them without doing anything, and focus goes back to the button that opened them.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ReleaseRun, type ReleaseStatus, type RunPage, type RunPreview } from '../api';
import { Dialog, ErrorNote, Field, formatSize, Loading } from './ui';
import { useToast } from './toast';

/** What a run takes, as the API asks for it. */
export interface RunScope {
  scope: 'branch' | 'space' | 'pages' | 'take_off';
  root_page_id?: string;
  page_ids?: string[];
  with_children?: boolean;
}

function PageList({ pages, reasons }: { pages: RunPage[]; reasons: boolean }) {
  const { t } = useTranslation();
  return (
    <ul className="plain-list">
      {pages.map((p) => (
        <li key={`${p.page_id}-${p.twin_page_id ?? ''}`}>
          {p.title}{' '}
          <span className="muted">
            {reasons && p.reason ? t(`publishing.skip_${p.reason}`) : t(`publishing.runAction_${p.action}`)}
          </span>
        </li>
      ))}
    </ul>
  );
}

/** The preview of a run, its note and its confirmation (PUB-430, PUB-1213). */
export function RunDialog({
  spaceId,
  scope,
  onClose,
  onStarted,
}: {
  spaceId: string;
  scope: RunScope;
  onClose: () => void;
  onStarted: (run: ReleaseRun) => void;
}) {
  const { t } = useTranslation();
  const toast = useToast();
  const [note, setNote] = useState('');
  const [tell, setTell] = useState(true);
  const [named, setNamed] = useState<string[]>([]);
  const [keepRemovals, setKeepRemovals] = useState(false);
  const preview = useQuery({
    queryKey: ['run-preview', spaceId, scope, named, keepRemovals],
    queryFn: () =>
      api.post<RunPreview>(`/spaces/${spaceId}/releases/preview`, {
        ...scope,
        named,
        keep_removals: keepRemovals,
      }),
    gcTime: 0,
  });
  const start = useMutation({
    mutationFn: () =>
      api.post<ReleaseRun>(`/spaces/${spaceId}/releases/${preview.data?.id}/start`, { note, silent: !tell }),
    onSuccess: (run) => {
      toast({ message: t('publishing.runStarted') });
      onStarted(run);
      onClose();
    },
  });
  const p = preview.data;
  const takingOff = scope.scope === 'take_off';
  const total = p
    ? p.counts.make + p.counts.change + p.counts.update + p.counts.take_off + p.counts.archive
    : 0;
  const restricted = p?.skipped.filter((s) => s.reason === 'restricted') ?? [];
  return (
    <Dialog
      title={takingOff ? t('publishing.takeOffTitle') : t('publishing.runTitle')}
      onClose={onClose}
      wide
    >
      <ErrorNote error={preview.error ?? start.error} />
      {preview.isLoading && <Loading />}
      {p && (
        <>
          <p>
            {t('publishing.runSentence', {
              count: total,
              change: p.counts.change + p.counts.update,
              make: p.counts.make,
              off: p.counts.take_off,
            })}
          </p>
          {p.counts.archive > 0 && <p>{t('publishing.runArchive', { count: p.counts.archive })}</p>}
          {p.pages.length > 0 && (
            <details>
              <summary>{t('publishing.runPages')}</summary>
              <PageList pages={p.pages} reasons={false} />
            </details>
          )}
          {p.skipped.length > 0 && (
            <details>
              <summary>{t('publishing.runSkipped')}</summary>
              <PageList pages={p.skipped} reasons />
            </details>
          )}
          {p.kept_below.length > 0 && (
            <details>
              <summary>{t('publishing.runKept')}</summary>
              <PageList pages={p.kept_below} reasons={false} />
            </details>
          )}
          {p.files > 0 && (
            <p>
              {p.room === null
                ? t('publishing.runFiles', { size: formatSize(p.bytes) })
                : t('publishing.runFilesRoom', { size: formatSize(p.bytes), room: formatSize(p.room) })}
            </p>
          )}
          {!p.fits && <p className="note-error">{t('publishing.runNoRoom')}</p>}
          {p.links_as_text > 0 && (
            <p className="note">{t('publishing.runLinks', { count: p.links_as_text })}</p>
          )}
          {p.batches > 0 && <p className="note">{t('publishing.runBatches', { count: p.batches })}</p>}
          {p.counts.hidden > 0 && (
            <p className="note">{t('publishing.runHidden', { count: p.counts.hidden })}</p>
          )}
          {restricted.length > 0 && (
            <fieldset>
              <legend>{t('publishing.runRestricted', { count: restricted.length })}</legend>
              {restricted.map((r) => (
                <label key={r.page_id} className="check">
                  <input
                    type="checkbox"
                    checked={named.includes(r.page_id)}
                    onChange={(e) =>
                      setNamed(
                        e.target.checked ? [...named, r.page_id] : named.filter((id) => id !== r.page_id),
                      )
                    }
                  />{' '}
                  {r.title} · {t('publishing.runName')}
                </label>
              ))}
            </fieldset>
          )}
          {scope.scope === 'space' && (
            <label className="check">
              <input
                type="checkbox"
                checked={!keepRemovals}
                onChange={(e) => setKeepRemovals(!e.target.checked)}
              />{' '}
              {t('publishing.runKeepRemovals')}
            </label>
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
        <button
          type="button"
          className="primary"
          disabled={!p || !p.fits || start.isPending}
          onClick={() => start.mutate()}
        >
          {takingOff
            ? t('publishing.takeOffConfirm', { count: total })
            : t('publishing.runConfirm', { count: total })}
        </button>
      </div>
    </Dialog>
  );
}

/** Leaves a page out of releases; a released page asks whether readers keep it (PUB-420, PUB-421). */
export function LeaveOutDialog({
  pageId,
  released,
  onClose,
  onTakeOff,
}: {
  pageId: string;
  released: boolean;
  onClose: () => void;
  onTakeOff: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [withChildren, setWithChildren] = useState(false);
  const [reason, setReason] = useState('');
  const [takeOff, setTakeOff] = useState(false);
  const leave = useMutation({
    mutationFn: () =>
      api.put<ReleaseStatus>(`/pages/${pageId}/leave-out`, { with_children: withChildren, reason }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['release', pageId] });
      await client.invalidateQueries({ queryKey: ['release-screen'] });
      onClose();
      if (takeOff) onTakeOff();
    },
  });
  return (
    <Dialog title={t('publishing.leaveOutTitle')} onClose={onClose}>
      <ErrorNote error={leave.error} />
      <p>{t('publishing.leaveOutExplain')}</p>
      <label className="check">
        <input type="checkbox" checked={withChildren} onChange={(e) => setWithChildren(e.target.checked)} />{' '}
        {t('publishing.leaveOutChildren')}
      </label>
      <Field label={t('publishing.leaveOutReason')}>
        <input value={reason} maxLength={500} onChange={(e) => setReason(e.target.value)} />
      </Field>
      {released && (
        <fieldset>
          <legend>{t('publishing.leaveOutReleased')}</legend>
          <label className="check">
            <input type="radio" name="leave-out-twin" checked={!takeOff} onChange={() => setTakeOff(false)} />{' '}
            {t('publishing.leaveOutKeep')}
          </label>
          <label className="check">
            <input type="radio" name="leave-out-twin" checked={takeOff} onChange={() => setTakeOff(true)} />{' '}
            {t('publishing.leaveOutTakeOff')}
          </label>
        </fieldset>
      )}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="primary" disabled={leave.isPending} onClick={() => leave.mutate()}>
          {t('publishing.leaveOutConfirm')}
        </button>
      </div>
    </Dialog>
  );
}
