// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';

import { matchesSpaceFilter, NO_SPACE_FILTER } from './admin-space-filter';

const now = new Date('2026-11-07T12:00:00Z');
const row = {
  name: 'Engineering',
  key: 'ENG',
  pages: 40,
  bytes: 5 << 20,
  lastActivity: '2026-10-01T00:00:00Z',
};

describe('filtering the spaces of the administration', () => {
  it('keeps every space without a filter', () => {
    expect(matchesSpaceFilter(row, NO_SPACE_FILTER, now)).toBe(true);
  });

  it('drops a space with fewer pages or less storage than asked', () => {
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, minPages: 41 }, now)).toBe(false);
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, minPages: 40 }, now)).toBe(true);
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, minMiB: 6 }, now)).toBe(false);
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, minMiB: 5 }, now)).toBe(true);
  });

  it('finds a space by its name or key, whatever the case', () => {
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, text: 'eng' }, now)).toBe(true);
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, text: 'sales' }, now)).toBe(false);
  });

  it('keeps only spaces quiet for longer than the months asked, and a space that never changed', () => {
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, quietMonths: 1 }, now)).toBe(true);
    expect(matchesSpaceFilter(row, { ...NO_SPACE_FILTER, quietMonths: 3 }, now)).toBe(false);
    expect(
      matchesSpaceFilter({ ...row, lastActivity: null }, { ...NO_SPACE_FILTER, quietMonths: 12 }, now),
    ).toBe(true);
  });
});
