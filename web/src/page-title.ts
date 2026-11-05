// SPDX-License-Identifier: Apache-2.0
// Small things a browser no longer does for an app that draws its own pages.
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

const SCROLL_KEY = 'diffluence.scroll';
const SAVE_DELAY_MS = 200;

/** The tab shows what is on screen and where it lives, so many open tabs stay readable. */
export function useDocumentTitle(title: string | undefined) {
  const { t } = useTranslation();
  useEffect(() => {
    document.title = title ? `${title} · ${t('app.name')}` : t('app.name');
    return () => {
      document.title = t('app.name');
    };
  }, [title, t]);
}

function wasReloaded(): boolean {
  const [entry] = performance.getEntriesByType('navigation');
  return (entry as PerformanceNavigationTiming | undefined)?.type === 'reload';
}

/** A reload returns to the same spot, once the page is there. */
export function useScrollMemory(ready: boolean) {
  useEffect(() => {
    if (!ready) return;
    const address = window.location.pathname;
    if (wasReloaded() && !window.location.hash) {
      const stored = JSON.parse(sessionStorage.getItem(SCROLL_KEY) ?? '{}') as Record<string, number>;
      const top = stored[address];
      if (top) requestAnimationFrame(() => window.scrollTo(0, top));
    }
    let timer: ReturnType<typeof setTimeout> | undefined;
    const remember = () => {
      clearTimeout(timer);
      timer = setTimeout(() => {
        sessionStorage.setItem(SCROLL_KEY, JSON.stringify({ [address]: window.scrollY }));
      }, SAVE_DELAY_MS);
    };
    window.addEventListener('scroll', remember, { passive: true });
    return () => {
      clearTimeout(timer);
      window.removeEventListener('scroll', remember);
    };
  }, [ready]);
}
