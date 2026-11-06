// SPDX-License-Identifier: Apache-2.0
// Our own charts, four kinds (spec 012, D7). A chart reads a table in the document,
// so it follows every edit. Series differ by shape as well as colour (WCAG 1.4.1),
// and a real table always sits beside the picture.
import { useState } from 'react';
import { useTranslation } from 'react-i18next';

const WIDTH = 640;
const HEIGHT = 280;
const MARGIN = 36;
const MAX_ROWS = 500;
const SERIES_CLASSES = 6;
const DASHES = ['', '6 3', '2 3', '8 3 2 3', '1 4', '10 4'];
const FULL_CIRCLE = Math.PI * 2;

export interface ChartData {
  labels: string[];
  series: { name: string; values: (number | null)[] }[];
  skipped: number;
}

/** Reads a number the way people write it: 1.5, 1,5 or 1 000. */
export function parseCell(text: string): number | null {
  const clean = text.trim().replace(/\s/g, '').replace(',', '.');
  if (clean === '') return null;
  const value = Number(clean);
  return Number.isFinite(value) ? value : null;
}

/** Turns table rows into a chart: the first row names the series. */
export function chartData(rows: string[][], axis: number, columns: number[] | null): ChartData {
  const [head = [], ...body] = rows.slice(0, MAX_ROWS);
  const wanted = columns ?? head.map((_, i) => i).filter((i) => i !== axis);
  let skipped = 0;
  const series = wanted.map((column) => ({
    name: head[column] ?? '',
    values: body.map((row) => {
      const value = parseCell(row[column] ?? '');
      if (value === null) skipped += 1;
      return value;
    }),
  }));
  return { labels: body.map((row) => row[axis] ?? ''), series, skipped };
}

function scale(data: ChartData): (value: number) => number {
  const values = data.series.flatMap((s) => s.values.filter((v): v is number => v !== null));
  const top = Math.max(0, ...values);
  const bottom = Math.min(0, ...values);
  const range = top - bottom || 1;
  return (value) => HEIGHT - MARGIN - ((value - bottom) / range) * (HEIGHT - 2 * MARGIN);
}

function Bars({ data }: { data: ChartData }) {
  const y = scale(data);
  const slot = (WIDTH - 2 * MARGIN) / Math.max(1, data.labels.length);
  const bar = slot / (data.series.length + 1);
  return data.series.map((series, s) =>
    series.values.map((value, i) =>
      value === null ? null : (
        <rect
          key={`${series.name}-${i}`}
          className={`chart-series-${s % SERIES_CLASSES}`}
          x={MARGIN + i * slot + s * bar + bar / 2}
          y={Math.min(y(value), y(0))}
          width={bar}
          height={Math.abs(y(0) - y(value))}
        />
      ),
    ),
  );
}

function Lines({ data, filled }: { data: ChartData; filled: boolean }) {
  const y = scale(data);
  const step = (WIDTH - 2 * MARGIN) / Math.max(1, data.labels.length - 1);
  return data.series.map((series, s) => {
    const points = series.values.flatMap((value, i) =>
      value === null ? [] : [`${MARGIN + i * step},${y(value)}`],
    );
    return (
      <polyline
        key={series.name}
        className={`chart-line chart-series-${s % SERIES_CLASSES} ${filled ? 'filled' : ''}`}
        points={points.join(' ')}
        strokeDasharray={DASHES[s % DASHES.length]}
      />
    );
  });
}

function Pie({ data }: { data: ChartData }) {
  const values = (data.series[0]?.values ?? []).map((v) => Math.max(0, v ?? 0));
  const total = values.reduce((sum, v) => sum + v, 0) || 1;
  const radius = HEIGHT / 2 - MARGIN / 2;
  const centre = { x: WIDTH / 2, y: HEIGHT / 2 };
  let angle = 0;
  return values.map((value, i) => {
    const start = angle;
    angle += (value / total) * FULL_CIRCLE;
    // An arc that ends where it starts draws nothing, so a whole pie is a circle.
    if (value === total) {
      return (
        <circle
          key={data.labels[i] ?? i}
          className={`chart-series-${i % SERIES_CLASSES} chart-slice`}
          cx={centre.x}
          cy={centre.y}
          r={radius}
        />
      );
    }
    const large = angle - start > Math.PI ? 1 : 0;
    const from = `${centre.x + radius * Math.sin(start)} ${centre.y - radius * Math.cos(start)}`;
    const to = `${centre.x + radius * Math.sin(angle)} ${centre.y - radius * Math.cos(angle)}`;
    return (
      <path
        key={data.labels[i] ?? i}
        className={`chart-series-${i % SERIES_CLASSES} chart-slice`}
        d={`M ${centre.x} ${centre.y} L ${from} A ${radius} ${radius} 0 ${large} 1 ${to} Z`}
      />
    );
  });
}

export function Chart({ data, kind, title }: { data: ChartData; kind: string; title: string }) {
  const { t } = useTranslation();
  const [showData, setShowData] = useState(false);
  const name = title || t('chart.untitled');
  return (
    <figure className="chart" role="group" aria-label={name}>
      {title && <figcaption>{title}</figcaption>}
      <div className="block-frame">
        <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label={t('chart.picture', { title: name })}>
          {kind === 'pie' ? (
            <Pie data={data} />
          ) : kind === 'line' || kind === 'area' ? (
            <Lines data={data} filled={kind === 'area'} />
          ) : (
            <Bars data={data} />
          )}
        </svg>
      </div>
      <ul className="chart-legend">
        {data.series.map((series, s) => (
          <li key={series.name}>
            <span className={`chart-key chart-series-${s % SERIES_CLASSES}`} aria-hidden="true" />
            {series.name}
          </li>
        ))}
      </ul>
      {data.skipped > 0 && <p className="muted">{t('chart.skipped', { count: data.skipped })}</p>}
      <button type="button" className="ghost" aria-expanded={showData} onClick={() => setShowData(!showData)}>
        {t('chart.showData')}
      </button>
      {showData && (
        <table>
          <thead>
            <tr>
              <th>{t('chart.label')}</th>
              {data.series.map((series) => (
                <th key={series.name}>{series.name}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.labels.map((label, i) => (
              <tr key={`${label}-${i}`}>
                <td>{label}</td>
                {data.series.map((series) => (
                  <td key={series.name}>{series.values[i] ?? ''}</td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </figure>
  );
}
