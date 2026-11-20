// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { differs } from './save-reset';
import { guideAddress } from './guide-link';

describe('whether a form changed', () => {
  it('is unchanged while every field holds what is stored', () => {
    expect(differs({ days: 30, types: ['exe'] }, { days: 30, types: ['exe'] })).toBe(false);
  });

  it('is changed once one field differs, also inside a list', () => {
    expect(differs({ days: 30, types: ['exe'] }, { days: 31, types: ['exe'] })).toBe(true);
    expect(differs({ days: 30, types: ['exe'] }, { days: 30, types: ['exe', 'bat'] })).toBe(true);
  });

  it('is unchanged again once a field is put back by hand', () => {
    const stored = { on: true };
    const changed = { on: false };
    expect(differs(stored, changed)).toBe(true);
    expect(differs(stored, { ...changed, on: true })).toBe(false);
  });
});

describe('where a part of the documentation lives', () => {
  it('is the address itself without a path', () => {
    expect(guideAddress('https://docs.example.com/', undefined)).toBe('https://docs.example.com/');
  });

  it('joins the address and the path with one slash', () => {
    expect(guideAddress('https://docs.example.com/', '/operations/backup-restore.md')).toBe(
      'https://docs.example.com/operations/backup-restore.md',
    );
    expect(guideAddress('https://docs.example.com', 'operations/upgrade.md')).toBe(
      'https://docs.example.com/operations/upgrade.md',
    );
  });
});
