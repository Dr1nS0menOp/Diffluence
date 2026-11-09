// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { craftOf, savedPDF, signalOf } from './craft-frame';

const bytesOf = (text: string) => new TextEncoder().encode(text).buffer;

describe('what the viewer gives back', () => {
  it('is kept when it is a PDF', () => {
    const bytes = bytesOf('%PDF-1.7 rest');
    expect(savedPDF({ craft: 'saved', bytes })).toBe(bytes);
  });

  it('is refused when it is not a PDF, too short, or not bytes at all', () => {
    expect(savedPDF({ craft: 'saved', bytes: bytesOf('<html>%PDF-') })).toBeNull();
    expect(savedPDF({ craft: 'saved', bytes: bytesOf('%PDF') })).toBeNull();
    expect(savedPDF({ craft: 'saved', bytes: '%PDF-1.7' })).toBeNull();
    expect(savedPDF({ craft: 'saved' })).toBeNull();
    expect(savedPDF(null)).toBeNull();
  });

  it('names its signal only in a message of its own shape', () => {
    expect(signalOf({ craft: 'saved' })).toBe('saved');
    expect(signalOf({ craft: 1 })).toBeUndefined();
    expect(signalOf('saved')).toBeUndefined();
  });
});

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
