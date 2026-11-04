#!/usr/bin/env node
// SPDX-License-Identifier: Apache-2.0
// The design standard (docs/design/README.md) as a check, so that it holds for
// every change and does not depend on anyone remembering it. Run from web/:
//   npm run design
// check-design.test.mjs proves that each rule really fails on a violation.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import ts from 'typescript';

const DESIGN_DIRECTORY = 'src/design';
const TOKENS_FILE = 'src/design/tokens.css';
const ICONS_FILE = 'src/components/icons.tsx';
const VIEWS_DIRECTORY = 'src/views';
const PICTURES_FILE = 'e2e/0-design.spec.ts';

const NORMAL_TEXT = 4.5;
const LARGE_OR_CONTROL = 3;

// Every pair of colours the interface puts on top of each other for reading.
export const CONTRAST_PAIRS = [
  ['ink', 'surface', NORMAL_TEXT],
  ['ink', 'side', NORMAL_TEXT],
  ['ink', 'subtle', NORMAL_TEXT],
  ['ink', 'subtle-hover', NORMAL_TEXT],
  ['ink', 'code', NORMAL_TEXT],
  ['ink', 'ok-tint', NORMAL_TEXT],
  ['ink', 'danger-tint', NORMAL_TEXT],
  ['ink', 'added', NORMAL_TEXT],
  ['ink', 'removed', NORMAL_TEXT],
  ['strong', 'surface', NORMAL_TEXT],
  ['muted', 'surface', NORMAL_TEXT],
  ['muted', 'side', NORMAL_TEXT],
  ['muted', 'subtle', NORMAL_TEXT],
  ['muted', 'code', NORMAL_TEXT],
  ['accent', 'surface', NORMAL_TEXT],
  ['accent', 'side', NORMAL_TEXT],
  ['accent', 'accent-tint', NORMAL_TEXT],
  ['accent-hover', 'surface', NORMAL_TEXT],
  ['on-accent', 'accent', NORMAL_TEXT],
  ['on-accent', 'accent-hover', NORMAL_TEXT],
  ['danger', 'surface', NORMAL_TEXT],
  ['danger', 'danger-tint', NORMAL_TEXT],
  ['on-lozenge', 'lozenge', NORMAL_TEXT],
  ['on-tint', 'tint-1', NORMAL_TEXT],
  ['on-tint', 'tint-2', NORMAL_TEXT],
  ['on-tint', 'tint-3', NORMAL_TEXT],
  ['on-tint', 'tint-4', NORMAL_TEXT],
  ['syntax-keyword', 'code', NORMAL_TEXT],
  ['syntax-string', 'code', NORMAL_TEXT],
  ['syntax-number', 'code', NORMAL_TEXT],
  ['syntax-title', 'code', NORMAL_TEXT],
  ['field', 'surface', LARGE_OR_CONTROL],
  ['accent', 'subtle', LARGE_OR_CONTROL],
];

const NAMED_COLOURS = /(?<![\w-])(white|black|red|green|blue|gray|grey|yellow|orange|purple|pink|brown|silver|navy|teal)(?![\w-])/i;
const COLOUR_LITERAL = /#[0-9a-f]{3,8}\b|\b(rgba?|hsla?|hwb|lab|lch|oklab|oklch)\(/i;
const LITERAL_LENGTH = /(?<![\w.-])-?\d*\.?\d+(px|rem|em)\b/;
const SPACING_PROPERTY = /^(margin|padding|gap|row-gap|column-gap|inset)(-|$)/;
const PHYSICAL_PROPERTY =
  /^((margin|padding|border)-(left|right|top|bottom)(-|$)|left$|right$|top$|bottom$|(min-|max-)?(width|height)$)/;
const PICTOGRAPH = /[\p{Extended_Pictographic}←-⇿■-◿☀-➿]/u;
const BROWSER_DIALOGS = new Set(['alert', 'confirm', 'prompt']);

// A property that may only take a token of the given prefix, or one of the words.
const TOKEN_ONLY = new Map([
  ['font-size', { prefix: '--text-', words: ['inherit'] }],
  ['font-family', { prefix: '--font-', words: ['inherit'] }],
  ['font-weight', { prefix: '--weight-', words: ['inherit'] }],
  ['line-height', { prefix: '--leading-', words: ['inherit'] }],
  ['box-shadow', { prefix: '--shadow-', words: ['none'] }],
  ['z-index', { prefix: '--layer-', words: ['auto'] }],
]);

