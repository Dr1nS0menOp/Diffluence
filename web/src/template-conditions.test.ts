// SPDX-License-Identifier: Apache-2.0
// The table of cases that Go runs as well (spec 074, TPL-401, TPL-403, TPL-431): the form's
// preview and the server never disagree on a case it holds, and the sentence an approver
// reads says what the table says (TPL-1014).
import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import {
  conditionSentence,
  holds,
  OPERATORS,
  validateCondition,
  type Answer,
  type Condition,
  type ConditionField,
  type ConditionKind,
  type Op,
} from './template-conditions';

interface Table {
  today: string;
  operators: Record<string, string[]>;
  tests: {
    name: string;
    kind: ConditionKind;
    op: Op;
    value?: string;
    values?: string[];
    today?: boolean;
    answer: string;
    result: boolean;
  }[];
  conditions: {
    name: string;
    condition: Condition;
    answers: Record<string, Answer>;
    space?: string;
    groups?: string[];
    missing_groups?: string[];
    result: boolean;
  }[];
  validate: {
    name: string;
    condition: Record<string, unknown>;
    space?: boolean;
    problem: string;
    test?: number;
  }[];
  schema: { fields: Record<string, ConditionField>; later: string[] };
  sentences: { name: string; condition: Condition; names?: Record<string, string>; sentence: string }[];
}

const table = JSON.parse(
  readFileSync(new URL('../../internal/conditions/testdata/cases.json', import.meta.url), 'utf8'),
) as Table;

/** Makes the long parts the table names with @, as the Go suite does. */
function expand(raw: Record<string, unknown>): Condition {
  const c = structuredClone(raw);
  if (typeof c.tests === 'string') {
    c.tests = Array.from({ length: Number(c.tests.slice('@tests'.length)) }, () => ({
      subject: 'field',
      field: 'Name',
      op: 'empty',
    }));
  }
  for (const test of c.tests as Record<string, unknown>[]) {
    if (typeof test.value === 'string' && /^@\d+$/.test(test.value))
      test.value = 'a'.repeat(Number(test.value.slice(1)));
  }
  return c as unknown as Condition;
}

describe('the operators of each kind, from the shared table', () => {
  it('matches', () => expect(OPERATORS).toEqual(table.operators));
});

describe('every operator, from the shared table', () => {
  for (const c of table.tests) {
    it(c.name, () => {
      const condition: Condition = {
        match: 'all',
        tests: [{ subject: 'field', field: 'Q', op: c.op, value: c.value, values: c.values, today: c.today }],
      };
      expect(
        holds(condition, { answers: { Q: { kind: c.kind, value: c.answer } }, today: table.today }),
      ).toBe(c.result);
    });
  }
});

describe('conditions, from the shared table', () => {
  for (const c of table.conditions) {
    it(c.name, () =>
      expect(
        holds(c.condition, {
          answers: c.answers,
          space: c.space,
          groups: c.groups,
          missingGroups: c.missing_groups,
          today: table.today,
        }),
      ).toBe(c.result),
    );
  }
  it('a part without a condition always applies', () =>
    expect(holds(undefined, { answers: {}, today: '' })).toBe(true));
});

describe('the validator, from the shared table', () => {
  for (const c of table.validate) {
    it(c.name, () => {
      const p = validateCondition(expand(c.condition), {
        fields: table.schema.fields,
        later: new Set(table.schema.later),
        space: c.space ?? false,
      });
      expect(p?.problem ?? '').toBe(c.problem);
      if (p && c.test !== undefined) expect(p.test).toBe(c.test);
    });
  }
});

describe('the sentence of a condition, from the shared table', () => {
  for (const c of table.sentences) {
    it(c.name, () =>
      expect(conditionSentence(c.condition, { name: (id) => c.names?.[id] ?? id, date: (d) => d })).toBe(
        c.sentence,
      ),
    );
  }
});

describe('the words of every problem', () => {
  it('has words in the editor for each problem the server can give', () => {
    const go = readFileSync(new URL('../../internal/conditions/conditions.go', import.meta.url), 'utf8');
    const codes = [...go.matchAll(/^\t"(\w+)": +"/gm)].map((m) => m[1] ?? '');
    const words = (
      JSON.parse(readFileSync(new URL('./locales/en.json', import.meta.url), 'utf8')) as {
        templates: { conditionProblem: Record<string, string> };
      }
    ).templates.conditionProblem;
    expect(codes.length).toBeGreaterThan(10);
    expect(codes.filter((code) => !words[code])).toEqual([]);
  });
});
