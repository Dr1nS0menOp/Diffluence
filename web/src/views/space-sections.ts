// SPDX-License-Identifier: Apache-2.0
// The entries of space settings and where each sits (spec 083, SPC-138 to SPC-145): four pinned
// entries with icons, then five groups that fold. Every entry sits in exactly one place; the
// sidebar, the jump field, SpaceSettingsView, the screens of their own and the space menu all read
// this one list.
import type { Capabilities } from '../capabilities';
import type { Space } from '../api';
import type { SettingsIcon } from '../components/settings-frame';
import { IconAutomation, IconFileText, IconInfo, IconPeople } from '../components/icons';

/**
 * The pinned entries, then the groups. Group ids start with `space-`, because the administration
 * keeps its folded groups in the same preference (D3).
 */
export type SpacePlace =
  'pinned' | 'space-general' | 'space-access' | 'space-pages' | 'space-look' | 'space-connections';

export const SPACE_PLACES: SpacePlace[] = [
  'pinned',
  'space-general',
  'space-access',
  'space-pages',
  'space-look',
  'space-connections',
];

/** The experimental parts that hide an entry while they are off (SC4 D8). */
type ExperimentalPart = keyof Pick<Capabilities, 'automation' | 'publishing'>;

export interface SpaceEntry {
  /** The `?section=` id, or for a screen of its own the name it has in the sidebar. */
  id: string;
  place: SpacePlace;
  /** One column of reading width centred in the large area, or the full width (D13). */
  width: 'column' | 'full';
  /** The permissions that open it; any one is enough. */
  needs: string[];
  /** A screen of its own: its path after `/spaces/:key` (D5). */
  path?: string;
  icon?: SettingsIcon;
  experimental?: ExperimentalPart;
  /** Old section ids that open this entry (D7). */
  aliases?: string[];
  /** Translation keys of the names of sections this entry took over, which the jump field finds (D11). */
  formerly?: string[];
  /** A document of our documentation, relative to its root (D17). */
  doc?: string;
}

const ADMINISTER = ['space.administer'];
const LOOK = ['space.look'];
const ACCESS = ['space.manage_access'];
const AUTOMATION = ['space.automation'];

export const SPACE_ENTRIES: SpaceEntry[] = [
  {
    id: 'automation',
    place: 'pinned',
    width: 'full',
    needs: AUTOMATION,
    path: '/settings/automation',
    icon: IconAutomation,
    experimental: 'automation',
  },
  {
    id: 'people',
    place: 'pinned',
    width: 'column',
    needs: ACCESS,
    icon: IconPeople,
    aliases: ['roles'],
    formerly: ['roles.spaceSection'],
  },
  { id: 'content', place: 'pinned', width: 'full', needs: ADMINISTER, path: '/content', icon: IconFileText },
  {
    id: 'details',
    place: 'pinned',
    width: 'column',
    needs: ['space.administer', 'space.look'],
    icon: IconInfo,
    aliases: ['key', 'categories'],
    formerly: ['spaceSettings.key', 'spaceSettings.owners', 'admin.categories', 'spaceSettings.home'],
  },
  { id: 'features', place: 'space-general', width: 'column', needs: ADMINISTER },
  {
    id: 'archive',
    place: 'space-general',
    width: 'column',
    needs: ADMINISTER,
    aliases: ['danger'],
    formerly: ['spaceSettings.archiveOrDelete'],
  },
  { id: 'delete', place: 'space-general', width: 'column', needs: ADMINISTER },
  { id: 'export', place: 'space-general', width: 'column', needs: ['space.administer', 'space.bundle'] },
  { id: 'import', place: 'space-general', width: 'column', needs: ADMINISTER },
  { id: 'picture', place: 'space-general', width: 'column', needs: ['space.administer', 'space.look'] },
  {
    id: 'statuses',
    place: 'space-general',
    width: 'column',
    needs: ['space.statuses', 'space.approvals'],
    aliases: ['rules'],
    formerly: ['spaceSettings.statuses', 'spaceSettings.rules'],
  },
  { id: 'workflows', place: 'space-general', width: 'column', needs: ['space.workflows'] },
  { id: 'insights', place: 'space-general', width: 'full', needs: ADMINISTER, path: '/insights' },
  {
    id: 'permissions',
    place: 'space-access',
    width: 'full',
    needs: ACCESS,
    path: '/access',
    formerly: ['access.open'],
  },
  // What someone without access sees (wave UR1, spec 094) takes its place here, after Permissions.
  {
    id: 'sites',
    place: 'space-access',
    width: 'column',
    needs: ['site.publish'],
    path: '/sites',
    doc: 'operations/public-sites.md',
  },
  { id: 'audit', place: 'space-access', width: 'full', needs: ['space.audit'], path: '/audit' },
  { id: 'placeholders', place: 'space-pages', width: 'full', needs: ADMINISTER },
  { id: 'files', place: 'space-pages', width: 'full', needs: ADMINISTER },
  { id: 'trash', place: 'space-pages', width: 'full', needs: ADMINISTER, path: '/trash' },
  { id: 'archived', place: 'space-pages', width: 'full', needs: ADMINISTER },
  { id: 'labels', place: 'space-pages', width: 'column', needs: ['space.labels'] },
  { id: 'shortcuts', place: 'space-pages', width: 'column', needs: LOOK },
  {
    id: 'published',
    place: 'space-pages',
    width: 'column',
    needs: ['space.administer', 'space.release'],
    experimental: 'publishing',
    doc: 'operations/published-spaces.md',
  },
  {
    id: 'templates',
    place: 'space-look',
    width: 'full',
    needs: ['space.templates'],
    path: '/templates',
    doc: 'operations/templates.md',
  },
  { id: 'layout', place: 'space-look', width: 'column', needs: LOOK },
  { id: 'pdf', place: 'space-look', width: 'column', needs: ADMINISTER },
  {
    id: 'webhooks',
    place: 'space-connections',
    width: 'full',
    needs: AUTOMATION,
    path: '/settings/webhooks',
  },
];

