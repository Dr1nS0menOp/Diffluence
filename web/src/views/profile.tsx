// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type LoginEntry, type Me } from '../api';
import { Dialog, ErrorNote, Field, NewPasswordField } from '../components/ui';
import { useDates } from '../dates';
import { useLocation } from '../router';
import { AccountNav } from './account';
import { TableBox } from '../components/table-box';

interface Security {
  has_password: boolean;
  totp_enabled: boolean;
  passkeys: number;
  source: string;
}

interface Session {
  id: string;
  last_seen_at: string;
  ip: string;
  user_agent: string;
  current: boolean;
}

interface Passkey {
  id: string;
  name: string;
  created_at: string;
}

function PasswordSection() {
  const { t } = useTranslation();
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [done, setDone] = useState(false);
  const change = useMutation({
    mutationFn: () => api.post('/me/password', { current_password: current, new_password: next }),
    onSuccess: () => {
      setDone(true);
      setCurrent('');
      setNext('');
    },
  });
  return (
    <section>
      <h2>{t('profile.changePassword')}</h2>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          change.mutate();
        }}
      >
        <ErrorNote error={change.error} />
        {done && (
          <p role="status" className="note">
            {t('profile.passwordChanged')}
          </p>
        )}
        <Field label={t('profile.currentPassword')}>
          <input
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            required
          />
        </Field>
        <NewPasswordField label={t('profile.newPassword')} value={next} onChange={setNext} />
        <button type="submit" className="primary">
          {t('common.save')}
        </button>
      </form>
    </section>
  );
}

/** A session someone stole survives the removal of a factor, so this is offered then. */
function EndOtherSessionsOffer() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const end = useMutation({
    mutationFn: () => api.post('/me/sessions/end-others'),
    onSuccess: () => client.invalidateQueries({ queryKey: ['sessions'] }),
  });
  if (end.isSuccess) {
    return (
      <p role="status" className="note">
        {t('account.otherSessionsEnded')}
      </p>
    );
  }
  return (
    <div role="status" className="note">
      <p>{t('account.endOtherSessionsHelp')}</p>
      <ErrorNote error={end.error} />
      <button type="button" onClick={() => end.mutate()}>
        {t('account.endOtherSessions')}
      </button>
    </div>
  );
}

function TOTPSection({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [setup, setSetup] = useState<{ secret: string; qr_code: string } | null>(null);
  const [recovery, setRecovery] = useState<string[] | null>(null);
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['security'] });
    await client.invalidateQueries({ queryKey: ['me'] });
  };
  const begin = useMutation({
    mutationFn: () => api.post<{ secret: string; qr_code: string }>('/me/totp/begin', { password }),
    onSuccess: (s) => {
      setSetup(s);
      setPassword('');
    },
  });
  const confirm = useMutation({
    mutationFn: () => api.post<{ recovery_codes: string[] }>('/me/totp/confirm', { code }),
    onSuccess: async (r) => {
      setRecovery(r.recovery_codes);
      setSetup(null);
      setCode('');
      await refresh();
    },
  });
  const disable = useMutation({
    mutationFn: () => api.post('/me/totp/disable', { password }),
    onSuccess: async () => {
      setPassword('');
      await refresh();
    },
  });
  const removed = disable.isSuccess && !enabled;
  return (
    <section>
      <h2>{t('profile.twoFactor')}</h2>
      <p>{enabled ? t('profile.totpOn') : t('profile.totpOff')}</p>
      <ErrorNote error={begin.error ?? confirm.error ?? disable.error} />
      {removed && <EndOtherSessionsOffer />}
      {!setup && (
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            if (enabled) disable.mutate();
            else begin.mutate();
          }}
        >
          <Field label={t('profile.currentPassword')}>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
          </Field>
          <button type="submit">{enabled ? t('profile.totpDisable') : t('profile.totpStart')}</button>
        </form>
      )}
      {setup && (
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            confirm.mutate();
          }}
        >
          <p>{t('profile.totpScan')}</p>
          <img src={setup.qr_code} alt={t('profile.totpScan')} width={240} height={240} />
          <p>
            {t('profile.totpSecret')} <code data-testid="totp-secret">{setup.secret}</code>
          </p>
          <Field label={t('profile.totpCode')}>
            <input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              autoComplete="one-time-code"
              required
            />
          </Field>
          <button type="submit" className="primary">
            {t('profile.totpConfirm')}
          </button>
        </form>
      )}
      {recovery && (
        <Dialog title={t('profile.recoveryTitle')} onClose={() => setRecovery(null)}>
          <p>{t('profile.recoveryHelp')}</p>
          <ul className="codes">
            {recovery.map((c) => (
              <li key={c}>
                <code>{c}</code>
              </li>
            ))}
          </ul>
        </Dialog>
      )}
    </section>
  );
}

