// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me } from '../api';
import { useCapabilities } from '../capabilities';
import { ExperimentalOff } from '../components/experimental-off';
import { ConfirmButton, ErrorNote, Field, formatSize, Loading } from '../components/ui';
import { useDates } from '../dates';
import { TableBox } from '../components/table-box';
import { ConfirmFields, needsConfirmation, type Confirmation } from '../components/confirm-change';

/** How often the backup page reads the worker's state again: a request is taken within a minute. */
const BACKUPS_POLL_MS = 15_000;

/** Restores the safety backup after a restore that stopped halfway (spec 059). A command, not text. */
const WAY_BACK_COMMAND = 'docker compose run --rm backup way-back';

/** Shows the log where the backup service writes the code that confirms a restore (spec 070). */
const CODE_LOG_COMMAND = 'docker compose logs backup';

interface BackupRun {
  name: string;
  reason: 'schedule' | 'request' | 'manual';
  started_at: string;
  finished_at: string;
  ok: boolean;
  size_bytes: number | null;
  message: string;
  restore_ok: boolean | null;
  restore_at: string | null;
  restore_seconds: number | null;
}

interface BackupOverview {
  settings: { interval_hours: number; keep: number };
  worker: {
    seen_at: string;
    absent: boolean;
    default_interval_hours: number;
    default_keep: number;
    key_present: boolean;
    key_fingerprint: string;
    /** What the backup service holds to; 0 until it said (spec 076). */
    held_keep: number;
    held_interval_hours: number;
  } | null;
  /** An ask for fewer backups or fewer rounds that waits for the code from the log. */
  confirmation: {
    asked_at: string;
    keep: number;
    interval_hours: number;
    code_shown: boolean;
    code_entered: boolean;
    wrong_codes: number;
  } | null;
  confirmation_tries: number;
  request: { requested_at: string; claimed_at: string | null } | null;
  runs: BackupRun[];
  restore_check: {
    name: string;
    checked_at: string;
    ok: boolean;
    seconds: number | null;
    message: string;
  } | null;
  requests_per_hour: number;
  restore: Restore | null;
  installation_name: string;
  restores_per_day: number;
  restore_code_tries: number;
}

interface Restore {
  id: string;
  backup_name: string;
  requested_by_name: string;
  requested_at: string;
  state:
    | 'requested'
    | 'preparing'
    | 'stopping'
    | 'backing_up'
    | 'restoring'
    | 'checking'
    | 'done'
    | 'refused'
    | 'failed';
  step: string;
  safety_name: string;
  changed: boolean;
  updated_at: string;
  finished_at: string | null;
  /** The backup service wrote the confirmation code to its log (spec 070). */
  code_shown: boolean;
  /** A code was entered and waits for the backup service to compare it. */
  code_entered: boolean;
  wrong_codes: number;
}

