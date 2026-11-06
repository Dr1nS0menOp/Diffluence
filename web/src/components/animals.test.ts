// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { DiffBlock } from '../api';
import { animalOf, ANIMALS, isAnimal } from './animals';
import { shownBlocks, wordRanges } from './editor/diff-view';

const paragraph = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });

describe('the animal of a space', () => {
  it('follows from the key, the same way every time and whatever the case', () => {
    const first = animalOf({ key: 'HANDBOOK', icon: '' });
    expect(first).not.toBeNull();
    expect(animalOf({ key: 'handbook', icon: '' })).toBe(first);
  });

  it('spreads keys that look alike over the animals', () => {
    const keys = ['ENG', 'ENG1', 'ENG2', 'ENG3', 'ENH', 'ENI', 'FNG', 'GNG'];
    const animals = new Set(keys.map((key) => animalOf({ key, icon: '' })));
    expect(animals.size).toBeGreaterThan(4);
  });

  it('gives a key in any script an animal', () => {
    expect(animalOf({ key: '日本語', icon: '' })).not.toBeNull();
  });

  it('keeps the animal a space chose', () => {
    expect(animalOf({ key: 'ENG', icon: 'moose' })).toBe('moose');
  });

  it('leaves a character of its own alone', () => {
    expect(animalOf({ key: 'ENG', icon: 'X' })).toBeNull();
  });

  it('knows its own list', () => {
    expect(ANIMALS).toHaveLength(30);
    expect(isAnimal('crocodile')).toBe(true);
    expect(isAnimal('dragon')).toBe(false);
  });
});

describe('words in a comparison', () => {
  it('finds added words and the place of removed ones', () => {
    const got = wordRanges('the new text here', [
      { op: 'equal', text: 'the' },
      { op: 'removed', text: 'old' },
      { op: 'added', text: 'new' },
      { op: 'equal', text: 'text here' },
    ]);
    expect(got).toEqual({ added: [[4, 7]], removed: [{ at: 4, text: 'old' }] });
  });

  it('puts removed words at the end when nothing follows', () => {
    const got = wordRanges('kept', [
      { op: 'equal', text: 'kept' },
      { op: 'removed', text: 'gone' },
    ]);
    expect(got?.removed).toEqual([{ at: 4, text: 'gone' }]);
  });

  it('copes with more than one space between words', () => {
    const got = wordRanges('one   two', [
      { op: 'equal', text: 'one' },
      { op: 'added', text: 'two' },
    ]);
    expect(got?.added).toEqual([[6, 9]]);
  });

  it('gives up when the words do not fit the text', () => {
    expect(wordRanges('something else', [{ op: 'equal', text: 'other words' }])).toBeNull();
    expect(wordRanges('more text here', [{ op: 'equal', text: 'more' }])).toBeNull();
    expect(wordRanges('short', [{ op: 'equal', text: 'short and long' }])).toBeNull();
  });
});

describe('what a comparison shows', () => {
  const words = [
    { op: 'removed' as const, text: 'old' },
    { op: 'added' as const, text: 'new' },
  ];

  it('marks words in a paragraph', () => {
    const block: DiffBlock = {
      op: 'changed',
      type: 'paragraph',
      words,
      node: paragraph('new'),
      previous: paragraph('old'),
    };
    expect(shownBlocks([block])).toEqual([{ op: 'changed', node: paragraph('new'), words }]);
  });

  it('shows a changed table as it was and as it is', () => {
    const block: DiffBlock = {
      op: 'changed',
      type: 'table',
      words,
      node: { type: 'table', id: 'now' },
      previous: { type: 'table', id: 'before' },
    };
    expect(shownBlocks([block]).map((b) => b.op)).toEqual(['removed', 'added']);
  });

  it('falls back when a paragraph holds what has no plain text', () => {
    const block: DiffBlock = {
      op: 'changed',
      type: 'paragraph',
      words,
      node: paragraph('other'),
      previous: paragraph('old'),
    };
    expect(shownBlocks([block]).map((b) => b.op)).toEqual(['removed', 'added']);
  });

  it('skips a block without a node', () => {
    expect(shownBlocks([{ op: 'equal', type: 'paragraph' }])).toEqual([]);
  });
});
