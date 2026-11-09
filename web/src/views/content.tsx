// SPDX-License-Identifier: Apache-2.0
// The Content screen: one table to keep a space tidy (spec 011, D12).
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type BulkFailure, type ContentRow, type DirectoryEntry, type Job, type TreeNode } from '../api';
import { Link, navigate, useLocation } from '../router';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { SubjectPicker } from '../components/picker';
import { StageMark } from '../components/page-marks';
import { Dialog, ErrorNote, Field, Loading, RelativeTime } from '../components/ui';
import { TableBox } from '../components/table-box';

const FLAGS = ['owner_disabled', 'restricted', 'broken_links', 'orphaned', 'past_review', 'with_parents'];
const ACTIONS = ['archive', 'delete', 'owner', 'stage', 'label_add', 'label_remove', 'move'] as const;
type Action = (typeof ACTIONS)[number];
const MONTHS = ['', '1', '3', '6', '12'];
const SORTS = ['title', 'changed', 'created', 'owner'];
const ROWS = 50;

interface Plan {
  pages: number;
  allowed: number;
  refused: BulkFailure[];
}

interface JobResult {
  done?: number;
  failed?: BulkFailure[];
}

/** Selecting a parent selects its children, which can be cleared one by one. */
export function selectWithChildren(rows: ContentRow[], picked: string[], id: string): string[] {
  if (picked.includes(id)) return picked.filter((x) => x !== id);
  const children = rows.filter((r) => r.parent_id === id).map((r) => r.id);
  return [...new Set([...picked, id, ...children])];
}

function useFilters() {
  const { pathname, searchParams } = useLocation();
  const set = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete('offset');
    const query = next.toString();
    navigate(query ? `${pathname}?${query}` : pathname, true);
  };
  return { params: searchParams, set };
}

