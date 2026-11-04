// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Me } from './api';
import { Link, match, navigate, useLocation } from './router';
import { ErrorNote, Loading } from './components/ui';
import { AdminView } from './views/admin';
import { HomeView } from './views/home';
import { LoginView } from './views/login';
import { EditView } from './views/edit';
import { CompareView, HistoryView, PageView } from './views/page';
import { ProfileView } from './views/profile';
import { SearchView } from './views/search';
import { SpaceHome, SpaceSettingsView, SpaceShell, TrashView } from './views/space';

function Shell({ me, children }: { me: Me; children: ReactNode }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [q, setQ] = useState('');
  const isAdmin = me.global_permissions.includes('administrator');
  const logout = async () => {
    await api.post('/auth/logout');
    client.clear();
    navigate('/login');
  };
  const search = (e: FormEvent) => {
    e.preventDefault();
    navigate('/search?q=' + encodeURIComponent(q));
  };
  return (
    <>
      <a className="skip" href="#main">
        {t('app.skip')}
      </a>
      <header className="top">
        <Link href="/" className="brand">
          {t('app.name')}
        </Link>
        <form role="search" onSubmit={search}>
          <input
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t('nav.searchPlaceholder')}
            aria-label={t('nav.search')}
          />
        </form>
        <nav aria-label={t('nav.mainMenu')} className="row">
          {isAdmin && <Link href="/admin/users">{t('nav.admin')}</Link>}
          <Link href="/me">{me.user.display_name}</Link>
          <button type="button" className="ghost" onClick={() => void logout()}>
            {t('nav.logout')}
          </button>
        </nav>
      </header>
      {children}
    </>
  );
}

function Plain({ children }: { children: ReactNode }) {
  return (
    <main id="main" className="plain">
      {children}
    </main>
  );
}

function Routes({ me }: { me: Me }) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  let m;
  if (pathname === '/') {
    return (
      <Plain>
        <HomeView me={me} />
      </Plain>
    );
  }
  if ((m = match('/search', pathname))) {
    return (
      <Plain>
        <SearchView />
      </Plain>
    );
  }
  if ((m = match('/me', pathname))) {
    return (
      <Plain>
        <ProfileView me={me} />
      </Plain>
    );
  }
  if ((m = match('/admin/:tab', pathname))) {
    return (
      <Plain>
        <AdminView tab={m.tab ?? 'users'} me={me} />
      </Plain>
    );
  }
  if ((m = match('/spaces/:key', pathname))) return <SpaceHome spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/settings', pathname))) return <SpaceSettingsView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/trash', pathname))) return <TrashView spaceKey={m.key ?? ''} />;
  if ((m = match('/pages/:id/edit', pathname))) {
    return (
      <SpaceShell pageId={m.id ?? ''}>
        <EditView pageId={m.id ?? ''} />
      </SpaceShell>
    );
  }
  if ((m = match('/pages/:id/history', pathname))) {
    return (
      <SpaceShell pageId={m.id ?? ''}>
        <HistoryView pageId={m.id ?? ''} />
      </SpaceShell>
    );
  }
  if ((m = match('/pages/:id/compare', pathname))) {
    return (
      <SpaceShell pageId={m.id ?? ''}>
        <CompareView pageId={m.id ?? ''} />
      </SpaceShell>
    );
  }
  if ((m = match('/pages/:id', pathname))) {
    return (
      <SpaceShell pageId={m.id ?? ''}>
        <PageView pageId={m.id ?? ''} me={me} />
      </SpaceShell>
    );
  }
  return (
    <Plain>
      <p role="alert">{t('app.notFound')}</p>
    </Plain>
  );
}

export function App() {
  const { pathname } = useLocation();
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });

  if (pathname === '/login') return <LoginView />;
  if (me.isPending) return <Loading />;
  if (me.error) {
    if (me.error instanceof ApiError && me.error.status === 401) {
      navigate('/login', true);
      return null;
    }
    return <ErrorNote error={me.error} />;
  }
  return (
    <Shell me={me.data}>
      <Routes me={me.data} />
    </Shell>
  );
}
