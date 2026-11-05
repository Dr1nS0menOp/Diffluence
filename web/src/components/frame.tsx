// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState, type KeyboardEvent, type PointerEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, type PageDetail, type PageSummary, type Shortcut, type Space, type TreeNode } from '../api';
import { Link, match, useLocation } from '../router';
import { usePreferences } from '../preferences';
import { CopyDialog, DeleteDialog, MoveDialog, RenameDialog } from '../views/page-dialogs';
import { NewPageDialog, type CreateTarget } from './create';
import {
  IconChevronLeft,
  IconChevronRight,
  IconDownload,
  IconExternal,
  IconHome,
  IconLink,
  IconMore,
  IconSliders,
  IconStar,
  IconTrash,
} from './icons';
import { Menu, MenuDownload, MenuItem, MenuLink } from './menu';
import { useShortcut } from './shortcuts';
import { useCopy } from './toast';
import { PagesSection, type RowDialog } from './tree';
import { ErrorNote, Loading, SpaceBadge } from './ui';

const SIDEBAR_KEY = 'diffluence.sidebar';
const SIDEBAR_HIDDEN = 'hidden';
const NARROW_SCREEN = '(max-width: 50rem)';
const PAGE_ROUTES = ['/pages/:id', '/pages/:id/history', '/pages/:id/compare'];
const SPACE_ROUTES = ['/spaces/:key/settings', '/spaces/:key/trash', '/spaces/:key/overview'];
const MIN_SIDEBAR_REM = 14;
const MAX_SIDEBAR_REM = 32;
const MENU_ICON_SIZE = 18;
const LINK_ICON_SIZE = 18;

export const spaceByKeyQuery = (key: string) => ({
  queryKey: ['space', key],
  queryFn: () => api.get<Space>(`/space-keys/${encodeURIComponent(key)}`),
});

function firstMatch(patterns: string[], path: string, part: string): string | undefined {
  return patterns.map((pattern) => match(pattern, path)?.[part]).find(Boolean);
}

/** The space of the current screen, and the page in it if there is one. */
export function useCurrentSpace(): { space: Space | null; page: PageDetail | undefined } {
  const { pathname } = useLocation();
  const pageId = firstMatch(PAGE_ROUTES, pathname, 'id');
  const spaceKey = firstMatch(SPACE_ROUTES, pathname, 'key');
  const page = useQuery({
    queryKey: ['page', pageId],
    queryFn: () => api.get<PageDetail>(`/pages/${pageId}`),
    enabled: !!pageId,
  });
  const spaceOfPage = useQuery({
    queryKey: ['space-id', page.data?.space_id],
    queryFn: () => api.get<Space>(`/spaces/${page.data?.space_id}`),
    enabled: !!page.data,
  });
  const spaceAtKey = useQuery({ ...spaceByKeyQuery(spaceKey ?? ''), enabled: !!spaceKey });
  return { space: (pageId ? spaceOfPage.data : spaceAtKey.data) ?? null, page: page.data };
}

/** Where the Create button puts a new page: below what you are reading, if you may. */
export function useCreateTarget(): CreateTarget | null {
  const { space, page } = useCurrentSpace();
  if (!space?.can?.['page.create']) return null;
  return { space, parent: page?.status === 'published' ? page.id : null };
}

function useSidebar(): [boolean, (visible: boolean) => void] {
  const [visible, setVisible] = useState(() => {
    const stored = localStorage.getItem(SIDEBAR_KEY);
    return stored ? stored !== SIDEBAR_HIDDEN : !window.matchMedia(NARROW_SCREEN).matches;
  });
  const set = (next: boolean) => {
    setVisible(next);
    localStorage.setItem(SIDEBAR_KEY, next ? 'visible' : SIDEBAR_HIDDEN);
  };
  return [visible, set];
}

