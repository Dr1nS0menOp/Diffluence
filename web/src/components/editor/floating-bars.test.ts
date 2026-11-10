// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { barFor, barPlacement, keepClearOfRow, type SelectionFacts } from './floating-bars';
import { stepFor } from './roving-focus';

const words: SelectionFacts = {
  empty: false,
  node: null,
  inCode: false,
  cells: false,
  composing: false,
  covered: false,
};

describe('when a floating bar shows', () => {
  it('shows nothing where it would be in the way or has nothing to do', () => {
    expect(barFor({ ...words, empty: true })).toBeNull();
    expect(barFor({ ...words, inCode: true })).toBeNull();
    expect(barFor({ ...words, cells: true })).toBeNull();
    expect(barFor({ ...words, composing: true })).toBeNull();
    expect(barFor({ ...words, covered: true })).toBeNull();
    expect(barFor({ ...words, node: 'table' })).toBeNull();
    expect(barFor({ ...words, node: 'image', covered: true })).toBeNull();
  });

  it('shows the text bar for selected words and the image bar for a selected image', () => {
    expect(barFor(words)).toBe('text');
    expect(barFor({ ...words, node: 'image', empty: false })).toBe('image');
  });
});

describe('where a floating bar goes', () => {
  it('goes above the selection, and below it on a touch screen', () => {
    expect(barPlacement(false, 0).placement).toBe('top');
    expect(barPlacement(true, 0).placement).toBe('bottom');
  });

  it('flips and shifts, and keeps clear of the sticky row', () => {
    const options = barPlacement(false, 120);
    expect(options.flip).toEqual({ padding: expect.objectContaining({ top: 128 }) });
    expect(options.shift).toEqual({ padding: expect.objectContaining({ top: 128, left: 8, right: 8 }) });
    expect(options.hide).toBe(true);
  });

  it('follows the row as it moves, in both middlewares at once', () => {
    const options = barPlacement(false, 0);
    keepClearOfRow(options, 56);
    expect(options.flip.padding.top).toBe(64);
    expect(options.shift.padding.top).toBe(64);
  });
});

describe('the arrow keys in a row of tools', () => {
  it('move the way they point, also on a right-to-left page', () => {
    expect(stepFor('ArrowRight', 1, 4, false)).toBe(2);
    expect(stepFor('ArrowLeft', 1, 4, false)).toBe(0);
    expect(stepFor('ArrowRight', 1, 4, true)).toBe(0);
    expect(stepFor('ArrowLeft', 1, 4, true)).toBe(2);
  });

  it('wrap at the ends, and Home and End go to the ends', () => {
    expect(stepFor('ArrowRight', 3, 4, false)).toBe(0);
    expect(stepFor('ArrowLeft', 0, 4, false)).toBe(3);
    expect(stepFor('Home', 2, 4, true)).toBe(0);
    expect(stepFor('End', 0, 4, false)).toBe(3);
    expect(stepFor('Enter', 0, 4, false)).toBeUndefined();
  });
});
