// SPDX-License-Identifier: Apache-2.0
// The settings search of the console. The index holds the keys of the words the screens
// themselves use, so a renamed setting is found under its new name (spec 013, F9).

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
  { label: 'rules.embedHosts', tab: 'rules' },
  { label: 'rules.secondFactor', tab: 'rules' },
  { label: 'rules.idleMinutes', tab: 'rules' },
  { label: 'rules.signupDomains', tab: 'rules' },
  { label: 'rules.guestInvites', tab: 'rules' },
  { label: 'rules.auditDays', tab: 'rules' },
  { label: 'rules.features', tab: 'rules' },
  { label: 'instance.instanceName', tab: 'look' },
  { label: 'instance.logo', tab: 'look' },
  { label: 'instance.accent', tab: 'look' },
  { label: 'instance.loginWelcome', tab: 'look' },
  { label: 'instance.termsUrl', tab: 'look' },
  { label: 'instance.banner', tab: 'look' },
  { label: 'instance.welcomePage', tab: 'look' },
  { label: 'admin.appearance', tab: 'settings' },
  { label: 'care.maintenance', tab: 'care' },
  { label: 'care.jobs', tab: 'care' },
  { label: 'care.largest', tab: 'care' },
  { label: 'care.unused', tab: 'care' },
  { label: 'admin.permissionSets', tab: 'permissions' },
  { label: 'admin.categories', tab: 'permissions' },
  { label: 'admin.holders', tab: 'permissions' },
  { label: 'admin.invitations', tab: 'invitations' },
  { label: 'admin.serviceAccounts', tab: 'users' },
];

const MAX_FOUND = 8;

/** Finds settings whose words hold the text, the best match first. */
export function findSettings(
  index: SettingEntry[],
  text: string,
  word: (key: string) => string,
): SettingEntry[] {
  const wanted = text.trim().toLowerCase();
  if (!wanted) return [];
  const named = index.map((entry) => ({ entry, name: word(entry.label).toLowerCase() }));
  const found = named.filter((n) => n.name.includes(wanted));
  found.sort((a, b) => a.name.indexOf(wanted) - b.name.indexOf(wanted));
  return found.slice(0, MAX_FOUND).map((n) => n.entry);
}
