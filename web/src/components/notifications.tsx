// SPDX-License-Identifier: Apache-2.0
// The bell. It asks for a small number twice a minute, only in a visible tab, which is
// live enough without a WebSocket (spec 010, D12). Its panel has two tabs, For me and
// Watched, and groups what it shows per day (spec 079, COL-113 to COL-118).
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Notification } from '../api';
import { useDates } from '../dates';
import { spreadPoll } from '../polling';
import { Link } from '../router';
import { IconBell, IconBlog, IconComment, IconExternal, IconMore, IconPage, IconTask } from './icons';
import { Menu, MenuItem, MenuLink } from './menu';
import { useShortcut } from './shortcuts';
import { Tabs } from './tabs';
import { Avatar, ErrorNote, Loading, RelativeTime } from './ui';
import { pageTitle } from '../page-name';

const POLL_MS = 30_000;
const SHOWN_IN_PANEL = 20;
const MOST_SHOWN = 99;
const KIND_ICON_SIZE = 14;
const MS_PER_DAY = 86_400_000;

/** The two tabs of the panel and the full screen (COL-115). */
export const NOTIFICATION_TABS = ['for_me', 'watched'] as const;
export type NotificationTab = (typeof NOTIFICATION_TABS)[number];

/** The kind of content a notification is about, for its icon (COL-118). */
export type ContentKind = 'page' | 'post' | 'comment' | 'task';

const TASK_TYPES = new Set(['task_assigned', 'task_due']);

/** The number on the bell, capped so it stays small. */
export function bellNumber(count: number): string {
  return count > MOST_SHOWN ? `${MOST_SHOWN}+` : String(count);
}

/** Where a notification goes: the exact spot. */
export function notificationLink(n: Notification): string {
  if (n.type === 'access_request') return `/pages/${n.page_id}?share=1`;
  if (n.type === 'export_ready') return `/exports/${n.data.export_id}`;
  if ((n.type === 'template_proposed' || n.type === 'template_decided') && n.data.template_id) {
    return `/templates/${n.data.template_id}/settings?section=versions`;
  }
  return n.comment_id ? `/pages/${n.page_id}#comment-${n.comment_id}` : `/pages/${n.page_id}`;
}

export function contentKind(n: Notification): ContentKind {
  if (n.comment_id) return 'comment';
  if (TASK_TYPES.has(n.type)) return 'task';
  return n.page_kind === 'post' ? 'post' : 'page';
}

/** One day of the panel: today, yesterday, or an earlier day that `at` names. */
export interface DayGroup {
  day: 'today' | 'yesterday' | 'earlier';
  at: string;
  items: Notification[];
}

function startOfDay(at: Date): number {
  return new Date(at.getFullYear(), at.getMonth(), at.getDate()).getTime();
}

/** Groups notifications, newest first already, under a heading per day (COL-116). */
export function byDay(items: Notification[], now: Date): DayGroup[] {
  const today = startOfDay(now);
  const groups: DayGroup[] = [];
  let current = Number.NaN;
  for (const n of items) {
    const day = startOfDay(new Date(n.at));
    const last = groups[groups.length - 1];
    if (last && day === current) {
      last.items.push(n);
      continue;
    }
    current = day;
    const ago = Math.round((today - day) / MS_PER_DAY);
    groups.push({ day: ago <= 0 ? 'today' : ago === 1 ? 'yesterday' : 'earlier', at: n.at, items: [n] });
  }
  return groups;
}

export function useNotificationSentence() {
  const { t } = useTranslation();
  return (n: Notification): string => {
    const [first] = n.actors;
    const name =
      n.actors.length > 1
        ? t('notifications.people', { count: n.actors.length })
        : (first ?? t('notifications.someone'));
    if (n.type === 'invitation_answer') {
      return t(`notifications.invitation_${n.data.answer ?? 'refused'}`, { name, email: n.data.email });
    }
    if (n.type === 'approval_release' && n.data.outcome !== 'released' && n.data.outcome !== 'unchanged') {
      // The page itself says why: its band names the reason in words (PUB-820).
      return t('notifications.approval_release_not', { page: pageTitle(t, n.page_title) });
    }
    const page = pageTitle(t, n.page_title);
    if (n.type === 'template_decided') {
      return t(`notifications.template_${n.data.decision ?? 'approved'}`, { name, page });
    }
    if (n.type === 'template_set') {
      const count = typeof n.data.pages === 'number' ? n.data.pages : 1;
      return n.data.ready
        ? t('notifications.template_set_ready', { page, count })
        : t('notifications.type_template_set', { name, page, count: count - 1 });
    }
    const words = t(`notifications.type_${n.type}`, { name, page, file: n.data.filename });
    // One mention for a whole set of pages says how many more of them mention the person (TPL-721).
    if (n.type === 'mention' && Array.isArray(n.data.pages) && n.data.pages.length > 1) {
      return t('notifications.mentionInSet', { words, count: n.data.pages.length - 1 });
    }
    return words;
  };
}

export function useRefreshNotifications() {
  const client = useQueryClient();
  return async () => {
    await client.invalidateQueries({ queryKey: ['notifications'] });
    await client.invalidateQueries({ queryKey: ['notification-count'] });
  };
}

function KindMark({ kind }: { kind: ContentKind }) {
  const { t } = useTranslation();
  const Icon = { page: IconPage, post: IconBlog, comment: IconComment, task: IconTask }[kind];
  return (
    <span className="notification-kind">
      <Icon size={KIND_ICON_SIZE} />
      <span className="visually-hidden">{t(`notifications.kind_${kind}`)}</span>
    </span>
  );
}

