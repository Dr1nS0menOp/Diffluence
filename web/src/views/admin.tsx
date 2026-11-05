// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  api,
  GLOBAL_PERMISSIONS,
  type AuditEvent,
  type Group,
  type MailEntry,
  type Me,
  type Space,
  type TrashItem,
  type UserDetail,
} from '../api';
import { Link, navigate } from '../router';
import { SubjectPicker } from '../components/picker';
import { ConfirmButton, Dialog, ErrorNote, Field, formatDate, Loading } from '../components/ui';
import { AppearanceSection } from './admin-appearance';

const TABS = ['users', 'groups', 'spaces', 'audit', 'settings', 'mail', 'scim', 'system'] as const;
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
          {p}
        </label>
      ))}
    </fieldset>
  );
}

function UserDialog({ user, onClose }: { user: UserDetail; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [perms, setPerms] = useState(user.global_permissions);
  const [password, setPassword] = useState('');
  const refresh = () => client.invalidateQueries({ queryKey: ['admin-users'] });
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
        {user.email} · {user.source}
      </p>
      <ErrorNote error={act.error ?? save.error ?? reset.error} />
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
      </div>
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

function UsersTab() {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [adding, setAdding] = useState(false);
  const [open, setOpen] = useState<string | null>(null);
  const q = useQuery({
    queryKey: ['admin-users', text],
    queryFn: () =>
      api.get<{ users: UserDetail[]; total: number }>('/admin/users?q=' + encodeURIComponent(text)),
  });
  const selected = q.data?.users.find((u) => u.id === open);
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
      <ErrorNote error={q.error} />
      <table>
        <thead>
          <tr>
            <th>{t('common.name')}</th>
            <th>{t('common.email')}</th>
            <th>{t('common.type')}</th>
            <th>{t('admin.source')}</th>
            <th>{t('common.status')}</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.users.map((u) => (
            <tr key={u.id}>
              <td>
                <button type="button" className="ghost" onClick={() => setOpen(u.id)}>
                  {u.display_name}
                </button>
              </td>
              <td>{u.email}</td>
              <td>{u.type}</td>
              <td>{u.source}</td>
              <td>{u.status === 'active' ? t('admin.active') : t('admin.disabled')}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {adding && <NewUserDialog onClose={() => setAdding(false)} />}
      {selected && <UserDialog user={selected} onClose={() => setOpen(null)} />}
    </>
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
  const managed = group.source !== 'local';
  return (
    <Dialog title={group.name} onClose={onClose}>
      {managed && <p className="note">{t('admin.managed')}</p>}
      <ErrorNote error={add.error ?? remove.error ?? save.error ?? del.error} />
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
      <GlobalPermissions
        current={perms}
        onChange={(next) => {
          setPerms(next);
          save.mutate(next);
        }}
      />
      {!managed && (
        <button type="button" className="danger" onClick={() => del.mutate()}>
          {t('common.delete')}
        </button>
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
      {selected && <GroupDialog group={selected} onClose={() => setOpen(null)} />}
    </>
  );
}

function SpacesTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['admin-spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/admin/spaces'),
  });
  const trashed = useQuery({
    queryKey: ['admin-trash'],
    queryFn: () => api.get<{ items: TrashItem[] }>('/admin/trash/spaces'),
  });
  const [recovering, setRecovering] = useState<Space | null>(null);
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
  return (
    <>
      <ErrorNote error={q.error ?? restore.error ?? purge.error} />
      <table>
        <thead>
          <tr>
            <th>{t('common.name')}</th>
            <th>{t('space.key')}</th>
            <th>{t('common.status')}</th>
            <th>{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.spaces.map((s) => (
            <tr key={s.id}>
              <td>
                <Link href={`/spaces/${s.key}`}>{s.name}</Link>
              </td>
              <td>{s.key}</td>
              <td>{s.status}</td>
              <td>
                <button type="button" className="ghost" onClick={() => setRecovering(s)}>
                  {t('admin.recover')}
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {(trashed.data?.items.length ?? 0) > 0 && (
        <>
          <h2>{t('admin.trashedSpaces')}</h2>
          <ul className="plain-list">
            {trashed.data?.items.map((i) => (
              <li key={i.id} className="row">
                {i.title} <span className="muted">{formatDate(i.purge_after)}</span>
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

function AuditTab() {
  const { t } = useTranslation();
  const [action, setAction] = useState('');
  const q = useQuery({
    queryKey: ['audit', action],
    queryFn: () =>
      api.get<{ events: AuditEvent[] }>(
        '/admin/audit-events?limit=100' + (action ? '&action=' + encodeURIComponent(action) : ''),
      ),
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
          value={action}
          onChange={(e) => setAction(e.target.value)}
          aria-label={t('admin.filterAction')}
          placeholder={t('admin.filterAction')}
        />
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
            <tr key={e.seq}>
              <td>{formatDate(e.occurred_at)}</td>
              <td>{e.actor_id || e.actor_type}</td>
              <td>{e.action}</td>
              <td>{e.object_type}</td>
              <td>{e.result}</td>
              <td>{e.ip}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

interface Settings {
  trash_retention_days: number;
  max_upload_mib: number;
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
  if (entries.length === 0) return <p className="muted">{t('admin.mailEmpty')}</p>;
  return (
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
            <td>{formatDate(m.created_at)}</td>
            <td>{m.kind}</td>
            <td>{m.status}</td>
            <td>{m.attempts}</td>
            <td>{m.last_error}</td>
          </tr>
        ))}
      </tbody>
    </table>
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
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
}

function ScimTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const [secret, setSecret] = useState<string | null>(null);
  const q = useQuery({
    queryKey: ['scim-tokens'],
    queryFn: () => api.get<{ tokens: ScimToken[] }>('/admin/scim-tokens'),
  });
  const refresh = () => client.invalidateQueries({ queryKey: ['scim-tokens'] });
  const create = useMutation({
    mutationFn: () => api.post<{ secret: string }>('/admin/scim-tokens', { name }),
    onSuccess: async (r) => {
      setSecret(r.secret);
      setName('');
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
        <button type="submit" className="primary">
          {t('admin.newToken')}
        </button>
      </form>
      <ErrorNote error={q.error ?? create.error ?? revoke.error} />
      <table>
        <thead>
          <tr>
            <th>{t('common.name')}</th>
            <th>{t('admin.created')}</th>
            <th>{t('admin.lastUsed')}</th>
            <th>{t('common.actions')}</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.tokens.map((k) => (
            <tr key={k.id}>
              <td>{k.name}</td>
              <td>{formatDate(k.created_at)}</td>
              <td>{formatDate(k.last_used_at)}</td>
              <td>
                {k.revoked_at ? (
                  t('admin.revoked')
                ) : (
                  <button type="button" className="ghost" onClick={() => revoke.mutate(k.id)}>
                    {t('admin.revoke')}
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {secret && (
        <Dialog title={t('admin.newToken')} onClose={() => setSecret(null)}>
          <p>{t('admin.tokenHelp')}</p>
          <code className="secret">{secret}</code>
        </Dialog>
      )}
    </>
  );
}

interface System {
  version: string;
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
  const q = useQuery({
    queryKey: ['system'],
    queryFn: () => api.get<System>('/admin/system'),
    refetchInterval: 10_000,
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const s = q.data;
  const state = (ok: boolean) => (ok ? t('admin.ok') : t('admin.problem'));
  const rows: [string, string | number][] = [
    [t('admin.version'), s.version],
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
      <h2>{t('admin.nodes')}</h2>
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
              <td>{formatDate(n.last_seen_at)}</td>
              <td>{n.live ? t('admin.live') : t('admin.gone')}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

export function AdminView({ tab, me }: { tab: string; me: Me }) {
  const { t } = useTranslation();
  if (!me.global_permissions.includes('administrator')) {
    return (
      <main id="main" className="plain reading">
        <p role="alert">{t('app.notFound')}</p>
      </main>
    );
  }
  return (
    <div className="split">
      <aside className="sidebar">
        <nav aria-label={t('admin.title')}>
          {TABS.map((name) => (
            <Link
              key={name}
              href={`/admin/${name}`}
              className="side-link"
              aria-current={name === tab ? 'page' : undefined}
            >
              {t(`admin.${name}`)}
            </Link>
          ))}
        </nav>
      </aside>
      <main id="main" className="content">
        <div className="column wide">
          <h1>{t(`admin.${TABS.find((name) => name === tab) ?? 'title'}`)}</h1>
          {tab === 'users' && <UsersTab />}
          {tab === 'groups' && <GroupsTab />}
          {tab === 'spaces' && <SpacesTab />}
          {tab === 'audit' && <AuditTab />}
          {tab === 'settings' && <SettingsTab />}
          {tab === 'mail' && <MailTab />}
          {tab === 'scim' && <ScimTab />}
          {tab === 'system' && <SystemTab />}
        </div>
      </main>
    </div>
  );
}
