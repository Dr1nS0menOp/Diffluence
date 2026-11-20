// SPDX-License-Identifier: Apache-2.0
// Settings of a space or of the instance: the sections in the sidebar, in groups that
// fold, with a way to jump to one; the large area beside it changes them (ADR-034).
// The administration passes tool sections and a way back as well (spec 081, ADM-060 to ADM-067).
import { useEffect, useRef, useState, type ReactElement, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePreferences } from '../preferences';
import { Link } from '../router';
import { IconArrowBack, IconChevronDown } from './icons';

export interface SettingsItem {
  id: string;
  label: string;
  href: string;
  /** Tool sections only: the icon before the name. */
  icon?: (props: { size?: number }) => ReactElement;
  /** A word in a lozenge after the name, such as Experimental (ADM-067). */
  mark?: string;
}

export interface SettingsGroup {
  id: string;
  title: string;
  items: SettingsItem[];
}

/** The way back out of the settings, and the name that heads its sidebar (ADM-060). */
export interface SettingsBack {
  href: string;
  /** What the way back says to a screen reader. */
  label: string;
}

const CHEVRON_SIZE = 14;
const ICON_SIZE = 18;
const BACK_SIZE = 18;

/** Which items a typed text leaves, matching the words people see. */
export function matchingItems(groups: SettingsGroup[], text: string): SettingsGroup[] {
  const wanted = text.trim().toLowerCase();
  if (!wanted) return groups;
  return groups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => item.label.toLowerCase().includes(wanted)),
    }))
    .filter((group) => group.items.length > 0);
}

/** The folded groups after one heading was pressed. */
export function toggledGroups(shut: string[], id: string): string[] {
  return shut.includes(id) ? shut.filter((g) => g !== id) : [...shut, id];
}

function ItemLink({
  item,
  current,
  onFollow,
}: {
  item: SettingsItem;
  current: string;
  onFollow: () => void;
}) {
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      className="side-link"
      title={item.label}
      aria-current={item.id === current ? 'page' : undefined}
      // Following the current entry changes nothing, so it must not swallow the next jump.
      onClick={item.id === current ? undefined : onFollow}
    >
      {Icon && <Icon size={ICON_SIZE} />}
      <span className="side-link-text">{item.label}</span>
      {item.mark && <span className="lozenge">{item.mark}</span>}
    </Link>
  );
}

/** The header row of the administration: a way back and the name, which stays in place. */
function BackRow({ back, title, scrolled }: { back: SettingsBack; title: string; scrolled: boolean }) {
  return (
    <div className={scrolled ? 'settings-head scrolled' : 'settings-head'}>
      <Link href={back.href} className="button icon quiet" aria-label={back.label} title={back.label}>
        <IconArrowBack size={BACK_SIZE} />
      </Link>
      <p className="settings-title">{title}</p>
    </div>
  );
}

/** Whether something scrolled beneath the header row, seen by a mark at the top of the list. */
function useScrolledBeneath(active: boolean) {
  const mark = useRef<HTMLDivElement>(null);
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const element = mark.current;
    if (!active || !element) return;
    const watcher = new IntersectionObserver(([entry]) => setScrolled(!entry?.isIntersecting), {
      root: element.parentElement,
    });
    watcher.observe(element);
    return () => watcher.disconnect();
  }, [active]);
  return { mark, scrolled };
}

/**
 * Brings the current section into view once when it was opened from somewhere other than the
 * sidebar, such as an address or the jump field (D7): its group shows open while it is the one
 * revealed, and the sidebar scrolls without motion. Moving by the sidebar never scrolls it. Only the
 * administration, which passes tool sections, does this; the settings of a space follow in SC5.
 */
function useCurrentInView(active: boolean, current: string, holder: string | null, entries: number) {
  const aside = useRef<HTMLElement>(null);
  const followed = useRef(false);
  const pending = useRef(false);
  const [revealed, setRevealed] = useState<string | null>(null);
  useEffect(() => {
    if (!active) return;
    if (followed.current) {
      followed.current = false;
      return;
    }
    setRevealed(holder);
    pending.current = true;
    // The number of entries joins in because entries that wait for an answer, such as experimental
    // sections, can push the current one out of view after the first look.
  }, [active, current, holder, entries]);
  useEffect(() => {
    if (!pending.current) return;
    pending.current = false;
    const link = aside.current?.querySelector<HTMLElement>('[aria-current="page"]');
    link?.scrollIntoView({ block: 'nearest', behavior: 'instant' });
  });
  return { aside, revealed, hide: () => setRevealed(null), follow: () => (followed.current = true) };
}

