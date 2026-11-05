// SPDX-License-Identifier: Apache-2.0
// A check nobody has seen fail proves nothing: every rule of the design standard
// is shown here to catch a violation, and to leave correct code alone.
import { describe, expect, it } from 'vitest';
import {
  checkContrast,
  checkCss,
  checkScreenPictures,
  checkSource,
  checkUnusedTokens,
  classNamesIn,
  contrastRatio,
  parseTokens,
} from './check-design.mjs';

const tokens = new Map([
  ['--color-ink', '#000000'],
  ['--space-2', '8px'],
  ['--text-ui', '1rem'],
  ['--font-sans', 'sans-serif'],
  ['--weight-strong', '600'],
  ['--leading-ui', '1.5'],
  ['--radius', '4px'],
  ['--shadow-menu', 'none'],
  ['--layer-menu', '10'],
]);

const inRule = (declaration) => `.thing {\n  ${declaration};\n}\n`;
const messages = (problems) => problems.map((p) => p.message).join('\n');

describe('style sheets', () => {
  it.each([
    ['a hexadecimal colour', 'color: #ff0000', /colour outside/],
    ['a short hexadecimal colour in a border', 'border: 1px solid #f00', /colour outside/],
    ['an rgb colour', 'background: rgb(0 0 0)', /colour outside/],
    ['a named colour', 'background: white', /colour outside/],
    ['a font size of its own', 'font-size: 14px', /--text-/],
    ['a font family of its own', 'font-family: Arial', /--font-/],
    ['a font shorthand', 'font: 12px Arial', /shorthand/],
    ['a weight of its own', 'font-weight: 500', /--weight-/],
    ['a line height of its own', 'line-height: 1.3', /--leading-/],
    ['a shadow of its own', 'box-shadow: 0 0 4px var(--color-ink)', /--shadow-/],
    ['a layer of its own', 'z-index: 99', /--layer-/],
    ['a corner of its own', 'border-radius: 3px', /--radius/],
    ['a margin of its own', 'margin-block: 13px', /--space-/],
    ['a padding of its own', 'padding: 1rem var(--space-2)', /--space-/],
    ['a gap of its own', 'gap: 5px', /--space-/],
    ['a physical margin', 'margin-left: var(--space-2)', /logical/],
    ['a physical width', 'width: 10rem', /logical/],
    ['a physical position', 'left: 0', /logical/],
    ['physical alignment', 'text-align: left', /start or end/],
    ['an unknown token', 'color: var(--color-nope)', /unknown token/],
    ['important', 'color: var(--color-ink) !important', /!important/],
  ])('refuses %s', (_name, declaration, expected) => {
    expect(messages(checkCss(inRule(declaration), tokens))).toMatch(expected);
  });

  it('accepts what the standard asks for', () => {
    const css = `.thing:hover {
      color: var(--color-ink);
      margin-block: 0 var(--space-2);
      padding: var(--space-2);
      inset-block-start: calc(100% + var(--space-2));
      border: 1px solid transparent;
      border-radius: var(--radius);
      font: inherit;
      font-size: var(--text-ui);
      text-align: start;
      box-shadow: none;
    }
    @media (max-width: 50rem) { .thing { gap: 0; } }`;
    expect(checkCss(css, tokens)).toEqual([]);
  });

  it('reports the line of a violation', () => {
    expect(checkCss(`/* one\n two */\n${inRule('color: #123456')}`, tokens)[0].line).toBe(4);
  });

  it('finds tokens nothing uses', () => {
    const unused = checkUnusedTokens(
      new Map([
        ['--space-1', '4px'],
        ['--space-2', '8px'],
      ]),
      ['.a { gap: var(--space-1); }'],
    );
    expect(messages(unused)).toBe('unused token: --space-2');
  });

  it('reads class names from selectors, not from values', () => {
    const names = classNamesIn('.a.b:hover, .c > .d { margin: 0.5rem; background: url(x.png); }');
    expect([...names].sort()).toEqual(['a', 'b', 'c', 'd']);
  });
});

