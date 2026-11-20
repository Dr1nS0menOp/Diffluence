// SPDX-License-Identifier: Apache-2.0
// The console shows what runs the installation (spec 075, wave CS1): the upgrade page, the
// events, the signatures of the audit log, the monitoring files, the log level for a while and
// the disk thresholds. It shows; the only changes are the log level and the thresholds.
import { useEffect, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type Me } from '../api';
import { Dialog, ErrorNote, Field, Loading } from '../components/ui';
import { Menu, MenuDownload } from '../components/menu';
import { TableBox } from '../components/table-box';
import { Notice } from '../components/notice';
import { SaveReset } from '../components/save-reset';
import { useDates } from '../dates';
import { ConfirmFields, needsConfirmation, type Confirmation } from '../components/confirm-change';

/** Pages that show what runs read it again as the backup page does. */
const INSTALLATION_POLL_MS = 15_000;

/** The ways to upgrade, as commands: the console cannot upgrade (CSL-1001). */
const UPGRADE_COMMANDS = {
  compose: 'git pull && docker compose up -d --build',
  helm: 'helm upgrade wiki deploy/helm/diffluence --reuse-values',
  package:
    'sudo apt install ./diffluence_<release>_amd64.deb   # or: sudo dnf install ./diffluence_<release>_amd64.rpm',
} as const;

interface NodeRow {
  id: string;
  role: string;
  version: string;
  max_migration: number;
  started_at: string;
  last_seen_at: string;
  live: boolean;
  log_level: string;
}

interface Upgrades {
  release: string;
  build_date: string;
  postgres: string;
  nodes: NodeRow[];
  newest: number;
  known: number;
  expand_waiting: number;
  contract: { version: number; name: string; nodes_behind: string[]; grace_ends: string | null }[];
  grace: { name: string; value: string; source: string };
  marker: {
    at: string;
    version: string | null;
    from_migration: number | null;
    to_migration: number | null;
    backup_noted_at: string | null;
  } | null;
}

