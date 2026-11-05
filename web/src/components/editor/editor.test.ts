// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { getSchema } from '@tiptap/core';
import '../../i18n';
import { resizeColumns } from './block-commands';
import { documentExtensions } from './extensions';
import { findMatches } from './find';
import { formatStoredDate, today } from './inline';
import { INSERT_ITEMS, matchingEmoji, matchingItems } from './insert';
import { sortRows } from './table';

const schema = getSchema(documentExtensions());

const paragraph = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
const cell = (type: string, text: string) => ({ type, content: [paragraph(text)] });
const row = (type: string, ...texts: string[]) =>
  schema.nodeFromJSON({ type: 'tableRow', content: texts.map((text) => cell(type, text)) });
const firstCells = (rows: ReturnType<typeof row>[]) => rows.map((r) => r.child(0).textContent);

describe('the insert list', () => {
  it('finds an entry by its name and by a word people might try', () => {
    expect(matchingItems('warning').map((i) => i.id)).toContain('panelWarning');
    expect(matchingItems('callout').map((i) => i.id)).toContain('panelInfo');
    expect(matchingItems('toggle').map((i) => i.id)).toEqual(['expand']);
  });

  it('shows everything for an empty query, and nothing for nonsense', () => {
    expect(matchingItems('')).toHaveLength(INSERT_ITEMS.length);
    expect(matchingItems('qqqq')).toEqual([]);
  });

  it('ignores case', () => {
    expect(matchingItems('TABLE').map((i) => i.id)).toContain('table');
  });
});

describe('emoji', () => {
  it('finds a character by a word', () => {
    expect(matchingEmoji('rocket')[0]?.glyph).toBe('🚀');
  });

  it('stops at a handful of results', () => {
    expect(matchingEmoji('e').length).toBeLessThanOrEqual(8);
  });
});

describe('dates', () => {
  it('stores today in the local calendar', () => {
    expect(today(new Date(2026, 0, 5, 23, 30))).toBe('2026-01-05');
  });

  it('shows a stored date on the same day, whatever the time zone', () => {
    expect(formatStoredDate('2026-11-05')).toMatch(/5/);
  });
});

describe('sorting a table', () => {
  const header = row('tableHeader', 'Name');
  const rows = ['item 10', 'item 2', 'Item 1'].map((text) => row('tableCell', text));

  it('uses natural order, so 2 comes before 10', () => {
    expect(firstCells(sortRows([header, ...rows], 0, 'up'))).toEqual(['Name', 'Item 1', 'item 2', 'item 10']);
  });

  it('keeps the header on top when sorting down', () => {
    expect(firstCells(sortRows([header, ...rows], 0, 'down'))).toEqual([
      'Name',
      'item 10',
      'item 2',
      'Item 1',
    ]);
  });

  it('survives a row with too few cells', () => {
    const short = row('tableCell', 'x');
    expect(sortRows([short, row('tableCell', 'a', 'b')], 1, 'up')).toHaveLength(2);
  });
});

describe('changing the number of columns', () => {
  const column = (text: string) => schema.nodeFromJSON({ type: 'column', content: [paragraph(text)] });
  const empty = () => schema.nodeFromJSON({ type: 'column', content: [{ type: 'paragraph' }] });

  it('moves the content of removed columns into the last one that stays', () => {
    const result = resizeColumns([column('a'), column('b'), column('c')], 2, empty);
    expect(result).toHaveLength(2);
    expect(result[1]?.childCount).toBe(2);
    expect(result[1]?.textContent).toContain('b');
    expect(result[1]?.textContent).toContain('c');
  });

  it('adds empty columns', () => {
    const result = resizeColumns([column('a'), column('b')], 4, empty);
    expect(result).toHaveLength(4);
    expect(result[3]?.textContent).toBe('');
  });
});

describe('finding text', () => {
  const doc = schema.nodeFromJSON({
    type: 'doc',
    content: [
      paragraph('The cat and the Cat'),
      {
        type: 'paragraph',
        content: [
          { type: 'text', text: 'before ' },
          { type: 'status', attrs: { text: 'cat', color: 'blue' } },
          { type: 'text', text: ' cat' },
        ],
      },
    ],
  });

  it('ignores case unless asked', () => {
    expect(findMatches(doc, 'cat', false)).toHaveLength(3);
    expect(findMatches(doc, 'Cat', true)).toHaveLength(1);
  });

  it('gives positions that select exactly the words', () => {
    const match = findMatches(doc, 'and', false)[0];
    expect(match && doc.textBetween(match.from, match.to)).toBe('and');
  });

  it('keeps positions right after an item without text', () => {
    const matches = findMatches(doc, 'cat', false);
    const last = matches[matches.length - 1];
    expect(last && doc.textBetween(last.from, last.to)).toBe('cat');
  });

  it('finds nothing for an empty query', () => {
    expect(findMatches(doc, '', false)).toEqual([]);
  });
});
