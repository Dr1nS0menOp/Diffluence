// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Notification, type NotificationSettings, type Space, type WatchEntry } from '../api';
import { Link } from '../router';
import { NotificationRow } from '../components/notifications';
import { ErrorNote, Field, Loading, RelativeTime } from '../components/ui';

const PAGE_SIZE = 50;
const TYPES = [
  'mention',
  'task_assigned',
  'task_due',
  'share',
  'comment',
  'reply',
  'reaction',
  'page_changed',
  'access_request',
  'access_answer',
  'rule',
];
const CHANNELS = ['off', 'app', 'app_mail', 'app_digest'];
const HOURS = Array.from({ length: 24 }, (_, hour) => hour);
const DAYS = [0, 1, 2, 3, 4, 5, 6];

export function NotificationsView() {
  const { t } = useTranslation();
  const [type, setType] = useState('');
  const [spaceId, setSpaceId] = useState('');
  const spaces = useQuery({ queryKey: ['spaces'], queryFn: () => api.get<{ spaces: Space[] }>('/spaces') });
  const q = useInfiniteQuery({
    queryKey: ['notifications', 'all', type, spaceId],
    initialPageParam: '',
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: String(PAGE_SIZE), type });
      if (pageParam) params.set('before', pageParam);
      if (spaceId) params.set('space_id', spaceId);
      return api.get<{ notifications: Notification[] }>(`/me/notifications?${params.toString()}`);
    },
    getNextPageParam: (last) =>
      last.notifications.length === PAGE_SIZE
        ? last.notifications[last.notifications.length - 1]?.at
        : undefined,
  });
  const rows = q.data?.pages.flatMap((p) => p.notifications) ?? [];
  return (
    <>
      <h1>{t('notifications.title')}</h1>
      <div className="row">
        <Field label={t('notifications.filterType')}>
          <select value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">{t('notifications.allTypes')}</option>
            {TYPES.map((name) => (
              <option key={name} value={name}>
                {t(`notifications.channel_${name}`)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('home.filterSpace')}>
          <select value={spaceId} onChange={(e) => setSpaceId(e.target.value)}>
            <option value="">{t('search.anySpace')}</option>
            {spaces.data?.spaces.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </Field>
      </div>
      {q.isPending && <Loading />}
      <ErrorNote error={q.error} />
      {q.data && rows.length === 0 && <p className="muted">{t('notifications.none')}</p>}
      <ul className="plain-list">
        {rows.map((n) => (
          <NotificationRow key={n.id} n={n} />
        ))}
      </ul>
      {q.hasNextPage && (
        <button type="button" disabled={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>
          {t('common.more')}
        </button>
      )}
    </>
  );
}

function Check({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (next: boolean) => void;
}) {
  return (
    <label className="check">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      {label}
    </label>
  );
}

export function NotificationSettingsView() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['notification-settings'],
    queryFn: () => api.get<NotificationSettings>('/me/notification-settings'),
  });
  const [form, setForm] = useState<NotificationSettings | null>(null);
  const [saved, setSaved] = useState(false);
  useEffect(() => {
    if (q.data) setForm(q.data);
  }, [q.data]);
  const save = useMutation({
    mutationFn: (next: NotificationSettings) => api.put('/me/notification-settings', next),
    onSuccess: async () => {
      setSaved(true);
      await client.invalidateQueries({ queryKey: ['notification-settings'] });
    },
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!form) return <Loading />;
  const set = (change: Partial<NotificationSettings>) => {
    setSaved(false);
    setForm({ ...form, ...change });
  };
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  return (
    <>
      <h1>{t('notifications.settings')}</h1>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate({ ...form, time_zone: zone });
        }}
      >
        <ErrorNote error={save.error} />
        {saved && (
          <p role="status" className="note">
            {t('admin.saved')}
          </p>
        )}
        <h2>{t('notifications.perType')}</h2>
        {TYPES.map((name) => (
          <Field key={name} label={t(`notifications.channel_${name}`)}>
            <select
              value={form.channels[name] ?? 'app'}
              onChange={(e) => set({ channels: { ...form.channels, [name]: e.target.value } })}
            >
              {CHANNELS.map((channel) => (
                <option key={channel} value={channel}>
                  {t(`notifications.way_${channel}`)}
                </option>
              ))}
            </select>
          </Field>
        ))}
        <h2>{t('notifications.digest')}</h2>
        <Field label={t('notifications.digestWhen')}>
          <select value={form.digest_frequency} onChange={(e) => set({ digest_frequency: e.target.value })}>
            <option value="daily">{t('notifications.daily')}</option>
            <option value="weekly">{t('notifications.weekly')}</option>
          </select>
        </Field>
        {form.digest_frequency === 'weekly' && (
          <Field label={t('notifications.digestDay')}>
            <select value={form.digest_day} onChange={(e) => set({ digest_day: Number(e.target.value) })}>
              {DAYS.map((day) => (
                <option key={day} value={day}>
                  {t(`notifications.day_${day}`)}
                </option>
              ))}
            </select>
          </Field>
        )}
        <Field label={t('notifications.digestHour')} help={t('notifications.timeZone', { zone })}>
          <select value={form.digest_hour} onChange={(e) => set({ digest_hour: Number(e.target.value) })}>
            {HOURS.map((hour) => (
              <option key={hour} value={hour}>
                {String(hour).padStart(2, '0')}:00
              </option>
            ))}
          </select>
        </Field>
        <h2>{t('notifications.quiet')}</h2>
        <div className="row">
          <Field label={t('notifications.quietStart')}>
            <input
              type="time"
              value={form.quiet_start}
              onChange={(e) => set({ quiet_start: e.target.value })}
            />
          </Field>
          <Field label={t('notifications.quietEnd')}>
            <input type="time" value={form.quiet_end} onChange={(e) => set({ quiet_end: e.target.value })} />
          </Field>
        </div>
        <Field label={t('notifications.pause')} help={t('notifications.pauseHelp')}>
          <input
            type="date"
            value={form.paused_until}
            onChange={(e) => set({ paused_until: e.target.value })}
          />
        </Field>
        <h2>{t('notifications.automatic')}</h2>
        <Check
          label={t('notifications.autoCreate')}
          checked={form.auto_watch_create}
          onChange={(auto_watch_create) => set({ auto_watch_create })}
        />
        <Check
          label={t('notifications.autoEdit')}
          checked={form.auto_watch_edit}
          onChange={(auto_watch_edit) => set({ auto_watch_edit })}
        />
        <Check
          label={t('notifications.autoComment')}
          checked={form.auto_watch_comment}
          onChange={(auto_watch_comment) => set({ auto_watch_comment })}
        />
        <Check
          label={t('notifications.own')}
          checked={form.notify_own}
          onChange={(notify_own) => set({ notify_own })}
        />
        <Field label={t('notifications.commentOrder')}>
          <select value={form.comment_order} onChange={(e) => set({ comment_order: e.target.value })}>
            <option value="oldest">{t('notifications.oldest')}</option>
            <option value="newest">{t('notifications.newest')}</option>
          </select>
        </Field>
        <button type="submit" className="primary" disabled={save.isPending}>
          {t('common.save')}
        </button>
      </form>
    </>
  );
}

