// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Category, type PageRef, type Shortcut, type Space, type TrashItem } from '../api';
import { Link, navigate, useLocation } from '../router';
import { AnimalChoice } from '../components/animal-choice';
import { Frame, spaceByKeyQuery } from '../components/frame';
import { useDelayed } from '../components/quick-search';
import { IconArrowDown, IconArrowUp, IconChevronLeft } from '../components/icons';
import { SettingsFrame, type SettingsGroup } from '../components/settings-frame';
import { SpaceRolesSection } from './roles';
import { ConfirmButton, ErrorNote, Field, Loading, SpaceBadge } from '../components/ui';
import { useDates } from '../dates';
import { ExportSection, ImportSection, PDFDesignSection } from './exchange';
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
import { TableBox } from '../components/table-box';

const SPACE_SECTIONS = [
  'details',
  'key',
  'features',
  'links',
  'shortcuts',
  'categories',
  'people',
  'roles',
  'statuses',
  'labels',
  'placeholders',
  'rules',
  'import',
  'export',
  'pdf',
  'danger',
] as const;
type SpaceSection = (typeof SPACE_SECTIONS)[number];
/** The permission a part of the space settings needs. Every other part needs space administration. */
const SECTION_NEEDS: Record<string, string> = {
  roles: 'space.manage_access',
  permissions: 'space.manage_access',
  audit: 'space.audit',
  templates: 'space.templates',
  rules: 'space.automation',
  automation: 'space.automation',
  webhooks: 'space.automation',
  sites: 'site.publish',
};
const BACK_ICON_SIZE = 16;

function useSpaceByKey(key: string) {
  return useQuery(spaceByKeyQuery(key));
}

/** Links to pages nobody has started. Nothing shows until there is something to do. */
function PlaceholdersSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['placeholders-of-space', space.id],
    queryFn: () =>
      api.get<{ placeholders: { title: string; source: PageRef }[] }>(`/spaces/${space.id}/placeholders`),
  });
  const list = q.data?.placeholders ?? [];
  if (list.length === 0) return <ErrorNote error={q.error} />;
  return (
    <section aria-labelledby="placeholders-heading">
      <h2 id="placeholders-heading">{t('space.placeholders')}</h2>
      <p className="muted">{t('space.placeholdersHelp')}</p>
      <ul className="plain-list">
        {list.map((one) => (
          <li key={`${one.source.id}-${one.title}`}>
            {one.title} · <Link href={`/pages/${one.source.id}`}>{one.source.title}</Link>
          </li>
        ))}
      </ul>
    </section>
  );
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

/** Categories are one list kept by administrators: here a space picks from it (D15). */
function CategoriesSection({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['categories'],
    queryFn: () => api.get<{ categories: Category[] }>('/categories'),
  });
  const chosen = useQuery({
    queryKey: ['space-categories', space.id],
    queryFn: () => api.get<{ categories: string[] }>(`/spaces/${space.id}/categories`),
  });
  const save = useMutation({
    mutationFn: (categories: string[]) => api.put(`/spaces/${space.id}/categories`, { categories }),
    onSettled: () => client.invalidateQueries({ queryKey: ['space-categories', space.id] }),
  });
  const picked = chosen.data?.categories ?? [];
  if ((q.data?.categories.length ?? 0) === 0) return <ErrorNote error={q.error} />;
  return (
    <section>
      <h2>{t('admin.categories')}</h2>
      <ErrorNote error={save.error} />
      {q.data?.categories.map((c) => (
        <label key={c.id} className="check">
          <input
            type="checkbox"
            checked={picked.includes(c.id)}
            onChange={(e) => {
              save.mutate(e.target.checked ? [...picked, c.id] : picked.filter((x) => x !== c.id));
            }}
          />
          {c.name}
        </label>
      ))}
    </section>
  );
}

