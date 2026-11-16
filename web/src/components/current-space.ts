// SPDX-License-Identifier: Apache-2.0
// Which space, and which page in it, the current screen shows. The top bar, the frame and the
// panels of the sidebar all ask, so it lives apart from any of them.
import { useQuery } from '@tanstack/react-query';
import { api, type PageDetail, type Space } from '../api';
import { mayCreateAnything } from '../create-permission';
import { match, useLocation } from '../router';
import type { CreateTarget } from './create';

const PAGE_ROUTES = ['/pages/:id', '/pages/:id/history', '/pages/:id/compare'];
const SPACE_ROUTES = [
  '/spaces/:key/settings',
  '/spaces/:key/trash',
  '/spaces/:key/overview',
  '/spaces/:key/files',
  '/spaces/:key/access',
  '/spaces/:key/people',
  '/spaces/:key/audit',
  '/spaces/:key/content',
  '/spaces/:key/blog',
  '/spaces/:key/archive',
  '/spaces/:key/labels',
  '/spaces/:key/templates',
  '/spaces/:key/release',
];

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

/** Where the Create button puts a new item: below what you are reading, if you may make any kind there. */
export function useCreateTarget(): CreateTarget | null {
  const { space, page } = useCurrentSpace();
  if (!space || !mayCreateAnything(space.can)) return null;
  return { space, parent: page?.status === 'published' ? page.id : null };
}