function hrefOf(entry: WatchEntry): string {
  return entry.type === 'space' ? '/spaces' : `/pages/${entry.id}`;
}

function stopPath(entry: WatchEntry): string {
  if (entry.type === 'space') return `/spaces/${entry.id}/watch`;
  return `/pages/${entry.id}/watch?scope=${entry.type}`;
}

export function WatchingView() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [filter, setFilter] = useState('');
  const q = useQuery({
    queryKey: ['watches'],
    queryFn: () => api.get<{ watches: WatchEntry[] }>('/me/watches'),
  });
  const stop = useMutation({
    mutationFn: (entry: WatchEntry) => api.del(stopPath(entry)),
    onSettled: () => client.invalidateQueries({ queryKey: ['watches'] }),
  });
  const rows =
    q.data?.watches.filter((w) => w.title.toLowerCase().includes(filter.trim().toLowerCase())) ?? [];
  return (
    <>
      <h1>{t('watching.title')}</h1>
      <input
        type="search"
        className="list-filter"
        value={filter}
        aria-label={t('watching.filter')}
        placeholder={t('watching.filter')}
        onChange={(e) => setFilter(e.target.value)}
      />
      {q.isPending && <Loading />}
      <ErrorNote error={q.error ?? stop.error} />
      {q.data && rows.length === 0 && <p className="muted">{t('watching.none')}</p>}
      <ul className="page-list">
        {rows.map((w) => (
          <li key={w.type + w.id}>
            <Link href={hrefOf(w)}>{w.title}</Link>
            <span className="muted">
              {t(`watching.kind_${w.type}`)} · <RelativeTime value={w.since} />
            </span>
            <button type="button" className="ghost" onClick={() => stop.mutate(w)}>
              {t('watching.stop')}
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}
