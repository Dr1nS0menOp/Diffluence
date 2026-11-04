// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { highlight } from './search';

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
