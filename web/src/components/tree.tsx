// SPDX-License-Identifier: Apache-2.0
import { useMemo, useState, type DragEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api, type Page, type TreeNode } from '../api';
import { Link } from '../router';
import { ErrorNote } from './ui';

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

function ancestorsOf(nodes: TreeNode[], id: string | undefined): Set<string> {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const out = new Set<string>();
  let cur = id ? byId.get(id) : undefined;
  while (cur?.parent_id) {
    out.add(cur.parent_id);
    cur = byId.get(cur.parent_id);
  }
  return out;
}

export function PageTree({
  spaceId,
  nodes,
  current,
  canMove,
}: {
  spaceId: string;
  nodes: TreeNode[];
  current?: string;
  canMove: boolean;
}) {
  const { t } = useTranslation();
  const client = useQueryClient();
  const tree = useMemo(() => buildTree(nodes), [nodes]);
  const auto = useMemo(() => ancestorsOf(nodes, current), [nodes, current]);
  const [open, setOpen] = useState<Set<string>>(new Set());
  const [closed, setClosed] = useState<Set<string>>(new Set());
  const [dragging, setDragging] = useState<string | null>(null);
  const [target, setTarget] = useState<string | null>(null);

  const move = useMutation({
    mutationFn: (v: { id: string; parent_id: string | null; before_id: string | null }) =>
      api.post<Page>(`/pages/${v.id}/move`, { parent_id: v.parent_id, before_id: v.before_id }),
    onSettled: () => client.invalidateQueries({ queryKey: ['tree', spaceId] }),
  });

  const isOpen = (id: string) => !closed.has(id) && (open.has(id) || auto.has(id));
  const toggle = (id: string) => {
    if (isOpen(id)) {
      setClosed((s) => new Set(s).add(id));
    } else {
      setClosed((s) => {
        const n = new Set(s);
        n.delete(id);
        return n;
      });
      setOpen((s) => new Set(s).add(id));
    }
  };

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

  const render = (list: Branch[], parent: string | null) => (
    <ul role={parent ? 'group' : 'tree'} aria-label={parent ? undefined : t('space.pages')}>
      {list.map((b) => {
        const expanded = isOpen(b.id);
        const hasKids = b.children.length > 0;
        return (
          <li
            key={b.id}
            role="treeitem"
            aria-expanded={hasKids ? expanded : undefined}
            aria-selected={b.id === current}
          >
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
              {hasKids ? (
                <button
                  type="button"
                  className="twist"
                  aria-label={b.title}
                  aria-expanded={expanded}
                  onClick={() => toggle(b.id)}
                >
                  {expanded ? '▾' : '▸'}
                </button>
              ) : (
                <span className="twist" />
              )}
              <Link href={`/pages/${b.id}`} draggable={false}>
                {b.title}
              </Link>
            </div>
            {hasKids && expanded && render(b.children, b.id)}
          </li>
        );
      })}
    </ul>
  );

  if (tree.length === 0) return <p className="muted">{t('space.emptyTree')}</p>;
  return (
    <nav className="tree" aria-label={t('space.pages')}>
      {render(tree, null)}
      {move.error && <ErrorNote error={move.error} />}
    </nav>
  );
}
