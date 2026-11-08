// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import {
  NO_FILTERS,
  filterCount,
  queryWith,
  resultsAddress,
  spaceChoices,
  splitQuery,
} from './search-filters';

describe('the filters of the search box are words of the search syntax', () => {
  it('writes chosen spaces, labels and people into the query', () => {
    const q = queryWith('plans', {
      ...NO_FILTERS,
      spaces: ['ENG', 'OPS'],
      labels: ['draft'],
      people: ['Anna Visser'],
    });
    expect(q).toBe('plans space:ENG space:OPS label:draft author:"Anna Visser"');
  });

  it('reads them back, so the results page shows the same filters', () => {
    const back = splitQuery('plans space:eng label:Draft author:"Anna Visser" title:"x y"');
    expect(back).toEqual({
      text: 'plans title:"x y"',
      spaces: ['ENG'],
      labels: ['draft'],
      people: ['Anna Visser'],
    });
  });

  it('keeps type and date as parameters, and searches every space without filters', () => {
    expect(resultsAddress('plans', NO_FILTERS)).toBe('/search?q=plans');
    expect(resultsAddress('plans', { ...NO_FILTERS, kind: 'post', within: 'week' })).toBe(
      '/search?q=plans&kind=post&within=week',
    );
  });

  it('offers any space by name, beyond the first few, and keeps the chosen ones in view', () => {
    const spaces = Array.from({ length: 30 }, (_, i) => ({ key: `S${i}`, name: `Space ${i}` }));
    expect(spaceChoices(spaces, [], '', 3).map((s) => s.key)).toEqual(['S0', 'S1', 'S2']);
    expect(spaceChoices(spaces, [], 'space 27', 3).map((s) => s.key)).toEqual(['S27']);
    expect(spaceChoices(spaces, ['s29'], '', 2).map((s) => s.key)).toEqual(['S29', 'S0']);
  });

  it('counts what is on, and leaves out an empty word', () => {
    expect(filterCount({ ...NO_FILTERS, spaces: ['A'], kind: 'page' })).toBe(2);
    expect(splitQuery('space: label:"" plans').text).toBe('plans');
  });
});
