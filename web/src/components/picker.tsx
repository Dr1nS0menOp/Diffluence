// SPDX-License-Identifier: Apache-2.0
import { useId, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, ApiError, type DirectoryEntry, type SearchResult } from '../api';
import { Field } from './ui';

/**
 * Finds a user or group. A combobox with a list: people show as soon as the field
 * has focus, and focus stays in the field while the arrows mark a row (spec 018).
 */
export function SubjectPicker({
  onPick,
  label,
  usersOnly,
}: {
  onPick: (e: DirectoryEntry) => void;
  label: string;
  usersOnly?: boolean;
}) {
  const { t } = useTranslation();
  const listId = useId();
  const [text, setText] = useState('');
  const [open, setOpen] = useState(false);
  const [marked, setMarked] = useState(-1);
  const q = useQuery({
    queryKey: ['directory', text],
    queryFn: () => api.get<{ entries: DirectoryEntry[] }>('/directory?q=' + encodeURIComponent(text)),
    enabled: open,
    placeholderData: (previous) => previous,
  });
  const entries = (q.data?.entries ?? []).filter((e) => !usersOnly || e.type === 'user');
  const refused = q.error instanceof ApiError && q.error.status === 403;
  const showList = open && !refused;

  const close = () => {
    setOpen(false);
    setMarked(-1);
  };
  const pick = (entry: DirectoryEntry) => {
    onPick(entry);
    setText('');
    close();
  };
  const keys = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (!open) {
        setOpen(true);
      } else if (entries.length > 0) {
        const step = e.key === 'ArrowDown' ? 1 : -1;
        const from = marked < 0 && step < 0 ? 0 : marked;
        setMarked((from + step + entries.length) % entries.length);
      }
    } else if (e.key === 'Enter') {
      // A picker sits in forms, and Enter here must not send them.
      e.preventDefault();
      const entry = entries[marked];
      if (open && entry) pick(entry);
    } else if (e.key === 'Escape' && open) {
      // The list closes first, and a second Escape is for the dialog around it.
      e.preventDefault();
      e.stopPropagation();
      close();
    }
  };

  const markedEntry = entries[marked];
  return (
    <div className="picker">
      <input
        type="text"
        role="combobox"
        autoComplete="off"
        value={text}
        placeholder={label}
        aria-label={label}
        aria-autocomplete="list"
        aria-expanded={showList}
        aria-controls={listId}
        aria-activedescendant={markedEntry ? `${listId}-${markedEntry.id}` : undefined}
        onChange={(e) => {
          setText(e.target.value);
          setMarked(-1);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen(true)}
        onBlur={close}
        onKeyDown={keys}
      />
      {refused && <p className="muted">{q.error?.message}</p>}
      <div id={listId} role="listbox" aria-label={label} className="picker-list" hidden={!showList}>
        {entries.map((entry, index) => (
          <div
            key={entry.type + entry.id}
            id={`${listId}-${entry.id}`}
            role="option"
            aria-selected={index === marked}
            className={index === marked ? 'quick-row marked' : 'quick-row'}
            // The field must keep focus, or the list closes before the click lands.
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => pick(entry)}
          >
            <span className="quick-title">{entry.name}</span>
            {entry.detail && <span className="quick-detail">{entry.detail}</span>}
          </div>
        ))}
        {q.isSuccess && entries.length === 0 && <p className="quick-note muted">{t('search.noResults')}</p>}
      </div>
    </div>
  );
}

const MAX_PAGE_RESULTS = 8;

/** Finds a page by its title. The value is its id, and empty means none. */
export function PagePicker({
  label,
  help,
  value,
  onChange,
}: {
  label: string;
  help: string;
  value: string;
  onChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const chosen = useQuery({
    queryKey: ['picked-page', value],
    queryFn: () => api.get<{ title: string }>(`/pages/${value}`),
    enabled: value !== '',
    retry: false,
  });
  const found = useQuery({
    queryKey: ['page-picker', search],
    queryFn: () =>
      api.get<{ results: SearchResult[] }>(
        `/search?q=${encodeURIComponent(search)}&limit=${MAX_PAGE_RESULTS}`,
      ),
    enabled: search.trim().length > 0,
  });
  if (value) {
    return (
      <div role="group" aria-label={label}>
        <strong>{label}</strong>
        <div className="row">
          <span>{chosen.data?.title ?? t('picker.pageGone')}</span>
          <button type="button" className="ghost" onClick={() => onChange('')}>
            {t('picker.changePage')}
          </button>
        </div>
      </div>
    );
  }
  return (
    <>
      <Field label={label} help={help}>
        <input type="search" value={search} onChange={(e) => setSearch(e.target.value)} />
      </Field>
      <ul className="plain-list">
        {found.data?.results.map((result) => (
          <li key={result.page_id}>
            <button
              type="button"
              className="ghost"
              onClick={() => {
                onChange(result.page_id);
                setSearch('');
              }}
            >
              {result.title}
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}
