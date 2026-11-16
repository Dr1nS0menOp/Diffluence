// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import {
  addValue,
  contentTabAddress,
  emptyWays,
  highlight,
  removeValue,
  suggestionAddress,
  taskParamsFor,
  withValue,
} from './search';

describe('a similar spelling (ORG-023)', () => {
  it('replaces the plain words and keeps the filters', () => {
    expect(suggestionAddress(new URLSearchParams('q=handbok label:nowhere&kind=page'), 'handbook')).toBe(
      '/search?q=handbook+label%3Anowhere&kind=page',
    );
  });
});

describe('highlight', () => {
  it('splits matches from plain text', () => {
    expect(highlight('a [[[b]]] c')).toEqual([
      { text: 'a ', hit: false },
      { text: 'b', hit: true },
      { text: ' c', hit: false },
    ]);
  });
  it('keeps markup as text', () => {
    expect(highlight('<script>[[[x]]]')).toEqual([
      { text: '<script>', hit: false },
      { text: 'x', hit: true },
    ]);
  });
});

describe('the filters in the address', () => {
  it('sets one value and takes it away again', () => {
    const address = new URLSearchParams('q=plan&below=a');
    expect(withValue(address, 'below', 'b').toString()).toBe('q=plan&below=b');
    expect(withValue(address, 'below', '').toString()).toBe('q=plan');
    expect(address.toString()).toBe('q=plan&below=a');
  });
  it('keeps several people, each once (ORG-017)', () => {
    const one = addValue(new URLSearchParams('q=plan'), 'contributor', 'u1');
    const two = addValue(one, 'contributor', 'u2');
    expect(two.getAll('contributor')).toEqual(['u1', 'u2']);
    expect(addValue(two, 'contributor', 'u1').getAll('contributor')).toEqual(['u1', 'u2']);
    expect(removeValue(two, 'contributor', 'u1').getAll('contributor')).toEqual(['u2']);
    expect(removeValue(two, 'contributor', 'u1').get('q')).toBe('plan');
  });
});

describe('the ways forward of a search that found nothing (NAV-175)', () => {
  it('offers the same words without filters, and archived pages', () => {
    const ways = emptyWays(new URLSearchParams('q=plan label:hr space:ENG&kind=post&below=p&within=week'));
    expect(ways).toEqual([
      { kind: 'filters', href: '/search?q=plan' },
      {
        kind: 'archived',
        href: '/search?q=plan+label%3Ahr+space%3AENG&kind=post&below=p&within=week&archived=1',
      },
    ]);
  });
  it('offers nothing it would not change, and keeps archived pages and the order', () => {
    expect(emptyWays(new URLSearchParams('q=plan author:anna&archived=1&sort=date'))).toEqual([
      { kind: 'filters', href: '/search?q=plan&archived=1&sort=date' },
    ]);
    expect(emptyWays(new URLSearchParams('q=plan&sort=date'))).toEqual([
      { kind: 'archived', href: '/search?q=plan&sort=date&archived=1' },
    ]);
    expect(emptyWays(new URLSearchParams('q=plan&archived=1&tab=content'))).toEqual([]);
    expect(emptyWays(new URLSearchParams('q=label:hr&archived=1'))).toEqual([]);
  });
});

describe('the other tabs of the results (spec 086)', () => {
  it('ask for tasks with the words, the space and archived pages, and no page filters', () => {
    const address = new URLSearchParams('q=boat&tab=tasks&space_id=s1&archived=1&label=x&kind=page&titles=1');
    expect(taskParamsFor(address).toString()).toBe('q=boat&space_id=s1&archived=1&kind=task');
  });
  it('leave out what is empty', () => {
    expect(taskParamsFor(new URLSearchParams('q=boat&tab=tasks&space_id=')).toString()).toBe(
      'q=boat&kind=task',
    );
  });
  it('offer the content tab with the same words as a way forward', () => {
    expect(contentTabAddress(new URLSearchParams('q=boat&tab=people&archived=1'))).toBe(
      '/search?q=boat&archived=1',
    );
  });
});
