// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  GLOBAL_PERMISSIONS,
  type AuditEvent,
  type DirectoryEntry,
  type Group,
  type MailEntry,
  type Me,
  type Space,
  type TrashItem,
  type UserDetail,
} from '../api';
import { Link, navigate, useLocation } from '../router';
import { PersonName } from '../components/person';
import { SubjectPicker } from '../components/picker';
import { ConfirmButton, Dialog, ErrorNote, Field, formatSize, Loading } from '../components/ui';
import { useDates } from '../dates';
import { AppearanceSection } from './admin-appearance';
import { AnalyticsTab, InsightsTab, OutgoingTab } from './admin-platform';
import { RuleList } from './rules';
import { WebhookList } from './webhooks';
import { OrganisingTab, TemplatesTab } from './admin-organising';
import { CareTab } from './admin-care';
import { ExportTab } from './exchange';
import { LookTab } from './admin-look';
import {
  AnonymiseButton,
  BulkBar,
  ExpiryField,
  InvitationsTab,
  ServiceAccounts,
  TypeChange,
} from './admin-people';
import { PermissionsTab, RulesTab } from './admin-rules';
import { StorageLimitDialog } from './admin-storage';
import { matchesSpaceFilter, NO_SPACE_FILTER, QUIET_MONTHS, type SpaceFilter } from './admin-space-filter';
import { SettingsSearch, SummaryTab } from './admin-summary';
import { HealthBand, HealthChecks } from './admin-health';
import { BackupsTab, ConfigurationTab, SessionsTab } from './admin-running';
import { SettingsFrame } from '../components/settings-frame';
import { TableBox } from '../components/table-box';

const TABS = [
  'summary',
  'users',
  'invitations',
  'groups',
  'spaces',
  'templates',
  'organising',
  'export',
  'audit',
  'permissions',
  'rules',
  'look',
  'settings',
  'mail',
  'care',
  'scim',
  'system',
  'backups',
  'configuration',
  'sessions',
  'insights',
  'analytics',
  'automation',
  'outgoing',
  'webhooks',
] as const;
/** The administration in categories (ADR-034). Every tab sits in exactly one. */
const ADMIN_GROUPS: { id: string; tabs: (typeof TABS)[number][] }[] = [
  { id: 'overview', tabs: ['summary'] },
  { id: 'people', tabs: ['users', 'invitations', 'groups', 'permissions', 'scim', 'sessions'] },
  { id: 'content', tabs: ['spaces', 'templates', 'organising', 'rules', 'export'] },
  { id: 'automation', tabs: ['automation', 'webhooks', 'outgoing'] },
  { id: 'look', tabs: ['look'] },
  {
    id: 'system',
    tabs: [
      'system',
      'backups',
      'configuration',
      'care',
      'settings',
      'mail',
      'audit',
      'insights',
      'analytics',
    ],
  },
];
const USER_FILTERS = ['type', 'status', 'source', 'role', 'space_id', 'group_id', 'idle_days'] as const;
/** The global permissions the server can filter people on. */
const USER_ROLES = ['administrator', 'system_administrator', 'create_space', 'invite_people'];
const USER_TYPES = ['member', 'guest'];
const USER_STATUSES = ['active', 'disabled'];
const USER_SOURCES = ['local', 'oidc', 'scim'];
const IDLE_DAYS = [30, 90, 365];
const AUDIT_FILTERS = [
  'actor_id',
  'action',
  'object_type',
  'object_id',
  'space_id',
  'result',
  'since',
  'until',
] as const;
const SYSTEM_POLL_MS = 10_000;
const DATE_LENGTH = 10;
const MAIL_SWITCHES = ['mail_enabled', 'mail_excerpts', 'reactions_enabled', 'group_mentions'] as const;

function GlobalPermissions({ current, onChange }: { current: string[]; onChange: (next: string[]) => void }) {
  const { t } = useTranslation();
  return (
    <fieldset className="matrix">
      <legend>{t('admin.globalPermissions')}</legend>
      {GLOBAL_PERMISSIONS.map((p) => (
        <label key={p}>
          <input
            type="checkbox"
            checked={current.includes(p)}
            onChange={(e) => onChange(e.target.checked ? [...current, p] : current.filter((x) => x !== p))}
          />
          {t(`admin.perm_${p}`)}
        </label>
      ))}
    </fieldset>
  );
}

/** Groups, spaces and the number of pages a person owns, without titles (ADM-010). */
function UserReach({ userId }: { userId: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['user-reach', userId],
    queryFn: () =>
      api.get<{ groups: string[]; spaces: string[]; pages_owned: number }>(`/admin/users/${userId}/reach`),
  });
  if (!q.data) return <ErrorNote error={q.error} />;
  const none = t('admin.reachNone');
  return (
    <dl className="facts">
      <dt>{t('admin.reachGroups')}</dt>
      <dd>{q.data.groups.join(', ') || none}</dd>
      <dt>{t('admin.reachSpaces')}</dt>
      <dd>{q.data.spaces.join(', ') || none}</dd>
      <dt>{t('admin.reachPages')}</dt>
      <dd>{q.data.pages_owned}</dd>
    </dl>
  );
}

