// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { fromFrame, OPAQUE_ORIGIN } from './frame-messages';

describe('a message from a frame', () => {
  const frame = {} as Window;
  const other = {} as Window;

  it('counts only from our frame, on our origin', () => {
    expect(fromFrame({ source: frame, origin: OPAQUE_ORIGIN, data: 1 }, frame, OPAQUE_ORIGIN)).toBe(true);
  });

  it('is refused from another window, even with the right origin', () => {
    expect(fromFrame({ source: other, origin: OPAQUE_ORIGIN, data: 1 }, frame, OPAQUE_ORIGIN)).toBe(false);
  });

  it('is refused from our frame on another origin', () => {
    expect(fromFrame({ source: frame, origin: 'https://evil.example', data: 1 }, frame, OPAQUE_ORIGIN)).toBe(
      false,
    );
  });

  it('is refused before the frame exists', () => {
    expect(fromFrame({ source: undefined, origin: OPAQUE_ORIGIN, data: 1 }, null, OPAQUE_ORIGIN)).toBe(false);
  });
});
