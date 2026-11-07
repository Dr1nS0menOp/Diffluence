// SPDX-License-Identifier: Apache-2.0
// Single-key shortcuts outside the editor (spec 009, B14). WCAG 2.1.4 asks for a
// way to turn them off, which is the "shortcuts" preference.
import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { navigate } from '../router';
import { Dialog } from './ui';

const SEQUENCE_MS = 1000;
const TYPING_ROLES = ['textbox', 'combobox', 'searchbox'];

export type ShortcutAction =
  | 'create'
  | 'edit'
  | 'favourite'
  | 'sidebar'
  | 'reader'
  | 'details'
  | 'watch'
  | 'comment'
  | 'nextComment'
  | 'previousComment'
  | 'notifications';

interface Shortcut {
  keys: string[];
  label: string;
  group: 'global' | 'page' | 'editor';
}

export const SHORTCUTS: Shortcut[] = [
  { keys: ['/'], label: 'search', group: 'global' },
  { keys: ['?'], label: 'overview', group: 'global' },
  { keys: ['g', 'h'], label: 'home', group: 'global' },
  { keys: ['g', 's'], label: 'spaces', group: 'global' },
  { keys: ['g', 'r'], label: 'recent', group: 'global' },
  { keys: ['g', 'f'], label: 'favourites', group: 'global' },
  { keys: ['g', 'n'], label: 'notifications', group: 'global' },
  { keys: ['c'], label: 'create', group: 'global' },
  { keys: ['['], label: 'sidebar', group: 'page' },
  { keys: ['e'], label: 'edit', group: 'page' },
  { keys: ['s'], label: 'favourite', group: 'page' },
  { keys: ['i'], label: 'details', group: 'page' },
  { keys: ['r'], label: 'reader', group: 'page' },
  { keys: ['w'], label: 'watch', group: 'page' },
  { keys: ['m'], label: 'comment', group: 'page' },
  { keys: ['n'], label: 'nextComment', group: 'page' },
  { keys: ['p'], label: 'previousComment', group: 'page' },
];

const SINGLE_KEYS: Record<string, ShortcutAction> = {
  c: 'create',
  e: 'edit',
  s: 'favourite',
  '[': 'sidebar',
  r: 'reader',
  i: 'details',
  w: 'watch',
  m: 'comment',
  n: 'nextComment',
  p: 'previousComment',
};

const AFTER_G: Record<string, string> = { h: '/', s: '/spaces', r: '/recent', f: '/favourites' };

/** While someone types, a letter is a letter. */
export function isTyping(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  if (target.isContentEditable) return true;
  if (['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return true;
  return TYPING_ROLES.includes(target.getAttribute('role') ?? '');
}

type Handlers = Map<ShortcutAction, () => void>;

const ShortcutContext = createContext<Handlers>(new Map());

/** Lets a screen answer a shortcut while it is on screen. */
export function useShortcut(action: ShortcutAction, handler: () => void) {
  const handlers = useContext(ShortcutContext);
  const latest = useRef(handler);
  latest.current = handler;
  useEffect(() => {
    const run = () => latest.current();
    handlers.set(action, run);
    return () => {
      if (handlers.get(action) === run) handlers.delete(action);
    };
  }, [action, handlers]);
}

function focusSearch() {
  document.getElementById('quick-search')?.focus();
}

export function ShortcutsProvider({ enabled, children }: { enabled: boolean; children: ReactNode }) {
  const handlers = useMemo<Handlers>(() => new Map(), []);
  const [overview, setOverview] = useState(false);

  useEffect(() => {
    if (!enabled) return;
    let waitingSince = 0;
    const onKey = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey || e.altKey || isTyping(e.target)) return;
      if (document.querySelector('dialog[open]')) return;
      const inSequence = Date.now() - waitingSince < SEQUENCE_MS;
      waitingSince = 0;
      const place = inSequence ? AFTER_G[e.key] : undefined;
      if (place) {
        e.preventDefault();
        navigate(place);
      } else if (inSequence && e.key === 'n') {
        e.preventDefault();
        handlers.get('notifications')?.();
      } else if (e.key === 'g') {
        waitingSince = Date.now();
      } else if (e.key === '/') {
        e.preventDefault();
        focusSearch();
      } else if (e.key === '?') {
        e.preventDefault();
        setOverview(true);
      } else {
        const action = SINGLE_KEYS[e.key];
        const run = action && handlers.get(action);
        if (!run) return;
        e.preventDefault();
        run();
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [enabled, handlers]);

  return (
    <ShortcutContext.Provider value={handlers}>
      {children}
      {overview && <ShortcutsDialog onClose={() => setOverview(false)} />}
    </ShortcutContext.Provider>
  );
}

// Only for this overview: the editor handles these keys itself (keys.ts).
const EDITOR_SHORTCUTS: Shortcut[] = [
  { keys: ['Ctrl', 'K'], label: 'editorLink', group: 'editor' },
  { keys: ['Ctrl', 'S'], label: 'editorSave', group: 'editor' },
  { keys: ['Ctrl', 'Enter'], label: 'editorPublish', group: 'editor' },
  { keys: ['/'], label: 'editorInsert', group: 'editor' },
  { keys: ['Ctrl', 'D'], label: 'editorDuplicate', group: 'editor' },
  { keys: ['Alt', 'Up', 'Down'], label: 'editorMove', group: 'editor' },
  { keys: ['Ctrl', 'Shift', '.'], label: 'editorBlockMenu', group: 'editor' },
  { keys: ['Ctrl', '\\'], label: 'editorClear', group: 'editor' },
  { keys: ['Alt', 'F10'], label: 'editorTools', group: 'editor' },
  { keys: ['Tab'], label: 'editorNextCell', group: 'editor' },
  { keys: ['Shift', 'Tab'], label: 'editorPreviousCell', group: 'editor' },
];

export function ShortcutsDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState('');
  const shown = [...SHORTCUTS, ...EDITOR_SHORTCUTS].filter((s) =>
    t(`shortcuts.${s.label}`).toLowerCase().includes(filter.trim().toLowerCase()),
  );
  return (
    <Dialog title={t('shortcuts.title')} onClose={onClose}>
      <input
        type="search"
        className="dialog-filter"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        aria-label={t('shortcuts.filter')}
        placeholder={t('shortcuts.filter')}
      />
      {shown.length === 0 && <p className="muted">{t('shortcuts.none')}</p>}
      {(['global', 'page', 'editor'] as const).map((group) => {
        const rows = shown.filter((s) => s.group === group);
        if (rows.length === 0) return null;
        return (
          <section key={group}>
            <h3>{t(`shortcuts.group_${group}`)}</h3>
            <dl className="shortcut-list">
              {rows.map((s) => (
                <div key={s.label}>
                  <dt>{t(`shortcuts.${s.label}`)}</dt>
                  <dd>
                    {s.keys.map((key) => (
                      <kbd key={key}>{key}</kbd>
                    ))}
                  </dd>
                </div>
              ))}
            </dl>
          </section>
        );
      })}
      <p className="muted">{t('shortcuts.turnOff')}</p>
    </Dialog>
  );
}
