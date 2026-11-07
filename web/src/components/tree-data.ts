// SPDX-License-Identifier: Apache-2.0
// The parts of the tree that the browser holds. A large space never comes whole: the
// start, the way to the open page, and the branches the reader opened (SPC-031).
import { useCallback, useMemo, useState } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import { api, type TreeNode, type TreeParts } from '../api';

/** The branches that one sidebar keeps asking for, newest first. */
const MAX_OPENED_BRANCHES = 40;

export interface MoreRows {
  parent: string | null;
  offset: number;
}

/** A level that goes on, with a place the reader has not asked for yet. */
export function levelsThatGoOn(parts: (TreeParts | undefined)[]): MoreRows[] {
  const next = new Map<string | null, number>();
  for (const part of parts) {
    for (const more of part?.more ?? []) {
      next.set(more.parent_id, Math.max(next.get(more.parent_id) ?? 0, more.offset));
    }
  }
  return [...next].map(([parent, offset]) => ({ parent, offset }));
}

/** All rows of the parts, each once. A later part knows better. */
export function mergeParts(parts: (TreeParts | undefined)[]): TreeNode[] {
  const rows = new Map<string, TreeNode>();
  for (const part of parts) {
    for (const node of part?.pages ?? []) rows.set(node.id, node);
  }
  return [...rows.values()];
}

/** The branches that still have to be asked for: the start holds the others. */
export function branchesToLoad(opened: string[], start: TreeParts | undefined): string[] {
  const loaded = new Set((start?.pages ?? []).flatMap((n) => (n.parent_id ? [n.parent_id] : [])));
  return opened.filter((id) => !loaded.has(id)).slice(-MAX_OPENED_BRANCHES);
}

const address = (spaceId: string, query: string) => `/spaces/${spaceId}/tree/branches${query}`;

export function useTreeParts(spaceId: string, current: string | undefined, opened: string[]) {
  const [asked, setAsked] = useState<MoreRows[]>([]);
  const start = useQuery({
    queryKey: ['tree', spaceId, 'start'],
    queryFn: () => api.get<TreeParts>(address(spaceId, '')),
  });
  const known = useMemo(() => new Set(start.data?.pages.map((n) => n.id)), [start.data]);
  const around = useQuery({
    queryKey: ['tree', spaceId, 'around', current],
    queryFn: () => api.get<TreeParts>(address(spaceId, `?around=${current ?? ''}`)),
    enabled: start.isSuccess && current !== undefined && !known.has(current),
  });
  const branches = useQueries({
    queries: branchesToLoad(opened, start.data).map((id) => ({
      queryKey: ['tree', spaceId, 'branch', id],
      queryFn: () => api.get<TreeParts>(address(spaceId, `?parent=${id}`)),
    })),
    combine: (results) => results.map((r) => r.data),
  });
  const rest = useQueries({
    queries: asked.map(({ parent, offset }) => ({
      queryKey: ['tree', spaceId, 'more', parent, offset],
      queryFn: () =>
        api.get<TreeParts>(address(spaceId, `?offset=${offset}${parent ? `&parent=${parent}` : ''}`)),
    })),
    combine: (results) => results.map((r) => r.data),
  });
  const nodes = useMemo(
    () => mergeParts([start.data, ...branches, around.data, ...rest]),
    [start.data, branches, around.data, rest],
  );
  const more = useMemo(() => {
    const reached = new Map(asked.map((a) => [a.parent, a.offset]));
    return levelsThatGoOn([start.data, ...branches, ...rest]).filter(
      (m) => (reached.get(m.parent) ?? -1) < m.offset,
    );
  }, [start.data, branches, rest, asked]);
  const showMore = useCallback((rows: MoreRows) => setAsked((list) => [...list, rows]), []);
  return { nodes, more, showMore, isPending: start.isPending, error: start.error };
}
