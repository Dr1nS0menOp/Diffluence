// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from 'vitest';
import { parseKept } from './kept-fields';

describe('text kept across a reload', () => {
  it('brings nothing back from text that is missing, broken or of another shape', () => {
    expect(parseKept(null)).toBeNull();
    expect(parseKept('')).toBeNull();
    expect(parseKept('{not json')).toBeNull();
    expect(parseKept('"a string"')).toBeNull();
    expect(parseKept('null')).toBeNull();
    expect(parseKept('{"fields":{}}')).toBeNull();
    expect(parseKept('{"path":"/x","fields":null}')).toBeNull();
    expect(parseKept('{"path":3,"fields":{}}')).toBeNull();
  });

  it('keeps only text values', () => {
    expect(parseKept('{"path":"/x","fields":{"0:0:name":"Team","0:1:":7,"0:2:":null}}')).toEqual({
      path: '/x',
      fields: { '0:0:name': 'Team' },
    });
  });
});
