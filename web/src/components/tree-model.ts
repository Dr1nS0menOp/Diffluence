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
  const parents = new Set(nodes.flatMap((n) => (n.parent_id ? [n.parent_id] : [])));
  return { opened: [], collapsed: [...parents] };
}

/** The pages that match, and their parents so a match keeps its place in the tree. */
export function filterTree(nodes: TreeNode[], text: string): Set<string> {
  const wanted = text.trim().toLowerCase();
  const kept = new Set<string>();
  for (const n of nodes) {
    if (!n.title.toLowerCase().includes(wanted)) continue;
    kept.add(n.id);
    ancestorsOf(nodes, n.id).forEach((id) => kept.add(id));
  }
  return kept;
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

const newest = (a: string | null, b: string | null) => (b ?? '').localeCompare(a ?? '');

/** The flat views: a list of pages in another order. Unvisited pages stay out of "visited". */
export function flatPages(nodes: TreeNode[], view: TreeView): TreeNode[] {
  if (view === 'visited') {
    return nodes.filter((n) => n.visited_at).sort((a, b) => newest(a.visited_at, b.visited_at));
  }
  if (view === 'updated') return [...nodes].sort((a, b) => newest(a.updated_at, b.updated_at));
  return [...nodes].sort((a, b) => a.title.localeCompare(b.title));
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
