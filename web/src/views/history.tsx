// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type DiffBlock, type Me, type PageDetail, type Version } from '../api';
import { Link, navigate, useLocation } from '../router';
import { DiffDocument } from '../components/editor/diff-view';
import { DocumentView } from '../components/editor/editor';
import { ConfirmButton, Dialog, ErrorNote, Field, Loading } from '../components/ui';
import { useDates } from '../dates';
import { TableBox } from '../components/table-box';

/** For a leaked secret. The number stays, and the audit log keeps who, when and why. */
function RemoveVersionDialog({
  pageId,
  number,
  onClose,
}: {
  pageId: string;
  number: number;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [reason, setReason] = useState('');
  const remove = useMutation({
    mutationFn: () => api.del(`/admin/pages/${pageId}/versions/${number}`, { reason }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['versions', pageId] });
      onClose();
    },
  });
  return (
    <Dialog title={t('history.removeTitle', { number })} onClose={onClose}>
      <ErrorNote error={remove.error} />
      <p>{t('history.removeHelp')}</p>
      <Field label={t('history.reason')}>
        <input value={reason} required autoFocus onChange={(e) => setReason(e.target.value)} />
      </Field>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        <button type="button" className="danger" disabled={!reason.trim()} onClick={() => remove.mutate()}>
          {t('history.remove')}
        </button>
      </div>
    </Dialog>
  );
}

interface Comparison {
  from: Version;
  to: Version;
  blocks: DiffBlock[];
}

const comparisonQuery = (pageId: string, from: string | number | null, to: string | number | null) => ({
  queryKey: ['compare', pageId, String(from), String(to)],
  queryFn: () => api.get<Comparison>(`/pages/${pageId}/compare?from=${from}&to=${to}`),
});

/** Opens on what changed in this version, because that is why people come here. */
function VersionDialog({ pageId, number, onClose }: { pageId: string; number: number; onClose: () => void }) {
  const { t } = useTranslation();
  const dates = useDates();
  const [clean, setClean] = useState(false);
  const version = useQuery({
    queryKey: ['version', pageId, number],
    queryFn: () => api.get<Version>(`/pages/${pageId}/versions/${number}`),
  });
  const comparison = useQuery({ ...comparisonQuery(pageId, number - 1, number), enabled: number > 1 });
  // An administrator can remove the version before this one, and then there is nothing to compare with.
  const previousGone = comparison.error instanceof ApiError && comparison.error.code === 'version_removed';
  const hasPrevious = number > 1 && !previousGone;
  const shown = version.data;
  return (
    <Dialog title={t('history.version', { number })} onClose={onClose} wide>
      <ErrorNote error={version.error ?? (previousGone ? null : comparison.error)} />
      {!shown?.document && <Loading />}
      {shown?.document && (
        <>
          <p role="status" className="note">
            {t('history.oldVersion', {
              number: shown.number,
              name: shown.author_name,
              date: dates.moment(shown.created_at),
            })}{' '}
            <Link href={`/pages/${pageId}`}>{t('history.toCurrent')}</Link>
          </p>
          {hasPrevious && (
            <p>
              <button type="button" onClick={() => setClean(!clean)}>
                {clean ? t('history.showChanges') : t('history.showClean')}
              </button>
            </p>
          )}
          {hasPrevious && !clean ? (
            comparison.data ? (
              <Changes comparison={comparison.data} />
            ) : (
              <Loading />
            )
          ) : (
            <DocumentView document={shown.document} plain />
          )}
        </>
      )}
    </Dialog>
  );
}

interface FileEvent {
  filename: string;
  action: 'added' | 'version' | 'removed' | 'restored' | 'moved_here' | 'moved_away';
  actor: string;
  at: string;
}

