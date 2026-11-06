// SPDX-License-Identifier: Apache-2.0
// Screens that list things by what they are: labels, blog posts, the archive.
import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  type ArchivedPage,
  type BlogPost,
  type LabelCount,
  type ListedPage,
  type Page,
  type Space,
} from '../api';
import { Link, navigate, useLocation } from '../router';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { PageMark } from '../components/topbar';
import { ErrorNote, Loading, RelativeTime } from '../components/ui';
import { useDates } from '../dates';

const POSTS_PER_LOAD = 20;

function InSpace({ spaceKey, children }: { spaceKey: string; children: (space: Space) => ReactNode }) {
  const q = useQuery(spaceByKeyQuery(spaceKey));
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!q.data) return <Loading />;
  return <Frame space={q.data}>{children(q.data)}</Frame>;
}

function LabelList({ spaceId, base }: { spaceId?: string; base: string }) {
  const { t } = useTranslation();
  const [byName, setByName] = useState(false);
  const q = useQuery({
    queryKey: ['label-list', spaceId, byName],
    queryFn: () =>
      api.get<{ labels: LabelCount[] }>(
        `/labels?${spaceId ? `space_id=${spaceId}&` : ''}${byName ? 'sort=name' : ''}`,
      ),
  });
  return (
    <>
      <h1>{t('organise.labels')}</h1>
      <label className="check">
        <input type="checkbox" checked={byName} onChange={(e) => setByName(e.target.checked)} />
        {t('organise.byName')}
      </label>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data?.labels.length === 0 && <p className="muted">{t('organise.noLabels')}</p>}
      <ul className="label-list">
        {q.data?.labels.map((l) => (
          <li key={l.label}>
            <Link
              href={`${base}/${encodeURIComponent(l.label)}${spaceId ? `?space_id=${spaceId}` : ''}`}
              className="label"
            >
              {l.label} <span className="muted">{l.count}</span>
            </Link>
          </li>
        ))}
      </ul>
    </>
  );
}

export function LabelsView({ spaceKey }: { spaceKey?: string }) {
  if (!spaceKey) return <LabelList base="/labels" />;
  return <InSpace spaceKey={spaceKey}>{(space) => <LabelList spaceId={space.id} base="/labels" />}</InSpace>;
}

export function LabelView({ label }: { label: string }) {
  const { t } = useTranslation();
  const { search } = useLocation();
  const [more, setMore] = useState('');
  const q = useQuery({
    queryKey: ['label-pages', label, search],
    queryFn: () => api.get<{ pages: ListedPage[] }>(`/labels/${encodeURIComponent(label)}/pages${search}`),
  });
  return (
    <>
      <h1>{t('organise.labelTitle', { label: label.split('+').join(' + ') })}</h1>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          navigate(`/labels/${encodeURIComponent(`${label}+${more.trim()}`)}${search}`);
          setMore('');
        }}
      >
        <input
          required
          value={more}
          aria-label={t('organise.alsoLabel')}
          placeholder={t('organise.alsoLabel')}
          onChange={(e) => setMore(e.target.value)}
        />
        <button type="submit">{t('common.add')}</button>
      </form>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data?.pages.length === 0 && <p className="muted">{t('organise.noLabelPages')}</p>}
      <ul className="page-list">
        {q.data?.pages.map((p) => (
          <li key={p.id}>
            <PageMark page={p} />
            <Link href={`/pages/${p.id}`}>{p.title}</Link>
            <span className="muted">{p.space_name}</span>
          </li>
        ))}
      </ul>
    </>
  );
}

function monthOf(value: string): string {
  return new Date(value).toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
}