function SpaceMenu({ space }: { space: Space }) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const copy = useCopy();
  const favourite = useMutation({
    mutationFn: (on: boolean) =>
      on ? api.put(`/spaces/${space.id}/favourite`) : api.del(`/spaces/${space.id}/favourite`),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: ['space-id', space.id] });
      await client.invalidateQueries({ queryKey: ['space', space.key] });
      await client.invalidateQueries({ queryKey: ['my-spaces'] });
    },
  });
  return (
    <Menu label={t('space.more')} trigger={<IconMore size={MENU_ICON_SIZE} />} floating>
      <MenuItem onClick={() => favourite.mutate(!space.favourite)}>
        <IconStar size={MENU_ICON_SIZE} /> {space.favourite ? t('space.unfavourite') : t('space.favourite')}
      </MenuItem>
      <MenuItem
        onClick={() => void copy(`${window.location.origin}/spaces/${space.key}`, t('tree.linkCopied'))}
      >
        <IconLink size={MENU_ICON_SIZE} /> {t('tree.copyLink')}
      </MenuItem>
      {space.can?.['space.export'] && (
        <MenuDownload href={`/api/v1/spaces/${space.id}/export.zip`}>
          <IconDownload size={MENU_ICON_SIZE} /> {t('space.export')}
        </MenuDownload>
      )}
      {space.can?.['space.administer'] && (
        <MenuLink href={`/spaces/${space.key}/settings`}>
          <IconSliders size={MENU_ICON_SIZE} /> {t('space.settings')}
        </MenuLink>
      )}
    </Menu>
  );
}

function ShortcutLink({ shortcut }: { shortcut: Shortcut }) {
  const { t } = useTranslation();
  if (shortcut.target.startsWith('/')) {
    return (
      <Link href={shortcut.target} className="side-link">
        <IconLink size={LINK_ICON_SIZE} /> {shortcut.label}
      </Link>
    );
  }
  return (
    <a href={shortcut.target} className="side-link" target="_blank" rel="noopener noreferrer">
      <IconExternal size={LINK_ICON_SIZE} /> {shortcut.label}
      <span className="visually-hidden">{t('space.opensNewTab')}</span>
    </a>
  );
}

function SpaceLinks({ space }: { space: Space }) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const shortcuts = useQuery({
    queryKey: ['shortcuts', space.id],
    queryFn: () => api.get<{ shortcuts: Shortcut[] }>(`/spaces/${space.id}/shortcuts`),
  });
  const overview = `/spaces/${space.key}/overview`;
  return (
    <nav className="side-links" aria-label={t('space.links')}>
      <Link href={overview} className="side-link" aria-current={pathname === overview ? 'page' : undefined}>
        <IconHome size={LINK_ICON_SIZE} /> {t('space.overview')}
      </Link>
      {shortcuts.data?.shortcuts.map((s) => (
        <ShortcutLink key={s.id} shortcut={s} />
      ))}
    </nav>
  );
}

function RowDialogs({
  open,
  onClose,
}: {
  open: { page: PageSummary; dialog: RowDialog };
  onClose: () => void;
}) {
  const { page, dialog } = open;
  if (dialog === 'rename') return <RenameDialog page={page} onClose={onClose} />;
  if (dialog === 'move') return <MoveDialog page={page} onClose={onClose} />;
  if (dialog === 'copy') return <CopyDialog page={page} onClose={onClose} />;
  return <DeleteDialog page={page} onClose={onClose} />;
}

