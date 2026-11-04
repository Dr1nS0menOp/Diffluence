// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useState, type DragEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Page, type TreeNode } from '../api';
import { Link } from '../router';
import { IconChevronDown, IconChevronRight, IconPage, IconPlus } from './icons';
import { ErrorNote } from './ui';

/** Branches this close to the top start open, so the shape of a space shows at once. */
const OPEN_DEPTH = 2;

interface Branch extends TreeNode {
  children: Branch[];
}

export function buildTree(nodes: TreeNode[]): Branch[] {
  const byId = new Map<string, Branch>(nodes.map((n) => [n.id, { ...n, children: [] }]));
  const roots: Branch[] = [];
  for (const b of byId.values()) {
    const parent = b.parent_id ? byId.get(b.parent_id) : undefined;
    if (parent) parent.children.push(b);
    else roots.push(b);
  }
  const order = (list: Branch[]) => {
    list.sort((a, b) => a.position - b.position);
    list.forEach((b) => order(b.children));
  };
  order(roots);
  return roots;
}

export function ancestorsOf(nodes: TreeNode[], id: string | undefined): Set<string> {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const out = new Set<string>();
  let cur = id ? byId.get(id) : undefined;
  while (cur?.parent_id && !out.has(cur.parent_id)) {
    out.add(cur.parent_id);
    cur = byId.get(cur.parent_id);
  }
  return out;
}

/** What the reader did by hand. Everything else follows the defaults. */
export interface BranchMemory {
  opened: string[];
  collapsed: string[];
}

export function readBranchMemory(stored: string | null): BranchMemory {
  try {
    const value = JSON.parse(stored ?? '') as Partial<BranchMemory>;
    if (Array.isArray(value.opened) && Array.isArray(value.collapsed)) {
      return { opened: value.opened, collapsed: value.collapsed };
    }
  } catch {
    // Nothing stored, or something unreadable: the defaults are a fine answer.
  }
  return { opened: [], collapsed: [] };
}

export function isBranchOpen(
  memory: BranchMemory,
  id: string,
  depth: number,
  ancestors: Set<string>,
): boolean {
  if (memory.collapsed.includes(id)) return false;
  return memory.opened.includes(id) || ancestors.has(id) || depth < OPEN_DEPTH;
}

export function openBranch(memory: BranchMemory, id: string): BranchMemory {
  return {
    opened: [...memory.opened.filter((x) => x !== id), id],
    collapsed: memory.collapsed.filter((x) => x !== id),
  };
}

export function collapseBranch(memory: BranchMemory, id: string): BranchMemory {
  return {
    opened: memory.opened.filter((x) => x !== id),
    collapsed: [...memory.collapsed.filter((x) => x !== id), id],
  };
}

export function revealPage(memory: BranchMemory, ancestors: Set<string>): BranchMemory {
  return { ...memory, collapsed: memory.collapsed.filter((id) => !ancestors.has(id)) };
}

/** Collapsed branches stay collapsed while the reader moves through the space. */
function useBranchMemory(spaceId: string): [BranchMemory, (next: BranchMemory) => void] {
  const key = `diffluence.tree.${spaceId}`;
  const [memory, setMemory] = useState(() => readBranchMemory(localStorage.getItem(key)));
  useEffect(() => localStorage.setItem(key, JSON.stringify(memory)), [key, memory]);
  return [memory, setMemory];
}

export function PageTree({
  spaceId,
  nodes,
  current,
  canMove,
  onAdd,
}: {
  spaceId: string;
  nodes: TreeNode[];
  current?: string;
  canMove: boolean;
  onAdd?: (parent: string) => void;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const tree = useMemo(() => buildTree(nodes), [nodes]);
  const ancestors = useMemo(() => ancestorsOf(nodes, current), [nodes, current]);
  const [memory, setMemory] = useBranchMemory(spaceId);
  const [dragging, setDragging] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);

  // Going to a page reveals it, also in a collapsed branch. Only a new page does:
  // collapsing the branch you are in must stay possible.
  const [revealedPage, setRevealedPage] = useState<string | undefined>();
  if (current !== revealedPage) {
    setRevealedPage(current);
    setMemory(revealPage(memory, ancestors));
  }

  const move = useMutation({
    mutationFn: (v: { id: string; parent_id: string | null; before_id: string | null }) =>
      api.post<Page>(`/pages/${v.id}/move`, { parent_id: v.parent_id, before_id: v.before_id }),
    onSettled: () => client.invalidateQueries({ queryKey: ['tree', spaceId] }),
  });

  const allow = (e: DragEvent, key: string) => {
    if (!dragging) return;
    e.preventDefault();
    setTarget(key);
  };
  const finish = (e: DragEvent, parent: string | null, before: string | null) => {
    e.preventDefault();
    e.stopPropagation();
    const id = dragging;
    setDragging(null);
    setTarget(null);
    if (id && id !== parent && id !== before) move.mutate({ id, parent_id: parent, before_id: before });
  };

  const render = (list: Branch[], parent: string | null, depth: number) => (
    <ul>
      {list.map((b) => {
        const expanded = isBranchOpen(memory, b.id, depth, ancestors);
        const hasChildren = b.children.length > 0;
        return (
          <li key={b.id}>
            {canMove && (
              <div
                className={target === 'gap:' + b.id ? 'gap active' : 'gap'}
                data-testid={`gap-${b.title}`}
                onDragOver={(e) => allow(e, 'gap:' + b.id)}
                onDragLeave={() => setTarget(null)}
                onDrop={(e) => finish(e, parent, b.id)}
              />
            )}
            <div
              className={[
                'node',
                b.id === current ? 'current' : '',
                target === 'in:' + b.id ? 'drop' : '',
              ].join(' ')}
              draggable={canMove}
              onDragStart={(e) => {
                e.dataTransfer.effectAllowed = 'move';
                e.dataTransfer.setData('text/plain', b.title);
                setDragging(b.id);
              }}
              onDragEnd={() => {
                setDragging(null);
                setTarget(null);
              }}
              onDragOver={(e) => allow(e, 'in:' + b.id)}
              onDragLeave={() => setTarget(null)}
              onDrop={(e) => finish(e, b.id, null)}
            >
              {hasChildren ? (
                <button
                  type="button"
                  className="twist"
                  aria-label={b.title}
                  aria-expanded={expanded}
                  onClick={() => setMemory((expanded ? collapseBranch : openBranch)(memory, b.id))}
                >
                  {expanded ? <IconChevronDown size={16} /> : <IconChevronRight size={16} />}
                </button>
              ) : (
                <span className="twist">
                  <IconPage size={16} />
                </span>
              )}
              <Link
                href={`/pages/${b.id}`}
                draggable={false}
                aria-current={b.id === current ? 'page' : undefined}
              >
                {b.title}
              </Link>
              {onAdd && b.status === 'published' && (
                <button
                  type="button"
                  className="node-add"
                  aria-label={t('space.addChild', { title: b.title })}
                  title={t('space.addChild', { title: b.title })}
                  onClick={() => onAdd(b.id)}
                >
                  <IconPlus size={16} />
                </button>
              )}
            </div>
            {hasChildren && expanded && render(b.children, b.id, depth + 1)}
          </li>
        );
      })}
    </ul>
  );

  if (tree.length === 0) return <p className="muted">{t('space.emptyTree')}</p>;
  return (
    <nav className="tree" aria-label={t('space.pages')}>
      {render(tree, null, 0)}
      {move.error && <ErrorNote error={move.error} />}
    </nav>
  );
}
