// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api, ApiError, type PageDetail, type Space } from '../api';
import { usePreferences } from '../preferences';
import { GlobalPart } from './global-sidebar';
import { SidebarToggle, useSidebarVisible } from './sidebar-state';
import { SpaceBlock } from './space-block';
import { ErrorNote, Loading } from './ui';

// Every screen of a space finds its space by the key in its address.
export { spaceByKeyQuery } from './current-space';

const MIN_SIDEBAR_REM = 14;
const MAX_SIDEBAR_REM = 32;

const remInPixels = () => parseFloat(getComputedStyle(document.documentElement).fontSize);
const clampWidth = (rem: number) => Math.min(MAX_SIDEBAR_REM, Math.max(MIN_SIDEBAR_REM, Math.round(rem)));

/** The edge of the sidebar. Dragging has a keyboard way as well (WCAG 2.5.7). */
export function SidebarEdge({ split }: { split: React.RefObject<HTMLDivElement | null> }) {
  const { t } = useTranslation();
  const { preferences, update } = usePreferences();
  const [width, setWidth] = useState(preferences.sidebar_width);
  const dragging = useRef(false);

  const [stored, setStored] = useState(preferences.sidebar_width);
  if (stored !== preferences.sidebar_width) {
    setStored(preferences.sidebar_width);
    setWidth(preferences.sidebar_width);
  }

  useEffect(() => {
    split.current?.style.setProperty('--sidebar-width', `${width}rem`);
  }, [width, split]);

  const isRtl = () => getComputedStyle(document.documentElement).direction === 'rtl';
  const widthAt = (e: PointerEvent<HTMLDivElement>) => {
    const box = split.current?.getBoundingClientRect();
    if (!box) return width;
    const pixels = isRtl() ? box.right - e.clientX : e.clientX - box.left;
    return clampWidth(pixels / remInPixels());
  };
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    const wider = isRtl() ? 'ArrowLeft' : 'ArrowRight';
    const narrower = isRtl() ? 'ArrowRight' : 'ArrowLeft';
    if (e.key !== wider && e.key !== narrower) return;
    e.preventDefault();
    const next = clampWidth(width + (e.key === wider ? 1 : -1));
    setWidth(next);
    update({ sidebar_width: next });
  };
  return (
    <div
      role="separator"
      tabIndex={0}
      className="sidebar-edge"
      aria-orientation="vertical"
      aria-label={t('space.sidebarWidth')}
      aria-valuemin={MIN_SIDEBAR_REM}
      aria-valuemax={MAX_SIDEBAR_REM}
      aria-valuenow={width}
      onKeyDown={keys}
      onPointerDown={(e) => {
        dragging.current = true;
        e.currentTarget.setPointerCapture(e.pointerId);
      }}
      onPointerMove={(e) => {
        if (dragging.current) setWidth(widthAt(e));
      }}
      onPointerUp={(e) => {
        if (!dragging.current) return;
        dragging.current = false;
        update({ sidebar_width: widthAt(e) });
      }}
    />
  );
}

/**
 * A screen inside a space: the sidebar with the global part and the block of the space, and a
 * reading column. The button that hides the sidebar sits in the top bar (NAV-092).
 */
export function Frame({
  space,
  current,
  parent,
  focus,
  children,
}: {
  space: Space;
  current?: string;
  parent?: string | null;
  /** The editor: no reading column, and the tools run the full width of what is left. */
  focus?: boolean;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const sidebarVisible = useSidebarVisible();
  const split = useRef<HTMLDivElement>(null);
  return (
    <div ref={split} className={sidebarVisible ? 'split' : 'split collapsed'}>
      <SidebarToggle />
      {sidebarVisible && (
        <div className="sidebar-frame">
          <aside className="sidebar">
            <GlobalPart />
            <SpaceBlock space={space} current={current} parent={parent ?? null} />
          </aside>
          <SidebarEdge split={split} />
        </div>
      )}
      {focus ? (
        <main id="main" className="focus">
          {children}
        </main>
      ) : (
        <main id="main" className="content">
          <div className="column">
            {space.status === 'archived' && (
              <p role="status" className="note">
                {t('space.archived')}
              </p>
            )}
            {children}
          </div>
        </main>
      )}
    </div>
  );
}

/** "Not found", with a way to ask only when the space of the page chose that (COL-049). */
function PageMissing({ pageId, error }: { pageId: string; error: unknown }) {
  const { t } = useTranslation();
  const ask = useMutation({
    mutationFn: () => api.post(`/pages/${pageId}/access-requests`, { kind: 'view' }),
  });
  const mayAsk = error instanceof ApiError && error.code === 'not_found_ask';
  return (
    <main id="main" className="plain reading">
      <ErrorNote error={error} />
      <ErrorNote error={ask.error} />
      {ask.isSuccess && (
        <p role="status" className="note">
          {t('share.asked')}
        </p>
      )}
      {mayAsk && !ask.isSuccess && (
        <button type="button" disabled={ask.isPending} onClick={() => ask.mutate()}>
          {t('share.askForAccess')}
        </button>
      )}
    </main>
  );
}

/** Frames a page with the sidebar of its space. */
export function SpaceShell({
  pageId,
  focus,
  children,
}: {
  pageId: string;
  focus?: boolean;
  children: ReactNode;
}) {
  const page = useQuery({
    queryKey: ['page', pageId],
    queryFn: () => api.get<PageDetail>(`/pages/${pageId}`),
    // The frame stays while the next page loads, so the tree does not blink.
    placeholderData: (previous) => previous,
  });
  const space = useQuery({
    queryKey: ['space-id', page.data?.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page.data?.space_id}`),
    enabled: !!page.data,
  });
  if (page.isPending || (page.data && space.isPending)) return <Loading />;
  if (page.error || space.error || !space.data) {
    return <PageMissing pageId={pageId} error={page.error ?? space.error} />;
  }
  return (
    <Frame
      space={space.data}
      current={pageId}
      parent={page.data?.status === 'published' ? pageId : null}
      focus={focus}
    >
      {children}
    </Frame>
  );
}
