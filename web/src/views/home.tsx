// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import {
  api,
  NIL_ID,
  type FeedItem,
  type ListedPage,
  type Me,
  type PageDetail,
  type Space,
  type TaskItem,
} from '../api';
import { Link, useLocation } from '../router';
import { NewSpaceDialog } from '../components/create';
import { FirstUseCard, WelcomePage } from '../components/first-use';
import { DocumentView } from '../components/editor/editor';
import { IconPlus } from '../components/icons';
import { PageMark } from '../components/topbar';
import { ErrorNote, Loading, RelativeTime, SpaceBadge, Tabs } from '../components/ui';

const TABS = ['for_you', 'recent', 'popular'] as const;
type Tab = (typeof TABS)[number];
const FEED_PAGE = 20;
const TASKS_SHOWN = 5;

function PageList({ title, path }: { title: string; path: string }) {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: [path], queryFn: () => api.get<{ pages: ListedPage[] }>(path) });
  return (
    <section>
      <h2>{title}</h2>
      {q.isPending && <Loading />}
      {q.data?.pages.length === 0 && <p className="muted">{t('home.nothingYet')}</p>}
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

function CompanyHome({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ['page', pageId], queryFn: () => api.get<PageDetail>(`/pages/${pageId}`) });
  if (q.isPending) return <Loading />;
  if (q.error) return <ErrorNote error={q.error} />;
  return (
    <>
      <p>
        <Link href="/?personal=1">{t('home.yourStart')}</Link>
      </p>
      <h1>{q.data.title}</h1>
      <DocumentView document={q.data.document} />
    </>
  );
}

/** The company home for those who may see it, unless they prefer their own screen. */
export function StartScreen({ me }: { me: Me }) {
  const { searchParams } = useLocation();
  const personal = searchParams.get('personal') === '1' || me.preferences.start_screen === 'personal';
  const start = useQuery({
    queryKey: ['start'],
    queryFn: () => api.get<{ page_id: string | null }>('/start'),
    enabled: !personal,
  });
  const homeId = start.data?.page_id;
  if (!personal && start.isPending) return <Loading />;
  if (!personal && homeId && homeId !== NIL_ID) return <CompanyHome pageId={homeId} />;
  return <HomeView me={me} />;
}

function OpenTasks() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['tasks', 'assigned', 'open'],
    queryFn: () => api.get<{ tasks: TaskItem[] }>('/me/tasks?tab=assigned&status=open'),
  });
  const list = q.data?.tasks.slice(0, TASKS_SHOWN) ?? [];
  return (
    <section>
      <h2>{t('tasks.title')}</h2>
      {q.data && list.length === 0 && <p className="muted">{t('tasks.none')}</p>}
      <ul className="page-list">
        {list.map((task) => (
          <li key={task.id}>
            <Link href={`/pages/${task.page_id}`}>{task.text || t('tasks.untitled')}</Link>
          </li>
        ))}
      </ul>
      {list.length > 0 && <Link href="/tasks">{t('tasks.all')}</Link>}
    </section>
  );
}

function FeedEntry({ item, at, total }: { item: FeedItem; at: number; total: number }) {
  const { t } = useTranslation();
  return (
    <article
      className="feed-item"
      tabIndex={0}
      aria-posinset={at}
      aria-setsize={total}
      aria-label={item.title}
    >
      <h3>
        <PageMark page={item} /> <Link href={`/pages/${item.page_id}`}>{item.title}</Link>
      </h3>
      <p className="muted">
        {item.space_name} · {t(`home.kind_${item.kind}`, { name: item.author_name || t('common.unknown') })} ·{' '}
        <RelativeTime value={item.at} />
      </p>
      {item.excerpt && <p>{item.excerpt}</p>}
      {(item.reactions > 0 || item.comments > 0) && (
        <p className="muted">{t('home.responses', { reactions: item.reactions, comments: item.comments })}</p>
      )}
    </article>
  );
}

