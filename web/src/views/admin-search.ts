// SPDX-License-Identifier: Apache-2.0
// The settings search of the console. The index holds the keys of the words the screens
// themselves use, so a renamed setting is found under its new name (spec 013, F9). It finds the
// sections themselves by name as well, from the registry, so there is one search (spec 081, D6).
import { sectionNameKey, type AdminSection } from './admin-sections';

export interface SettingEntry {
  /** A translation key: the same word as on the screen. */
  label: string;
  tab: string;
}

export const SETTINGS_INDEX: SettingEntry[] = [
  { label: 'admin.retention', tab: 'settings' },
  { label: 'admin.maxUpload', tab: 'settings' },
  { label: 'rules.defaultTimeZone', tab: 'rules' },
  { label: 'rules.defaultLanguage', tab: 'rules' },
  { label: 'rules.helpUrl', tab: 'rules' },
  { label: 'rules.embedHosts', tab: 'outgoing' },
  { label: 'rules.secondFactor', tab: 'login' },
  { label: 'rules.idleMinutes', tab: 'login' },
  { label: 'rules.signupDomains', tab: 'login' },
  { label: 'rules.guestInvites', tab: 'login' },
  { label: 'rules.auditDays', tab: 'rules' },
  { label: 'rules.features', tab: 'rules' },
  { label: 'instance.instanceName', tab: 'look' },
  { label: 'instance.logo', tab: 'look' },
  { label: 'instance.accent', tab: 'look' },
  { label: 'instance.loginWelcome', tab: 'look' },
  { label: 'instance.termsUrl', tab: 'look' },
  { label: 'instance.banner', tab: 'announcements' },
  { label: 'instance.welcomePage', tab: 'look' },
  { label: 'admin.font', tab: 'look' },
  { label: 'care.maintenance', tab: 'care' },
  { label: 'care.jobs', tab: 'care' },
  { label: 'care.largest', tab: 'care' },
  { label: 'care.unused', tab: 'care' },
  { label: 'roles.spaceRoles', tab: 'permissions' },
  { label: 'roles.instanceRoles', tab: 'permissions' },
  { label: 'admin.categories', tab: 'organising' },
  { label: 'admin.holders', tab: 'instance-permissions' },
  { label: 'admin.invitations', tab: 'invitations' },
  { label: 'admin.serviceAccounts', tab: 'service-accounts' },
  { label: 'installation.diskWarn', tab: 'settings' },
  { label: 'installation.diskRefuse', tab: 'settings' },
  { label: 'installation.logLevel', tab: 'system' },
  { label: 'installation.monitoringFiles', tab: 'system' },
  { label: 'admin.authTitle', tab: 'login' },
  { label: 'adminOrganise.defaultPreset', tab: 'space-defaults' },
  { label: 'adminOrganise.spaceTemplates', tab: 'templates' },
  { label: 'exchange.bundles', tab: 'import-space' },
  { label: 'admin.trashedSpaces', tab: 'spaces-trash' },
];

const MAX_FOUND = 8;

/**
 * What the jump field searches: the sections shown, by their names, then the settings inside them.
 * A setting of a section that is not shown, such as an experimental part that is off, is left out.
 */
export function jumpIndex(shown: AdminSection[]): SettingEntry[] {
  const ids = new Set(shown.map((section) => section.id));
  return [
    ...shown.map((section) => ({ label: sectionNameKey(section.id), tab: section.id })),
    ...SETTINGS_INDEX.filter((entry) => ids.has(entry.tab)),
  ];
}

/** Finds settings whose words hold the text, the best match first, each name and place once. */
export function findSettings(
  index: SettingEntry[],
  text: string,
  word: (key: string) => string,
): SettingEntry[] {
  const wanted = text.trim().toLowerCase();
  if (!wanted) return [];
  const seen = new Set<string>();
  const named = index
    .map((entry) => ({ entry, name: word(entry.label).toLowerCase() }))
    .filter((n) => {
      const place = `${n.name}\n${n.entry.tab}`;
      if (seen.has(place)) return false;
      seen.add(place);
      return true;
    });
  const found = named.filter((n) => n.name.includes(wanted));
  found.sort((a, b) => a.name.indexOf(wanted) - b.name.indexOf(wanted));
  return found.slice(0, MAX_FOUND).map((n) => n.entry);
}
