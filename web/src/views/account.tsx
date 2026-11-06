// SPDX-License-Identifier: Apache-2.0
// Your account: who you are to others, your tokens and your data.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me, type MemberGroup, type Profile, type Token } from '../api';
import { useDates } from '../dates';
import { Link, useLocation } from '../router';
import { CropDialog } from '../components/crop-dialog';
import { IconLock } from '../components/icons';
import { PersonPicture } from '../components/person';
import { useCopy } from '../components/toast';
import { ConfirmButton, Dialog, ErrorNote, Field, FilePicker, Loading } from '../components/ui';
import type { Crop } from '../crop';

const SECTIONS = [
  ['/me', 'sectionProfile'],
  ['/me/preferences', 'sectionPreferences'],
  ['/me/security', 'sectionSecurity'],
  ['/me/tokens', 'sectionTokens'],
  ['/me/data', 'sectionData'],
] as const;
const PROFILE_FIELDS = ['job_title', 'department', 'location'] as const;
const VISIBILITY = ['everyone', 'members', 'me'] as const;
const DAY_MS = 86_400_000;
const DEFAULT_TOKEN_DAYS = 90;
const MAX_TOKEN_DAYS = 365;
const LOCK_ICON_SIZE = 16;
const PICTURE_TYPES = 'image/png,image/jpeg,image/gif';

/** The sections of Your account. */
export function AccountNav() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  return (
    <nav className="tabs account-tabs" aria-label={t('account.sections')}>
      {SECTIONS.map(([href, label]) => (
        <Link key={href} href={href} aria-current={pathname === href ? 'page' : undefined}>
          {t(`account.${label}`)}
        </Link>
      ))}
    </nav>
  );
}

function PictureSection({ profile }: { profile: Profile }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['profile', profile.id] });
    await client.invalidateQueries({ queryKey: ['person', profile.id] });
  };
  const save = useMutation({
    mutationFn: (crop: Crop) => {
      if (!file) throw new Error(t('app.error'));
      return api.replace(`/me/picture?x=${crop.x}&y=${crop.y}&size=${crop.size}`, file);
    },
    onSuccess: async () => {
      setFile(null);
      await refresh();
    },
  });
  const remove = useMutation({ mutationFn: () => api.del('/me/picture'), onSuccess: refresh });
  return (
    <section>
      <h2>{t('account.picture')}</h2>
      <div className="person-head">
        <PersonPicture person={profile} size="large" />
        <div className="row">
          <FilePicker label={t('account.pictureUpload')} accept={PICTURE_TYPES} onPick={setFile} />
          {profile.has_picture && (
            <button type="button" onClick={() => remove.mutate()}>
              {t('common.remove')}
            </button>
          )}
        </div>
      </div>
      <p className="muted">{t('account.pictureHelp')}</p>
      <ErrorNote error={save.error ?? remove.error} />
      {file && (
        <CropDialog
          file={file}
          busy={save.isPending}
          onSave={(crop) => save.mutate(crop)}
          onClose={() => setFile(null)}
        />
      )}
    </section>
  );
}

