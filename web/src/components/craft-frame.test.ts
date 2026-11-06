// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { craftOf } from './craft-frame';

describe('craftOf', () => {
  it('opens a PDF by its type or by its name', () => {
    expect(craftOf('report', 'application/pdf')).toBe('pdfcraft');
    expect(craftOf('REPORT.PDF', '')).toBe('pdfcraft');
  });

  it('opens a spreadsheet by its name', () => {
    expect(craftOf('sums.xlsx', '')).toBe('gridcraft');
    expect(craftOf('sums.csv', 'text/csv')).toBe('gridcraft');
  });

  it('leaves every other file a download', () => {
    expect(craftOf('notes.txt', 'text/plain')).toBeNull();
    expect(craftOf('pdf', '')).toBeNull();
  });
});
