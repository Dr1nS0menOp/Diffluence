// SPDX-License-Identifier: Apache-2.0

/** What the administration can filter the list of spaces by (ADM-033). */
export interface SpaceFilter {
  text: string;
  minPages: number;
  minMiB: number;
  /** Months without any change, or 0 for any. */
  quietMonths: number;
}

export const NO_SPACE_FILTER: SpaceFilter = { text: '', minPages: 0, minMiB: 0, quietMonths: 0 };

export const QUIET_MONTHS = [0, 1, 3, 6, 12] as const;

interface Row {
  name: string;
  key: string;
  pages: number;
  bytes: number;
  lastActivity: string | null;
}

const MEBIBYTE = 1 << 20;

/** Keeps the rows that pass every part of the filter. A space with no activity at all counts as quiet. */
export function matchesSpaceFilter(row: Row, filter: SpaceFilter, now: Date): boolean {
  const text = filter.text.trim().toLowerCase();
  if (text && !row.name.toLowerCase().includes(text) && !row.key.toLowerCase().includes(text)) return false;
  if (row.pages < filter.minPages) return false;
  if (row.bytes < filter.minMiB * MEBIBYTE) return false;
  if (filter.quietMonths === 0) return true;
  if (!row.lastActivity) return true;
  const limit = new Date(now);
  limit.setMonth(limit.getMonth() - filter.quietMonths);
  return new Date(row.lastActivity) < limit;
}
