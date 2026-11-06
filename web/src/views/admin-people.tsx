// SPDX-License-Identifier: Apache-2.0
// Accounts in the console beyond the plain list: invitations, service accounts, and the
// steps with lasting effect, which first show what they do.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Group, type UserDetail } from '../api';
import { appearanceQuery } from '../appearance';
import { useDates } from '../dates';
import { Dialog, ErrorNote, Field, FilePicker } from '../components/ui';

const DAY_MS = 86_400_000;
const SERVICE_TOKEN_DAYS = 365;

interface Invitation {
  id: string;
  email: string;
  type: string;
  expires_at: string;
}

function NewInvitation({ onLink }: { onLink: (link: string) => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [email, setEmail] = useState('');
  const [type, setType] = useState('member');
  const invite = useMutation({
    mutationFn: () => api.post<{ link: string }>('/admin/invitations', { email, type, groups: [] }),
    onSuccess: async (r) => {
      setEmail('');
      onLink(r.link);
      await client.invalidateQueries({ queryKey: ['admin-invitations'] });
    },
  });
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        invite.mutate();
      }}
    >
      <ErrorNote error={invite.error} />
      <Field label={t('common.email')}>
        <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
      </Field>
      <Field label={t('admin.userType')}>
        <select value={type} onChange={(e) => setType(e.target.value)}>
          <option value="member">{t('admin.member')}</option>
          <option value="guest">{t('admin.guest')}</option>
        </select>
      </Field>
      <button type="submit" className="primary" disabled={invite.isPending}>
        {t('access.invite')}
      </button>
    </form>
  );
}

interface ImportRow {
  line: number;
  email: string;
  type: string;
  problem: string;
}

