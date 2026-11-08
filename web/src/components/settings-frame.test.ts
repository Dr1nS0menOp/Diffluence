// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { matchingItems, type SettingsGroup } from './settings-frame';

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

  it('leaves nothing when nothing matches', () => {
    expect(matchingItems(groups, 'nothing')).toEqual([]);
  });
});
