// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { copyName } from '../attachments';
import { listFromText, matchesPattern } from './kinds';

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
