// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { withoutRole, withRole } from './access-roles';

describe('the roles someone holds in a space (spec 049)', () => {
  it('adds a role and keeps the ones held', () => {
    expect(withRole(['viewer'], 'editor')).toEqual(['viewer', 'editor']);
  });

  it('adds a role that is already held only once', () => {
    expect(withRole(['viewer', 'editor'], 'editor')).toEqual(['viewer', 'editor']);
  });

  it('takes one role away and keeps the others', () => {
    expect(withoutRole(['viewer', 'editor', 'reviewer'], 'editor')).toEqual(['viewer', 'reviewer']);
  });

  it('takes the last role away, which removes access', () => {
    expect(withoutRole(['viewer'], 'viewer')).toEqual([]);
  });
});
