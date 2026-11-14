// SPDX-License-Identifier: Apache-2.0
// The table of cases that Go runs as well (spec 074, TPL-325): the form and the server never
// disagree on a case it holds.
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { checkAnswer, checkField, defaultValue, type FieldSpec } from './template-fields';

const CREATOR = '01938f2e-7b3a-7cde-8f00-00000000c0de';

interface Table {
  today: string;
  answers: {
    name: string;
    field: Record<string, unknown>;
    answer: string;
    today?: string;
    problem: string;
    params?: Record<string, string>;
    value?: string;
  }[];
  specs: { name: string; field: Record<string, unknown>; problem: string }[];
  defaults: { name: string; field: Record<string, unknown>; value: string }[];
}

const table = JSON.parse(
  readFileSync(new URL('../../internal/templates/fields/testdata/cases.json', import.meta.url), 'utf8'),
) as Table;

/** Makes the long values the table names with @, as the Go suite does. */
function expand(v: string): string {
  if (v === '@creator') return CREATOR;
  const count = (prefix: string) => Number(v.slice(prefix.length));
  if (v.startsWith('@people')) {
    return Array.from(
      { length: count('@people') },
      (_, i) => `01938f2e-7b3a-7cde-8f00-${String(i + 1).padStart(12, '0')}`,
    ).join('\n');
  }
  if (v.startsWith('@label')) return 'a'.repeat(count('@label'));
  if (v.startsWith('@link')) {
    const prefix = 'https://example.org/';
    return prefix + 'a'.repeat(count('@link') - prefix.length);
  }
  if (/^@\d+$/.test(v)) return 'a'.repeat(count('@'));
  return v;
}

function specOf(raw: Record<string, unknown>): FieldSpec {
  const field: Record<string, unknown> = { name: 'Field', required: false, ...raw };
  for (const key of ['help', 'placeholder']) {
    if (typeof field[key] === 'string') field[key] = expand(field[key]);
  }
  const options = field.options;
  if (typeof options === 'string') {
    field.options = Array.from({ length: Number(options.slice('@options'.length)) }, (_, i) => `o${i + 1}`);
  } else if (Array.isArray(options)) {
    field.options = options.map((o) => expand(String(o)));
  }
  return field as unknown as FieldSpec;
}

describe('answers, from the shared table', () => {
  it('holds its cases', () => expect(table.answers.length).toBeGreaterThanOrEqual(100));
  for (const c of table.answers) {
    it(c.name, () => {
      const [value, problem] = checkAnswer(specOf(c.field), expand(c.answer), c.today ?? table.today);
      expect(problem?.problem ?? '').toBe(c.problem);
      if (problem && c.params) expect(problem.params).toEqual(c.params);
      if (c.value !== undefined) expect(value).toBe(c.value);
    });
  }
});

describe('fields as their author writes them, from the shared table', () => {
  for (const c of table.specs) {
    it(c.name, () => expect(checkField(specOf(c.field), table.today)?.problem ?? '').toBe(c.problem));
  }
});

describe('the words of every problem', () => {
  it('has words in the form for each problem the server can give', () => {
    const go = readFileSync(new URL('../../internal/templates/fields/fields.go', import.meta.url), 'utf8');
    const codes = [...go.matchAll(/^\t"(\w+)": +"/gm)].map((m) => m[1] ?? '');
    const words = (
      JSON.parse(readFileSync(new URL('./locales/en.json', import.meta.url), 'utf8')) as {
        templates: { problem: Record<string, string> };
      }
    ).templates.problem;
    expect(codes.length).toBeGreaterThan(40);
    expect(codes.filter((code) => !words[code])).toEqual([]);
  });
});

describe('defaults, from the shared table', () => {
  for (const c of table.defaults) {
    it(c.name, () => expect(defaultValue(specOf(c.field), table.today, CREATOR)).toBe(expand(c.value)));
  }
});
