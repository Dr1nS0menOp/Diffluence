// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { changedParts, rebased, saveInOrder, type DetailsValues } from './space-details-save';

const stored: DetailsValues = {
  name: 'Handbook',
  description: 'How we work',
  owners: ['a'],
  categories: ['c1'],
  home: 'p1',
  key: 'HANDBOOK',
};

describe('which parts a Save of Details sends', () => {
  it('sends nothing when nothing changed', () => {
    expect(changedParts(stored, { ...stored })).toEqual([]);
  });

  it('sends the changed parts in a fixed order, with the key last because the address follows it', () => {
    const form = { ...stored, key: 'HB', home: 'p2', owners: ['a', 'b'], name: 'Hand book' };
    expect(changedParts(stored, form)).toEqual(['details', 'owners', 'home', 'key']);
  });

  it('treats the description as part of the name and description', () => {
    expect(changedParts(stored, { ...stored, description: '' })).toEqual(['details']);
  });

  it('sends no categories or home page while the fold has not loaded them', () => {
    const form = { ...stored, categories: null, home: null };
    expect(changedParts({ ...stored, categories: null, home: null }, form)).toEqual([]);
  });

  it('compares owners and categories as sets, not by their order', () => {
    expect(changedParts({ ...stored, owners: ['a', 'b'] }, { ...stored, owners: ['b', 'a'] })).toEqual([]);
  });
});

describe('the form when what is stored changes', () => {
  it('takes the new stored value of a field nobody touched, and keeps what was typed', () => {
    const typed = { ...stored, name: 'Typed', categories: null };
    const before = { ...stored, categories: null };
    const now = { ...stored, categories: ['c1', 'c2'], owners: ['a', 'b'] };
    expect(rebased(before, now, typed)).toEqual({ ...now, name: 'Typed' });
  });
});

describe('saving the parts', () => {
  it('saves every part in order when none is refused', async () => {
    const sent: string[] = [];
    const result = await saveInOrder(['details', 'owners', 'key'], async (part) => {
      sent.push(part);
    });
    expect(sent).toEqual(['details', 'owners', 'key']);
    expect(result).toEqual({ saved: ['details', 'owners', 'key'] });
  });

  it('stops at the first refusal, and says which part was refused and which were saved', async () => {
    const sent: string[] = [];
    const refusal = new Error('no owner left');
    const result = await saveInOrder(['details', 'owners', 'home', 'key'], async (part) => {
      sent.push(part);
      if (part === 'owners') throw refusal;
    });
    expect(sent).toEqual(['details', 'owners']);
    expect(result).toEqual({ saved: ['details'], refused: { part: 'owners', error: refusal } });
  });
});
