// SPDX-License-Identifier: Apache-2.0
// The search box in the top bar: a combobox with a list popup (spec 009, B3). The
// input keeps focus, and aria-activedescendant points at the marked row.
import { useEffect, useId, useRef, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type ListedPage, type QuickResults, type SearchResult, type Space } from '../api';
import { personalKey, usePerson } from '../personal';
import { navigate } from '../router';
import { IconSearch } from './icons';
import { FileMark, KindMark } from './kind-mark';
import { Avatar, SpaceBadge } from './ui';
import { SearchFilterBar } from './search-filter-bar';
import { NO_FILTERS, queryWith, resultsAddress, type SearchFilters } from './search-filters';
import { useDates } from '../dates';
import { appearanceQuery } from '../appearance';
import { pageTitle } from '../page-name';

const MAX_SEARCHES = 8;
const PAUSE_MS = 200;
const RECENT_PAGES_SHOWN = 5;

/** What stands before the title of a row: the mark of its kind (NAV-156). */
export type RowMark =
  | { type: 'page'; kind?: string; home?: boolean; emoji?: string }
  | { type: 'file'; filename: string }
  | { type: 'space'; space: Space }
  | { type: 'person'; name: string }
  | { type: 'none' };

export interface QuickRow {
  id: string;
  group: string;
  title: string;
  detail: string;
  run: 'open' | 'search' | 'clear';
  target: string;
  mark: RowMark;
}

const NO_MARK: RowMark = { type: 'none' };

function pageMark(p: SearchResult): RowMark {
  return { type: 'page', kind: p.kind, home: p.home, emoji: p.emoji };
}

function RowMarkView({ mark }: { mark: RowMark }) {
  switch (mark.type) {
    case 'page':
      return <KindMark item={mark} />;
    case 'file':
      return <FileMark filename={mark.filename} />;
    case 'space':
      return <SpaceBadge space={mark.space} size="small" />;
    case 'person':
      return <Avatar name={mark.name} size="small" />;
    case 'none':
      return null;
  }
}

export function readSearches(stored: string | null): string[] {
  try {
    const value: unknown = JSON.parse(stored ?? '[]');
    return Array.isArray(value) ? value.filter((s): s is string => typeof s === 'string') : [];
  } catch {
    // Nothing usable stored: no recent searches is a fine answer.
    return [];
  }
}

/** Newest first, each once. */
export function addSearch(searches: string[], text: string): string[] {
  return [text, ...searches.filter((s) => s !== text)].slice(0, MAX_SEARCHES);
}

/** What the panel shows, in the order of the arrow keys. */
export function rowsOf(
  results: QuickResults | undefined,
  labels: { pages: string; posts: string; spaces: string; files: string; people: string; untitled: string },
  day: (value: string) => string,
): QuickRow[] {
  if (!results) return [];
  const untitled = () => labels.untitled;
  return [
    ...results.pages.map((p) => ({
      id: `page-${p.page_id}`,
      group: labels.pages,
      title: pageTitle(untitled, p.title),
      detail: `${p.space_name} · ${day(p.updated_at)}`,
      run: 'open' as const,
      target: `/pages/${p.page_id}`,
      mark: pageMark(p),
    })),
    ...(results.posts ?? []).map((p) => ({
      id: `post-${p.page_id}`,
      group: labels.posts,
      title: pageTitle(untitled, p.title),
      detail: `${p.space_name} · ${day(p.updated_at)}`,
      run: 'open' as const,
      target: `/pages/${p.page_id}`,
      mark: pageMark(p),
    })),
    ...results.spaces.map((s) => ({
      id: `space-${s.id}`,
      group: labels.spaces,
      title: s.name,
      detail: s.key,
      run: 'open' as const,
      target: `/spaces/${s.key}`,
      mark: { type: 'space' as const, space: s },
    })),
    ...results.files.map((f) => ({
      id: `file-${f.id}`,
      group: labels.files,
      title: f.filename,
      detail: `${pageTitle(untitled, f.page_title)} · ${f.space_name}`,
      run: 'open' as const,
      target: `/pages/${f.page_id}`,
      mark: { type: 'file' as const, filename: f.filename },
    })),
    ...results.people.map((person) => ({
      id: `person-${person.id}`,
      group: labels.people,
      title: person.name,
      detail: '',
      run: 'search' as const,
      target: person.name,
      mark: { type: 'person' as const, name: person.name },
    })),
  ];
}

export function useDelayed(value: string): string {
  const [delayed, setDelayed] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDelayed(value), PAUSE_MS);
    return () => clearTimeout(timer);
  }, [value]);
  return delayed;
}

