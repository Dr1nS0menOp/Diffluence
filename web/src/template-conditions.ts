// SPDX-License-Identifier: Apache-2.0
/**
 * Conditions of a template: "all of" or "any of" at most ten tests on one level, picked from
 * lists (spec 074, TPL-400 to TPL-403, TPL-431). This mirrors internal/conditions in Go, and
 * both run the same table, internal/conditions/testdata/cases.json. The browser only previews:
 * the server decides again on submit, and its answer is the only one that counts (TPL-1406).
 */
import i18n from './i18n';

export type Match = 'all' | 'any';
export type Subject = 'field' | 'space' | 'groups';

export type ConditionKind =
  | 'line'
  | 'text'
  | 'link'
  | 'number'
  | 'date'
  | 'yesno'
  | 'choice'
  | 'choices'
  | 'person'
  | 'people'
  | 'page'
  | 'label'
  | 'space'
  | 'groups';

export type Op =
  | 'empty'
  | 'not_empty'
  | 'is'
  | 'is_not'
  | 'contains'
  | 'eq'
  | 'ne'
  | 'lt'
  | 'le'
  | 'gt'
  | 'ge'
  | 'before'
  | 'after'
  | 'on_or_before'
  | 'on_or_after'
  | 'yes'
  | 'no'
  | 'one_of'
  | 'includes'
  | 'excludes'
  | 'includes_all'
  | 'member'
  | 'not_member';

export interface Test {
  subject: Subject;
  field?: string;
  op: Op;
  value?: string;
  values?: string[];
  /** A test of a date compares with the day the set is made. */
  today?: boolean;
}

export interface Condition {
  match: Match;
  tests: Test[];
}

/** A checked answer and the kind of its question; several items come one per line. */
export interface Answer {
  kind: ConditionKind | '';
  value: string;
}

export interface Facts {
  answers: Record<string, Answer>;
  today: string;
  space?: string;
  groups?: string[];
  missingGroups?: string[];
}

export const MAX_TESTS = 10;
export const MAX_VALUES = 50;
const MAX_VALUE_RUNES = 200;
const MAX_ITEMS = 50;
const MAX_LABEL = 64;

const WORDS: Op[] = ['empty', 'not_empty', 'is', 'is_not', 'contains'];

/** The operators each kind may use, in the order the editor offers them (TPL-401). */
export const OPERATORS: Record<ConditionKind, Op[]> = {
  line: WORDS,
  text: WORDS,
  link: WORDS,
  number: ['empty', 'not_empty', 'eq', 'ne', 'lt', 'le', 'gt', 'ge'],
  date: ['empty', 'not_empty', 'is', 'before', 'after', 'on_or_before', 'on_or_after'],
  yesno: ['yes', 'no'],
  choice: ['empty', 'is', 'is_not', 'one_of'],
  choices: ['empty', 'includes', 'excludes', 'includes_all'],
  person: ['empty', 'not_empty'],
  people: ['empty', 'not_empty'],
  page: ['empty', 'not_empty'],
  label: ['is', 'is_not', 'one_of'],
  space: ['one_of'],
  groups: ['member', 'not_member'],
};

/** The operators that compare with nothing, with one value, or with a list of values. */
export const NO_VALUE: Op[] = ['empty', 'not_empty', 'yes', 'no'];
export const LIST_VALUE: Op[] = ['one_of', 'includes_all'];

/** Go's strings.TrimSpace, for the whitespace a form can hold. */
const trim = (s: string) => s.replace(/^\s+|\s+$/gu, '');
const runes = (s: string) => [...s].length;
const isDigits = (s: string) => s.length > 0 && [...s].every((c) => c >= '0' && c <= '9');

/** Reads a number with a point or a comma as its decimal sign, as Go's ParseNumber. */
export function parseNumber(value: string): number | null {
  const digits = value.startsWith('-') ? value.slice(1) : value;
  const sign = digits.search(/[.,]/);
  const whole = sign >= 0 ? digits.slice(0, sign) : digits;
  if (sign >= 0 && !isDigits(digits.slice(sign + 1))) return null;
  if (!isDigits(whole)) return null;
  const n = Number(value.replace(',', '.'));
  return Number.isFinite(n) ? n : null;
}

/** A calendar date written as YYYY-MM-DD that exists. */
export function isDay(value: string): boolean {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!m) return false;
  const [year, month, day] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const d = new Date(Date.UTC(year, month - 1, day));
  return d.getUTCFullYear() === year && d.getUTCMonth() === month - 1 && d.getUTCDate() === day;
}

const isId = (v: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v);

