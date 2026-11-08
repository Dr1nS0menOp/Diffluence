// SPDX-License-Identifier: Apache-2.0
// The filters of the search box are words of the search syntax the server already reads
// (space:, label:, author:), so a typed query and the buttons mean the same thing, and a
// query language can grow from it later (spec 033). Type and date have no words yet: they
// travel as the parameters the results page already reads, kind and within.

export interface SearchFilters {
  /** Keys of spaces, any of which; none means every space. */
  spaces: string[];
  labels: string[];
  /** Names of people who wrote or changed a page. */
  people: string[];
  /** page, post, database or comment; empty for any. */
  kind: string;
  /** day, week, month or year; empty for any time. */
  within: string;
}

export const NO_FILTERS: SearchFilters = { spaces: [], labels: [], people: [], kind: '', within: '' };

const FIELD = /^(space|label|author):(?:"([^"]*)"|(\S*))$/i;

function word(field: string, value: string): string {
  return /[\s"]/.test(value) ? `${field}:"${value.replace(/"/g, '')}"` : `${field}:${value}`;
}

/** Splits a query into its plain words and the filter words it holds. */
export function splitQuery(query: string): {
  text: string;
  spaces: string[];
  labels: string[];
  people: string[];
} {
  const out = { text: '', spaces: [] as string[], labels: [] as string[], people: [] as string[] };
  const plain: string[] = [];
  for (const piece of query.match(/\S+:"[^"]*"|"[^"]*"|\S+/g) ?? []) {
    const m = FIELD.exec(piece);
    if (!m) {
      plain.push(piece);
      continue;
    }
    const value = (m[2] ?? m[3] ?? '').trim();
    if (!value) continue;
    const field = (m[1] ?? '').toLowerCase();
    if (field === 'space') out.spaces.push(value.toUpperCase());
    else if (field === 'label') out.labels.push(value.toLowerCase());
    else out.people.push(value);
  }
  out.text = plain.join(' ');
  return out;
}

/** The query with the filters written into it as words of the syntax. */
export function queryWith(text: string, filters: SearchFilters): string {
  const words = [
    ...filters.spaces.map((s) => word('space', s)),
    ...filters.labels.map((l) => word('label', l)),
    ...filters.people.map((p) => word('author', p)),
  ];
  return [text.trim(), ...words].filter(Boolean).join(' ');
}

/** The address of the results page for a query and its filters. */
export function resultsAddress(text: string, filters: SearchFilters): string {
  const params = new URLSearchParams({ q: queryWith(text, filters) });
  if (filters.kind) params.set('kind', filters.kind);
  if (filters.within) params.set('within', filters.within);
  return `/search?${params.toString()}`;
}

/**
 * The spaces to offer as choices: the chosen ones first, so they can always be undone, then
 * those whose name holds the typed text, up to `limit` in all.
 */
export function spaceChoices<S extends { key: string; name: string }>(
  spaces: S[],
  chosen: string[],
  text: string,
  limit: number,
): S[] {
  const chosenKeys = new Set(chosen.map((k) => k.toUpperCase()));
  const isChosen = (s: S) => chosenKeys.has(s.key.toUpperCase());
  const wanted = text.trim().toLowerCase();
  const matching = spaces.filter((s) => !isChosen(s) && s.name.toLowerCase().includes(wanted));
  return [...spaces.filter(isChosen), ...matching].slice(0, Math.max(limit, chosenKeys.size));
}

/** How many filters are on, for the buttons. */
export function filterCount(filters: SearchFilters): number {
  return (
    filters.spaces.length +
    filters.labels.length +
    filters.people.length +
    (filters.kind ? 1 : 0) +
    (filters.within ? 1 : 0)
  );
}