/** The search box. It searches every space; filters narrow it on request (spec 033). */
export function QuickSearch() {
  const { t } = useTranslation();
  const dates = useDates();
  const searchesKey = personalKey(usePerson(), 'searches');
  const listId = useId();
  const root = useRef<HTMLFormElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const [text, setText] = useState('');
  const [open, setOpen] = useState(false);
  const [marked, setMarked] = useState(-1);
  const [filters, setFilters] = useState<SearchFilters>(NO_FILTERS);
  // A name, not markup: it only ever goes into an attribute as text.
  const organisation = useQuery(appearanceQuery).data?.organisation.trim() ?? '';
  const [searches, setSearches] = useState(() => readSearches(localStorage.getItem(searchesKey)));
  const typed = useDelayed(text.trim());
  // The quick list reads the same words of the syntax as the results page.
  const query = queryWith(typed, filters);

  const recent = useQuery({
    queryKey: ['/me/recent'],
    queryFn: () => api.get<{ pages: ListedPage[] }>('/me/recent'),
    enabled: open && typed === '',
  });
  const results = useQuery({
    queryKey: ['quick', query],
    queryFn: () => api.get<QuickResults>(`/search/quick?${new URLSearchParams({ q: query }).toString()}`),
    enabled: open && typed !== '',
    // The rows stay while the next ones load. The key keeps an old answer from
    // ever replacing a newer one.
    placeholderData: (previous) => previous,
  });

  const labels = {
    pages: t('search.groupPages'),
    posts: t('search.groupPosts'),
    spaces: t('search.groupSpaces'),
    files: t('search.groupFiles'),
    people: t('search.groupPeople'),
    untitled: t('page.untitled'),
  };
  const emptyRows: QuickRow[] = [
    ...(recent.data?.pages ?? []).slice(0, RECENT_PAGES_SHOWN).map((p) => ({
      id: `page-${p.id}`,
      group: t('search.recentPages'),
      title: pageTitle(t, p.title),
      detail: p.space_name,
      run: 'open' as const,
      target: `/pages/${p.id}`,
      mark: { type: 'page' as const, emoji: p.emoji },
    })),
    ...searches.map((s) => ({
      id: `search-${s}`,
      group: t('search.recentSearches'),
      title: s,
      detail: '',
      run: 'search' as const,
      target: s,
      mark: NO_MARK,
    })),
    ...(searches.length > 0
      ? [
          {
            id: 'clear',
            group: t('search.recentSearches'),
            title: t('search.clearRecent'),
            detail: '',
            run: 'clear' as const,
            target: '',
            mark: NO_MARK,
          },
        ]
      : []),
  ];
  const allResults: QuickRow = {
    id: 'all',
    group: '',
    title: t('search.allResults', { text: text.trim() }),
    detail: '',
    run: 'search',
    target: text.trim(),
    mark: NO_MARK,
  };
  const rows = text.trim() === '' ? emptyRows : [...rowsOf(results.data, labels, dates.day), allResults];
  const groups = [...new Set(rows.map((r) => r.group))];

  const rememberAndSearch = (query: string) => {
    const next = addSearch(searches, query);
    setSearches(next);
    localStorage.setItem(searchesKey, JSON.stringify(next));
    navigate(resultsAddress(query, filters));
  };

  const close = () => {
    setOpen(false);
    setMarked(-1);
  };

  const choose = (row: QuickRow | undefined) => {
    if (row?.run === 'clear') {
      setSearches([]);
      localStorage.removeItem(searchesKey);
      return;
    }
    close();
    if (row?.run === 'open') navigate(row.target);
    else if (row) rememberAndSearch(row.target);
    else if (text.trim()) rememberAndSearch(text.trim());
    if (!row && !text.trim()) return;
    setText('');
    // The search is done: letters now belong to the page again, such as shortcuts.
    input.current?.blur();
  };

  useEffect(() => {
    if (!open) return;
    const outside = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) close();
    };
    document.addEventListener('mousedown', outside);
    return () => document.removeEventListener('mousedown', outside);
  }, [open]);

  const keys = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (!open) {
        setOpen(true);
        return;
      }
      if (rows.length === 0) return;
      const step = e.key === 'ArrowDown' ? 1 : -1;
      setMarked((marked + step + rows.length + (marked < 0 && step < 0 ? 1 : 0)) % rows.length);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      choose(rows[marked]);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      if (open) close();
      else setText('');
    }
  };

  const showPanel = open && (rows.length > 0 || results.isError);
  const markedRow = rows[marked];
  return (
    <form
      ref={root}
      role="search"
      className="top-search"
      // Leaving the box and its panel with Tab or a click closes the panel.
      onBlur={(e) => {
        if (!root.current?.contains(e.relatedTarget as Node | null)) close();
      }}
      onSubmit={(e) => {
        e.preventDefault();
        choose(rows[marked]);
      }}
    >
      <IconSearch size={18} />
      <input
        ref={input}
        id="quick-search"
        type="text"
        role="combobox"
        autoComplete="off"
        aria-label={t('nav.search')}
        aria-autocomplete="list"
        aria-expanded={showPanel}
        aria-controls={listId}
        aria-activedescendant={markedRow ? `${listId}-${markedRow.id}` : undefined}
        placeholder={organisation ? t('nav.searchIn', { name: organisation }) : t('nav.searchPlaceholder')}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setMarked(-1);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen(true)}
        onKeyDown={keys}
      />
      <span role="status" className="visually-hidden">
        {open && typed !== '' && results.data ? t('search.quickCount', { count: rows.length - 1 }) : ''}
      </span>
      <div className="quick-drop" hidden={!open}>
        <SearchFilterBar filters={filters} onChange={setFilters} />
        <div
          id={listId}
          role="listbox"
          aria-label={t('nav.search')}
          className="quick-panel"
          hidden={!showPanel}
        >
          {results.isError && <p className="quick-note">{t('search.quickFailed')}</p>}
          {groups.map((group) => (
            <div key={group} role="group" aria-label={group || t('search.allResultsGroup')}>
              {group && (
                <div className="quick-group" aria-hidden="true">
                  {group}
                </div>
              )}
              {rows.map((row, index) =>
                row.group === group ? (
                  <div
                    key={row.id}
                    id={`${listId}-${row.id}`}
                    role="option"
                    aria-selected={index === marked}
                    className={index === marked ? 'quick-row with-mark marked' : 'quick-row with-mark'}
                    // The input must keep focus, or the panel closes before the click lands.
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => choose(row)}
                  >
                    <RowMarkView mark={row.mark} />
                    <span className="quick-text">
                      <span className="quick-title">{row.title}</span>
                      {row.detail && <span className="quick-detail">{row.detail}</span>}
                    </span>
                  </div>
                ) : null,
              )}
            </div>
          ))}
        </div>
      </div>
    </form>
  );
}