function UserDialog({ user, onClose }: { user: UserDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [perms, setPerms] = useState(user.global_permissions);
  const [password, setPassword] = useState('');
  const refresh = () => client.invalidateQueries({ queryKey: ['admin-users'] });
  const finish = async () => {
    await refresh();
    onClose();
  };
  const act = useMutation({
    mutationFn: (path: string) => api.post(`/admin/users/${user.id}/${path}`),
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  const save = useMutation({
    mutationFn: (next: string[]) => api.put(`/admin/users/${user.id}/permissions`, { permissions: next }),
    onSuccess: refresh,
  });
  const reset = useMutation({
    mutationFn: () => api.post(`/admin/users/${user.id}/reset-password`, { password }),
    onSuccess: () => setPassword(''),
  });
  return (
    <Dialog title={user.display_name} onClose={onClose}>
      <p className="muted">
        {user.email} · {t(`admin.source_${user.source}`)}
      </p>
      <ErrorNote error={act.error ?? save.error ?? reset.error} />
      {user.password_locked && (
        <div className="row" role="status">
          <p>{t('admin.passwordLocked')}</p>
          <button type="button" onClick={() => act.mutate('unlock-password')}>
            {t('admin.liftPasswordLock')}
          </button>
        </div>
      )}
      <UserReach userId={user.id} />
      <GlobalPermissions
        current={perms}
        onChange={(next) => {
          setPerms(next);
          save.mutate(next);
        }}
      />
      <div className="row">
        {user.status === 'active' ? (
          <button type="button" className="danger" onClick={() => act.mutate('disable')}>
            {t('admin.disable')}
          </button>
        ) : (
          <button type="button" onClick={() => act.mutate('enable')}>
            {t('admin.enable')}
          </button>
        )}
        {user.totp_enabled && (
          <button type="button" onClick={() => act.mutate('reset-mfa')}>
            {t('admin.resetMfa')}
          </button>
        )}
        <TypeChange user={user} onDone={() => void finish()} />
        <AnonymiseButton user={user} onDone={() => void finish()} />
      </div>
      <ExpiryField user={user} onDone={() => void finish()} />
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          reset.mutate();
        }}
      >
        <Field label={t('admin.newPassword')}>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            required
          />
        </Field>
        <button type="submit">{t('admin.resetPassword')}</button>
      </form>
    </Dialog>
  );
}

function NewUserDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [form, setForm] = useState({ email: '', display_name: '', type: 'member', password: '' });
  const create = useMutation({
    mutationFn: () => api.post('/admin/users', form),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['admin-users'] });
      onClose();
    },
  });
  return (
    <Dialog title={t('admin.newUser')} onClose={onClose}>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <ErrorNote error={create.error} />
        <Field label={t('common.email')}>
          <input
            type="email"
            value={form.email}
            onChange={(e) => setForm({ ...form, email: e.target.value })}
            required
            autoFocus
          />
        </Field>
        <Field label={t('admin.displayName')}>
          <input
            value={form.display_name}
            onChange={(e) => setForm({ ...form, display_name: e.target.value })}
            required
          />
        </Field>
        <Field label={t('admin.userType')}>
          <select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })}>
            <option value="member">{t('admin.member')}</option>
            <option value="guest">{t('admin.guest')}</option>
          </select>
        </Field>
        <Field label={t('common.password')}>
          <input
            type="password"
            value={form.password}
            onChange={(e) => setForm({ ...form, password: e.target.value })}
            autoComplete="new-password"
            required
          />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('common.create')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** Filters that live in the address, so a view can be shared and survives a reload. */
function useAddressFilters<K extends string>(path: string, keys: readonly K[]) {
  const { searchParams } = useLocation();
  const values = Object.fromEntries(keys.map((k) => [k, searchParams.get(k) ?? ''])) as Record<K, string>;
  const set = (key: K, value: string) => {
    const next = new URLSearchParams();
    for (const k of keys) {
      const v = k === key ? value : values[k];
      if (v) next.set(k, v);
    }
    const query = next.toString();
    navigate(query ? `${path}?${query}` : path, true);
  };
  return { values, set };
}

function FilterSelect({
  label,
  value,
  options,
  name,
  onChange,
}: {
  label: string;
  value: string;
  options: (string | number)[];
  name: (option: string | number) => string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <select aria-label={label} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{`${label}: ${t('admin.any')}`}</option>
      {options.map((o) => (
        <option key={o} value={o}>
          {name(o)}
        </option>
      ))}
    </select>
  );
}

function UserFilters({
  values,
  set,
}: {
  values: Record<(typeof USER_FILTERS)[number], string>;
  set: (key: (typeof USER_FILTERS)[number], value: string) => void;
}) {
  const { t } = useTranslation();
  const groups = useQuery({
    queryKey: ['admin-groups'],
    queryFn: () => api.get<{ groups: Group[] }>('/admin/groups'),
  });
  const spaces = useQuery({
    queryKey: ['admin-spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/admin/spaces'),
  });
  return (
    <div className="row filters">
      <FilterSelect
        label={t('common.type')}
        value={values.type}
        options={USER_TYPES}
        name={(o) => t(`admin.${o}`)}
        onChange={(v) => set('type', v)}
      />
      <FilterSelect
        label={t('common.status')}
        value={values.status}
        options={USER_STATUSES}
        name={(o) => t(`admin.${o}`)}
        onChange={(v) => set('status', v)}
      />
      <FilterSelect
        label={t('admin.source')}
        value={values.source}
        options={USER_SOURCES}
        name={(o) => t(`admin.source_${o}`)}
        onChange={(v) => set('source', v)}
      />
      <FilterSelect
        label={t('admin.role')}
        value={values.role}
        options={USER_ROLES}
        name={(o) => t(`permissions.global_${o}`)}
        onChange={(v) => set('role', v)}
      />
      <FilterSelect
        label={t('admin.space')}
        value={values.space_id}
        options={spaces.data?.spaces.map((s) => s.id) ?? []}
        name={(o) => spaces.data?.spaces.find((s) => s.id === o)?.name ?? ''}
        onChange={(v) => set('space_id', v)}
      />
      <FilterSelect
        label={t('admin.groups')}
        value={values.group_id}
        options={groups.data?.groups.map((g) => g.id) ?? []}
        name={(o) => groups.data?.groups.find((g) => g.id === o)?.name ?? ''}
        onChange={(v) => set('group_id', v)}
      />
      <FilterSelect
        label={t('admin.lastLogin')}
        value={values.idle_days}
        options={IDLE_DAYS}
        name={(o) => t('admin.idleDays', { count: Number(o) })}
        onChange={(v) => set('idle_days', v)}
      />
    </div>
  );
}

