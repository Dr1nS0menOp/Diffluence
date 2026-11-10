// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { createPermission, mayCreate, mayCreateAnything } from './create-permission';

describe('the permission that makes each kind', () => {
  it('asks for its own permission for posts and databases', () => {
    expect(createPermission('post')).toBe('post.create');
    expect(createPermission('database')).toBe('database.create');
  });

  it('makes pages, folders and links with page.create', () => {
    for (const kind of ['page', 'folder', 'link']) expect(createPermission(kind)).toBe('page.create');
  });

  it('offers nothing when the space says nothing', () => {
    expect(mayCreate(undefined, 'page')).toBe(false);
    expect(mayCreateAnything(undefined)).toBe(false);
    expect(mayCreateAnything({ 'page.edit': true })).toBe(false);
  });

  it('offers a post to someone who may only write posts', () => {
    const can = { 'post.create': true };
    expect(mayCreate(can, 'post')).toBe(true);
    expect(mayCreate(can, 'page')).toBe(false);
    expect(mayCreate(can, 'database')).toBe(false);
    expect(mayCreateAnything(can)).toBe(true);
  });

  it('offers a database to someone who may only make databases', () => {
    expect(mayCreateAnything({ 'database.create': true })).toBe(true);
  });
});
