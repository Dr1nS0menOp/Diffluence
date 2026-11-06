// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { MAX_ZOOM, moveCrop, startCrop, zoomCrop } from '../crop';
import { idleState, needsPing, PING_MS, safeReturnPath, WARNING_MS } from '../idle';
import { findSettings, SETTINGS_INDEX } from '../views/admin-search';
import { restrictionState } from '../views/page-dialogs';
import { countBelow } from './tree-model';

describe('cropping a picture', () => {
  it('starts on the largest square in the middle', () => {
    expect(startCrop(400, 200)).toEqual({ x: 100, y: 0, size: 200 });
    expect(startCrop(200, 400)).toEqual({ x: 0, y: 100, size: 200 });
  });

  it('never leaves the picture when moving', () => {
    const crop = { x: 0, y: 0, size: 100 };
    expect(moveCrop(crop, { x: -1, y: -1 }, 200, 200)).toEqual(crop);
    const end = { x: 100, y: 100, size: 100 };
    expect(moveCrop(end, { x: 1, y: 1 }, 200, 200)).toEqual(end);
  });

  it('moves by a share of the square, so a step feels the same at every zoom', () => {
    expect(moveCrop({ x: 50, y: 50, size: 100 }, { x: 1, y: 0 }, 400, 400).x).toBe(55);
    expect(moveCrop({ x: 50, y: 50, size: 200 }, { x: 1, y: 0 }, 400, 400).x).toBe(60);
  });

  it('zooms around the middle of the square', () => {
    const closer = zoomCrop({ x: 100, y: 100, size: 200 }, 1, 400, 400);
    expect(closer.size).toBe(160);
    expect(closer.x).toBe(120);
    expect(closer.y).toBe(120);
  });

  it('stops zooming at both ends', () => {
    let crop = startCrop(400, 400);
    for (let i = 0; i < 40; i++) crop = zoomCrop(crop, 1, 400, 400);
    expect(crop.size).toBe(400 / MAX_ZOOM);
    for (let i = 0; i < 40; i++) crop = zoomCrop(crop, -1, 400, 400);
    expect(crop).toEqual({ x: 0, y: 0, size: 400 });
  });

  it('stays inside when zooming out near an edge', () => {
    const crop = zoomCrop({ x: 300, y: 300, size: 100 }, -1, 400, 400);
    expect(crop.x + crop.size).toBeLessThanOrEqual(400);
    expect(crop.y + crop.size).toBeLessThanOrEqual(400);
  });
});

describe('the idle timeout', () => {
  const minutes = 10;
  const limit = minutes * 60_000;

  it('says nothing when there is no timeout', () => {
    expect(idleState(limit * 100, 0, 0)).toEqual({ kind: 'fine' });
  });

  it('warns two minutes before the end, and counts down', () => {
    expect(idleState(limit - WARNING_MS - 1, 0, minutes)).toEqual({ kind: 'fine' });
    expect(idleState(limit - WARNING_MS, 0, minutes)).toEqual({ kind: 'warning', seconds: 120 });
    expect(idleState(limit - 1000, 0, minutes)).toEqual({ kind: 'warning', seconds: 1 });
  });

  it('ends when the time is up', () => {
    expect(idleState(limit, 0, minutes)).toEqual({ kind: 'ended' });
  });

  it('tells the server about typing, at most once a minute', () => {
    expect(needsPing(PING_MS, 0, 10)).toBe(true);
    expect(needsPing(PING_MS - 1, 0, 10)).toBe(false);
    expect(needsPing(PING_MS, 10, 5)).toBe(false);
  });

  it('only returns to an address inside this site after login', () => {
    expect(safeReturnPath('/pages/1?x=2')).toBe('/pages/1?x=2');
    expect(safeReturnPath('//evil.example')).toBe('/');
    expect(safeReturnPath('/\\evil.example')).toBe('/');
    expect(safeReturnPath('https://evil.example')).toBe('/');
    expect(safeReturnPath(null)).toBe('/');
  });
});

describe('the settings search', () => {
  const word = (key: string) => (key === 'rules.idleMinutes' ? 'Log out after idle minutes' : key);

  it('finds a setting by the word on its screen', () => {
    const found = findSettings(SETTINGS_INDEX, 'IDLE', word);
    expect(found.map((f) => f.tab)).toContain('rules');
  });

  it('finds nothing for empty text', () => {
    expect(findSettings(SETTINGS_INDEX, '  ', word)).toEqual([]);
  });

  it('puts a match at the start of a name first', () => {
    const index = [
      { label: 'a', tab: 'one' },
      { label: 'b', tab: 'two' },
    ];
    const names = (key: string) => (key === 'a' ? 'the logo' : 'logo');
    expect(findSettings(index, 'logo', names).map((f) => f.tab)).toEqual(['two', 'one']);
  });
});

describe('restrictions', () => {
  it('reads the three states from what is stored', () => {
    expect(restrictionState({ view: [], edit: [] })).toBe('open');
    expect(restrictionState({ view: [], edit: [1] })).toBe('view_only');
    expect(restrictionState({ view: [1], edit: [] })).toBe('private');
    expect(restrictionState({ view: [1], edit: [1] })).toBe('private');
  });

  it('counts the pages below a page at any depth', () => {
    const nodes = [
      { id: 'a', parent_id: null },
      { id: 'b', parent_id: 'a' },
      { id: 'c', parent_id: 'b' },
      { id: 'd', parent_id: null },
    ];
    expect(countBelow(nodes, 'a')).toBe(2);
    expect(countBelow(nodes, 'c')).toBe(0);
  });
});
