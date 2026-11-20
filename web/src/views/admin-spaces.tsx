// SPDX-License-Identifier: Apache-2.0
// Administration: every space of the instance (a tool section), and the spaces in the trash (a
// settings section of Data, ADM-075). Both call the routes they called as one screen.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Space, type TrashItem } from '../api';
import { Link } from '../router';
import { ConfirmButton, Dialog, ErrorNote, Field, formatSize, Loading } from '../components/ui';
import { TableBox, TableEmpty } from '../components/table-box';
import { useDates } from '../dates';
import { StorageLimitDialog } from './admin-storage';
import { matchesSpaceFilter, NO_SPACE_FILTER, QUIET_MONTHS, type SpaceFilter } from './admin-space-filter';

interface SpaceTotal {
  id: string;
  pages: number;
  bytes: number;
  last_activity: string | null;
  limit_mib: number;
}

const MEBIBYTE = 1 << 20;
const SPACE_COLUMNS = 7;
const TRASH_COLUMNS = 3;

function RecoverDialog({ space, onClose }: { space: Space; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const [reason, setReason] = useState('');
  const recover = useMutation({
    mutationFn: () => api.post(`/admin/spaces/${space.id}/recover`, { reason }),
    onSuccess: async () => {
      onClose();
      await client.invalidateQueries({ queryKey: ['spaces'] });
    },
  });
  return (
    <Dialog title={t('admin.recover')} onClose={onClose}>
      <p>{t('admin.recoverHelp')}</p>
      <form
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          recover.mutate();
        }}
      >
        <ErrorNote error={recover.error} />
        <Field label={t('admin.reason')}>
          <input value={reason} onChange={(e) => setReason(e.target.value)} required autoFocus />
        </Field>
        <div className="dialog-actions">
          <button type="submit" className="primary">
            {t('admin.recover')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

function SpaceFilters({ filter, onChange }: { filter: SpaceFilter; onChange: (next: SpaceFilter) => void }) {
  const { t } = useTranslation();
  const count = (raw: string) => Math.max(0, Number.parseInt(raw, 10) || 0);
  return (
    <div className="row">
      <input
        type="search"
        value={filter.text}
        onChange={(e) => onChange({ ...filter, text: e.target.value })}
        aria-label={t('admin.spaceFind')}
        placeholder={t('admin.spaceFind')}
      />
      <input
        type="number"
        min={0}
        value={filter.minPages || ''}
        onChange={(e) => onChange({ ...filter, minPages: count(e.target.value) })}
        aria-label={t('admin.spaceMinPages')}
        placeholder={t('admin.spaceMinPages')}
      />
      <input
        type="number"
        min={0}
        value={filter.minMiB || ''}
        onChange={(e) => onChange({ ...filter, minMiB: count(e.target.value) })}
        aria-label={t('admin.spaceMinMiB')}
        placeholder={t('admin.spaceMinMiB')}
      />
      <select
        aria-label={t('admin.spaceQuiet')}
        value={filter.quietMonths}
        onChange={(e) => onChange({ ...filter, quietMonths: Number(e.target.value) })}
      >
        {QUIET_MONTHS.map((m) => (
          <option key={m} value={m}>
            {m === 0 ? `${t('admin.spaceQuiet')}: ${t('admin.any')}` : t('admin.spaceQuietFor', { count: m })}
          </option>
        ))}
      </select>
    </div>
  );
}

export function SpacesTab() {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['admin-spaces'],
    queryFn: () => api.get<{ spaces: Space[]; totals: SpaceTotal[] }>('/admin/spaces'),
  });
  const [recovering, setRecovering] = useState<Space | null>(null);
  const [limiting, setLimiting] = useState<Space | null>(null);
  const [filter, setFilter] = useState<SpaceFilter>(NO_SPACE_FILTER);
  const now = new Date();
  const totalOf = (id: string) => q.data?.totals.find((row) => row.id === id);
  const shown = (q.data?.spaces ?? []).filter((s) => {
    const total = totalOf(s.id);
    return matchesSpaceFilter(
      {
        name: s.name,
        key: s.key,
        pages: total?.pages ?? 0,
        bytes: total?.bytes ?? 0,
        lastActivity: total?.last_activity ?? null,
      },
      filter,
      now,
    );
  });
  return (
    <>
      <ErrorNote error={q.error} />
      <SpaceFilters filter={filter} onChange={setFilter} />
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('space.key')}</th>
              <th>{t('common.status')}</th>
              <th>{t('admin.spacePages')}</th>
              <th>{t('admin.spaceStorage')}</th>
              <th>{t('admin.spaceActivity')}</th>
              <th>
                <span className="visually-hidden">{t('common.actions')}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {q.data && shown.length === 0 && (
              <TableEmpty columns={SPACE_COLUMNS}>{t('admin.spaceNoneFit')}</TableEmpty>
            )}
            {shown.map((s) => {
              const total = totalOf(s.id);
              return (
                <tr key={s.id}>
                  <td>
                    <Link href={`/spaces/${s.key}`}>{s.name}</Link>
                  </td>
                  <td>{s.key}</td>
                  <td>{s.status}</td>
                  <td>{total?.pages ?? 0}</td>
                  <td>
                    {formatSize(total?.bytes ?? 0)}
                    {total?.limit_mib ? ` / ${formatSize(total.limit_mib * MEBIBYTE)}` : ''}
                  </td>
                  <td>{total?.last_activity ? dates.moment(total.last_activity) : ''}</td>
                  <td className="row-actions">
                    <button type="button" className="ghost" onClick={() => setLimiting(s)}>
                      {t('admin.storageLimit')}
                    </button>
                    <button type="button" className="ghost" onClick={() => setRecovering(s)}>
                      {t('admin.recover')}
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </TableBox>
      {limiting && (
        <StorageLimitDialog
          space={limiting}
          limitMiB={totalOf(limiting.id)?.limit_mib ?? 0}
          onClose={() => setLimiting(null)}
        />
      )}
      {recovering && <RecoverDialog space={recovering} onClose={() => setRecovering(null)} />}
    </>
  );
}

/** Spaces in the trash (ADM-075): each waits until its time is up, and can come back until then. */
export function SpacesTrashTab() {
  const { t } = useTranslation();
  const dates = useDates();
  const client = useQueryClient();
  const trashed = useQuery({
    queryKey: ['admin-trash'],
    queryFn: () => api.get<{ items: TrashItem[] }>('/admin/trash/spaces'),
  });
  const refresh = async () => {
    await client.invalidateQueries({ queryKey: ['admin-trash'] });
    await client.invalidateQueries({ queryKey: ['admin-spaces'] });
  };
  const restore = useMutation({
    mutationFn: (id: string) => api.post(`/trash/${id}/restore`),
    onSettled: refresh,
  });
  const purge = useMutation({ mutationFn: (id: string) => api.del(`/trash/${id}`), onSettled: refresh });
  if (trashed.error) return <ErrorNote error={trashed.error} />;
  if (!trashed.data) return <Loading />;
  return (
    <>
      <ErrorNote error={restore.error ?? purge.error} />
      <p className="warning-line">{t('admin.purgeWarning')}</p>
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('common.name')}</th>
              <th>{t('admin.purgeAfter')}</th>
              <th>
                <span className="visually-hidden">{t('common.actions')}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {trashed.data.items.length === 0 && (
              <TableEmpty columns={TRASH_COLUMNS}>{t('admin.trashEmpty')}</TableEmpty>
            )}
            {trashed.data.items.map((i) => (
              <tr key={i.id}>
                <td>{i.title}</td>
                <td>{dates.moment(i.purge_after)}</td>
                <td className="row-actions">
                  <button type="button" className="ghost" onClick={() => restore.mutate(i.id)}>
                    {t('trash.restore')}
                  </button>
                  <ConfirmButton
                    label={t('trash.purge')}
                    message={t('trash.confirmPurge')}
                    className="danger"
                    onConfirm={() => purge.mutate(i.id)}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </>
  );
}
