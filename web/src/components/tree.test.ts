// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import {
  ancestorsOf,
  buildTree,
  collapseBranch,
  isBranchOpen,
  openBranch,
  readBranchMemory,
  revealPage,
} from './tree-model';

const node = (id: string, parent_id: string | null, position: number) => ({
  id,
  parent_id,
  position,
  emoji: '',
  restricted: false,
  has_draft: false,
  updated_at: '2026-01-01T00:00:00Z',
  updated_by_name: '',
  visited_at: null,
  kind: 'page',
  link_target: '',
  owner_name: '',
  stage_text: '',
  stage_color: '',
  title: id,
  status: 'published',
});

describe('buildTree', () => {
  it('nests and orders', () => {
    const tree = buildTree([node('b', null, 1), node('a', null, 0), node('c', 'a', 0)]);
    expect(tree.map((n) => n.id)).toEqual(['a', 'b']);
    expect(tree[0]?.children.map((n) => n.id)).toEqual(['c']);
  });
  it('keeps a page whose parent is hidden at the top level', () => {
    const tree = buildTree([node('x', 'hidden', 0)]);
    expect(tree.map((n) => n.id)).toEqual(['x']);
  });
});

describe('branches', () => {
  const nothing = { opened: [], collapsed: [] };
  const noAncestors = new Set<string>();

  it('start folded from the first level, so the top pages of a space show at once', () => {
    expect(isBranchOpen(nothing, 'a', 0, noAncestors)).toBe(false);
    expect(isBranchOpen(nothing, 'a', 1, noAncestors)).toBe(false);
  });
  it('open deep down on the way to the current page', () => {
    expect(isBranchOpen(nothing, 'a', 5, new Set(['a']))).toBe(true);
  });
  it('stay collapsed when the reader collapsed them, also on the way to the page', () => {
    const memory = collapseBranch(nothing, 'a');
    expect(isBranchOpen(memory, 'a', 0, new Set(['a']))).toBe(false);
  });
  it('remember being opened again', () => {
    const memory = openBranch(collapseBranch(nothing, 'a'), 'a');
    expect(memory).toEqual({ opened: ['a'], collapsed: [] });
    expect(isBranchOpen(memory, 'a', 9, noAncestors)).toBe(true);
  });
  it('open up when going to a page inside them, and leave other branches alone', () => {
    const memory = collapseBranch(collapseBranch(nothing, 'a'), 'b');
    expect(revealPage(memory, new Set(['a'])).collapsed).toEqual(['b']);
  });
  it('survive stored nonsense', () => {
    expect(readBranchMemory('not json')).toEqual(nothing);
    expect(readBranchMemory('{"opened":1}')).toEqual(nothing);
    expect(readBranchMemory(null)).toEqual(nothing);
    expect(readBranchMemory('{"opened":["a"],"collapsed":["b"]}')).toEqual({
      opened: ['a'],
      collapsed: ['b'],
    });
  });
});

describe('ancestorsOf', () => {
  it('walks up to the root', () => {
    const nodes = [node('a', null, 0), node('b', 'a', 0), node('c', 'b', 0)];
    expect([...ancestorsOf(nodes, 'c')].sort()).toEqual(['a', 'b']);
  });
  it('ends on a loop instead of hanging', () => {
    const nodes = [node('a', 'b', 0), node('b', 'a', 0)];
    expect([...ancestorsOf(nodes, 'a')].sort()).toEqual(['a', 'b']);
  });
});
