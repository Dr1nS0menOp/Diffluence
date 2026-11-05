// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type ListedPage, type Me, type Space } from '../api';
import { Link } from '../router';
import { NewSpaceDialog } from '../components/create';
import { IconPlus } from '../components/icons';
import { PageMark } from '../components/topbar';
import { ErrorNote, Loading, SpaceBadge } from '../components/ui';

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

export function HomeView({ me }: { me: Me }) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  return (
    <>
      <h1 className="greeting">{t('home.welcome', { name: me.user.display_name })}</h1>
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
      <div className="columns">
        <PageList title={t('home.recent')} path="/me/recent" />
        <PageList title={t('home.favourites')} path="/me/favourites" />
      </div>
      {adding && <NewSpaceDialog onClose={() => setAdding(false)} />}
    </>
  );
}
