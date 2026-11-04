// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type ListedPage, type Me, type Space } from '../api';
import { Link, navigate } from '../router';
import { Dialog, ErrorNote, Field, Loading } from '../components/ui';

function PageList({ title, path }: { title: string; path: string }) {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: [path], queryFn: () => api.get<{ pages: ListedPage[] }>(path) });
  return (
    <section>
      <h2>{title}</h2>
      {q.isPending && <Loading />}
      {q.data?.pages.length === 0 && <p className="muted">{t('home.nothingYet')}</p>}
      <ul className="plain-list">
        {q.data?.pages.map((p) => (
          <li key={p.id}>
            <Link href={`/pages/${p.id}`}>{p.title}</Link> <span className="muted">{p.space_name}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function NewSpace({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [form, setForm] = useState({ key: '', name: '', description: '', icon: '' });
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) =>
    setForm({ ...form, [k]: e.target.value });
  const create = useMutation({
    mutationFn: () => api.post<Space>('/spaces', form),
    onSuccess: async (s) => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      navigate(`/spaces/${s.key}`);
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    create.mutate();
  };
  return (
    <Dialog title={t('home.newSpace')} onClose={onClose}>
      <form onSubmit={submit}>
        <ErrorNote error={create.error} />
        <Field label={t('common.name')}>
          <input value={form.name} onChange={set('name')} required autoFocus />
        </Field>
        <Field label={t('space.key')} help={t('space.keyHelp')}>
          <input value={form.key} onChange={set('key')} required pattern="[A-Za-z0-9][A-Za-z0-9_\-]{1,31}" />
        </Field>
        <Field label={t('space.description')}>
          <textarea value={form.description} onChange={set('description')} rows={2} />
        </Field>
        <Field label={t('space.icon')}>
          <input value={form.icon} onChange={set('icon')} maxLength={8} />
        </Field>
        <button type="submit" className="primary" disabled={create.isPending}>
          {t('common.create')}
        </button>
      </form>
    </Dialog>
  );
}

export function HomeView({ me }: { me: Me }) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  return (
    <>
      <div className="row">
        <h1>{t('home.spaces')}</h1>
        {me.global_permissions.includes('create_space') && (
          <button type="button" className="primary" onClick={() => setAdding(true)}>
            {t('home.newSpace')}
          </button>
        )}
      </div>
      {spaces.isPending && <Loading />}
      <ErrorNote error={spaces.error} />
      {spaces.data?.spaces.length === 0 && <p className="muted">{t('home.noSpaces')}</p>}
      <ul className="cards">
        {spaces.data?.spaces.map((s) => (
          <li key={s.id}>
            <Link href={`/spaces/${s.key}`}>
              <span aria-hidden="true">{s.icon}</span> {s.name}
            </Link>
            <p className="muted">{s.description}</p>
          </li>
        ))}
      </ul>
      <div className="columns">
        <PageList title={t('home.favourites')} path="/me/favourites" />
        <PageList title={t('home.recent')} path="/me/recent" />
      </div>
      {adding && <NewSpace onClose={() => setAdding(false)} />}
    </>
  );
}
