// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { Comment, CommentAnchor, CommentPage, Notification } from '../api';
import { taskGroup } from '../views/tasks';
import { findQuote } from './comments/anchor';
import { threadsFor } from './comments/section';
import { nextThread } from './editor/outline';
import { dueState } from './editor/people';
import { bellNumber, notificationLink } from './notifications';

const TODAY = new Date('2026-11-05T12:00:00Z');

function quote(exact: string, more: Partial<CommentAnchor> = {}): CommentAnchor {
  return { kind: 'text', exact, ...more };
}

describe('finding a quote again', () => {
  it('finds nothing when the words are gone', () => {
    expect(findQuote('a page about something else', quote('the budget'))).toBeNull();
  });

  it('finds nothing for an empty quote', () => {
    expect(findQuote('some text', quote(''))).toBeNull();
  });

  it('refuses to guess between places when nothing tells them apart', () => {
    const text = 'the plan and the plan';
    expect(findQuote(text, quote('the plan', { prefix: 'old ', suffix: ' new' }))).toBeNull();
  });

  it('finds the only place, also when the text around it changed', () => {
    const anchor = quote('the plan', { prefix: 'old ', suffix: ' new' });
    expect(findQuote('here is the plan today', anchor)).toEqual({ from: 8, to: 16 });
  });

  it('tells two places apart by the words around them', () => {
    const text = 'first the plan, second the plan.';
    const anchor = quote('the plan', { prefix: 'second ', suffix: '.' });
    expect(findQuote(text, anchor)).toEqual({ from: 23, to: 31 });
  });

  it('uses the offset when the words around them are the same', () => {
    const text = 'a the plan b a the plan b';
    const anchor = quote('the plan', { prefix: 'a ', suffix: ' b', offset: 15 });
    expect(findQuote(text, anchor)).toEqual({ from: 15, to: 23 });
  });

  it('points at nothing when the offset does not fit either place', () => {
    const text = 'a the plan b a the plan b';
    const anchor = quote('the plan', { prefix: 'a ', suffix: ' b', offset: 3 });
    expect(findQuote(text, anchor)).toBeNull();
  });
});

describe('which group a task belongs to', () => {
  it('puts a task without a date last', () => {
    expect(taskGroup(null, TODAY)).toBe('none');
  });

  it('calls yesterday overdue', () => {
    expect(taskGroup('2026-11-04', TODAY)).toBe('overdue');
  });

  it('keeps today apart', () => {
    expect(taskGroup('2026-11-05', TODAY)).toBe('today');
  });

  it('counts the seventh day as this week and the eighth as later', () => {
    expect(taskGroup('2026-11-12', TODAY)).toBe('week');
    expect(taskGroup('2026-11-13', TODAY)).toBe('later');
  });
});

describe('how near a date is', () => {
  it('knows what is past, near and far', () => {
    expect(dueState('2026-11-04', TODAY)).toBe('overdue');
    expect(dueState('2026-11-05', TODAY)).toBe('soon');
    expect(dueState('2026-11-12', TODAY)).toBe('soon');
    expect(dueState('2026-11-13', TODAY)).toBe('later');
  });
});

describe('moving between threads', () => {
  it('has nowhere to go without threads', () => {
    expect(nextThread([], null, 1)).toBeNull();
  });

  it('starts at the first going forward and the last going back', () => {
    expect(nextThread(['a', 'b'], null, 1)).toBe('a');
    expect(nextThread(['a', 'b'], null, -1)).toBe('b');
  });

  it('goes round at both ends', () => {
    expect(nextThread(['a', 'b'], 'b', 1)).toBe('a');
    expect(nextThread(['a', 'b'], 'a', -1)).toBe('b');
  });

  it('starts over when the current thread went away', () => {
    expect(nextThread(['a', 'b'], 'gone', 1)).toBe('a');
  });
});

describe('the bell', () => {
  it('shows the number up to its cap', () => {
    expect(bellNumber(1)).toBe('1');
    expect(bellNumber(99)).toBe('99');
    expect(bellNumber(100)).toBe('99+');
  });

  const notification = (more: Partial<Notification>) =>
    ({ type: 'comment', page_id: 'p', comment_id: null, ...more }) as Notification;

  it('sends a request for access to the place to answer it', () => {
    expect(notificationLink(notification({ type: 'access_request' }))).toBe('/pages/p?share=1');
  });

  it('goes to the exact comment, or else the page', () => {
    expect(notificationLink(notification({ comment_id: 'c' }))).toBe('/pages/p#comment-c');
    expect(notificationLink(notification({}))).toBe('/pages/p');
  });
});

describe('the tabs of the comments panel', () => {
  const comment = (id: string, more: Partial<Comment> = {}) =>
    ({ id, status: 'open', unread: false, replies: [], ...more }) as Comment;
  const page = {
    inline: [
      comment('open'),
      comment('resolved', { status: 'resolved' }),
      comment('inline-reply', { replies: [comment('r', { unread: true })] }),
    ],
    threads: [comment('under', { unread: true })],
  } as CommentPage;
  const ids = (tab: Parameters<typeof threadsFor>[0]) => threadsFor(tab, page).map((c) => c.id);

  it('leaves threads under the page out of open and resolved', () => {
    expect(ids('open')).toEqual(['open', 'inline-reply']);
    expect(ids('resolved')).toEqual(['resolved']);
  });

  it('counts a thread as unread when only a reply is new', () => {
    expect(ids('unread')).toEqual(['inline-reply', 'under']);
  });

  it('shows the threads under the page on their own tab', () => {
    expect(ids('page')).toEqual(['under']);
  });
});