const isLabel = (v: string) =>
  runes(v) <= MAX_LABEL && v.toLowerCase() === v && [...v].every((c) => /^[\p{L}\p{Nd}_-]$/u.test(c));

function compareNumbers(op: Op, answer: string, value: string): boolean {
  const a = parseNumber(answer);
  const b = parseNumber(value);
  if (a === null || b === null) return false;
  switch (op) {
    case 'eq':
      return a === b;
    case 'ne':
      return a !== b;
    case 'lt':
      return a < b;
    case 'le':
      return a <= b;
    case 'gt':
      return a > b;
    case 'ge':
      return a >= b;
    default:
      return false;
  }
}

function dayOf(t: Test, today: string): string {
  const day = t.today ? today : (t.value ?? '');
  return isDay(day) ? day : '';
}

function compareDays(op: Op, answer: string, day: string): boolean {
  if (!day || !isDay(answer)) return false;
  switch (op) {
    case 'before':
      return answer < day;
    case 'after':
      return answer > day;
    case 'on_or_before':
      return answer <= day;
    case 'on_or_after':
      return answer >= day;
    default:
      return false;
  }
}

/** The items of an answer of several, or null when it has more than a question can hold. */
function itemsOf(value: string): Set<string> | null {
  const lines = value.split('\n');
  return lines.length > MAX_ITEMS ? null : new Set(lines.map(trim));
}

/**
 * Tests one answer. Every test of an empty answer is false, except "is empty", "is not", "≠",
 * "does not include" and "is no" (TPL-403); an unanswered yes/no question is a no.
 */
function answerHolds(t: Test, a: Answer | undefined, today: string): boolean {
  const kind = a?.kind ?? '';
  if (kind !== '' && !OPERATORS[kind].includes(t.op)) return false;
  const value = trim(a?.value ?? '');
  const empty = value === '';
  if (t.op === 'empty') return empty;
  if (t.op === 'not_empty') return !empty;
  if (t.op === 'yes') return value === 'yes';
  if (t.op === 'no') return value !== 'yes';
  if (empty) return t.op === 'is_not' || t.op === 'ne' || t.op === 'excludes';
  const target = t.value ?? '';
  switch (t.op) {
    case 'is':
    case 'is_not': {
      let same: boolean;
      if (kind === 'line' || kind === 'text' || kind === 'link')
        same = value.toLowerCase() === target.toLowerCase();
      else if (kind === 'date') same = dayOf(t, today) !== '' && value === dayOf(t, today);
      else same = value === target;
      return same === (t.op === 'is');
    }
    case 'contains':
      return value.toLowerCase().includes(target.toLowerCase());
    case 'eq':
    case 'ne':
    case 'lt':
    case 'le':
    case 'gt':
    case 'ge':
      return compareNumbers(t.op, value, target);
    case 'before':
    case 'after':
    case 'on_or_before':
    case 'on_or_after':
      return compareDays(t.op, value, dayOf(t, today));
    case 'one_of':
      return (t.values ?? []).includes(value);
    case 'includes':
    case 'excludes': {
      const items = itemsOf(value);
      return items !== null && items.has(target) === (t.op === 'includes');
    }
    case 'includes_all': {
      const items = itemsOf(value);
      const wanted = t.values ?? [];
      return items !== null && wanted.length > 0 && wanted.every((w) => items.has(w));
    }
    default:
      return false;
  }
}

function testHolds(t: Test, f: Facts): boolean {
  if ((t.values ?? []).length > MAX_VALUES) return false;
  switch (t.subject) {
    case 'space':
      return t.op === 'one_of' && !!f.space && (t.values ?? []).includes(f.space);
    case 'groups': {
      const group = t.value ?? '';
      if (group === '' || (f.missingGroups ?? []).includes(group)) return false;
      const member = (f.groups ?? []).includes(group);
      if (t.op === 'member') return member;
      if (t.op === 'not_member') return !member;
      return false;
    }
    case 'field':
      return answerHolds(t, f.answers[t.field ?? ''], f.today);
    default:
      return false;
  }
}

/** Decides a condition, as the server will; a part without one always applies. */
export function holds(c: Condition | undefined | null, f: Facts): boolean {
  if (!c) return true;
  const n = c.tests.length;
  if (n === 0 || n > MAX_TESTS) return false;
  if (c.match === 'all') return c.tests.every((t) => testHolds(t, f));
  if (c.match === 'any') return c.tests.some((t) => testHolds(t, f));
  return false;
}

/** A question a condition may test. */
export interface ConditionField {
  kind: ConditionKind;
  options?: string[];
}

