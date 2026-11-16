// SPDX-License-Identifier: Apache-2.0
// The upper part of the sidebar: a person's own places, the same on every screen outside the
// settings (spec 078, NAV-125 to NAV-128), and the favourite spaces of the personal screens.
import { useRef, useState, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Me } from '../api';
import { usePreferences } from '../preferences';
import { Link, useLocation } from '../router';
import { NewSpaceDialog } from './create';
import { CustomiseSidebarDialog } from './customise-sidebar';
import { IconApps, IconChevronRight, IconMore, IconSliders } from './icons';
import { Menu, MenuItem, MenuLink, MenuSeparator } from './menu';
import { mySpacesQuery } from './my-spaces';
import { FavouritesPanel, RecentPanel, SpacesPanel } from './sidebar-panels';
import { arrangeEntries, type SidebarEntry } from './sidebar-entries';
import { ENTRY_VIEWS, START_ENTRY, type EntryView, type PanelName } from './sidebar-entry-views';
import { PAGE_DRAG_TYPE } from './tree-model';
import { SpaceBadge } from './ui';

const ENTRY_ICON_SIZE = 18;
const ARROW_SIZE = 14;
const FAVOURITE_SPACES_SHOWN = 20;

function useMe(): Me | undefined {
  return useQuery({ queryKey: ['me'], queryFn: () => api.get<Me>('/me') }).data;
}

/** An entry that opens a panel: a button with an arrow at its end, tinted while its panel is open. */
function PanelEntry({
  entry,
  open,
  opener,
  onToggle,
}: {
  entry: SidebarEntry;
  open: boolean;
  opener: RefObject<HTMLButtonElement | null>;
  onToggle: () => void;
}) {
  const { t } = useTranslation();
  const { label, Icon, panel } = ENTRY_VIEWS[entry];
  return (
    <button
      ref={opener}
      type="button"
      className={open ? 'side-link side-entry open' : 'side-link side-entry'}
      aria-expanded={open}
      aria-controls={open ? `panel-${panel}` : undefined}
      onClick={onToggle}
      onDragEnter={(e) => {
        // A page held over Spaces opens the panel, so it can be dropped on a space (SPC-044).
        if (panel === 'spaces' && !open && e.dataTransfer.types.includes(PAGE_DRAG_TYPE)) onToggle();
      }}
    >
      <Icon size={ENTRY_ICON_SIZE} />
      <span className="side-link-text">{t(label)}</span>
      <span className="side-entry-arrow">
        <IconChevronRight size={ARROW_SIZE} />
      </span>
    </button>
  );
}