/** Files change without a new version of the page, so they get their own list (FIL-050). */
function FileHistory({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['file-events', pageId],
    queryFn: () => api.get<{ events: FileEvent[] }>(`/pages/${pageId}/file-events`),
  });
  if (q.data?.events.length === 0) return null;
  return (
    <section aria-label={t('history.files')}>
      <h2>{t('history.files')}</h2>
      <ErrorNote error={q.error} />
      <ul className="plain-list">
        {q.data?.events.map((event, index) => (
          <li key={`${event.at}-${index}`}>
            {t(`history.file_${event.action}`, { name: event.filename, person: event.actor })}{' '}
            <span className="muted">{dates.moment(event.at)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function HistoryView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const dates = useDates();
  const client = useQueryClient();
  const page = useQuery({
    queryKey: ['page', pageId],
    queryFn: () => api.get<PageDetail>(`/pages/${pageId}`),
  });
  const q = useQuery({
    queryKey: ['versions', pageId],
    queryFn: () => api.get<{ versions: Version[] }>(`/pages/${pageId}/versions`),
  });
  const [picked, setPicked] = useState<number[]>([]);
  const [shown, setShown] = useState<number | null>(null);
  const restore = useMutation({
    mutationFn: (n: number) => api.post(`/pages/${pageId}/versions/${n}/restore`),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', pageId] });
      await client.invalidateQueries({ queryKey: ['versions', pageId] });
      navigate(`/pages/${pageId}`);
    },
  });
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  const isAdministrator = me.data?.global_permissions.includes('administrator') ?? false;
  const [removing, setRemoving] = useState<number | null>(null);
  const toggle = (n: number) =>
    setPicked((p) => (p.includes(n) ? p.filter((x) => x !== n) : [...p.slice(-1), n]));
  const pair = [...picked].sort((a, b) => a - b);
  const canEdit = page.data?.can['page.edit'];
  const canCompare = (q.data?.versions.length ?? 0) > 1;
  return (
    <>
      <h1>{t('history.title')}</h1>
      <p>
        <Link href={`/pages/${pageId}`}>{page.data?.title}</Link>
      </p>
      <ErrorNote error={q.error ?? restore.error} />
      <TableBox>
        <table>
          <thead>
            <tr>
              {canCompare && <th>{t('history.pick')}</th>}
              <th>{t('common.name')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.versions.map((v, i) => (
              <tr key={v.number}>
                {canCompare && (
                  <td>
                    <input
                      type="checkbox"
                      aria-label={t('history.version', { number: v.number })}
                      checked={picked.includes(v.number)}
                      onChange={() => toggle(v.number)}
                    />
                  </td>
                )}
                <td>
                  <strong>{t('history.version', { number: v.number })}</strong>{' '}
                  {t('history.by', { name: v.author_name })}{' '}
                  <span className="muted">{dates.moment(v.created_at)}</span>{' '}
                  {i === 0 && <span className="lozenge">{t('history.current')}</span>}
                  {v.restored_from !== null && (
                    <span className="muted"> · {t('history.restoredFrom', { number: v.restored_from })}</span>
                  )}
                  {v.removed && <span className="lozenge">{t('history.removed')}</span>}
                  {v.imported && <span className="muted"> · {t('history.imported')}</span>}
                  {v.summary && <div>{v.summary}</div>}
                </td>
                <td className="row">
                  {!v.removed && (
                    <button type="button" className="ghost" onClick={() => setShown(v.number)}>
                      {t('history.view')}
                    </button>
                  )}
                  {isAdministrator && !v.removed && i > 0 && (
                    <button type="button" className="ghost" onClick={() => setRemoving(v.number)}>
                      {t('history.remove')}
                    </button>
                  )}
                  {canEdit && !v.removed && i > 0 && (
                    <ConfirmButton
                      label={t('history.restore')}
                      message={t('history.confirmRestore')}
                      className="ghost"
                      tone="primary"
                      onConfirm={() => restore.mutate(v.number)}
                    />
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
      {pair.length === 2 && (
        <div>
          <Link className="button primary" href={`/pages/${pageId}/compare?from=${pair[0]}&to=${pair[1]}`}>
            {t('history.compareSelected')}
          </Link>
        </div>
      )}
      <FileHistory pageId={pageId} />
      {removing !== null && (
        <RemoveVersionDialog pageId={pageId} number={removing} onClose={() => setRemoving(null)} />
      )}
      {shown !== null && <VersionDialog pageId={pageId} number={shown} onClose={() => setShown(null)} />}
    </>
  );
}

const UNCHANGED_KEPT = 2;

export interface CompareRun {
  blocks: DiffBlock[];
  folded: boolean;
}

/** Long stretches without changes fold, and the blocks next to a change stay. */
export function foldUnchanged(blocks: DiffBlock[]): CompareRun[] {
  const runs: CompareRun[] = [];
  let at = 0;
  while (at < blocks.length) {
    let end = at;
    const same = blocks[at]?.op === 'equal';
    while (end < blocks.length && (blocks[end]?.op === 'equal') === same) end++;
    const run = blocks.slice(at, end);
    const first = at === 0;
    const last = end === blocks.length;
    const keptBefore = first ? 0 : UNCHANGED_KEPT;
    const keptAfter = last ? 0 : UNCHANGED_KEPT;
    if (same && run.length > keptBefore + keptAfter) {
      if (keptBefore) runs.push({ blocks: run.slice(0, keptBefore), folded: false });
      runs.push({ blocks: run.slice(keptBefore, run.length - keptAfter), folded: true });
      if (keptAfter) runs.push({ blocks: run.slice(-keptAfter), folded: false });
    } else {
      runs.push({ blocks: run, folded: false });
    }
    at = end;
  }
  return runs;
}

function FoldedRun({ blocks }: { blocks: DiffBlock[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  if (open) return <DiffDocument blocks={blocks} />;
  return (
    <p className="diff-fold">
      <button type="button" className="ghost" onClick={() => setOpen(true)}>
        {t('history.showUnchanged', { count: blocks.length })}
      </button>
    </p>
  );
}

/** What changed, in the look of the page. Long runs without changes fold. */
function Changes({ comparison }: { comparison: Comparison }) {
  const { t } = useTranslation();
  const { from, to, blocks } = comparison;
  const changed = blocks.some((b) => b.op !== 'equal') || from.title !== to.title;
  return (
    <>
      {!changed && <p className="muted">{t('history.unchanged')}</p>}
      {from.title !== to.title && <p>{t('history.titleChanged', { from: from.title, to: to.title })}</p>}
      {foldUnchanged(blocks).map((run, i) =>
        run.folded ? <FoldedRun key={i} blocks={run.blocks} /> : <DiffDocument key={i} blocks={run.blocks} />,
      )}
    </>
  );
}

export function CompareView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const { searchParams } = useLocation();
  const from = searchParams.get('from');
  const to = searchParams.get('to');
  const q = useQuery(comparisonQuery(pageId, from, to));
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <>
      <h1>{t('editor.compare')}</h1>
      <p>
        <Link href={`/pages/${pageId}/history`}>{t('history.title')}</Link>
      </p>
      <p>{t('history.comparing', { from: q.data.from.number, to: q.data.to.number })}</p>
      <Changes comparison={q.data} />
    </>
  );
}
