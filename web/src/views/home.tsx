// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState, type ComponentType, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api, NIL_ID, type FeedItem, type ListedPage, type Me, type PageDetail, type Space } from '../api';
import { Link, useLocation } from '../router';
import { NewSpaceDialog } from '../components/create';
import { FirstUseCard, WelcomePage } from '../components/first-use';
import { DocumentView } from '../components/editor/editor';
import { IconClock, IconComment, IconEye, IconPlus } from '../components/icons';
import { lastTouched } from '../components/sidebar-entries';
import { PageMark } from '../components/topbar';
import { pageTitle } from '../page-name';
import { Avatar, ErrorNote, Loading, RelativeTime } from '../components/ui';

const TABS = ['for_you', 'recent', 'popular'] as const;
type Tab = (typeof TABS)[number];
const TAB_ICONS: Record<Tab, ComponentType<{ size?: number }>> = {
  for_you: IconEye,
  recent: IconClock,
  popular: IconComment,
};
const FEED_PAGE = 20;
const CARDS_SHOWN = 6;
const RESPONSE_ICON_SIZE = 16;

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
      <h1>{pageTitle(t, q.data.title)}</h1>
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

/** What the person last worked on or visited, as cards, three to a row (NAV-104, NAV-105). */
function PickUpCards() {
  const { t } = useTranslation();
  const visited = useQuery({
    queryKey: ['recent-list', 'visited'],
    queryFn: () => api.get<{ pages: ListedPage[] }>('/me/recent?kind=visited'),
  });
  const worked = useQuery({
    queryKey: ['recent-list', 'worked'],
    queryFn: () => api.get<{ pages: ListedPage[] }>('/me/recent?kind=worked'),
  });
  const cards = lastTouched(visited.data?.pages, worked.data?.pages, CARDS_SHOWN);
  return (
    <section aria-labelledby="pick-up-heading" className="start-part">
      <h2 id="pick-up-heading">{t('home.pickUp')}</h2>
      {(visited.isPending || worked.isPending) && <Loading />}
      <ErrorNote error={visited.error ?? worked.error} />
      {visited.data && worked.data && cards.length === 0 && (
        <p className="muted">{t('home.nothingTouched')}</p>
      )}
      <ul className="touch-cards">
        {cards.map(({ page, did }) => (
          <li key={page.id}>
            <Link
              href={page.unpublished ? `/pages/${page.id}/edit` : `/pages/${page.id}`}
              className="touch-card"
            >
              <span className="touch-kind">
                <PageMark page={page} />
              </span>
              <span className="touch-title">
                <strong>{pageTitle(t, page.title)}</strong>
                {page.unpublished && <span className="lozenge neutral">{t('home.draftMark')}</span>}
              </span>
              <span className="muted">{page.space_name}</span>
              <span className="touch-did muted">
                {t(`home.did_${did}`)} <RelativeTime value={page.at} />
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}

/** One page in the feed: who did what and when, the page, its first lines and its responses (NAV-107). */
function FeedEntry({ item, at, total }: { item: FeedItem; at: number; total: number }) {
  const { t } = useTranslation();
  const author = item.author_name || t('common.unknown');
  return (
    <article
      className="feed-card"
      tabIndex={0}
      aria-posinset={at}
      aria-setsize={total}
      aria-label={pageTitle(t, item.title)}
    >
      <div className="feed-card-head">
        <Avatar name={author} size="small" />
        <strong>{author}</strong>
        <span className="muted">
          {t(`home.event_${item.kind}`, { count: item.pages ?? 0 })} · <RelativeTime value={item.at} />
        </span>
      </div>
      <div className="feed-card-body">
        <h3 className="feed-card-title">
          <PageMark page={item} /> <Link href={`/pages/${item.page_id}`}>{pageTitle(t, item.title)}</Link>
        </h3>
        <p className="muted">{item.space_name}</p>
        {item.excerpt && <p className="feed-card-excerpt">{item.excerpt}</p>}
        <div className="feed-card-foot">
          {item.reactions > 0 && <span>{t('home.reactionCount', { count: item.reactions })}</span>}
          <span className="feed-card-comments">
            <IconComment size={RESPONSE_ICON_SIZE} />
            <span aria-hidden="true">{item.comments}</span>
            <span className="visually-hidden">{t('home.commentCount', { count: item.comments })}</span>
          </span>
        </div>
      </div>
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
    <div id="feed-panel">
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

/** Without any space there is nothing to read yet: one sentence and the way to make the first (NAV-037). */
function FirstSpace({ me }: { me: Me }) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  return (
    <section className="start-part">
      <p>{t('home.noSpaces')}</p>
      {me.global_permissions.includes('create_space') && (
        <button type="button" className="primary" onClick={() => setAdding(true)}>
          <IconPlus size={RESPONSE_ICON_SIZE} /> {t('home.newSpace')}
        </button>
      )}
      {adding && <NewSpaceDialog onClose={() => setAdding(false)} />}
    </section>
  );
}

/**
 * The start screen: one centred column with getting started, the cards of what the person last
 * touched, the welcome message, and the feed with its choices as pills (spec 078, NAV-102 to NAV-106).
 */
export function HomeView({ me }: { me: Me }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState<Tab>('for_you');
  const [spaceId, setSpaceId] = useState('');
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });

  return (
    <>
      <h1 className="visually-hidden">{t('nav.home')}</h1>
      <FirstUseCard />
      <ErrorNote error={spaces.error} />
      {spaces.data?.spaces.length === 0 && <FirstSpace me={me} />}
      <PickUpCards />
      <WelcomePage />
      <section aria-labelledby="activity-heading" className="start-part">
        <h2 id="activity-heading">{t('home.activity')}</h2>
        <div role="group" aria-label={t('home.feedChoice')} className="pills">
          {TABS.map((name) => {
            const Icon = TAB_ICONS[name];
            return (
              <button
                key={name}
                type="button"
                className="pill"
                aria-pressed={tab === name}
                onClick={() => setTab(name)}
              >
                <Icon size={RESPONSE_ICON_SIZE} /> {t(`home.tab_${name}`)}
              </button>
            );
          })}
        </div>
        {tab === 'recent' && (spaces.data?.spaces.length ?? 0) > 1 && (
          <select
            className="feed-filter"
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
      </section>
    </>
  );
}