/** A file of addresses becomes invitations, after a preview of each line. */
function ImportSection() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const preview = useMutation({
    mutationFn: (picked: File) => api.upload<{ rows: ImportRow[] }>('/admin/users/import?preview=1', picked),
  });
  const run = useMutation({
    mutationFn: (picked: File) => api.upload<{ rows: ImportRow[] }>('/admin/users/import', picked),
    onSuccess: () => client.invalidateQueries({ queryKey: ['admin-invitations'] }),
  });
  const rows = run.data?.rows ?? preview.data?.rows;
  return (
    <section>
      <h2>{t('admin.import')}</h2>
      <p className="muted">{t('admin.importHelp')}</p>
      <ErrorNote error={preview.error ?? run.error} />
      <FilePicker
        label={t('admin.importFile')}
        accept=".csv,text/csv"
        onPick={(picked) => {
          setFile(picked);
          run.reset();
          preview.mutate(picked);
        }}
      />
      {rows && (
        <table>
          <thead>
            <tr>
              <th>{t('admin.importLine')}</th>
              <th>{t('common.email')}</th>
              <th>{t('common.type')}</th>
              <th>{t('common.status')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.line}>
                <td>{r.line}</td>
                <td>{r.email}</td>
                <td>{r.type}</td>
                <td>{r.problem ? t(`admin.import_${r.problem}`, r.problem) : t('admin.importGood')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {file && rows && !run.data && (
        <button type="button" className="primary" disabled={run.isPending} onClick={() => run.mutate(file)}>
          {t('admin.importRun')}
        </button>
      )}
    </section>
  );
}

export function InvitationsTab() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const dates = useDates();
  const [link, setLink] = useState('');
  const mailReady = useQuery(appearanceQuery).data?.mail_ready;
  const q = useQuery({
    queryKey: ['admin-invitations'],
    queryFn: () => api.get<{ invitations: Invitation[] }>('/admin/invitations'),
  });
  const cancel = useMutation({
    mutationFn: (id: string) => api.del(`/admin/invitations/${id}`),
    onSettled: () => client.invalidateQueries({ queryKey: ['admin-invitations'] }),
  });
  return (
    <>
      {mailReady === false && <p className="note">{t('admin.invitationNoMail')}</p>}
      <NewInvitation onLink={setLink} />
      {link && (
        <p role="status" className="note">
          {t('access.inviteLink', { link })}
        </p>
      )}
      <ErrorNote error={q.error ?? cancel.error} />
      {q.data?.invitations.length === 0 && <p className="muted">{t('admin.invitationsNone')}</p>}
      <ImportSection />
      {(q.data?.invitations.length ?? 0) > 0 && (
        <table>
          <thead>
            <tr>
              <th>{t('common.email')}</th>
              <th>{t('common.type')}</th>
              <th>{t('account.tokenExpires')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.invitations.map((i) => (
              <tr key={i.id}>
                <td>{i.email}</td>
                <td>{i.type}</td>
                <td>{dates.day(i.expires_at)}</td>
                <td>
                  <button type="button" className="ghost" onClick={() => cancel.mutate(i.id)}>
                    {t('common.cancel')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

/** Shows what turning a guest into a member and back changes, before it happens. */
export function TypeChange({ user, onDone }: { user: UserDetail; onDone: () => void }) {
  const { t } = useTranslation();
  const [asking, setAsking] = useState(false);
  const target = user.type === 'guest' ? 'member' : 'guest';
  const impact = useQuery({
    queryKey: ['type-impact', user.id],
    queryFn: () =>
      api.get<{ spaces: string[]; permissions: string[] }>(`/admin/users/${user.id}/type-impact`),
    enabled: asking,
  });
  const change = useMutation({
    mutationFn: () => api.patch(`/admin/users/${user.id}`, { display_name: user.display_name, type: target }),
    onSuccess: onDone,
  });
  const label = target === 'member' ? t('admin.makeMember') : t('admin.makeGuest');
  return (
    <>
      <button type="button" onClick={() => setAsking(true)}>
        {label}
      </button>
      {asking && (
        <Dialog title={label} onClose={() => setAsking(false)}>
          <ErrorNote error={impact.error ?? change.error} />
          {impact.data && (
            <>
              <p>{t('admin.typeImpactSpaces', { list: impact.data.spaces.join(', ') || '-' })}</p>
              <p>{t('admin.typeImpactPermissions', { list: impact.data.permissions.join(', ') || '-' })}</p>
            </>
          )}
          <div className="dialog-actions">
            <button type="button" className="primary" onClick={() => change.mutate()}>
              {label}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}

/** Erasure needs a reason, and says what it cannot reach (spec 013, D10). */
export function AnonymiseButton({ user, onDone }: { user: UserDetail; onDone: () => void }) {
  const { t } = useTranslation();
  const [asking, setAsking] = useState(false);
  const [reason, setReason] = useState('');
  const go = useMutation({
    mutationFn: () => api.post(`/admin/users/${user.id}/anonymise`, { reason }),
    onSuccess: onDone,
  });
  return (
    <>
      <button type="button" className="danger" onClick={() => setAsking(true)}>
        {t('admin.anonymise')}
      </button>
      {asking && (
        <Dialog title={t('admin.anonymise')} onClose={() => setAsking(false)}>
          <p>{t('admin.anonymiseHelp')}</p>
          <p className="muted">{t('admin.anonymiseLimits')}</p>
          <form
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              go.mutate();
            }}
          >
            <ErrorNote error={go.error} />
            <Field label={t('admin.reason')}>
              <input value={reason} onChange={(e) => setReason(e.target.value)} required autoFocus />
            </Field>
            <div className="dialog-actions">
              <button type="submit" className="danger" disabled={go.isPending}>
                {t('admin.anonymise')}
              </button>
            </div>
          </form>
        </Dialog>
      )}
    </>
  );
}

/** The date a guest stops being able to work here. */
export function ExpiryField({ user, onDone }: { user: UserDetail; onDone: () => void }) {
  const { t } = useTranslation();
  const [until, setUntil] = useState(user.expires_at?.slice(0, 10) ?? '');
  const save = useMutation({
    mutationFn: () =>
      api.put(`/admin/users/${user.id}/expiry`, { expires_at: until ? new Date(until).toISOString() : null }),
    onSuccess: onDone,
  });
  return (
    <form
      className="row"
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      <Field label={t('admin.expiry')}>
        <input type="date" value={until} onChange={(e) => setUntil(e.target.value)} />
      </Field>
      <button type="submit">{t('common.save')}</button>
    </form>
  );
}

/** Acts on the rows a person chose, after saying how many. */
export function BulkBar({ users, onDone }: { users: string[]; onDone: () => void }) {
  const { t } = useTranslation();
  const [asking, setAsking] = useState<'disable' | 'add_to_group' | null>(null);
  const [group, setGroup] = useState('');
  const groups = useQuery({
    queryKey: ['admin-groups'],
    queryFn: () => api.get<{ groups: Group[] }>('/admin/groups'),
    enabled: asking === 'add_to_group',
  });
  const run = useMutation({
    mutationFn: () =>
      api.post<{ done: number; failed: number }>('/admin/users/bulk', {
        action: asking,
        users,
        group_id: asking === 'add_to_group' ? group : undefined,
      }),
    onSuccess: () => {
      setAsking(null);
      onDone();
    },
  });
  return (
    <div className="row">
      <span className="muted">{t('admin.selected', { count: users.length })}</span>
      <button type="button" onClick={() => setAsking('disable')}>
        {t('admin.disable')}
      </button>
      <button type="button" onClick={() => setAsking('add_to_group')}>
        {t('admin.addToGroup')}
      </button>
      <ErrorNote error={run.error} />
      {run.data && (
        <span role="status" className="muted">
          {t('admin.bulkDone', { done: run.data.done, failed: run.data.failed })}
        </span>
      )}
      {asking && (
        <Dialog
          title={asking === 'disable' ? t('admin.disable') : t('admin.addToGroup')}
          onClose={() => setAsking(null)}
        >
          <p>{t('admin.bulkConfirm', { count: users.length })}</p>
          {asking === 'add_to_group' && (
            <Field label={t('admin.groups')}>
              <select value={group} onChange={(e) => setGroup(e.target.value)} required>
                <option value="" disabled>
                  {t('admin.chooseGroup')}
                </option>
                {groups.data?.groups.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.name}
                  </option>
                ))}
              </select>
            </Field>
          )}
          <div className="dialog-actions">
            <button
              type="button"
              className="primary"
              disabled={asking === 'add_to_group' && !group}
              onClick={() => run.mutate()}
            >
              {t('common.save')}
            </button>
          </div>
        </Dialog>
      )}
    </div>
  );
}

export function ServiceAccounts() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const [secret, setSecret] = useState('');
  const q = useQuery({
    queryKey: ['admin-users', 'service'],
    queryFn: () => api.get<{ users: UserDetail[] }>('/admin/users?type=service'),
  });
  const create = useMutation({
    mutationFn: () => api.post('/admin/service-accounts', { name }),
    onSuccess: async () => {
      setName('');
      await client.invalidateQueries({ queryKey: ['admin-users', 'service'] });
    },
  });
  const token = useMutation({
    mutationFn: (id: string) =>
      api.post<{ secret: string }>(`/admin/service-accounts/${id}/tokens`, {
        name: t('admin.serviceToken'),
        can_write: false,
        expires_at: new Date(Date.now() + SERVICE_TOKEN_DAYS * DAY_MS).toISOString(),
      }),
    onSuccess: (r) => setSecret(r.secret),
  });
  return (
    <section>
      <h2>{t('admin.serviceAccounts')}</h2>
      <p className="muted">{t('admin.serviceAccountsHelp')}</p>
      <ErrorNote error={q.error ?? create.error ?? token.error} />
      <ul className="plain-list">
        {q.data?.users.map((u) => (
          <li key={u.id} className="row">
            <span className="grow">{u.display_name}</span>
            <button type="button" className="ghost" onClick={() => token.mutate(u.id)}>
              {t('admin.newToken')}
            </button>
          </li>
        ))}
      </ul>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <input
          value={name}
          aria-label={t('admin.serviceName')}
          placeholder={t('admin.serviceName')}
          onChange={(e) => setName(e.target.value)}
          required
        />
        <button type="submit">{t('common.add')}</button>
      </form>
      {secret && (
        <Dialog title={t('admin.newToken')} onClose={() => setSecret('')}>
          <p>{t('admin.tokenHelp')}</p>
          <code className="secret">{secret}</code>
        </Dialog>
      )}
    </section>
  );
}
