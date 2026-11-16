// SPDX-License-Identifier: Apache-2.0
// The shortcuts of a space: links its administrators choose (SPC-025). The same form is a
// section of the space settings and a dialog from the sidebar (spec 078, SPC-100).
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Shortcut, type Space } from '../api';
import { moved } from '../views/lists';
import { IconArrowDown, IconArrowUp } from './icons';
import { ErrorNote } from './ui';

const MOVE_ICON_SIZE = 16;

export const shortcutsQuery = (space: Pick<Space, 'id'>) => ({
  queryKey: ['shortcuts', space.id],
  queryFn: () => api.get<{ shortcuts: Shortcut[] }>(`/spaces/${space.id}/shortcuts`),
});

/** The list with its order, and the form that adds one. */
export function ShortcutsForm({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const query = shortcutsQuery(space);
  const q = useQuery(query);
  const [label, setLabel] = useState('');
  const [target, setTarget] = useState('');
  const save = useMutation({
    mutationFn: (shortcuts: Shortcut[]) =>
      api.put(`/spaces/${space.id}/shortcuts`, {
        shortcuts: shortcuts.map((s) => ({ label: s.label, target: s.target })),
      }),
    onSuccess: () => {
      setLabel('');
      setTarget('');
    },
    onSettled: () => client.invalidateQueries({ queryKey: query.queryKey }),
  });
  const list = q.data?.shortcuts ?? [];
  return (
    <>
      <p className="muted">{t('shortcutLinks.help')}</p>
      <ErrorNote error={q.error ?? save.error} />
      <ul className="page-list">
        {list.map((s, at) => (
          <li key={s.id ?? s.label}>
            <span className="grow">{s.label}</span>
            <span className="muted">{s.target}</span>
            <button
              type="button"
              className="icon"
              aria-label={t('favourites.moveUp', { title: s.label })}
              disabled={at === 0}
              onClick={() => save.mutate(moved(list, at, -1))}
            >
              <IconArrowUp size={MOVE_ICON_SIZE} />
            </button>
            <button
              type="button"
              className="icon"
              aria-label={t('favourites.moveDown', { title: s.label })}
              disabled={at === list.length - 1}
              onClick={() => save.mutate(moved(list, at, 1))}
            >
              <IconArrowDown size={MOVE_ICON_SIZE} />
            </button>
            <button
              type="button"
              className="ghost"
              onClick={() => save.mutate(list.filter((_, i) => i !== at))}
            >
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate([...list, { label, target }]);
        }}
      >
        <input
          value={label}
          aria-label={t('shortcutLinks.label')}
          placeholder={t('shortcutLinks.label')}
          required
          onChange={(e) => setLabel(e.target.value)}
        />
        <input
          className="grow"
          value={target}
          aria-label={t('shortcutLinks.target')}
          placeholder={t('shortcutLinks.target')}
          required
          onChange={(e) => setTarget(e.target.value)}
        />
        <button type="submit">{t('common.add')}</button>
      </form>
    </>
  );
}
