// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  NIL_ID,
  type Grant,
  type Job,
  type Shortcut,
  type Space,
  type Subject,
  type TrashItem,
} from '../api';
import { navigate } from '../router';
import { AnimalChoice } from '../components/animal-choice';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { useDelayed } from '../components/quick-search';
import { IconArrowDown, IconArrowUp } from '../components/icons';
import { ConfirmButton, Dialog, ErrorNote, Field, FilePicker, formatDate, Loading } from '../components/ui';
import { SubjectPicker } from '../components/picker';
import { moved } from './lists';
import {
  DeleteSpaceButton,
  FeaturesSection,
  KeySection,
  LabelsSection,
  PeopleSection,
  RulesSection,
  SpaceLinksSection,
  StatusesSection,
} from './space-organise';

function useSpaceByKey(key: string) {
  return useQuery(spaceByKeyQuery(key));
}

/** Opening a space goes to its home page. */
export function SpaceHome({ spaceKey }: { spaceKey: string }) {
  const space = useSpaceByKey(spaceKey);
  useEffect(() => {
    if (space.data?.home_page_id) navigate(`/pages/${space.data.home_page_id}`, true);
  }, [space.data]);
  if (space.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={space.error} />
      </main>
    );
  }
  return <Loading />;
}

function ImportBox({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [job, setJob] = useState<string | null>(null);
  const start = useMutation({
    mutationFn: (f: File) =>
      api.upload<{ job_id: string }>(`/spaces/${space.id}/imports?filename=${encodeURIComponent(f.name)}`, f),
    onSuccess: (r) => setJob(r.job_id),
  });
  const status = useQuery({
    queryKey: ['job', job],
    queryFn: () => api.get<Job>(`/jobs/${job}`),
    enabled: !!job,
    refetchInterval: (q) => (['done', 'failed'].includes(q.state.data?.status ?? '') ? false : 1000),
  });
  useEffect(() => {
    if (status.data?.status === 'done') void client.invalidateQueries({ queryKey: ['tree', space.id] });
  }, [status.data?.status, client, space.id]);
  const r = status.data?.result;
  return (
    <section>
      <h2>{t('space.import')}</h2>
      <p className="muted">{t('space.importHelp')}</p>
      <FilePicker label={t('space.import')} accept=".md,.markdown,.zip" onPick={(f) => start.mutate(f)} />
      <ErrorNote error={start.error} />
      {job && status.data?.status !== 'done' && status.data?.status !== 'failed' && (
        <p role="status">{t('space.importing')}</p>
      )}
      {status.data?.status === 'done' && (
        <p role="status" className="note">
          {t('space.importDone', { pages: r?.pages ?? 0, files: r?.files ?? 0 })}
        </p>
      )}
      {status.data?.status === 'failed' && (
        <p role="alert" className="note note-error">
          {t('space.importFailed', { error: r?.error ?? '' })}
        </p>
      )}
      {(r?.warnings?.length ?? 0) > 0 && (
        <ul className="muted">
          {r?.warnings?.map((w) => (
            <li key={w}>{w}</li>
          ))}
        </ul>
      )}
    </section>
  );
}

const TEMPLATES = ['viewer', 'editor', 'space_admin'];

function templateOf(perms: string[], templates: Record<string, string[]>): string {
  const sorted = [...perms].sort().join();
  return TEMPLATES.find((n) => [...(templates[n] ?? [])].sort().join() === sorted) ?? 'custom';
}

function PermissionsSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['grants', space.id],
    queryFn: () =>
      api.get<{ grants: Grant[]; all_permissions: string[]; templates: Record<string, string[]> }>(
        `/spaces/${space.id}/grants`,
      ),
  });
  const [detail, setDetail] = useState<Grant | null>(null);
  const refresh = () => client.invalidateQueries({ queryKey: ['grants', space.id] });
  const set = useMutation({
    mutationFn: (v: { subject: Subject; permissions: string[] }) => api.put(`/spaces/${space.id}/grants`, v),
    onSettled: refresh,
  });
  const preset = useMutation({
    mutationFn: (name: string) => api.post(`/spaces/${space.id}/preset`, { preset: name }),
    onSettled: refresh,
  });
  if (!q.data) return <Loading />;
  const { grants, all_permissions: all, templates } = q.data;
  const label = (g: Grant) =>
    g.subject.type === 'all_members'
      ? t('permissions.allMembers')
      : g.subject.type === 'all_guests'
        ? t('permissions.allGuests')
        : g.name;
  return (
    <section>
      <h2>{t('space.permissions')}</h2>
      <ErrorNote error={set.error ?? preset.error} />
      <table>
        <thead>
          <tr>
            <th>{t('permissions.subject')}</th>
            <th>{t('permissions.template')}</th>
            <th>{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {grants.map((g) => (
            <tr key={g.subject.type + g.subject.id}>
              <td>{label(g)}</td>
              <td>
                <select
                  aria-label={t('permissions.template')}
                  value={templateOf(g.permissions, templates)}
                  onChange={(e) =>
                    set.mutate({ subject: g.subject, permissions: templates[e.target.value] ?? [] })
                  }
                >
                  {TEMPLATES.map((n) => (
                    <option key={n} value={n}>
                      {t(`permissions.${n}`)}
                    </option>
                  ))}
                  <option value="custom" disabled>
                    {t('permissions.custom')}
                  </option>
                </select>
              </td>
              <td className="row">
                <button type="button" className="ghost" onClick={() => setDetail(g)}>
                  {t('permissions.matrix')}
                </button>
                <button
                  type="button"
                  className="ghost"
                  onClick={() => set.mutate({ subject: g.subject, permissions: [] })}
                >
                  {t('permissions.removeAccess')}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <h3>{t('permissions.addSubject')}</h3>
      <div className="row">
        <SubjectPicker
          label={t('permissions.addSubject')}
          onPick={(e) =>
            set.mutate({ subject: { type: e.type, id: e.id }, permissions: templates.viewer ?? [] })
          }
        />
        <button
          type="button"
          onClick={() =>
            set.mutate({ subject: { type: 'all_members', id: NIL_ID }, permissions: templates.viewer ?? [] })
          }
        >
          {t('permissions.allMembers')}
        </button>
        <button
          type="button"
          onClick={() =>
            set.mutate({ subject: { type: 'all_guests', id: NIL_ID }, permissions: templates.viewer ?? [] })
          }
        >
          {t('permissions.allGuests')}
        </button>
      </div>
      <h3>{t('permissions.preset')}</h3>
      <div className="row">
        <button type="button" onClick={() => preset.mutate('team_space')}>
          {t('permissions.team_space')}
        </button>
        <button type="button" onClick={() => preset.mutate('read_only_kb')}>
          {t('permissions.read_only_kb')}
        </button>
      </div>
      {detail && (
        <Dialog title={label(detail)} onClose={() => setDetail(null)}>
          <fieldset className="matrix">
            <legend>{t('permissions.matrix')}</legend>
            {all.map((p) => (
              <label key={p}>
                <input
                  type="checkbox"
                  checked={detail.permissions.includes(p)}
                  onChange={(e) => {
                    const next = e.target.checked
                      ? [...detail.permissions, p]
                      : detail.permissions.filter((x) => x !== p);
                    setDetail({ ...detail, permissions: next });
                    set.mutate({ subject: detail.subject, permissions: next });
                  }}
                />
                {t(`permissions.${p.replace('.', '_')}`)}
              </label>
            ))}
          </fieldset>
        </Dialog>
      )}
    </section>
  );
}

function ShortcutsSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['shortcuts', space.id];
  const q = useQuery({
    queryKey: key,
    queryFn: () => api.get<{ shortcuts: Shortcut[] }>(`/spaces/${space.id}/shortcuts`),
  });
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
    onSettled: () => client.invalidateQueries({ queryKey: key }),
  });
  const list = q.data?.shortcuts ?? [];
  return (
    <section>
      <h2>{t('shortcutLinks.title')}</h2>
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
              <IconArrowUp size={16} />
            </button>
            <button
              type="button"
              className="icon"
              aria-label={t('favourites.moveDown', { title: s.label })}
              disabled={at === list.length - 1}
              onClick={() => save.mutate(moved(list, at, 1))}
            >
              <IconArrowDown size={16} />
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
    </section>
  );
}

