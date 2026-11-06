// SPDX-License-Identifier: Apache-2.0
// The first page of the console: how the instance is doing, and what needs attention.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api';
import { Link, navigate } from '../router';
import { SubjectPicker } from '../components/picker';
import { Dialog, ErrorNote, Field, formatSize, Loading } from '../components/ui';
import { findSettings, SETTINGS_INDEX } from './admin-search';

interface OrphanedPage {
  id: string;
  space_name: string;
}

interface Summary {
  users: number;
  spaces: number;
  pages: number;
  files: number;
  storage: number;
  failed_jobs: number;
}

interface AttentionItem {
  kind: string;
  count: number;
}

interface UnmanagedSpace {
  id: string;
  key: string;
  name: string;
  kind: string;
}

interface Attention {
  items: AttentionItem[];
  unmanaged_spaces: UnmanagedSpace[];
  orphaned_pages: OrphanedPage[];
}

/** Where to go to act on what needs attention. */
const ATTENTION_LINKS: Record<string, string> = {
  failed_jobs: '/admin/care',
  accounts_expiring: '/admin/users?type=guest',
  removal_requests: '/admin/audit?action=account.removal_requested',
  nodes_differ: '/admin/system',
  no_passkey: '/me/security',
  tokens_expiring: '/admin/scim',
  invitations_waiting: '/admin/invitations',
  events_failed: '/admin/care',
  events_waiting: '/admin/care',
  audit_damaged: '/admin/audit',
};

function SummaryTiles() {
  const { t } = useTranslation();
  const q = useQuery({ queryKey: ['admin-summary'], queryFn: () => api.get<Summary>('/admin/summary') });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const s = q.data;
  const tiles: [string, string | number][] = [
    [t('admin.users'), s.users],
    [t('admin.spaces'), s.spaces],
    [t('summary.pages'), s.pages],
    [t('summary.files'), s.files],
    [t('admin.storage'), formatSize(s.storage)],
    [t('summary.failedJobs'), s.failed_jobs],
  ];
  return (
    <dl className="tiles">
      {tiles.map(([name, value]) => (
        <div key={name}>
          <dt>{name}</dt>
          <dd>{value}</dd>
        </div>
      ))}
    </dl>
  );
}

function HandoverDialog({ space, onClose }: { space: UnmanagedSpace; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const hand = useMutation({
    mutationFn: (to: string) => api.post(`/admin/spaces/${space.id}/handover`, { to }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['admin-attention'] });
      onClose();
    },
  });
  return (
    <Dialog title={t('summary.handover', { name: space.name })} onClose={onClose}>
      <p className="muted">{t('summary.handoverHelp')}</p>
      <ErrorNote error={hand.error} />
      <SubjectPicker label={t('summary.handoverTo')} usersOnly onPick={(e) => hand.mutate(e.id)} />
    </Dialog>
  );
}

function RecoverPageDialog({ page, onClose }: { page: OrphanedPage; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [reason, setReason] = useState('');
  const recover = useMutation({
    mutationFn: () => api.post(`/admin/pages/${page.id}/recover`, { reason }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: ['admin-attention'] });
      onClose();
    },
  });
  return (
    <Dialog title={t('summary.orphanedPage', { space: page.space_name })} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          recover.mutate();
        }}
      >
        <ErrorNote error={recover.error} />
        <Field label={t('summary.orphanedReason')}>
          <input value={reason} onChange={(e) => setReason(e.target.value)} maxLength={500} required />
        </Field>
        <button type="submit" className="primary">
          {t('summary.recoverPage')}
        </button>
      </form>
    </Dialog>
  );
}

function AttentionList() {
  const { t } = useTranslation();
  const [handing, setHanding] = useState<UnmanagedSpace | null>(null);
  const [recovering, setRecovering] = useState<OrphanedPage | null>(null);
  const q = useQuery({
    queryKey: ['admin-attention'],
    queryFn: () => api.get<Attention>('/admin/attention'),
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const { items, unmanaged_spaces: unmanaged, orphaned_pages: orphaned } = q.data;
  return (
    <section>
      <h2>{t('summary.attention')}</h2>
      {items.length === 0 && unmanaged.length === 0 && <p className="muted">{t('summary.allFine')}</p>}
      <ul className="plain-list attention">
        {items.map((item) => (
          <li key={item.kind} className="note note-error">
            <span className="grow">{t(`summary.item_${item.kind}`, { count: item.count })}</span>
            {ATTENTION_LINKS[item.kind] && (
              <Link href={ATTENTION_LINKS[item.kind] ?? ''}>{t('summary.look')}</Link>
            )}
          </li>
        ))}
      </ul>
      {unmanaged.length > 0 && (
        <>
          <h3>{t('summary.unmanaged')}</h3>
          <ul className="plain-list">
            {unmanaged.map((s) => (
              <li key={s.id} className="row">
                <Link href={`/spaces/${s.key}`} className="grow">
                  {s.name}
                </Link>
                <button type="button" className="ghost" onClick={() => setHanding(s)}>
                  {t('summary.handoverButton')}
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
      {orphaned.length > 0 && (
        <>
          <h3>{t('summary.orphaned')}</h3>
          <ul className="plain-list">
            {orphaned.map((page) => (
              <li key={page.id} className="row">
                <span className="grow">{t('summary.orphanedPage', { space: page.space_name })}</span>
                <button type="button" className="ghost" onClick={() => setRecovering(page)}>
                  {t('summary.recoverPage')}
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
      {handing && <HandoverDialog space={handing} onClose={() => setHanding(null)} />}
      {recovering && <RecoverPageDialog page={recovering} onClose={() => setRecovering(null)} />}
    </section>
  );
}

export function SummaryTab() {
  return (
    <>
      <SummaryTiles />
      <AttentionList />
    </>
  );
}

/** Finds a setting by name, and goes to its screen. */
export function SettingsSearch() {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const found = findSettings(SETTINGS_INDEX, text, (key) => t(key));
  return (
    <div className="picker settings-search">
      <input
        type="search"
        value={text}
        aria-label={t('admin.findSetting')}
        placeholder={t('admin.findSetting')}
        onChange={(e) => setText(e.target.value)}
      />
      {found.length > 0 && (
        <ul className="plain-list">
          {found.map((entry) => (
            <li key={entry.label}>
              <button
                type="button"
                className="ghost"
                onClick={() => {
                  setText('');
                  navigate(`/admin/${entry.tab}`);
                }}
              >
                {t(entry.label)}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
