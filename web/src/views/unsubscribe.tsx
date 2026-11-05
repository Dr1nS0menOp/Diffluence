// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { api } from '../api';
import { useLocation } from '../router';
import { ErrorNote } from '../components/ui';

/** A link in a mail lands here. Opening it changes nothing: a mail scanner follows links. */
export function UnsubscribeView({ token }: { token: string }) {
  const { t } = useTranslation();
  const { searchParams } = useLocation();
  const watch = searchParams.get('what') === 'watch';
  const stop = useMutation({ mutationFn: () => api.post(`/mail/unsubscribe/${encodeURIComponent(token)}`) });
  return (
    <main id="main" className="login-page">
      <div className="login">
        <h1>{watch ? t('unsubscribe.titleWatch') : t('unsubscribe.title')}</h1>
        <ErrorNote error={stop.error} />
        {stop.isSuccess ? (
          <p role="status">{watch ? t('unsubscribe.doneWatch') : t('unsubscribe.done')}</p>
        ) : (
          <>
            <p>{watch ? t('unsubscribe.questionWatch') : t('unsubscribe.question')}</p>
            <button type="button" className="primary" disabled={stop.isPending} onClick={() => stop.mutate()}>
              {watch ? t('unsubscribe.titleWatch') : t('unsubscribe.button')}
            </button>
          </>
        )}
      </div>
    </main>
  );
}
