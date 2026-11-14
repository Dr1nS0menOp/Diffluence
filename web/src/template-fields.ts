// SPDX-License-Identifier: Apache-2.0
/**
 * The kinds of question a template asks, and what each accepts (spec 074, TPL-300 to
 * TPL-325). This mirrors internal/templates/fields in Go, and both run the same table of
 * cases, internal/templates/fields/testdata/cases.json. The server decides; this only
 * helps the person before they send.
 */

export type FieldKind =
  | 'line'
  | 'text'
  | 'choice'
  | 'number'
  | 'date'
  | 'yesno'
  | 'page'
  | 'person'
  | 'people'
  | 'choices'
  | 'link'
  | 'label';

/** Every kind, in the order the template editor offers them. */
export const FIELD_KINDS: FieldKind[] = [
  'line',
  'text',
  'number',
  'date',
  'yesno',
  'choice',
  'choices',
  'person',
  'people',
  'page',
  'link',
  'label',
];

export type LineFormat = 'email' | 'integer' | 'alphanumeric';
export const LINE_FORMATS: LineFormat[] = ['email', 'integer', 'alphanumeric'];

/** A day: fixed, or the day the pages are made moved by days or weeks (TPL-304). */
export interface DateRef {
  fixed?: string;
  offset?: number;
  unit?: 'day' | 'week';
}

/** What a field shows filled in (TPL-316). Exactly one part is set. */
export interface FieldDefault {
  value?: string;
  date?: DateRef;
  creator?: boolean;
}

export interface FieldSpec {
  name: string;
  kind: FieldKind;
  required: boolean;
  help?: string;
  placeholder?: string;
  options?: string[];
  min_length?: number;
  max_length?: number;
  format?: LineFormat;
  min?: number;
  max?: number;
  decimals?: number;
  earliest?: DateRef;
  latest?: DateRef;
  min_choices?: number;
  max_choices?: number;
  space_id?: string;
  show?: 'words' | 'none' | 'commas' | 'list';
  default?: FieldDefault;
  /** The suggested person is no longer active (TPL-316). */
  default_gone?: boolean;
}

/** Why an answer or a field is refused: a code under templates.problem, and its values. */
export interface Problem {
  problem: string;
  params?: Record<string, string>;
}

const MAX_ANSWER = 2000;
const MAX_HELP = 300;
const MAX_PLACEHOLDER = 100;
const MAX_OPTIONS = 50;
const MAX_OPTION = 200;
const MAX_DECIMALS = 4;
export const MAX_PEOPLE = 20;
const MAX_NUMBER_DIGITS = 15;
const MAX_LABEL = 64;
const MAX_OFFSET_DAYS = 3650;
const DAYS_PER_WEEK = 7;
const ID_LENGTH = 36;

const problem = (code: string, params?: Record<string, string>): Problem =>
  params ? { problem: code, params } : { problem: code };

/** Counts characters as Go does: one per code point, not per UTF-16 unit. */
const runes = (s: string) => [...s].length;

/** Go's strings.TrimSpace, for the whitespace a form can hold. */
const trim = (s: string) => s.replace(/^\s+|\s+$/gu, '');

const isDigits = (s: string) => s.length > 0 && [...s].every((c) => c >= '0' && c <= '9');

/** An id in the one form the product writes. */
export function isId(value: string): boolean {
  return (
    value.length === ID_LENGTH &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)
  );
}

function lengthProblem(s: FieldSpec, value: string): Problem | null {
  const n = runes(value);
  const most = s.max_length !== undefined && s.max_length < MAX_ANSWER ? s.max_length : MAX_ANSWER;
  if (n > most) return problem('too_long', { max: String(most) });
  if (s.min_length !== undefined && n < s.min_length)
    return problem('too_short', { min: String(s.min_length) });
  return null;
}

const isLetterOrDigit = (c: string) => /^[\p{L}\p{Nd}]$/u.test(c);

function isEmail(value: string): boolean {
  const at = value.indexOf('@');
  if (at <= 0 || value.indexOf('@', at + 1) >= 0 || /\s/u.test(value)) return false;
  const domain = value.slice(at + 1);
  const dot = domain.lastIndexOf('.');
  return dot > 0 && dot < domain.length - 1 && !domain.startsWith('.');
}