function PostList({ path }: { path: string }) {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useInfiniteQuery({
    queryKey: ['posts', path],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.get<{ posts: BlogPost[]; total: number }>(`${path}?limit=${POSTS_PER_LOAD}&offset=${pageParam}`),
    getNextPageParam: (last, all) => {
      const loaded = all.reduce((sum, p) => sum + p.posts.length, 0);
      return loaded < last.total ? loaded : undefined;
    },
  });
  const posts = q.data?.pages.flatMap((p) => p.posts) ?? [];
  const months = [...new Set(posts.map((p) => monthOf(p.published_at)))];
  return (
    <>
      <h1>{t('blog.title')}</h1>
      {path !== '/posts' && (
        <p>
          <Link href="/blog">{t('blog.all')}</Link>
        </p>
      )}
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && posts.length === 0 && <p className="muted">{t('blog.none')}</p>}
      {months.map((month) => (
        <section key={month}>
          <h2>{month}</h2>
          <ul className="post-list">
            {posts
              .filter((p) => monthOf(p.published_at) === month)
              .map((p) => (
                <li key={p.id}>
                  <Link href={`/pages/${p.id}`}>{p.title}</Link>
                  <span className="muted">
                    {t('page.byline', { name: p.author_name })} · {dates.day(p.published_at)} · {p.space_name}
                  </span>
                  {p.excerpt && <p>{p.excerpt}</p>}
                </li>
              ))}
          </ul>
        </section>
      ))}
      {q.hasNextPage && (
        <button type="button" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t('common.more')}
        </button>
      )}
    </>
  );
}

export function BlogView({ spaceKey }: { spaceKey?: string }) {
  if (!spaceKey) return <PostList path="/posts" />;
  return <InSpace spaceKey={spaceKey}>{(space) => <PostList path={`/spaces/${space.id}/posts`} />}</InSpace>;
}

function ArchiveList({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { searchParams } = useLocation();
  const [filter, setFilter] = useState(searchParams.get('q') ?? '');
  const key = ['archive', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ pages: ArchivedPage[] }>(`/spaces/${space.id}/archive`),
  });
  const restore = useMutation({
    mutationFn: (id: string) => api.post(`/pages/${id}/unarchive`, { with_parents: true }),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: key });
      await client.invalidateQueries({ queryKey: ['tree', space.id] });
    },
  });
  const wanted = filter.trim().toLowerCase();
  const pages = q.data?.pages.filter((p) => p.title.toLowerCase().includes(wanted)) ?? [];
  return (
    <>
      <h1>{t('organise.archive')}</h1>
      <input
        type="search"
        className="list-filter"
        value={filter}
        aria-label={t('recent.filter')}
        placeholder={t('recent.filter')}
        onChange={(e) => setFilter(e.target.value)}
      />
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? restore.error} />
      {q.data && pages.length === 0 && <p className="muted">{t('organise.archiveEmpty')}</p>}
      <ul className="page-list">
        {pages.map((p) => (
          <li key={p.id}>
            <Link href={`/pages/${p.id}`}>{p.title}</Link>
            <span className="muted">
              {p.archived_by_name} · <RelativeTime value={p.archived_at} />
            </span>
            <button type="button" className="ghost" onClick={() => restore.mutate(p.id)}>
              {t('organise.restore')}
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}

/** A link to a page that does not exist yet starts it, for those who may. */
export function MissingPage({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const { searchParams } = useLocation();
  const title = searchParams.get('title') ?? '';
  const space = useQuery(spaceByKeyQuery(spaceKey));
  const mayCreate = space.data?.can?.['page.create'] ?? false;
  const spaceId = space.data?.id;
  const create = useMutation({
    mutationFn: () => api.post<Page>('/pages', { space_id: spaceId, title }),
    onSuccess: (p) => navigate(`/pages/${p.id}/edit`, true),
  });
  const { mutate, isIdle } = create;
  useEffect(() => {
    if (mayCreate && title && isIdle) mutate();
  }, [mayCreate, title, isIdle, mutate]);
  if (space.isPending || (mayCreate && title && !create.error)) return <Loading />;
  return (
    <main id="main" className="plain reading">
      <ErrorNote error={space.error ?? create.error} />
      <p role="alert">{t('organise.pageMissing')}</p>
    </main>
  );
}

export function ArchiveView({ spaceKey }: { spaceKey: string }) {
  return <InSpace spaceKey={spaceKey}>{(space) => <ArchiveList space={space} />}</InSpace>;
}
