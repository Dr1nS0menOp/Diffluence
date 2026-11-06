// SPDX-License-Identifier: Apache-2.0
// Keeping an instance healthy: maintenance mode, jobs and data care.
import { useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me } from '../api';
import { appearanceQuery } from '../appearance';
import { useDates } from '../dates';
import { ConfirmButton, ErrorNote, Field, formatSize } from '../components/ui';

interface Job {
  id: string;
  kind: string;
  status: string;
  created_at: string;
  finished_at: string | null;
}

interface Sized {
  name: string;
  size: number;
}

function Maintenance({ me }: { me: Me }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const state = useQuery(appearanceQuery).data?.maintenance;
  const [message, setMessage] = useState('');
  const set = useMutation({
    mutationFn: (on: boolean) => api.put('/admin/maintenance', { on, message }),
    onSettled: () => client.invalidateQueries({ queryKey: appearanceQuery.queryKey }),
  });
  if (!me.global_permissions.includes('system_administrator')) return null;
  return (
    <section>
      <h2>{t('care.maintenance')}</h2>
      <p className="muted">{t('care.maintenanceHelp')}</p>
      <ErrorNote error={set.error} />
      {state?.on ? (
        <button type="button" className="primary" onClick={() => set.mutate(false)}>
          {t('care.maintenanceOff')}
        </button>
      ) : (
        <form
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            set.mutate(true);
          }}
        >
          <Field label={t('care.maintenanceMessage')}>
            <input value={message} onChange={(e) => setMessage(e.target.value)} />
          </Field>
          <button type="submit" className="danger">
            {t('care.maintenanceOn')}
          </button>
        </form>
      )}
    </section>
  );
}

function Jobs() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const dates = useDates();
  const q = useQuery({ queryKey: ['admin-jobs'], queryFn: () => api.get<{ jobs: Job[] }>('/admin/jobs') });
  const act = useMutation({
    mutationFn: (v: { id: string; action: 'retry' | 'cancel' }) =>
      api.post(`/admin/jobs/${v.id}/${v.action}`),
    onSettled: () => client.invalidateQueries({ queryKey: ['admin-jobs'] }),
  });
  return (
    <section>
      <h2>{t('care.jobs')}</h2>
      <ErrorNote error={q.error ?? act.error} />
      {q.data?.jobs.length === 0 && <p className="muted">{t('care.noJobs')}</p>}
      {(q.data?.jobs.length ?? 0) > 0 && (
        <table>
          <thead>
            <tr>
              <th>{t('common.type')}</th>
              <th>{t('common.status')}</th>
              <th>{t('admin.when')}</th>
              <th>{t('common.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data?.jobs.map((j) => (
              <tr key={j.id}>
                <td>{j.kind}</td>
                <td>{j.status}</td>
                <td>{dates.moment(j.finished_at ?? j.created_at)}</td>
                <td>
                  {j.status === 'failed' && (
                    <button
                      type="button"
                      className="ghost"
                      onClick={() => act.mutate({ id: j.id, action: 'retry' })}
                    >
                      {t('care.retry')}
                    </button>
                  )}
                  {j.status === 'queued' && (
                    <button
                      type="button"
                      className="ghost"
                      onClick={() => act.mutate({ id: j.id, action: 'cancel' })}
                    >
                      {t('common.cancel')}
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

function SizedList({ title, rows }: { title: string; rows: Sized[] }) {
  return (
    <>
      <h3>{title}</h3>
      <ul className="plain-list">
        {rows.map((r) => (
          <li key={r.name} className="row">
            <span className="grow">{r.name}</span>
            <span className="muted">{formatSize(r.size)}</span>
          </li>
        ))}
      </ul>
    </>
  );
}

function Largest() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['admin-largest'],
    queryFn: () => api.get<{ spaces: Sized[]; files: Sized[] }>('/admin/reports/largest'),
  });
  return (
    <section>
      <h2>{t('care.largest')}</h2>
      <ErrorNote error={q.error} />
      {q.data && <SizedList title={t('admin.spaces')} rows={q.data.spaces} />}
      {q.data && <SizedList title={t('summary.files')} rows={q.data.files} />}
    </section>
  );
}

function UnusedFiles() {
  const { t } = useTranslation();
  const client = useQueryClient();
  const q = useQuery({
    queryKey: ['admin-unused'],
    queryFn: () => api.get<{ files: { key: string; size: number }[] }>('/admin/storage/unused'),
    enabled: false,
  });
  const remove = useMutation({
    mutationFn: () => api.post<{ removed: number }>('/admin/storage/unused'),
    onSuccess: () => client.resetQueries({ queryKey: ['admin-unused'] }),
  });
  const files = q.data?.files;
  return (
    <section>
      <h2>{t('care.unused')}</h2>
      <p className="muted">{t('care.unusedHelp')}</p>
      <ErrorNote error={q.error ?? remove.error} />
      {remove.data && (
        <p role="status" className="note">
          {t('care.unusedRemoved', { count: remove.data.removed })}
        </p>
      )}
      <div className="row">
        <button type="button" disabled={q.isFetching} onClick={() => void q.refetch()}>
          {t('care.unusedFind')}
        </button>
        {(files?.length ?? 0) > 0 && (
          <ConfirmButton
            label={t('care.unusedRemove')}
            message={t('care.unusedConfirm', { count: files?.length ?? 0 })}
            className="danger"
            onConfirm={() => remove.mutate()}
          />
        )}
      </div>
      {files?.length === 0 && <p className="muted">{t('care.unusedNone')}</p>}
      {files && (
        <SizedList title={t('summary.files')} rows={files.map((f) => ({ name: f.key, size: f.size }))} />
      )}
    </section>
  );
}

export function CareTab({ me }: { me: Me }) {
  return (
    <>
      <Maintenance me={me} />
      <Jobs />
      <Largest />
      <UnusedFiles />
    </>
  );
}
