// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { drawingAddress, drawingMessage, isDrawing, loadMessage, pngFromDataAddress } from './drawing';

const ORIGIN = 'https://draw.example.com';
const frame = {} as Window;
const message = (over: Partial<{ source: unknown; origin: string; data: unknown }>) => ({
  source: frame,
  origin: ORIGIN,
  data: JSON.stringify({ event: 'init' }),
  ...over,
});

describe('a saved drawing', () => {
  it('is known by the name we gave it', () => {
    expect(isDrawing('drawing-20261106101500.png')).toBe(true);
    expect(isDrawing('holiday.png')).toBe(false);
    expect(isDrawing('drawing-notes.pdf')).toBe(false);
  });

  it('opens with what is stored, and otherwise starts empty', () => {
    expect(loadMessage('data:image/png;base64,AAAA')).toEqual({
      action: 'load',
      xmlpng: 'data:image/png;base64,AAAA',
      autosave: 0,
    });
    expect(loadMessage(null)).toEqual({ action: 'load', xml: '<mxfile/>', autosave: 0 });
  });
});

describe('drawingMessage', () => {
  it('accepts a message from the frame and the origin we set', () => {
    expect(drawingMessage(message({}), frame, ORIGIN)).toEqual({ event: 'init' });
  });

  it('refuses another window, even with the right origin', () => {
    expect(drawingMessage(message({ source: {} }), frame, ORIGIN)).toBeNull();
  });

  it('refuses another origin, even from the frame', () => {
    expect(drawingMessage(message({ origin: 'https://evil.example.com' }), frame, ORIGIN)).toBeNull();
  });

  it('refuses what is not a JSON message with an event', () => {
    expect(drawingMessage(message({ data: 'not json' }), frame, ORIGIN)).toBeNull();
    expect(drawingMessage(message({ data: 7 }), frame, ORIGIN)).toBeNull();
    expect(drawingMessage(message({ data: '{"event":3}' }), frame, ORIGIN)).toBeNull();
    expect(drawingMessage(message({ data: 'null' }), frame, ORIGIN)).toBeNull();
  });
});

describe('pngFromDataAddress', () => {
  it('reads a PNG', () => {
    expect(pngFromDataAddress('data:image/png;base64,iVBO')?.type).toBe('image/png');
  });

  it('refuses anything else', () => {
    expect(pngFromDataAddress('data:text/html;base64,PHNjcmlwdD4=')).toBeNull();
    expect(pngFromDataAddress('data:image/png;base64,***')).toBeNull();
  });
});

describe('drawingAddress', () => {
  it('asks for embed mode that talks to nobody else', () => {
    const address = new URL(drawingAddress(ORIGIN));
    expect(address.origin).toBe(ORIGIN);
    expect(address.searchParams.get('embed')).toBe('1');
    expect(address.searchParams.get('proto')).toBe('json');
    expect(address.searchParams.get('offline')).toBe('1');
    expect(address.searchParams.get('stealth')).toBe('1');
  });
});
