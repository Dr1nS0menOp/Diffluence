// SPDX-License-Identifier: Apache-2.0
// The menus of the top bar. Their lists load when they open, so the bar costs
// nothing on each page.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type ListedPage, type Me, type Space, type SystemInfo } from '../api';
import { IconChevronDown, IconHelp, IconPage } from './icons';
import { Menu, MenuDownload, MenuItem, MenuLink, MenuSeparator } from './menu';
import { ShortcutsDialog } from './shortcuts';
import { Avatar, Dialog, ErrorNote, Loading, RelativeTime, SpaceBadge } from './ui';

const RECENT_IN_MENU = 8;
const FAVOURITES_IN_MENU = 12;
const SPACES_IN_MENU = 8;
const SPACE_MENU_ICON_SIZE = 16;
const DOCUMENTATION = 'https://github.com/Dr1nS0menOp/Diffluence/tree/main/docs';
const THIRD_PARTY_LICENCES = 'https://github.com/Dr1nS0menOp/Diffluence/blob/main/docs/dependencies.md';

function SpaceEntries() {
  const { t } = useTranslation();
  const mine = useQuery({
    queryKey: ['my-spaces'],
    queryFn: () => api.get<{ favourites: Space[]; recent: Space[] }>('/me/spaces'),
  });
  const all = useQuery({
    queryKey: ['spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/spaces'),
  });
  const known = [...(mine.data?.favourites ?? []), ...(mine.data?.recent ?? [])];
  const knownIds = new Set(known.map((s) => s.id));
  const others = (all.data?.spaces ?? []).filter((s) => !knownIds.has(s.id));
  const entries = [...known, ...others].slice(0, SPACES_IN_MENU);
  return (
    <>
      {entries.map((s) => (
        <MenuLink key={s.id} href={`/spaces/${s.key}`}>
          <SpaceBadge space={s} size="small" /> {s.name}
        </MenuLink>
      ))}
      {entries.length > 0 && <MenuSeparator />}
      <MenuLink href="/spaces">{t('nav.allSpaces')}</MenuLink>
    </>
  );
}

export function SpacesMenu() {
  const { t } = useTranslation();
  return (
    <Menu
      label={t('nav.spaces')}
      align="start"
      className="nav-link"
      trigger={
        <>
          {t('nav.spaces')} <IconChevronDown size={SPACE_MENU_ICON_SIZE} />
        </>
      }
    >
      <SpaceEntries />
    </Menu>
  );
}

function PageEntries({
  path,
  limit,
  empty,
  more,
  moreLabel,
}: {
  path: string;
  limit: number;
  empty: string;
  more: string;
  moreLabel: string;
}) {
  const q = useQuery({ queryKey: [path], queryFn: () => api.get<{ pages: ListedPage[] }>(path) });
  const pages = (q.data?.pages ?? []).slice(0, limit);
  return (
    <>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data?.pages.length === 0 && <p className="menu-note">{empty}</p>}
      {pages.map((p) => (
        <MenuLink key={p.id} href={`/pages/${p.id}`}>
          <PageMark page={p} />
          <span className="menu-page">
            <span>{p.title}</span>
            <span className="muted">
              {p.space_name} · <RelativeTime value={p.at} />
            </span>
          </span>
        </MenuLink>
      ))}
      {pages.length > 0 && <MenuSeparator />}
      <MenuLink href={more}>{moreLabel}</MenuLink>
    </>
  );
}

/** A page's own emoji, or the page icon. */
export function PageMark({ page }: { page: { emoji: string } }) {
  return page.emoji ? (
    <span className="page-emoji" aria-hidden="true">
      {page.emoji}
    </span>
  ) : (
    <IconPage size={SPACE_MENU_ICON_SIZE} />
  );
}

export function RecentMenu() {
  const { t } = useTranslation();
  return (
    <Menu
      label={t('nav.recent')}
      align="start"
      className="nav-link"
      trigger={
        <>
          {t('nav.recent')} <IconChevronDown size={SPACE_MENU_ICON_SIZE} />
        </>
      }
    >
      <PageEntries
        path="/me/recent"
        limit={RECENT_IN_MENU}
        empty={t('nav.noRecent')}
        more="/recent"
        moreLabel={t('nav.allRecent')}
      />
    </Menu>
  );
}

export function FavouritesMenu() {
  const { t } = useTranslation();
  return (
    <Menu
      label={t('nav.favourites')}
      align="start"
      className="nav-link"
      trigger={
        <>
          {t('nav.favourites')} <IconChevronDown size={SPACE_MENU_ICON_SIZE} />
        </>
      }
    >
      <PageEntries
        path="/me/favourites"
        limit={FAVOURITES_IN_MENU}
        empty={t('nav.noFavourites')}
        more="/favourites"
        moreLabel={t('nav.allFavourites')}
      />
    </Menu>
  );
}

function AboutDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const info = useQuery({ queryKey: ['system-info'], queryFn: () => api.get<SystemInfo>('/system/info') });
  return (
    <Dialog title={t('help.about')} onClose={onClose}>
      <ErrorNote error={info.error} />
      {info.data && <p>{t('help.version', { version: info.data.version })}</p>}
      <p>{t('help.licence')}</p>
      <p>
        <a href={THIRD_PARTY_LICENCES} target="_blank" rel="noopener noreferrer">
          {t('help.thirdParty')}
        </a>
      </p>
    </Dialog>
  );
}

export function HelpMenu() {
  const { t } = useTranslation();
  const [dialog, setDialog] = useState<'shortcuts' | 'about' | null>(null);
  const close = () => setDialog(null);
  return (
    <>
      <Menu label={t('help.menu')} trigger={<IconHelp />}>
        <MenuItem onClick={() => setDialog('shortcuts')}>{t('shortcuts.title')}</MenuItem>
        <MenuDownload href={DOCUMENTATION} target="_blank" rel="noopener noreferrer">
          {t('help.documentation')}
        </MenuDownload>
        <MenuItem onClick={() => setDialog('about')}>{t('help.about')}</MenuItem>
      </Menu>
      {dialog === 'shortcuts' && <ShortcutsDialog onClose={close} />}
      {dialog === 'about' && <AboutDialog onClose={close} />}
    </>
  );
}

export function AccountMenu({ me, onLogout }: { me: Me; onLogout: () => void }) {
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
        <MenuLink href="/me/preferences">{t('nav.preferences')}</MenuLink>
        {isAdmin && <MenuLink href="/admin/users">{t('nav.admin')}</MenuLink>}
        <MenuSeparator />
        <MenuItem onClick={onLogout}>{t('nav.logout')}</MenuItem>
      </Menu>
    </div>
  );
}
