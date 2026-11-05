// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type TaskItem } from '../api';
import { Link } from '../router';
import { formatStoredDate, today } from '../components/editor/inline';
import { ErrorNote, Field, Loading, Tabs } from '../components/ui';

const TABS = ['assigned', 'created'] as const;
type Tab = (typeof TABS)[number];
const GROUPS = ['overdue', 'today', 'week', 'later', 'none'] as const;
export type TaskGroup = (typeof GROUPS)[number];
const WEEK_DAYS = 7;
const MS_PER_DAY = 86_400_000;

/** Where a task belongs in the list, by its date. */
export function taskGroup(due: string | null, now = new Date()): TaskGroup {
  if (!due) return 'none';
  const days = (Date.parse(`${due}T00:00:00Z`) - Date.parse(`${today(now)}T00:00:00Z`)) / MS_PER_DAY;
  if (days < 0) return 'overdue';
  if (days === 0) return 'today';
  return days <= WEEK_DAYS ? 'week' : 'later';
}

function Row({ task, onDone }: { task: TaskItem; onDone: (done: boolean) => void }) {
  const { t } = useTranslation();
  const title = task.text || t('tasks.untitled');
  return (
    <li>
      <input
        type="checkbox"
        aria-label={t('tasks.tick', { title })}
        checked={task.done}
        onChange={(e) => onDone(e.target.checked)}
      />
      <Link href={`/pages/${task.page_id}`}>{title}</Link>
      <span className="muted">
        {task.page_title} · {task.space_name}
        {task.due && ` · ${formatStoredDate(task.due)}`}
      </span>
    </li>
  );
}

export function TasksView() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [tab, setTab] = useState<Tab>('assigned');
  const [status, setStatus] = useState('open');
  const key = ['tasks', tab, status];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ tasks: TaskItem[] }>(`/me/tasks?tab=${tab}&status=${status}`),
  });
  const tick = useMutation({
    mutationFn: (v: { id: string; done: boolean }) => api.post(`/tasks/${v.id}/done`, { done: v.done }),
    onSettled: () => client.invalidateQueries({ queryKey: ['tasks'] }),
  });

  const list = q.data?.tasks ?? [];
  return (
    <>
      <h1>{t('tasks.title')}</h1>
      <Tabs
        names={TABS}
        current={tab}
        label={t('tasks.title')}
        nameOf={(name) => t(`tasks.tab_${name}`)}
        onChange={setTab}
      />
      <div className="tabs-filter">
        <Field label={t('tasks.show')}>
          <select value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="open">{t('tasks.open')}</option>
            <option value="done">{t('tasks.done')}</option>
            <option value="all">{t('tasks.all')}</option>
          </select>
        </Field>
      </div>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? tick.error} />
      {q.data && list.length === 0 && <p className="muted">{t('tasks.none')}</p>}
      {GROUPS.map((group) => {
        const rows = list.filter((task) => taskGroup(task.due) === group);
        if (rows.length === 0) return null;
        return (
          <section key={group}>
            <h2>{t(`tasks.group_${group}`)}</h2>
            <ul className="page-list">
              {rows.map((task) => (
                <Row key={task.id} task={task} onDone={(done) => tick.mutate({ id: task.id, done })} />
              ))}
            </ul>
          </section>
        );
      })}
    </>
  );
}
