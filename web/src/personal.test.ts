// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  dropDraft,
  dropOldDrafts,
  dropUnkeyedData,
  forgetPersonalData,
  keepDraft,
  markSharedSaved,
  markSharedUnsaved,
  personalKey,
  readDraft,
  readRecentItems,
  rememberItem,
  sharedDocumentName,
  sharedDocumentsToForget,
  unsavedKey,
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

describe('text kept for working offline', () => {
  const saved = sharedDocumentName('anna', 'page-1');
  const unsaved = sharedDocumentName('anna', 'page-2');

  it('leaves at logout, unless it was never saved', () => {
    markSharedUnsaved('anna', 'page-2', NOW);
    expect(sharedDocumentsToForget([saved, unsaved, 'something-else'])).toEqual([saved]);
  });

  it('leaves once the service has it', () => {
    markSharedUnsaved('anna', 'page-2', NOW);
    markSharedSaved('anna', 'page-2');
    expect(sharedDocumentsToForget([unsaved])).toEqual([unsaved]);
  });

  it('is keyed per person', () => {
    expect(sharedDocumentName('anna', 'page')).not.toBe(sharedDocumentName('bob', 'page'));
    expect(unsavedKey('anna', 'page')).not.toBe(unsavedKey('bob', 'page'));
  });

  it('survives logout as a mark and as a backup, under the person', () => {
    markSharedUnsaved('anna', 'page-2', NOW);
    localStorage.setItem(unsavedKey('anna', 'page-3'), JSON.stringify({ saved: NOW }));
    forgetPersonalData();
    expect(localStorage.length).toBe(2);
  });

  it('stays for a short time only, so the document goes with its mark', () => {
    markSharedUnsaved('anna', 'page-2', NOW - 8 * DAY_MS);
    dropOldDrafts(NOW);
    expect(sharedDocumentsToForget([unsaved])).toEqual([unsaved]);
  });

  it('when the app starts, leaves the person logged in alone and clears the others', () => {
    const other = sharedDocumentName('bob', 'page-1');
    expect(sharedDocumentsToForget([saved, other], 'anna')).toEqual([other]);
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

describe('what a person took from the / menu lately', () => {
  beforeEach(() => vi.stubGlobal('localStorage', new FakeStorage()));

  it('keeps the newest first, without doubles, and at most five', () => {
    ['a', 'b', 'c', 'd', 'e', 'f', 'b'].forEach((id) => rememberItem('me', id));
    expect(readRecentItems('me')).toEqual(['b', 'f', 'e', 'd', 'c']);
  });

  it('belongs to one person, and goes at logout', () => {
    rememberItem('me', 'a');
    expect(readRecentItems('someone else')).toEqual([]);
    forgetPersonalData();
    expect(readRecentItems('me')).toEqual([]);
  });

  it('survives something unreadable in the browser', () => {
    const key = personalKey('me', 'insert.recent');
    localStorage.setItem(key, 'not json');
    expect(readRecentItems('me')).toEqual([]);
    localStorage.setItem(key, '{"a":1}');
    expect(readRecentItems('me')).toEqual([]);
  });
});
