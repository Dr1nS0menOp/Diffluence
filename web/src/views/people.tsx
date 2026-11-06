// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api, type ListedPage, type MemberGroup, type Person, type Profile } from '../api';
import { localTimeIn, useDates } from '../dates';
import { Link, navigate, useLocation } from '../router';
import { PersonMark, PersonPicture } from '../components/person';
import { PageMark } from '../components/topbar';
import { ErrorNote, Loading } from '../components/ui';

const PEOPLE_PER_LOAD = 50;

function useFilters() {
  const { searchParams } = useLocation();
  const text = searchParams.get('q') ?? '';
  const group = searchParams.get('group') ?? '';
  const set = (change: { q?: string; group?: string }) => {
    const merged = { q: text, group, ...change };
    const next = new URLSearchParams();
    if (merged.q) next.set('q', merged.q);
    if (merged.group) next.set('group', merged.group);
    const query = next.toString();
    navigate(query ? `/people?${query}` : '/people', true);
  };
  return { text, group, set };
}

function PersonRow({ person }: { person: Person }) {
  return (
    <li className="person-row">
      <PersonPicture person={person} />
      <span className="person-row-text">
        <span>
          <Link href={`/people/${person.id}`}>{person.name}</Link> <PersonMark person={person} />
        </span>
        <span className="muted">{[person.job_title, person.department].filter(Boolean).join(' · ')}</span>
      </span>
    </li>
  );
}

export function PeopleView() {
  const { t } = useTranslation();
  const { text, group, set } = useFilters();
  const [input, setInput] = useState(text);
  const groups = useQuery({
    queryKey: ['my-groups'],
    queryFn: () => api.get<{ groups: MemberGroup[] }>('/me/groups'),
  });
  const q = useInfiniteQuery({
    queryKey: ['people', text, group],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({
        q: text,
        limit: String(PEOPLE_PER_LOAD),
        offset: String(pageParam),
      });
      if (group) params.set('group_id', group);
      return api.get<{ people: Person[]; total: number }>(`/people?${params.toString()}`);
    },
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((sum, p) => sum + p.people.length, 0);
      return loaded < last.total ? loaded : undefined;
    },
  });
  const rows = q.data?.pages.flatMap((p) => p.people) ?? [];
  return (
    <>
      <h1>{t('people.title')}</h1>
      <form
        className="row filters"
        onSubmit={(e) => {
          e.preventDefault();
          set({ q: input.trim() });
        }}
      >
        <input
          type="search"
          className="grow"
          value={input}
          aria-label={t('people.filter')}
          placeholder={t('people.filter')}
          onChange={(e) => {
            setInput(e.target.value);
            set({ q: e.target.value.trim() });
          }}
        />
        {(groups.data?.groups.length ?? 0) > 0 && (
          <select
            aria-label={t('people.group')}
            value={group}
            onChange={(e) => set({ group: e.target.value })}
          >
            <option value="">{t('people.allGroups')}</option>
            {groups.data?.groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        )}
      </form>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && rows.length === 0 && <p className="muted">{t('people.none')}</p>}
      {rows.length > 0 && (
        <>
          <p role="status" className="muted">
            {t('people.count', { count: q.data?.pages[0]?.total ?? 0 })}
          </p>
          <ul className="people">
            {rows.map((p) => (
              <PersonRow key={p.id} person={p} />
            ))}
          </ul>
        </>
      )}
      {q.hasNextPage && (
        <button type="button" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t('common.more')}
        </button>
      )}
    </>
  );
}

function Fact({ name, value }: { name: string; value: string }) {
  if (!value) return null;
  return (
    <>
      <dt>{name}</dt>
      <dd>{value}</dd>
    </>
  );
}

function PersonPages({ id }: { id: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['person-pages', id],
    queryFn: () => api.get<{ pages: ListedPage[] }>(`/people/${id}/pages`),
  });
  if ((q.data?.pages.length ?? 0) === 0) return <ErrorNote error={q.error} />;
  return (
    <section>
      <h2>{t('people.recentPages')}</h2>
      <ul className="page-list">
        {q.data?.pages.map((p) => (
          <li key={p.id}>
            <PageMark page={p} />
            <Link href={`/pages/${p.id}`}>{p.title}</Link>
            <span className="muted">{p.space_name}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function PersonView({ id }: { id: string }) {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['profile', id],
    queryFn: () => api.get<Profile>(`/people/${id}`),
  });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  const p = q.data;
  const isEmpty = !p.job_title && !p.department && !p.location && !p.about && !p.email;
  return (
    <>
      <div className="person-head">
        <PersonPicture person={p} size="large" />
        <div>
          <h1>{p.name}</h1>
          <PersonMark person={p} />
        </div>
        {p.is_self && (
          <Link href="/me" className="button">
            {t('common.edit')}
          </Link>
        )}
      </div>
      {isEmpty && <p className="muted">{t('people.empty', { name: p.name })}</p>}
      {p.about && <p>{p.about}</p>}
      <dl className="facts">
        <Fact name={t('people.jobTitle')} value={p.job_title} />
        <Fact name={t('people.department')} value={p.department} />
        <Fact name={t('people.location')} value={p.location} />
        <Fact name={t('people.localTime')} value={localTimeIn(p.time_zone, dates.language)} />
        <Fact name={t('common.email')} value={p.email} />
      </dl>
      <PersonPages id={id} />
    </>
  );
}
