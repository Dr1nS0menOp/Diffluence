// SPDX-License-Identifier: Apache-2.0
// Settings of a space, of the instance or of one template: the sections in the sidebar, in groups
// that fold, with a way to jump to one; the large area beside it changes them (ADR-034). One
// description per kind of sidebar says how they differ (spec 083, D4): the administration has tool
// sections and a search of its own (spec 081), a space has a head with its badge and four pinned
// entries (spec 083, SPC-136 to SPC-147), and a single template keeps the plain list (D19).
import { useEffect, useRef, useState, type ReactElement, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { usePreferences } from '../preferences';
import { Link, navigate } from '../router';
import { IconArrowBack, IconChevronDown, IconSearch } from './icons';

export type SettingsIcon = (props: { size?: number }) => ReactElement;

export interface SettingsItem {
  id: string;
  label: string;
  href: string;
  /** Tool sections and pinned entries only: the icon before the name. */
  icon?: SettingsIcon;
  /** A word in a lozenge after the name, such as Experimental (ADM-067). */
  mark?: string;
  /** Names of sections this entry took over, which the jump field also finds (spec 083, D11). */
  formerly?: string[];
}

export interface SettingsGroup {
  id: string;
  title: string;
  items: SettingsItem[];
}

/** The way back out of the settings, and the name that heads its sidebar (ADM-060, SPC-136). */
export interface SettingsBack {
  href: string;
  /** What the way back says to a screen reader. */
  label: string;
}

/** What sits in the sidebar besides the groups that fold, one description per kind (D4). */
export type SettingsSidebar =
  | {
      kind: 'admin';
      back: SettingsBack;
      /** The administration's own search, which finds settings inside sections too. */
      search: ReactNode;
      /** Sections that are always open, each entry with an icon, before the groups that fold. */
      tools: SettingsGroup[];
      /** The heading over the groups that fold, with the search under it. */
      settingsTitle: string;
    }
  | {
      kind: 'space';
      back: SettingsBack;
      /** The row under the way back: the space's badge and name. */
      head: ReactNode;
      /** The entries on their own above the divider, each with an icon (SPC-138). */
      pinned: SettingsItem[];
    }
  | {
      kind: 'single';
      /** What sits above the sections, such as the way back to a template. */
      head?: ReactNode;
    };

const CHEVRON_SIZE = 14;
const ICON_SIZE = 18;
const BACK_SIZE = 18;
const SEARCH_SIZE = 16;

/** Whether an item's name, or a name it took over, holds the typed words. */
function itemMatches(item: SettingsItem, wanted: string): boolean {
  return [item.label, ...(item.formerly ?? [])].some((name) => name.toLowerCase().includes(wanted));
}

/** Which items a typed text leaves, matching the words people see. */
export function matchingItems(groups: SettingsGroup[], text: string): SettingsGroup[] {
  const wanted = text.trim().toLowerCase();
  if (!wanted) return groups;
  return groups
    .map((group) => ({ ...group, items: group.items.filter((item) => itemMatches(item, wanted)) }))
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

/** A way back and the name of the settings, in one row. */
function BackLine({ back, title }: { back: SettingsBack; title: string }) {
  return (
    <>
      <Link href={back.href} className="button icon quiet" aria-label={back.label} title={back.label}>
        <IconArrowBack size={BACK_SIZE} />
      </Link>
      <p className="settings-title">{title}</p>
    </>
  );
}

/** The header row of the administration: it stays in place, with a line once something is under it. */
function BackRow({ back, title, scrolled }: { back: SettingsBack; title: string; scrolled: boolean }) {
  return (
    <div className={scrolled ? 'settings-head scrolled' : 'settings-head'}>
      <BackLine back={back} title={title} />
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
 * sidebar, such as an address or the jump field (SC4 D7): its group shows open while it is the
 * one revealed, and the sidebar scrolls without motion. Moving by the sidebar never scrolls it.
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

/** The jump field of a space or a template: Enter opens the first match, Escape empties it. */
function JumpField({
  text,
  onText,
  first,
  marked,
}: {
  text: string;
  onText: (text: string) => void;
  first: SettingsItem | undefined;
  /** With a search mark at its start and a border at rest, as in the administration. */
  marked: boolean;
}) {
  const { t } = useTranslation();
  const field = (
    <input
      type="search"
      className="settings-jump"
      value={text}
      aria-label={t('settings.jump')}
      placeholder={t('settings.jump')}
      onChange={(e) => onText(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' && text.trim() && first) {
          e.preventDefault();
          onText('');
          navigate(first.href);
        }
        if (e.key === 'Escape') onText('');
      }}
    />
  );
  if (!marked) return field;
  return (
    <div className="settings-search">
      <IconSearch size={SEARCH_SIZE} />
      {field}
    </div>
  );
}

export function SettingsFrame({
  title,
  sidebar,
  groups,
  current,
  width = 'full',
  children,
}: {
  title: string;
  sidebar: SettingsSidebar;
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
  const pinned = sidebar.kind === 'space' ? matchingItems([pinnedGroup(sidebar.pinned)], text) : [];
  const pinnedShown = pinned[0]?.items ?? [];
  const { mark, scrolled } = useScrolledBeneath(sidebar.kind === 'admin');
  const holder = groups.find((group) => group.items.some((item) => item.id === current))?.id ?? null;
  const tools = sidebar.kind === 'admin' ? sidebar.tools : [];
  const entries = [...tools, ...groups].reduce((sum, group) => sum + group.items.length, 0);
  const { aside, revealed, hide, follow } = useCurrentInView(
    sidebar.kind !== 'single',
    current,
    holder,
    entries + pinnedShown.length,
  );
  const first = [...pinnedShown, ...shown.flatMap((group) => group.items)][0];
  const jump = <JumpField text={text} onText={setText} first={first} marked={sidebar.kind === 'space'} />;
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
      {shown.length === 0 && pinnedShown.length === 0 && <p className="muted">{t('settings.nothing')}</p>}
    </>
  );
  return (
    <div className="split">
      <div className="sidebar-frame">
        <aside ref={aside} className={`sidebar settings-sidebar settings-${sidebar.kind}`}>
          {sidebar.kind === 'admin' && (
            <>
              <div ref={mark} className="settings-scroll-mark" />
              <BackRow back={sidebar.back} title={title} scrolled={scrolled} />
            </>
          )}
          {sidebar.kind === 'space' && (
            // The head stays in place, opaque, with no line under it at any time (SPC-136).
            <div className="settings-head space-head">
              <div className="space-head-back">
                <BackLine back={sidebar.back} title={title} />
              </div>
              {sidebar.head}
              {jump}
            </div>
          )}
          {sidebar.kind === 'single' && (
            <>
              {sidebar.head}
              {jump}
            </>
          )}
          <nav aria-label={title} className="settings-nav">
            {tools.map((group) => (
              <div key={group.id} className="settings-tools">
                <h2 className="settings-tools-title">{group.title}</h2>
                <div className="side-links">
                  {group.items.map((item) => (
                    <ItemLink key={item.id} item={item} current={current} onFollow={follow} />
                  ))}
                </div>
              </div>
            ))}
            {sidebar.kind === 'admin' && (
              <div className="settings-part">
                <h2 className="settings-part-title">{sidebar.settingsTitle}</h2>
                {sidebar.search}
              </div>
            )}
            {pinnedShown.length > 0 && (
              <>
                <div className="side-links">
                  {pinnedShown.map((item) => (
                    <ItemLink key={item.id} item={item} current={current} onFollow={follow} />
                  ))}
                </div>
                {shown.length > 0 && <hr className="settings-divider" />}
              </>
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

function pinnedGroup(items: SettingsItem[]): SettingsGroup {
  return { id: 'pinned', title: '', items };
}