const blankComments = (css) => css.replace(/\/\*[\s\S]*?\*\//g, (comment) => comment.replace(/[^\n]/g, ' '));

const lineOf = (text, index) => text.slice(0, index).split('\n').length;

/** The custom properties of the light theme and the dark theme. */
export function parseTokens(css) {
  const clean = blankComments(css);
  const darkStart = clean.indexOf('@media (prefers-color-scheme: dark)');
  const collect = (text) => new Map([...text.matchAll(/(--[\w-]+):\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]));
  const light = collect(darkStart === -1 ? clean : clean.slice(0, darkStart));
  const dark = new Map([...light, ...collect(darkStart === -1 ? '' : clean.slice(darkStart))]);
  return { light, dark };
}

function relativeLuminance(hex) {
  const value = hex.replace('#', '');
  const full = value.length === 3 ? [...value].map((c) => c + c).join('') : value;
  const channel = (start) => {
    const c = parseInt(full.slice(start, start + 2), 16) / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  };
  return 0.2126 * channel(0) + 0.7152 * channel(2) + 0.0722 * channel(4);
}

/** The WCAG contrast ratio of two hexadecimal colours. */
export function contrastRatio(first, second) {
  const [lighter, darker] = [relativeLuminance(first), relativeLuminance(second)].sort((a, b) => b - a);
  return (lighter + 0.05) / (darker + 0.05);
}

export function checkContrast(tokens, pairs = CONTRAST_PAIRS) {
  const problems = [];
  for (const [theme, values] of Object.entries(tokens)) {
    for (const [foreground, background, minimum] of pairs) {
      const a = values.get(`--color-${foreground}`);
      const b = values.get(`--color-${background}`);
      if (!a || !b) {
        problems.push({ line: 1, message: `${theme}: contrast pair ${foreground} on ${background} has no such token` });
        continue;
      }
      const ratio = contrastRatio(a, b);
      if (ratio < minimum) {
        problems.push({ line: 1, message: `${theme}: ${foreground} on ${background} is ${ratio.toFixed(2)}:1, needs ${minimum}:1` });
      }
    }
  }
  return problems;
}

function declarationProblem(property, value, tokens) {
  const bareValue = value.replace(/var\(--[\w-]+\)/g, '');
  if (COLOUR_LITERAL.test(value) || NAMED_COLOURS.test(bareValue)) return `colour outside the tokens: ${property}: ${value}`;
  if (value.includes('!important')) return `!important: ${property}: ${value}`;
  if (PHYSICAL_PROPERTY.test(property)) return `physical property, use the logical one: ${property}`;
  if (/^(text-align|float|clear)$/.test(property) && /^(left|right)$/.test(value)) {
    return `physical value, use start or end: ${property}: ${value}`;
  }
  if (property === 'font' && value !== 'inherit') return `font shorthand outside the tokens: ${value}`;
  const rule = TOKEN_ONLY.get(property);
  if (rule && !rule.words.includes(value) && !new RegExp(`^var\\(${rule.prefix}[\\w-]+\\)$`).test(value)) {
    return `${property} must use a ${rule.prefix}* token: ${value}`;
  }
  if (/radius$/.test(property) && value !== '0' && !/^var\(--radius[\w-]*\)$/.test(value)) {
    return `${property} must use a --radius token: ${value}`;
  }
  if (SPACING_PROPERTY.test(property) && LITERAL_LENGTH.test(bareValue)) {
    return `spacing must use a --space-* token: ${property}: ${value}`;
  }
  const unknown = [...value.matchAll(/var\((--[\w-]+)/g)].find((m) => !tokens.has(m[1]));
  return unknown ? `unknown token: ${unknown[1]}` : null;
}

/** Checks one style sheet against the tokens of the light theme. */
export function checkCss(css, tokens) {
  const clean = blankComments(css);
  const problems = [];
  for (const m of clean.matchAll(/(?<=[{;}\s])([a-z-]+)\s*:\s*([^;{}]+?)\s*(?=[;}])/g)) {
    const message = declarationProblem(m[1], m[2], tokens);
    if (message) problems.push({ line: lineOf(clean, m.index), message });
  }
  return problems;
}

/** Every token must be used, so the list stays the truth about the design. */
export function checkUnusedTokens(tokens, styleSheets) {
  const everything = styleSheets.join('\n');
  return [...tokens.keys()]
    .filter((name) => !everything.includes(`var(${name})`))
    .map((name) => ({ line: 1, message: `unused token: ${name}` }));
}

export function classNamesIn(css) {
  const clean = blankComments(css).replace(/\{[^{}]*\}/g, '{}');
  return new Set([...clean.matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)].map((m) => m[1]));
}

function classesInLiteral(node) {
  if (ts.isStringLiteralLike(node)) return node.text.split(/\s+/);
  const text = node.text ?? '';
  const words = text.split(/\s+/);
  // A word touching a ${} is only half a class name.
  if (!ts.isTemplateHead(node) && !/^\s/.test(text)) words.shift();
  if (!ts.isTemplateTail(node) && !/\s$/.test(text)) words.pop();
  return words;
}