/** The global part: the start screen, the person's entries in their order, then More or Customise. */
export function GlobalPart() {
  const { t } = useTranslation();
  const me = useMe();
  const { pathname } = useLocation();
  const { preferences } = usePreferences();
  const [panel, setPanel] = useState<{ name: PanelName; opener: RefObject<HTMLButtonElement | null> } | null>(
    null,
  );
  const [customising, setCustomising] = useState(false);
  const [creatingSpace, setCreatingSpace] = useState(false);
  const recentRef = useRef<HTMLButtonElement>(null);
  const favouritesRef = useRef<HTMLButtonElement>(null);
  const spacesRef = useRef<HTMLButtonElement>(null);
  const moreRef = useRef<HTMLDivElement>(null);
  const openers: Record<PanelName, RefObject<HTMLButtonElement | null>> = {
    recent: recentRef,
    favourites: favouritesRef,
    spaces: spacesRef,
  };
  const { shown, hidden } = arrangeEntries(preferences.sidebar_order, preferences.sidebar_hidden);
  const toggle = (name: PanelName, opener: RefObject<HTMLButtonElement | null>) =>
    setPanel(panel?.name === name ? null : { name, opener });
  const close = () => setPanel(null);
  // An entry under More that opens a panel gives the focus back to the More button.
  const openFromMore = (name: PanelName) =>
    toggle(name, { current: moreRef.current?.querySelector('button') ?? null });

  const linkEntry = (href: string, label: string, Icon: EntryView['Icon']) => (
    <Link key={href} href={href} className="side-link" aria-current={pathname === href ? 'page' : undefined}>
      <Icon size={ENTRY_ICON_SIZE} />
      <span className="side-link-text">{t(label)}</span>
    </Link>
  );

  return (
    <>
      <nav className="global-part side-links" aria-label={t('nav.yourPlaces')}>
        {linkEntry(START_ENTRY.href, START_ENTRY.label, START_ENTRY.Icon)}
        {shown.map((entry) => {
          const view = ENTRY_VIEWS[entry];
          if (view.panel) {
            const name = view.panel;
            return (
              <PanelEntry
                key={entry}
                entry={entry}
                open={panel?.name === name}
                opener={openers[name]}
                onToggle={() => toggle(name, openers[name])}
              />
            );
          }
          return linkEntry(view.href ?? '/', view.label, view.Icon);
        })}
        <div className="global-part-end" ref={moreRef}>
          {hidden.length > 0 ? (
            <Menu
              label={t('nav.moreEntries')}
              className="side-link side-entry"
              align="start"
              trigger={
                <>
                  <IconMore size={ENTRY_ICON_SIZE} />
                  <span className="side-link-text">{t('nav.moreEntries')}</span>
                </>
              }
            >
              {hidden.map((entry) => {
                const view = ENTRY_VIEWS[entry];
                const name = view.panel;
                return name ? (
                  <MenuItem key={entry} onClick={() => openFromMore(name)}>
                    <view.Icon size={ENTRY_ICON_SIZE} /> {t(view.label)}
                  </MenuItem>
                ) : (
                  <MenuLink key={entry} href={view.href ?? '/'}>
                    <view.Icon size={ENTRY_ICON_SIZE} /> {t(view.label)}
                  </MenuLink>
                );
              })}
              <MenuSeparator />
              <MenuItem onClick={() => setCustomising(true)}>
                <IconSliders size={ENTRY_ICON_SIZE} /> {t('nav.customise')}
              </MenuItem>
            </Menu>
          ) : (
            <button type="button" className="side-link side-entry" onClick={() => setCustomising(true)}>
              <IconSliders size={ENTRY_ICON_SIZE} />
              <span className="side-link-text">{t('nav.customise')}</span>
            </button>
          )}
        </div>
      </nav>
      {panel?.name === 'recent' && <RecentPanel name="recent" opener={panel.opener} onClose={close} />}
      {panel?.name === 'favourites' && (
        <FavouritesPanel name="favourites" opener={panel.opener} onClose={close} />
      )}
      {panel?.name === 'spaces' && me && (
        <SpacesPanel
          name="spaces"
          me={me}
          opener={panel.opener}
          onClose={close}
          onCreateSpace={() => {
            close();
            setCreatingSpace(true);
          }}
        />
      )}
      {customising && <CustomiseSidebarDialog onClose={() => setCustomising(false)} />}
      {creatingSpace && <NewSpaceDialog onClose={() => setCreatingSpace(false)} />}
    </>
  );
}

/** On the personal screens: the favourite spaces, and the way to all of them (NAV-127). */
export function FavouriteSpaces() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const mine = useQuery(mySpacesQuery);
  const spaces = (mine.data?.favourites ?? []).slice(0, FAVOURITE_SPACES_SHOWN);
  return (
    <nav className="side-links favourite-spaces" aria-labelledby="favourite-spaces-title">
      <h2 id="favourite-spaces-title" className="side-heading">
        {t('nav.favouriteSpaces')}
      </h2>
      {spaces.map((space) => (
        <Link key={space.id} href={`/spaces/${space.key}`} className="side-link">
          <SpaceBadge space={space} size="small" />
          <span className="side-link-text">{space.name}</span>
        </Link>
      ))}
      <Link href="/spaces" className="side-link" aria-current={pathname === '/spaces' ? 'page' : undefined}>
        <IconApps size={ENTRY_ICON_SIZE} />
        <span className="side-link-text">{t('nav.allSpaces')}</span>
      </Link>
    </nav>
  );
}
