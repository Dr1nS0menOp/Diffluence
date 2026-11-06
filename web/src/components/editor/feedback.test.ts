// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { positionAfterDrag } from '../page-header';
import { orderLanguages } from './languages';
import { scaledWidths, stretchAt, stretchedWidth } from './table';

describe('stretching a table', () => {
  const column = 800;
  const room = 1600;

  it('goes from the text column to the whole room', () => {
    expect(stretchedWidth(column, room, 0)).toBe(800);
    expect(stretchedWidth(column, room, 50)).toBe(1200);
    expect(stretchedWidth(column, room, 100)).toBe(1600);
  });

  it('never gets narrower than the column when there is no room', () => {
    expect(stretchedWidth(column, 500, 100)).toBe(800);
  });

  it('finds the stretch from the edge, in steps, on either side of the centre', () => {
    expect(stretchAt(600, column, room)).toBe(50);
    expect(stretchAt(-600, column, room)).toBe(50);
    expect(stretchAt(601, column, room)).toBe(50);
    expect(stretchAt(620, column, room)).toBe(55);
  });

  it('stays inside its limits', () => {
    expect(stretchAt(100, column, room)).toBe(0);
    expect(stretchAt(5000, column, room)).toBe(100);
  });

  it('has nothing to stretch into when the column already fills the room', () => {
    expect(stretchAt(900, column, column)).toBe(0);
  });

  it('keeps fixed columns in proportion', () => {
    expect(scaledWidths([100, 300], 1.5)).toEqual([150, 450]);
    expect(scaledWidths([1], 0.1)).toEqual([1]);
  });
});

describe('choosing a language', () => {
  const names = ['rust', 'python', 'java', 'javascript'];

  it('puts what the page uses first, and the rest from A to Z', () => {
    expect(orderLanguages(names, ['rust'], '', false)).toEqual(['rust', 'java', 'javascript', 'python']);
  });

  it('narrows by what was typed', () => {
    expect(orderLanguages(names, [], 'java', false)).toEqual(['java', 'javascript']);
    expect(orderLanguages(names, ['rust'], 'py', false)).toEqual(['python']);
  });

  it('offers an alias as the first answer', () => {
    expect(orderLanguages(names, [], 'js', true)).toEqual(['js']);
    expect(orderLanguages(names, [], 'java', true)).toEqual(['java', 'javascript']);
  });
});

describe('placing the header image', () => {
  it('follows the pointer, and goes the other way than the image moves', () => {
    expect(positionAfterDrag(50, 50, 100)).toBe(0);
    expect(positionAfterDrag(50, -50, 100)).toBe(100);
    expect(positionAfterDrag(50, 10, 100)).toBe(40);
  });

  it('stays inside its limits', () => {
    expect(positionAfterDrag(50, 500, 100)).toBe(0);
    expect(positionAfterDrag(50, -500, 100)).toBe(100);
  });

  it('does nothing when the whole image already shows', () => {
    expect(positionAfterDrag(50, 40, 0)).toBe(50);
  });
});
