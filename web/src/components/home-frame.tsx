// SPDX-License-Identifier: Apache-2.0
// The frame of the personal screens: the home page, Recent, Favourites, Tasks, Watching
// and the list of spaces. The sidebar stays in view here too (ADR-034, spec 033).
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useLocation } from '../router';
import { IconApps, IconClock, IconEye, IconHome, IconStar, IconTask } from './icons';
import { SidebarPart } from './sidebar-part';
import { useOrderedSpaces } from './space-strip';
import { SpaceBadge } from './ui';

const LINK_ICON_SIZE = 18;
const SPACES_SHOWN = 12;

const PERSONAL_LINKS = [
  { href: '/', label: 'home.forYou', Icon: IconHome },
  { href: '/recent', label: 'recent.title', Icon: IconClock },
  { href: '/favourites', label: 'favourites.title', Icon: IconStar },
  { href: '/tasks', label: 'tasks.title', Icon: IconTask },
  { href: '/me/watching', label: 'watching.title', Icon: IconEye },
] as const;

function HomeSidebar() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const spaces = useOrderedSpaces().slice(0, SPACES_SHOWN);
  return (
    <aside className="sidebar">
      <SidebarPart name="you" title={t('home.you')}>
        <nav className="side-links" aria-label={t('home.you')}>
          {PERSONAL_LINKS.map(({ href, label, Icon }) => (
            <Link
              key={href}
              href={href}
              className="side-link"
              aria-current={pathname === href ? 'page' : undefined}
            >
              <Icon size={LINK_ICON_SIZE} /> {t(label)}
            </Link>
          ))}
        </nav>
      </SidebarPart>
      <SidebarPart name="spaces" title={t('home.spaces')}>
        <nav className="side-links" aria-label={t('home.spaces')}>
          {spaces.map((space) => (
            <Link key={space.id} href={`/spaces/${space.key}`} className="side-link">
              <SpaceBadge space={space} /> <span className="side-link-text">{space.name}</span>
            </Link>
          ))}
          <Link
            href="/spaces"
            className="side-link"
            aria-current={pathname === '/spaces' ? 'page' : undefined}
          >
            <IconApps size={LINK_ICON_SIZE} /> {t('home.allSpaces')}
          </Link>
        </nav>
      </SidebarPart>
    </aside>
  );
}

/** A personal screen with the sidebar beside it. */
export function HomeFrame({ width, children }: { width?: 'reading'; children: ReactNode }) {
  return (
    <div className="split">
      <div className="sidebar-frame">
        <HomeSidebar />
      </div>
      <main id="main" className="content">
        <div className={width === 'reading' ? 'column' : 'column wide'}>{children}</div>
      </main>
    </div>
  );
}
