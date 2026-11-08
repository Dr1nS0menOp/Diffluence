// SPDX-License-Identifier: Apache-2.0
// Settings of a space or of the instance: the sections in the sidebar, in groups that
// fold, with a way to jump to one; the large area beside it changes them (ADR-034).
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from '../router';
import { IconChevronDown } from './icons';

export interface SettingsItem {
  id: string;
  label: string;
  href: string;
}

export interface SettingsGroup {
  id: string;
  title: string;
  items: SettingsItem[];
}

const CHEVRON_SIZE = 14;

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

export function SettingsFrame({
  title,
  head,
  search,
  groups,
  current,
  children,
}: {
  title: string;
  /** What sits above the sections, such as the way back to a space. */
  head?: ReactNode;
  /** A search of the caller's own; without one, typing narrows the sections. */
  search?: ReactNode;
  groups: SettingsGroup[];
  current: string;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [shut, setShut] = useState<string[]>([]);
  const shown = matchingItems(groups, text);
  const toggle = (id: string) => setShut(shut.includes(id) ? shut.filter((g) => g !== id) : [...shut, id]);
  return (
    <div className="split">
      <div className="sidebar-frame">
        <aside className="sidebar settings-sidebar">
          {head}
          {search ?? (
            <input
              type="search"
              className="settings-jump"
              value={text}
              aria-label={t('settings.jump')}
              placeholder={t('settings.jump')}
              onChange={(e) => setText(e.target.value)}
            />
          )}
          <nav aria-label={title} className="settings-nav">
            {shown.map((group) => {
              // A search shows every group it found something in, folded or not.
              const open = text.trim() !== '' || !shut.includes(group.id);
              const listId = `settings-group-${group.id}`;
              return (
                <div key={group.id} className="settings-group">
                  <button
                    type="button"
                    className="sidebar-part-toggle"
                    aria-expanded={open}
                    aria-controls={listId}
                    onClick={() => toggle(group.id)}
                  >
                    <span className={open ? 'part-chevron' : 'part-chevron shut'}>
                      <IconChevronDown size={CHEVRON_SIZE} />
                    </span>
                    {group.title}
                  </button>
                  {open && (
                    <div id={listId} className="side-links">
                      {group.items.map((item) => (
                        <Link
                          key={item.id}
                          href={item.href}
                          className="side-link"
                          aria-current={item.id === current ? 'page' : undefined}
                        >
                          {item.label}
                        </Link>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
            {shown.length === 0 && <p className="muted">{t('settings.nothing')}</p>}
          </nav>
        </aside>
      </div>
      <main id="main" className="content">
        <div className="column wide settings-area">{children}</div>
      </main>
    </div>
  );
}
