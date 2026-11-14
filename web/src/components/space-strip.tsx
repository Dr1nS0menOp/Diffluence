// SPDX-License-Identifier: Apache-2.0
// The strip of spaces at the top of the sidebar (spec 016, H1). It scrolls on its
// own, so the sidebar and the page stay where they are.
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type Space } from '../api';
import { Link } from '../router';
import { IconApps } from './icons';
import { SpaceBadge } from './ui';

const ALL_SPACES_ICON_SIZE = 16;

interface MySpaces {
  favourites: Space[];
  recent: Space[];
}

/** One order everywhere: favourites, then recent spaces, then the rest. */
export function orderSpaces(mine: MySpaces | undefined, all: Space[] | undefined): Space[] {
  const known = [...(mine?.favourites ?? []), ...(mine?.recent ?? [])];
  const knownIds = new Set(known.map((s) => s.id));
  return [...known, ...(all ?? []).filter((s) => !knownIds.has(s.id))];
}

/** A person must see where they are, also in a space the lists leave out. */
export function withCurrent(spaces: Space[], current: Space): Space[] {
  return spaces.some((s) => s.id === current.id) ? spaces : [current, ...spaces];
}

export function useOrderedSpaces(): Space[] {
  const mine = useQuery({
    queryKey: ['my-spaces'],
    queryFn: () => api.get<MySpaces>('/me/spaces'),
  });
  const all = useQuery({
    queryKey: ['spaces'],
    queryFn: () => api.get<{ spaces: Space[] }>('/spaces'),
  });
  return orderSpaces(mine.data, all.data?.spaces);
}

export function SpaceStrip({ current }: { current: Space }) {
  const { t } = useTranslation();
  const spaces = withCurrent(useOrderedSpaces(), current);
  const list = useRef<HTMLUListElement>(null);
  const place = spaces.findIndex((s) => s.id === current.id);

  useEffect(() => {
    const box = list.current?.getBoundingClientRect();
    const chip = list.current?.querySelector('[aria-current]')?.getBoundingClientRect();
    if (!list.current || !box || !chip) return;
    // Only the strip moves: scrollIntoView would take the sidebar along.
    if (chip.left < box.left) list.current.scrollLeft -= box.left - chip.left;
    else if (chip.right > box.right) list.current.scrollLeft += chip.right - box.right;
    // The list arrives in two answers; the current space can move when the second one lands.
  }, [place, spaces.length]);

  useEffect(() => {
    const box = list.current;
    if (!box) return;
    // A mouse wheel only goes up and down, and here that should move the strip.
    const wheel = (e: WheelEvent) => {
      if (box.scrollWidth <= box.clientWidth || e.deltaY === 0) return;
      e.preventDefault();
      box.scrollLeft += e.deltaY;
    };
    box.addEventListener('wheel', wheel, { passive: false });
    return () => box.removeEventListener('wheel', wheel);
  }, []);

  return (
    <nav className="space-strip" aria-label={t('space.strip')}>
      <ul ref={list}>
        {spaces.map((s) => (
          <li key={s.id}>
            <Link
              href={`/spaces/${s.key}`}
              className="space-chip"
              title={s.name}
              aria-current={s.id === current.id ? 'true' : undefined}
            >
              <SpaceBadge space={s} size="small" />
              <span>{s.name}</span>
            </Link>
          </li>
        ))}
      </ul>
      <Link
        href="/spaces"
        className="space-strip-all"
        aria-label={t('nav.allSpaces')}
        title={t('nav.allSpaces')}
      >
        <IconApps size={ALL_SPACES_ICON_SIZE} />
      </Link>
    </nav>
  );
}
