// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import en from './locales/en.json';
import { groupPermissions, permissionGroups, permissionKey, type PermissionScope } from './permission-words';

// Every permission the words name (a Go test keeps those complete) sits in exactly one group.
const named = Object.keys(en.permissions)
  .filter((k) => k.endsWith('_help'))
  .map((k) => k.slice(0, -'_help'.length));

describe('permission groups', () => {
  it('hold every permission that has words, once', () => {
    const keys = (['space', 'instance'] as PermissionScope[]).flatMap((scope) =>
      permissionGroups(scope).flatMap(([, list]) => list.map((p) => permissionKey(scope, p))),
    );
    expect([...keys].sort()).toEqual([...named].sort());
  });
  it('show a permission they do not know yet in the last group', () => {
    const groups = groupPermissions('space', ['space.view', 'space.new_thing']);
    expect(groups.map((g) => g.group)).toEqual(['pages', 'space']);
    expect(groups.at(-1)?.permissions).toEqual(['space.new_thing']);
  });
});
