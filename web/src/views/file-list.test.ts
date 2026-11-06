// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { Attachment } from '../api';
import { arrangedFiles } from './file-list';

const file = (filename: string, size: number, updated_at: string) =>
  ({ filename, size, updated_at }) as Attachment;

const files = [
  file('b 10.txt', 5, '2026-11-01T10:00:00Z'),
  file('b 2.pdf', 50, '2026-11-03T10:00:00Z'),
  file('A.zip', 500, '2026-11-02T10:00:00Z'),
];
const names = (list: Attachment[]) => list.map((f) => f.filename);

describe('the files of a page', () => {
  it('sorts names as people read them', () => {
    expect(names(arrangedFiles(files, 'name', ''))).toEqual(['A.zip', 'b 2.pdf', 'b 10.txt']);
  });

  it('puts the newest and the largest first', () => {
    expect(names(arrangedFiles(files, 'date', ''))[0]).toBe('b 2.pdf');
    expect(names(arrangedFiles(files, 'size', ''))[0]).toBe('A.zip');
  });

  it('groups by type', () => {
    expect(names(arrangedFiles(files, 'type', ''))).toEqual(['b 2.pdf', 'b 10.txt', 'A.zip']);
  });

  it('filters on the name, whatever the case', () => {
    expect(names(arrangedFiles(files, 'name', ' ZIP '))).toEqual(['A.zip']);
    expect(arrangedFiles(files, 'name', 'nothing')).toEqual([]);
  });

  it('leaves the list it got alone', () => {
    const before = names(files);
    arrangedFiles(files, 'size', '');
    expect(names(files)).toEqual(before);
  });
});