export function SpaceSettingsView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useSpaceByKey(spaceKey);
  const [form, setForm] = useState<{ name: string; description: string; icon: string } | null>(null);
  const space = q.data;
  useEffect(() => {
    if (space) setForm({ name: space.name, description: space.description, icon: space.icon });
  }, [space]);
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['space', spaceKey] });
    await client.invalidateQueries({ queryKey: ['space-id', space?.id] });
    await client.invalidateQueries({ queryKey: ['spaces'] });
    await client.invalidateQueries({ queryKey: ['my-spaces'] });
  };
  const save = useMutation({ mutationFn: () => api.patch(`/spaces/${space?.id}`, form), onSuccess: refresh });
  const archive = useMutation({
    mutationFn: (to: string) => api.post(`/spaces/${space?.id}/${to}`),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: () => api.del(`/spaces/${space?.id}`),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      navigate('/');
    },
  });
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!space || !form) return <Loading />;
  return (
    <Frame space={space}>
      <h1>{t('space.settings')}</h1>
      <section>
        <h2>{t('space.details')}</h2>
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <ErrorNote error={save.error} />
          <Field label={t('common.name')}>
            <input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          </Field>
          <Field label={t('space.description')}>
            <textarea
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              rows={2}
            />
          </Field>
          <AnimalChoice
            spaceKey={space.key}
            name={form.name}
            value={form.icon}
            onChange={(icon) => setForm({ ...form, icon })}
          />
          <button type="submit" className="primary">
            {t('common.save')}
          </button>
        </form>
      </section>
      <SpaceLinksSection space={space} />
      <KeySection space={space} />
      <PeopleSection space={space} />
      <FeaturesSection space={space} />
      <StatusesSection space={space} />
      <LabelsSection space={space} />
      <RulesSection space={space} />
      <ShortcutsSection space={space} />
      <PermissionsSection space={space} />
      <ImportBox space={space} />
      <section>
        <h2>{t('common.actions')}</h2>
        <ErrorNote error={archive.error ?? remove.error} />
        <div className="row">
          <a className="button" href={`/api/v1/spaces/${space.id}/export.zip`}>
            {t('space.export')}
          </a>
          {space.status === 'archived' ? (
            <button type="button" onClick={() => archive.mutate('unarchive')}>
              {t('space.unarchive')}
            </button>
          ) : (
            <button type="button" onClick={() => archive.mutate('archive')}>
              {t('space.archive')}
            </button>
          )}
          <DeleteSpaceButton space={space} onDelete={() => remove.mutate()} />
        </div>
      </section>
    </Frame>
  );
}