function VisibilitySelect({
  field,
  value,
  onChange,
}: {
  field: string;
  value: string;
  onChange: (next: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <select
      aria-label={t('account.whoSees', { field: t(`people.field_${field}`) })}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      {VISIBILITY.map((v) => (
        <option key={v} value={v}>
          {t(`account.visible_${v}`)}
        </option>
      ))}
    </select>
  );
}

function ProfileForm({ profile }: { profile: Profile }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [form, setForm] = useState({
    job_title: profile.job_title,
    department: profile.department,
    location: profile.location,
    about: profile.about,
  });
  const [visibility, setVisibility] = useState(profile.visibility ?? {});
  const locked = profile.locked_fields ?? [];
  const save = useMutation({
    mutationFn: () => api.put('/me/profile', { ...form, visibility }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['profile', profile.id] });
      await client.invalidateQueries({ queryKey: ['person', profile.id] });
    },
  });
  const see = (field: string) => (next: string) => setVisibility({ ...visibility, [field]: next });
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <ErrorNote error={save.error} />
      {save.isSuccess && (
        <p role="status" className="note">
          {t('admin.saved')}
        </p>
      )}
      {PROFILE_FIELDS.map((field) => (
        <div className="profile-field" key={field}>
          <Field label={t(`people.field_${field}`)}>
            <input
              value={form[field]}
              disabled={locked.includes(field)}
              onChange={(e) => setForm({ ...form, [field]: e.target.value })}
            />
          </Field>
          <VisibilitySelect field={field} value={visibility[field] ?? 'everyone'} onChange={see(field)} />
          {locked.includes(field) && (
            <small className="muted">
              <IconLock size={LOCK_ICON_SIZE} /> {t('account.lockedByProvider')}
            </small>
          )}
        </div>
      ))}
      <div className="profile-field">
        <Field label={t('people.field_about')}>
          <textarea
            rows={3}
            value={form.about}
            onChange={(e) => setForm({ ...form, about: e.target.value })}
          />
        </Field>
        <VisibilitySelect field="about" value={visibility.about ?? 'everyone'} onChange={see('about')} />
      </div>
      <div className="profile-field">
        <Field label={t('common.email')}>
          <input value={profile.email} disabled />
        </Field>
        <VisibilitySelect field="email" value={visibility.email ?? 'members'} onChange={see('email')} />
      </div>
      <button type="submit" className="primary" disabled={save.isPending}>
        {t('common.save')}
      </button>
    </form>
  );
}

function OwnGroups() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['my-groups'],
    queryFn: () => api.get<{ groups: MemberGroup[] }>('/me/groups'),
  });
  if ((q.data?.groups.length ?? 0) === 0) return <ErrorNote error={q.error} />;
  return (
    <section>
      <h2>{t('account.groups')}</h2>
      <ul className="plain-list">
        {q.data?.groups.map((g) => (
          <li key={g.id}>
            {g.owner ? <Link href={`/groups/${g.id}`}>{g.name}</Link> : g.name}{' '}
            {g.owner && <span className="lozenge">{t('account.groupOwner')}</span>}
          </li>
        ))}
      </ul>
    </section>
  );
}

export function ProfileView({ me }: { me: Me }) {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['profile', me.user.id],
    queryFn: () => api.get<Profile>(`/people/${me.user.id}`),
  });
  return (
    <>
      <h1>{t('profile.title')}</h1>
      <AccountNav />
      <p>
        {me.user.display_name} <span className="muted">{me.user.email}</span>
      </p>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && (
        <>
          <PictureSection profile={q.data} />
          <section>
            <h2>{t('account.aboutYou')}</h2>
            <ProfileForm profile={q.data} />
          </section>
        </>
      )}
      <OwnGroups />
    </>
  );
}

function NewTokenForm({ onCreated }: { onCreated: (secret: string) => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const dayAfter = (days: number) => new Date(Date.now() + days * DAY_MS).toISOString().slice(0, 10);
  const [name, setName] = useState('');
  const [canWrite, setCanWrite] = useState(false);
  const [until, setUntil] = useState(() => dayAfter(DEFAULT_TOKEN_DAYS));
  const create = useMutation({
    mutationFn: () =>
      api.post<{ secret: string }>('/me/tokens', {
        name,
        can_write: canWrite,
        expires_at: new Date(until).toISOString(),
      }),
    onSuccess: async (r) => {
      setName('');
      onCreated(r.secret);
      await client.invalidateQueries({ queryKey: ['tokens'] });
    },
  });
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <ErrorNote error={create.error} />
      <Field label={t('common.name')}>
        <input value={name} onChange={(e) => setName(e.target.value)} required />
      </Field>
      <Field label={t('account.tokenExpires')} help={t('account.tokenExpiresHelp')}>
        <input
          type="date"
          value={until}
          min={dayAfter(1)}
          max={dayAfter(MAX_TOKEN_DAYS)}
          onChange={(e) => setUntil(e.target.value)}
          required
        />
      </Field>
      <Field label={t('account.tokenScope')}>
        <select value={canWrite ? 'write' : 'read'} onChange={(e) => setCanWrite(e.target.value === 'write')}>
          <option value="read">{t('account.tokenRead')}</option>
          <option value="write">{t('account.tokenWrite')}</option>
        </select>
      </Field>
      <button type="submit" className="primary" disabled={create.isPending}>
        {t('account.tokenNew')}
      </button>
    </form>
  );
}

