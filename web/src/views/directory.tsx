// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Category, type DirectorySpace, type Me } from '../api';
import { Link, navigate, useLocation } from '../router';
import { NewSpaceDialog } from '../components/create';
import { IconPlus, IconStar } from '../components/icons';
import { ErrorNote, Loading, RelativeTime, SpaceBadge } from '../components/ui';

const ROWS_PER_LOAD = 50;
const KINDS = ['all', 'team', 'personal', 'favourites', 'archived'];
const SORTS = ['name', 'changed', 'pages'];

interface DirectoryPage {
  spaces: DirectorySpace[];
  total: number;
}

function useFilters() {
  const { searchParams } = useLocation();
  const text = searchParams.get('q') ?? '';
  const kind = searchParams.get('kind') ?? 'all';
  const sort = searchParams.get('sort') ?? 'name';
  const category = searchParams.get('category') ?? '';
  const set = (change: { q?: string; kind?: string; sort?: string; category?: string }) => {
    const next = new URLSearchParams();
    const merged = { q: text, kind, sort, category, ...change };
    if (merged.q) next.set('q', merged.q);
    if (merged.kind !== 'all') next.set('kind', merged.kind);
    if (merged.sort !== 'name') next.set('sort', merged.sort);
    if (merged.category) next.set('category', merged.category);
    const query = next.toString();
    navigate(query ? `/spaces?${query}` : '/spaces', true);
  };
  return { text, kind, sort, category, set };
}

function Row({ space, onStar }: { space: DirectorySpace; onStar: (space: DirectorySpace) => void }) {
  const { t } = useTranslation();
  const label = space.favourite ? t('space.unfavourite') : t('space.favourite');
  return (
    <tr>
      <td>
        <button
          type="button"
          className="icon"
          aria-label={`${label}: ${space.name}`}
          title={label}
          aria-pressed={space.favourite}
          onClick={() => onStar(space)}
        >
          <IconStar size={18} />
        </button>
      </td>
      <td>
        <span className="directory-name">
          <SpaceBadge space={space} size="small" />
          <Link href={`/spaces/${space.key}`}>{space.name}</Link>
        </span>
        {space.description && <div className="muted">{space.description}</div>}
      </td>
      <td>{space.key}</td>
      <td>{space.administrators.join(', ')}</td>
      <td>{space.page_count}</td>
      <td>
        <RelativeTime value={space.last_changed} />
      </td>
    </tr>
  );
}

export function DirectoryView({ me }: { me: Me }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { text, kind, sort, category, set } = useFilters();
  const categories = useQuery({
    queryKey: ['categories'],
    queryFn: () => api.get<{ categories: Category[] }>('/categories'),
  });
  const [input, setInput] = useState(text);
  const [adding, setAdding] = useState(false);
  const key = ['directory', text, kind, sort, category];
  const q = useInfiniteQuery({
    queryKey: key,
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({
        q: text,
        kind,
        sort,
        limit: String(ROWS_PER_LOAD),
        offset: String(pageParam),
      });
      if (category) params.set('category', category);
      return api.get<DirectoryPage>(`/space-directory?${params.toString()}`);
    },
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((sum, p) => sum + p.spaces.length, 0);
      return loaded < last.total ? loaded : undefined;
    },
  });
  const star = useMutation({
    mutationFn: (space: DirectorySpace) =>
      space.favourite ? api.del(`/spaces/${space.id}/favourite`) : api.put(`/spaces/${space.id}/favourite`),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['directory'] });
      await client.invalidateQueries({ queryKey: ['my-spaces'] });
    },
  });
  const rows = q.data?.pages.flatMap((p) => p.spaces) ?? [];
  const total = q.data?.pages[0]?.total ?? 0;
  return (
    <>
      <div className="section-head">
        <h1>{t('directory.title')}</h1>
        {me.global_permissions.includes('create_space') && (
          <button type="button" onClick={() => setAdding(true)}>
            <IconPlus size={16} /> {t('home.newSpace')}
          </button>
        )}
      </div>
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
          aria-label={t('directory.filter')}
          placeholder={t('directory.filter')}
          onChange={(e) => {
            setInput(e.target.value);
            set({ q: e.target.value.trim() });
          }}
        />
        <select aria-label={t('directory.kind')} value={kind} onChange={(e) => set({ kind: e.target.value })}>
          {KINDS.map((k) => (
            <option key={k} value={k}>
              {t(`directory.kind_${k}`)}
            </option>
          ))}
        </select>
        {(categories.data?.categories.length ?? 0) > 0 && (
          <select
            aria-label={t('directory.category')}
            value={category}
            onChange={(e) => set({ category: e.target.value })}
          >
            <option value="">{t('directory.allCategories')}</option>
            {categories.data?.categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        )}
        <select aria-label={t('directory.sort')} value={sort} onChange={(e) => set({ sort: e.target.value })}>
          {SORTS.map((s) => (
            <option key={s} value={s}>
              {t(`directory.sort_${s}`)}
            </option>
          ))}
        </select>
      </form>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? star.error} />
      {q.data && rows.length === 0 && (
        <p className="muted">{kind === 'archived' ? t('directory.noneArchived') : t('directory.none')}</p>
      )}
      {rows.length > 0 && (
        <>
          <p role="status" className="muted">
            {t('directory.count', { count: total })}
          </p>
          <table>
            <thead>
              <tr>
                <th>
                  <span className="visually-hidden">{t('directory.star')}</span>
                </th>
                <th>{t('common.name')}</th>
                <th>{t('space.key')}</th>
                <th>{t('directory.administrators')}</th>
                <th>{t('space.pages')}</th>
                <th>{t('directory.lastChange')}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((space) => (
                <Row key={space.id} space={space} onStar={(s) => star.mutate(s)} />
              ))}
            </tbody>
          </table>
        </>
      )}
      {q.hasNextPage && (
        <button type="button" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t('common.more')}
        </button>
      )}
      {adding && <NewSpaceDialog onClose={() => setAdding(false)} />}
    </>
  );
}