/** A whole number of 1 or more from a field, or 0 for "as the installation was started". */
function wholeOrDefault(text: string): number {
  const n = Number.parseInt(text, 10);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

/** Whether the backup service holds to more backups, or more rounds, than the schedule asks:
 * fewer take effect only with the code from its log (spec 076). */
function holdsMore(data: BackupOverview): boolean {
  const w = data.worker;
  if (!w || w.held_keep === 0) return false;
  const keep = data.settings.keep || w.default_keep;
  const hours = data.settings.interval_hours || w.default_interval_hours;
  return w.held_keep > keep || w.held_interval_hours < hours;
}

function BackupSchedule({ data, isSystem }: { data: BackupOverview; isSystem: boolean }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [hours, setHours] = useState(
    data.settings.interval_hours ? String(data.settings.interval_hours) : '',
  );
  const [keep, setKeep] = useState(data.settings.keep ? String(data.settings.keep) : '');
  const [confirm, setConfirm] = useState<Confirmation | null>(null);
  useEffect(() => {
    setHours(data.settings.interval_hours ? String(data.settings.interval_hours) : '');
    setKeep(data.settings.keep ? String(data.settings.keep) : '');
  }, [data.settings.interval_hours, data.settings.keep]);
  const save = useMutation({
    mutationFn: () => {
      const schedule = { interval_hours: wholeOrDefault(hours), keep: wholeOrDefault(keep) };
      return confirm
        ? api.put('/admin/settings/backups/confirmed', { settings: schedule, code: confirm.code.trim() })
        : api.put('/admin/settings/backups', schedule);
    },
    onSuccess: () => {
      setConfirm(null);
      return qc.invalidateQueries({ queryKey: ['backups'] });
    },
    onError: (error) => {
      if (isSystem && needsConfirmation(error)) setConfirm((c) => c ?? { code: '', reason: '' });
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  const w = data.worker;
  return (
    <form onSubmit={submit} className="settings-form">
      <h2>{t('running.schedule')}</h2>
      {w && holdsMore(data) && (
        <p role="status" className="note">
          {t('running.heldMore', { keep: w.held_keep, hours: w.held_interval_hours })}
        </p>
      )}
      <Field
        label={t('running.intervalHours')}
        help={
          w
            ? t('running.intervalHelp', { hours: w.default_interval_hours })
            : t('running.intervalHelpNoWorker')
        }
      >
        <input inputMode="numeric" value={hours} onChange={(e) => setHours(e.target.value)} />
      </Field>
      <Field
        label={t('running.keep')}
        help={w ? t('running.keepHelp', { keep: w.default_keep }) : t('running.keepHelpNoWorker')}
      >
        <input inputMode="numeric" value={keep} onChange={(e) => setKeep(e.target.value)} />
      </Field>
      <ErrorNote error={save.error} />
      {confirm && <ConfirmFields value={confirm} onChange={setConfirm} note={t('running.fewerNote')} />}
      <div>
        <button type="submit" className="primary" disabled={save.isPending}>
          {confirm ? t('rules.confirmButton') : t('common.save')}
        </button>
      </div>
    </form>
  );
}

/** Where an ask for fewer backups stands: waiting for the code in the log, or for its check. */
function FewerBackups({ data, isSystem }: { data: BackupOverview; isSystem: boolean }) {
  const { t } = useTranslation();
  const c = data.confirmation;
  if (!c) return null;
  if (!c.code_shown) {
    return (
      <p role="status" className="note">
        {t('running.fewerAsked')}
      </p>
    );
  }
  if (!isSystem) {
    return (
      <p role="status" className="note">
        {t('running.fewerSystemOnly')}
      </p>
    );
  }
  return (
    <LogCode
      path="/admin/backups/schedule/code"
      entered={c.code_entered}
      wrongCodes={c.wrong_codes}
      tries={data.confirmation_tries}
      button={t('running.fewerCodeButton')}
    />
  );
}

function WorkerState({ data }: { data: BackupOverview }) {
  const { t } = useTranslation();
  const dates = useDates();
  const w = data.worker;
  if (!w) {
    return (
      <p role="status" className="note">
        {t('running.workerNone')}
      </p>
    );
  }
  return (
    <>
      <p role="status" className={w.absent ? 'note' : 'muted'}>
        {w.absent
          ? t('running.workerAbsent', { when: dates.moment(w.seen_at) })
          : t('running.workerSeen', { when: dates.moment(w.seen_at) })}
      </p>
      <p className={w.key_present ? 'muted' : 'note'}>
        {w.key_present
          ? t('running.keyPresent', { fingerprint: w.key_fingerprint })
          : t('running.keyMissing')}
      </p>
    </>
  );
}

function BackUpNow({ data }: { data: BackupOverview }) {
  const { t } = useTranslation();
  const dates = useDates();
  const qc = useQueryClient();
  const ask = useMutation({
    mutationFn: () => api.post('/admin/backups/request'),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backups'] }),
  });
  const r = data.request;
  return (
    <>
      {r ? (
        <p role="status">
          {r.claimed_at
            ? t('running.making', { when: dates.moment(r.requested_at) })
            : t('running.requested', { when: dates.moment(r.requested_at) })}
        </p>
      ) : (
        <p>
          <button type="button" onClick={() => ask.mutate()} disabled={ask.isPending}>
            {t('running.backUpNow')}
          </button>{' '}
          <small className="muted">{t('running.backUpNowHelp', { count: data.requests_per_hour })}</small>
        </p>
      )}
      <ErrorNote error={ask.error} />
    </>
  );
}

function BackupList({ runs }: { runs: BackupRun[] }) {
  const { t } = useTranslation();
  const dates = useDates();
  if (runs.length === 0) return <p className="muted">{t('running.empty')}</p>;
  return (
    <TableBox>
      <table>
        <thead>
          <tr>
            <th>{t('running.colTime')}</th>
            <th>{t('running.colReason')}</th>
            <th>{t('running.colSize')}</th>
            <th>{t('running.colState')}</th>
            <th>{t('running.colRestore')}</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((r) => (
            <tr key={`${r.name}-${r.finished_at}`}>
              <td>
                <time dateTime={r.finished_at}>{dates.moment(r.finished_at)}</time>
                <br />
                <small className="muted">{r.name}</small>
              </td>
              <td>{t(`running.reason_${r.reason}`)}</td>
              <td>{r.size_bytes === null ? '' : formatSize(r.size_bytes)}</td>
              <td>{r.ok ? t('running.stateOk') : t('running.stateFailed', { message: r.message })}</td>
              <td>
                {r.restore_ok === null
                  ? t('running.restoreNotTested')
                  : r.restore_ok
                    ? t('running.restoreTestedOk', { seconds: r.restore_seconds ?? 0 })
                    : t('running.restoreTestedFailed')}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableBox>
  );
}

function RestoreCheck({ data }: { data: BackupOverview }) {
  const { t } = useTranslation();
  const dates = useDates();
  const c = data.restore_check;
  return (
    <>
      <h2>{t('running.restoreTitle')}</h2>
      {!c ? (
        <p className="note">{t('running.restoreNone')}</p>
      ) : c.ok ? (
        <p>
          {t('running.restoreOk', {
            name: c.name,
            when: dates.moment(c.checked_at),
            seconds: c.seconds ?? 0,
          })}
        </p>
      ) : (
        <p role="alert" className="note note-error">
          {t('running.restoreFailed', { name: c.name, when: dates.moment(c.checked_at), message: c.message })}
        </p>
      )}
    </>
  );
}

/** The code the backup service wrote to its log, typed in by someone who can read that log
 * (specs 070 and 076). Only a system administrator, who could ask for what it confirms,
 * enters it. */
function LogCode({
  path,
  entered,
  wrongCodes,
  tries,
  button,
}: {
  path: string;
  entered: boolean;
  wrongCodes: number;
  tries: number;
  button: string;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [code, setCode] = useState('');
  const enter = useMutation({
    mutationFn: () => api.post(path, { code }),
    onSuccess: () => {
      setCode('');
      return qc.invalidateQueries({ queryKey: ['backups'] });
    },
  });
  if (entered) {
    return <p role="status">{t('running.restoreCodeWaits')}</p>;
  }
  const submit = (e: FormEvent) => {
    e.preventDefault();
    enter.mutate();
  };
  return (
    <form onSubmit={submit} className="settings-form">
      {wrongCodes > 0 && (
        <p role="alert" className="note">
          {t('running.restoreCodeWrong', { count: Math.max(tries - wrongCodes, 0) })}
        </p>
      )}
      <p>{t('running.restoreCodeShown')}</p>
      <pre>
        <code>{CODE_LOG_COMMAND}</code>
      </pre>
      <Field label={t('running.restoreCodeLabel')}>
        <input
          value={code}
          onChange={(e) => setCode(e.target.value)}
          autoComplete="off"
          inputMode="numeric"
        />
      </Field>
      <ErrorNote error={enter.error} />
      <div>
        <button type="submit" disabled={enter.isPending || code.trim() === ''}>
          {button}
        </button>
      </div>
    </form>
  );
}

/** Where the latest restore stands: waiting for the worker, running, done, or stopped. Only a
 * system administrator confirms or takes back a restore (spec 076). */
function RestoreState({
  r,
  tries,
  canConfirm,
  canCancel,
}: {
  r: Restore;
  tries: number;
  canConfirm: boolean;
  canCancel: boolean;
}) {
  const { t } = useTranslation();
  const dates = useDates();
  const qc = useQueryClient();
  const cancel = useMutation({
    mutationFn: () => api.del('/admin/backups/restore'),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backups'] }),
  });
  if (r.state === 'requested') {
    return (
      <>
        <p role="status">
          {t('running.restoreAsked', { name: r.backup_name, when: dates.moment(r.requested_at) })}
        </p>
        {canConfirm && r.code_shown && (
          <LogCode
            path="/admin/backups/restore/code"
            entered={r.code_entered}
            wrongCodes={r.wrong_codes}
            tries={tries}
            button={t('running.restoreCodeButton')}
          />
        )}
        {canCancel && (
          <p>
            <button type="button" onClick={() => cancel.mutate()} disabled={cancel.isPending}>
              {t('running.restoreCancel')}
            </button>
          </p>
        )}
        <ErrorNote error={cancel.error} />
      </>
    );
  }
  if (r.state === 'done') {
    return (
      <p role="status" className="muted">
        {t('running.restoreDone', {
          name: r.backup_name,
          when: dates.moment(r.finished_at ?? r.updated_at),
          safety: r.safety_name,
        })}
      </p>
    );
  }
  if (r.state === 'refused') {
    return (
      <p role="status" className="note">
        {t('running.restoreRefused', { name: r.backup_name, step: r.step })}
      </p>
    );
  }
  if (r.state === 'failed') {
    return (
      <div role="alert" className="note note-error">
        <p>{t('running.restoreBroke', { name: r.backup_name, step: r.step })}</p>
        <pre>
          <code>{WAY_BACK_COMMAND}</code>
        </pre>
      </div>
    );
  }
  return <p role="status">{t('running.restoreRunning', { name: r.backup_name, step: r.step })}</p>;
}

/** A restore over the live installation (spec 059): the worker stops every node, makes a
 * safety backup, restores and checks. It asks for the installation's name typed out and a
 * second factor; a passkey confirmed just before counts too. */
function RestoreOver({ data }: { data: BackupOverview }) {
  const { t } = useTranslation();
  const dates = useDates();
  const qc = useQueryClient();
  const good = data.runs.filter((r) => r.ok);
  const [name, setName] = useState('');
  const [confirm, setConfirm] = useState('');
  const [code, setCode] = useState('');
  const chosen = good.find((r) => r.name === name) ?? good[0];
  const ask = useMutation({
    mutationFn: () =>
      api.post('/admin/backups/restore', { name: chosen?.name ?? '', confirm, code: code.trim() }),
    onSuccess: () => {
      setConfirm('');
      setCode('');
      return qc.invalidateQueries({ queryKey: ['backups'] });
    },
  });
  const open = data.restore !== null && data.restore.finished_at === null;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    ask.mutate();
  };
  return (
    <>
      <h2>{t('running.restoreHowTitle')}</h2>
      {data.restore && <RestoreState r={data.restore} tries={data.restore_code_tries} canConfirm canCancel />}
      {!open && chosen && (
        <form onSubmit={submit} className="settings-form">
          <Field label={t('running.restoreWhich')}>
            <select value={chosen.name} onChange={(e) => setName(e.target.value)}>
              {good.map((r) => (
                <option key={r.name} value={r.name}>
                  {r.name}
                </option>
              ))}
            </select>
          </Field>
          <p className="note">
            {t('running.restoreLoss', { name: chosen.name, when: dates.moment(chosen.started_at) })}
          </p>
          <p>{t('running.restoreSafety')}</p>
          <Field label={t('running.restoreConfirmLabel', { name: data.installation_name })}>
            <input value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="off" />
          </Field>
          <Field label={t('running.restoreCode')} help={t('running.restoreCodeHelp')}>
            <input
              value={code}
              onChange={(e) => setCode(e.target.value)}
              autoComplete="one-time-code"
              inputMode="text"
            />
          </Field>
          <ErrorNote error={ask.error} />
          <div>
            <button
              type="submit"
              className="danger"
              disabled={ask.isPending || confirm.trim() !== data.installation_name}
            >
              {t('running.restoreButton')}
            </button>{' '}
            <small className="muted">{t('running.restoreLimit', { count: data.restores_per_day })}</small>
          </div>
        </form>
      )}
      {chosen && (
        <>
          <p>{t('running.restoreByCommand')}</p>
          <pre>
            <code>{`docker compose exec backup diffluence backup restore-request --name ${chosen.name} --confirm '${data.installation_name}'`}</code>
          </pre>
        </>
      )}
    </>
  );
}

export function BackupsTab({ me }: { me: Me }) {
  const { t } = useTranslation();
  const capabilities = useCapabilities();
  const q = useQuery({
    queryKey: ['backups'],
    queryFn: () => api.poll<BackupOverview>('/admin/backups'),
    refetchInterval: BACKUPS_POLL_MS,
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const isSystem = me.global_permissions.includes('system_administrator');
  return (
    <>
      <p>{t('running.backupsIntro')}</p>
      <WorkerState data={q.data} />
      <BackUpNow data={q.data} />
      <BackupSchedule data={q.data} isSystem={isSystem} />
      <FewerBackups data={q.data} isSystem={isSystem} />
      <h2>{t('running.list')}</h2>
      <BackupList runs={q.data.runs} />
      <RestoreCheck data={q.data} />
      {/* A restore brings back every account and grant of the backup (spec 066). Restore
          from the console is an experimental part (spec 069). */}
      {isSystem && capabilities.console_restore ? (
        <RestoreOver data={q.data} />
      ) : (
        <>
          <h2>{t('running.restoreHowTitle')}</h2>
          {q.data.restore && (
            <RestoreState
              r={q.data.restore}
              tries={q.data.restore_code_tries}
              canConfirm={false}
              canCancel={isSystem}
            />
          )}
          {capabilities.console_restore ? (
            <p className="muted">{t('running.restoreSystemOnly')}</p>
          ) : (
            <ExperimentalOff />
          )}
        </>
      )}
    </>
  );
}

interface Configuration {
  settings: { name: string; value: string; source: string }[];
  problems: string[];
}

export function ConfigurationTab() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['configuration'],
    queryFn: () => api.get<Configuration>('/admin/configuration'),
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  return (
    <>
      <p>{t('running.configIntro')}</p>
      {q.data.problems.length > 0 && (
        <div role="alert" className="note note-error">
          <p>{t('running.configProblems')}</p>
          <ul>
            {q.data.problems.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        </div>
      )}
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('running.colSetting')}</th>
              <th>{t('running.colValue')}</th>
              <th>{t('running.colSource')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data.settings.map((s) => (
              <tr key={s.name}>
                <th scope="row">
                  <code>{s.name}</code>
                </th>
                <td>{s.value}</td>
                <td>{s.source}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </>
  );
}

export function SessionsTab() {
  const { t } = useTranslation();
  const [email, setEmail] = useState('');
  const [ended, setEnded] = useState<number | null>(null);
  const end = useMutation({
    mutationFn: (body: { email: string } | { all: true }) =>
      api.post<{ ended: number }>('/admin/sessions/end', body),
    onSuccess: (out) => setEnded(out.ended),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (email.trim()) end.mutate({ email: email.trim() });
  };
  return (
    <>
      <p>{t('running.sessionsIntro')}</p>
      <form onSubmit={submit} className="settings-form">
        <Field label={t('running.oneEmail')}>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <div>
          <button type="submit" disabled={end.isPending || !email.trim()}>
            {t('running.endOne')}
          </button>
        </div>
      </form>
      <h2>{t('running.everyoneTitle')}</h2>
      <p>{t('running.endAllWarn')}</p>
      <ConfirmButton
        label={t('running.endAll')}
        message={t('running.endAllConfirm')}
        tone="danger"
        onConfirm={() => end.mutate({ all: true })}
      />
      <ErrorNote error={end.error} />
      {ended !== null && <p role="status">{t('running.ended', { count: ended })}</p>}
    </>
  );
}
