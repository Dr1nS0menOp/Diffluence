// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError, type Me, type Space } from './api';
import { useImportedFont } from './appearance';
import { Link, match, navigate, useLocation } from './router';
import { NewPageDialog, NewSpaceDialog } from './components/create';
import { SpaceShell, useCreateTarget } from './components/frame';
import { IconChevronDown, IconPlus, IconSearch, Logo } from './components/icons';
import { Menu, MenuItem, MenuLink, MenuSeparator } from './components/menu';
import { Avatar, ErrorNote, Loading, SpaceBadge } from './components/ui';
import { AdminView } from './views/admin';
import { HomeView } from './views/home';
import { LoginView } from './views/login';
import { EditView } from './views/edit';
import { CompareView, HistoryView } from './views/history';
import { PageView } from './views/page';
import { ProfileView } from './views/profile';
import { SearchView } from './views/search';
import { SpaceHome, SpaceSettingsView, TrashView } from './views/space';

const SPACES_IN_MENU = 12;

function SpacesMenu() {
  const { t } = useTranslation();
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  return (
    <Menu
      label={t('nav.spaces')}
      align="start"
      className="nav-link"
      trigger={
        <>
          {t('nav.spaces')} <IconChevronDown size={16} />
        </>
      }
    >
      {spaces.data?.spaces.slice(0, SPACES_IN_MENU).map((s) => (
        <MenuLink key={s.id} href={`/spaces/${s.key}`}>
          <SpaceBadge space={s} size="small" /> {s.name}
        </MenuLink>
      ))}
      {(spaces.data?.spaces.length ?? 0) > 0 && <MenuSeparator />}
      <MenuLink href="/">{t('nav.allSpaces')}</MenuLink>
    </Menu>
  );
}

function AccountMenu({ me, onLogout }: { me: Me; onLogout: () => void }) {
  const { t } = useTranslation();
  const isAdmin = me.global_permissions.includes('administrator');
  return (
    <div className="top-account">
      <Menu
        label={t('nav.accountMenu')}
        className="avatar-button"
        trigger={<Avatar name={me.user.display_name} />}
      >
        <div role="none" className="menu-heading">
          <strong>{me.user.display_name}</strong>
          <span className="muted">{me.user.email}</span>
        </div>
        <MenuSeparator />
        <MenuLink href="/me">{t('nav.profile')}</MenuLink>
        {isAdmin && <MenuLink href="/admin/users">{t('nav.admin')}</MenuLink>}
        <MenuSeparator />
        <MenuItem onClick={onLogout}>{t('nav.logout')}</MenuItem>
      </Menu>
    </div>
  );
}

function Shell({ me, children }: { me: Me; children: ReactNode }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { pathname } = useLocation();
  const [q, setQ] = useState('');
  const target = useCreateTarget();
  const [creating, setCreating] = useState(false);
  const canCreate = target !== null || me.global_permissions.includes('create_space');
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
          <Logo /> {t('app.name')}
        </Link>
        <nav aria-label={t('nav.mainMenu')} className="top-nav">
          <Link href="/" className="nav-link" aria-current={pathname === '/' ? 'page' : undefined}>
            {t('nav.home')}
          </Link>
          <SpacesMenu />
          {canCreate && (
            <button type="button" className="primary top-create" onClick={() => setCreating(true)}>
              <IconPlus size={16} /> {t('nav.create')}
            </button>
          )}
        </nav>
        <form role="search" onSubmit={search} className="top-search">
          <IconSearch size={18} />
          <input
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t('nav.searchPlaceholder')}
            aria-label={t('nav.search')}
          />
        </form>
        <AccountMenu me={me} onLogout={() => void logout()} />
      </header>
      {children}
      {creating && target && (
        <NewPageDialog space={target.space} parent={target.parent} onClose={() => setCreating(false)} />
      )}
      {creating && !target && <NewSpaceDialog onClose={() => setCreating(false)} />}
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
  if ((m = match('/admin/:tab', pathname))) return <AdminView tab={m.tab ?? 'users'} me={me} />;
  if ((m = match('/spaces/:key', pathname))) return <SpaceHome spaceKey={m.key ?? ''} />;
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