function formatProblem(format: LineFormat | undefined, value: string): Problem | null {
  if (format === 'email' && !isEmail(value)) return problem('not_an_email');
  if (format === 'integer' && !isDigits(value.startsWith('-') ? value.slice(1) : value)) {
    return problem('not_a_whole_number');
  }
  if (format === 'alphanumeric' && ![...value].every(isLetterOrDigit)) return problem('not_letters_digits');
  return null;
}

/** Reads a number with a point or a comma as its decimal sign and no other separator. */
export function parseNumber(value: string): { n: number; decimals: number } | null {
  const digits = value.startsWith('-') ? value.slice(1) : value;
  const plain = digits.replace(/,/g, '.');
  const dot = plain.indexOf('.');
  const whole = dot < 0 ? plain : plain.slice(0, dot);
  const fraction = dot < 0 ? '' : plain.slice(dot + 1);
  if (!isDigits(whole) || whole.length > MAX_NUMBER_DIGITS || (dot >= 0 && !isDigits(fraction))) return null;
  return { n: Number(value.replace(',', '.')), decimals: fraction.length };
}

/** Writes a bound the way Go does, so both give the same words. */
const formatNumber = (n: number) => String(n);

function numberProblem(s: FieldSpec, value: string): Problem | null {
  const parsed = parseNumber(value);
  if (!parsed) return problem('not_a_number');
  const decimals = s.decimals ?? 0;
  if (parsed.decimals > decimals) return problem('too_many_decimals', { decimals: String(decimals) });
  if (s.min !== undefined && parsed.n < s.min) return problem('below_min', { min: formatNumber(s.min) });
  if (s.max !== undefined && parsed.n > s.max) return problem('above_max', { max: formatNumber(s.max) });
  return null;
}

/** Reads a calendar date. Dates are days, so they are counted in UTC and never shift. */
function parseDay(value: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!m) return null;
  const [year, month, day] = [Number(m[1]), Number(m[2]), Number(m[3])];
  const d = new Date(Date.UTC(year, month - 1, day));
  return d.getUTCFullYear() === year && d.getUTCMonth() === month - 1 && d.getUTCDate() === day ? d : null;
}

const DAY_TEXT_LENGTH = 'YYYY-MM-DD'.length;
const dayText = (d: Date) => d.toISOString().slice(0, DAY_TEXT_LENGTH);

/** The day a DateRef names, counted from today. */
export function resolveDate(ref: DateRef, today: string): string | null {
  if (ref.fixed) return parseDay(ref.fixed) ? ref.fixed : null;
  const base = parseDay(today);
  if (!base) return null;
  const days = (ref.offset ?? 0) * (ref.unit === 'week' ? DAYS_PER_WEEK : 1);
  base.setUTCDate(base.getUTCDate() + days);
  return dayText(base);
}

function dateProblem(s: FieldSpec, value: string, today: string): Problem | null {
  if (!parseDay(value)) return problem('not_a_date');
  const earliest = s.earliest ? resolveDate(s.earliest, today) : null;
  if (earliest && value < earliest) return problem('too_early', { date: earliest });
  const latest = s.latest ? resolveDate(s.latest, today) : null;
  if (latest && value > latest) return problem('too_late', { date: latest });
  return null;
}

function listAnswer(
  s: FieldSpec,
  value: string,
  check: (item: string) => Problem | null,
): [string, Problem | null] {
  const items: string[] = [];
  for (const line of value.split('\n')) {
    const item = trim(line);
    if (item === '') continue;
    const p = check(item);
    if (p) return ['', p];
    if (items.includes(item)) return ['', problem('repeated')];
    items.push(item);
  }
  if (s.kind === 'people' && items.length > MAX_PEOPLE)
    return ['', problem('too_many_people', { max: String(MAX_PEOPLE) })];
  if (s.min_choices !== undefined && items.length < s.min_choices) {
    return ['', problem('too_few_choices', { min: String(s.min_choices) })];
  }
  if (s.max_choices !== undefined && items.length > s.max_choices) {
    return ['', problem('too_many_choices', { max: String(s.max_choices) })];
  }
  return [items.join('\n'), null];
}

