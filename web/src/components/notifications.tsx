// SPDX-License-Identifier: Apache-2.0
// The bell. It asks for a small number twice a minute, only in a visible tab, which is
// live enough without a WebSocket (spec 010, D12).
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Notification } from '../api';
import { Link } from '../router';
import { IconBell } from './icons';
import { useShortcut } from './shortcuts';
import { Avatar, ErrorNote, Loading, RelativeTime } from './ui';

const POLL_MS = 30_000;
const SHOWN_IN_PANEL = 20;
const MOST_SHOWN = 99;

/** The number on the bell, capped so it stays small. */
export function bellNumber(count: number): string {
  return count > MOST_SHOWN ? `${MOST_SHOWN}+` : String(count);
}

/** Where a notification goes: the exact spot. */
export function notificationLink(n: Notification): string {
  if (n.type === 'access_request') return `/pages/${n.page_id}?share=1`;
  return n.comment_id ? `/pages/${n.page_id}#comment-${n.comment_id}` : `/pages/${n.page_id}`;
}

export function useNotificationSentence() {
  const { t } = useTranslation();
  return (n: Notification): string => {
    const [first] = n.actors;
    const name =
      n.actors.length > 1
        ? t('notifications.people', { count: n.actors.length })
        : (first ?? t('notifications.someone'));
    return t(`notifications.type_${n.type}`, { name, page: n.page_title });
  };
}

export function useRefreshNotifications() {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['notifications'] });
    await client.invalidateQueries({ queryKey: ['notification-count'] });
  };
}

export function NotificationRow({ n, onOpen }: { n: Notification; onOpen?: () => void }) {
  const { t } = useTranslation();
  const sentence = useNotificationSentence();
  const refresh = useRefreshNotifications();
  const mark = useMutation({
    mutationFn: (read: boolean) => api.put(`/me/notifications/${n.id}/${read ? 'read' : 'unread'}`),
    onSettled: refresh,
  });
  return (
    <li className={n.read ? 'notification' : 'notification unread'}>
      {!n.read && <span className="notification-dot" />}
      <Avatar name={n.actors[0] ?? t('notifications.someone')} size="small" />
      <div className="notification-main">
        <Link
          href={notificationLink(n)}
          onClick={() => {
            if (!n.read) mark.mutate(true);
            onOpen?.();
          }}
        >
          {sentence(n)}
        </Link>
        {!n.read && <span className="visually-hidden">{t('notifications.unreadWord')}</span>}
        {n.data.message && <q className="comment-quote">{n.data.message}</q>}
        <span className="muted">
          {n.space_name} · {t(`notifications.reason_${n.reason}`)} · <RelativeTime value={n.at} />
        </span>
      </div>
      <button type="button" className="ghost" onClick={() => mark.mutate(!n.read)}>
        {n.read ? t('notifications.markUnread') : t('notifications.markRead')}
      </button>
    </li>
  );
}

function Panel({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const [unreadOnly, setUnreadOnly] = useState(false);
  const refresh = useRefreshNotifications();
  const q = useQuery({
    queryKey: ['notifications', 'panel', unreadOnly],
    queryFn: () =>
      api.get<{ notifications: Notification[] }>(
        `/me/notifications?limit=${SHOWN_IN_PANEL}&unread=${unreadOnly}`,
      ),
  });
  const readAll = useMutation({
    mutationFn: () => api.post('/me/notifications/read-all'),
    onSettled: refresh,
  });
  return (
    <div className="bell-panel" role="region" aria-label={t('notifications.title')}>
      <div className="section-head">
        <h2>{t('notifications.title')}</h2>
        <button type="button" className="ghost" onClick={() => readAll.mutate()}>
          {t('notifications.readAll')}
        </button>
      </div>
      <label className="check">
        <input type="checkbox" checked={unreadOnly} onChange={(e) => setUnreadOnly(e.target.checked)} />
        {t('notifications.unreadOnly')}
      </label>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? readAll.error} />
      {q.data?.notifications.length === 0 && <p className="muted">{t('notifications.none')}</p>}
      <ul className="plain-list">
        {q.data?.notifications.map((n) => (
          <NotificationRow key={n.id} n={n} onOpen={onClose} />
        ))}
      </ul>
      <Link href="/notifications" onClick={onClose}>
        {t('notifications.all')}
      </Link>
    </div>
  );
}

export function NotificationBell() {
  const { t } = useTranslation();
  const root = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const count = useQuery({
    queryKey: ['notification-count'],
    queryFn: () => api.get<{ unread: number }>('/me/notifications/count'),
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: false,
  });
  const unread = count.data?.unread ?? 0;
  const phrase = t('notifications.bell', { count: unread });
  // A screen reader only hears news: the line changes when the number goes up.
  const [spoken, setSpoken] = useState({ count: 0, phrase: '' });
  if (unread !== spoken.count)
    setSpoken({ count: unread, phrase: unread > spoken.count ? phrase : spoken.phrase });

  useShortcut('notifications', () => setOpen(true));

  useEffect(() => {
    if (!open) return;
    const outside = (e: MouseEvent) => {
      if (!root.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', outside);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', outside);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  return (
    <div className="menu" ref={root}>
      <button
        type="button"
        className="icon bell"
        aria-label={phrase}
        title={phrase}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <IconBell />
        {unread > 0 && (
          <span className="bell-number" aria-hidden="true">
            {bellNumber(unread)}
          </span>
        )}
      </button>
      <span role="status" aria-atomic="true" className="visually-hidden">
        {spoken.phrase}
      </span>
      {open && <Panel onClose={() => setOpen(false)} />}
    </div>
  );
}
