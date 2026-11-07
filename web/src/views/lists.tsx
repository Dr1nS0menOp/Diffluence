// SPDX-License-Identifier: Apache-2.0
// The two personal lists: where a person has been, and what they starred.
import { useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ListedPage, type Space } from '../api';
import { Link } from '../router';
import { IconArrowDown, IconArrowUp, IconGrip } from '../components/icons';
import { PageMark } from '../components/topbar';
import { ConfirmButton, ErrorNote, Loading, RelativeTime, SpaceBadge } from '../components/ui';

const TABS = ['visited', 'worked', 'drafts'] as const;
type Tab = (typeof TABS)[number];
const MOVE_ICON_SIZE = 16;

const matches = (title: string, filter: string) => title.toLowerCase().includes(filter.trim().toLowerCase());

/** Moves an entry one step. A move past either end changes nothing. */
export function moved<T>(list: T[], at: number, step: number): T[] {
  const to = at + step;
  const item = list[at];
  if (item === undefined || to < 0 || to >= list.length) return list;
  const next = [...list];
  next.splice(at, 1);
  next.splice(to, 0, item);
  return next;
}

function DraftActions({ page }: { page: ListedPage }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const remove = useMutation({
    mutationFn: () => (page.unpublished ? api.del(`/pages/${page.id}`) : api.del(`/pages/${page.id}/draft`)),
    onSettled: () => client.invalidateQueries({ queryKey: ['recent-list'] }),
  });
  return (
    <>
      <ConfirmButton
        label={t('recent.deleteDraft')}
        message={page.unpublished ? t('recent.confirmDeletePage') : t('editor.confirmDiscard')}
        className="ghost"
        onConfirm={() => remove.mutate()}
      />
      <ErrorNote error={remove.error} />
    </>
  );
}

function RecentList({ tab, filter }: { tab: Tab; filter: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['recent-list', tab],
    queryFn: () => api.get<{ pages: ListedPage[] }>(`/me/recent?kind=${tab}`),
  });
  const pages = q.data?.pages.filter((p) => matches(p.title, filter)) ?? [];
  return (
    <>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && pages.length === 0 && <p className="muted">{t(`recent.empty_${tab}`)}</p>}
      <ul className="page-list">
        {pages.map((p) => (
          <li key={p.id}>
            <PageMark page={p} />
            <Link href={tab === 'drafts' ? `/pages/${p.id}/edit` : `/pages/${p.id}`}>
              {p.title || t('page.untitled')}
            </Link>
            <span className="muted">
              {p.space_name} · <RelativeTime value={p.at} />
            </span>
            {tab === 'drafts' && <DraftActions page={p} />}
          </li>
        ))}
      </ul>
    </>
  );
}

export function RecentView() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>('visited');
  const [filter, setFilter] = useState('');
  const list = useRef<HTMLDivElement>(null);
  const keys = (e: KeyboardEvent) => {
    const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const next = TABS[(TABS.indexOf(tab) + step + TABS.length) % TABS.length] ?? 'visited';
    setTab(next);
    list.current?.querySelector<HTMLElement>(`#tab-${next}`)?.focus();
  };
  return (
    <>
      <h1>{t('recent.title')}</h1>
      <div ref={list} role="tablist" aria-label={t('recent.title')} className="tabs" onKeyDown={keys}>
        {TABS.map((name) => (
          <button
            key={name}
            id={`tab-${name}`}
            type="button"
            role="tab"
            aria-selected={tab === name}
            aria-controls="recent-panel"
            tabIndex={tab === name ? 0 : -1}
            onClick={() => setTab(name)}
          >
            {t(`recent.tab_${name}`)}
          </button>
        ))}
      </div>
      <input
        type="search"
        className="list-filter"
        value={filter}
        aria-label={t('recent.filter')}
        placeholder={t('recent.filter')}
        onChange={(e) => setFilter(e.target.value)}
      />
      <div id="recent-panel" role="tabpanel" aria-labelledby={`tab-${tab}`}>
        <RecentList tab={tab} filter={filter} />
      </div>
    </>
  );
}

