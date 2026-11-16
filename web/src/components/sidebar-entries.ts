// SPDX-License-Identifier: Apache-2.0
// The entries of a person's own sidebar and their order (spec 078, NAV-125, NAV-137).
import type { ListedPage } from '../api';

/** Every entry a person may order and hide. The start screen is always first, so it is not one. */
export const SIDEBAR_ENTRIES = [
  'recent',
  'favourites',
  'spaces',
  'drafts',
  'tasks',
  'people',
  'watching',
] as const;
export type SidebarEntry = (typeof SIDEBAR_ENTRIES)[number];

/** Entries that start under More until the person ticks them (NAV-142). */
const HIDDEN_AT_FIRST: readonly SidebarEntry[] = ['watching'];

const isEntry = (name: string): name is SidebarEntry => (SIDEBAR_ENTRIES as readonly string[]).includes(name);

/** Every entry once, in the person's order, with the entries it does not name at the end. */
export function orderedEntries(order: readonly string[] | null): SidebarEntry[] {
  const named = [...new Set((order ?? []).filter(isEntry))];
  return [...named, ...SIDEBAR_ENTRIES.filter((e) => !named.includes(e))];
}

/**
 * The entries in the person's order, split into shown and hidden. An entry a stored order does
 * not name, such as one a later release added, goes at the end, shown unless it starts hidden.
 */
export function arrangeEntries(
  order: readonly string[] | null,
  hidden: readonly string[] | null,
): { shown: SidebarEntry[]; hidden: SidebarEntry[] } {
  const named = [...new Set((order ?? []).filter(isEntry))];
  const all = orderedEntries(order);
  const isHidden = (entry: SidebarEntry) =>
    hidden === null
      ? HIDDEN_AT_FIRST.includes(entry)
      : hidden.includes(entry) || (HIDDEN_AT_FIRST.includes(entry) && !named.includes(entry));
  return { shown: all.filter((e) => !isHidden(e)), hidden: all.filter(isHidden) };
}

/** The rows of the dialog turned back into what the preferences store. */
export function chosenEntries(rows: readonly { id: SidebarEntry; shown: boolean }[]): {
  order: SidebarEntry[];
  hidden: SidebarEntry[];
} {
  return { order: rows.map((r) => r.id), hidden: rows.filter((r) => !r.shown).map((r) => r.id) };
}

export interface TouchedPage {
  page: ListedPage;
  did: 'edited' | 'visited';
}

/** What a person last worked on or visited, each page once with what they did last, newest first. */
export function lastTouched(
  visited: readonly ListedPage[] | undefined,
  worked: readonly ListedPage[] | undefined,
  limit: number,
): TouchedPage[] {
  // Times differ in their fractions of a second, so they are compared as moments, not as text.
  const moment = (touched: TouchedPage) => Date.parse(touched.page.at);
  const newest = new Map<string, TouchedPage>();
  const add = (page: ListedPage, did: TouchedPage['did']) => {
    const known = newest.get(page.id);
    const next = { page, did };
    if (!known || moment(known) < moment(next)) newest.set(page.id, next);
  };
  (visited ?? []).forEach((p) => add(p, 'visited'));
  (worked ?? []).forEach((p) => add(p, 'edited'));
  return [...newest.values()].sort((a, b) => moment(b) - moment(a)).slice(0, limit);
}