function Sidebar({
  space,
  current,
  parent,
  onHide,
}: {
  space: Space;
  current?: string;
  parent: string | null;
  onHide: () => void;
}) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState<{ parent: string | null } | null>(null);
  const [rowDialog, setRowDialog] = useState<{ page: PageSummary; dialog: RowDialog } | null>(null);
  const tree = useQuery({
    queryKey: ['tree', space.id],
    queryFn: () => api.get<{ pages: TreeNode[] }>(`/spaces/${space.id}/tree`),
  });
  const canCreate = space.can?.['page.create'] ?? false;
  return (
    <aside className="sidebar">
      <div className="sidebar-head">
        <Link href={`/spaces/${space.key}`} className="space-link">
          <SpaceBadge space={space} /> {space.name}
        </Link>
        <SpaceMenu space={space} />
        <button
          type="button"
          className="icon sidebar-toggle"
          aria-label={t('space.hideSidebar')}
          title={t('space.hideSidebar')}
          onClick={onHide}
        >
          <IconChevronLeft size={MENU_ICON_SIZE} />
        </button>
      </div>
      <SpaceLinks space={space} />
      {tree.isPending && <Loading />}
      <ErrorNote error={tree.error} />
      {tree.data && (
        <PagesSection
          key={space.id}
          spaceId={space.id}
          nodes={tree.data.pages}
          current={current}
          canMove={canCreate}
          onAdd={canCreate ? (id) => setAdding({ parent: id }) : undefined}
          onAddRoot={canCreate ? () => setAdding({ parent }) : undefined}
          onDialog={(page, dialog) => setRowDialog({ page, dialog })}
        />
      )}
      {space.can?.['space.administer'] && (
        <nav className="sidebar-foot" aria-label={t('space.settings')}>
          <Link href={`/spaces/${space.key}/settings`} className="side-link">
            <IconSliders size={MENU_ICON_SIZE} /> {t('space.settings')}
          </Link>
          <Link href={`/spaces/${space.key}/trash`} className="side-link">
            <IconTrash size={MENU_ICON_SIZE} /> {t('space.trash')}
          </Link>
        </nav>
      )}
      {adding && <NewPageDialog space={space} parent={adding.parent} onClose={() => setAdding(null)} />}
      {rowDialog && <RowDialogs open={rowDialog} onClose={() => setRowDialog(null)} />}
    </aside>
  );
}

const remInPixels = () => parseFloat(getComputedStyle(document.documentElement).fontSize);
const clampWidth = (rem: number) => Math.min(MAX_SIDEBAR_REM, Math.max(MIN_SIDEBAR_REM, Math.round(rem)));

/** The edge of the sidebar. Dragging has a keyboard way as well (WCAG 2.5.7). */
function SidebarEdge({ split }: { split: React.RefObject<HTMLDivElement | null> }) {
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

/** A screen inside a space: the sidebar with the page tree, and a reading column. */
export function Frame({
  space,
  current,
  parent,
  children,
}: {
  space: Space;
  current?: string;
  parent?: string | null;
  children: ReactNode;
}) {
  const { t } = useTranslation();
  const [sidebarVisible, setSidebarVisible] = useSidebar();
  const split = useRef<HTMLDivElement>(null);
  useShortcut('sidebar', () => setSidebarVisible(!sidebarVisible));
  return (
    <div ref={split} className={sidebarVisible ? 'split' : 'split collapsed'}>
      {sidebarVisible && (
        <div className="sidebar-frame">
          <Sidebar
            space={space}
            current={current}
            parent={parent ?? null}
            onHide={() => setSidebarVisible(false)}
          />
          <SidebarEdge split={split} />
        </div>
      )}
      <main id="main" className="content">
        {!sidebarVisible && (
          <button
            type="button"
            className="icon sidebar-toggle"
            aria-label={t('space.showSidebar')}
            title={t('space.showSidebar')}
            onClick={() => setSidebarVisible(true)}
          >
            <IconChevronRight size={MENU_ICON_SIZE} />
          </button>
        )}
        <div className="column">
          {space.status === 'archived' && (
            <p role="status" className="note">
              {t('space.archived')}
            </p>
          )}
          {children}
        </div>
      </main>
    </div>
  );
}

/** Frames a page with the sidebar of its space. */
export function SpaceShell({ pageId, children }: { pageId: string; children: ReactNode }) {
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
    return (
      <main id="main" className="plain reading">
        <ErrorNote error={page.error ?? space.error} />
      </main>
    );
  }
  return (
    <Frame space={space.data} current={pageId} parent={page.data?.status === 'published' ? pageId : null}>
      {children}
    </Frame>
  );
}
