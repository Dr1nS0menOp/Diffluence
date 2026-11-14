// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import type { ContentRow, Json, TemplateSummary } from '../api';
import { cleanLabel } from '../components/page-labels';
import { forReaders } from '../components/publish-options';
import { stepsOf } from '../components/template-gallery';
import { rangeOfRows, toggleRow } from '../components/tree-model';
import { selectWithChildren } from './content';
import { readBackup } from './edit';
import { changedTitle } from './move-dialogs';
import { searchParamsFor } from './search';

const NOW = Date.parse('2026-11-05T12:00:00Z');

const row = (id: string, parent: string | null): ContentRow => ({
  id,
  parent_id: parent,
  title: id,
  owner_id: '',
  owner_name: '',
  owner_disabled: false,
  stage_text: '',
  stage_color: '',
  labels: [],
  created_at: '',
  updated_at: '',
  children: 0,
});

const template = (extra: Partial<TemplateSummary>): TemplateSummary => ({
  id: 't',
  space_id: null,
  name: 'Notes',
  description: '',
  category: '',
  labels: [],
  title_pattern: '',
  steps: [],
  default_parent: null,
  hidden: false,
  promoted: false,
  starter: false,
  favourite: false,
  version: 1,
  guided: false,
  template_version: 1,
  ...extra,
});

describe('writing a label', () => {
  it('makes capitals small and spaces dashes while typing', () => {
    expect(cleanLabel('Team Plans  2026')).toBe('team-plans-2026');
  });

  it('keeps letters of any language', () => {
    expect(cleanLabel('Überblick Café')).toBe('überblick-café');
  });
});

describe('titles of a copy', () => {
  const nothing = { prefix: '', suffix: '', find: '', replace: '' };

  it('leaves a title alone when nothing is asked', () => {
    expect(changedTitle('Plan', nothing)).toBe('Plan');
  });

  it('adds a prefix and a suffix', () => {
    expect(changedTitle('Plan', { ...nothing, prefix: 'Old ', suffix: ' 2026' })).toBe('Old Plan 2026');
  });

  it('replaces every match, and treats the search text as plain text', () => {
    expect(changedTitle('a.b a.b', { ...nothing, find: 'a.b', replace: 'c' })).toBe('c c');
    expect(changedTitle('axb', { ...nothing, find: 'a.b', replace: 'c' })).toBe('axb');
  });
});

describe('selecting in the Content table', () => {
  const rows = [row('parent', null), row('one', 'parent'), row('two', 'parent'), row('other', null)];

  it('selects the children of a parent as well', () => {
    expect(selectWithChildren(rows, [], 'parent')).toEqual(['parent', 'one', 'two']);
  });

  it('lets a child be cleared on its own', () => {
    expect(selectWithChildren(rows, ['parent', 'one', 'two'], 'one')).toEqual(['parent', 'two']);
  });

  it('adds nothing twice', () => {
    expect(selectWithChildren(rows, ['one'], 'parent')).toEqual(['one', 'parent', 'two']);
  });
});

describe('selecting rows in the tree', () => {
  const order = ['a', 'b', 'c', 'd'];

  it('adds and takes away one row', () => {
    expect(toggleRow(['a'], 'b')).toEqual(['a', 'b']);
    expect(toggleRow(['a', 'b'], 'a')).toEqual(['b']);
  });

  it('adds every row between two rows, in either direction', () => {
    expect(rangeOfRows(order, [], 'b', 'd')).toEqual(['b', 'c', 'd']);
    expect(rangeOfRows(order, ['a'], 'd', 'c')).toEqual(['a', 'c', 'd']);
  });

  it('falls back to one row when the last row is out of view', () => {
    expect(rangeOfRows(order, [], 'gone', 'b')).toEqual(['b']);
  });
});

describe('filters in the address', () => {
  const read = (address: string) =>
    Object.fromEntries(searchParamsFor(new URLSearchParams(address), NOW).entries());

  it('passes the text and the filters on', () => {
    expect(read('q=plan&label=hr&kind=post&sort=date')).toEqual({
      q: 'plan',
      label: 'hr',
      kind: 'post',
      sort: 'date',
    });
  });

  it('keeps several labels, which mean or', () => {
    const params = searchParamsFor(new URLSearchParams('label=hr&label=it'), NOW);
    expect(params.getAll('label')).toEqual(['hr', 'it']);
  });

  it('reads a comma as another label', () => {
    const params = searchParamsFor(new URLSearchParams('label=hr, it'), NOW);
    expect(params.getAll('label')).toEqual(['hr', 'it']);
  });

  it('turns a date preset into a moment', () => {
    expect(read('within=week').since).toBe('2026-10-29T12:00:00.000Z');
  });

  it('leaves out what is empty, what is off and what the server does not know', () => {
    expect(read('q=&titles=0&tab=people&foo=bar')).toEqual({});
  });
});

describe('the form of a template', () => {
  const fields = [
    { name: 'Goal', kind: 'line', required: true },
    { name: 'Notes', kind: 'text', required: false },
  ];

  it('shows one step when there are no steps', () => {
    expect(stepsOf(template({ fields }))).toEqual([{ title: '', fields }]);
  });

  it('groups fields into steps, and puts the rest last', () => {
    const steps = stepsOf(template({ fields, steps: [{ title: 'Why', fields: ['Goal'] }] }));
    expect(steps.map((s) => s.title)).toEqual(['Why', '']);
    expect(steps[1]?.fields.map((f) => f.name)).toEqual(['Notes']);
  });
});

describe('the preview of a page', () => {
  it('leaves out instruction text at every level', () => {
    const document = {
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [
            { type: 'text', text: 'kept' },
            { type: 'instruction', attrs: { text: 'help' } },
          ],
        },
      ],
    } as Json;
    expect(JSON.stringify(forReaders(document))).not.toContain('instruction');
    expect(JSON.stringify(forReaders(document))).toContain('kept');
  });
});

describe('text kept in the browser', () => {
  it('reads what was kept', () => {
    const kept = { title: 'T', document: { type: 'doc' }, at: '2026-11-05T12:00:00Z' };
    expect(readBackup(JSON.stringify(kept))).toEqual(kept);
  });

  it('ignores anything else', () => {
    expect(readBackup(null)).toBeNull();
    expect(readBackup('not json')).toBeNull();
    expect(readBackup('{"title":1}')).toBeNull();
  });
});
