// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { Avatar } from './ui';

const BEAT_MS = 20_000;
const ENGAGED_AFTER_MS = 20_000;
const MAX_PICTURES = 5;
const LAST_PAGE_KEY = 'diffluence-last-page';

export interface PresentPerson {
  id: string;
  name: string;
  mode: 'reading' | 'editing';
}

interface Beat {
  people: PresentPerson[];
  stamp: string;
}

/**
 * Says every 20 seconds that this person is here, and hears who else is. A change of
 * the stamp means comments, labels or the status moved, so those parts refresh.
 */
export function usePresence(pageId: string, mode: 'reading' | 'editing'): PresentPerson[] {
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['presence', pageId, mode],
    queryFn: () => api.put<Beat>(`/pages/${pageId}/presence`, { mode }),
    refetchInterval: BEAT_MS,
    refetchOnWindowFocus: false,
    retry: false,
  });
  const stamp = q.data?.stamp;
  const seen = useRef(stamp);
  useEffect(() => {
    if (seen.current && stamp && seen.current !== stamp && mode === 'reading') {
      void client.invalidateQueries({ queryKey: ['page', pageId] });
      void client.invalidateQueries({ queryKey: ['comments', pageId] });
    }
    seen.current = stamp;
  }, [client, mode, pageId, stamp]);
  return q.data?.people ?? [];
}

/** Who else is here: pictures up to five, then a number, and a quiet line for screen readers. */
export function PresencePeople({ people }: { people: PresentPerson[] }) {
  const { t } = useTranslation();
  if (people.length === 0) return null;
  const shown = people.slice(0, MAX_PICTURES);
  const more = people.length - shown.length;
  const names = people.map((p) => p.name).join(', ');
  return (
    <span className="presence" title={names}>
      {shown.map((p) => (
        <Avatar key={p.id} name={p.name} size="small" />
      ))}
      {more > 0 && <span className="muted">{t('presence.more', { count: more })}</span>}
      <span role="status" className="visually-hidden">
        {t('presence.here', { names })}
      </span>
    </span>
  );
}

/** The line in the strip of the editor: nobody is blocked, but nobody is surprised (PAG-034). */
export function EditingTogether({ pageId }: { pageId: string }) {
  const { t } = useTranslation();
  const editors = usePresence(pageId, 'editing').filter((p) => p.mode === 'editing');
  if (editors.length === 0) return null;
  return (
    <span
      role="status"
      className="lozenge"
      title={t('presence.editing', { names: editors.map((p) => p.name).join(', ') })}
    >
      {t('presence.editing', { names: editors.map((p) => p.name).join(', ') })}
    </span>
  );
}

function lastPage(pageId: string): string | null {
  const from = sessionStorage.getItem(LAST_PAGE_KEY);
  sessionStorage.setItem(LAST_PAGE_KEY, pageId);
  return from && from !== pageId ? from : null;
}

/**
 * Counts a visit from the reading screen (D1), and marks it engaged after 20 seconds
 * with the tab in view. A failed count changes nothing for the reader, so it is not
 * retried. Answers with the count for the byline, for those who may see it.
 */
export function useViewCount(pageId: string, isPublished: boolean): number | null {
  const [views, setViews] = useState<number | null>(null);
  const opened = useMutation({
    mutationFn: () =>
      api.post<{ views: number | null }>(`/pages/${pageId}/views`, { from: lastPage(pageId) }),
    onSuccess: (r) => setViews(r.views),
    retry: false,
  });
  const engaged = useMutation({ mutationFn: () => api.patch(`/pages/${pageId}/views`, {}), retry: false });
  const open = opened.mutate;
  const engage = engaged.mutate;
  useEffect(() => {
    if (!isPublished) return undefined;
    open();
    let timer = 0;
    const watch = () => {
      window.clearTimeout(timer);
      if (document.visibilityState === 'visible') timer = window.setTimeout(() => engage(), ENGAGED_AFTER_MS);
    };
    watch();
    document.addEventListener('visibilitychange', watch);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', watch);
    };
  }, [engage, isPublished, open, pageId]);
  return views;
}
