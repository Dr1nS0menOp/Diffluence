// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Me } from './api';
import { appearanceQuery, useAccent, useImportedFont } from './appearance';
import { forgetPersonalData, PersonProvider } from './personal';
import { Link, match, navigate, useLocation } from './router';
import { applyTheme } from './preferences';
import { NewPageDialog, NewPostDialog, NewSpaceDialog } from './components/create';
import { SpaceShell, useCreateTarget, useCurrentSpace } from './components/frame';
import { IconChevronDown, IconPlus, Logo } from './components/icons';
import { Menu, MenuItem } from './components/menu';
import { TemplateGallery } from './components/template-gallery';
import { NotificationBell } from './components/notifications';
import { QuickSearch } from './components/quick-search';
import { IdleWarning, ReauthProvider } from './components/session';
import { ShortcutsProvider, useShortcut } from './components/shortcuts';
import { SiteNotes } from './components/site-notes';
import { ToastProvider } from './components/toast';
import { AccountMenu, FavouritesMenu, HelpMenu, RecentMenu, SpacesMenu } from './components/topbar';
import { ErrorNote, Loading } from './components/ui';
import { PersonalDataView, ProfileView, TokensView } from './views/account';
import { ForgotView, InvitationView, ResetView, SignupView } from './views/account-links';
import { AdminView } from './views/admin';
import { ContentView } from './views/content';
import { DirectoryView } from './views/directory';
import { GroupView } from './views/group';
import { StartScreen } from './views/home';
import { FavouritesView, RecentView } from './views/lists';
import { LoginView } from './views/login';
import { ArchiveView, BlogView, LabelsView, LabelView, MissingPage } from './views/organise';
import { NotificationSettingsView, NotificationsView, WatchingView } from './views/notifications';
import { EditView } from './views/edit';
import { ExportView } from './views/exchange';
import { CompareView, HistoryView } from './views/history';
import { FilesView } from './views/files';
import { SpaceOverviewView } from './views/overview';
import { PageView } from './views/page';
import { PeopleView, PersonView } from './views/people';
import { PreferencesView } from './views/preferences';
import { SecurityView } from './views/profile';
import { SearchView } from './views/search';
import { SitesView } from './views/sites';
import { SpaceHome, SpaceSettingsView, TrashView } from './views/space';
import { SpaceAccessView, SpaceAuditView, SpacePeopleView } from './views/space-access';
import { TasksView } from './views/tasks';
import { TemplatesView, TemplateView } from './views/templates';
import { UnsubscribeView } from './views/unsubscribe';

type Creating = 'page' | 'template' | 'post' | 'space' | null;

