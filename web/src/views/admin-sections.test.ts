// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { NOTHING_ON, type Capabilities } from '../capabilities';
import { ADMIN_GROUPS, ADMIN_SECTIONS, adminSection, sectionsShown } from './admin-sections';
import { SETTINGS_INDEX } from './admin-search';
import en from '../locales/en.json';

/** The words of a translation key, or nothing when the key does not exist. */
const word = (key: string): unknown =>
  key
    .split('.')
    .reduce<unknown>(
      (at, part) => (at && typeof at === 'object' ? (at as Record<string, unknown>)[part] : undefined),
      en,
    );

/** Every address the administration answered before wave SC4 (spec 081, D2). */
const ADDRESSES_BEFORE_SC4 = [
  'summary',
  'users',
  'invitations',
  'groups',
  'spaces',
  'templates',
  'organising',
  'control',
  'export',
  'audit',
  'permissions',
  'rules',
  'look',
  'settings',
  'mail',
  'care',
  'scim',
  'system',
  'backups',
  'upgrades',
  'events',
  'configuration',
  'sessions',
  'insights',
  'analytics',
  'automation',
  'outgoing',
  'webhooks',
];

const ADDED_IN_SC4 = [
  'announcements',
  'service-accounts',
  'login',
  'instance-permissions',
  'space-defaults',
  'import-space',
  'spaces-trash',
];

const ALL_ON: Capabilities = Object.fromEntries(
  Object.keys(NOTHING_ON).map((name) => [name, true]),
) as unknown as Capabilities;

describe('the sections of the administration', () => {
  it('puts every section in exactly one known group (ADM-077)', () => {
    const groups = new Set(ADMIN_GROUPS.map((group) => group.id));
    for (const section of ADMIN_SECTIONS) expect(groups).toContain(section.group);
    const ids = ADMIN_SECTIONS.map((section) => section.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it('still answers every address of before, and the seven new ones', () => {
    for (const id of [...ADDRESSES_BEFORE_SC4, ...ADDED_IN_SC4]) expect(adminSection(id)?.id).toBe(id);
    expect(ADMIN_SECTIONS).toHaveLength(ADDRESSES_BEFORE_SC4.length + ADDED_IN_SC4.length);
  });

  it('does not take an unknown id, or a name of a group, for a section', () => {
    expect(adminSection('nosuch')).toBeUndefined();
    expect(adminSection('admin-data')).toBeUndefined();
    expect(adminSection('')).toBeUndefined();
    expect(adminSection('__proto__')).toBeUndefined();
  });

  it('orders the tool sections and the settings groups as ADM-068 to ADM-075 do', () => {
    expect(ADMIN_GROUPS.map((group) => [group.tier, group.id])).toEqual([
      ['tool', 'overview'],
      ['tool', 'tools'],
      ['tool', 'people'],
      ['settings', 'admin-instance'],
      ['settings', 'admin-security'],
      ['settings', 'admin-permissions'],
      ['settings', 'admin-look'],
      ['settings', 'admin-data'],
    ]);
    const inGroup = (group: string) => ADMIN_SECTIONS.filter((s) => s.group === group).map((s) => s.id);
    expect(inGroup('overview')).toEqual([
      'summary',
      'insights',
      'analytics',
      'audit',
      'system',
      'events',
      'upgrades',
      'configuration',
    ]);
    expect(inGroup('tools')).toEqual(['automation', 'spaces', 'announcements']);
    expect(inGroup('people')).toEqual(['users', 'groups', 'invitations', 'service-accounts']);
    expect(inGroup('admin-instance')).toEqual([
      'settings',
      'rules',
      'organising',
      'mail',
      'templates',
      'control',
    ]);
    expect(inGroup('admin-security')).toEqual(['login', 'sessions', 'scim', 'export', 'outgoing']);
    expect(inGroup('admin-permissions')).toEqual(['instance-permissions', 'permissions', 'space-defaults']);
    expect(inGroup('admin-look')).toEqual(['look']);
    expect(inGroup('admin-data')).toEqual(['care', 'backups', 'import-space', 'spaces-trash', 'webhooks']);
  });

  it('gives the tool sections an icon and the settings groups none', () => {
    const tier = (group: string) => ADMIN_GROUPS.find((g) => g.id === group)?.tier;
    for (const section of ADMIN_SECTIONS) {
      if (tier(section.group) === 'tool') expect(section.icon, section.id).toBeDefined();
      else expect(section.icon, section.id).toBeUndefined();
    }
  });

  it('starts every group id of the settings with admin-, so space settings fold apart (D3)', () => {
    for (const group of ADMIN_GROUPS.filter((g) => g.tier === 'settings')) {
      expect(group.id).toMatch(/^admin-[a-z][a-z0-9-]{0,39}$/);
    }
  });

  it('leaves out a section whose experimental part is off, and keeps it once it is on', () => {
    const off = sectionsShown(NOTHING_ON).map((s) => s.id);
    expect(off).not.toContain('automation');
    expect(off).not.toContain('import-space');
    expect(off).toContain('summary');
    const on = sectionsShown(ALL_ON).map((s) => s.id);
    expect(on).toContain('automation');
    expect(on).toContain('import-space');
    expect(on).toHaveLength(ADMIN_SECTIONS.length);
  });

  it('lays out wide lists at full width and the rest in the centred column (ADM-078)', () => {
    const full = ADMIN_SECTIONS.filter((s) => s.width === 'full').map((s) => s.id);
    expect(full.sort()).toEqual(['audit', 'automation', 'events', 'insights', 'spaces', 'users']);
  });

  it('points every entry of the settings search at a section that exists', () => {
    for (const entry of SETTINGS_INDEX) expect(adminSection(entry.tab), entry.label).toBeDefined();
  });
  it('has a name and an introduction for every section, and a name for every group', () => {
    for (const section of ADMIN_SECTIONS) {
      expect(typeof word(`adminSections.${section.id}.name`), section.id).toBe('string');
      expect(typeof word(`adminSections.${section.id}.intro`), section.id).toBe('string');
    }
    for (const group of ADMIN_GROUPS) expect(typeof word(`adminGroups.${group.id}`), group.id).toBe('string');
  });

  it('names the section settings General, because Settings heads the whole tier (spec 081, item 15)', () => {
    expect(word('adminSections.settings.name')).toBe('General');
  });

  it('finds every word of the settings search in the translations', () => {
    for (const entry of SETTINGS_INDEX) expect(typeof word(entry.label), entry.label).toBe('string');
  });
});
