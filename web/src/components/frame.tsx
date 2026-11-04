// SPDX-License-Identifier: Apache-2.0
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api, type PageDetail, type Space, type TreeNode } from '../api';
import { Link, match, useLocation } from '../router';
import { NewPageDialog, type CreateTarget } from './create';
import { IconChevronLeft, IconChevronRight, IconPlus, IconSliders, IconTrash } from './icons';
import { PageTree } from './tree';
import { ErrorNote, Loading, SpaceBadge } from './ui';

const SIDEBAR_KEY = 'diffluence.sidebar';
const SIDEBAR_HIDDEN = 'hidden';
const NARROW_SCREEN = '(max-width: 50rem)';
const PAGE_ROUTES = ['/pages/:id', '/pages/:id/history', '/pages/:id/compare'];
const SPACE_ROUTES = ['/spaces/:key/settings', '/spaces/:key/trash'];

export const spaceByKeyQuery = (key: string) => ({
  queryKey: ['space', key],
  queryFn: () => api.get<Space>(`/space-keys/${encodeURIComponent(key)}`),
});

function firstMatch(patterns: string[], path: string, part: string): string | undefined {
  return patterns.map((pattern) => match(pattern, path)?.[part]).find(Boolean);
}

/** Where the Create button puts a new page: below what you are reading, if you may. */
export function useCreateTarget(): CreateTarget | null {
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
  const space = pageId ? spaceOfPage.data : spaceAtKey.data;
  if (!space?.can?.['page.create']) return null;
  return { space, parent: page.data?.status === 'published' ? page.data.id : null };
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
        <button
          type="button"
          className="icon sidebar-toggle"
          aria-label={t('space.hideSidebar')}
          title={t('space.hideSidebar')}
          onClick={onHide}
        >
          <IconChevronLeft size={18} />
        </button>
      </div>
      <div className="sidebar-section">
        {t('space.pages')}
        {canCreate && (
          <button
            type="button"
            className="node-add always"
            aria-label={t('space.newPage')}
            title={t('space.newPage')}
            onClick={() => setAdding({ parent })}
          >
            <IconPlus size={16} />
          </button>
        )}
      </div>
      {tree.isPending && <Loading />}
      <ErrorNote error={tree.error} />
      {tree.data && (
        <PageTree
          key={space.id}
          spaceId={space.id}
          nodes={tree.data.pages}
          current={current}
          canMove={canCreate}
          onAdd={canCreate ? (id) => setAdding({ parent: id }) : undefined}
        />
      )}
      {space.can?.['space.administer'] && (
        <nav className="sidebar-foot" aria-label={t('space.settings')}>
          <Link href={`/spaces/${space.key}/settings`} className="side-link">
            <IconSliders size={18} /> {t('space.settings')}
          </Link>
          <Link href={`/spaces/${space.key}/trash`} className="side-link">
            <IconTrash size={18} /> {t('space.trash')}
          </Link>
        </nav>
      )}
      {adding && <NewPageDialog space={space} parent={adding.parent} onClose={() => setAdding(null)} />}
    </aside>
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
  return (
    <div className={sidebarVisible ? 'split' : 'split collapsed'}>
      {sidebarVisible && (
        <Sidebar
          space={space}
          current={current}
          parent={parent ?? null}
          onHide={() => setSidebarVisible(false)}
        />
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
            <IconChevronRight size={18} />
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
