// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  dropDraft,
  dropOldDrafts,
  dropUnkeyedData,
  forgetPersonalData,
  keepDraft,
  personalKey,
  readDraft,
} from './personal';

const DAY_MS = 86_400_000;
const NOW = Date.parse('2026-11-05T12:00:00Z');
const BODY = { type: 'doc', content: [] };

class FakeStorage {
  private items = new Map<string, string>();
  get length() {
    return this.items.size;
  }
  key(at: number) {
    return [...this.items.keys()][at] ?? null;
  }
  getItem(key: string) {
    return this.items.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    this.items.set(key, value);
  }
  removeItem(key: string) {
    this.items.delete(key);
  }
  clear() {
    this.items.clear();
  }
}

beforeEach(() => {
  vi.stubGlobal('localStorage', new FakeStorage());
  vi.stubGlobal('sessionStorage', new FakeStorage());
});

describe('what a browser keeps for a person', () => {
  it('keeps people apart', () => {
    expect(personalKey('anna', 'searches')).not.toBe(personalKey('bob', 'searches'));
  });

  it('gives a draft only to the person who wrote it', () => {
    keepDraft('anna', 'page', BODY, NOW);
    expect(readDraft('anna', 'page', NOW)).toEqual(BODY);
    expect(readDraft('bob', 'page', NOW)).toBeUndefined();
  });

  it('removes everything of everyone at logout, and leaves the device settings', () => {
    localStorage.setItem(personalKey('anna', 'searches'), '["secret"]');
    localStorage.setItem(personalKey('bob', 'tree.space'), '{}');
    keepDraft('anna', 'page', BODY, NOW);
    localStorage.setItem('diffluence.theme', 'dark');
    sessionStorage.setItem('diffluence.scroll', '{}');

    forgetPersonalData();

    expect(localStorage.length).toBe(1);
    expect(localStorage.getItem('diffluence.theme')).toBe('dark');
    expect(sessionStorage.length).toBe(0);
  });

  it('drops what older versions kept without a person', () => {
    localStorage.setItem('diffluence.searches', '["secret"]');
    localStorage.setItem('diffluence.tree.space', '{}');
    localStorage.setItem('diffluence.theme', 'dark');

    dropUnkeyedData();

    expect(localStorage.length).toBe(1);
    expect(localStorage.getItem('diffluence.theme')).toBe('dark');
  });
});

describe('a comment draft', () => {
  it('goes when it is posted', () => {
    keepDraft('anna', 'page', BODY, NOW);
    dropDraft('anna', 'page');
    expect(readDraft('anna', 'page', NOW)).toBeUndefined();
  });

  it('is still there after six days, and gone after eight', () => {
    keepDraft('anna', 'page', BODY, NOW);
    expect(readDraft('anna', 'page', NOW + 6 * DAY_MS)).toEqual(BODY);
    expect(readDraft('anna', 'page', NOW + 8 * DAY_MS)).toBeUndefined();
    expect(localStorage.length).toBe(0);
  });

  it('is swept away when the app starts, for any person', () => {
    keepDraft('anna', 'old', BODY, NOW - 8 * DAY_MS);
    keepDraft('bob', 'new', BODY, NOW);
    localStorage.setItem(personalKey('anna', 'searches'), '[]');

    dropOldDrafts(NOW);

    expect(readDraft('anna', 'old', NOW)).toBeUndefined();
    expect(readDraft('bob', 'new', NOW)).toEqual(BODY);
    expect(localStorage.getItem(personalKey('anna', 'searches'))).toBe('[]');
  });

  it('counts as gone when it cannot be read', () => {
    localStorage.setItem(personalKey('anna', 'draft.page'), 'not json');
    expect(readDraft('anna', 'page', NOW)).toBeUndefined();
  });
});