function Feed({ tab, spaceId }: { tab: Tab; spaceId: string }) {
  const { t } = useTranslation();
  const end = useRef<HTMLDivElement>(null);
  const q = useInfiniteQuery({
    queryKey: ['feed', tab, spaceId],
    initialPageParam: '',
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ tab, limit: String(FEED_PAGE) });
      if (pageParam) params.set('before', pageParam);
      if (spaceId) params.set('space_id', spaceId);
      return api.get<{ items: FeedItem[] }>(`/feed?${params.toString()}`);
    },
    getNextPageParam: (last) =>
      tab !== 'popular' && last.items.length === FEED_PAGE
        ? last.items[last.items.length - 1]?.at
        : undefined,
  });
  const items = q.data?.pages.flatMap((p) => p.items) ?? [];
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = q;

  useEffect(() => {
    const target = end.current;
    if (!target || !hasNextPage) return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting) && !isFetchingNextPage) void fetchNextPage();
    });
    observer.observe(target);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  // The feed pattern: Page Down and Page Up move between articles.
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    const step = e.key === 'PageDown' ? 1 : e.key === 'PageUp' ? -1 : 0;
    if (!step) return;
    const list = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('article'));
    const at = list.indexOf((e.target as HTMLElement).closest('article') as HTMLElement);
    const next = list[at + step];
    if (!next) return;
    e.preventDefault();
    next.focus();
  };

  return (
    <div id="feed-panel" role="tabpanel" aria-label={t(`home.tab_${tab}`)}>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && items.length === 0 && <p className="muted">{t(`home.empty_${tab}`)}</p>}
      {items.length > 0 && (
        <div role="feed" aria-busy={q.isFetching} aria-label={t(`home.tab_${tab}`)} onKeyDown={keys}>
          {items.map((item, i) => (
            <FeedEntry
              key={`${item.page_id}-${item.at}`}
              item={item}
              at={i + 1}
              total={hasNextPage ? -1 : items.length}
            />
          ))}
        </div>
      )}
      <div ref={end} />
      {hasNextPage && (
        <button type="button" disabled={isFetchingNextPage} onClick={() => void fetchNextPage()}>
          {t('common.more')}
        </button>
      )}
    </div>
  );
}

export function HomeView({ me }: { me: Me }) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  const [tab, setTab] = useState<Tab>('for_you');
  const [spaceId, setSpaceId] = useState('');
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });

  return (
    <>
      <h1 className="greeting">{t('home.welcome', { name: me.user.display_name })}</h1>
      <WelcomePage />
      <FirstUseCard />
      <div className="section-head">
        <h2>{t('home.spaces')}</h2>
        {me.global_permissions.includes('create_space') && (
          <button type="button" onClick={() => setAdding(true)}>
            <IconPlus size={16} /> {t('home.newSpace')}
          </button>
        )}
      </div>
      {spaces.isPending && <Loading />}
      <ErrorNote error={spaces.error} />
      {spaces.data?.spaces.length === 0 && <p className="muted">{t('home.noSpaces')}</p>}
      <ul className="cards">
        {spaces.data?.spaces.map((s) => (
          <li key={s.id}>
            <Link href={`/spaces/${s.key}`} className="space-card">
              <SpaceBadge space={s} size="large" />
              <span>
                <strong>{s.name}</strong>
                {s.description && <p>{s.description}</p>}
              </span>
            </Link>
          </li>
        ))}
      </ul>
      <div className="home-columns">
        <div>
          <Tabs
            names={TABS}
            current={tab}
            label={t('home.activity')}
            nameOf={(name) => t(`home.tab_${name}`)}
            panel="feed-panel"
            onChange={setTab}
          />
          {tab === 'recent' && (spaces.data?.spaces.length ?? 0) > 1 && (
            <select
              className="tabs-filter"
              aria-label={t('home.filterSpace')}
              value={spaceId}
              onChange={(e) => setSpaceId(e.target.value)}
            >
              <option value="">{t('search.anySpace')}</option>
              {spaces.data?.spaces.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          )}
          <Feed tab={tab} spaceId={tab === 'recent' ? spaceId : ''} />
        </div>
        <aside className="home-side" aria-label={t('home.yourThings')}>
          <PageList title={t('home.recent')} path="/me/recent" />
          <PageList title={t('home.favourites')} path="/me/favourites" />
          <OpenTasks />
        </aside>
      </div>
      {adding && <NewSpaceDialog onClose={() => setAdding(false)} />}
    </>
  );
}