export function SettingsFrame({
  title,
  head,
  back,
  search,
  tools,
  settingsTitle,
  groups,
  current,
  width = 'full',
  children,
}: {
  title: string;
  /** What sits above the sections, such as the way back to a space. */
  head?: ReactNode;
  /** A header row with a way back and the title, which stays in place while the list scrolls. */
  back?: SettingsBack;
  /** A search of the caller's own; without one, typing narrows the sections. */
  search?: ReactNode;
  /** Sections that are always open, each entry with an icon, before the groups that fold. */
  tools?: SettingsGroup[];
  /** The heading over the groups that fold, with the search under it. */
  settingsTitle?: string;
  groups: SettingsGroup[];
  current: string;
  /** One column of reading width centred in the area, or the full width beside the sidebar. */
  width?: 'column' | 'full';
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  // Folded groups follow the person to other devices, like the parts of the sidebar.
  const { preferences, update } = usePreferences();
  const shut = preferences.settings_shut ?? [];
  const shown = matchingItems(groups, text);
  const { mark, scrolled } = useScrolledBeneath(back !== undefined);
  const holder = groups.find((group) => group.items.some((item) => item.id === current))?.id ?? null;
  const entries = [...(tools ?? []), ...groups].reduce((sum, group) => sum + group.items.length, 0);
  const { aside, revealed, hide, follow } = useCurrentInView(tools !== undefined, current, holder, entries);
  const jump = search ?? (
    <input
      type="search"
      className="settings-jump"
      value={text}
      aria-label={t('settings.jump')}
      placeholder={t('settings.jump')}
      onChange={(e) => setText(e.target.value)}
    />
  );
  const folding = (
    <>
      {shown.map((group) => {
        // A search shows every group it found something in, folded or not, and a shut group
        // shows the section opened from elsewhere until its heading is pressed.
        const isShut = shut.includes(group.id);
        const isRevealed = isShut && group.id === revealed;
        const open = text.trim() !== '' || !isShut || isRevealed;
        const listId = `settings-group-${group.id}`;
        return (
          <div key={group.id} className="settings-group">
            <button
              type="button"
              className="sidebar-part-toggle"
              aria-expanded={open}
              aria-controls={listId}
              onClick={() => (isRevealed ? hide() : update({ settings_shut: toggledGroups(shut, group.id) }))}
            >
              <span className={open ? 'part-chevron' : 'part-chevron shut'}>
                <IconChevronDown size={CHEVRON_SIZE} />
              </span>
              {group.title}
            </button>
            {open && (
              <div id={listId} className="side-links">
                {group.items.map((item) => (
                  <ItemLink key={item.id} item={item} current={current} onFollow={follow} />
                ))}
              </div>
            )}
          </div>
        );
      })}
      {shown.length === 0 && <p className="muted">{t('settings.nothing')}</p>}
    </>
  );
  return (
    <div className="split">
      <div className="sidebar-frame">
        <aside ref={aside} className={tools ? 'sidebar settings-sidebar tiered' : 'sidebar settings-sidebar'}>
          {back && <div ref={mark} className="settings-scroll-mark" />}
          {back && <BackRow back={back} title={title} scrolled={scrolled} />}
          {head}
          {!tools && jump}
          <nav aria-label={title} className="settings-nav">
            {tools?.map((group) => (
              <div key={group.id} className="settings-tools">
                <h2 className="settings-tools-title">{group.title}</h2>
                <div className="side-links">
                  {group.items.map((item) => (
                    <ItemLink key={item.id} item={item} current={current} onFollow={follow} />
                  ))}
                </div>
              </div>
            ))}
            {tools && (
              <div className="settings-part">
                <h2 className="settings-part-title">{settingsTitle}</h2>
                {jump}
              </div>
            )}
            {folding}
          </nav>
        </aside>
      </div>
      <main id="main" className="content">
        <div className={width === 'column' ? 'column settings-area centred' : 'column wide settings-area'}>
          {children}
        </div>
      </main>
    </div>
  );
}
