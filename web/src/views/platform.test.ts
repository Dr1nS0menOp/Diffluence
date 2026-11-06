// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { sortRows } from '../components/sort-table';
import { periodQuery, trendKey } from './insights';
import { moveStep } from './rules';

describe('sorting a table', () => {
  it('sorts numbers by size, and text by the language', () => {
    const rows = [
      { n: 10, s: 'b' },
      { n: 9, s: 'a' },
    ];
    expect(sortRows(rows, (r) => r.n, true).map((r) => r.n)).toEqual([9, 10]);
    expect(sortRows(rows, (r) => r.s, false).map((r) => r.s)).toEqual(['b', 'a']);
  });
});

describe('a period', () => {
  const now = new Date('2026-03-10T12:00:00Z');
  it('counts today', () => {
    expect(periodQuery('7', now)).toBe('from=2026-03-04&to=2026-03-10');
  });
  it('starts a year in January', () => {
    expect(periodQuery('year', now)).toBe('from=2026-01-01&to=2026-03-10');
  });
});

describe('the words beside a chart', () => {
  it('compares the two halves', () => {
    expect(trendKey([1, 1, 5, 5])).toBe('up');
    expect(trendKey([5, 5, 1, 1])).toBe('down');
    expect(trendKey([2, 2])).toBe('flat');
  });
});

describe('moving a step', () => {
  it('keeps the list inside its ends', () => {
    expect(moveStep(['a', 'b'], 0, 1)).toEqual(['b', 'a']);
    expect(moveStep(['a', 'b'], 0, -1)).toEqual(['a', 'b']);
  });
});
