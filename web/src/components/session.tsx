// SPDX-License-Identifier: Apache-2.0
// What guards a running session: proving who you are again, and the idle warning.
import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { answerReauthWith, api, lastRequestTime } from '../api';
import { idleState, needsPing, type IdleState } from '../idle';
import { provePasskey } from '../passkey';
import { navigate } from '../router';
import { Dialog, ErrorNote, Field } from './ui';

const TICK_MS = 1000;

interface Security {
  has_password: boolean;
  passkeys: number;
}

function ReauthDialog({ onDone }: { onDone: (confirmed: boolean) => void }) {
  const { t } = useTranslation();
  const [password, setPassword] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const security = useQuery({ queryKey: ['security'], queryFn: () => api.get<Security>('/me/security') });
  const providers = useQuery({
    queryKey: ['providers'],
    queryFn: () => api.get<{ oidc?: { name: string } }>('/auth/providers'),
  });
  const run = async (prove: () => Promise<unknown>) => {
    setBusy(true);
    setError(null);
    try {
      await prove();
      onDone(true);
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  };
  const provider = providers.data?.oidc;
  return (
    <Dialog title={t('reauth.title')} onClose={() => onDone(false)}>
      <p>{t('reauth.help')}</p>
      <ErrorNote error={error} />
      {security.data?.has_password && (
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            void run(() => api.post('/auth/reauth', { password }));
          }}
        >
          <Field label={t('common.password')}>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
              autoFocus
            />
          </Field>
          <div className="dialog-actions">
            <button type="submit" className="primary" disabled={busy}>
              {t('reauth.confirm')}
            </button>
          </div>
        </form>
      )}
      {(security.data?.passkeys ?? 0) > 0 && (
        <button
          type="button"
          disabled={busy}
          onClick={() => void run(() => provePasskey('/auth/reauth/passkey'))}
        >
          {t('login.passkey')}
        </button>
      )}
      {security.data?.has_password === false && provider && (
        <a className="button" href="/api/v1/auth/oidc/start?prompt=login">
          {t('reauth.provider', { name: provider.name })}
        </a>
      )}
    </Dialog>
  );
}

/** Answers `reauth_required` for every request, so no screen has to think of it. */
export function ReauthProvider({ children }: { children: ReactNode }) {
  const [asking, setAsking] = useState(false);
  const waiting = useRef<((confirmed: boolean) => void)[]>([]);
  useEffect(() => {
    answerReauthWith(
      () =>
        new Promise<boolean>((resolve) => {
          waiting.current.push(resolve);
          setAsking(true);
        }),
    );
    return () => answerReauthWith(null);
  }, []);
  const finish = (confirmed: boolean) => {
    setAsking(false);
    for (const resolve of waiting.current.splice(0)) resolve(confirmed);
  };
  return (
    <>
      {children}
      {asking && <ReauthDialog onDone={finish} />}
    </>
  );
}

function useIdleState(minutes: number): IdleState {
  const [state, setState] = useState<IdleState>({ kind: 'fine' });
  const lastInput = useRef(0);
  useEffect(() => {
    if (minutes <= 0) return;
    const note = () => {
      lastInput.current = Date.now();
    };
    const tick = () => {
      const now = Date.now();
      if (needsPing(now, lastRequestTime(), lastInput.current)) {
        api.get('/me').catch((error: unknown) => console.warn('The session could not be kept open.', error));
      }
      setState(idleState(now, lastRequestTime(), minutes));
    };
    window.addEventListener('keydown', note);
    window.addEventListener('pointerdown', note);
    const timer = setInterval(tick, TICK_MS);
    return () => {
      window.removeEventListener('keydown', note);
      window.removeEventListener('pointerdown', note);
      clearInterval(timer);
    };
  }, [minutes]);
  return state;
}

/** Warns before an idle session ends. A draft is safe either way: autosave goes on. */
export function IdleWarning({ minutes }: { minutes: number }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const state = useIdleState(minutes);
  const ended = state.kind === 'ended';
  useEffect(() => {
    if (!ended) return;
    const here = window.location.pathname + window.location.search;
    client.clear();
    navigate(`/login?next=${encodeURIComponent(here)}`, true);
  }, [ended, client]);
  if (state.kind !== 'warning') return null;
  const stay = () => {
    api.get('/me').catch((error: unknown) => console.warn('The session could not be kept open.', error));
  };
  return (
    <Dialog title={t('idle.title')} onClose={stay}>
      <p role="timer">{t('idle.help', { count: state.seconds })}</p>
      <div className="dialog-actions">
        <button type="button" className="primary" onClick={stay}>
          {t('idle.stay')}
        </button>
      </div>
    </Dialog>
  );
}
