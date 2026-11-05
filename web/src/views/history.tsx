// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type DiffBlock, type PageDetail, type Version } from '../api';
import { Link, navigate, useLocation } from '../router';
import { DocumentView } from '../components/editor/editor';
import { ConfirmButton, Dialog, ErrorNote, formatDate, Loading } from '../components/ui';

export function HistoryView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
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
  const version = useQuery({
    queryKey: ['version', pageId, shown],
    queryFn: () => api.get<Version>(`/pages/${pageId}/versions/${shown}`),
    enabled: shown !== null,
  });
  const restore = useMutation({
    mutationFn: (n: number) => api.post(`/pages/${pageId}/versions/${n}/restore`),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['page', pageId] });
      await client.invalidateQueries({ queryKey: ['versions', pageId] });
      navigate(`/pages/${pageId}`);
    },
  });
  const toggle = (n: number) =>
    setPicked((p) => (p.includes(n) ? p.filter((x) => x !== n) : [...p.slice(-1), n]));
  const pair = [...picked].sort((a, b) => a - b);
  const canEdit = page.data?.can['page.edit'];
  return (
    <>
      <h1>{t('history.title')}</h1>
      <p>
        <Link href={`/pages/${pageId}`}>{page.data?.title}</Link>
      </p>
      <ErrorNote error={q.error ?? restore.error} />
      <table>
        <thead>
          <tr>
            <th>{t('history.pick')}</th>
            <th>{t('common.name')}</th>
            <th>{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.versions.map((v, i) => (
            <tr key={v.number}>
              <td>
                <input
                  type="checkbox"
                  aria-label={t('history.version', { number: v.number })}
                  checked={picked.includes(v.number)}
                  onChange={() => toggle(v.number)}
                />
              </td>
              <td>
                <strong>{t('history.version', { number: v.number })}</strong>{' '}
                {t('history.by', { name: v.author_name })}{' '}
                <span className="muted">{formatDate(v.created_at)}</span>{' '}
                {i === 0 && <span className="lozenge">{t('history.current')}</span>}
                {v.restored_from !== null && (
                  <span className="muted"> · {t('history.restoredFrom', { number: v.restored_from })}</span>
                )}
                {v.summary && <div>{v.summary}</div>}
              </td>
              <td className="row">
                <button type="button" className="ghost" onClick={() => setShown(v.number)}>
                  {t('history.view')}
                </button>
                {canEdit && i > 0 && (
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
      {pair.length === 2 && (
        <div>
          <Link className="button primary" href={`/pages/${pageId}/compare?from=${pair[0]}&to=${pair[1]}`}>
            {t('history.compareSelected')}
          </Link>
        </div>
      )}
      {shown !== null && (
        <Dialog title={t('history.version', { number: shown })} onClose={() => setShown(null)} wide>
          {version.data?.document ? <DocumentView document={version.data.document} /> : <Loading />}
        </Dialog>
      )}
    </>
  );
}

function Block({ block }: { block: DiffBlock }) {
  if (block.op === 'changed' && block.words) {
    return (
      <p className="diff changed">
        {block.words.map((w, i) => (
          <span key={i} className={w.op}>
            {w.text}{' '}
          </span>
        ))}
      </p>
    );
  }
  return (
    <p className={'diff ' + block.op}>{block.op === 'added' ? block.after : block.before || block.after}</p>
  );
}

export function CompareView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const { searchParams } = useLocation();
  const from = searchParams.get('from');
  const to = searchParams.get('to');
  const q = useQuery({
    queryKey: ['compare', pageId, from, to],
    queryFn: () =>
      api.get<{ from: Version; to: Version; blocks: DiffBlock[] }>(
        `/pages/${pageId}/compare?from=${from}&to=${to}`,
      ),
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const changed = q.data.blocks.some((b) => b.op !== 'equal') || q.data.from.title !== q.data.to.title;
  return (
    <>
      <h1>{t('editor.compare')}</h1>
      <p>
        <Link href={`/pages/${pageId}/history`}>{t('history.title')}</Link>
      </p>
      <p>{t('history.comparing', { from: q.data.from.number, to: q.data.to.number })}</p>
      {!changed && <p className="muted">{t('history.unchanged')}</p>}
      <div>
        {q.data.blocks.map((b, i) => (
          <Block key={i} block={b} />
        ))}
      </div>
    </>
  );
}
