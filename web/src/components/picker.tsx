// SPDX-License-Identifier: Apache-2.0
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, ApiError, type DirectoryEntry } from '../api';

/** Finds a user or group. */
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
  const [text, setText] = useState('');
  const q = useQuery({
    queryKey: ['directory', text],
    queryFn: () => api.get<{ entries: DirectoryEntry[] }>('/directory?q=' + encodeURIComponent(text)),
    enabled: text.length > 0,
  });
  const entries = (q.data?.entries ?? []).filter((e) => !usersOnly || e.type === 'user');
  return (
    <div className="picker">
      <input
        type="search"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={label}
        aria-label={label}
      />
      {q.error instanceof ApiError && q.error.status === 403 && <p className="muted">{q.error.message}</p>}
      {text.length > 0 && (
        <ul className="plain-list">
          {entries.map((e) => (
            <li key={e.type + e.id}>
              <button
                type="button"
                className="ghost"
                onClick={() => {
                  onPick(e);
                  setText('');
                }}
              >
                {e.name} <span className="muted">{e.detail}</span>
              </button>
            </li>
          ))}
          {q.isSuccess && entries.length === 0 && <li className="muted">{t('search.noResults')}</li>}
        </ul>
      )}
    </div>
  );
}
