// SPDX-License-Identifier: Apache-2.0
// The round buttons at the end of the top bar: help, the administration and the account
// (spec 078, NAV-094 to NAV-098). Their lists load when they open, so the bar costs nothing.
import { useEffect, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type Space, type SystemInfo } from '../api';
import { appearanceQuery } from '../appearance';
import { usePreferences } from '../preferences';
import { Link, navigate } from '../router';
import {
  IconChevronRight,
  IconHelp,
  IconLogout,
  IconPage,
  IconPerson,
  IconSettings,
  IconSliders,
  IconTheme,
  IconTick,
} from './icons';
import { DOCUMENTATION_ADDRESS } from './guide-link';
import { Menu, MenuDownload, MenuItem, MenuLink, MenuSeparator } from './menu';
import { personQuery, PersonPicture } from './person';
import { ShortcutsDialog } from './shortcuts';
import { Avatar, Dialog, ErrorNote } from './ui';

const MENU_ICON_SIZE = 18;
const TOP_ICON_SIZE = 20;
const PAGE_ICON_SIZE = 16;
const THEMES = ['light', 'dark', 'system'] as const;
const THIRD_PARTY_LICENCES = 'https://github.com/Dr1nS0menOp/Diffluence/blob/main/docs/dependencies.md';

/** A page's own emoji, or the page icon. */
export function PageMark({ page }: { page: { emoji: string } }) {
  return page.emoji ? (
    <span className="page-emoji" aria-hidden="true">
      {page.emoji}
    </span>
  ) : (
    <IconPage size={PAGE_ICON_SIZE} />
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
  const helpAddress = useQuery(appearanceQuery).data?.help_url || DOCUMENTATION_ADDRESS;
  return (
    <>
      <Menu label={t('help.menu')} className="icon top-icon" trigger={<IconHelp size={TOP_ICON_SIZE} />}>
        <MenuItem onClick={() => setDialog('shortcuts')}>{t('shortcuts.title')}</MenuItem>
        <MenuDownload href={helpAddress} target="_blank" rel="noopener noreferrer">
          {t('help.documentation')}
        </MenuDownload>
        <MenuItem onClick={() => setDialog('about')}>{t('help.about')}</MenuItem>
      </Menu>
      {dialog === 'shortcuts' && <ShortcutsDialog onClose={close} />}
      {dialog === 'about' && <AboutDialog onClose={close} />}
    </>
  );
}

/** The administration, for those who may open it (NAV-095). */
export function SettingsButton({ me }: { me: Me }) {
  const { t } = useTranslation();
  if (!me.global_permissions.includes('administrator')) return null;
  return (
    <Link href="/admin/summary" className="top-icon" aria-label={t('nav.admin')} title={t('nav.admin')}>
      <IconSettings size={TOP_ICON_SIZE} />
    </Link>
  );
}

/** My space, or the way to get one. Without the permission there is nothing to show. */
function PersonalSpaceEntry({ me }: { me: Me }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const create = useMutation({
    mutationFn: () => api.post<Space>('/me/personal-space'),
    onSuccess: async (space) => {
      await client.invalidateQueries({ queryKey: ['me'] });
      navigate(`/spaces/${space.key}`);
    },
  });
  if (me.personal_space) return <MenuLink href={`/spaces/${me.personal_space}`}>{t('nav.mySpace')}</MenuLink>;
  if (!me.global_permissions.includes('create_personal_space')) return null;
  return <MenuItem onClick={() => create.mutate()}>{t('nav.createMySpace')}</MenuItem>;
}

/**
 * Theme: a sub-menu of light, dark and system, with a tick on the choice (NAV-097). It opens
 * toward the start side, because the account menu stands at the end of the bar.
 */
function ThemeEntry() {
  const { t } = useTranslation();
  const { preferences, update } = usePreferences();
  const [open, setOpen] = useState(false);
  const entry = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (open) list.current?.querySelector<HTMLElement>('[role="menuitemradio"]')?.focus();
  }, [open]);
  const isRtl = () => getComputedStyle(document.documentElement).direction === 'rtl';
  const inward = () => (isRtl() ? 'ArrowLeft' : 'ArrowRight');
  const outward = () => (isRtl() ? 'ArrowRight' : 'ArrowLeft');
  const closeSub = () => {
    setOpen(false);
    entry.current?.focus();
  };
  const entryKeys = (e: KeyboardEvent<HTMLButtonElement>) => {
    if (e.key !== inward()) return;
    e.preventDefault();
    e.stopPropagation();
    setOpen(true);
  };
  const subKeys = (e: KeyboardEvent<HTMLDivElement>) => {
    const items = Array.from(list.current?.querySelectorAll<HTMLElement>('[role="menuitemradio"]') ?? []);
    const at = items.indexOf(document.activeElement as HTMLElement);
    const step = e.key === 'ArrowDown' ? 1 : e.key === 'ArrowUp' ? -1 : 0;
    if (step) {
      items[(at + step + items.length) % items.length]?.focus();
    } else if (e.key === 'Escape' || e.key === outward()) {
      closeSub();
    } else if (e.key === 'Home' || e.key === 'End') {
      items[e.key === 'Home' ? 0 : items.length - 1]?.focus();
    } else {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
  };
  return (
    <div className="sub-menu" role="none">
      <button
        ref={entry}
        type="button"
        role="menuitem"
        aria-haspopup="menu"
        aria-expanded={open}
        onKeyDown={entryKeys}
        onClick={(e: MouseEvent) => {
          // The account menu closes on a click inside it; this click opens the sub-menu instead.
          e.stopPropagation();
          setOpen(!open);
        }}
      >
        <IconTheme size={MENU_ICON_SIZE} />
        <span className="grow">{t('nav.theme')}</span>
        <span className="menu-arrow">
          <IconChevronRight size={MENU_ICON_SIZE} />
        </span>
      </button>
      {open && (
        <div
          ref={list}
          role="menu"
          aria-label={t('nav.theme')}
          className="menu-list sub-menu-list"
          onKeyDown={subKeys}
        >
          {THEMES.map((theme) => (
            <button
              key={theme}
              type="button"
              role="menuitemradio"
              aria-checked={preferences.theme === theme}
              onClick={() => update({ theme })}
            >
              <span className="menu-tick">
                {preferences.theme === theme && <IconTick size={MENU_ICON_SIZE} />}
              </span>
              {t(`preferences.theme_${theme}`)}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/** The account menu: who you are, your own places, the theme and Log out (NAV-096). */
export function AccountMenu({ me, onLogout }: { me: Me; onLogout: () => void }) {
  const { t } = useTranslation();
  const person = useQuery(personQuery(me.user.id)).data;
  const instance = useQuery(appearanceQuery).data?.name || t('app.name');
  return (
    <div className="top-account">
      <Menu
        label={t('nav.accountMenu')}
        className="avatar-button top-icon"
        trigger={
          person ? (
            <PersonPicture person={person} size="small" />
          ) : (
            <Avatar name={me.user.display_name} size="small" />
          )
        }
      >
        <div role="none" className="menu-identity">
          {person ? (
            <PersonPicture person={person} size="large" />
          ) : (
            <Avatar name={me.user.display_name} size="large" />
          )}
          <span className="menu-identity-text">
            <strong>{me.user.display_name}</strong>
            <span className="muted">{me.user.email}</span>
          </span>
        </div>
        <MenuLink href={`/people/${me.user.id}`}>
          <IconPerson size={MENU_ICON_SIZE} /> {t('nav.ownProfile')}
        </MenuLink>
        <MenuLink href="/me">
          <IconSliders size={MENU_ICON_SIZE} /> {t('nav.profile')}
        </MenuLink>
        <ThemeEntry />
        <MenuSeparator />
        <div role="group" aria-labelledby="account-instance-heading">
          <div role="presentation" id="account-instance-heading" className="menu-group-title">
            {instance}
          </div>
          <PersonalSpaceEntry me={me} />
          <MenuLink href="/tasks">{t('nav.tasks')}</MenuLink>
          <MenuLink href="/me/preferences">{t('nav.preferences')}</MenuLink>
        </div>
        <MenuSeparator />
        <MenuItem onClick={onLogout}>
          <IconLogout size={MENU_ICON_SIZE} /> {t('nav.logout')}
        </MenuItem>
      </Menu>
    </div>
  );
}
