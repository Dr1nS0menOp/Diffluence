// SPDX-License-Identifier: Apache-2.0
// Works out formulas. It is a small parser of its own and never runs text as code:
// a formula can reach numbers and other cells, and nothing else (spec 019, D8).
import { isFormula, type Sheet } from './model';

export const ERROR_VALUE = '#ERROR';
export const CYCLE_VALUE = '#CYCLE';

const MAX_DEPTH = 200;
const DECIMALS = 12;
const LETTERS = 26;
const TOKEN = /\s*(\d+(?:\.\d+)?|[A-Za-z]+\d+|[A-Za-z]+|[-+*/(),;:])/y;
const CELL = /^([A-Za-z]+)(\d+)$/;

class FormulaError extends Error {
  constructor(readonly value: string) {
    super(value);
  }
}

type Reader = (row: number, column: number) => number | null;

function tokensOf(text: string): string[] {
  const tokens: string[] = [];
  TOKEN.lastIndex = 0;
  while (TOKEN.lastIndex < text.trimEnd().length) {
    const match = TOKEN.exec(text);
    if (!match?.[1]) throw new FormulaError(ERROR_VALUE);
    tokens.push(match[1]);
  }
  return tokens;
}

function cellOf(token: string): { row: number; column: number } | null {
  const match = CELL.exec(token);
  if (!match?.[1] || !match[2]) return null;
  let column = 0;
  for (const letter of match[1].toUpperCase()) column = column * LETTERS + letter.charCodeAt(0) - 64;
  return { row: Number(match[2]) - 1, column: column - 1 };
}

const FUNCTIONS: Record<string, (values: number[]) => number> = {
  SUM: (values) => values.reduce((a, b) => a + b, 0),
  AVERAGE: (values) => {
    if (values.length === 0) throw new FormulaError(ERROR_VALUE);
    return values.reduce((a, b) => a + b, 0) / values.length;
  },
  MIN: (values) => (values.length ? Math.min(...values) : 0),
  MAX: (values) => (values.length ? Math.max(...values) : 0),
  COUNT: (values) => values.length,
  ROUND: (values) => {
    const [value = 0, places = 0] = values;
    // Through text, because 2.345 * 100 is just under 234.5 in binary.
    return Number(`${Math.round(Number(`${value}e${places}`))}e${-places}`);
  },
};

class Parser {
  private at = 0;
  private depth = 0;

  constructor(
    private readonly tokens: string[],
    private readonly read: Reader,
  ) {}

  private peek(): string | undefined {
    return this.tokens[this.at];
  }

  private next(): string {
    const token = this.tokens[this.at];
    if (token === undefined) throw new FormulaError(ERROR_VALUE);
    this.at += 1;
    return token;
  }

  private expect(token: string) {
    if (this.next() !== token) throw new FormulaError(ERROR_VALUE);
  }

  run(): number {
    const value = this.sum();
    if (this.peek() !== undefined) throw new FormulaError(ERROR_VALUE);
    return value;
  }

  private sum(): number {
    let value = this.product();
    for (let op = this.peek(); op === '+' || op === '-'; op = this.peek()) {
      this.next();
      const right = this.product();
      value = op === '+' ? value + right : value - right;
    }
    return value;
  }

  private product(): number {
    let value = this.unary();
    for (let op = this.peek(); op === '*' || op === '/'; op = this.peek()) {
      this.next();
      const right = this.unary();
      if (op === '/' && right === 0) throw new FormulaError(ERROR_VALUE);
      value = op === '*' ? value * right : value / right;
    }
    return value;
  }

  private unary(): number {
    if (this.peek() === '-') {
      this.next();
      return -this.unary();
    }
    if (this.peek() === '+') this.next();
    return this.atom();
  }

  private atom(): number {
    this.depth += 1;
    if (this.depth > MAX_DEPTH) throw new FormulaError(ERROR_VALUE);
    const token = this.next();
    const value = this.valueOf(token);
    this.depth -= 1;
    return value;
  }

  private valueOf(token: string): number {
    if (token === '(') {
      const value = this.sum();
      this.expect(')');
      return value;
    }
    if (/^\d/.test(token)) return Number(token);
    const cell = cellOf(token);
    if (cell) return this.read(cell.row, cell.column) ?? 0;
    return this.call(token.toUpperCase());
  }

  private call(name: string): number {
    const run = FUNCTIONS[name];
    if (!run) throw new FormulaError(ERROR_VALUE);
    this.expect('(');
    const values: number[] = [];
    while (this.peek() !== ')') {
      values.push(...this.argument());
      if (this.peek() === ',' || this.peek() === ';') this.next();
      else if (this.peek() !== ')') throw new FormulaError(ERROR_VALUE);
    }
    this.expect(')');
    return run(values);
  }

  /** One argument: a range gives every number in it, and leaves out text and empty cells. */
  private argument(): number[] {
    const first = this.peek();
    const start = first === undefined ? null : cellOf(first);
    if (!start || this.tokens[this.at + 1] !== ':') return [this.sum()];
    this.at += 2;
    const end = cellOf(this.next());
    if (!end) throw new FormulaError(ERROR_VALUE);
    const values: number[] = [];
    for (let row = Math.min(start.row, end.row); row <= Math.max(start.row, end.row); row += 1) {
      for (let c = Math.min(start.column, end.column); c <= Math.max(start.column, end.column); c += 1) {
        const value = this.read(row, c);
        if (value !== null) values.push(value);
      }
    }
    return values;
  }
}

function numberOf(value: string): number | null {
  const trimmed = value.trim();
  if (trimmed === '') return null;
  const number = Number(trimmed);
  return Number.isFinite(number) ? number : null;
}

function shown(value: number): string {
  if (!Number.isFinite(value)) return ERROR_VALUE;
  return String(Number(value.toPrecision(DECIMALS)));
}

/** Gives a reader of shown values. Each formula is worked out once. */
export function evaluator(sheet: Sheet): (row: number, column: number) => string {
  const done = new Map<string, string>();
  const busy = new Set<string>();

  const cell = (row: number, column: number): string => {
    const id = sheet.columns[column]?.id;
    const raw = id === undefined ? undefined : sheet.rows[row]?.cells[id];
    if (raw === undefined) return '';
    if (!isFormula(raw)) return raw;
    const key = `${row}:${column}`;
    const known = done.get(key);
    if (known !== undefined) return known;
    if (busy.has(key)) throw new FormulaError(CYCLE_VALUE);
    if (busy.size > MAX_DEPTH) throw new FormulaError(ERROR_VALUE);
    busy.add(key);
    try {
      const reader: Reader = (r, c) => {
        const value = cell(r, c);
        if (value === ERROR_VALUE || value === CYCLE_VALUE) throw new FormulaError(value);
        return numberOf(value);
      };
      const value = shown(new Parser(tokensOf(raw.slice(1)), reader).run());
      done.set(key, value);
      return value;
    } catch (e) {
      if (!(e instanceof FormulaError)) throw e;
      done.set(key, e.value);
      return e.value;
    } finally {
      busy.delete(key);
    }
  };
  // Top to bottom first: a running total is then one short step per row, in any sort order.
  sheet.rows.forEach((row, r) => {
    sheet.columns.forEach((column, c) => {
      if (isFormula(row.cells[column.id] ?? '')) cell(r, c);
    });
  });
  return cell;
}