function collectClasses(node, found) {
  if (ts.isConditionalExpression(node)) {
    collectClasses(node.whenTrue, found);
    collectClasses(node.whenFalse, found);
    return;
  }
  if (ts.isStringLiteralLike(node) || ts.isTemplateHead(node) || ts.isTemplateMiddle(node) || ts.isTemplateTail(node)) {
    found.push(...classesInLiteral(node).filter(Boolean));
  }
  ts.forEachChild(node, (child) => collectClasses(child, found));
}

/** Checks one source file: no colours, glyphs or browser dialogs of its own. */
export function checkSource(file, source, classNames) {
  const tree = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const problems = [];
  const report = (node, message) => {
    problems.push({ line: tree.getLineAndCharacterOfPosition(node.getStart()).line + 1, message });
  };
  const visit = (node) => {
    if (ts.isJsxAttribute(node)) {
      const name = node.name.getText();
      if (name === 'style') report(node, 'inline style: use a class from src/design');
      if (name === 'className' && node.initializer) {
        const found = [];
        collectClasses(node.initializer, found);
        for (const className of found) {
          if (!classNames.has(className)) report(node, `class without a style: ${className}`);
        }
      }
    }
    if ((ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) && node.tagName.getText() === 'svg') {
      if (!file.replaceAll('\\', '/').endsWith(ICONS_FILE)) report(node, `svg outside ${ICONS_FILE}`);
    }
    if (ts.isCallExpression(node) && BROWSER_DIALOGS.has(node.expression.getText().replace(/^window\./, ''))) {
      report(node, `browser dialog: use ConfirmDialog or Dialog instead of ${node.expression.getText()}`);
    }
    if (ts.isJsxText(node) || ts.isStringLiteralLike(node)) {
      if (PICTOGRAPH.test(node.text)) report(node, `glyph used as an icon: ${node.text.trim()}`);
      if (/^(#[0-9a-f]{3,8}|(rgb|hsl)a?\(.*)$/i.test(node.text.trim())) report(node, `colour outside the tokens: ${node.text}`);
    }
    ts.forEachChild(node, visit);
  };
  visit(tree);
  return problems;
}

/** A screen without an approved picture could drift without anyone noticing. */
export function checkScreenPictures(viewSources, pictureTests) {
  const screens = viewSources.flatMap((source) => [...source.matchAll(/export function (\w+View)\(/g)].map((m) => m[1]));
  return screens
    .filter((screen) => !pictureTests.includes(`${screen}:`))
    .map((screen) => ({ line: 1, message: `screen without a picture: add a test named '${screen}: ...'` }));
}

function filesBelow(directory) {
  return readdirSync(directory).flatMap((name) => {
    const path = join(directory, name);
    return statSync(path).isDirectory() ? filesBelow(path) : [path];
  });
}

function checkProject() {
  const report = (file, problems) => problems.map((p) => `${file}:${p.line}: ${p.message}`);
  const tokenCss = readFileSync(TOKENS_FILE, 'utf8');
  const tokens = parseTokens(tokenCss);
  const styleFiles = filesBelow(DESIGN_DIRECTORY).filter((f) => f.endsWith('.css'));
  const sheets = new Map(styleFiles.map((file) => [file, readFileSync(file, 'utf8')]));
  const classNames = new Set([...sheets.values()].flatMap((css) => [...classNamesIn(css)]));
  const outsideDesign = filesBelow('src').filter((f) => f.endsWith('.css') && !styleFiles.includes(f));
  const sources = filesBelow('src').filter((f) => /\.tsx?$/.test(f) && !/\.test\.tsx?$/.test(f));
  const views = filesBelow(VIEWS_DIRECTORY).map((file) => readFileSync(file, 'utf8'));
  return [
    ...report(PICTURES_FILE, checkScreenPictures(views, readFileSync(PICTURES_FILE, 'utf8'))),
    ...report(TOKENS_FILE, checkContrast(tokens)),
    ...report(TOKENS_FILE, checkUnusedTokens(tokens.light, [...sheets.values()])),
    ...outsideDesign.map((file) => `${file}: style sheet outside ${DESIGN_DIRECTORY}`),
    ...styleFiles
      .filter((file) => file.replaceAll('\\', '/') !== TOKENS_FILE)
      .flatMap((file) => report(file, checkCss(sheets.get(file), tokens.light))),
    ...sources.flatMap((file) => report(file, checkSource(file, readFileSync(file, 'utf8'), classNames))),
  ];
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const problems = checkProject();
  for (const problem of problems) console.error(problem);
  if (problems.length > 0) {
    console.error(`${problems.length} design problem(s). The standard is docs/design/README.md.`);
    process.exit(1);
  }
  console.log('Design OK');
}
