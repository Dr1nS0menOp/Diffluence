// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { setupStateQuery } from '../setup-state';
import { navigate } from '../router';
import { ErrorNote, Field, Loading, NewPasswordField } from '../components/ui';
import { LoginCard } from './login';

function browserZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
}

/**
 * The first setup of a new install: the organisation, the first administrator and the
 * defaults. It asks for the one-time code from the server's log, so only the person who
 * installed the server can use it (spec 038).
 */
export function SetupView() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const state = useQuery(setupStateQuery);
  const [code, setCode] = useState('');
  const [organisation, setOrganisation] = useState('');
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [zone, setZone] = useState(browserZone);
  const finish = useMutation({
    mutationFn: () =>
      api.post<{ signed_in: boolean }>('/setup', {
        code,
        organisation,
        name,
        email,
        password,
        language: 'en',
        time_zone: zone,
      }),
    onSuccess: async (answer) => {
      await client.invalidateQueries();
      // Right after the setup the new administrator adds a passkey: their account reaches everything.
      navigate(answer.signed_in ? '/me/security?welcome=1' : '/login', true);
    },
  });

  if (state.isPending) {
    return (
      <LoginCard title={t('setup.title')}>
        <Loading />
      </LoginCard>
    );
  }
  if (!state.data) {
    return (
      <LoginCard title={t('setup.closedTitle')}>
        <p>{t('setup.closed')}</p>
        <a className="button" href="/login">
          {t('setup.toLogin')}
        </a>
      </LoginCard>
    );
  }
  const submit = (e: FormEvent) => {
    e.preventDefault();
    finish.mutate();
  };
  return (
    <LoginCard title={t('setup.title')}>
      <p className="muted">{t('setup.intro')}</p>
      <ErrorNote error={finish.error} />
      <form onSubmit={submit}>
        <Field label={t('setup.code')} help={t('setup.codeHelp')}>
          <input
            value={code}
            onChange={(e) => setCode(e.target.value)}
            autoComplete="one-time-code"
            autoCapitalize="characters"
            spellCheck={false}
            required
            autoFocus
          />
        </Field>
        <Field label={t('setup.organisation')} help={t('setup.organisationHelp')}>
          <input
            value={organisation}
            onChange={(e) => setOrganisation(e.target.value)}
            autoComplete="organization"
            maxLength={200}
          />
        </Field>
        <h2>{t('setup.you')}</h2>
        <Field label={t('admin.displayName')}>
          <input value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" required />
        </Field>
        <Field label={t('common.email')}>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="username"
            required
          />
        </Field>
        <NewPasswordField label={t('common.password')} value={password} onChange={setPassword} />
        <h2>{t('setup.defaults')}</h2>
        <Field label={t('setup.language')}>
          <select value="en" disabled>
            <option value="en">{t('setup.english')}</option>
          </select>
        </Field>
        <Field label={t('setup.timeZone')}>
          <select value={zone} onChange={(e) => setZone(e.target.value)}>
            {['UTC', ...Intl.supportedValuesOf('timeZone')].map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </select>
        </Field>
        <button type="submit" className="primary" disabled={finish.isPending}>
          {t('setup.submit')}
        </button>
      </form>
    </LoginCard>
  );
}
