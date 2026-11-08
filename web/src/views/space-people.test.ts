// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { Holder } from '../api';
import { inTab } from './space-people';

const holder = (change: Partial<Holder>): Holder => ({
  subject: { type: 'user', id: 'u1' },
  name: 'Anna',
  role: 'editor',
  source: 'direct',
  via: '',
  guest: false,
  ...change,
});

describe('the tabs of the People screen', () => {
  it('shows everyone under Everyone', () => {
    expect(inTab(holder({}), 'everyone')).toBe(true);
  });

  it('shows only space administrators under Administrators', () => {
    expect(inTab(holder({ role: 'space_admin' }), 'admins')).toBe(true);
    expect(inTab(holder({}), 'admins')).toBe(false);
  });

  it('shows groups under Groups, and guests and all guests under Guests', () => {
    expect(inTab(holder({ subject: { type: 'group', id: 'g1' } }), 'groups')).toBe(true);
    expect(inTab(holder({}), 'groups')).toBe(false);
    expect(inTab(holder({ guest: true }), 'guests')).toBe(true);
    expect(inTab(holder({ subject: { type: 'all_guests', id: '' } }), 'guests')).toBe(true);
    expect(inTab(holder({}), 'guests')).toBe(false);
  });
});