function FavouritePages({ filter }: { filter: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['/me/favourites'],
    queryFn: () => api.get<{ pages: ListedPage[] }>('/me/favourites'),
  });
  const [picked, setPicked] = useState<string[]>([]);
  const [dragged, setDragged] = useState<number | null>(null);
  const refresh = () => client.invalidateQueries({ queryKey: ['/me/favourites'] });
  const order = useMutation({
    mutationFn: (pages: ListedPage[]) =>
      api.put('/me/favourites/order', { page_ids: pages.map((p) => p.id) }),
    onMutate: (pages) => {
      client.setQueryData(['/me/favourites'], { pages });
    },
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: async (ids: string[]) => {
      await Promise.all(ids.map((id) => api.del(`/pages/${id}/favourite`)));
    },
    onSuccess: () => setPicked([]),
    onSettled: refresh,
  });
  const all = q.data?.pages ?? [];
  const filtering = filter.trim() !== '';
  const pages = all.filter((p) => matches(p.title, filter));
  const toggle = (id: string) =>
    setPicked(picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id]);
  return (
    <section>
      <h2>{t('favourites.pages')}</h2>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? order.error ?? remove.error} />
      {q.data && pages.length === 0 && <p className="muted">{t('favourites.noPages')}</p>}
      <ul className="page-list">
        {pages.map((p) => {
          const at = all.indexOf(p);
          return (
            <li
              key={p.id}
              draggable={!filtering}
              onDragStart={() => setDragged(at)}
              onDragOver={(e) => {
                if (dragged !== null) e.preventDefault();
              }}
              onDrop={(e) => {
                e.preventDefault();
                if (dragged !== null && dragged !== at) order.mutate(moved(all, dragged, at - dragged));
                setDragged(null);
              }}
            >
              <input
                type="checkbox"
                aria-label={t('favourites.pick', { title: p.title })}
                checked={picked.includes(p.id)}
                onChange={() => toggle(p.id)}
              />
              {!filtering && <IconGrip size={MOVE_ICON_SIZE} />}
              <PageMark page={p} />
              <Link href={`/pages/${p.id}`}>{p.title}</Link>
              <span className="muted">{p.space_name}</span>
              {!filtering && (
                <>
                  <button
                    type="button"
                    className="icon"
                    aria-label={t('favourites.moveUp', { title: p.title })}
                    disabled={at === 0}
                    onClick={() => order.mutate(moved(all, at, -1))}
                  >
                    <IconArrowUp size={MOVE_ICON_SIZE} />
                  </button>
                  <button
                    type="button"
                    className="icon"
                    aria-label={t('favourites.moveDown', { title: p.title })}
                    disabled={at === all.length - 1}
                    onClick={() => order.mutate(moved(all, at, 1))}
                  >
                    <IconArrowDown size={MOVE_ICON_SIZE} />
                  </button>
                </>
              )}
            </li>
          );
        })}
      </ul>
      {picked.length > 0 && (
        <button type="button" onClick={() => remove.mutate(picked)}>
          {t('favourites.removeSelected', { count: picked.length })}
        </button>
      )}
    </section>
  );
}

function FavouriteSpaces({ filter }: { filter: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['my-spaces'],
    queryFn: () => api.get<{ favourites: Space[]; recent: Space[] }>('/me/spaces'),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/spaces/${id}/favourite`),
    onSettled: () => client.invalidateQueries({ queryKey: ['my-spaces'] }),
  });
  const spaces = q.data?.favourites.filter((s) => matches(s.name, filter)) ?? [];
  return (
    <section>
      <h2>{t('favourites.spaces')}</h2>
      <ErrorNote error={q.error ?? remove.error} />
      {q.data && spaces.length === 0 && <p className="muted">{t('favourites.noSpaces')}</p>}
      <ul className="page-list">
        {spaces.map((s) => (
          <li key={s.id}>
            <SpaceBadge space={s} size="small" />
            <Link href={`/spaces/${s.key}`}>{s.name}</Link>
            <button type="button" className="ghost" onClick={() => remove.mutate(s.id)}>
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function FavouritesView() {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  return (
    <>
      <h1>{t('favourites.title')}</h1>
      <input
        type="search"
        className="list-filter"
        value={filter}
        aria-label={t('favourites.filter')}
        placeholder={t('favourites.filter')}
        onChange={(e) => setFilter(e.target.value)}
      />
      <FavouritePages filter={filter} />
      <FavouriteSpaces filter={filter} />
    </>
  );
}
