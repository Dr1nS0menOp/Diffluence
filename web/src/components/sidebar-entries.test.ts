// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { ListedPage } from '../api';
import {
  arrangeEntries,
  chosenEntries,
  lastTouched,
  orderedEntries,
  SIDEBAR_ENTRIES,
} from './sidebar-entries';

describe('the entries of a person sidebar', () => {
  it('shows every entry but Watching in the default order when nothing was chosen', () => {
    expect(arrangeEntries(null, null)).toEqual({
      shown: ['recent', 'favourites', 'spaces', 'drafts', 'tasks', 'people'],
      hidden: ['watching'],
    });
  });

  it('leaves out names it does not know and names given twice', () => {
    const { shown, hidden } = arrangeEntries(['tasks', 'secret', 'tasks', 'recent'], []);
    expect(shown.slice(0, 2)).toEqual(['tasks', 'recent']);
    expect([...shown, ...hidden].sort()).toEqual([...SIDEBAR_ENTRIES].sort());
  });

  it('puts an entry missing from a stored order at the end, shown', () => {
    const { shown, hidden } = arrangeEntries(['tasks', 'recent', 'favourites', 'spaces', 'drafts'], []);
    expect(shown).toEqual(['tasks', 'recent', 'favourites', 'spaces', 'drafts', 'people']);
    expect(hidden).toEqual(['watching']);
  });

  it('keeps Watching shown once the person ticked it', () => {
    const order = [...SIDEBAR_ENTRIES];
    expect(arrangeEntries(order, []).hidden).toEqual([]);
  });

  it('keeps the order of the hidden entries the same as the order', () => {
    const order = ['people', 'tasks', 'recent', 'favourites', 'spaces', 'drafts', 'watching'];
    expect(arrangeEntries(order, ['tasks', 'people']).hidden).toEqual(['people', 'tasks']);
  });

  it('lists every entry once for the dialog, hidden ones included', () => {
    expect(orderedEntries(null)).toEqual([...SIDEBAR_ENTRIES]);
    expect(orderedEntries(['watching', 'secret'])[0]).toBe('watching');
    expect(orderedEntries(['watching'])).toHaveLength(SIDEBAR_ENTRIES.length);
  });

  it('turns a choice back into an order and a hidden list', () => {
    const rows = [
      { id: 'tasks' as const, shown: true },
      { id: 'watching' as const, shown: false },
      { id: 'recent' as const, shown: true },
    ];
    expect(chosenEntries(rows)).toEqual({ order: ['tasks', 'watching', 'recent'], hidden: ['watching'] });
  });
});

const listed = (id: string, at: string, unpublished = false): ListedPage => ({
  id,
  title: id,
  emoji: '',
  space_key: 'S',
  space_name: 'Space',
  at,
  unpublished,
});

describe('what a person last worked on or visited', () => {
  it('is empty when nothing came back', () => {
    expect(lastTouched(undefined, undefined, 6)).toEqual([]);
  });

  it('keeps each page once, with what the person did last', () => {
    const visited = [listed('a', '2026-11-15T10:00:00Z'), listed('b', '2026-11-15T08:00:00Z')];
    const worked = [listed('a', '2026-11-15T09:00:00Z'), listed('b', '2026-11-15T11:00:00Z')];
    expect(lastTouched(visited, worked, 6).map((c) => [c.page.id, c.did])).toEqual([
      ['b', 'edited'],
      ['a', 'visited'],
    ]);
  });

  it('shows the newest first and no more than asked', () => {
    const visited = ['1', '2', '3', '4', '5', '6', '7'].map((n) => listed(n, `2026-11-0${n}T00:00:00Z`));
    expect(lastTouched(visited, [], 6).map((c) => c.page.id)).toEqual(['7', '6', '5', '4', '3', '2']);
  });
});
