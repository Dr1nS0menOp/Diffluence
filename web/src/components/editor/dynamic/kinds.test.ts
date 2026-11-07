// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { Json } from '../../../api';
import { copyName } from '../attachments';
import { listFromText, matchesPattern, sectionOf, timelineOf } from './kinds';

describe('matchesPattern', () => {
  it('treats a star as any text, and nothing else as special', () => {
    expect(matchesPattern('report.pdf', '*.pdf')).toBe(true);
    expect(matchesPattern('report.pdf', 'rep*')).toBe(true);
    expect(matchesPattern('report.pdf', '(a+)+$')).toBe(false);
  });

  it('holds a pattern without a star to the start of the text', () => {
    expect(matchesPattern('the report', 'report')).toBe(false);
  });
});

describe('listFromText', () => {
  it('splits on commas and drops empty parts', () => {
    expect(listFromText(' a, b ,,c')).toEqual(['a', 'b', 'c']);
  });
});

describe('copyName', () => {
  it('puts the number before the extension', () => {
    expect(copyName('a.txt', 2)).toBe('a (2).txt');
  });

  it('adds the number at the end when there is no extension', () => {
    expect(copyName('README', 3)).toBe('README (3)');
    expect(copyName('.env', 2)).toBe('.env (2)');
  });
});

describe('a timeline', () => {
  it('turns a line into a bar, and a line without an end into a marker', () => {
    const [bar, marker] = timelineOf([
      'Build | First | 2026-01-01 | 2026-02-01',
      'Build | Launch | 2026-03-01',
    ]);
    expect(bar?.end).toBeGreaterThan(bar?.start ?? 0);
    expect(marker?.end).toBeUndefined();
  });

  it('leaves out a line it cannot read', () => {
    expect(timelineOf(['no dates here', 'Lane | Label | someday', 'Lane | | 2026-01-01'])).toEqual([]);
    expect(timelineOf('not a list')).toEqual([]);
  });

  it('treats an end before the start as a marker', () => {
    expect(timelineOf(['Lane | Label | 2026-03-01 | 2026-01-01'])[0]?.end).toBeUndefined();
  });
});

describe('the part of a page a section block is about', () => {
  const anchor = (name: string) => ({ type: 'paragraph', content: [{ type: 'anchor', attrs: { name } }] });
  const heading = (text: string) => ({ type: 'heading', content: [{ type: 'text', text }] });
  const page = {
    type: 'doc',
    content: [
      heading('Before'),
      anchor('start'),
      heading('Inside'),
      anchor('end'),
      heading('After'),
      { type: 'expand', attrs: { title: 'More' }, content: [heading('Folded')] },
    ],
  } as unknown as Json;
  const texts = (part: Json) =>
    ((part as unknown as { content: { content?: { text?: string }[] }[] }).content ?? []).flatMap(
      (node) => node.content?.map((child) => child.text).filter(Boolean) ?? [],
    );

  it('runs between two anchors', () => {
    expect(texts(sectionOf(page, { start: 'start', end: 'end' }))).toEqual(['Inside']);
  });

  it('can be one collapsible section', () => {
    expect(texts(sectionOf(page, { section: 'More' }))).toEqual(['Folded']);
  });

  it('is empty for a section that does not exist', () => {
    expect(texts(sectionOf(page, { section: 'Nowhere' }))).toEqual([]);
  });
});
