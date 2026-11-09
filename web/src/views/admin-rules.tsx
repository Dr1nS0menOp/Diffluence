// SPDX-License-Identifier: Apache-2.0
// The typed settings of the console, one form per section (spec 013, F11), and who
// holds which global permission.
import { useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Category } from '../api';
import { ErrorNote, Field, Loading } from '../components/ui';
import { permissionLabel } from '../permission-words';
import { RolesTab } from './roles';

const FEATURES = [
  'comments',
  'reactions',
  'blogs',
  'invitations',
  'personal_spaces',
  'self_signup',
  'analytics',
  'automation',
  'tokens',
  'presence',
] as const;
const SECOND_FACTOR_RULES = ['administrators_after_grace', 'none', 'administrators', 'members'];

interface General {
  default_language: string;
  default_time_zone: string;
  help_url: string;
  embed_hosts: string[];
}

type Features = Record<(typeof FEATURES)[number], boolean>;

interface Access {
  second_factor: string;
  idle_minutes: number;
  signup_domains: string[];
  guest_invites: boolean;
}

interface Retention {
  audit_days: number;
  longer_because: string;
}

/** Loads a section, and hands the form to the screen once it is there. */
export function SectionForm<T extends object>({
  section,
  title,
  children,
}: {
  section: string;
  title: string;
  children: (form: T, set: (change: Partial<T>) => void) => ReactNode;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const key = ['section', section];
  const q = useQuery({ queryKey: key, queryFn: () => api.get<T>(`/admin/settings/${section}`) });
  const [changes, setChanges] = useState<Partial<T>>({});
  const save = useMutation({
    mutationFn: (form: T) => api.put<T>(`/admin/settings/${section}`, form),
    onSuccess: async () => {
      setChanges({});
      await client.invalidateQueries({ queryKey: key });
      await client.invalidateQueries({ queryKey: ['appearance'] });
    },
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const form: T = { ...q.data, ...changes };
  return (
    <section>
      <h2>{title}</h2>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          save.mutate(form);
        }}
      >
        <ErrorNote error={save.error} />
        {save.isSuccess && (
          <p role="status" className="note">
            {t('admin.saved')}
          </p>
        )}
        {children(form, (change) => setChanges({ ...changes, ...change }))}
        <button type="submit" className="primary" disabled={save.isPending}>
          {t('common.save')}
        </button>
      </form>
    </section>
  );
}

function Switch({
  label,
  value,
  onChange,
}: {
  label: string;
  value: boolean;
  onChange: (next: boolean) => void;
}) {
  return (
    <label className="check">
      <input type="checkbox" checked={value} onChange={(e) => onChange(e.target.checked)} />
      {label}
    </label>
  );
}

function GeneralForm() {
  const { t } = useTranslation();
  return (
    <SectionForm<General> section="general" title={t('rules.general')}>
      {(form, set) => (
        <>
          <Field label={t('rules.defaultLanguage')}>
            <select value={form.default_language} onChange={(e) => set({ default_language: e.target.value })}>
              <option value="en">{t('preferences.language_en')}</option>
            </select>
          </Field>
          <Field label={t('rules.defaultTimeZone')}>
            <select
              value={form.default_time_zone}
              onChange={(e) => set({ default_time_zone: e.target.value })}
            >
              {['UTC', ...Intl.supportedValuesOf('timeZone')].map((zone) => (
                <option key={zone} value={zone}>
                  {zone}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t('rules.helpUrl')}>
            <input type="url" value={form.help_url} onChange={(e) => set({ help_url: e.target.value })} />
          </Field>
          <Field label={t('rules.embedHosts')} help={t('rules.embedHostsHelp')}>
            <textarea
              rows={2}
              value={form.embed_hosts.join('\n')}
              onChange={(e) => set({ embed_hosts: e.target.value.split('\n').filter(Boolean) })}
            />
          </Field>
        </>
      )}
    </SectionForm>
  );
}

function FeaturesForm() {
  const { t } = useTranslation();
  return (
    <SectionForm<Features> section="features" title={t('rules.features')}>
      {(form, set) => (
        <>
          <p className="muted">{t('rules.featuresHelp')}</p>
          {FEATURES.map((feature) => (
            <Switch
              key={feature}
              label={t(`rules.feature_${feature}`)}
              value={form[feature]}
              onChange={(on) => set({ [feature]: on })}
            />
          ))}
        </>
      )}
    </SectionForm>
  );
}

function AccessForm() {
  const { t } = useTranslation();
  return (
    <SectionForm<Access> section="access" title={t('rules.access')}>
      {(form, set) => (
        <>
          <Field label={t('rules.secondFactor')} help={t('rules.secondFactorHelp')}>
            <select value={form.second_factor} onChange={(e) => set({ second_factor: e.target.value })}>
              {SECOND_FACTOR_RULES.map((rule) => (
                <option key={rule} value={rule}>
                  {t(`rules.secondFactor_${rule}`)}
                </option>
              ))}
            </select>
          </Field>
          <Field label={t('rules.idleMinutes')} help={t('rules.idleMinutesHelp')}>
            <input
              type="number"
              min={0}
              value={form.idle_minutes}
              onChange={(e) => set({ idle_minutes: Number(e.target.value) })}
            />
          </Field>
          <Field label={t('rules.signupDomains')} help={t('rules.signupDomainsHelp')}>
            <textarea
              rows={2}
              value={form.signup_domains.join('\n')}
              onChange={(e) => set({ signup_domains: e.target.value.split('\n').filter(Boolean) })}
            />
          </Field>
          <Switch
            label={t('rules.guestInvites')}
            value={form.guest_invites}
            onChange={(guest_invites) => set({ guest_invites })}
          />
        </>
      )}
    </SectionForm>
  );
}

function RetentionForm() {
  const { t } = useTranslation();
  return (
    <SectionForm<Retention> section="retention" title={t('rules.retention')}>
      {(form, set) => (
        <>
          <Field label={t('rules.auditDays')} help={t('rules.auditDaysHelp')}>
            <input
              type="number"
              min={0}
              value={form.audit_days}
              onChange={(e) => set({ audit_days: Number(e.target.value) })}
            />
          </Field>
          <Field label={t('rules.longerBecause')} help={t('rules.longerBecauseHelp')}>
            <input value={form.longer_because} onChange={(e) => set({ longer_because: e.target.value })} />
          </Field>
        </>
      )}
    </SectionForm>
  );
}

export function RulesTab() {
  return (
    <>
      <GeneralForm />
      <FeaturesForm />
      <AccessForm />
      <RetentionForm />
    </>
  );
}

interface GlobalHolder {
  permission: string;
  type: string;
  name: string;
}

function GlobalHolders() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['global-holders'],
    queryFn: () => api.get<{ holders: GlobalHolder[] }>('/admin/global-permissions'),
  });
  return (
    <section>
      <h2>{t('admin.holders')}</h2>
      <ErrorNote error={q.error} />
      <table>
        <thead>
          <tr>
            <th>{t('roles.permission')}</th>
            <th>{t('common.name')}</th>
            <th>{t('common.type')}</th>
          </tr>
        </thead>
        <tbody>
          {q.data?.holders.map((h) => (
            <tr key={h.permission + h.type + h.name}>
              <td>{permissionLabel(t, 'instance', h.permission)}</td>
              <td>{h.name}</td>
              <td>{t(`roles.subject_${h.type}`)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function Categories() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [name, setName] = useState('');
  const q = useQuery({
    queryKey: ['categories'],
    queryFn: () => api.get<{ categories: Category[] }>('/categories'),
  });
  const refresh = () => client.invalidateQueries({ queryKey: ['categories'] });
  const add = useMutation({
    mutationFn: () => api.post('/admin/categories', { name }),
    onSuccess: async () => {
      setName('');
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/admin/categories/${id}`),
    onSettled: refresh,
  });
  return (
    <section>
      <h2>{t('admin.categories')}</h2>
      <ErrorNote error={q.error ?? add.error ?? remove.error} />
      <ul className="plain-list">
        {q.data?.categories.map((c) => (
          <li key={c.id} className="row">
            <span className="grow">{c.name}</span>
            <button type="button" className="ghost" onClick={() => remove.mutate(c.id)}>
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <form
        className="row"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          add.mutate();
        }}
      >
        <input
          value={name}
          aria-label={t('admin.categoryName')}
          placeholder={t('admin.categoryName')}
          onChange={(e) => setName(e.target.value)}
          required
        />
        <button type="submit">{t('common.add')}</button>
      </form>
    </section>
  );
}

/** The roles of the instance and its spaces, and who holds what, in words (spec 036). */
export function PermissionsTab() {
  return (
    <>
      <RolesTab />
      <GlobalHolders />
      <Categories />
    </>
  );
}