describe('contrast', () => {
  it('computes the WCAG ratio', () => {
    expect(contrastRatio('#000000', '#ffffff')).toBeCloseTo(21, 1);
    expect(contrastRatio('#ffffff', '#ffffff')).toBeCloseTo(1, 5);
    expect(contrastRatio('#777777', '#ffffff')).toBeCloseTo(4.48, 1);
  });

  const css = `:root { --color-a: light-dark(#777777, #333333); --color-b: light-dark(#ffffff, #000000); }`;

  it('reads a colour with commas inside light-dark', () => {
    const { light, dark } = parseTokens(
      ':root { --color-a: light-dark(rgb(1, 2, 3), rgb(4, 5, 6)); --size: 1px; }',
    );
    expect(light.get('--color-a')).toBe('rgb(1, 2, 3)');
    expect(dark.get('--color-a')).toBe('rgb(4, 5, 6)');
    expect(dark.get('--size')).toBe('1px');
  });

  it('checks the light and the dark theme separately', () => {
    const problems = messages(checkContrast(parseTokens(css), [['a', 'b', 4.5]]));
    expect(problems).toMatch(/light: a on b is 4\.48:1/);
    expect(problems).toMatch(/dark: a on b is 1\.66:1/);
  });

  it('accepts a pair that passes', () => {
    expect(checkContrast(parseTokens(css), [['a', 'b', 1.5]])).toEqual([]);
  });

  it('refuses a pair without a token', () => {
    expect(messages(checkContrast(parseTokens(css), [['nope', 'b', 3]]))).toMatch(/no such token/);
  });
});

describe('pictures of screens', () => {
  const views = [
    'export function HomeView({ me }) {}\nfunction Helper() {}',
    'export function TrashView() {}',
  ];

  it('refuses a screen without a picture', () => {
    const problems = messages(checkScreenPictures(views, "test('HomeView: the start screen', ...)"));
    expect(problems).toMatch(/TrashView/);
    expect(problems).not.toMatch(/HomeView/);
  });

  it('accepts screens that all have one', () => {
    expect(checkScreenPictures(views, "test('HomeView: a', ...); test(`TrashView: b`, ...)")).toEqual([]);
  });
});

describe('source files', () => {
  const known = new Set(['row', 'primary', 'menu-list']);
  const check = (source, file = 'src/views/x.tsx') => messages(checkSource(file, source, known));

  it.each([
    ['an inline style', '<p style={{ margin: 0 }} />', /inline style/],
    ['a class without a style', '<p className="nope" />', /class without a style: nope/],
    ['such a class in an expression', '<p className={on ? "row" : "nope"} />', /nope/],
    ['such a class in a template', '<p className={`nope ${x}`} />', /nope/],
    ['an svg outside the icon set', '<svg viewBox="0 0 1 1" />', /svg outside/],
    ['window.confirm', 'window.confirm("x")', /browser dialog/],
    ['window.prompt', 'window.prompt("x")', /browser dialog/],
    ['alert', 'alert("x")', /browser dialog/],
    ['an emoji in text', '<p>🔗</p>', /glyph/],
    ['an arrow in text', '<p>a → b</p>', /glyph/],
    ['a glyph in a string', 'const icon = "☑";', /glyph/],
    ['a colour in code', 'const colour = "#1d5fbf";', /colour outside/],
  ])('refuses %s', (_name, source, expected) => {
    expect(check(source)).toMatch(expected);
  });

  it('accepts correct code, and half a class name in a template', () => {
    const source = '<a href="#main" className={`row tint-${n} primary`}>{t("x")} · {y}</a>';
    expect(check(source)).toBe('');
  });

  it('does not take the string a condition compares for a class', () => {
    expect(check('<p className={target === "in:" + id ? "row" : "primary"} />')).toBe('');
  });

  it('allows svg in the icon set only', () => {
    expect(check('<svg viewBox="0 0 1 1" />', 'src/components/icons.tsx')).toBe('');
  });
});
