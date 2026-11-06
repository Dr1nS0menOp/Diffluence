// SPDX-License-Identifier: Apache-2.0
// The screens behind a link in a mail: a new password, an invitation, a sign-up.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '../api';
import { Link, useLocation } from '../router';
import { ErrorNote, Field, Loading, NewPasswordField } from '../components/ui';
import { LoginCard } from './login';

function BackToLogin() {
  const { t } = useTranslation();
  return (
    <p className="muted center login-or">
      <Link href="/login">{t('login.back')}</Link>
    </p>
  );
}

/** An address goes in, and the same answer comes out whether it is known or not. */
function AddressForm({
  title,
  route,
  sent,
  action,
}: {
  title: string;
  route: string;
  sent: string;
  action: string;
}) {
  const { t } = useTranslation();
  const [email, setEmail] = useState('');
  const send = useMutation({ mutationFn: () => api.post(route, { email }) });
  return (
    <LoginCard title={title}>
      {send.isSuccess ? (
        <p role="status" className="note">
          {sent}
        </p>
      ) : (
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            send.mutate();
          }}
        >
          <ErrorNote error={send.error} />
          <Field label={t('common.email')}>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="username"
              required
              autoFocus
            />
          </Field>
          <button type="submit" className="primary" disabled={send.isPending}>
            {action}
          </button>
        </form>
      )}
      <BackToLogin />
    </LoginCard>
  );
}

export function ForgotView() {
  const { t } = useTranslation();
  return (
    <AddressForm
      title={t('login.forgotTitle')}
      route="/auth/forgot"
      sent={t('login.forgotSent')}
      action={t('login.forgotSubmit')}
    />
  );
}

export function ResetView() {
  const { t } = useTranslation();
  const token = useLocation().searchParams.get('token') ?? '';
  const [password, setPassword] = useState('');
  const reset = useMutation({ mutationFn: () => api.post('/auth/reset', { token, password }) });
  return (
    <LoginCard title={t('login.resetTitle')}>
      {reset.isSuccess ? (
        <p role="status" className="note">
          {t('login.resetDone')}
        </p>
      ) : (
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            reset.mutate();
          }}
        >
          <ErrorNote error={reset.error} />
          <NewPasswordField label={t('profile.newPassword')} value={password} onChange={setPassword} />
          <button type="submit" className="primary" disabled={reset.isPending}>
            {t('common.save')}
          </button>
        </form>
      )}
      <BackToLogin />
    </LoginCard>
  );
}

/** A name and a password turn a link into an account. */
function NewAccountForm({ route, token, email }: { route: string; token: string; email?: string }) {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const create = useMutation({
    mutationFn: () => api.post(route, { token, display_name: name, password }),
  });
  if (create.isSuccess) {
    return (
      <p role="status" className="note">
        {t('login.accountReady')}
      </p>
    );
  }
  return (
    <form
      onSubmit={(e: FormEvent) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <ErrorNote error={create.error} />
      {email && <p className="muted">{email}</p>}
      <Field label={t('admin.displayName')}>
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoComplete="name"
          required
          autoFocus
        />
      </Field>
      <NewPasswordField label={t('common.password')} value={password} onChange={setPassword} />
      <button type="submit" className="primary" disabled={create.isPending}>
        {t('common.create')}
      </button>
    </form>
  );
}

export function InvitationView() {
  const { t } = useTranslation();
  const token = useLocation().searchParams.get('token') ?? '';
  const q = useQuery({
    queryKey: ['invitation', token],
    queryFn: () => api.get<{ email: string }>(`/auth/invitation?token=${encodeURIComponent(token)}`),
  });
  return (
    <LoginCard title={t('login.invitationTitle')}>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && <NewAccountForm route="/auth/invitation" token={token} email={q.data.email} />}
      <BackToLogin />
    </LoginCard>
  );
}

/** With a token from the mail it finishes a sign-up, without one it starts it. */
export function SignupView() {
  const { t } = useTranslation();
  const token = useLocation().searchParams.get('token') ?? '';
  if (!token) {
    return (
      <AddressForm
        title={t('login.signUp')}
        route="/auth/signup"
        sent={t('login.signUpSent')}
        action={t('login.signUpSubmit')}
      />
    );
  }
  return (
    <LoginCard title={t('login.signUp')}>
      <NewAccountForm route="/auth/signup/confirm" token={token} />
      <BackToLogin />
    </LoginCard>
  );
}
