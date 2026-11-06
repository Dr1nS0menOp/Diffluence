// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Space, type SpaceOverview } from '../api';
import { Link } from '../router';
import { Frame, spaceByKeyQuery, useSpaceWatch } from '../components/frame';
import { IconEye, IconStar } from '../components/icons';
import { PageMark } from '../components/topbar';
import { ErrorNote, Loading, RelativeTime, SpaceBadge } from '../components/ui';

function WatchButton({ space }: { space: Space }) {
  const { t } = useTranslation();
  const watch = useSpaceWatch(space);
  const label = watch.watching ? t('watching.stopSpace') : t('watching.watchSpace');
  return (
    <button
      type="button"
      className="icon"
      aria-label={label}
      title={label}
      aria-pressed={watch.watching}
      onClick={() => watch.set(!watch.watching)}
    >
      <IconEye />
    </button>
  );
}

export function SpaceOverviewView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery(spaceByKeyQuery(spaceKey));
  const space = q.data;
  const overview = useQuery({
    queryKey: ['overview', space?.id],
    queryFn: () => api.get<SpaceOverview>(`/spaces/${space?.id}/overview`),
    enabled: !!space,
  });
  const star = useMutation({
    mutationFn: () =>
      space?.favourite ? api.del(`/spaces/${space.id}/favourite`) : api.put(`/spaces/${space?.id}/favourite`),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['space', spaceKey] });
      await client.invalidateQueries({ queryKey: ['space-id', space?.id] });
      await client.invalidateQueries({ queryKey: ['my-spaces'] });
    },
  });
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!space) return <Loading />;
  const label = space.favourite ? t('space.unfavourite') : t('space.favourite');
  const facts = overview.data;
  return (
    <Frame space={space}>
      <div className="section-head">
        <h1 className="overview-title">
          <SpaceBadge space={space} size="large" /> {space.name}
        </h1>
        <span className="row">
          <WatchButton space={space} />
          <button
            type="button"
            className="icon"
            aria-label={label}
            title={label}
            aria-pressed={space.favourite}
            onClick={() => star.mutate()}
          >
            <IconStar />
          </button>
        </span>
      </div>
      {space.description && <p>{space.description}</p>}
      <ErrorNote error={overview.error ?? star.error} />
      {overview.isPending && <Loading />}
      {facts && (
        <>
          <dl className="details-facts">
            <div>
              <dt>{t('directory.administrators')}</dt>
              <dd>{facts.administrators.join(', ') || t('common.none')}</dd>
            </div>
            {facts.owners.length > 0 && (
              <div>
                <dt>{t('spaceSettings.owners')}</dt>
                <dd>{facts.owners.map((o) => o.name).join(', ')}</dd>
              </div>
            )}
            <div>
              <dt>{t('space.pages')}</dt>
              <dd>{facts.page_count}</dd>
            </div>
          </dl>
          <section>
            <h2>{t('overview.recent')}</h2>
            {facts.recent.length === 0 && <p className="muted">{t('overview.noRecent')}</p>}
            <ul className="page-list">
              {facts.recent.map((p) => (
                <li key={p.id}>
                  <PageMark page={p} />
                  <Link href={`/pages/${p.id}`}>{p.title}</Link>
                  <span className="muted">
                    <RelativeTime value={p.at} />
                  </span>
                </li>
              ))}
            </ul>
          </section>
          {facts.posts.length > 0 && (
            <section>
              <h2>{t('blog.title')}</h2>
              <ul className="page-list">
                {facts.posts.map((p) => (
                  <li key={p.id}>
                    <Link href={`/pages/${p.id}`}>{p.title}</Link>
                    <span className="muted">
                      <RelativeTime value={p.published_at} />
                    </span>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </>
      )}
    </Frame>
  );
}
