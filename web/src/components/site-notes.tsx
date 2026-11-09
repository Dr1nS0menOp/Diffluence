// SPDX-License-Identifier: Apache-2.0
// What a whole instance has to say, under the top bar. These are static regions: they
// are there on load and take no focus. Only a maintenance state that starts while a
// person is working is an alert (WAI-ARIA alert pattern).
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me } from '../api';
import { appearanceQuery, liveAppearanceQuery } from '../appearance';
import { reloadWhenSaved } from '../before-reload';
import { Link, useLocation } from '../router';
import { ErrorNote } from './ui';

const DISMISSED_BANNER_KEY = 'diffluence.banner';

function Maintenance({ message, isNews }: { message: string; isNews: boolean }) {
  const { t } = useTranslation();
  return (
    <div role={isNews ? 'alert' : undefined}>
      <p className="site-note danger-note" data-testid="maintenance-note">
        <strong>{t('site.maintenance')}</strong> {message || t('site.maintenanceHelp')}
      </p>
    </div>
  );
}

function SiteBanner({ text, level }: { text: string; level: string }) {
  const { t } = useTranslation();
  const [dismissed, setDismissed] = useState(() => localStorage.getItem(DISMISSED_BANNER_KEY));
  if (dismissed === text) return null;
  return (
    <div
      className={level === 'warning' ? 'site-note danger-note' : 'site-note'}
      role="region"
      aria-label={t('site.banner')}
    >
      <span className="grow">{text}</span>
      <button
        type="button"
        className="ghost"
        onClick={() => {
          localStorage.setItem(DISMISSED_BANNER_KEY, text);
          setDismissed(text);
        }}
      >
        {t('common.close')}
      </button>
    </div>
  );
}

/** Asks for a stronger sign-in. A person can put it off, never switch it off (spec 013, D24). */
function SignInPrompt() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const later = useMutation({
    mutationFn: () => api.post('/me/sign-in-prompt/later'),
    onSuccess: () => client.invalidateQueries({ queryKey: ['me'] }),
  });
  return (
    <div className="site-note" role="region" aria-label={t('site.signIn')}>
      <span className="grow">{t('site.signInHelp')}</span>
      <ErrorNote error={later.error} />
      <Link href="/me/security" className="button">
        {t('site.signInSetUp')}
      </Link>
      <button type="button" className="ghost" onClick={() => later.mutate()}>
        {t('site.signInLater')}
      </button>
    </div>
  );
}

/**
 * Says that the server now ships a newer frontend than this tab runs (spec 041). It offers a
 * reload and never forces one: it takes no focus, so typing goes on, and the reload waits for
 * an open editor to save its draft. A status, so a screen reader hears it once.
 */
function NewVersion({ build }: { build: string }) {
  const { t } = useTranslation();
  const [started] = useState(build);
  if (!started || !build || build === started) return null;
  return (
    <div className="site-note" role="status" aria-label={t('site.newVersion')}>
      <span className="grow">{t('site.newVersionHelp')}</span>
      <button type="button" onClick={() => void reloadWhenSaved()}>
        {t('site.reload')}
      </button>
    </div>
  );
}

/** Asks again when a person comes back to the tab, the likely moment after a new version. */
function useAskWhenShown() {
  const client = useQueryClient();
  useEffect(() => {
    const shown = () => {
      if (document.visibilityState === 'visible')
        client.invalidateQueries({ queryKey: appearanceQuery.queryKey });
    };
    document.addEventListener('visibilitychange', shown);
    return () => document.removeEventListener('visibilitychange', shown);
  }, [client]);
}

export function SiteNotes({ me }: { me: Me }) {
  const { pathname } = useLocation();
  const appearance = useQuery(liveAppearanceQuery);
  useAskWhenShown();
  const maintenance = appearance.data?.maintenance;
  const banner = appearance.data?.banner;
  const [sawItOff, setSawItOff] = useState(false);
  if (maintenance && !maintenance.on && !sawItOff) setSawItOff(true);
  return (
    <>
      {appearance.data && <NewVersion build={appearance.data.build} />}
      {maintenance?.on && <Maintenance message={maintenance.message} isNews={sawItOff} />}
      {banner && <SiteBanner text={banner.text} level={banner.level} />}
      {me.sign_in.prompt && pathname !== '/me/security' && <SignInPrompt />}
    </>
  );
}
