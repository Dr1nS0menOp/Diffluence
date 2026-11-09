// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { NOTHING_ON, offered, offeredNames } from './capabilities';

describe('what a screen offers', () => {
  it('leaves out drawing and PDF while their helper is off', () => {
    const items = [{ id: 'paragraph' }, { id: 'block_drawing' }];
    expect(offered(items, NOTHING_ON).map((item) => item.id)).toEqual(['paragraph']);
    expect(offeredNames(['markdown', 'pdf', 'docx'], NOTHING_ON)).toEqual(['markdown', 'docx']);
  });

  it('offers them once their helper is on', () => {
    const on = { ...NOTHING_ON, drawing: true, pdf_export: true };
    expect(offered([{ id: 'block_drawing' }], on)).toHaveLength(1);
    expect(offeredNames(['pdf'], on)).toEqual(['pdf']);
  });
});