export function SpaceSettingsView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useSpaceByKey(spaceKey);
  const current = useLocation().searchParams.get('section') ?? 'details';
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
  const base = `/spaces/${space.key}`;
  const item = (id: SpaceSection, label: string) => ({ id, label, href: `${base}/settings?section=${id}` });
  const away = (id: string, label: string, href: string) => ({ id, label, href });
  const allGroups: SettingsGroup[] = [
    {
      id: 'general',
      title: t('spaceSettings.groupGeneral'),
      items: [
        item('details', t('space.details')),
        item('key', t('spaceSettings.key')),
        item('features', t('spaceSettings.features')),
        item('links', t('spaceSettings.keepTidy')),
        item('shortcuts', t('shortcutLinks.title')),
        item('categories', t('admin.categories')),
      ],
    },
    {
      id: 'access',
      title: t('spaceSettings.groupAccess'),
      items: [
        item('people', t('spaceSettings.owners')),
        item('roles', t('roles.spaceSection')),
        away('permissions', t('access.open'), `${base}/access`),
        away('audit', t('access.auditTitle'), `${base}/audit`),
      ],
    },
    {
      id: 'content',
      title: t('spaceSettings.groupContent'),
      items: [
        item('statuses', t('spaceSettings.statuses')),
        item('labels', t('organise.labels')),
        item('placeholders', t('space.placeholders')),
        away('content', t('content.title'), `${base}/content`),
        away('templates', t('templates.title'), `${base}/templates`),
        away('trash', t('space.trash'), `${base}/trash`),
      ],
    },
    {
      id: 'automation',
      title: t('spaceSettings.groupAutomation'),
      items: [
        item('rules', t('spaceSettings.rules')),
        away('automation', t('admin.automation'), `${base}/settings/automation`),
        away('webhooks', t('admin.webhooks'), `${base}/settings/webhooks`),
      ],
    },
    {
      id: 'exchange',
      title: t('spaceSettings.groupExchange'),
      items: [
        item('import', t('space.import')),
        item('export', t('exchange.export')),
        item('pdf', t('exchange.pdfDesign')),
        away('sites', t('sites.title'), `${base}/sites`),
      ],
    },
    {
      id: 'danger',
      title: t('spaceSettings.groupDanger'),
      items: [item('danger', t('spaceSettings.archiveOrDelete'))],
    },
  ];
  // Each part shows to whoever holds the permission it needs, so a role with only
  // publishing or automation still finds its way here.
  const groups = allGroups
    .map((g) => ({
      ...g,
      items: g.items.filter((i) => space.can?.[SECTION_NEEDS[i.id] ?? 'space.administer']),
    }))
    .filter((g) => g.items.length > 0);
  const allowed = groups.flatMap((g) => g.items).map((i) => i.id);
  const asked = SPACE_SECTIONS.includes(current as SpaceSection) ? (current as SpaceSection) : 'details';
  // Someone who may only open pages elsewhere, such as the sites, gets no part here.
  const first = SPACE_SECTIONS.find((s) => allowed.includes(s)) ?? null;
  const section = allowed.includes(asked) ? asked : first;
  const label = groups.flatMap((g) => g.items).find((i) => i.id === section)?.label ?? t('space.settings');
  return (
    <SettingsFrame
      title={t('space.settings')}
      head={<SettingsBack space={space} />}
      groups={groups}
      current={section ?? ''}
    >
      {space.status === 'archived' && (
        <p role="status" className="note">
          {t('space.archived')}
        </p>
      )}
      <h1>{label}</h1>
      {section === null && <p className="muted">{t('spaceSettings.pickOne')}</p>}
      {section === 'details' && (
        <form
          className="settings-form"
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
      )}
      {section === 'key' && <KeySection space={space} />}
      {section === 'features' && <FeaturesSection space={space} />}
      {section === 'links' && <SpaceLinksSection space={space} />}
      {section === 'shortcuts' && <ShortcutsSection space={space} />}
      {section === 'categories' && <CategoriesSection space={space} />}
      {section === 'people' && <PeopleSection space={space} />}
      {section === 'roles' && <SpaceRolesSection space={space} />}
      {section === 'statuses' && <StatusesSection space={space} />}
      {section === 'labels' && <LabelsSection space={space} />}
      {section === 'placeholders' && <PlaceholdersSection space={space} />}
      {section === 'rules' && <RulesSection space={space} />}
      {section === 'import' && <ImportSection space={space} />}
      {section === 'export' && <ExportSection space={space} />}
      {section === 'pdf' && <PDFDesignSection space={space} />}
      {section === 'danger' && (
        <section>
          <ErrorNote error={archive.error ?? remove.error} />
          <div className="row">
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
      )}
    </SettingsFrame>
  );
}

/** The top of the settings sidebar: the space, and the way back to it. */
function SettingsBack({ space }: { space: Space }) {
  const { t } = useTranslation();
  return (
    <>
      <Link href={`/spaces/${space.key}`} className="side-link settings-back">
        <IconChevronLeft size={BACK_ICON_SIZE} /> {t('spaceSettings.back')}
      </Link>
      <div className="sidebar-head">
        <span className="space-link">
          <SpaceBadge space={space} /> {space.name}
        </span>
      </div>
    </>
  );
}

export function TrashView({ spaceKey }: { spaceKey: string }) {
  const { t } = useTranslation();
  const dates = useDates();
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
        <TableBox>
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
                  <td>{dates.moment(i.deleted_at)}</td>
                  <td>{dates.moment(i.purge_after)}</td>
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
        </TableBox>
      )}
    </Frame>
  );
}