function UserTable({
  users,
  picked,
  onToggle,
  onOpen,
}: {
  users: UserDetail[];
  picked: string[];
  onToggle: (id: string) => void;
  onOpen: (id: string) => void;
}) {
  const { t } = useTranslation();
  const dates = useDates();
  return (
    <TableBox>
      <table>
        <thead>
          <tr>
            <th>
              <span className="visually-hidden">{t('admin.select')}</span>
            </th>
            <th>{t('common.name')}</th>
            <th>{t('common.email')}</th>
            <th>{t('common.type')}</th>
            <th>{t('admin.source')}</th>
            <th>{t('common.status')}</th>
            <th>{t('admin.lastLogin')}</th>
          </tr>
        </thead>
        <tbody>
          {users.map((u) => (
            <tr key={u.id}>
              <td>
                <input
                  type="checkbox"
                  aria-label={t('admin.selectUser', { name: u.display_name })}
                  checked={picked.includes(u.id)}
                  onChange={() => onToggle(u.id)}
                />
              </td>
              <td>
                <button type="button" className="ghost" onClick={() => onOpen(u.id)}>
                  {u.display_name}
                </button>
              </td>
              <td className="email">{u.email}</td>
              <td>
                {t(`admin.${u.type}`)}
                {u.type === 'guest' && (
                  <span className="muted">
                    {' · '}
                    {u.spaces?.length ? u.spaces.join(', ') : t('admin.guestNoSpaces')}
                  </span>
                )}
              </td>
              <td>{t(`admin.source_${u.source}`)}</td>
              <td>{u.status === 'active' ? t('admin.active') : t('admin.disabled')}</td>
              <td>{dates.moment(u.last_login_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableBox>
  );
}

function UsersTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const { values, set } = useAddressFilters('/admin/users', USER_FILTERS);
  const [text, setText] = useState('');
  const [adding, setAdding] = useState(false);
  const [open, setOpen] = useState<string | null>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const query = new URLSearchParams({ q: text });
  for (const key of USER_FILTERS) if (values[key]) query.set(key, values[key]);
  const q = useQuery({
    queryKey: ['admin-users', query.toString()],
    queryFn: () => api.get<{ users: UserDetail[]; total: number }>(`/admin/users?${query.toString()}`),
  });
  const selected = q.data?.users.find((u) => u.id === open);
  const toggle = (id: string) =>
    setPicked(picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id]);
  return (
    <>
      <div className="row">
        <input
          type="search"
          value={text}
          onChange={(e) => setText(e.target.value)}
          aria-label={t('common.search')}
          placeholder={t('common.search')}
        />
        <button type="button" className="primary" onClick={() => setAdding(true)}>
          {t('admin.newUser')}
        </button>
      </div>
      <UserFilters values={values} set={set} />
      <ErrorNote error={q.error} />
      {picked.length > 0 && (
        <BulkBar
          users={picked}
          onDone={() => {
            setPicked([]);
            void client.invalidateQueries({ queryKey: ['admin-users'] });
          }}
        />
      )}
      <UserTable users={q.data?.users ?? []} picked={picked} onToggle={toggle} onOpen={setOpen} />
      <ServiceAccounts />
      {adding && <NewUserDialog onClose={() => setAdding(false)} />}
      {selected && <UserDialog user={selected} onClose={() => setOpen(null)} />}
    </>
  );
}

/** Says which spaces and restrictions use a group, before it goes (spec 013, F8). */
function GroupDeleteDialog({
  group,
  onDelete,
  onClose,
}: {
  group: Group;
  onDelete: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['group-impact', group.id],
    queryFn: () =>
      api.get<{ spaces: string[]; restrictions: number; leaves_space_without_admin: boolean }>(
        `/admin/groups/${group.id}/impact`,
      ),
  });
  return (
    <Dialog title={t('admin.groupDelete', { name: group.name })} onClose={onClose}>
      <ErrorNote error={q.error} />
      {q.data && (
        <>
          <p>{t('admin.groupSpaces', { list: q.data.spaces.join(', ') || '-' })}</p>
          <p>{t('admin.groupRestrictions', { count: q.data.restrictions })}</p>
          {q.data.leaves_space_without_admin && (
            <p className="note note-error">{t('admin.groupLastAdmin')}</p>
          )}
        </>
      )}
      <div className="dialog-actions">
        <button
          type="button"
          className="danger"
          disabled={!q.data || q.data.leaves_space_without_admin}
          onClick={() => {
            onClose();
            onDelete();
          }}
        >
          {t('common.delete')}
        </button>
      </div>
    </Dialog>
  );
}

function GroupDialog({ group, onClose }: { group: Group; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['group-members', group.id];
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: key });
    await client.invalidateQueries({ queryKey: ['admin-groups'] });
  };
  const q = useQuery({
    queryKey: key,
    queryFn: () =>
      api.get<{ members: { id: string; email: string; display_name: string }[] }>(
        `/admin/groups/${group.id}/members`,
      ),
  });
  const [perms, setPerms] = useState(group.global_permissions);
  const add = useMutation({
    mutationFn: (id: string) => api.put(`/admin/groups/${group.id}/members/${id}`),
    onSettled: refresh,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/admin/groups/${group.id}/members/${id}`),
    onSettled: refresh,
  });
  const save = useMutation({
    mutationFn: (next: string[]) => api.put(`/admin/groups/${group.id}/permissions`, { permissions: next }),
    onSettled: refresh,
  });
  const del = useMutation({
    mutationFn: () => api.del(`/admin/groups/${group.id}`),
    onSuccess: async () => {
      await refresh();
      onClose();
    },
  });
  const [owners, setOwners] = useState<DirectoryEntry[]>([]);
  const saveOwners = useMutation({
    mutationFn: (next: DirectoryEntry[]) =>
      api.put(`/admin/groups/${group.id}/owners`, { owners: next.map((o) => o.id) }),
  });
  const pickOwner = (entry: DirectoryEntry) => {
    const next = [...owners, entry];
    setOwners(next);
    saveOwners.mutate(next);
  };
  const [deleting, setDeleting] = useState(false);
  const managed = group.source !== 'local';
  return (
    <Dialog title={group.name} onClose={onClose}>
      {managed && <p className="note">{t('admin.managed')}</p>}
      <ErrorNote error={add.error ?? remove.error ?? save.error ?? del.error ?? saveOwners.error} />
      <h3>{t('admin.members')}</h3>
      <ul className="plain-list">
        {q.data?.members.map((m) => (
          <li key={m.id} className="row">
            {m.display_name} <span className="muted">{m.email}</span>
            {!managed && (
              <button type="button" className="ghost" onClick={() => remove.mutate(m.id)}>
                {t('common.remove')}
              </button>
            )}
          </li>
        ))}
      </ul>
      {!managed && <SubjectPicker label={t('admin.addMember')} usersOnly onPick={(e) => add.mutate(e.id)} />}
      <h3>{t('admin.groupOwners')}</h3>
      <p className="muted">{t('admin.groupOwnersHelp')}</p>
      <ul className="plain-list">
        {owners.map((o) => (
          <li key={o.id}>{o.name}</li>
        ))}
      </ul>
      <SubjectPicker label={t('admin.addOwner')} usersOnly onPick={pickOwner} />
      <GlobalPermissions
        current={perms}
        onChange={(next) => {
          setPerms(next);
          save.mutate(next);
        }}
      />
      {!managed && (
        <button type="button" className="danger" onClick={() => setDeleting(true)}>
          {t('common.delete')}
        </button>
      )}
      {deleting && (
        <GroupDeleteDialog group={group} onDelete={() => del.mutate()} onClose={() => setDeleting(false)} />
      )}
    </Dialog>
  );
}

function GroupsTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const q = useQuery({
    queryKey: ['admin-groups'],
    queryFn: () => api.get<{ groups: Group[] }>('/admin/groups'),
  });
  const create = useMutation({
    mutationFn: () => api.post('/admin/groups', { name }),
    onSuccess: async () => {
      setName('');
      await client.invalidateQueries({ queryKey: ['admin-groups'] });
    },
  });
  const selected = q.data?.groups.find((g) => g.id === open);
  return (
    <>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label={t('admin.newGroup')}
          placeholder={t('admin.newGroup')}
          required
        />
        <button type="submit" className="primary">
          {t('common.create')}
        </button>
      </form>
      <ErrorNote error={q.error ?? create.error} />
      {q.data?.groups.length === 0 ? (
        <p className="muted">{t('admin.noGroups')}</p>
      ) : (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('common.name')}</th>
                <th>{t('admin.members')}</th>
                <th>{t('admin.source')}</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.groups.map((g) => (
                <tr key={g.id}>
                  <td>
                    <button type="button" className="ghost" onClick={() => setOpen(g.id)}>
                      {g.name}
                    </button>
                  </td>
                  <td>{g.members}</td>
                  <td>{g.source}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
      {selected && <GroupDialog group={selected} onClose={() => setOpen(null)} />}
    </>
  );
}

interface SpaceTotal {
  id: string;
  pages: number;
  bytes: number;
  last_activity: string | null;
  limit_mib: number;
}

const MEBIBYTE = 1 << 20;

function SpacesTab() {
  const { t } = useTranslation();
  const dates = useDates();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['admin-spaces'],
    queryFn: () => api.get<{ spaces: Space[]; totals: SpaceTotal[] }>('/admin/spaces'),
  });
  const trashed = useQuery({
    queryKey: ['admin-trash'],
    queryFn: () => api.get<{ items: TrashItem[] }>('/admin/trash/spaces'),
  });
  const [recovering, setRecovering] = useState<Space | null>(null);
  const [limiting, setLimiting] = useState<Space | null>(null);
  const [reason, setReason] = useState('');
  const recover = useMutation({
    mutationFn: () => api.post(`/admin/spaces/${recovering?.id}/recover`, { reason }),
    onSuccess: async () => {
      setRecovering(null);
      setReason('');
      await client.invalidateQueries({ queryKey: ['spaces'] });
    },
  });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['admin-trash'] });
    await client.invalidateQueries({ queryKey: ['admin-spaces'] });
  };
  const restore = useMutation({
    mutationFn: (id: string) => api.post(`/trash/${id}/restore`),
    onSettled: refresh,
  });
  const purge = useMutation({ mutationFn: (id: string) => api.del(`/trash/${id}`), onSettled: refresh });
  const [filter, setFilter] = useState<SpaceFilter>(NO_SPACE_FILTER);
  const now = new Date();
  const shown = (q.data?.spaces ?? []).filter((s) => {
    const total = q.data?.totals.find((row) => row.id === s.id);
    return matchesSpaceFilter(
      {
        name: s.name,
        key: s.key,
        pages: total?.pages ?? 0,
        bytes: total?.bytes ?? 0,
        lastActivity: total?.last_activity ?? null,
      },
      filter,
      now,
    );
  });
  const count = (raw: string) => Math.max(0, Number.parseInt(raw, 10) || 0);
  return (
    <>
      <ErrorNote error={q.error ?? restore.error ?? purge.error} />
      <div className="row">
        <input
          type="search"
          value={filter.text}
          onChange={(e) => setFilter({ ...filter, text: e.target.value })}
          aria-label={t('admin.spaceFind')}
          placeholder={t('admin.spaceFind')}
        />
        <input
          type="number"
          min={0}
          value={filter.minPages || ''}
          onChange={(e) => setFilter({ ...filter, minPages: count(e.target.value) })}
          aria-label={t('admin.spaceMinPages')}
          placeholder={t('admin.spaceMinPages')}
        />
        <input
          type="number"
          min={0}
          value={filter.minMiB || ''}
          onChange={(e) => setFilter({ ...filter, minMiB: count(e.target.value) })}
          aria-label={t('admin.spaceMinMiB')}
          placeholder={t('admin.spaceMinMiB')}
        />
        <select
          aria-label={t('admin.spaceQuiet')}
          value={filter.quietMonths}
          onChange={(e) => setFilter({ ...filter, quietMonths: Number(e.target.value) })}
        >
          {QUIET_MONTHS.map((m) => (
            <option key={m} value={m}>
              {m === 0
                ? `${t('admin.spaceQuiet')}: ${t('admin.any')}`
                : t('admin.spaceQuietFor', { count: m })}
            </option>
          ))}
        </select>
      </div>
      {q.data && shown.length === 0 && <p className="muted">{t('admin.spaceNoneFit')}</p>}
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('space.key')}</th>
              <th>{t('common.status')}</th>
              <th>{t('admin.spacePages')}</th>
              <th>{t('admin.spaceStorage')}</th>
              <th>{t('admin.spaceActivity')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((s) => {
              const total = q.data?.totals.find((row) => row.id === s.id);
              return (
                <tr key={s.id}>
                  <td>
                    <Link href={`/spaces/${s.key}`}>{s.name}</Link>
                  </td>
                  <td>{s.key}</td>
                  <td>{s.status}</td>
                  <td>{total?.pages ?? 0}</td>
                  <td>
                    {formatSize(total?.bytes ?? 0)}
                    {total?.limit_mib ? ` / ${formatSize(total.limit_mib * MEBIBYTE)}` : ''}
                  </td>
                  <td>{total?.last_activity ? dates.moment(total.last_activity) : ''}</td>
                  <td className="row">
                    <button type="button" className="ghost" onClick={() => setLimiting(s)}>
                      {t('admin.storageLimit')}
                    </button>
                    <button type="button" className="ghost" onClick={() => setRecovering(s)}>
                      {t('admin.recover')}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </TableBox>
      {(trashed.data?.items.length ?? 0) > 0 && (
        <>
          <h2>{t('admin.trashedSpaces')}</h2>
          <ul className="plain-list">
            {trashed.data?.items.map((i) => (
              <li key={i.id} className="row">
                {i.title} <span className="muted">{dates.moment(i.purge_after)}</span>
                <button type="button" onClick={() => restore.mutate(i.id)}>
                  {t('trash.restore')}
                </button>
                <ConfirmButton
                  label={t('trash.purge')}
                  message={t('trash.confirmPurge')}
                  className="danger"
                  onConfirm={() => purge.mutate(i.id)}
                />
              </li>
            ))}
          </ul>
        </>
      )}
      {limiting && (
        <StorageLimitDialog
          space={limiting}
          limitMiB={q.data?.totals.find((row) => row.id === limiting.id)?.limit_mib ?? 0}
          onClose={() => setLimiting(null)}
        />
      )}
      {recovering && (
        <Dialog title={t('admin.recover')} onClose={() => setRecovering(null)}>
          <p>{t('admin.recoverHelp')}</p>
          <form
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              recover.mutate();
            }}
          >
            <ErrorNote error={recover.error} />
            <Field label={t('admin.reason')}>
              <input value={reason} onChange={(e) => setReason(e.target.value)} required autoFocus />
            </Field>
            <div className="dialog-actions">
              <button type="submit" className="primary">
                {t('admin.recover')}
              </button>
            </div>
          </form>
        </Dialog>
      )}
    </>
  );
}

/** A page in the log, by its id. A title the reader may not see comes censored. */
function auditObject(e: AuditEvent): string {
  const title = e.details?.title;
  return [e.object_id, typeof title === 'string' ? title : ''].filter(Boolean).join(' ');
}

interface PageNote {
  id: string;
  title: string;
  space_key: string;
  space_name: string;
  status: string;
  restricted: boolean;
  readable: boolean;
}

/** What an administrator may know of a page from the log (ADR-029). */
function PageNoteFacts({ id }: { id: string }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['page-note', id],
    queryFn: () => api.get<PageNote>(`/admin/pages/${id}`),
    retry: false,
  });
  if (q.error) return <p className="muted">{t('admin.pageGone')}</p>;
  if (!q.data) return <Loading />;
  const note = q.data;
  return (
    <dl className="facts">
      <dt>{t('page.title')}</dt>
      <dd>{note.readable ? <Link href={`/pages/${id}`}>{note.title}</Link> : note.title}</dd>
      <dt>{t('admin.spaces')}</dt>
      <dd>{note.space_name}</dd>
      <dt>{t('common.status')}</dt>
      <dd>{note.status}</dd>
      <dt>{t('page.restrictions')}</dt>
      <dd>{note.restricted ? t('common.yes') : t('common.no')}</dd>
    </dl>
  );
}

function AuditTab() {
  const { t } = useTranslation();
  const dates = useDates();
  const { values, set } = useAddressFilters('/admin/audit', AUDIT_FILTERS);
  const [open, setOpen] = useState<AuditEvent | null>(null);
  const query = new URLSearchParams({ limit: '100' });
  for (const key of AUDIT_FILTERS) if (values[key]) query.set(key, values[key]);
  const q = useQuery({
    queryKey: ['audit', query.toString()],
    queryFn: () => api.get<{ events: AuditEvent[] }>(`/admin/audit-events?${query.toString()}`),
  });
  const verify = useMutation({
    mutationFn: () =>
      api.get<{ ok: boolean; checked: number; first_bad_seq?: number; reason?: string }>(
        '/admin/audit-events/verify',
      ),
  });
  return (
    <>
      <div className="row">
        <input
          value={values.action}
          onChange={(e) => set('action', e.target.value)}
          aria-label={t('admin.filterAction')}
          placeholder={t('admin.filterAction')}
        />
        <input
          value={values.object_type}
          onChange={(e) => set('object_type', e.target.value)}
          aria-label={t('admin.filterObject')}
          placeholder={t('admin.filterObject')}
        />
        <input
          value={values.object_id}
          onChange={(e) => set('object_id', e.target.value)}
          aria-label={t('admin.filterObjectId')}
          placeholder={t('admin.filterObjectId')}
        />
        <input
          value={values.actor_id}
          onChange={(e) => set('actor_id', e.target.value)}
          aria-label={t('admin.filterActor')}
          placeholder={t('admin.filterActor')}
        />
        <input
          value={values.space_id}
          onChange={(e) => set('space_id', e.target.value)}
          aria-label={t('admin.filterSpace')}
          placeholder={t('admin.filterSpace')}
        />
        <FilterSelect
          label={t('admin.result')}
          value={values.result}
          options={['success', 'denied', 'failure']}
          name={String}
          onChange={(v) => set('result', v)}
        />
        <Field label={t('admin.since')}>
          <input
            type="date"
            value={values.since.slice(0, DATE_LENGTH)}
            onChange={(e) => set('since', e.target.value ? new Date(e.target.value).toISOString() : '')}
          />
        </Field>
        <Field label={t('admin.until')}>
          <input
            type="date"
            value={values.until.slice(0, DATE_LENGTH)}
            onChange={(e) => set('until', e.target.value ? new Date(e.target.value).toISOString() : '')}
          />
        </Field>
        <a className="button" href="/api/v1/admin/audit-events/export">
          {t('admin.exportCsv')}
        </a>
        <button type="button" onClick={() => verify.mutate()}>
          {t('admin.verify')}
        </button>
      </div>
      <ErrorNote error={q.error ?? verify.error} />
      {verify.data && (
        <p role="status" className={verify.data.ok ? 'note' : 'note note-error'}>
          {verify.data.ok
            ? t('admin.verifyOk', { checked: verify.data.checked })
            : t('admin.verifyBad', { seq: verify.data.first_bad_seq, reason: verify.data.reason })}
        </p>
      )}
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('admin.when')}</th>
              <th>{t('admin.actor')}</th>
              <th>{t('admin.action')}</th>
              <th>{t('admin.object')}</th>
              <th>{t('admin.result')}</th>
              <th>{t('admin.ip')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.events.map((e) => (
              <tr key={e.id}>
                <td>
                  <button type="button" className="ghost" onClick={() => setOpen(e)}>
                    {dates.moment(e.occurred_at)}
                  </button>
                </td>
                <td>{e.actor_id ? <PersonName id={e.actor_id} name={e.actor_id} /> : e.actor_type}</td>
                <td>{e.action}</td>
                <td>
                  {e.object_type} <span className="muted">{auditObject(e)}</span>
                </td>
                <td>{e.result}</td>
                <td>{e.ip}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
      {open && (
        <Dialog title={open.action} onClose={() => setOpen(null)}>
          {open.object_type === 'page' && open.object_id && <PageNoteFacts id={open.object_id} />}
          {/* Focusable, so the keyboard can scroll it (WCAG 2.1.1). */}
          <pre className="audit-details" tabIndex={0}>
            {JSON.stringify(open, null, 2)}
          </pre>
        </Dialog>
      )}
    </>
  );
}

interface Settings {
  trash_retention_days: number;
  max_upload_mib: number;
  blocked_file_types: string[];
}

function ExampleSpaceSection() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const add = useMutation({
    mutationFn: () => api.post<{ key: string }>('/admin/example-space'),
    onSuccess: async ({ key }) => {
      await client.invalidateQueries({ queryKey: ['spaces'] });
      navigate(`/spaces/${key}`);
    },
  });
  return (
    <>
      <h2>{t('admin.exampleTitle')}</h2>
      <p className="muted">{t('admin.exampleHelp')}</p>
      <ErrorNote error={add.error} />
      <button type="button" disabled={add.isPending} onClick={() => add.mutate()}>
        {t('admin.exampleAdd')}
      </button>
    </>
  );
}

function SettingsTab() {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ['settings'], queryFn: () => api.get<Settings>('/admin/settings') });
  const system = useQuery({
    queryKey: ['system'],
    queryFn: () => api.get<{ oidc_configured: boolean }>('/admin/system'),
  });
  const [form, setForm] = useState<Settings | null>(null);
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    if (q.data) setForm(q.data);
  }, [q.data]);
  const save = useMutation({
    mutationFn: () => api.put('/admin/settings', form),
    onSuccess: () => setSaved(true),
  });
  if (!form) return <Loading />;
  return (
    <>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          setSaved(false);
          save.mutate();
        }}
      >
        <ErrorNote error={save.error} />
        {saved && (
          <p role="status" className="note">
            {t('admin.saved')}
          </p>
        )}
        <Field label={t('admin.retention')}>
          <input
            type="number"
            min={1}
            max={3650}
            value={form.trash_retention_days}
            onChange={(e) => setForm({ ...form, trash_retention_days: Number(e.target.value) })}
          />
        </Field>
        <Field label={t('admin.maxUpload')}>
          <input
            type="number"
            min={1}
            max={10240}
            value={form.max_upload_mib}
            onChange={(e) => setForm({ ...form, max_upload_mib: Number(e.target.value) })}
          />
        </Field>
        <Field label={t('admin.blockedTypes')} help={t('admin.blockedTypesHelp')}>
          <input
            value={form.blocked_file_types.join(',')}
            onChange={(e) => setForm({ ...form, blocked_file_types: e.target.value.split(',') })}
          />
        </Field>
        <button type="submit" className="primary">
          {t('common.save')}
        </button>
      </form>
      <AppearanceSection />
      <ExampleSpaceSection />
      <h2>{t('admin.authTitle')}</h2>
      <p className="muted">{t('admin.authHelp')}</p>
      <p>{system.data?.oidc_configured ? t('admin.oidcOn') : t('admin.oidcOff')}</p>
    </>
  );
}

type MailSettings = Record<(typeof MAIL_SWITCHES)[number], boolean>;

function MailQueue({ entries }: { entries: MailEntry[] }) {
  const { t } = useTranslation();
  const dates = useDates();
  if (entries.length === 0) return <p className="muted">{t('admin.mailEmpty')}</p>;
  return (
    <TableBox>
      <table>
        <thead>
          <tr>
            <th>{t('admin.when')}</th>
            <th>{t('common.type')}</th>
            <th>{t('common.status')}</th>
            <th>{t('admin.mailAttempts')}</th>
            <th>{t('admin.mailError')}</th>
          </tr>
        </thead>
        <tbody>
          {entries.map((m) => (
            <tr key={m.id}>
              <td>{dates.moment(m.created_at)}</td>
              <td>{m.kind}</td>
              <td>{m.status}</td>
              <td>{m.attempts}</td>
              <td>{m.last_error}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableBox>
  );
}

function MailTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const settings = useQuery({
    queryKey: ['settings'],
    queryFn: () => api.get<MailSettings>('/admin/settings'),
  });
  const mail = useQuery({
    queryKey: ['mail-queue'],
    queryFn: () => api.get<{ configured: boolean; queue: MailEntry[] }>('/admin/mail'),
  });
  const save = useMutation({
    mutationFn: (change: Partial<MailSettings>) => api.put('/admin/settings', change),
    onSettled: () => client.invalidateQueries({ queryKey: ['settings'] }),
  });
  const test = useMutation({
    mutationFn: () => api.post('/admin/mail/test'),
    onSettled: () => client.invalidateQueries({ queryKey: ['mail-queue'] }),
  });
  if (settings.error || mail.error) return <ErrorNote error={settings.error ?? mail.error} />;
  if (!settings.data || !mail.data) return <Loading />;
  const current = settings.data;
  return (
    <>
      <ErrorNote error={save.error ?? test.error} />
      {!mail.data.configured && <p className="note">{t('admin.mailOff')}</p>}
      {MAIL_SWITCHES.map((name) => (
        <label key={name} className="check">
          <input
            type="checkbox"
            checked={current[name]}
            onChange={(e) => save.mutate({ [name]: e.target.checked })}
          />
          {t(`admin.${name}`)}
        </label>
      ))}
      <button type="button" disabled={!mail.data.configured} onClick={() => test.mutate()}>
        {t('admin.mailTest')}
      </button>
      {test.isSuccess && (
        <p role="status" className="note">
          {t('admin.mailTestQueued')}
        </p>
      )}
      <h2>{t('admin.mailQueue')}</h2>
      <MailQueue entries={mail.data.queue} />
    </>
  );
}

interface ScimToken {
  id: string;
  name: string;
  expires_at: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

function ScimTab() {
  const { t } = useTranslation();
  const dates = useDates();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const [until, setUntil] = useState('');
  const [secret, setSecret] = useState<string | null>(null);
  const q = useQuery({
    queryKey: ['scim-tokens'],
    queryFn: () => api.get<{ tokens: ScimToken[] }>('/admin/scim-tokens'),
  });
  const refresh = () => client.invalidateQueries({ queryKey: ['scim-tokens'] });
  const create = useMutation({
    mutationFn: () =>
      api.post<{ secret: string }>('/admin/scim-tokens', {
        name,
        expires_at: until ? new Date(until).toISOString() : undefined,
      }),
    onSuccess: async (r) => {
      setSecret(r.secret);
      setName('');
      setUntil('');
      await refresh();
    },
  });
  const rotate = useMutation({
    mutationFn: (id: string) => api.post<{ secret: string }>(`/admin/scim-tokens/${id}/rotate`),
    onSuccess: async (r) => {
      setSecret(r.secret);
      await refresh();
    },
  });
  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/admin/scim-tokens/${id}`),
    onSettled: refresh,
  });
  return (
    <>
      <p>
        {t('admin.scimUrl')}: <code>{window.location.origin + '/scim/v2'}</code>
      </p>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label={t('admin.tokenName')}
          placeholder={t('admin.tokenName')}
          required
        />
        <input
          type="date"
          value={until}
          aria-label={t('admin.tokenEnds')}
          title={t('admin.tokenEndsHelp')}
          onChange={(e) => setUntil(e.target.value)}
        />
        <button type="submit" className="primary">
          {t('admin.newToken')}
        </button>
      </form>
      <p className="muted">{t('admin.tokenEndsHelp')}</p>
      <ErrorNote error={q.error ?? create.error ?? revoke.error ?? rotate.error} />
      {q.data?.tokens.length === 0 ? (
        <p className="muted">{t('admin.noTokens')}</p>
      ) : (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('common.name')}</th>
                <th>{t('admin.created')}</th>
                <th>{t('admin.tokenEnds')}</th>
                <th>{t('admin.lastUsed')}</th>
                <th>{t('common.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {q.data?.tokens.map((k) => (
                <tr key={k.id}>
                  <td>{k.name}</td>
                  <td>{dates.moment(k.created_at)}</td>
                  <td>{dates.day(k.expires_at)}</td>
                  <td>{dates.moment(k.last_used_at)}</td>
                  <td className="row">
                    {k.revoked_at ? (
                      t('admin.revoked')
                    ) : (
                      <>
                        <button type="button" className="ghost" onClick={() => rotate.mutate(k.id)}>
                          {t('admin.rotate')}
                        </button>
                        <button type="button" className="ghost" onClick={() => revoke.mutate(k.id)}>
                          {t('admin.revoke')}
                        </button>
                      </>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
      {secret && (
        <Dialog title={t('admin.newToken')} onClose={() => setSecret(null)}>
          <p>{t('admin.tokenHelp')}</p>
          <p className="muted">{t('admin.rotateHelp')}</p>
          <code className="secret">{secret}</code>
        </Dialog>
      )}
    </>
  );
}

interface System {
  version: string;
  build_date: string;
  ready: string;
  storage: string;
  queue_healthy: boolean;
  nodes: { id: string; role: string; version: string; last_seen_at: string; live: boolean }[];
  migrations_applied: number;
  contract_pending: number;
  outbox_unpublished: number;
  jobs_queued: number;
  jobs_failed_7d: number;
  trash_items: number;
}

function SystemTab() {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['system'],
    queryFn: () => api.poll<System>('/admin/system'),
    refetchInterval: SYSTEM_POLL_MS,
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const s = q.data;
  const state = (ok: boolean) => (ok ? t('admin.ok') : t('admin.problem'));
  const rows: [string, string | number][] = [
    [t('admin.version'), s.version],
    ...(s.build_date ? [[t('health.builtOn'), s.build_date] as [string, string]] : []),
    [t('admin.ready'), s.ready],
    [t('admin.storage'), state(s.storage === 'ok')],
    [t('admin.queue'), state(s.queue_healthy)],
    [t('admin.migrations'), s.migrations_applied],
    [t('admin.contractPending'), s.contract_pending],
    [t('admin.outbox'), s.outbox_unpublished],
    [t('admin.jobsQueued'), s.jobs_queued],
    [t('admin.jobsFailed'), s.jobs_failed_7d],
    [t('admin.trashItems'), s.trash_items],
  ];
  return (
    <>
      <HealthChecks />
      <h2>{t('admin.nodesAndQueues')}</h2>
      <TableBox>
        <table>
          <tbody>
            {rows.map(([k, v]) => (
              <tr key={k}>
                <th scope="row">{k}</th>
                <td>{v}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
      <h2>{t('admin.nodes')}</h2>
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('admin.role')}</th>
              <th>{t('admin.version')}</th>
              <th>{t('admin.lastSeen')}</th>
              <th>{t('common.status')}</th>
            </tr>
          </thead>
          <tbody>
            {s.nodes.map((n) => (
              <tr key={n.id}>
                <td>{n.role}</td>
                <td>{n.version}</td>
                <td>{dates.moment(n.last_seen_at)}</td>
                <td>{n.live ? t('admin.live') : t('admin.gone')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </>
  );
}

export function AdminView({ tab, me }: { tab: string; me: Me }) {
  const { t } = useTranslation();
  const isKnownTab = TABS.some((name) => name === tab);
  if (!me.global_permissions.includes('administrator') || !isKnownTab) {
    return (
      <main id="main" className="plain reading">
        <p role="alert">{t('app.notFound')}</p>
      </main>
    );
  }
  return (
    <SettingsFrame
      title={t('admin.title')}
      head={<p className="settings-title">{t('admin.title')}</p>}
      search={<SettingsSearch />}
      groups={ADMIN_GROUPS.map((group) => ({
        id: group.id,
        title: t(`admin.group_${group.id}`),
        items: group.tabs.map((name) => ({ id: name, label: t(`admin.${name}`), href: `/admin/${name}` })),
      }))}
      current={tab}
    >
      <h1>{t(`admin.${TABS.find((name) => name === tab) ?? 'title'}`)}</h1>
      <HealthBand />
      {tab === 'summary' && <SummaryTab />}
      {tab === 'users' && <UsersTab />}
      {tab === 'invitations' && <InvitationsTab />}
      {tab === 'permissions' && <PermissionsTab />}
      {tab === 'rules' && <RulesTab />}
      {tab === 'look' && <LookTab />}
      {tab === 'care' && <CareTab me={me} />}
      {tab === 'groups' && <GroupsTab />}
      {tab === 'spaces' && <SpacesTab />}
      {tab === 'templates' && <TemplatesTab />}
      {tab === 'organising' && <OrganisingTab />}
      {tab === 'export' && <ExportTab />}
      {tab === 'audit' && <AuditTab />}
      {tab === 'settings' && <SettingsTab />}
      {tab === 'mail' && <MailTab />}
      {tab === 'scim' && <ScimTab />}
      {tab === 'system' && <SystemTab />}
      {tab === 'backups' && <BackupsTab />}
      {tab === 'configuration' && <ConfigurationTab />}
      {tab === 'sessions' && <SessionsTab />}
      {tab === 'insights' && <InsightsTab />}
      {tab === 'analytics' && <AnalyticsTab />}
      {tab === 'automation' && <RuleList spaceId={null} />}
      {tab === 'outgoing' && <OutgoingTab />}
      {tab === 'webhooks' && <WebhookList path="/admin/webhooks" />}
    </SettingsFrame>
  );
}
