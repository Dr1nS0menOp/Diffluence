// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { buildTree } from './tree';

const node = (id: string, parent_id: string | null, position: number) => ({
  id,
  parent_id,
  position,
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