export function TrashView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useSpaceByKey(spaceKey);
  const space = q.data;
  const [filter, setFilter] = useState('');
  const [byTitle, setByTitle] = useState(false);
  const [picked, setPicked] = useState<string[]>([]);
  const wanted = useDelayed(filter.trim());
  const items = useQuery({
    queryKey: ['trash', space?.id, wanted, byTitle],
    queryFn: () => {
      const params = new URLSearchParams({ q: wanted });
      if (byTitle) params.set('sort', 'title');
      return api.get<{ items: TrashItem[] }>(`/spaces/${space?.id}/trash?${params.toString()}`);
    },
    enabled: !!space,
  });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['trash', space?.id] });
    await client.invalidateQueries({ queryKey: ['tree', space?.id] });
  };
  const restore = useMutation({
    mutationFn: (id: string) => api.post(`/trash/${id}/restore`),
    onSettled: refresh,
  });
  const purge = useMutation({ mutationFn: (id: string) => api.del(`/trash/${id}`), onSettled: refresh });
  const bulk = useMutation({
    mutationFn: (action: 'restore' | 'purge') =>
      api.post(`/spaces/${space?.id}/trash/bulk`, { action, ids: picked }),
    onSuccess: () => setPicked([]),
    onSettled: refresh,
  });
  if (q.error) {
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={q.error} />
      </main>
    );
  }
  if (!space) return <Loading />;
  return (
    <Frame space={space}>
      <h1>{t('trash.title')}</h1>
      <ErrorNote error={items.error ?? restore.error ?? purge.error ?? bulk.error} />
      <div className="row">
        <input
          type="search"
          className="grow"
          value={filter}
          aria-label={t('trash.filter')}
          placeholder={t('trash.filter')}
          onChange={(e) => setFilter(e.target.value)}
        />
        <label className="check">
          <input type="checkbox" checked={byTitle} onChange={(e) => setByTitle(e.target.checked)} />
          {t('organise.byName')}
        </label>
      </div>
      {picked.length > 0 && (
        <div className="row" role="group" aria-label={t('content.actions')}>
          <span>{t('content.selected', { count: picked.length })}</span>
          <button type="button" onClick={() => bulk.mutate('restore')}>
            {t('trash.restore')}
          </button>
          <ConfirmButton
            label={t('trash.purge')}
            message={t('trash.confirmPurgeMany', { count: picked.length })}
            className="danger"
            onConfirm={() => bulk.mutate('purge')}
          />
        </div>
      )}
      {items.data?.items.length === 0 && <p className="muted">{t('trash.empty')}</p>}
      {(items.data?.items.length ?? 0) > 0 && (
        <table>
          <thead>
            <tr>
              <th>{t('history.pick')}</th>
              <th>{t('common.name')}</th>
              <th>{t('common.type')}</th>
              <th>{t('trash.deletedBy')}</th>
              <th>{t('trash.deleted')}</th>
              <th>{t('trash.purgeAfter')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {items.data?.items.map((i) => (
              <tr key={i.id} className={picked.includes(i.id) ? 'picked' : undefined}>
                <td>
                  <input
                    type="checkbox"
                    aria-label={t('favourites.pick', { title: i.hidden ? t('trash.restricted') : i.title })}
                    checked={picked.includes(i.id)}
                    onChange={(e) =>
                      setPicked(e.target.checked ? [...picked, i.id] : picked.filter((x) => x !== i.id))
                    }
                  />
                </td>
                <td>{i.hidden ? t('trash.restricted') : i.title}</td>
                <td>{i.object_type}</td>
                <td>{i.deleted_by_name || t('common.unknown')}</td>
                <td>{formatDate(i.deleted_at)}</td>
                <td>{formatDate(i.purge_after)}</td>
                <td className="row">
                  <button type="button" onClick={() => restore.mutate(i.id)}>
                    {t('trash.restore')}
                  </button>
                  {!i.hidden && (
                    <ConfirmButton
                      label={t('trash.purge')}
                      message={t('trash.confirmPurge')}
                      className="danger"
                      onConfirm={() => purge.mutate(i.id)}
                    />
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Frame>
  );
}
