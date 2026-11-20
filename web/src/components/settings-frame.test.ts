// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { matchingItems, toggledGroups, type SettingsGroup } from './settings-frame';

const groups: SettingsGroup[] = [
  { id: 'general', title: 'General', items: [{ id: 'details', label: 'Details', href: '?section=details' }] },
  {
    id: 'content',
    title: 'Content',
    items: [
      { id: 'labels', label: 'Labels', href: '?section=labels' },
      { id: 'statuses', label: 'Page statuses', href: '?section=statuses' },
    ],
  },
];

describe('jumping to a setting', () => {
  it('keeps every group without a text', () => {
    expect(matchingItems(groups, '  ')).toBe(groups);
  });

  it('keeps only the sections whose name holds the text, in their groups', () => {
    expect(matchingItems(groups, 'STAT')).toEqual([
      {
        id: 'content',
        title: 'Content',
        items: [{ id: 'statuses', label: 'Page statuses', href: '?section=statuses' }],
      },
    ]);
  });

  it('finds an entry by a name of a section it took over (spec 083, D11)', () => {
    const merged: SettingsGroup[] = [
      {
        id: 'pinned',
        title: '',
        items: [{ id: 'details', label: 'Details', href: '?section=details', formerly: ['Space key'] }],
      },
    ];
    expect(matchingItems(merged, 'key')[0]?.items.map((item) => item.id)).toEqual(['details']);
  });

  it('leaves nothing when nothing matches', () => {
    expect(matchingItems(groups, 'nothing')).toEqual([]);
  });
});

describe('folding a group', () => {
  it('shuts an open group and opens a shut one, leaving the others as they were', () => {
    expect(toggledGroups(['admin-data'], 'admin-security')).toEqual(['admin-data', 'admin-security']);
    expect(toggledGroups(['admin-data', 'admin-security'], 'admin-data')).toEqual(['admin-security']);
  });

  it('keeps the old ids of other settings in the list untouched (spec 081, Data)', () => {
    expect(toggledGroups(['content', 'system'], 'admin-look')).toEqual(['content', 'system', 'admin-look']);
  });

  it('folds a group of space settings under its space- id, leaving those of the administration', () => {
    expect(toggledGroups(['admin-look'], 'space-look')).toEqual(['admin-look', 'space-look']);
  });
});
