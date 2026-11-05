// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { DiffBlock, QuickResults, Space, TreeNode } from '../api';
import { foldUnchanged } from '../views/history';
import { moved } from '../views/lists';
import { suggestKey } from './create';
import { contentsOf, countWords, headingsOf, readingMinutes, slidesOf, slugify } from './editor/outline';
import { addSearch, readSearches, rowsOf } from './quick-search';
import { orderSpaces, withCurrent } from './space-strip';
import { buildTree, collapseEverything, filterTree, flatPages, matchParts, typeAheadRow } from './tree-model';
import { relativeTime } from './ui';

const heading = (level: number, text: string) => ({
  type: 'heading',
  attrs: { level },
  content: [{ type: 'text', text }],
});
const paragraph = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
const doc = (...content: unknown[]) => ({ type: 'doc', content });

describe('the outline of a document', () => {
  it('makes a readable address, and keeps letters of any language', () => {
    expect(slugify('Your first day!')).toBe('your-first-day');
    expect(slugify('Überblick & Ziele')).toBe('überblick-ziele');
    expect(slugify('日本語')).toBe('日本語');
  });

  it('gives two headings with the same text their own address', () => {
    const slugs = headingsOf(doc(heading(2, 'Notes'), heading(2, 'Notes'), heading(2, 'Notes')));
    expect(slugs.map((h) => h.slug)).toEqual(['notes', 'notes-2', 'notes-3']);
  });

  it('gives a heading without letters an address too', () => {
    expect(headingsOf(doc(heading(2, '!!!')))[0]?.slug).toBe('section');
  });

  it('finds headings inside panels, in document order', () => {
    const panel = { type: 'panel', content: [heading(3, 'Inside')] };
    expect(headingsOf(doc(heading(2, 'Before'), panel)).map((h) => h.text)).toEqual(['Before', 'Inside']);
  });

  it('leaves the smallest headings out of the contents', () => {
    const found = contentsOf(doc(heading(1, 'a'), heading(3, 'b'), heading(4, 'c'), heading(6, 'd')));
    expect(found.map((h) => h.text)).toEqual(['a', 'b']);
  });

  it('counts words, and leaves out the read time of a short page', () => {
    expect(countWords(doc(paragraph('one two'), paragraph('  three  ')))).toBe(3);
    expect(readingMinutes(99)).toBeNull();
    expect(readingMinutes(100)).toBe(1);
    expect(readingMinutes(230)).toBe(1);
    expect(readingMinutes(231)).toBe(2);
  });

  it('splits slides at the highest heading level, and keeps what comes first', () => {
    const slides = slidesOf(
      doc(paragraph('intro'), heading(2, 'a'), heading(3, 'inside a'), heading(2, 'b'), paragraph('text')),
    );
    expect(slides).toHaveLength(3);
    expect(slides[0]).toEqual(doc(paragraph('intro')));
    expect(slides[1]).toEqual(doc(heading(2, 'a'), heading(3, 'inside a')));
  });

  it('shows a page without headings as one slide', () => {
    const page = doc(paragraph('only text'));
    expect(slidesOf(page)).toEqual([page]);
  });
});

const node = (id: string, parent: string | null, title: string, extra: Partial<TreeNode> = {}): TreeNode => ({
  id,
  parent_id: parent,
  position: 0,
  title,
  status: 'published',
  emoji: '',
  restricted: false,
  has_draft: false,
  updated_at: '2026-01-01T00:00:00Z',
  updated_by_name: '',
  visited_at: null,
  ...extra,
});

describe('the tree', () => {
  const nodes = [node('a', null, 'Handbook'), node('b', 'a', 'People'), node('c', 'b', 'Holiday plan')];

  it('keeps a match together with its parents', () => {
    expect([...filterTree(nodes, 'holiday')].sort()).toEqual(['a', 'b', 'c']);
    expect([...filterTree(nodes, 'people')].sort()).toEqual(['a', 'b']);
    expect(filterTree(nodes, 'nothing').size).toBe(0);
  });

  it('marks every matching part of a title, ignoring case', () => {
    expect(matchParts('Plan a plan', 'PLAN')).toEqual([
      { text: 'Plan', hit: true },
      { text: ' a ', hit: false },
      { text: 'plan', hit: true },
    ]);
    expect(matchParts('Title', ' ')).toEqual([{ text: 'Title', hit: false }]);
  });

  it('keeps the children of the home page first at the top, each group in its own order', () => {
    const home = 'hidden-home';
    const rows = [
      node('free-1', null, 'Free one', { position: 0 }),
      node('kid-2', home, 'Kid two', { position: 1 }),
      node('free-2', null, 'Free two', { position: 1 }),
      node('kid-1', home, 'Kid one', { position: 0 }),
    ];
    expect(buildTree(rows).map((b) => b.title)).toEqual(['Kid one', 'Kid two', 'Free one', 'Free two']);
  });

  it('closes every branch at once', () => {
    expect(collapseEverything(nodes).collapsed.sort()).toEqual(['a', 'b']);
  });

  it('lists only visited pages, newest first', () => {
    const list = [
      node('a', null, 'Old', { visited_at: '2026-01-01T00:00:00Z' }),
      node('b', null, 'Never'),
      node('c', null, 'New', { visited_at: '2026-02-01T00:00:00Z' }),
    ];
    expect(flatPages(list, 'visited').map((n) => n.title)).toEqual(['New', 'Old']);
  });

  it('sorts from A to Z without changing the list it was given', () => {
    const list = [node('a', null, 'Zebra'), node('b', null, 'Apple')];
    expect(flatPages(list, 'alphabet').map((n) => n.title)).toEqual(['Apple', 'Zebra']);
    expect(list[0]?.title).toBe('Zebra');
  });

  it('jumps to the next row that starts with the letters, and goes round', () => {
    const titles = ['Apple', 'Banana', 'Avocado'];
    expect(typeAheadRow(titles, 0, 'a')).toBe(2);
    expect(typeAheadRow(titles, 2, 'a')).toBe(0);
    expect(typeAheadRow(titles, 0, 'x')).toBe(-1);
  });
});

