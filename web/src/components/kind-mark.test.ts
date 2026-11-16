// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { fileKindOf, pageKindOf } from './kind-mark';

describe('the kind of a page in a list (NAV-156)', () => {
  it('marks the home page of a space whatever its kind', () => {
    expect(pageKindOf({ kind: 'page', home: true })).toBe('home');
  });
  it('shows the kinds it knows', () => {
    expect(['post', 'database', 'folder', 'link'].map((kind) => pageKindOf({ kind }))).toEqual([
      'post',
      'database',
      'folder',
      'link',
    ]);
  });
  it('shows a kind it does not know, or none, as a page', () => {
    expect(pageKindOf({ kind: 'template' })).toBe('page');
    expect(pageKindOf({})).toBe('page');
  });
});

describe('the kind of a file in a list (NAV-156)', () => {
  it('reads the extension, whatever its case', () => {
    expect(fileKindOf('Welcome pack.PDF')).toBe('pdf');
    expect(fileKindOf('plan.v2.pptx')).toBe('slides');
    expect(fileKindOf('photo.jpeg')).toBe('picture');
    expect(fileKindOf('budget.csv')).toBe('sheet');
    expect(fileKindOf('notes.md')).toBe('document');
  });
  it('calls a name without a known extension a file', () => {
    expect(fileKindOf('README')).toBe('file');
    expect(fileKindOf('archive.tar.zst')).toBe('file');
    expect(fileKindOf('trailing.')).toBe('file');
  });
});