function linkProblem(value: string): Problem | null {
  if (runes(value) > MAX_ANSWER) return problem('too_long', { max: String(MAX_ANSWER) });
  // Whitespace and control characters, as Go's unicode.IsSpace and IsControl see them.
  if (/[\s\p{Cc}]/u.test(value)) return problem('not_a_link');
  try {
    const u = new URL(value);
    if ((u.protocol !== 'https:' && u.protocol !== 'http:') || u.host === '') return problem('not_a_link');
  } catch {
    // Not an address at all: that is the answer to give, not an error to pass on.
    return problem('not_a_link');
  }
  return null;
}

function labelAnswer(value: string): [string, Problem | null] {
  const label = value.toLowerCase().split(/\s+/u).filter(Boolean).join('-');
  if (runes(label) > MAX_LABEL || ![...label].every((c) => isLetterOrDigit(c) || c === '-' || c === '_')) {
    return ['', problem('not_a_label')];
  }
  return [label, null];
}

/**
 * Checks one answer and gives it as it lands in the page, or the problem with it. An empty
 * answer is no answer; several people or options come one per line.
 */
export function checkAnswer(s: FieldSpec, raw: string, today: string): [string, Problem | null] {
  const value = trim(raw);
  if (value === '') return ['', s.required && s.kind !== 'yesno' ? problem('required') : null];
  switch (s.kind) {
    case 'text':
      return [value, lengthProblem(s, value)];
    case 'choice':
      return (s.options ?? []).includes(value) ? [value, null] : ['', problem('not_an_option')];
    case 'number':
      return [value, numberProblem(s, value)];
    case 'date':
      return [value, dateProblem(s, value, today)];
    case 'yesno':
      return value === 'yes' || value === 'no' ? [value, null] : ['', problem('not_yes_no')];
    case 'page':
    case 'person':
      return isId(value) ? [value, null] : ['', problem('not_an_id')];
    case 'people':
      return listAnswer(s, value, (item) => (isId(item) ? null : problem('not_an_id')));
    case 'choices':
      return listAnswer(s, value, (item) =>
        (s.options ?? []).includes(item) ? null : problem('not_an_option'),
      );
    case 'link':
      return [value, linkProblem(value)];
    case 'label':
      return labelAnswer(value);
    default: {
      if (/[\r\n]/.test(value)) return ['', problem('one_line')];
      const p = lengthProblem(s, value);
      if (p) return ['', p];
      return [value, formatProblem(s.format, value)];
    }
  }
}

/** The answer a field starts with. */
export function defaultValue(s: FieldSpec, today: string, creatorId: string): string {
  const d = s.default;
  if (!d) return '';
  if (d.creator) return creatorId;
  if (d.date) return resolveDate(d.date, today) ?? '';
  return d.value ?? '';
}

type Setting =
  'placeholder' | 'length' | 'format' | 'options' | 'number' | 'dates' | 'choices' | 'space' | 'show';

/** What each kind may set beyond its name, required, help and default. */
export const KIND_SETTINGS: Record<FieldKind, Setting[]> = {
  line: ['placeholder', 'length', 'format'],
  text: ['placeholder', 'length'],
  choice: ['options'],
  number: ['placeholder', 'number'],
  date: ['dates'],
  yesno: ['show'],
  page: ['space'],
  person: [],
  people: [],
  choices: ['options', 'choices', 'show'],
  link: ['placeholder'],
  label: ['placeholder'],
};

function used(s: FieldSpec): Setting[] {
  const out: Setting[] = [];
  const add = (set: boolean, name: Setting) => {
    if (set && !out.includes(name)) out.push(name);
  };
  add(!!s.placeholder, 'placeholder');
  add(s.min_length !== undefined || s.max_length !== undefined, 'length');
  add(!!s.format, 'format');
  add((s.options ?? []).length > 0, 'options');
  add(s.min !== undefined || s.max !== undefined || (s.decimals ?? 0) !== 0, 'number');
  add(!!s.earliest || !!s.latest, 'dates');
  add(s.min_choices !== undefined || s.max_choices !== undefined, 'choices');
  add(!!s.space_id, 'space');
  add(!!s.show, 'show');
  return out;
}

