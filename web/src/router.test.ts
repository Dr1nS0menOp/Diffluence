// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { match } from './router';

describe('match', () => {
  it('reads named parts', () => {
    expect(match('/pages/:id/edit', '/pages/abc/edit')).toEqual({ id: 'abc' });
  });
  it('ignores a trailing slash', () => {
    expect(match('/search', '/search/')).toEqual({});
  });
  it('refuses another shape', () => {
    expect(match('/pages/:id', '/pages/abc/edit')).toBeNull();
    expect(match('/pages/:id', '/spaces/abc')).toBeNull();
  });
  it('decodes values', () => {
    expect(match('/spaces/:key', '/spaces/a%20b')).toEqual({ key: 'a b' });
  });
});
