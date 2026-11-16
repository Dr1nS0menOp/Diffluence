// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { Notification } from '../api';
import { sidewaysShift, submenuPlace } from './menu';
import { byDay, contentKind } from './notifications';
import { accessOf, RESTRICTION_STATES, stateOf } from './restriction-state';

const WIDE = { width: 1280, height: 800 };
const PHONE = { width: 360, height: 640 };
const SUBMENU = { width: 220, height: 120 };

function entry(left: number, right: number, top = 100) {
  return { left, right, top, bottom: top + 32 };
}

describe('where a sub-menu opens', () => {
  it('opens beside its entry on the end side when there is room', () => {
    expect(submenuPlace(entry(700, 940), SUBMENU, WIDE, false)).toEqual({ top: 100, left: 944 });
  });

  it('opens on the start side when the end side has no room', () => {
    expect(submenuPlace(entry(1000, 1270), SUBMENU, WIDE, false).left).toBe(776);
  });

  it('prefers the start side, which is the left, in right-to-left', () => {
    expect(submenuPlace(entry(300, 540), SUBMENU, WIDE, true).left).toBe(76);
  });

  it('opens over the menu, lower and further in, on a phone, and stays on the screen', () => {
    const place = submenuPlace(entry(110, 350), SUBMENU, PHONE, false);
    expect(place.top).toBe(132);
    expect(place.left).toBe(134);
    expect(place.left + SUBMENU.width).toBeLessThanOrEqual(PHONE.width);
  });

  it('moves up rather than run past the bottom of the screen', () => {
    expect(submenuPlace(entry(900, 1140, 760), SUBMENU, WIDE, false).top).toBe(676);
  });
});

describe('a menu on a narrow screen', () => {
  it('stays where it is when it fits', () => {
    expect(sidewaysShift(100, 320, PHONE.width)).toBe(0);
  });

  it('moves back from the end edge, as in right-to-left on a phone', () => {
    expect(sidewaysShift(183, 423, PHONE.width)).toBe(-67);
  });

  it('moves back from the start edge', () => {
    expect(sidewaysShift(-30, 210, PHONE.width)).toBe(34);
  });

  it('keeps its start inside when it is wider than the window', () => {
    expect(sidewaysShift(0, 400, PHONE.width)).toBe(4);
  });
});

describe('the access row of Share', () => {
  it('turns every state into a reach and an ability and back', () => {
    for (const state of RESTRICTION_STATES) {
      const { reach, ability } = accessOf(state);
      expect(stateOf(reach, ability)).toBe(state);
    }
  });

  it('makes a page private whatever the ability, once only the people chosen reach it', () => {
    expect(stateOf('chosen', 'view')).toBe('private');
    expect(stateOf('chosen', 'edit')).toBe('private');
  });

  it('lets everyone in the space only view when the ability is view', () => {
    expect(stateOf('space', 'view')).toBe('view_only');
    expect(accessOf('open')).toEqual({ reach: 'space', ability: 'edit' });
  });
});

function notification(more: Partial<Notification>): Notification {
  return {
    id: 'n',
    type: 'page_changed',
    reason: 'watching',
    actors: ['Ada'],
    count: 1,
    page_id: 'p',
    page_title: 'Page',
    page_kind: 'page',
    space_id: 's',
    space_name: 'Space',
    comment_id: null,
    read: false,
    at: '2026-11-15T12:00:00Z',
    data: {},
    ...more,
  };
}

describe('the notification panel', () => {
  const now = new Date('2026-11-15T15:00:00Z');

  it('groups items per day, newest day first, keeping their order', () => {
    const groups = byDay(
      [
        notification({ id: 'a', at: '2026-11-15T14:00:00Z' }),
        notification({ id: 'b', at: '2026-11-15T09:00:00Z' }),
        notification({ id: 'c', at: '2026-11-14T12:00:00Z' }),
        notification({ id: 'd', at: '2026-11-02T12:00:00Z' }),
      ],
      now,
    );
    expect(groups.map((g) => g.day)).toEqual(['today', 'yesterday', 'earlier']);
    expect(groups.map((g) => g.items.map((n) => n.id))).toEqual([['a', 'b'], ['c'], ['d']]);
    expect(groups[2]?.at).toBe('2026-11-02T12:00:00Z');
  });

  it('gives two earlier days a heading each', () => {
    const groups = byDay(
      [
        notification({ id: 'a', at: '2026-11-10T12:00:00Z' }),
        notification({ id: 'b', at: '2026-11-09T12:00:00Z' }),
      ],
      now,
    );
    expect(groups.map((g) => g.items.map((n) => n.id))).toEqual([['a'], ['b']]);
  });

  it('names the kind of content: a comment, a task, a blog post or a page', () => {
    expect(contentKind(notification({ type: 'reply', comment_id: 'c' }))).toBe('comment');
    expect(contentKind(notification({ type: 'task_due' }))).toBe('task');
    expect(contentKind(notification({ page_kind: 'post' }))).toBe('post');
    expect(contentKind(notification({}))).toBe('page');
  });
});
