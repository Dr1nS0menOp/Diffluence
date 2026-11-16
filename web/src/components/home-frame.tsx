// SPDX-License-Identifier: Apache-2.0
// The frame of the personal screens: the start screen, Drafts, Tasks, Watching, Recent,
// Favourites and the list of spaces. The sidebar holds the global part and the favourite
// spaces (spec 078, NAV-125, NAV-127).
import { useRef, type ReactNode } from 'react';
import { SidebarEdge } from './frame';
import { FavouriteSpaces, GlobalPart } from './global-sidebar';
import { SidebarToggle, useSidebarVisible } from './sidebar-state';

/** A personal screen with the sidebar beside it. */
export function HomeFrame({ width, children }: { width?: 'reading'; children: ReactNode }) {
  const sidebarVisible = useSidebarVisible();
  const split = useRef<HTMLDivElement>(null);
  return (
    <div ref={split} className={sidebarVisible ? 'split' : 'split collapsed'}>
      <SidebarToggle />
      {sidebarVisible && (
        <div className="sidebar-frame">
          <aside className="sidebar">
            <GlobalPart />
            <FavouriteSpaces />
          </aside>
          <SidebarEdge split={split} />
        </div>
      )}
      <main id="main" className="content">
        <div className={width === 'reading' ? 'column' : 'column wide'}>{children}</div>
      </main>
    </div>
  );
}