/** Proposing templates opens Templates while guided templates are on (spec 074, TPL-1002). */
const PROPOSING = 'template.propose';

/** The entry at a `?section=` id or one of its aliases, or nothing for an id that is no entry. */
export function spaceEntryAt(id: string): SpaceEntry | undefined {
  return SPACE_ENTRIES.find((entry) => entry.id === id || (entry.aliases ?? []).includes(id));
}

/** The permissions that open an entry; any one is enough. */
export function openersOf(id: string, capabilities: Capabilities): string[] {
  const entry = SPACE_ENTRIES.find((e) => e.id === id);
  if (!entry) return [];
  return id === 'templates' && capabilities.guided_templates ? [...entry.needs, PROPOSING] : entry.needs;
}

/** The entries the sidebar may list: one whose experimental part is off is left out. */
export function entriesShown(capabilities: Capabilities): SpaceEntry[] {
  return SPACE_ENTRIES.filter((entry) => !entry.experimental || capabilities[entry.experimental]);
}

/** The entries a person may open in a space, in the order of the sidebar. */
export function entriesOpen(space: Space, capabilities: Capabilities): SpaceEntry[] {
  return entriesShown(capabilities).filter((entry) =>
    openersOf(entry.id, capabilities).some((permission) => space.can?.[permission]),
  );
}

/**
 * Entries the space menu reaches through an item of its own (SPC-107): its People item opens People,
 * whose introduction leads on to Permissions. Someone who may open only these gets no Space settings
 * item as well, as before this wave.
 */
const OWN_MENU_ITEM = new Set(['people', 'permissions']);

/**
 * The permissions that make the space menu offer Space settings, whatever is on (spec 036); proposing
 * templates joins them while guided templates are on.
 */
export const SETTINGS_OPENERS: readonly string[] = [
  ...new Set(SPACE_ENTRIES.filter((e) => !OWN_MENU_ITEM.has(e.id)).flatMap((e) => e.needs)),
];

/** Whether the space menu offers Space settings to a person. */
export function mayOpenSpaceSettings(space: Space, capabilities: Capabilities): boolean {
  return entriesOpen(space, capabilities).some((entry) => !OWN_MENU_ITEM.has(entry.id));
}

/** The address of an entry in a space. */
export function entryHref(spaceKey: string, entry: SpaceEntry): string {
  const base = `/spaces/${spaceKey}`;
  return entry.path ? `${base}${entry.path}` : `${base}/settings?section=${entry.id}`;
}

/** The translation key of an entry's name, the same words in the sidebar and as its title. */
export const entryNameKey = (id: string) => `spaceSections.${id}.name`;

/** The translation key of the one or two sentences that start an entry (NAV-113). */
export const entryIntroKey = (id: string) => `spaceSections.${id}.intro`;