describe('quick search', () => {
  const labels = { pages: 'Pages', spaces: 'Spaces', files: 'Files', people: 'People' };

  it('puts the groups in order, so arrows follow what is on screen', () => {
    const results: QuickResults = {
      pages: [
        {
          page_id: 'p',
          space_key: 'S',
          space_name: 'Space',
          title: 'Page',
          snippet: '',
          updated_at: '2026-01-01T00:00:00Z',
        },
      ],
      spaces: [],
      files: [{ id: 'f', filename: 'a.pdf', page_id: 'p', page_title: 'Page', space_name: 'Space' }],
      people: [{ id: 'u', name: 'Anna' }],
    };
    const rows = rowsOf(results, labels);
    expect(rows.map((r) => r.group)).toEqual(['Pages', 'Files', 'People']);
    expect(rows[1]?.target).toBe('/pages/p');
  });

  it('has no rows before an answer', () => {
    expect(rowsOf(undefined, labels)).toEqual([]);
  });

  it('keeps each search once, newest first, and at most eight', () => {
    const many = ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'];
    expect(addSearch(many, 'c')[0]).toBe('c');
    expect(addSearch(many, 'c')).toHaveLength(8);
    expect(addSearch(many, 'new')).toHaveLength(8);
    expect(addSearch(many, 'new')).not.toContain('h');
  });

  it('survives something unreadable in the browser', () => {
    expect(readSearches('not json')).toEqual([]);
    expect(readSearches('{"a":1}')).toEqual([]);
    expect(readSearches('["a",1]')).toEqual(['a']);
    expect(readSearches(null)).toEqual([]);
  });
});

describe('the strip of spaces', () => {
  const space = (id: string): Space => ({
    id,
    key: id,
    name: id,
    description: '',
    icon: '',
    status: 'active',
    home_page_id: null,
  });

  it('lists favourites, then recent spaces, then the rest, each once', () => {
    const [a, b, c, d] = [space('a'), space('b'), space('c'), space('d')];
    const mine = { favourites: [c], recent: [b] };
    expect(orderSpaces(mine, [a, b, c, d]).map((s) => s.id)).toEqual(['c', 'b', 'a', 'd']);
  });

  it('has nothing to show before an answer', () => {
    expect(orderSpaces(undefined, undefined)).toEqual([]);
  });

  it('always shows the current space, also when a list leaves it out', () => {
    expect(withCurrent([space('a')], space('old')).map((s) => s.id)).toEqual(['old', 'a']);
    expect(withCurrent([space('a')], space('a'))).toHaveLength(1);
  });
});

describe('small helpers', () => {
  const now = Date.parse('2026-06-15T12:00:00Z');
  const before = (ms: number) => new Date(now - ms).toISOString();

  it('says how long ago something was, and gives a date for old things', () => {
    expect(relativeTime(before(10_000), now, 'en')).toBe('now');
    expect(relativeTime(before(5 * 60_000), now, 'en')).toBe('5 minutes ago');
    expect(relativeTime(before(3 * 3_600_000), now, 'en')).toBe('3 hours ago');
    expect(relativeTime(before(86_400_000), now, 'en')).toBe('yesterday');
    expect(relativeTime(before(31 * 86_400_000), now, 'en')).not.toContain('ago');
  });

  it('suggests a key from the letters of a name', () => {
    expect(suggestKey('Team Handbook')).toBe('TEAMHANDBOOK');
    expect(suggestKey('Über uns')).toBe('UBERUNS');
    expect(suggestKey('A very long space name indeed')).toHaveLength(12);
  });

  it('moves an entry one step, and stays inside the list', () => {
    expect(moved(['a', 'b', 'c'], 1, -1)).toEqual(['b', 'a', 'c']);
    expect(moved(['a', 'b', 'c'], 0, -1)).toEqual(['a', 'b', 'c']);
    expect(moved(['a', 'b', 'c'], 2, 1)).toEqual(['a', 'b', 'c']);
  });

  it('folds long unchanged runs, and keeps the blocks next to a change', () => {
    const equal = (n: number): DiffBlock => ({ op: 'equal', type: 'paragraph', before: `${n}` });
    const change: DiffBlock = { op: 'added', type: 'paragraph', after: 'new' };
    const runs = foldUnchanged([
      equal(1),
      equal(2),
      equal(3),
      change,
      equal(4),
      equal(5),
      equal(6),
      equal(7),
    ]);
    expect(runs.map((r) => [r.folded, r.blocks.length])).toEqual([
      [true, 1],
      [false, 2],
      [false, 1],
      [false, 2],
      [true, 2],
    ]);
  });

  it('does not fold a short run between two changes', () => {
    const equal: DiffBlock = { op: 'equal', type: 'paragraph', before: 'x' };
    const change: DiffBlock = { op: 'added', type: 'paragraph', after: 'new' };
    const runs = foldUnchanged([change, equal, equal, change]);
    expect(runs.some((r) => r.folded)).toBe(false);
  });
});