function PasskeySection() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['passkeys'],
    queryFn: () => api.get<{ passkeys: Passkey[] }>('/me/passkeys'),
  });
  const [name, setName] = useState('');
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['passkeys'] });
    await client.invalidateQueries({ queryKey: ['security'] });
    await client.invalidateQueries({ queryKey: ['me'] });
  };
  const add = useMutation({
    mutationFn: async () => {
      const begin = await api.post<{
        options: { publicKey: PublicKeyCredentialCreationOptionsJSON };
        token: string;
      }>('/me/passkeys/begin');
      const publicKey = PublicKeyCredential.parseCreationOptionsFromJSON(begin.options.publicKey);
      const credential = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null;
      if (!credential) throw new Error('no credential');
      await api.post('/me/passkeys/finish', { token: begin.token, name, response: credential.toJSON() });
    },
    onSuccess: async () => {
      setName('');
      await refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.del(`/me/passkeys/${id}`),
    onSettled: refresh,
  });
  return (
    <section>
      <h2>{t('profile.passkeys')}</h2>
      <ErrorNote error={add.error ?? remove.error} />
      {remove.isSuccess && <EndOtherSessionsOffer />}
      {q.data?.passkeys.length === 0 && <p className="muted">{t('profile.passkeyNone')}</p>}
      <ul className="plain-list">
        {q.data?.passkeys.map((p) => (
          <li key={p.id} className="row">
            {p.name}
            <button type="button" className="ghost" onClick={() => remove.mutate(p.id)}>
              {t('common.remove')}
            </button>
          </li>
        ))}
      </ul>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          add.mutate();
        }}
      >
        <Field label={t('profile.passkeyName')}>
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </Field>
        <button type="submit">{t('profile.passkeyAdd')}</button>
      </form>
    </section>
  );
}

function SessionsSection() {
  const { t } = useTranslation();
  const dates = useDates();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['sessions'],
    queryFn: () => api.get<{ sessions: Session[] }>('/me/sessions'),
  });
  const revoke = useMutation({
    mutationFn: (id: string) => api.del(`/me/sessions/${id}`),
    onSettled: () => client.invalidateQueries({ queryKey: ['sessions'] }),
  });
  return (
    <section>
      <h2>{t('profile.sessions')}</h2>
      <TableBox>
        <table>
          <tbody>
            {q.data?.sessions.map((s) => (
              <tr key={s.id}>
                <td>{s.user_agent}</td>
                <td>{s.ip}</td>
                <td>{dates.moment(s.last_seen_at)}</td>
                <td>
                  {s.current ? (
                    t('profile.thisSession')
                  ) : (
                    <button type="button" className="ghost" onClick={() => revoke.mutate(s.id)}>
                      {t('profile.revoke')}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </section>
  );
}

function LoginHistorySection() {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['logins'],
    queryFn: () => api.get<{ logins: LoginEntry[] }>('/me/logins'),
  });
  return (
    <section>
      <h2>{t('account.logins')}</h2>
      <ErrorNote error={q.error} />
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('admin.when')}</th>
              <th>{t('account.loginMethod')}</th>
              <th>{t('admin.ip')}</th>
              <th>{t('account.loginDevice')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.logins.map((l) => (
              <tr key={l.at}>
                <td>{dates.moment(l.at)}</td>
                <td>{l.method}</td>
                <td>{l.ip}</td>
                <td>{l.user_agent}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </section>
  );
}

/** Says why a stronger sign-in matters, for as long as a password is all there is. */
function SignInNote({ me }: { me: Me }) {
  const { t } = useTranslation();
  if (me.needs_second_factor) {
    return (
      <p role="alert" className="note note-error">
        {t('account.secondFactorRequired')}
      </p>
    );
  }
  if (!me.sign_in.weak) return null;
  return <p className="note">{t('site.signInHelp')}</p>;
}

export function SecurityView({ me }: { me: Me }) {
  const { t } = useTranslation();
  // Right after the first setup: the new administrator's account reaches everything.
  const welcome = useLocation().searchParams.get('welcome') === '1';
  const security = useQuery({ queryKey: ['security'], queryFn: () => api.get<Security>('/me/security') });
  return (
    <>
      <h1>{t('profile.title')}</h1>
      {!me.needs_second_factor && <AccountNav />}
      {welcome && (
        <p role="status" className="note">
          {t('setup.welcome')}
        </p>
      )}
      <SignInNote me={me} />
      {security.data?.has_password === false && <p className="muted">{t('profile.noPassword')}</p>}
      {security.data?.has_password && <PasswordSection />}
      {security.data?.has_password && <TOTPSection enabled={security.data.totp_enabled} />}
      <PasskeySection />
      {!me.needs_second_factor && <SessionsSection />}
      {!me.needs_second_factor && <LoginHistorySection />}
    </>
  );
}