/**
 * One notification: the actor's picture; the sentence in bold with its age; the kind of
 * content with the space; the start of a comment; and the mark that reads or unreads it
 * at the inner end (COL-117, COL-118).
 */
export function NotificationItem({ n, onOpen }: { n: Notification; onOpen?: () => void }) {
  const { t } = useTranslation();
  const sentence = useNotificationSentence();
  const refresh = useRefreshNotifications();
  const mark = useMutation({
    mutationFn: (read: boolean) => api.put(`/me/notifications/${n.id}/${read ? 'read' : 'unread'}`),
    onSettled: refresh,
  });
  const markWords = n.read ? t('notifications.markUnread') : t('notifications.markRead');
  return (
    <li className={n.read ? 'notification' : 'notification unread'}>
      <Avatar name={n.actors[0] ?? t('notifications.someone')} size="small" />
      <div className="notification-main">
        <p className="notification-line">
          <Link
            href={notificationLink(n)}
            onClick={() => {
              if (!n.read) mark.mutate(true);
              onOpen?.();
            }}
          >
            {sentence(n)}
          </Link>{' '}
          <span className="muted">
            <RelativeTime value={n.at} />
          </span>
          {!n.read && <span className="visually-hidden">{t('notifications.unreadWord')}</span>}
        </p>
        <p className="notification-where">
          <KindMark kind={contentKind(n)} />
          {n.space_name}
        </p>
        {n.excerpt && <p className="notification-excerpt">{n.excerpt}</p>}
        {n.data.message && <q className="comment-quote">{n.data.message}</q>}
        {n.data.reason && <q className="comment-quote">{n.data.reason}</q>}
      </div>
      <button
        type="button"
        className="notification-mark"
        aria-label={markWords}
        title={markWords}
        onClick={() => mark.mutate(!n.read)}
      >
        <span className="notification-dot" />
      </button>
    </li>
  );
}

function DayHeading({ group }: { group: DayGroup }) {
  const { t } = useTranslation();
  const dates = useDates();
  return (
    <h3 className="notification-day">
      {group.day === 'earlier' ? dates.day(group.at) : t(`notifications.${group.day}`)}
    </h3>
  );
}

function Panel({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const [unreadOnly, setUnreadOnly] = useState(false);
  const [tab, setTab] = useState<NotificationTab>('for_me');
  const refresh = useRefreshNotifications();
  const q = useQuery({
    queryKey: ['notifications', 'panel', tab, unreadOnly],
    queryFn: () =>
      api.get<{ notifications: Notification[] }>(
        `/me/notifications?limit=${SHOWN_IN_PANEL}&unread=${unreadOnly}&tab=${tab}&excerpts=true`,
      ),
  });
  const readAll = useMutation({
    mutationFn: () => api.post(`/me/notifications/read-all?tab=${tab}`),
    onSettled: refresh,
  });
  const groups = byDay(q.data?.notifications ?? [], new Date());
  return (
    <div className="bell-panel" role="region" aria-label={t('notifications.title')}>
      <div className="bell-head">
        <h2>{t('notifications.title')}</h2>
        <div className="bell-tools">
          <label className="switch">
            {t('notifications.unreadOnly')}
            <input
              type="checkbox"
              role="switch"
              checked={unreadOnly}
              onChange={(e) => setUnreadOnly(e.target.checked)}
            />
          </label>
          <Link
            href="/notifications"
            className="button icon"
            aria-label={t('notifications.all')}
            title={t('notifications.all')}
            onClick={onClose}
          >
            <IconExternal size={18} />
          </Link>
          <Menu label={t('notifications.more')} className="icon" trigger={<IconMore size={18} />}>
            <MenuItem onClick={() => readAll.mutate()}>{t('notifications.readAll')}</MenuItem>
            <MenuLink href="/me/notifications" onClick={onClose}>
              {t('notifications.settings')}
            </MenuLink>
          </Menu>
        </div>
      </div>
      <Tabs
        label={t('notifications.title')}
        panel="bell-list"
        tabs={NOTIFICATION_TABS}
        current={tab}
        name={(name) => t(`notifications.tab_${name}`)}
        onChange={setTab}
      />
      <div id="bell-list" role="tabpanel" aria-labelledby={`bell-list-tab-${tab}`} className="bell-list">
        {q.isPending && <Loading />}
        <ErrorNote error={q.error ?? readAll.error} />
        {q.data?.notifications.length === 0 && <p className="muted">{t('notifications.none')}</p>}
        {groups.map((group) => (
          <section key={group.at} className="notification-group">
            <DayHeading group={group} />
            <ul className="plain-list">
              {group.items.map((n) => (
                <NotificationItem key={n.id} n={n} onOpen={onClose} />
              ))}
            </ul>
          </section>
        ))}
      </div>
    </div>
  );
}

export function NotificationBell() {
  const { t } = useTranslation();
  const root = useRef<HTMLDivElement>(null);
  const bell = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const count = useQuery({
    queryKey: ['notification-count'],
    queryFn: () => api.poll<{ unread: number }>('/me/notifications/count'),
    refetchInterval: spreadPoll(POLL_MS),
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
      // A menu inside the panel takes its own Escape first.
      if (e.key !== 'Escape' || e.defaultPrevented) return;
      if ((e.target as Element | null)?.closest?.('[role=menu]')) return;
      setOpen(false);
      bell.current?.focus();
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
        ref={bell}
        type="button"
        className="icon bell"
        aria-label={phrase}
        title={phrase}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <IconBell />
        {unread > 0 && !open && (
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
