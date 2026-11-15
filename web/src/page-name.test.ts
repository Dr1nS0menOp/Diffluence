// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { pageTitle } from './page-name';

const t = (key: string) => (key === 'page.untitled' ? 'Untitled' : key);

describe('pageTitle', () => {
  it('names a page without a title Untitled', () => {
    expect(pageTitle(t, '')).toBe('Untitled');
  });
  it('names a page whose title is only spaces Untitled', () => {
    expect(pageTitle(t, '   ')).toBe('Untitled');
  });
  it('names a page whose title is missing Untitled', () => {
    expect(pageTitle(t, null)).toBe('Untitled');
    expect(pageTitle(t, undefined)).toBe('Untitled');
  });
  it('keeps a title as it was written', () => {
    expect(pageTitle(t, ' Working agreement ')).toBe(' Working agreement ');
  });
});
