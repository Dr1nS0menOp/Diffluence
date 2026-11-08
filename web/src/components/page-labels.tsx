// SPDX-License-Identifier: Apache-2.0
// Labels at the bottom of a page. The box is a combobox: focus stays in the input,
// and it keeps open after adding, so several labels go in quickly.
import { useId, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type LabelCount, type PageSummary } from '../api';
import { Link } from '../router';
import { IconClose } from './icons';
import { ErrorNote } from './ui';

const MAX_SUGGESTIONS = 8;
const REMOVE_ICON_SIZE = 12;

/** Writes a label the one allowed way while typing. */
export function cleanLabel(text: string): string {
  return text.toLowerCase().replace(/\s+/g, '-');
}

export function LabelBar({ page }: { page: PageSummary & { labels?: string[] } }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const listId = useId();
  const [text, setText] = useState('');
  const [marked, setMarked] = useState(-1);
  const editable = (page.can['page.edit'] ?? false) && !page.archived_at;
  const current = useQuery({
    queryKey: ['page-labels', page.id],
    queryFn: () => api.get<{ labels: string[] }>(`/pages/${page.id}/labels`),
    initialData: { labels: page.labels ?? [] },
  });
  const labels = current.data.labels;
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['page-labels', page.id] });
    await client.invalidateQueries({ queryKey: ['page', page.id] });
  };
  const suggestions = useQuery({
    queryKey: ['labels', page.space_id, text],
    queryFn: () =>
      api.get<{ labels: LabelCount[] }>(`/labels?q=${encodeURIComponent(text)}&space_id=${page.space_id}`),
    enabled: editable && text !== '',
  });
  const add = useMutation({
    mutationFn: (label: string) => api.post(`/pages/${page.id}/labels`, { label }),
    onSuccess: () => {
      setText('');
      setMarked(-1);
    },
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: (label: string) => api.del(`/pages/${page.id}/labels/${encodeURIComponent(label)}`),
    onSettled: refresh,
  });
  const rows = (suggestions.data?.labels ?? [])
    .filter((l) => !labels.includes(l.label))
    .slice(0, MAX_SUGGESTIONS);
  const open = text !== '' && rows.length > 0;

  const keys = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      if (rows.length === 0) return;
      setMarked((marked + (e.key === 'ArrowDown' ? 1 : rows.length - 1) + rows.length) % rows.length);
    } else if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      const chosen = rows[marked]?.label ?? text.trim();
      if (chosen) add.mutate(chosen);
    } else if (e.key === 'Escape') {
      setText('');
      setMarked(-1);
    }
  };

  if (!editable && labels.length === 0) return null;
  return (
    <section className="label-bar" aria-label={t('organise.labels')}>
      <ul className="label-list">
        {labels.map((label) => (
          <li key={label}>
            <Link href={`/labels/${encodeURIComponent(label)}`} className="label">
              <span className="label-mark" aria-hidden="true">
                #
              </span>
              {label}
            </Link>
            {editable && (
              <button
                type="button"
                className="label-remove"
                aria-label={t('organise.removeLabel', { label })}
                onClick={() => remove.mutate(label)}
              >
                <IconClose size={REMOVE_ICON_SIZE} />
              </button>
            )}
          </li>
        ))}
      </ul>
      {editable && (
        <div className="label-box">
          <input
            role="combobox"
            className="label-input"
            aria-label={t('organise.addLabel')}
            aria-autocomplete="list"
            aria-expanded={open}
            aria-controls={listId}
            aria-activedescendant={rows[marked] ? `${listId}-${marked}` : undefined}
            placeholder={t('organise.addLabel')}
            value={text}
            onChange={(e) => {
              setText(cleanLabel(e.target.value));
              setMarked(-1);
            }}
            onKeyDown={keys}
          />
          <ul
            id={listId}
            role="listbox"
            aria-label={t('organise.labels')}
            className="label-options"
            hidden={!open}
          >
            {rows.map((row, index) => (
              <li
                key={row.label}
                id={`${listId}-${index}`}
                role="option"
                aria-selected={index === marked}
                className={index === marked ? 'quick-row marked' : 'quick-row'}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => add.mutate(row.label)}
              >
                {row.label}
              </li>
            ))}
          </ul>
        </div>
      )}
      <ErrorNote error={add.error ?? remove.error} />
    </section>
  );
}
