// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Me } from './api';
import { useImportedFont } from './appearance';
import { forgetPersonalData, PersonProvider } from './personal';
import { Link, match, navigate, useLocation } from './router';
import { applyTheme } from './preferences';
import { NewPageDialog, NewSpaceDialog } from './components/create';
import { SpaceShell, useCreateTarget, useCurrentSpace } from './components/frame';
import { IconChevronDown, IconPlus, Logo } from './components/icons';
import { Menu, MenuItem } from './components/menu';
import { NotificationBell } from './components/notifications';
import { QuickSearch } from './components/quick-search';
import { ShortcutsProvider, useShortcut } from './components/shortcuts';
import { ToastProvider } from './components/toast';
import { AccountMenu, FavouritesMenu, HelpMenu, RecentMenu, SpacesMenu } from './components/topbar';
import { ErrorNote, Loading } from './components/ui';
import { AdminView } from './views/admin';
import { DirectoryView } from './views/directory';
import { HomeView } from './views/home';
import { FavouritesView, RecentView } from './views/lists';
import { LoginView } from './views/login';
import { NotificationSettingsView, NotificationsView, WatchingView } from './views/notifications';
import { EditView } from './views/edit';
import { CompareView, HistoryView } from './views/history';
import { SpaceOverviewView } from './views/overview';
import { PageView } from './views/page';
import { PreferencesView } from './views/preferences';
import { ProfileView } from './views/profile';
import { SearchView } from './views/search';
import { SpaceHome, SpaceSettingsView, TrashView } from './views/space';
import { TasksView } from './views/tasks';
import { UnsubscribeView } from './views/unsubscribe';

type Creating = 'page' | 'space' | null;

function CreateButton({ me, onCreate }: { me: Me; onCreate: (what: Creating) => void }) {
  const { t } = useTranslation();
  const target = useCreateTarget();
  const canCreateSpace = me.global_permissions.includes('create_space');
  const main = target ? 'page' : 'space';
  useShortcut('create', () => {
    if (target || canCreateSpace) onCreate(main);
  });
  if (!target && !canCreateSpace) return null;
  return (
    <span className="top-create">
      <button type="button" className="primary" onClick={() => onCreate(main)}>
        <IconPlus size={16} /> {t('nav.create')}
      </button>
      {target && canCreateSpace && (
        <Menu
          label={t('nav.createMenu')}
          align="start"
          className="primary top-create-more"
          trigger={<IconChevronDown size={16} />}
        >
          <MenuItem onClick={() => onCreate('page')}>{t('space.newPage')}</MenuItem>
          <MenuItem onClick={() => onCreate('space')}>{t('home.newSpace')}</MenuItem>
        </Menu>
      )}
    </span>
  );
}

function Shell({ me, children }: { me: Me; children: ReactNode }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { pathname } = useLocation();
  const target = useCreateTarget();
  const { space } = useCurrentSpace();
  const [creating, setCreating] = useState<Creating>(null);
  const logout = async () => {
    await api.post('/auth/logout');
    forgetPersonalData();
    client.clear();
    navigate('/login');
  };
  return (
    <>
      <a className="skip" href="#main">
        {t('app.skip')}
      </a>
      <header className="top">
        <Link href="/" className="brand">
          <Logo /> {t('app.name')}
        </Link>
        <nav aria-label={t('nav.mainMenu')} className="top-nav">
          <Link href="/" className="nav-link" aria-current={pathname === '/' ? 'page' : undefined}>
            {t('nav.home')}
          </Link>
          <SpacesMenu />
          <RecentMenu />
          <FavouritesMenu />
          <CreateButton me={me} onCreate={setCreating} />
        </nav>
        <QuickSearch space={space} />
        <NotificationBell />
        <HelpMenu />
        <AccountMenu me={me} onLogout={() => void logout()} />
      </header>
      {children}
      {creating === 'page' && target && (
        <NewPageDialog space={target.space} parent={target.parent} onClose={() => setCreating(null)} />
      )}
      {creating === 'space' && <NewSpaceDialog onClose={() => setCreating(null)} />}
    </>
  );
}

function Plain({ width, children }: { width?: 'reading'; children: ReactNode }) {
  return (
    <main id="main" className={`plain ${width ?? ''}`}>
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
      <Plain width="reading">
        <SearchView />
      </Plain>
    );
  }
  if ((m = match('/me', pathname))) {
    return (
      <Plain width="reading">
        <ProfileView me={me} />
      </Plain>
    );
  }
  if ((m = match('/me/preferences', pathname))) {
    return (
      <Plain width="reading">
        <PreferencesView />
      </Plain>
    );
  }
  if ((m = match('/notifications', pathname))) {
    return (
      <Plain width="reading">
        <NotificationsView />
      </Plain>
    );
  }
  if ((m = match('/me/notifications', pathname))) {
    return (
      <Plain width="reading">
        <NotificationSettingsView />
      </Plain>
    );
  }
  if ((m = match('/me/watching', pathname))) {
    return (
      <Plain width="reading">
        <WatchingView />
      </Plain>
    );
  }
  if ((m = match('/tasks', pathname))) {
    return (
      <Plain width="reading">
        <TasksView />
      </Plain>
    );
  }
  if ((m = match('/recent', pathname))) {
    return (
      <Plain width="reading">
        <RecentView />
      </Plain>
    );
  }
  if ((m = match('/favourites', pathname))) {
    return (
      <Plain width="reading">
        <FavouritesView />
      </Plain>
    );
  }
  if ((m = match('/spaces', pathname))) {
    return (
      <Plain>
        <DirectoryView me={me} />
      </Plain>
    );
  }
  if ((m = match('/admin/:tab', pathname))) return <AdminView tab={m.tab ?? 'users'} me={me} />;
  if ((m = match('/spaces/:key', pathname))) return <SpaceHome spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/overview', pathname))) return <SpaceOverviewView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/settings', pathname))) return <SpaceSettingsView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/trash', pathname))) return <TrashView spaceKey={m.key ?? ''} />;
  if ((m = match('/pages/:id/edit', pathname))) {
    return (
      <main id="main" className="focus">
        <EditView pageId={m.id ?? ''} />
      </main>
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
        <PageView pageId={m.id ?? ''} />
      </SpaceShell>
    );
  }
  return (
    <Plain width="reading">
      <p role="alert">{t('app.notFound')}</p>
    </Plain>
  );
}

export function App() {
  const { pathname } = useLocation();
  const me = useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') });
  useImportedFont();
  const theme = me.data?.preferences.theme;
  useEffect(() => {
    if (theme) applyTheme(theme);
  }, [theme]);

  if (pathname === '/login') return <LoginView />;
  // Mail links reach people who are not signed in, so this comes before the login check.
  const unsubscribe = match('/unsubscribe/:token', pathname);
  if (unsubscribe) return <UnsubscribeView token={unsubscribe.token ?? ''} />;
  if (me.isPending) return <Loading />;
  if (me.error) {
    if (me.error instanceof ApiError && me.error.status === 401) {
      navigate('/login', true);
      return null;
    }
    return <ErrorNote error={me.error} />;
  }
  return (
    <ToastProvider>
      <ShortcutsProvider enabled={me.data.preferences.shortcuts}>
        <PersonProvider value={me.data.user.id}>
          <Shell me={me.data}>
            <Routes me={me.data} />
          </Shell>
        </PersonProvider>
      </ShortcutsProvider>
    </ToastProvider>
  );
}