/** These need no session: they are how a person gets one. */
const PUBLIC_SCREENS: Record<string, () => ReactNode> = {
  '/login': () => <LoginView />,
  '/forgot': () => <ForgotView />,
  '/reset': () => <ResetView />,
  '/invitation': () => <InvitationView />,
  '/signup': () => <SignupView />,
};

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
      {target && (
        <Menu
          label={t('nav.createMenu')}
          align="start"
          className="primary top-create-more"
          trigger={<IconChevronDown size={16} />}
        >
          <MenuItem onClick={() => onCreate('page')}>{t('space.newPage')}</MenuItem>
          <MenuItem onClick={() => onCreate('template')}>{t('organise.fromTemplate')}</MenuItem>
          {(target.space.features?.blog ?? true) && (
            <MenuItem onClick={() => onCreate('post')}>{t('blog.newPost')}</MenuItem>
          )}
          {canCreateSpace && <MenuItem onClick={() => onCreate('space')}>{t('home.newSpace')}</MenuItem>}
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
  const appearance = useQuery(appearanceQuery).data;
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
          {appearance?.logo ? <img src={appearance.logo} alt="" className="brand-logo" /> : <Logo />}{' '}
          {appearance?.name || t('app.name')}
        </Link>
        <nav aria-label={t('nav.mainMenu')} className="top-nav">
          <Link href="/" className="nav-link" aria-current={pathname === '/' ? 'page' : undefined}>
            {t('nav.home')}
          </Link>
          <SpacesMenu />
          <Link
            href="/people"
            className="nav-link"
            aria-current={pathname === '/people' ? 'page' : undefined}
          >
            {t('nav.people')}
          </Link>
          <RecentMenu />
          <FavouritesMenu />
          <CreateButton me={me} onCreate={setCreating} />
        </nav>
        <QuickSearch space={space} />
        <NotificationBell />
        <HelpMenu />
        <AccountMenu me={me} onLogout={() => void logout()} />
      </header>
      <SiteNotes me={me} />
      {children}
      <IdleWarning minutes={me.idle_minutes} />
      {creating === 'page' && target && (
        <NewPageDialog space={target.space} parent={target.parent} onClose={() => setCreating(null)} />
      )}
      {creating === 'template' && target && (
        <TemplateGallery space={target.space} parent={target.parent} onClose={() => setCreating(null)} />
      )}
      {creating === 'post' && target && (
        <NewPostDialog space={target.space} onClose={() => setCreating(null)} />
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
        <StartScreen me={me} />
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
  if ((m = match('/me/security', pathname))) {
    return (
      <Plain width="reading">
        <SecurityView me={me} />
      </Plain>
    );
  }
  if ((m = match('/me/tokens', pathname))) {
    return (
      <Plain width="reading">
        <TokensView />
      </Plain>
    );
  }
  if ((m = match('/me/data', pathname))) {
    return (
      <Plain width="reading">
        <PersonalDataView />
      </Plain>
    );
  }
  if ((m = match('/people', pathname))) {
    return (
      <Plain width="reading">
        <PeopleView />
      </Plain>
    );
  }
  if ((m = match('/people/:id', pathname))) {
    return (
      <Plain width="reading">
        <PersonView id={m.id ?? ''} />
      </Plain>
    );
  }
  if ((m = match('/groups/:id', pathname))) {
    return (
      <Plain width="reading">
        <GroupView id={m.id ?? ''} />
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
  if ((m = match('/labels', pathname))) {
    return (
      <Plain width="reading">
        <LabelsView />
      </Plain>
    );
  }
  if ((m = match('/labels/:label', pathname))) {
    return (
      <Plain width="reading">
        <LabelView label={m.label ?? ''} />
      </Plain>
    );
  }
  if ((m = match('/blog', pathname))) {
    return (
      <Plain width="reading">
        <BlogView />
      </Plain>
    );
  }
  if ((m = match('/templates/:id/edit', pathname))) {
    return (
      <main id="main" className="focus">
        <TemplateView templateId={m.id ?? ''} />
      </main>
    );
  }
  if ((m = match('/admin', pathname))) return <AdminView tab="summary" me={me} />;
  if ((m = match('/admin/:tab', pathname))) return <AdminView tab={m.tab ?? 'summary'} me={me} />;
  if ((m = match('/spaces/:key', pathname))) return <SpaceHome spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/overview', pathname))) return <SpaceOverviewView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/files', pathname))) return <FilesView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/settings', pathname))) return <SpaceSettingsView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/trash', pathname))) return <TrashView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/sites', pathname))) return <SitesView spaceKey={m.key ?? ''} />;
  if ((m = match('/exports/:id', pathname))) return <ExportView id={m.id ?? ''} />;
  if ((m = match('/spaces/:key/access', pathname))) return <SpaceAccessView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/people', pathname))) return <SpacePeopleView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/audit', pathname))) return <SpaceAuditView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/content', pathname))) return <ContentView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/blog', pathname))) return <BlogView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/archive', pathname))) return <ArchiveView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/labels', pathname))) return <LabelsView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/templates', pathname))) return <TemplatesView spaceKey={m.key ?? ''} />;
  if ((m = match('/spaces/:key/new', pathname))) return <MissingPage spaceKey={m.key ?? ''} />;
  if ((m = match('/pages/:id/edit', pathname))) {
    return (
      <SpaceShell pageId={m.id ?? ''} focus>
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
  useAccent();
  const theme = me.data?.preferences.theme;
  useEffect(() => {
    if (theme) applyTheme(theme);
  }, [theme]);

  const publicScreen = PUBLIC_SCREENS[pathname];
  if (publicScreen) return publicScreen();
  // Mail links reach people who are not signed in, so this comes before the login check.
  const unsubscribe = match('/unsubscribe/:token', pathname);
  if (unsubscribe) return <UnsubscribeView token={unsubscribe.token ?? ''} />;
  if (me.isPending) return <Loading />;
  if (me.error) {
    if (me.error instanceof ApiError && me.error.status === 401) {
      const here = pathname === '/' ? '' : `?next=${encodeURIComponent(pathname + window.location.search)}`;
      navigate(`/login${here}`, true);
      return null;
    }
    return <ErrorNote error={me.error} />;
  }
  return (
    <ToastProvider>
      <ReauthProvider>
        {me.data.needs_second_factor ? (
          <Plain width="reading">
            <SecurityView me={me.data} />
          </Plain>
        ) : (
          <ShortcutsProvider enabled={me.data.preferences.shortcuts}>
            <PersonProvider value={me.data.user.id}>
              <Shell me={me.data}>
                <Routes me={me.data} />
              </Shell>
            </PersonProvider>
          </ShortcutsProvider>
        )}
      </ReauthProvider>
    </ToastProvider>
  );
}
