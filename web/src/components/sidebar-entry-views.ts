// SPDX-License-Identifier: Apache-2.0
// What each entry of the global part of the sidebar shows, and where it leads (NAV-126).
import type { ComponentType } from 'react';
import { IconApps, IconClock, IconEye, IconHome, IconPeople, IconPencil, IconStar, IconTask } from './icons';
import type { SidebarEntry } from './sidebar-entries';

export type PanelName = 'recent' | 'favourites' | 'spaces';

export interface EntryView {
  label: string;
  Icon: ComponentType<{ size?: number }>;
  /** A screen of its own, or else a panel beside the sidebar. */
  href?: string;
  panel?: PanelName;
}

export const ENTRY_VIEWS: Record<SidebarEntry, EntryView> = {
  recent: { label: 'nav.recent', Icon: IconClock, panel: 'recent' },
  favourites: { label: 'nav.favourites', Icon: IconStar, panel: 'favourites' },
  spaces: { label: 'nav.spaces', Icon: IconApps, panel: 'spaces' },
  drafts: { label: 'nav.drafts', Icon: IconPencil, href: '/drafts' },
  tasks: { label: 'tasks.title', Icon: IconTask, href: '/tasks' },
  people: { label: 'nav.people', Icon: IconPeople, href: '/people' },
  watching: { label: 'watching.title', Icon: IconEye, href: '/me/watching' },
};

/** The start screen: always first and always shown (NAV-139). */
export const START_ENTRY = { label: 'nav.home', Icon: IconHome, href: '/' } as const;
