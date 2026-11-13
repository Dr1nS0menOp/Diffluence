// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError } from '../api';
import { appearanceQuery } from '../appearance';
import { safeReturnPath } from '../idle';
import { provePasskey } from '../passkey';
import { Link, navigate, useLocation } from '../router';
import { Logo } from '../components/icons';
import { Field } from '../components/ui';
import { setupStateQuery } from '../setup-state';

interface Providers {
  oidc?: { name: string };
}

interface LoginAnswer {
  mfa_required?: boolean;
  mfa_token?: string;
  /** The second factors the account has: a code, a passkey, or both. */
  mfa_methods?: string[];
}

const OIDC_ERRORS = ['oidc_failed', 'oidc_unavailable', 'oidc_account_exists', 'oidc_no_account'];

/** The frame of everything that happens before a person is logged in. */
export function LoginCard({ title, children }: { title: string; children: ReactNode }) {
  const { t } = useTranslation();
  const appearance = useQuery(appearanceQuery).data;
  return (
    <main id="main" className="login-page">
      <div className="login">
        <span className="brand">
          {appearance?.logo ? <img src={appearance.logo} alt="" className="brand-logo" /> : <Logo />}{' '}
          {appearance?.name || t('app.name')}
        </span>
        <h1>{title}</h1>
        {appearance?.maintenance.on && (
          <p className="note note-error">{appearance.maintenance.message || t('site.maintenanceHelp')}</p>
        )}
        {children}
      </div>
    </main>
  );
}

/** What an administrator wrote for the login page, and what else a visitor can do. */
function LoginLinks() {
  const { t } = useTranslation();
  const appearance = useQuery(appearanceQuery).data;
  return (
    <>
      {appearance?.mail_ready && (
        <p className="muted center login-or">
          <Link href="/forgot">{t('login.forgot')}</Link>
        </p>
      )}
      {appearance?.self_signup && appearance.mail_ready && (
        <p className="muted center login-or">
          <Link href="/signup">{t('login.signUp')}</Link>
        </p>
      )}
      {appearance?.terms_url && (
        <p className="muted center login-or">
          <a href={appearance.terms_url} target="_blank" rel="noopener noreferrer">
            {t('login.terms')}
          </a>
        </p>
      )}
    </>
  );
}

export function LoginView() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const location = useLocation();
  const appearance = useQuery(appearanceQuery).data;
  const providers = useQuery({
    queryKey: ['providers'],
    queryFn: () => api.get<Providers>('/auth/providers'),
  });
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [mfaToken, setMfaToken] = useState('');
  const [mfaMethods, setMfaMethods] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  // A new install has nobody to log in yet: send the visitor to the setup, which asks for
  // the code from the server's log.
  const setupOpen = useQuery(setupStateQuery).data;
  useEffect(() => {
    if (setupOpen) navigate('/setup', true);
  }, [setupOpen]);

  const fromUrl = location.searchParams.get('error') ?? '';
  const urlError = OIDC_ERRORS.includes(fromUrl) ? t(`login.${fromUrl}`) : '';

  const done = async () => {
    await client.invalidateQueries({ queryKey: ['me'] });
    navigate(safeReturnPath(location.searchParams.get('next')), true);
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
        setMfaMethods(a.mfa_methods ?? ['code']);
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
      await provePasskey('/auth/passkey');
      await done();
    }, t('login.passkeyFailed'));

  // The second step with the passkey of the account whose password was right.
  const secondPasskey = () =>
    run(async () => {
      await provePasskey('/auth/login/passkey', { mfa_token: mfaToken });
      await done();
    }, t('login.passkeyFailed'));

  return (
    <LoginCard title={t('login.title')}>
      {appearance?.login_welcome && <p className="login-welcome">{appearance.login_welcome}</p>}
      {(error || urlError) && (
        <p role="alert" className="note note-error">
          {error || urlError}
        </p>
      )}
      {mfaToken ? (
        <>
          {mfaMethods.includes('code') && (
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
          )}
          {mfaMethods.includes('passkey') && (
            <>
              {mfaMethods.includes('code') ? (
                <p className="muted center login-or">{t('login.orWith')}</p>
              ) : (
                <p>{t('login.secondPasskeyHelp')}</p>
              )}
              <button
                type="button"
                className={mfaMethods.includes('code') ? undefined : 'primary'}
                onClick={() => void secondPasskey()}
                disabled={busy}
              >
                {t('login.secondPasskey')}
              </button>
            </>
          )}
        </>
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
          <LoginLinks />
        </>
      )}
    </LoginCard>
  );
}
