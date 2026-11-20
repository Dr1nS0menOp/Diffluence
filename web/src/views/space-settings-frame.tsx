// SPDX-License-Identifier: Apache-2.0
// The Settings frame of a space (spec 083, SPC-134 to SPC-147), and the frame of a screen of its
// own, such as Content or Trash: inside the settings for whoever may open them, and in the frame
// of the space for everyone else, as before (D6).
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { Space } from '../api';
import { useCapabilities } from '../capabilities';
import { Frame } from '../components/frame';
import { SettingsFrame, type SettingsGroup, type SettingsItem } from '../components/settings-frame';
import { SpaceBadge } from '../components/ui';
import {
  SPACE_PLACES,
  entriesOpen,
  entryHref,
  entryNameKey,
  spaceEntryAt,
  type SpaceEntry,
} from './space-sections';

export function SpaceSettingsFrame({
  space,
  current,
  children,
}: {
  space: Space;
  /** The id of the entry that is open, or empty when the person may open none. */
  current: string;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const capabilities = useCapabilities();
  const open = entriesOpen(space, capabilities);
  const item = (entry: SpaceEntry): SettingsItem => ({
    id: entry.id,
    label: t(entryNameKey(entry.id)),
    href: entryHref(space.key, entry),
    icon: entry.icon,
    mark: entry.experimental ? t('adminFrame.experimental') : undefined,
    formerly: (entry.formerly ?? []).map((key) => t(key)),
  });
  const groups: SettingsGroup[] = SPACE_PLACES.filter((place) => place !== 'pinned')
    .map((place) => ({
      id: place,
      title: t(`spaceGroups.${place}`),
      items: open.filter((entry) => entry.place === place).map(item),
    }))
    // A group with no entry the person may open is absent (SPC-139).
    .filter((group) => group.items.length > 0);
  return (
    <SettingsFrame
      title={t('space.settings')}
      sidebar={{
        kind: 'space',
        back: { href: `/spaces/${space.key}`, label: t('spaceSettings.back') },
        head: (
          <div className="space-head-name">
            <SpaceBadge space={space} />
            <span className="side-link-text">{space.name}</span>
          </div>
        ),
        pinned: open.filter((entry) => entry.place === 'pinned').map(item),
      }}
      groups={groups}
      current={current}
      width={spaceEntryAt(current)?.width ?? 'column'}
    >
      {children}
    </SettingsFrame>
  );
}

/**
 * A screen of its own that is also an entry of space settings (D5): it keeps its address, and
 * opens beside the settings sidebar for whoever may open space settings, else in the space.
 */
export function SpaceToolFrame({
  space,
  entry,
  children,
}: {
  space: Space;
  entry: string;
  children: ReactNode;
}) {
  const capabilities = useCapabilities();
  if (entriesOpen(space, capabilities).length === 0) return <Frame space={space}>{children}</Frame>;
  return (
    <SpaceSettingsFrame space={space} current={entry}>
      {children}
    </SpaceSettingsFrame>
  );
}
