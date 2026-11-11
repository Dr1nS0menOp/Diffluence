// SPDX-License-Identifier: Apache-2.0
// The health checks of the console (spec 051): the list on System health, the band about
// the disk on every page of the console, and the lines on the summary.
import type { TFunction } from 'i18next';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api';
import { Link } from '../router';
import { ErrorNote, formatSize, Loading } from '../components/ui';
import { TableBox } from '../components/table-box';

type HealthState = 'ok' | 'warn' | 'problem';

export interface HealthCheck {
  name: string;
  state: HealthState;
  code: string;
  values: Record<string, string | number | boolean | undefined>;
}

/** The server keeps its answer for 30 seconds, so asking more often gains nothing. */
const HEALTH_POLL_MS = 30_000;
/** The codes of the disk check that the band on every page reports. */
const DISK_TROUBLE = ['disk_low', 'disk_full', 'disk_fills'];

/** Reads the health checks; every page of the console shares one answer. */
export function useHealth() {
  return useQuery({
    queryKey: ['admin-health'],
    queryFn: () => api.get<{ checks: HealthCheck[] }>('/admin/health'),
    refetchInterval: HEALTH_POLL_MS,
  });
}

function size(value: string | number | boolean | undefined): string {
  return typeof value === 'number' ? formatSize(value) : '';
}

/** Says a check in words, with its sizes written for people. */
export function healthText(t: TFunction, check: HealthCheck): string {
  const v = check.values;
  const params = {
    ...v,
    free: size(v.free),
    total: size(v.total),
    bytes: size(v.bytes),
    growth: size(v.growth),
    count: typeof v.count === 'number' ? v.count : v.outbox,
  };
  const key = check.code === 'database_size' && v.growth !== undefined ? 'database_growth' : check.code;
  return t(`health.${key}`, params);
}

/** Every check, with what it says, first on System health. */
export function HealthChecks() {
  const { t } = useTranslation();
  const q = useHealth();
  if (q.error) return <ErrorNote error={q.error} />;
  if (!q.data) return <Loading />;
  return (
    <>
      <h2>{t('health.checks')}</h2>
      <TableBox>
        <table>
          <thead>
            <tr>
              <th>{t('health.check')}</th>
              <th>{t('health.state')}</th>
              <th>{t('health.what')}</th>
            </tr>
          </thead>
          <tbody>
            {q.data.checks.map((check) => (
              <tr key={check.name} data-state={check.state}>
                <th scope="row">{t(`health.name_${check.name}`)}</th>
                <td>{t(`health.state_${check.state}`)}</td>
                <td>{healthText(t, check)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableBox>
    </>
  );
}

/** The band on every page of the console while the disk for files runs out (OPS-104). */
export function HealthBand() {
  const { t } = useTranslation();
  const q = useHealth();
  const disk = q.data?.checks.find((check) => check.name === 'disk');
  if (!disk || !DISK_TROUBLE.includes(disk.code)) return null;
  return (
    <p role="status" className={disk.state === 'problem' ? 'note note-error' : 'note'}>
      {t('health.band')} <Link href="/admin/system">{t('health.bandLook')}</Link>
    </p>
  );
}

/**
 * The checks the summary lists. The disk has the band and a line of its own, and a missing
 * mail server has its own line too, so neither is said twice.
 */
export function summaryChecks(checks: HealthCheck[] | undefined): HealthCheck[] {
  return (checks ?? []).filter(
    (check) => check.state !== 'ok' && check.name !== 'disk' && check.code !== 'mail_off',
  );
}

/** The checks that are not fine, as lines of the summary's list of what needs attention. */
export function HealthAttention({ checks }: { checks: HealthCheck[] }) {
  const { t } = useTranslation();
  return (
    <>
      {checks.map((check) => (
        <li key={`health-${check.name}`} className="note note-error">
          <span className="grow">
            {t(check.state === 'problem' ? 'summary.healthNow' : 'summary.healthLook', {
              name: t(`health.name_${check.name}`),
            })}
            {' · '}
            {healthText(t, check)}
          </span>
          <Link href="/admin/system">{t('summary.look')}</Link>
        </li>
      ))}
    </>
  );
}