function validRef(d: DateRef | undefined): boolean {
  if (!d) return true;
  if (d.fixed) return !!parseDay(d.fixed) && !d.offset && !d.unit;
  if (d.unit !== undefined && d.unit !== 'day' && d.unit !== 'week') return false;
  const days = (d.offset ?? 0) * (d.unit === 'week' ? DAYS_PER_WEEK : 1);
  return days >= -MAX_OFFSET_DAYS && days <= MAX_OFFSET_DAYS;
}

function optionsProblem(s: FieldSpec): Problem | null {
  if (s.kind !== 'choice' && s.kind !== 'choices') return null;
  const options = s.options ?? [];
  if (options.length === 0) return problem('needs_options');
  if (options.length > MAX_OPTIONS) return problem('too_many_options');
  const bad = options.some(
    (o, i) => trim(o) === '' || runes(o) > MAX_OPTION || /[\r\n]/.test(o) || options.indexOf(o) !== i,
  );
  return bad ? problem('bad_option') : null;
}

function settingProblem(s: FieldSpec): Problem | null {
  const low = s.min_length ?? 0;
  const high = s.max_length ?? MAX_ANSWER;
  if (low < 0 || high > MAX_ANSWER || low > high) return problem('bad_length');
  if (s.format && !LINE_FORMATS.includes(s.format)) return problem('bad_format');
  const decimals = s.decimals ?? 0;
  if (decimals < 0 || decimals > MAX_DECIMALS) return problem('bad_decimals');
  if (s.min !== undefined && s.max !== undefined && s.min > s.max) return problem('bad_range');
  if (!validRef(s.earliest) || !validRef(s.latest)) return problem('bad_date_bound');
  if (s.earliest?.fixed && s.latest?.fixed && s.earliest.fixed > s.latest.fixed)
    return problem('bad_date_bound');
  const count = (s.options ?? []).length;
  const fewest = s.min_choices ?? 0;
  const most = s.max_choices ?? count;
  if (fewest < 0 || most > count || fewest > most) return problem('bad_choices');
  if (s.space_id && !isId(s.space_id)) return problem('bad_space');
  const show = s.show;
  if (show && !(s.kind === 'yesno' ? ['words', 'none'] : ['commas', 'list']).includes(show))
    return problem('bad_show');
  return null;
}

function defaultProblem(s: FieldSpec, today: string): Problem | null {
  const d = s.default;
  if (!d) return null;
  const parts = [!!d.value, !!d.date, !!d.creator].filter(Boolean).length;
  const wrongPart =
    (!!d.creator && s.kind !== 'person' && s.kind !== 'people') ||
    !!d.date !== (s.kind === 'date') ||
    !validRef(d.date);
  if (parts !== 1 || wrongPart) return problem('bad_default');
  if (d.creator) return null;
  const [, p] = checkAnswer({ ...s, required: false }, defaultValue(s, today, ''), today);
  return p ? problem('bad_default') : null;
}

/** Checks a field as its author wrote it, as the server does before it saves. */
export function checkField(s: FieldSpec, today: string): Problem | null {
  const allowed = KIND_SETTINGS[s.kind] as Setting[] | undefined;
  if (!allowed) return problem('unknown_kind');
  if (s.kind === 'yesno' && s.required) return problem('yesno_required');
  if (runes(s.help ?? '') > MAX_HELP) return problem('help_too_long');
  if (runes(s.placeholder ?? '') > MAX_PLACEHOLDER) return problem('placeholder_too_long');
  if (used(s).some((setting) => !allowed.includes(setting))) return problem('setting_not_for_kind');
  return optionsProblem(s) ?? settingProblem(s) ?? defaultProblem(s, today);
}

/** Reads a question as the server sent it: a kind this release does not know asks for a line. */
export function fieldSpecOf(field: Omit<FieldSpec, 'kind'> & { kind: string }): FieldSpec {
  const kind = FIELD_KINDS.find((k) => k === field.kind) ?? 'line';
  return { ...field, kind };
}

/** Today in the reader's own calendar, as YYYY-MM-DD. The server's day decides on submit. */
export function localToday(): string {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}