export function TokensView() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const dates = useDates();
  const copy = useCopy();
  const [secret, setSecret] = useState<string | null>(null);
  const q = useQuery({ queryKey: ['tokens'], queryFn: () => api.get<{ tokens: Token[] }>('/me/tokens') });
  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/me/tokens/${id}`),
    onSettled: () => client.invalidateQueries({ queryKey: ['tokens'] }),
  });
  return (
    <>
      <h1>{t('profile.title')}</h1>
      <AccountNav />
      <p className="muted">{t('account.tokensHelp')}</p>
      <ErrorNote error={q.error ?? revoke.error} />
      {q.data?.tokens.length === 0 && <p className="muted">{t('account.tokensNone')}</p>}
      {(q.data?.tokens.length ?? 0) > 0 && (
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('account.tokenScope')}</th>
              <th>{t('account.tokenExpires')}</th>
              <th>{t('admin.lastUsed')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.tokens.map((k) => (
              <tr key={k.id}>
                <td>{k.name}</td>
                <td>{k.can_write ? t('account.tokenWrite') : t('account.tokenRead')}</td>
                <td>{dates.day(k.expires_at)}</td>
                <td>{dates.moment(k.last_used_at)}</td>
                <td>
                  <ConfirmButton
                    label={t('admin.revoke')}
                    message={t('account.tokenRevokeHelp', { name: k.name })}
                    className="ghost"
                    onConfirm={() => revoke.mutate(k.id)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <section>
        <h2>{t('account.tokenNew')}</h2>
        <NewTokenForm onCreated={setSecret} />
      </section>
      {secret && (
        <Dialog title={t('account.tokenNew')} onClose={() => setSecret(null)}>
          <p>{t('admin.tokenHelp')}</p>
          <code className="secret" data-testid="token-secret">
            {secret}
          </code>
          <div className="dialog-actions">
            <button type="button" onClick={() => void copy(secret, t('account.tokenCopied'))}>
              {t('account.tokenCopy')}
            </button>
          </div>
        </Dialog>
      )}
    </>
  );
}

function saveAsFile(name: string, content: unknown) {
  const url = URL.createObjectURL(new Blob([JSON.stringify(content, null, 2)], { type: 'application/json' }));
  const link = document.createElement('a');
  link.href = url;
  link.download = name;
  link.click();
  URL.revokeObjectURL(url);
}

export function PersonalDataView() {
  const { t } = useTranslation();
  const download = useMutation({
    mutationFn: () => api.post<unknown>('/me/export'),
    onSuccess: (data) => saveAsFile('my-diffluence-data.json', data),
  });
  const removal = useMutation({ mutationFn: () => api.post('/me/removal-request') });
  return (
    <>
      <h1>{t('profile.title')}</h1>
      <AccountNav />
      <section>
        <h2>{t('account.download')}</h2>
        <p className="muted">{t('account.downloadHelp')}</p>
        <ErrorNote error={download.error} />
        <button type="button" disabled={download.isPending} onClick={() => download.mutate()}>
          {t('account.download')}
        </button>
      </section>
      <section>
        <h2>{t('account.removal')}</h2>
        <p className="muted">{t('account.removalHelp')}</p>
        <ErrorNote error={removal.error} />
        {removal.isSuccess ? (
          <p role="status" className="note">
            {t('account.removalSent')}
          </p>
        ) : (
          <ConfirmButton
            label={t('account.removal')}
            message={t('account.removalConfirm')}
            onConfirm={() => removal.mutate()}
          />
        )}
      </section>
    </>
  );
}
