// SPDX-License-Identifier: Apache-2.0
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Space } from '../api';
import { Chart, type ChartData } from '../components/chart';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { SortTable, type SortColumn } from '../components/sort-table';
import { ErrorNote, Field, Loading } from '../components/ui';
import { Link } from '../router';

const DAY_MS = 86_400_000;
const PERIODS = ['7', '30', '90', 'year'] as const;
type Period = (typeof PERIODS)[number];

interface Day {
  day: string;
  views: number;
  readers: number;
  engaged: number;
}
interface LinkCount {
  id: string;
  title: string;
  count: number;
}
interface Reader {
  id: string;
  name: string;
  last_at: string;
  views: number;
  last_version: number;
}
interface PageInsights {
  enabled: boolean;
  mode: string;
  views: number;
  days: Day[];
  from: LinkCount[];
  to: LinkCount[];
  readers: Reader[];
  mentioned: { id: string; name: string; read: boolean }[];
}
interface ContentRow {
  id: string;
  title: string;
  updated_at: string;
  last_viewed: string | null;
  comments: number;
  readers: number;
  views: number;
}
interface SpaceInsights {
  enabled: boolean;
  mode: string;
  popular: LinkCount[];
  stale: LinkCount[];
}
interface PersonRow {
  id: string;
  name: string;
  pages: number;
  edits: number;
  comments: number;
  views: number;
}

function dateOnly(at: Date): string {
  return at.toISOString().slice(0, 10);
}

/** The query of a period. Empty dates mean the last 30 days to the server. */
export function periodQuery(period: Period, now: Date): string {
  const from =
    period === 'year'
      ? new Date(Date.UTC(now.getUTCFullYear(), 0, 1))
      : new Date(now.getTime() - (Number(period) - 1) * DAY_MS);
  return `from=${dateOnly(from)}&to=${dateOnly(now)}`;
}

function PeriodChoice({ value, onChange }: { value: Period; onChange: (next: Period) => void }) {
  const { t } = useTranslation();
  return (
    <Field label={t('insights.period')}>
      <select value={value} onChange={(e) => onChange(e.target.value as Period)}>
        {PERIODS.map((p) => (
          <option key={p} value={p}>
            {t(`insights.period_${p}`)}
          </option>
        ))}
      </select>
    </Field>
  );
}

function dayChart(days: Day[], name: string, value: (d: Day) => number): ChartData {
  return { labels: days.map((d) => d.day), series: [{ name, values: days.map(value) }], skipped: 0 };
}

/** The sentence beside a chart: the total, and whether the second half rose (WCAG 1.1.1). */
export function trendKey(values: number[]): 'up' | 'down' | 'flat' {
  const middle = Math.floor(values.length / 2);
  const sum = (list: number[]) => list.reduce((a, b) => a + b, 0);
  const first = sum(values.slice(0, middle));
  const second = sum(values.slice(values.length - middle));
  if (second > first) return 'up';
  return second < first ? 'down' : 'flat';
}

function LinkList({ title, rows }: { title: string; rows: LinkCount[] }) {
  const { t } = useTranslation();
  return (
    <section>
      <h2>{title}</h2>
      {rows.length === 0 && <p className="muted">{t('insights.nothingYet')}</p>}
      <ul className="page-list">
        {rows.map((r) => (
          <li key={r.id}>
            <Link href={`/pages/${r.id}`}>{r.title}</Link>
            {r.count > 0 && <span className="muted">{r.count}</span>}
          </li>
        ))}
      </ul>
    </section>
  );
}

