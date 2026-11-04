// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError } from '../api';
import { navigate, useLocation } from '../router';
import { Logo } from '../components/icons';
import { Field } from '../components/ui';

interface Providers {
  oidc?: { name: string };
}

interface LoginAnswer {
  mfa_required?: boolean;
  mfa_token?: string;
}

const OIDC_ERRORS = ['oidc_failed', 'oidc_unavailable', 'oidc_account_exists', 'oidc_no_account'];

export function LoginView() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const location = useLocation();
  const providers = useQuery({
    queryKey: ['providers'],
    queryFn: () => api.get<Providers>('/auth/providers'),
  });
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [mfaToken, setMfaToken] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const fromUrl = location.searchParams.get('error') ?? '';
  const urlError = OIDC_ERRORS.includes(fromUrl) ? t(`login.${fromUrl}`) : '';

  const done = async () => {
    await client.invalidateQueries({ queryKey: ['me'] });
    navigate('/', true);
  };

  const run = async (act: () => Promise<void>, failure: string) => {
    setBusy(true);
    setError('');
    try {
      await act();
    } catch (e) {
      setError(e instanceof ApiError && e.status === 429 ? e.message : failure);
    } finally {
      setBusy(false);
    }
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void run(async () => {
      const a = await api.post<LoginAnswer>('/auth/login', { email, password });
      if (a.mfa_required && a.mfa_token) {
        setMfaToken(a.mfa_token);
        return;
      }
      await done();
    }, t('login.failed'));
  };

  const submitCode = (e: FormEvent) => {
    e.preventDefault();
    void run(async () => {
      await api.post('/auth/login/totp', { mfa_token: mfaToken, code });
      await done();
    }, t('login.wrongCode'));
  };

  const passkey = () =>
    run(async () => {
      const begin = await api.post<{
        options: { publicKey: PublicKeyCredentialRequestOptionsJSON };
        token: string;
      }>('/auth/passkey/begin');
      const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(begin.options.publicKey);
      const credential = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
      if (!credential) throw new Error('no credential');
      await api.post('/auth/passkey/finish', { token: begin.token, response: credential.toJSON() });
      await done();
    }, t('login.passkeyFailed'));

  return (
    <main id="main" className="login-page">
      <div className="login">
        <span className="brand">
          <Logo /> {t('app.name')}
        </span>
        <h1>{t('login.title')}</h1>
        {(error || urlError) && (
          <p role="alert" className="note note-error">
            {error || urlError}
          </p>
        )}
        {mfaToken ? (
          <form onSubmit={submitCode}>
            <Field label={t('login.code')}>
              <input
                value={code}
                onChange={(e) => setCode(e.target.value)}
                autoComplete="one-time-code"
                autoFocus
                required
              />
            </Field>
            <button type="submit" className="primary" disabled={busy}>
              {t('login.codeSubmit')}
            </button>
          </form>
        ) : (
          <>
            <form onSubmit={submit}>
              <Field label={t('common.email')}>
                <input
                  type="email"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  autoComplete="username"
                  required
                />
              </Field>
              <Field label={t('common.password')}>
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="current-password"
                  required
                />
              </Field>
              <button type="submit" className="primary" disabled={busy}>
                {t('login.submit')}
              </button>
            </form>
            <p className="muted center login-or">{t('login.orWith')}</p>
            <button type="button" onClick={() => void passkey()} disabled={busy}>
              {t('login.passkey')}
            </button>
            {providers.data?.oidc && (
              <a className="button" href="/api/v1/auth/oidc/start">
                {providers.data.oidc.name}
              </a>
            )}
          </>
        )}
      </div>
    </main>
  );
}
