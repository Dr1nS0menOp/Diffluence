// SPDX-License-Identifier: Apache-2.0
// What the page tree works out, without any screen: branches, filters, order.
import type { TreeNode } from '../api';

/** Branches this close to the top start open, so the shape of a space shows at once. */
const OPEN_DEPTH = 2;

export interface Branch extends TreeNode {
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
  // The home page is above the tree, so its children are top rows beside pages
  // without a parent. Each group numbers its own positions, so they cannot mix.
  return [...roots.filter((b) => b.parent_id), ...roots.filter((b) => !b.parent_id)];
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

/** Every branch closed, whatever was open before. */
export function collapseEverything(nodes: TreeNode[]): BranchMemory {
  const parents = new Set(
    nodes.flatMap((n) => [...(n.parent_id ? [n.parent_id] : []), ...(n.has_children ? [n.id] : [])]),
  );
  return { opened: [], collapsed: [...parents] };
}

/**
 * A branch can only start open when its rows came along. A branch the reader opened, or
 * the way to the current page, opens anyway, and its rows follow.
 */
export function showsBranch(
  memory: BranchMemory,
  branch: Branch,
  depth: number,
  ancestors: Set<string>,
): boolean {
  if (!isBranchOpen(memory, branch.id, depth, ancestors)) return false;
  return branch.children.length > 0 || memory.opened.includes(branch.id) || ancestors.has(branch.id);
}

export interface TitlePart {
  text: string;
  hit: boolean;
}

/** Splits a title around what was typed, to mark the matching letters. */
export function matchParts(title: string, text: string): TitlePart[] {
  const wanted = text.trim().toLowerCase();
  if (!wanted) return [{ text: title, hit: false }];
  const parts: TitlePart[] = [];
  const lower = title.toLowerCase();
  let from = 0;
  for (let at = lower.indexOf(wanted); at >= 0; at = lower.indexOf(wanted, from)) {
    if (at > from) parts.push({ text: title.slice(from, at), hit: false });
    parts.push({ text: title.slice(at, at + wanted.length), hit: true });
    from = at + wanted.length;
  }
  if (from < title.length) parts.push({ text: title.slice(from), hit: false });
  return parts;
}

export type TreeView = 'tree' | 'visited' | 'updated' | 'alphabet';

/** Lets a dragged row reach the Spaces menu in the top bar. */
export const PAGE_DRAG_TYPE = 'application/x-diffluence-page';

/** Ctrl, Space and a click on a checkbox add or take away one row. */
export function toggleRow(picked: string[], id: string): string[] {
  return picked.includes(id) ? picked.filter((x) => x !== id) : [...picked, id];
}

/** Shift adds every row between the last one touched and this one. */
export function rangeOfRows(order: string[], picked: string[], anchor: string, id: string): string[] {
  const from = order.indexOf(anchor);
  const to = order.indexOf(id);
  if (from < 0 || to < 0) return toggleRow(picked, id);
  const between = order.slice(Math.min(from, to), Math.max(from, to) + 1);
  return [...new Set([...picked, ...between])];
}

/** The next row after `from` whose title starts with what was typed, going round. */
export function typeAheadRow(titles: string[], from: number, typed: string): number {
  const wanted = typed.toLowerCase();
  for (let step = 1; step <= titles.length; step++) {
    const at = (from + step) % titles.length;
    if (titles[at]?.toLowerCase().startsWith(wanted)) return at;
  }
  return -1;
}

/** How many pages stand below a page, at any depth. */
export function countBelow(nodes: { id: string; parent_id: string | null }[], id: string): number {
  const children = nodes.filter((n) => n.parent_id === id);
  return children.reduce((sum, child) => sum + 1 + countBelow(nodes, child.id), 0);
}