function Readers({ data }: { data: PageInsights }) {
  const { t } = useTranslation();
  const columns = useMemo<SortColumn<Reader>[]>(
    () => [
      { key: 'name', label: t('insights.reader'), value: (r) => r.name },
      { key: 'views', label: t('insights.views'), value: (r) => r.views },
      { key: 'last', label: t('insights.lastRead'), value: (r) => r.last_at.slice(0, 10) },
      { key: 'version', label: t('insights.lastVersion'), value: (r) => r.last_version },
    ],
    [t],
  );
  return (
    <section>
      <h2>{t('insights.readers')}</h2>
      <SortTable caption={t('insights.readers')} columns={columns} rows={data.readers} rowKey={(r) => r.id} />
      {data.mentioned.length > 0 && (
        <>
          <h3>{t('insights.mentioned')}</h3>
          <ul className="plain-list">
            {data.mentioned.map((m) => (
              <li key={m.id}>
                {m.name}: {m.read ? t('insights.hasRead') : t('insights.hasNotRead')}
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  );
}

function PageReport({ data }: { data: PageInsights }) {
  const { t } = useTranslation();
  const views = data.days.map((d) => d.views);
  const engaged = data.days.reduce((sum, d) => sum + d.engaged, 0);
  return (
    <>
      <dl className="tiles">
        <div>
          <dt>{t('insights.views')}</dt>
          <dd>{data.views}</dd>
        </div>
        <div>
          <dt>{t('insights.engaged')}</dt>
          <dd>{engaged}</dd>
        </div>
        <div>
          <dt>{t('insights.brief')}</dt>
          <dd>{data.views - engaged}</dd>
        </div>
      </dl>
      <p className="muted">{t('insights.readersPerDay')}</p>
      {data.days.length > 0 && (
        <>
          <p>{t(`insights.trend_${trendKey(views)}`, { count: data.views })}</p>
          <Chart
            data={dayChart(data.days, t('insights.views'), (d) => d.views)}
            kind="bars"
            title={t('insights.viewsPerDay')}
          />
          <Chart
            data={dayChart(data.days, t('insights.readers'), (d) => d.readers)}
            kind="bars"
            title={t('insights.readersDay')}
          />
        </>
      )}
      <LinkList title={t('insights.cameFrom')} rows={data.from} />
      <LinkList title={t('insights.wentTo')} rows={data.to} />
      {data.mode === 'names' && <Readers data={data} />}
    </>
  );
}

/** What a page did: for those with View analytics. */
export function PageInsightsView({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const [period, setPeriod] = useState<Period>('30');
  const query = periodQuery(period, new Date());
  const q = useQuery({
    queryKey: ['page-insights', pageId, query],
    queryFn: () => api.get<PageInsights>(`/pages/${pageId}/insights?${query}`),
  });
  return (
    <div className="column">
      <p>
        <Link href={`/pages/${pageId}`}>{t('insights.backToPage')}</Link>
      </p>
      <h1>{t('insights.title')}</h1>
      <PeriodChoice value={period} onChange={setPeriod} />
      <ErrorNote error={q.error} />
      {q.isPending && <Loading />}
      {q.data && !q.data.enabled && <p className="note">{t('insights.off')}</p>}
      {q.data?.enabled && q.data.days.length === 0 && <p className="muted">{t('insights.noData')}</p>}
      {q.data?.enabled && <PageReport data={q.data} />}
    </div>
  );
}

function ContentTable({ space, query }: { space: Space; query: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['space-insight-content', space.id, query],
    queryFn: () => api.get<{ pages: ContentRow[] }>(`/spaces/${space.id}/insights/content?${query}`),
  });
  const columns = useMemo<SortColumn<ContentRow>[]>(
    () => [
      {
        key: 'title',
        label: t('insights.page'),
        value: (r) => r.title,
        show: (r) => <Link href={`/pages/${r.id}`}>{r.title}</Link>,
      },
      { key: 'views', label: t('insights.views'), value: (r) => r.views },
      { key: 'readers', label: t('insights.readerDays'), value: (r) => r.readers },
      { key: 'comments', label: t('insights.comments'), value: (r) => r.comments },
      { key: 'viewed', label: t('insights.lastViewed'), value: (r) => r.last_viewed ?? '' },
      { key: 'changed', label: t('insights.lastChanged'), value: (r) => r.updated_at.slice(0, 10) },
    ],
    [t],
  );
  return (
    <section>
      <div className="section-head">
        <h2>{t('insights.content')}</h2>
        <a className="button" href={`/api/v1/spaces/${space.id}/insights/content?${query}&format=csv`}>
          {t('insights.csv')}
        </a>
      </div>
      <ErrorNote error={q.error} />
      {q.data && (
        <SortTable
          caption={t('insights.content')}
          columns={columns}
          rows={q.data.pages}
          rowKey={(r) => r.id}
        />
      )}
    </section>
  );
}

function PeopleTable({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['space-insight-people', space.id],
    queryFn: () => api.get<{ people: PersonRow[] }>(`/spaces/${space.id}/insights/people`),
  });
  const columns = useMemo<SortColumn<PersonRow>[]>(
    () => [
      { key: 'name', label: t('insights.reader'), value: (r) => r.name },
      { key: 'views', label: t('insights.views'), value: (r) => r.views },
      { key: 'pages', label: t('insights.pages'), value: (r) => r.pages },
      { key: 'edits', label: t('insights.edits'), value: (r) => r.edits },
      { key: 'comments', label: t('insights.comments'), value: (r) => r.comments },
    ],
    [t],
  );
  return (
    <section>
      <h2>{t('insights.people')}</h2>
      <ErrorNote error={q.error} />
      {q.data && (
        <SortTable
          caption={t('insights.people')}
          columns={columns}
          rows={q.data.people}
          rowKey={(r) => r.id}
        />
      )}
    </section>
  );
}

function SpaceReport({ space }: { space: Space }) {
  const { t } = useTranslation();
  const [period, setPeriod] = useState<Period>('30');
  const query = periodQuery(period, new Date());
  const q = useQuery({
    queryKey: ['space-insights', space.id],
    queryFn: () => api.get<SpaceInsights>(`/spaces/${space.id}/insights`),
  });
  return (
    <>
      <h1>{t('insights.spaceTitle')}</h1>
      <ErrorNote error={q.error} />
      {q.data && !q.data.enabled && <p className="note">{t('insights.off')}</p>}
      <PeriodChoice value={period} onChange={setPeriod} />
      {q.data && <LinkList title={t('insights.popular')} rows={q.data.popular} />}
      {q.data && (
        <section>
          <LinkList title={t('insights.stale')} rows={q.data.stale} />
          <Link href={`/spaces/${space.key}/content`}>{t('insights.toContent')}</Link>
        </section>
      )}
      <ContentTable space={space} query={query} />
      {q.data?.mode === 'names' && <PeopleTable space={space} />}
    </>
  );
}

/** What a space did: for those with View analytics. */
export function SpaceInsightsView({ spaceKey }: { spaceKey: string }) {
  const q = useQuery(spaceByKeyQuery(spaceKey));
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!q.data) return <Loading />;
  return (
    <Frame space={q.data}>
      <SpaceReport space={q.data} />
    </Frame>
  );
}