/** What a condition may test where it stands; later questions are refused (TPL-421). */
export interface ConditionSchema {
  fields: Record<string, ConditionField>;
  later: Set<string>;
  space: boolean;
}

/** Why a condition is refused: a code under templates.conditionProblem, and its test. */
export interface ConditionProblem {
  problem: string;
  test: number;
}

function oneProblem(v: string, f: ConditionField): string {
  if (v === '' || /[\r\n]/.test(v) || runes(v) > MAX_VALUE_RUNES) return 'bad_value';
  switch (f.kind) {
    case 'number':
      return parseNumber(v) === null ? 'bad_value' : '';
    case 'choice':
    case 'choices':
      return (f.options ?? []).includes(v) ? '' : 'not_an_option';
    case 'label':
      return isLabel(v) ? '' : 'bad_value';
    case 'space':
    case 'groups':
      return isId(v) ? '' : 'bad_value';
    default:
      return '';
  }
}

function valueProblem(t: Test, f: ConditionField): string {
  const value = t.value ?? '';
  const values = t.values ?? [];
  if (NO_VALUE.includes(t.op)) return value !== '' || values.length > 0 || t.today ? 'bad_value' : '';
  if (LIST_VALUE.includes(t.op)) {
    if (value !== '' || t.today || values.length === 0 || values.length > MAX_VALUES) return 'bad_value';
    for (const [i, v] of values.entries()) {
      if (values.indexOf(v) !== i) return 'bad_value';
      const p = oneProblem(v, f);
      if (p) return p;
    }
    return '';
  }
  if (values.length > 0) return 'bad_value';
  if (f.kind === 'date')
    return !!t.today === (value !== '') || (!t.today && !isDay(value)) ? 'bad_value' : '';
  if (t.today) return 'bad_value';
  return oneProblem(value, f);
}

function testProblem(t: Test, s: ConditionSchema): string {
  let field: ConditionField;
  switch (t.subject as string) {
    case 'field': {
      const name = t.field ?? '';
      if (s.later.has(name)) return 'later_field';
      const known = s.fields[name];
      if (!known) return 'unknown_field';
      field = known;
      break;
    }
    case 'space':
      if (!s.space) return 'space_not_offered';
      field = { kind: 'space' };
      break;
    case 'groups':
      field = { kind: 'groups' };
      break;
    default:
      return 'bad_subject';
  }
  if (!OPERATORS[field.kind].includes(t.op)) return 'bad_op';
  return valueProblem(t, field);
}

/** Checks a condition as its author wrote it, as the server does before it saves (TPL-432). */
export function validateCondition(c: Condition, s: ConditionSchema): ConditionProblem | null {
  if (c.match !== 'all' && c.match !== 'any') return { problem: 'bad_match', test: -1 };
  if (c.tests.length === 0) return { problem: 'no_tests', test: -1 };
  if (c.tests.length > MAX_TESTS) return { problem: 'too_many_tests', test: MAX_TESTS };
  for (const [i, t] of c.tests.entries()) {
    const p = testProblem(t, s);
    if (p) return { problem: p, test: i };
  }
  return null;
}

/** How the sentence names what a test compares with: spaces and groups by name, days in words. */
export interface SentenceWords {
  name: (id: string) => string;
  date: (day: string) => string;
}

function valueWords(t: Test, words: SentenceWords): string {
  const list = ', ';
  if (LIST_VALUE.includes(t.op)) {
    return (t.values ?? []).map((v) => (t.subject === 'space' ? words.name(v) : v)).join(list);
  }
  if (t.today) return i18n.t('templates.condition.today');
  const value = t.value ?? '';
  if (t.subject === 'groups') return words.name(value);
  return isDay(value) ? words.date(value) : value;
}

/** One test in words, such as "Works on site is no". */
export function testSentence(t: Test, words: SentenceWords): string {
  const value = valueWords(t, words);
  if (t.subject === 'space') return i18n.t('templates.condition.space_one_of', { value });
  if (t.subject === 'groups') return i18n.t(`templates.condition.op_${t.op}`, { value });
  return i18n.t(`templates.condition.op_${t.op}`, { field: t.field ?? '', value });
}

/** What a condition means, in one sentence an approver reads (TPL-1014). */
export function conditionSentence(c: Condition, words: SentenceWords): string {
  const join = i18n.t(c.match === 'any' ? 'templates.condition.or' : 'templates.condition.and');
  return i18n.t('templates.condition.sentence', {
    tests: c.tests.map((t) => testSentence(t, words)).join(` ${join} `),
  });
}