function ActionDialog({
  spaceId,
  action,
  ids,
  onClose,
  onDone,
}: {
  spaceId: string;
  action: Action;
  ids: string[];
  onClose: () => void;
  onDone: () => void;
}) {
  const { t } = useTranslation();
  const [owner, setOwner] = useState<DirectoryEntry | null>(null);
  const [word, setWord] = useState('');
  const [parent, setParent] = useState('');
  const [job, setJob] = useState<string | null>(null);
  const tree = useQuery({
    queryKey: ['tree', spaceId],
    queryFn: () => api.get<{ pages: TreeNode[] }>(`/spaces/${spaceId}/tree`),
    enabled: action === 'move',
  });
  const body = {
    action,
    page_ids: ids,
    ...(owner ? { owner_id: owner.id } : {}),
    ...(action === 'stage' ? { stage_text: word, stage_color: 'grey' } : {}),
    ...(action.startsWith('label') ? { label: word } : {}),
    ...(action === 'move' ? { parent_id: parent || null } : {}),
  };
  const plan = useQuery({
    queryKey: ['content-plan', action, ids],
    queryFn: () => api.post<Plan>(`/spaces/${spaceId}/content/preview`, body),
  });
  const run = useMutation({
    mutationFn: () => api.post<{ job_id: string }>(`/spaces/${spaceId}/content/run`, body),
    onSuccess: (r) => setJob(r.job_id),
  });
  const status = useQuery({
    queryKey: ['job', job],
    queryFn: () => api.get<Job>(`/jobs/${job}`),
    enabled: !!job,
    refetchInterval: (q) => (['done', 'failed'].includes(q.state.data?.status ?? '') ? false : 1000),
  });
  const finished = status.data?.status === 'done' || status.data?.status === 'failed';
  useEffect(() => {
    if (finished) onDone();
  }, [finished, onDone]);
  const result = status.data?.result as JobResult | undefined;
  const needsWord = action === 'stage' || action.startsWith('label');
  const ready = (action !== 'owner' || owner) && (!needsWord || word.trim() !== '');
  return (
    <Dialog title={t(`content.action_${action}`)} onClose={onClose}>
      <ErrorNote error={plan.error ?? run.error} />
      {plan.data && !job && (
        <p>
          {t('content.summary', { allowed: plan.data.allowed, pages: plan.data.pages })}
          {plan.data.refused.length > 0 && ` ${t('content.refused', { count: plan.data.refused.length })}`}
        </p>
      )}
      {action === 'owner' && !job && (
        <>
          <SubjectPicker label={t('content.newOwner')} usersOnly onPick={setOwner} />
          {owner && <p>{owner.name}</p>}
        </>
      )}
      {needsWord && !job && (
        <Field label={action === 'stage' ? t('organise.statusWord') : t('organise.addLabel')}>
          <input value={word} onChange={(e) => setWord(e.target.value)} />
        </Field>
      )}
      {action === 'move' && !job && (
        <Field label={t('page.moveTo')}>
          <select value={parent} onChange={(e) => setParent(e.target.value)}>
            <option value="">{t('page.topLevel')}</option>
            {tree.data?.pages
              .filter((n) => !ids.includes(n.id) && n.status === 'published')
              .map((n) => (
                <option key={n.id} value={n.id}>
                  {n.title}
                </option>
              ))}
          </select>
        </Field>
      )}
      {job && !finished && (
        <p role="status" className="muted">
          {t('content.working')}
        </p>
      )}
      {finished && (
        <div role="status">
          <p>{t('content.report', { done: result?.done ?? 0, failed: result?.failed?.length ?? 0 })}</p>
          <ul className="plain-list">
            {result?.failed?.map((f) => (
              <li key={f.id}>
                <Link href={`/pages/${f.id}`}>{f.id}</Link> <span className="muted">{f.reason}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {job ? t('organise.done') : t('common.cancel')}
        </button>
        {!job && (
          <button
            type="button"
            className="primary"
            disabled={!ready || run.isPending}
            onClick={() => run.mutate()}
          >
            {t(`content.action_${action}`)}
          </button>
        )}
      </div>
    </Dialog>
  );
}

export function ContentView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery(spaceByKeyQuery(spaceKey));
  const space = q.data;
  const { params, set } = useFilters();
  const offset = Number(params.get('offset') ?? 0);
  const [picked, setPicked] = useState<string[]>([]);
  const [action, setAction] = useState<Action | null>(null);
  const query = params.toString();
  const rows = useQuery({
    queryKey: ['content', space?.id, query],
    queryFn: () =>
      api.get<{ pages: ContentRow[]; total: number }>(`/spaces/${space?.id}/content?${query}&limit=${ROWS}`),
    enabled: !!space,
  });
  const list = rows.data?.pages ?? [];
  const total = rows.data?.total ?? 0;
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!space) return <Loading />;
  return (
    <Frame space={space}>
      <div className="wide-area">
        <h1>{t('content.title')}</h1>
        <form
          className="filters content-filters"
          aria-label={t('content.filters')}
          onSubmit={(e) => e.preventDefault()}
        >
          <label className="field">
            <span>{t('organise.status')}</span>
            <input value={params.get('stage') ?? ''} onChange={(e) => set('stage', e.target.value)} />
          </label>
          <label className="field">
            <span>{t('organise.labels')}</span>
            <input value={params.get('label') ?? ''} onChange={(e) => set('label', e.target.value)} />
          </label>
          <label className="field">
            <span>{t('content.inactive')}</span>
            <select
              value={params.get('inactive_months') ?? ''}
              onChange={(e) => set('inactive_months', e.target.value)}
            >
              {MONTHS.map((m) => (
                <option key={m} value={m}>
                  {m ? t('content.months', { count: Number(m) }) : t('content.any')}
                </option>
              ))}
            </select>
          </label>
          {(['changed_from', 'changed_until', 'created_from', 'created_until'] as const).map((key) => (
            <label key={key} className="field">
              <span>{t(`content.${key}`)}</span>
              <input type="date" value={params.get(key) ?? ''} onChange={(e) => set(key, e.target.value)} />
            </label>
          ))}
          <label className="field">
            <span>{t('directory.sort')}</span>
            <select value={params.get('sort') ?? 'title'} onChange={(e) => set('sort', e.target.value)}>
              {SORTS.map((s) => (
                <option key={s} value={s}>
                  {t(`content.sort_${s}`)}
                </option>
              ))}
            </select>
          </label>
          <SubjectPicker
            label={t('content.owner')}
            usersOnly
            onPick={(person) => set('owner_id', person.id)}
          />
          {params.get('owner_id') && (
            <button type="button" className="ghost" onClick={() => set('owner_id', '')}>
              {t('content.anyOwner')}
            </button>
          )}
          <SubjectPicker
            label={t('content.contributor')}
            usersOnly
            onPick={(person) => set('contributor_id', person.id)}
          />
          {params.get('contributor_id') && (
            <button type="button" className="ghost" onClick={() => set('contributor_id', '')}>
              {t('content.anyContributor')}
            </button>
          )}
          {FLAGS.map((flag) => (
            <label key={flag} className="check">
              <input
                type="checkbox"
                checked={params.get(flag) === '1'}
                onChange={(e) => set(flag, e.target.checked ? '1' : '')}
              />
              {t(`content.flag_${flag}`)}
            </label>
          ))}
        </form>
        <ErrorNote error={rows.error} />
        {rows.isPending && <Loading />}
        <p role="status" className="muted">
          {t('content.count', { count: total })}
        </p>
        {picked.length > 0 && (
          <div className="row content-actions" role="group" aria-label={t('content.actions')}>
            <span>{t('content.selected', { count: picked.length })}</span>
            {ACTIONS.map((a) => (
              <button key={a} type="button" onClick={() => setAction(a)}>
                {t(`content.action_${a}`)}
              </button>
            ))}
          </div>
        )}
        {list.length > 0 && (
          <TableBox>
            <table>
              <thead>
                <tr>
                  <th>
                    <input
                      type="checkbox"
                      aria-label={t('content.selectAll')}
                      checked={list.every((r) => picked.includes(r.id))}
                      onChange={(e) => setPicked(e.target.checked ? list.map((r) => r.id) : [])}
                    />
                  </th>
                  <th>{t('page.title')}</th>
                  <th>{t('content.owner')}</th>
                  <th>{t('organise.status')}</th>
                  <th>{t('organise.labels')}</th>
                  <th>{t('directory.lastChange')}</th>
                </tr>
              </thead>
              <tbody>
                {list.map((r) => (
                  <tr key={r.id} className={picked.includes(r.id) ? 'picked' : undefined}>
                    <td>
                      <input
                        type="checkbox"
                        aria-label={t('favourites.pick', { title: r.title })}
                        checked={picked.includes(r.id)}
                        onChange={() => setPicked(selectWithChildren(list, picked, r.id))}
                      />
                    </td>
                    <td>
                      <Link href={`/pages/${r.id}`}>{r.title}</Link>
                    </td>
                    <td>
                      {r.owner_name}
                      {r.owner_disabled && <span className="muted"> {t('content.noLongerActive')}</span>}
                    </td>
                    <td>
                      <StageMark text={r.stage_text} color={r.stage_color} />
                    </td>
                    <td>{r.labels.join(', ')}</td>
                    <td>
                      <RelativeTime value={r.updated_at} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableBox>
        )}
        {offset + list.length < total && (
          <button type="button" onClick={() => set('offset', String(offset + ROWS))}>
            {t('common.more')}
          </button>
        )}
        {action && (
          <ActionDialog
            spaceId={space.id}
            action={action}
            ids={picked}
            onClose={() => setAction(null)}
            onDone={() => {
              setPicked([]);
              void client.invalidateQueries({ queryKey: ['content'] });
              void client.invalidateQueries({ queryKey: ['tree', space.id] });
            }}
          />
        )}
      </div>
    </Frame>
  );
}
