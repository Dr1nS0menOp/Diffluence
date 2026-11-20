// SPDX-License-Identifier: Apache-2.0
// The sections of the administration and where each sits (spec 081, ADM-068 to ADM-078): three
// tool sections that are always open, then five settings groups that fold. Every section sits in
// exactly one group; the sidebar, the jump field and AdminView all read this one list.
import type { ReactElement } from 'react';
import type { Capabilities } from '../capabilities';
import {
  IconApps,
  IconArrowUp,
  IconAutomation,
  IconChart,
  IconEye,
  IconFileText,
  IconFolder,
  IconKey,
  IconList,
  IconMail,
  IconMegaphone,
  IconPeople,
  IconPerson,
  IconPulse,
  IconSliders,
} from '../components/icons';

export type AdminTier = 'tool' | 'settings';

/**
 * The tool sections, then the settings groups. Settings group ids start with `admin-`, because
 * the settings of a space keep their folded groups in the same preference (D3).
 */
export type AdminGroupId =
  | 'overview'
  | 'tools'
  | 'people'
  | 'admin-instance'
  | 'admin-security'
  | 'admin-permissions'
  | 'admin-look'
  | 'admin-data';

/** One column of reading width centred in the large area, or the full width beside the sidebar. */
export type AdminWidth = 'column' | 'full';

/** The experimental parts that hide a section while they are off (spec 069, D8). */
type ExperimentalPart = keyof Pick<Capabilities, 'automation' | 'bundles'>;

export type AdminIcon = (props: { size?: number }) => ReactElement;

export interface AdminSection {
  /** The address after `/admin/`, kept from before this wave (D2). */
  id: string;
  group: AdminGroupId;
  width: AdminWidth;
  /** Tool sections only: the icon before the name. */
  icon?: AdminIcon;
  experimental?: ExperimentalPart;
  /** A document of our documentation, relative to its root (D10). */
  doc?: string;
}

export const ADMIN_GROUPS: { id: AdminGroupId; tier: AdminTier }[] = [
  { id: 'overview', tier: 'tool' },
  { id: 'tools', tier: 'tool' },
  { id: 'people', tier: 'tool' },
  { id: 'admin-instance', tier: 'settings' },
  { id: 'admin-security', tier: 'settings' },
  { id: 'admin-permissions', tier: 'settings' },
  { id: 'admin-look', tier: 'settings' },
  { id: 'admin-data', tier: 'settings' },
];

const MONITORING = 'operations/monitoring.md';
const IDENTITY = 'operations/identity-providers.md';
const REGULATED = 'operations/regulated-use.md';

export const ADMIN_SECTIONS: AdminSection[] = [
  { id: 'summary', group: 'overview', width: 'column', icon: IconApps },
  { id: 'insights', group: 'overview', width: 'full', icon: IconChart },
  { id: 'analytics', group: 'overview', width: 'column', icon: IconEye },
  { id: 'audit', group: 'overview', width: 'full', icon: IconFileText, doc: REGULATED },
  { id: 'system', group: 'overview', width: 'column', icon: IconPulse, doc: MONITORING },
  { id: 'events', group: 'overview', width: 'full', icon: IconList, doc: MONITORING },
  { id: 'upgrades', group: 'overview', width: 'column', icon: IconArrowUp, doc: 'operations/upgrade.md' },
  {
    id: 'configuration',
    group: 'overview',
    width: 'column',
    icon: IconSliders,
    doc: 'operations/install.md',
  },
  { id: 'automation', group: 'tools', width: 'full', icon: IconAutomation, experimental: 'automation' },
  { id: 'spaces', group: 'tools', width: 'full', icon: IconFolder },
  { id: 'announcements', group: 'tools', width: 'column', icon: IconMegaphone },
  { id: 'users', group: 'people', width: 'full', icon: IconPerson },
  { id: 'groups', group: 'people', width: 'column', icon: IconPeople },
  { id: 'invitations', group: 'people', width: 'column', icon: IconMail },
  { id: 'service-accounts', group: 'people', width: 'column', icon: IconKey },
  { id: 'settings', group: 'admin-instance', width: 'column' },
  { id: 'rules', group: 'admin-instance', width: 'column', doc: REGULATED },
  { id: 'organising', group: 'admin-instance', width: 'column' },
  { id: 'mail', group: 'admin-instance', width: 'column' },
  { id: 'templates', group: 'admin-instance', width: 'column', doc: 'operations/templates.md' },
  { id: 'control', group: 'admin-instance', width: 'column', doc: REGULATED },
  { id: 'login', group: 'admin-security', width: 'column', doc: IDENTITY },
  { id: 'sessions', group: 'admin-security', width: 'column', doc: 'operations/incidents.md' },
  { id: 'scim', group: 'admin-security', width: 'column', doc: IDENTITY },
  { id: 'export', group: 'admin-security', width: 'column', doc: 'operations/public-sites.md' },
  { id: 'outgoing', group: 'admin-security', width: 'column' },
  { id: 'instance-permissions', group: 'admin-permissions', width: 'column' },
  { id: 'permissions', group: 'admin-permissions', width: 'column' },
  { id: 'space-defaults', group: 'admin-permissions', width: 'column' },
  { id: 'look', group: 'admin-look', width: 'column' },
  { id: 'care', group: 'admin-data', width: 'column' },
  { id: 'backups', group: 'admin-data', width: 'column', doc: 'operations/backup-restore.md' },
  { id: 'import-space', group: 'admin-data', width: 'column', experimental: 'bundles' },
  { id: 'spaces-trash', group: 'admin-data', width: 'column' },
  { id: 'webhooks', group: 'admin-data', width: 'column' },
];

/** The section at an address, or nothing for an id that is no section. */
export function adminSection(id: string): AdminSection | undefined {
  return ADMIN_SECTIONS.find((section) => section.id === id);
}

/** The sections the sidebar lists: one whose experimental part is off is left out (D8). */
export function sectionsShown(capabilities: Capabilities): AdminSection[] {
  return ADMIN_SECTIONS.filter((section) => !section.experimental || capabilities[section.experimental]);
}

/** The translation key of a section's name, the same words in the sidebar and as its title. */
export const sectionNameKey = (id: string) => `adminSections.${id}.name`;

/** The translation key of the one or two sentences that start a section (NAV-113). */
export const sectionIntroKey = (id: string) => `adminSections.${id}.intro`;
