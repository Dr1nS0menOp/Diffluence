#!/usr/bin/env node
// SPDX-License-Identifier: Apache-2.0
// Every user-visible word goes through the translation files (CLAUDE.md). This
// walks the JSX and fails on hard-coded text, and on translation keys that do
// not exist. Run from web/: npm run strings
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import ts from 'typescript';

const messages = JSON.parse(readFileSync('src/locales/en.json', 'utf8'));
const hasKey = (key) => key.split('.').reduce((o, p) => (o && typeof o === 'object' ? o[p] : undefined), messages) !== undefined;

// Symbols and code are not language: toolbar glyphs and separators.
const neutral = /^[\s\p{P}\p{S}\d]*$/u;
const textAttributes = new Set(['title', 'placeholder', 'aria-label', 'alt', 'label']);
const glyphTool = /^(B|I|U|S|H\d|\+R|\+C|−R|−C|`)$/;
let problems = 0;

function report(file, node, text) {
  const { line } = node.getSourceFile().getLineAndCharacterOfPosition(node.getStart());
  console.error(`${file}:${line + 1}: ${text}`);
  problems++;
}

function check(file) {
  const src = ts.createSourceFile(file, readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  const visit = (node) => {
    if (ts.isJsxText(node) && !neutral.test(node.text)) {
      report(file, node, `hard-coded text: ${node.text.trim()}`);
    }
    if (ts.isJsxAttribute(node) && textAttributes.has(node.name.getText()) && node.initializer && ts.isStringLiteral(node.initializer)) {
      if (!neutral.test(node.initializer.text)) report(file, node, `hard-coded ${node.name.getText()}: ${node.initializer.text}`);
    }
    if (ts.isJsxAttribute(node) && node.name.getText() === 'text' && node.initializer && ts.isStringLiteral(node.initializer)) {
      const v = node.initializer.text;
      if (!neutral.test(v) && !glyphTool.test(v)) report(file, node, `hard-coded text: ${v}`);
    }
    if (ts.isCallExpression(node) && node.expression.getText() === 't' && node.arguments[0] && ts.isStringLiteral(node.arguments[0])) {
      const key = node.arguments[0].text;
      if (!hasKey(key)) report(file, node, `unknown translation key: ${key}`);
    }
    ts.forEachChild(node, visit);
  };
  visit(src);
}

function walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path);
    else if (path.endsWith('.tsx') && !path.endsWith('.test.tsx')) check(path);
  }
}

walk('src');
if (problems) {
  console.error(`${problems} problem(s)`);
  process.exit(1);
}
console.log('UI strings OK');
