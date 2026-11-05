// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Page, type Space } from '../api';
import { navigate } from '../router';
import { Dialog, ErrorNote, Field } from './ui';

const SUGGESTED_KEY_LENGTH = 12;
const VALID_KEY = /^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$/;

/** A key from a name: its letters and digits, in capitals. */
export function suggestKey(name: string): string {
  const letters = name.normalize('NFKD').replace(/[^A-Za-z0-9]/g, '');
  return letters.slice(0, SUGGESTED_KEY_LENGTH).toUpperCase();
}

/** Where the Create button in the top bar puts a new page. */
export interface CreateTarget {
  space: Space;
  parent: string | null;
}

export function NewPageDialog({
  space,
  parent,
  onClose,
}: {
  space: Space;
  parent: string | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [title, setTitle] = useState('');
  const create = useMutation({
    mutationFn: () => api.post<Page>('/pages', { space_id: space.id, parent_id: parent, title }),
    onSuccess: (p) => {
      onClose();
      navigate(`/pages/${p.id}/edit`);
    },
  });
  return (
    <Dialog title={t('space.newPage')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('page.title')}>
          <input value={title} onChange={(e) => setTitle(e.target.value)} required autoFocus />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={create.isPending}>
            {t('common.create')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

export function NewSpaceDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [form, setForm] = useState({ key: '', name: '', description: '', icon: '' });
  const [keyTouched, setKeyTouched] = useState(false);
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) =>
    setForm({ ...form, [k]: e.target.value });
  const free = useQuery({
    queryKey: ['key-check', form.key],
    queryFn: () => api.get<{ free: boolean }>(`/space-key-check?key=${encodeURIComponent(form.key)}`),
    enabled: VALID_KEY.test(form.key),
  });
  const create = useMutation({
    mutationFn: () => api.post<Space>('/spaces', form),
    onSuccess: async (s) => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      onClose();
      navigate(`/spaces/${s.key}`);
    },
  });
  return (
    <Dialog title={t('home.newSpace')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('common.name')}>
          <input
            value={form.name}
            onChange={(e) =>
              setForm({
                ...form,
                name: e.target.value,
                key: keyTouched ? form.key : suggestKey(e.target.value),
              })
            }
            required
            autoFocus
          />
        </Field>
        <Field label={t('space.key')} help={t('space.keyHelp')}>
          <input
            value={form.key}
            onChange={(e) => {
              setKeyTouched(true);
              set('key')(e);
            }}
            required
            pattern="[A-Za-z0-9][A-Za-z0-9_\-]{1,31}"
          />
        </Field>
        {free.data?.free === false && (
          <p role="alert" className="note note-error">
            {t('space.keyTaken')}
          </p>
        )}
        <Field label={t('space.description')}>
          <textarea value={form.description} onChange={set('description')} rows={2} />
        </Field>
        <Field label={t('space.icon')}>
          <input value={form.icon} onChange={set('icon')} maxLength={8} />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary" disabled={create.isPending}>
            {t('common.create')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