function NodesTable({ nodes }: { nodes: NodeRow[] }) {
  const { t } = useTranslation();
  const dates = useDates();
  if (nodes.length === 0) return <p className="muted">{t('installation.nodesNone')}</p>;
  return (
    <TableBox>
      <table>
        <thead>
          <tr>
            <th>{t('installation.colRole')}</th>
            <th>{t('installation.colVersion')}</th>
            <th>{t('installation.colMigration')}</th>
            <th>{t('installation.colLogLevel')}</th>
            <th>{t('installation.colLastSeen')}</th>
            <th>{t('installation.colState')}</th>
          </tr>
        </thead>
        <tbody>
          {nodes.map((n) => (
            <tr key={n.id}>
              <td>{n.role}</td>
              <td>{n.version}</td>
              <td>{n.max_migration}</td>
              <td>{n.log_level ? t(`installation.level_${n.log_level}`) : ''}</td>
              <td>
                <time dateTime={n.last_seen_at}>{dates.moment(n.last_seen_at)}</time>
              </td>
              <td>
                <span className="lozenge">{n.live ? t('installation.live') : t('installation.gone')}</span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableBox>
  );
}

function ContractMigrations({ data }: { data: Upgrades }) {
  const { t } = useTranslation();
  const dates = useDates();
  return (
    <>
      <h2>{t('installation.contractTitle')}</h2>
      {data.contract.length === 0 ? (
        <p className="muted">{t('installation.contractNone')}</p>
      ) : (
        <TableBox>
          <table>
            <thead>
              <tr>
                <th>{t('installation.colName')}</th>
                <th>{t('installation.colWaitsFor')}</th>
                <th>{t('installation.colGraceEnds')}</th>
              </tr>
            </thead>
            <tbody>
              {data.contract.map((c) => (
                <tr key={c.version}>
                  <td>
                    <code>{`${String(c.version).padStart(4, '0')}_${c.name}`}</code>
                  </td>
                  <td>
                    {c.nodes_behind.length > 0
                      ? t('installation.waitsForNodes', { count: c.nodes_behind.length })
                      : t('installation.waitsForNothing')}
                  </td>
                  <td>
                    {c.grace_ends ? (
                      <time dateTime={c.grace_ends}>{dates.moment(c.grace_ends)}</time>
                    ) : (
                      t('installation.noLiveNode')
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </TableBox>
      )}
      <p className="muted">
        {t('installation.grace', { value: data.grace.value, source: data.grace.source })}
      </p>
    </>
  );
}

function UpgradeMarker({ data }: { data: Upgrades }) {
  const { t } = useTranslation();
  const dates = useDates();
  const m = data.marker;
  return (
    <>
      <h2>{t('installation.markerTitle')}</h2>
      {!m ? (
        <p className="muted">{t('installation.markerNone')}</p>
      ) : (
        <p>
          {t('installation.markerText', {
            when: dates.moment(m.at),
            version: m.version ?? '',
            from: m.from_migration ?? '',
            to: m.to_migration ?? '',
          })}{' '}
          {m.backup_noted_at
            ? t('installation.markerBackup', { when: dates.moment(m.backup_noted_at) })
            : t('installation.markerNoBackup')}
        </p>
      )}
    </>
  );
}

function HowToUpgrade() {
  const { t } = useTranslation();
  return (
    <>
      <h2>{t('installation.howTitle')}</h2>
      <p>{t('installation.howIntro')}</p>
      <p>{t('installation.howCompose')}</p>
      <pre>
        <code>{UPGRADE_COMMANDS.compose}</code>
      </pre>
      <p>{t('installation.howHelm')}</p>
      <pre>
        <code>{UPGRADE_COMMANDS.helm}</code>
      </pre>
      <p>{t('installation.howPackage')}</p>
      <pre>
        <code>{UPGRADE_COMMANDS.package}</code>
      </pre>
      <p className="muted">{t('installation.howGuide')}</p>
    </>
  );
}

/** System, Upgrades (CSL-560 to CSL-562, CSL-817): read-only. */
export function UpgradesTab() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['upgrades'],
    queryFn: () => api.poll<Upgrades>('/admin/upgrades'),
    refetchInterval: INSTALLATION_POLL_MS,
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const d = q.data;
  const newestNode = d.nodes.reduce<NodeRow | undefined>(
    (best, n) => (!best || n.max_migration > best.max_migration ? n : best),
    undefined,
  );
  const tiles: [string, string | number][] = [
    [t('installation.tileRelease'), d.release],
    [t('installation.tileNewestNode'), newestNode?.version ?? ''],
    [t('installation.tileExpand'), d.expand_waiting],
    [t('installation.tileContract'), d.contract.length],
  ];
  return (
    <>
      <p>{t('installation.upgradesIntro')}</p>
      <dl className="tiles">
        {tiles.map(([name, value]) => (
          <div key={name}>
            <dt>{name}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
      <p className="muted">
        {d.build_date && t('installation.builtOn', { release: d.release, date: d.build_date })}{' '}
        {t('installation.postgres', { version: d.postgres, newest: d.newest, known: d.known })}
      </p>
      {d.expand_waiting > 0 && (
        <Notice kind="info">{t('installation.expandWaiting', { count: d.expand_waiting })}</Notice>
      )}
      <h2>{t('installation.nodesTitle')}</h2>
      <NodesTable nodes={d.nodes} />
      <ContractMigrations data={d} />
      <UpgradeMarker data={d} />
      <HowToUpgrade />
    </>
  );
}

interface EventsState {
  transport: 'nats' | 'postgres';
  changed_at: string;
  drained_at: string | null;
  older_nodes: { role: string; version: string; max_migration: number }[];
  problem: string;
}

/** System, Events (CSL-590): what diffluence events status says, read-only. */
export function EventsTab() {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['events-state'],
    queryFn: () => api.poll<EventsState>('/admin/events'),
    refetchInterval: INSTALLATION_POLL_MS,
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const s = q.data;
  return (
    <>
      <p>{t('installation.eventsIntro')}</p>
      <p role="status">
        {t(`installation.transport_${s.transport}`, { when: dates.moment(s.changed_at) })}{' '}
        {s.transport === 'nats'
          ? t('installation.natsInUse')
          : s.drained_at
            ? t('installation.natsDrained', { when: dates.moment(s.drained_at) })
            : t('installation.natsNotDrained')}
      </p>
      <h2>{t('installation.olderTitle')}</h2>
      {s.older_nodes.length === 0 ? (
        <p className="muted">{t('installation.olderNone')}</p>
      ) : (
        <ul>
          {s.older_nodes.map((n) => (
            <li key={`${n.role}-${n.version}-${n.max_migration}`}>
              {t('installation.olderNode', { version: n.version, role: n.role, migration: n.max_migration })}
            </li>
          ))}
        </ul>
      )}
      {s.problem ? (
        <Notice kind="warning" title={t('installation.withoutNatsNot')}>
          <p>{s.problem}</p>
        </Notice>
      ) : (
        <Notice kind="info">{t('installation.withoutNatsOk')}</Notice>
      )}
    </>
  );
}

interface AuditArea {
  checkpoint: { seq: number; at: string } | null;
  keys: { from_seq: number; public_key_start: string; installation: boolean; endorsed: boolean }[];
  witness: {
    checked_at: string;
    ok: boolean;
    compared: number;
    first_bad_seq?: number;
    reason?: string;
  } | null;
  witness_warn_after_days: number;
}

const DAY_MS = 24 * 60 * 60 * 1000;

function WitnessLine({ area }: { area: AuditArea }) {
  const { t } = useTranslation();
  const dates = useDates();
  const w = area.witness;
  if (!w) return <p className="muted">{t('installation.witnessNever')}</p>;
  const days = Math.floor((Date.now() - new Date(w.checked_at).getTime()) / DAY_MS);
  return (
    <>
      {w.ok ? (
        <p>{t('installation.witnessOk', { when: dates.moment(w.checked_at), compared: w.compared })}</p>
      ) : (
        <Notice kind="error">
          {t('installation.witnessFailed', {
            when: dates.moment(w.checked_at),
            seq: w.first_bad_seq ?? 0,
            reason: w.reason ?? '',
          })}
        </Notice>
      )}
      {days > area.witness_warn_after_days && (
        <Notice kind="warning" title={t('installation.witnessOldTitle')}>
          {t('installation.witnessOld', { days, limit: area.witness_warn_after_days })}
        </Notice>
      )}
    </>
  );
}

/** Under Audit, beside the check: the newest checkpoint, the keys and the witness (CSL-820). */
export function AuditSignatures() {
  const { t } = useTranslation();
  const dates = useDates();
  const q = useQuery({
    queryKey: ['audit-overview'],
    queryFn: () => api.get<AuditArea>('/admin/audit-overview'),
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const area = q.data;
  return (
    <section aria-labelledby="audit-signatures">
      <h2 id="audit-signatures">{t('installation.signaturesTitle')}</h2>
      {area.checkpoint ? (
        <p>
          {t('installation.checkpoint', { seq: area.checkpoint.seq, when: dates.moment(area.checkpoint.at) })}
        </p>
      ) : (
        <p className="muted">{t('installation.checkpointNone')}</p>
      )}
      <h3>{t('installation.keysTitle')}</h3>
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('installation.colKind')}</th>
              <th>{t('installation.colFrom')}</th>
              <th>{t('installation.colKey')}</th>
              <th>{t('installation.colEndorsed')}</th>
            </tr>
          </thead>
          <tbody>
            {area.keys.map((k) => (
              <tr key={`${k.from_seq}-${k.public_key_start}`}>
                <td>{k.installation ? t('installation.kindInstallation') : t('installation.kindLater')}</td>
                <td>{k.from_seq}</td>
                <td>
                  <code>{k.public_key_start}</code>
                </td>
                <td>
                  <span className="lozenge">
                    {k.endorsed ? t('installation.endorsed') : t('installation.notEndorsed')}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
      <h3>{t('installation.witnessTitle')}</h3>
      <WitnessLine area={area} />
    </section>
  );
}

/** On System health: the rules and the dashboard of this release, and whether /metrics is on. */
export function MonitoringFiles() {
  const { t } = useTranslation();
  const q = useQuery({
    queryKey: ['monitoring'],
    queryFn: () => api.get<{ metrics_on: boolean }>('/admin/monitoring'),
  });
  return (
    <>
      <h2>{t('installation.monitoringTitle')}</h2>
      <Menu
        label={t('installation.monitoringFiles')}
        trigger={t('installation.monitoringFiles')}
        className=""
        align="start"
      >
        <MenuDownload href="/api/v1/admin/monitoring/rules" download>
          {t('installation.rules')}
        </MenuDownload>
        <MenuDownload href="/api/v1/admin/monitoring/dashboard" download>
          {t('installation.dashboard')}
        </MenuDownload>
      </Menu>
      <ErrorNote error={q.error} />
      {q.data && (
        <p className="muted">
          {q.data.metrics_on ? t('installation.metricsOn') : t('installation.metricsOff')}
        </p>
      )}
    </>
  );
}

interface LogLevelArea {
  configured: string;
  override: { level: string; node: string; until: string } | null;
  nodes: NodeRow[];
  levels: string[];
  hours: number[];
}

/** Whether level is quieter than the one the node was started with (spec 075, D26). */
function quieter(levels: string[], level: string, configured: string) {
  return levels.indexOf(level) > levels.indexOf(configured);
}

function LogLevelDialog({
  area,
  isSystem,
  onClose,
}: {
  area: LogLevelArea;
  isSystem: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [level, setLevel] = useState(area.override?.level ?? 'debug');
  const [hours, setHours] = useState(area.hours[0] ?? 1);
  const [node, setNode] = useState(area.override?.node ?? '');
  const [confirm, setConfirm] = useState<Confirmation | null>(null);
  const done = () => {
    onClose();
    return qc.invalidateQueries({ queryKey: ['log-level'] });
  };
  const save = useMutation({
    mutationFn: () => {
      const body = { level, hours, node };
      return confirm
        ? api.put('/admin/log-level/confirmed', { ...body, code: confirm.code.trim() })
        : api.put('/admin/log-level', body);
    },
    onSuccess: done,
    onError: (error) => {
      if (isSystem && needsConfirmation(error)) setConfirm((c) => c ?? { code: '', reason: '' });
    },
  });
  const end = useMutation({ mutationFn: () => api.del('/admin/log-level'), onSuccess: done });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate();
  };
  return (
    <Dialog title={t('installation.logDialog')} onClose={onClose}>
      <form onSubmit={submit} className="settings-form">
        <p className="muted">{t('installation.logIntro')}</p>
        <Field label={t('installation.logLevelField')}>
          <select value={level} onChange={(e) => setLevel(e.target.value)}>
            {area.levels.map((l) => (
              <option key={l} value={l}>
                {t(`installation.level_${l}`)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('installation.logHours')}>
          <select value={hours} onChange={(e) => setHours(Number(e.target.value))}>
            {area.hours.map((h) => (
              <option key={h} value={h}>
                {t('installation.hours', { count: h })}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t('installation.logNodes')}>
          <select value={node} onChange={(e) => setNode(e.target.value)}>
            <option value="">{t('installation.logAllNodes')}</option>
            {area.nodes
              .filter((n) => n.live)
              .map((n) => (
                <option key={n.id} value={n.id}>
                  {t('installation.logOneNode', { role: n.role, version: n.version })}
                </option>
              ))}
          </select>
        </Field>
        {quieter(area.levels, level, area.configured) && !confirm && (
          <p className="muted">{t('installation.logQuieterNote')}</p>
        )}
        {confirm && (
          <ConfirmFields value={confirm} onChange={setConfirm} note={t('installation.logQuieterNote')} />
        )}
        <ErrorNote error={save.error ?? end.error} />
        <div className="dialog-actions">
          {area.override && (
            <button type="button" className="quiet" onClick={() => end.mutate()} disabled={end.isPending}>
              {t('installation.logEnd')}
            </button>
          )}
          <button type="submit" className="primary" disabled={save.isPending}>
            {t('installation.logSave')}
          </button>
        </div>
      </form>
    </Dialog>
  );
}

/** The foot of System health: the level the nodes log at, and Change (CSL-570, CSL-822). */
export function LogLevelLine({ me }: { me: Me }) {
  const { t } = useTranslation();
  const dates = useDates();
  const [open, setOpen] = useState(false);
  const q = useQuery({
    queryKey: ['log-level'],
    queryFn: () => api.poll<LogLevelArea>('/admin/log-level'),
    refetchInterval: INSTALLATION_POLL_MS,
  });
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  const a = q.data;
  const o = a.override;
  const level = (name: string) => t(`installation.level_${name}`);
  return (
    <>
      <h2>{t('installation.logLevelTitle')}</h2>
      <p role="status">
        {!o
          ? t('installation.logConfigured', { level: level(a.configured) })
          : t(o.node ? 'installation.logOverrideOne' : 'installation.logOverrideAll', {
              level: level(o.level),
              until: dates.moment(o.until),
            })}{' '}
        <button type="button" onClick={() => setOpen(true)}>
          {t('installation.logChange')}
        </button>
      </p>
      {open && (
        <LogLevelDialog
          area={a}
          isSystem={me.global_permissions.includes('system_administrator')}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}

interface HealthSection {
  disk_warn_percent: number;
  disk_refuse_percent: number;
}

/** A whole percentage from a field, or 0 for the default. */
function percentOrDefault(text: string): number {
  const n = Number.parseInt(text, 10);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

/** Settings, Health: the two disk thresholds within their bounds (CSL-581, CSL-823). */
export function DiskThresholds({ isSystem }: { isSystem: boolean }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ['settings-health'],
    queryFn: () => api.get<HealthSection>('/admin/settings/health'),
  });
  const [warn, setWarn] = useState('');
  const [refuse, setRefuse] = useState('');
  const [confirm, setConfirm] = useState<Confirmation | null>(null);
  const [saved, setSaved] = useState(false);
  const storedWarn = q.data?.disk_warn_percent ? String(q.data.disk_warn_percent) : '';
  const storedRefuse = q.data?.disk_refuse_percent ? String(q.data.disk_refuse_percent) : '';
  const reset = () => {
    setWarn(storedWarn);
    setRefuse(storedRefuse);
    setConfirm(null);
  };
  useEffect(() => {
    if (!q.data) return;
    setWarn(q.data.disk_warn_percent ? String(q.data.disk_warn_percent) : '');
    setRefuse(q.data.disk_refuse_percent ? String(q.data.disk_refuse_percent) : '');
  }, [q.data]);
  const save = useMutation({
    mutationFn: () => {
      const levels = {
        disk_warn_percent: percentOrDefault(warn),
        disk_refuse_percent: percentOrDefault(refuse),
      };
      return confirm
        ? api.put('/admin/settings/health/confirmed', { settings: levels, code: confirm.code.trim() })
        : api.put('/admin/settings/health', levels);
    },
    onSuccess: () => {
      setConfirm(null);
      setSaved(true);
      return qc.invalidateQueries({ queryKey: ['settings-health'] });
    },
    onError: (error) => {
      if (isSystem && needsConfirmation(error)) setConfirm((c) => c ?? { code: '', reason: '' });
    },
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    setSaved(false);
    save.mutate();
  };
  return (
    <form onSubmit={submit} className="settings-form settings-unit">
      <h2>{t('installation.healthTitle')}</h2>
      <p className="muted">{t('installation.healthIntro')}</p>
      {q.error && <ErrorNote error={q.error} />}
      <Field label={t('installation.diskWarn')} help={t('installation.diskWarnHelp')}>
        <input inputMode="numeric" value={warn} onChange={(e) => setWarn(e.target.value)} />
      </Field>
      <Field label={t('installation.diskRefuse')} help={t('installation.diskRefuseHelp')}>
        <input inputMode="numeric" value={refuse} onChange={(e) => setRefuse(e.target.value)} />
      </Field>
      <ErrorNote error={save.error} />
      {confirm && (
        <ConfirmFields value={confirm} onChange={setConfirm} note={t('installation.diskLowerNote')} />
      )}
      {saved && <Notice kind="info">{t('installation.saved')}</Notice>}
      <SaveReset
        changed={q.data !== undefined && (warn !== storedWarn || refuse !== storedRefuse)}
        busy={save.isPending}
        saveLabel={confirm ? t('rules.confirmButton') : undefined}
        onReset={reset}
      />
    </form>
  );
}
