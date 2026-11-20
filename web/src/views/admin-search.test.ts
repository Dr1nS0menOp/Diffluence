// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { NOTHING_ON } from '../capabilities';
import { findSettings, jumpIndex, SETTINGS_INDEX } from './admin-search';
import { ADMIN_SECTIONS, sectionNameKey, sectionsShown } from './admin-sections';

const WORDS: Record<string, string> = {
  [sectionNameKey('look')]: 'Look',
  [sectionNameKey('login')]: 'Login and second factor',
  [sectionNameKey('invitations')]: 'Invitations',
  [sectionNameKey('import-space')]: 'Import a space',
  'admin.invitations': 'Invitations',
  'admin.font': 'Font',
  'rules.idleMinutes': 'End an idle session after this many minutes',
  'exchange.bundles': 'Bundles between instances',
};
const word = (key: string) => WORDS[key] ?? key;

describe('the jump field of the administration', () => {
  it('finds a section by its name, from the registry (D6)', () => {
    const found = findSettings(jumpIndex(ADMIN_SECTIONS), 'second factor', word);
    expect(found).toContainEqual({ label: sectionNameKey('login'), tab: 'login' });
  });

  it('finds a setting inside a section, at the section that holds it now', () => {
    expect(findSettings(jumpIndex(ADMIN_SECTIONS), 'font', word)).toEqual([
      { label: 'admin.font', tab: 'look' },
    ]);
    expect(findSettings(jumpIndex(ADMIN_SECTIONS), 'idle', word)).toEqual([
      { label: 'rules.idleMinutes', tab: 'login' },
    ]);
  });

  it('lists a name and its place once, though both the registry and the index hold it', () => {
    const found = findSettings(jumpIndex(ADMIN_SECTIONS), 'invitations', word);
    expect(found.filter((entry) => entry.tab === 'invitations')).toHaveLength(1);
  });

  it('finds nothing of a section that is not shown, such as an experimental part that is off', () => {
    const shown = sectionsShown(NOTHING_ON);
    expect(findSettings(jumpIndex(shown), 'import a space', word)).toEqual([]);
    expect(findSettings(jumpIndex(shown), 'bundles', word)).toEqual([]);
  });

  it('finds nothing for an empty text', () => {
    expect(findSettings(jumpIndex(ADMIN_SECTIONS), '   ', word)).toEqual([]);
  });

  it('keeps every entry of the index that belongs to a shown section', () => {
    expect(jumpIndex(ADMIN_SECTIONS)).toHaveLength(ADMIN_SECTIONS.length + SETTINGS_INDEX.length);
  });
});
