// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { chartData, parseCell } from './chart';

describe('parseCell', () => {
  it('reads numbers the way people write them', () => {
    expect(parseCell('1.5')).toBe(1.5);
    expect(parseCell('1,5')).toBe(1.5);
    expect(parseCell('1 000')).toBe(1000);
  });

  it('gives nothing for text and empty cells', () => {
    expect(parseCell('')).toBeNull();
    expect(parseCell('soon')).toBeNull();
    expect(parseCell('Infinity')).toBeNull();
  });
});

describe('chartData', () => {
  const rows = [
    ['Quarter', 'Pages', 'Notes'],
    ['Q1', '12', 'x'],
    ['Q2', '18', '4'],
  ];

  it('uses the first row for names and the axis column for labels', () => {
    const data = chartData(rows, 0, null);
    expect(data.labels).toEqual(['Q1', 'Q2']);
    expect(data.series.map((s) => s.name)).toEqual(['Pages', 'Notes']);
  });

  it('counts the cells it left out', () => {
    const data = chartData(rows, 0, null);
    expect(data.series[1]?.values).toEqual([null, 4]);
    expect(data.skipped).toBe(1);
  });

  it('keeps to the chosen columns', () => {
    expect(chartData(rows, 0, [1]).series).toHaveLength(1);
  });

  it('survives an empty table', () => {
    expect(chartData([], 0, null)).toEqual({ labels: [], series: [], skipped: 0 });
  });
});
