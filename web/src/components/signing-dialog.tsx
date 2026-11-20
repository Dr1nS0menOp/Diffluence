// SPDX-License-Identifier: Apache-2.0
// The signing dialog (spec 064, DOC-720, DOC-730, DOC-731, DOC-923). It names the meaning, the
// version and its title before anything else, asks for a fresh confirmation of identity in
// the same dialog, and says the result in a live region. Its heading and its button carry the
// meaning, so a screen reader hears what the person puts their name to.
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api, type SignatureMeaning, type SignaturePrompt } from '../api';
import { Dialog, ErrorNote, Field, Loading } from './ui';
import { pageTitle } from '../page-name';

/** The step a person signs: an author's own step, or their approval as a reviewer. */
export interface SigningStep {
  transition?: string;
  version: number;
}

/** What a page remembers of a signing step while the browser is at the identity provider. */
interface PendingSigning {
  pageId: string;
  step: SigningStep;
  token: string;
  comment: string;
}

const PENDING_KEY = 'kenwerk.signing';

/** A signing step that came back from the identity provider for this page, if any. */
export function pendingSigning(pageId: string): PendingSigning | null {
  try {
    const raw = sessionStorage.getItem(PENDING_KEY);
    if (!raw) return null;
    const p = JSON.parse(raw) as PendingSigning;
    return p.pageId === pageId ? p : null;
  } catch {
    return null;
  }
}

function forgetPending() {
  try {
    sessionStorage.removeItem(PENDING_KEY);
  } catch {
    // Storage can be off; the step then simply ends here.
  }
}

/** The words of a meaning: those of the four every space has are translated by key. */
export function meaningName(
  t: (key: string) => string,
  m: Pick<SignatureMeaning, 'key' | 'name' | 'built_in'>,
): string {
  return m.built_in ? t(`signing.meaning_${m.key}`) : m.name;
}

export function SigningDialog({
  pageId,
  step,
  comment,
  resumed,
  onSign,
  onSigned,
  onClose,
}: {
  pageId: string;
  step: SigningStep;
  comment: string;
  /** A step that comes back confirmed from the identity provider, with its token. */
  resumed?: string;
  /** Takes the step with the confirmation; it fails when the server refuses. */
  onSign: (confirmation: string) => Promise<unknown>;
  onSigned: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [password, setPassword] = useState('');
  const [code, setCode] = useState('');
  const [status, setStatus] = useState('');
  const prompt = useQuery({
    queryKey: ['signing', pageId, step.transition ?? 'approve', step.version],
    queryFn: () => api.post<SignaturePrompt>(`/pages/${pageId}/signing`, step),
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
  });
  const data = prompt.data;
  const token = resumed ?? data?.start.token ?? '';
  const methods = data?.start.methods ?? [];
  const sign = useMutation({
    mutationFn: async (how: 'password' | 'passkey' | 'confirmed') => {
      if (how === 'password') {
        await api.post('/signing/confirm', { token, password, code });
      } else if (how === 'passkey' && data?.start.passkey) {
        const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(data.start.passkey.publicKey);
        const credential = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
        if (!credential) throw new Error(t('signing.passkeyCancelled'));
        await api.post('/signing/passkey', { token, response: credential.toJSON() });
      }
      await onSign(token);
    },
    onSuccess: () => {
      forgetPending();
      setStatus(t('signing.signed'));
      onSigned();
    },
    onError: () => setStatus(t('signing.notSigned')),
  });

  useEffect(() => {
    // Back from the identity provider: the dialog shows the step it left, already confirmed.
    if (resumed) forgetPending();
  }, [resumed]);

  const meaning = data ? meaningName(t, data.meaning) : '';
  const title = data
    ? t('signing.heading', { meaning, version: data.version, title: pageTitle(t, data.title) })
    : t('signing.loading');
  const signLabel = t('signing.signAs', { meaning });
  const toProvider = () => {
    try {
      sessionStorage.setItem(
        PENDING_KEY,
        JSON.stringify({ pageId, step, token, comment } satisfies PendingSigning),
      );
    } catch {
      // Without storage the step cannot come back; the provider's login still confirms it.
    }
    window.location.assign(`/api/v1/auth/oidc/sign?token=${encodeURIComponent(token)}`);
  };
  return (
    <Dialog title={title} onClose={onClose}>
      {prompt.isPending && <Loading />}
      <ErrorNote error={prompt.error} />
      {data && (
        <>
          <p>
            {t('signing.statement', { meaning, version: data.version, title: pageTitle(t, data.title) })}{' '}
            <a href={`/pages/${pageId}/history?version=${data.version}`} target="_blank" rel="noreferrer">
              {t('signing.exactText')}
            </a>
          </p>
          <p className="muted">
            {data.meaning.built_in ? t(`signing.meaningHelp_${data.meaning.key}`) : data.meaning.description}
          </p>
          {resumed ? (
            <p>{t('signing.confirmedAtProvider')}</p>
          ) : (
            <>
              {methods.includes('password_and_code') && (
                <form
                  id="signing-form"
                  onSubmit={(e) => {
                    e.preventDefault();
                    sign.mutate('password');
                  }}
                >
                  <Field label={t('signing.password')}>
                    <input
                      type="password"
                      autoComplete="current-password"
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                    />
                  </Field>
                  <Field label={t('signing.code')} help={t('signing.codeHelp')}>
                    <input
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      value={code}
                      onChange={(e) => setCode(e.target.value)}
                    />
                  </Field>
                </form>
              )}
              {methods.includes('passkey') && (
                <p>
                  <button type="button" disabled={sign.isPending} onClick={() => sign.mutate('passkey')}>
                    {t('signing.withPasskey', { meaning })}
                  </button>
                </p>
              )}
              {methods.includes('identity_provider') && (
                <p>
                  <button type="button" onClick={toProvider}>
                    {t('signing.atProvider')}
                  </button>
                </p>
              )}
            </>
          )}
          <ErrorNote error={sign.error} />
        </>
      )}
      <p role="status" className="visually-hidden">
        {status}
      </p>
      <div className="dialog-actions">
        <button type="button" className="quiet" onClick={onClose}>
          {t('common.cancel')}
        </button>
        {data && (resumed || methods.includes('password_and_code')) && (
          <button
            type={resumed ? 'button' : 'submit'}
            form={resumed ? undefined : 'signing-form'}
            className="primary"
            disabled={sign.isPending || (!resumed && (password === '' || code.trim() === ''))}
            onClick={resumed ? () => sign.mutate('confirmed') : undefined}
          >
            {signLabel}
          </button>
        )}
      </div>
    </Dialog>
  );
}
