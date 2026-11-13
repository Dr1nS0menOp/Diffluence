// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { Chart } from '../components/chart';
import { SortTable, type SortColumn } from '../components/sort-table';
import { ConfirmButton, ErrorNote, Field, Loading } from '../components/ui';

interface InstanceDay {
  day: string;
  active_users: number;
  views: number;
  new_pages: number;
  comments: number;
  searches: number;
}
interface Term {
  term: string;
  searches: number;
  empty: number;
}
interface Settings {
  enabled: boolean;
  mode: 'counts' | 'names';
  retention_days: number;
}

function SearchTerms() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['admin-search-terms'],
    queryFn: () => api.get<{ terms: Term[] }>('/admin/insights/search'),
  });
  const columns = useMemo<SortColumn<Term>[]>(
    () => [
      { key: 'term', label: t('insights.term'), value: (r) => r.term },
      { key: 'searches', label: t('insights.searches'), value: (r) => r.searches },
      { key: 'empty', label: t('insights.withoutResults'), value: (r) => r.empty },
    ],
    [t],
  );
  return (
    <section>
      <div className="section-head">
        <h2>{t('insights.searchTitle')}</h2>
        <a className="button" href="/api/v1/admin/insights/search?format=csv">
          {t('insights.csv')}
        </a>
      </div>
      <p className="muted">{t('insights.searchHelp')}</p>
      <ErrorNote error={q.error} />
      {q.data?.terms.length === 0 && <p className="muted">{t('insights.nothingYet')}</p>}
      {(q.data?.terms.length ?? 0) > 0 && (
        <SortTable
          caption={t('insights.searchTitle')}
          columns={columns}
          rows={q.data?.terms ?? []}
          rowKey={(r) => r.term}
        />
      )}
    </section>
  );
}

/** Administration, Insights: the instance by day, and what people look for. */
export function InsightsTab() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['admin-insights'],
    queryFn: () => api.get<{ days: InstanceDay[] }>('/admin/insights'),
  });
  const days = q.data?.days ?? [];
  const columns = useMemo<SortColumn<InstanceDay>[]>(
    () => [
      { key: 'day', label: t('insights.day'), value: (r) => r.day },
      { key: 'active', label: t('insights.activeUsers'), value: (r) => r.active_users },
      { key: 'views', label: t('insights.views'), value: (r) => r.views },
      { key: 'pages', label: t('insights.newPages'), value: (r) => r.new_pages },
      { key: 'comments', label: t('insights.comments'), value: (r) => r.comments },
      { key: 'searches', label: t('insights.searches'), value: (r) => r.searches },
    ],
    [t],
  );
  return (
    <>
      <ErrorNote error={q.error} />
      {q.isPending && <Loading />}
      {q.data && days.length === 0 && <p className="muted">{t('insights.noDays')}</p>}
      {days.length > 0 && (
        <>
          <Chart
            data={{
              labels: days.map((d) => d.day),
              series: [{ name: t('insights.activeUsers'), values: days.map((d) => d.active_users) }],
              skipped: 0,
            }}
            kind="bars"
            title={t('insights.activeUsers')}
          />
          <div className="section-head">
            <h2>{t('insights.perDay')}</h2>
            <a className="button" href="/api/v1/admin/insights?format=csv">
              {t('insights.csv')}
            </a>
          </div>
          <SortTable caption={t('insights.perDay')} columns={columns} rows={days} rowKey={(r) => r.day} />
        </>
      )}
      <SearchTerms />
    </>
  );
}

function AnalyticsForm({ current }: { current: Settings }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [form, setForm] = useState(current);
  useEffect(() => setForm(current), [current]);
  const save = useMutation({
    mutationFn: (next: Settings) => api.put('/admin/analytics', next),
    onSettled: () => client.invalidateQueries({ queryKey: ['admin-analytics'] }),
  });
  const remove = useMutation({ mutationFn: () => api.del('/admin/analytics/data') });
  const losesNames = current.mode === 'names' && form.mode === 'counts';
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate(form);
      }}
    >
      <ErrorNote error={save.error ?? remove.error} />
      <label className="check">
        <input
          type="checkbox"
          checked={form.enabled}
          onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
        />
        {t('analytics.enabled')} <span className="muted">{t('experimental.label')}</span>
      </label>
      <p className="muted">{t('analytics.enabledHelp')}</p>
      <Field label={t('analytics.mode')} help={t(`analytics.mode_${form.mode}_help`)}>
        <select
          value={form.mode}
          onChange={(e) => setForm({ ...form, mode: e.target.value as Settings['mode'] })}
        >
          <option value="counts">{t('analytics.mode_counts')}</option>
          <option value="names">{t('analytics.mode_names')}</option>
        </select>
      </Field>
      <Field label={t('analytics.retention')}>
        <input
          type="number"
          min={30}
          max={3650}
          value={form.retention_days}
          onChange={(e) => setForm({ ...form, retention_days: Number(e.target.value) })}
        />
      </Field>
      <div className="row">
        {losesNames ? (
          <ConfirmButton
            label={t('common.save')}
            message={t('analytics.losesNames')}
            className="primary"
            tone="danger"
            onConfirm={() => save.mutate(form)}
          />
        ) : (
          <button type="submit" className="primary" disabled={save.isPending}>
            {t('common.save')}
          </button>
        )}
        <ConfirmButton
          label={t('analytics.delete')}
          message={t('analytics.deleteConfirm')}
          className="danger"
          onConfirm={() => remove.mutate()}
        />
      </div>
      {remove.isSuccess && (
        <p role="status" className="note">
          {t('analytics.deleted')}
        </p>
      )}
    </form>
  );
}

/** Administration, Analytics. */
export function AnalyticsTab() {
  const q = useQuery({ queryKey: ['admin-analytics'], queryFn: () => api.get<Settings>('/admin/analytics') });
  return (
    <>
      <ErrorNote error={q.error} />
      {q.data ? <AnalyticsForm current={q.data} /> : <Loading />}
    </>
  );
}

/** Administration, Outgoing calls: empty by default, so nothing leaves. */
export function OutgoingTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [host, setHost] = useState('');
  const q = useQuery({
    queryKey: ['outgoing-hosts'],
    queryFn: () => api.get<{ hosts: string[] }>('/admin/outgoing-hosts'),
  });
  const hosts = q.data?.hosts ?? [];
  const save = useMutation({
    mutationFn: (next: string[]) => api.put('/admin/outgoing-hosts', { hosts: next }),
    onSuccess: () => setHost(''),
    onSettled: () => client.invalidateQueries({ queryKey: ['outgoing-hosts'] }),
  });
  return (
    <>
      <p className="muted">{t('outgoing.help')}</p>
      <ErrorNote error={q.error ?? save.error} />
      {q.data && hosts.length === 0 && <p className="muted">{t('outgoing.empty')}</p>}
      <ul className="plain-list">
        {hosts.map((h) => (
          <li key={h} className="row">
            <span className="grow">{h}</span>
            <button
              type="button"
              className="ghost"
              aria-label={t('outgoing.remove', { host: h })}
              onClick={() => save.mutate(hosts.filter((x) => x !== h))}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate([...hosts, host]);
        }}
      >
        <Field label={t('outgoing.host')} help={t('outgoing.hostHelp')}>
          <input value={host} onChange={(e) => setHost(e.target.value)} required />
        </Field>
        <button type="submit" className="primary" disabled={save.isPending}>
          {t('outgoing.add')}
        </button>
      </form>
    </>
  );
}
