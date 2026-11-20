// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { NOTHING_ON, type Capabilities } from '../capabilities';
import {
  SETTINGS_OPENERS,
  SPACE_ENTRIES,
  SPACE_PLACES,
  entriesShown,
  entryIntroKey,
  entryNameKey,
  openersOf,
  spaceEntryAt,
} from './space-sections';
import en from '../locales/en.json';

// What opened each part of space settings before wave SC5 (specs 036 and 040), frozen here so a
// move can never narrow who reaches a part. Every part not named needed space.administer.
const OLD_NEEDS: Record<string, string[]> = {
  roles: ['space.manage_access'],
  permissions: ['space.manage_access'],
  audit: ['space.audit'],
  templates: ['space.templates'],
  rules: ['space.statuses', 'space.approvals', 'space.look'],
  automation: ['space.automation'],
  webhooks: ['space.automation'],
  sites: ['site.publish'],
  statuses: ['space.statuses'],
  labels: ['space.labels'],
  workflows: ['space.workflows'],
  published: ['space.administer', 'space.release'],
  shortcuts: ['space.look'],
  people: ['space.administer', 'space.look'],
  export: ['space.administer', 'space.bundle'],
};
const OLD_IDS = [
  'details',
  'key',
  'features',
  'shortcuts',
  'categories',
  'people',
  'roles',
  'statuses',
  'labels',
  'workflows',
  'published',
  'placeholders',
  'rules',
  'import',
  'export',
  'pdf',
  'danger',
];
// Where each part of an old section went (spec 083, "Every section today"). The old Space owners
// section (`people`) held the owners and the home page, which are now in Details.
const OLD_HOME: Record<string, string[]> = {
  key: ['details'],
  categories: ['details'],
  people: ['details'],
  roles: ['people'],
  rules: ['statuses', 'layout', 'picture'],
  danger: ['archive', 'delete'],
};
// The set SC1 used in the space menu, before it was derived from the registry.
const SC1_OPENERS = [
  'space.administer',
  'space.audit',
  'space.templates',
  'space.automation',
  'site.publish',
  'space.labels',
  'space.statuses',
  'space.approvals',
  'space.workflows',
  'space.release',
  'space.bundle',
  'space.look',
];

const ALL_ON: Capabilities = { ...NOTHING_ON };
for (const key of Object.keys(ALL_ON) as (keyof Capabilities)[]) ALL_ON[key] = true;

describe('the registry of space settings', () => {
  it('gives every entry a unique id, and every alias leads to one entry only', () => {
    const ids = SPACE_ENTRIES.map((e) => e.id);
    expect(new Set(ids).size).toBe(ids.length);
    const aliases = SPACE_ENTRIES.flatMap((e) => e.aliases ?? []);
    expect(new Set(aliases).size).toBe(aliases.length);
    for (const alias of aliases) expect(ids).not.toContain(alias);
  });

  it('puts every entry in a known place, the four pinned ones in the order of SPC-138', () => {
    for (const entry of SPACE_ENTRIES) expect(SPACE_PLACES).toContain(entry.place);
    expect(SPACE_ENTRIES.filter((e) => e.place === 'pinned').map((e) => e.id)).toEqual([
      'automation',
      'people',
      'content',
      'details',
    ]);
    expect(SPACE_PLACES).toEqual([
      'pinned',
      'space-general',
      'space-access',
      'space-pages',
      'space-look',
      'space-connections',
    ]);
  });

  it('keeps the entries of each group in the order SPC-139 to SPC-144 give', () => {
    const of = (place: string) => SPACE_ENTRIES.filter((e) => e.place === place).map((e) => e.id);
    expect(of('space-general')).toEqual([
      'features',
      'archive',
      'delete',
      'export',
      'import',
      'picture',
      'statuses',
      'workflows',
      'insights',
    ]);
    expect(of('space-access')).toEqual(['permissions', 'sites', 'audit']);
    expect(of('space-pages')).toEqual([
      'placeholders',
      'files',
      'trash',
      'archived',
      'labels',
      'shortcuts',
      'published',
    ]);
    expect(of('space-look')).toEqual(['templates', 'layout', 'pdf']);
    expect(of('space-connections')).toEqual(['webhooks']);
  });

  it('opens an entry for every address of before, directly or by an alias', () => {
    for (const id of OLD_IDS) expect(spaceEntryAt(id), id).toBeDefined();
    expect(spaceEntryAt('key')?.id).toBe('details');
    expect(spaceEntryAt('categories')?.id).toBe('details');
    expect(spaceEntryAt('roles')?.id).toBe('people');
    expect(spaceEntryAt('rules')?.id).toBe('statuses');
    expect(spaceEntryAt('danger')?.id).toBe('archive');
    expect(spaceEntryAt('people')?.id).toBe('people');
    expect(spaceEntryAt('links')).toBeUndefined();
  });

  it('opens every moved part for at least everyone it opened for before', () => {
    const allOld = [...OLD_IDS, 'permissions', 'audit', 'templates', 'automation', 'webhooks', 'sites'];
    for (const old of allOld) {
      const homes = OLD_HOME[old] ?? [spaceEntryAt(old)?.id ?? old];
      const now = new Set(homes.flatMap((id) => openersOf(id, ALL_ON)));
      for (const permission of OLD_NEEDS[old] ?? ['space.administer']) {
        expect(now.has(permission), `${old} for ${permission}`).toBe(true);
      }
    }
  });

  it('opens Templates to a proposer only while guided templates are on', () => {
    expect(openersOf('templates', NOTHING_ON)).toEqual(['space.templates']);
    expect(openersOf('templates', ALL_ON)).toEqual(['space.templates', 'template.propose']);
  });

  it('leaves out an experimental entry while its part is off', () => {
    const off = entriesShown(NOTHING_ON).map((e) => e.id);
    expect(off).not.toContain('automation');
    expect(off).not.toContain('published');
    const on = entriesShown(ALL_ON).map((e) => e.id);
    expect(on).toContain('automation');
    expect(on).toContain('published');
  });

  it('offers Space settings in the space menu for the same permissions as before (spec 036)', () => {
    expect([...SETTINGS_OPENERS].sort()).toEqual([...SC1_OPENERS].sort());
  });

  it('has a name and an introduction for every entry, a title for every group and every old name', () => {
    const word = (key: string): unknown =>
      key
        .split('.')
        .reduce<unknown>(
          (at, part) => (at && typeof at === 'object' ? (at as Record<string, unknown>)[part] : undefined),
          en,
        );
    for (const entry of SPACE_ENTRIES) {
      expect(typeof word(entryNameKey(entry.id)), entry.id).toBe('string');
      expect(typeof word(entryIntroKey(entry.id)), entry.id).toBe('string');
      for (const old of entry.formerly ?? []) expect(typeof word(old), old).toBe('string');
    }
    for (const place of SPACE_PLACES.filter((p) => p !== 'pinned')) {
      expect(typeof word(`spaceGroups.${place}`), place).toBe('string');
    }
  });

  it('names a width for every entry, full for the wide lists of D13', () => {
    const full = SPACE_ENTRIES.filter((e) => e.width === 'full').map((e) => e.id);
    expect(full.sort()).toEqual(
      [
        'automation',
        'content',
        'permissions',
        'audit',
        'trash',
        'files',
        'archived',
        'templates',
        'webhooks',
        'insights',
        'placeholders',
      ].sort(),
    );
  });
});
