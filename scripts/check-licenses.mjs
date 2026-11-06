#!/usr/bin/env node
// SPDX-License-Identifier: Apache-2.0
// Licence gate (blueprint 02). Everything that ships must be MIT, BSD, Apache-2.0
// or ISC. Tools that only run during development are reported, and allowed a few
// extra permissive licences, because they are never part of what we distribute.
// Run it in the development toolbox: scripts/dev node scripts/check-licenses.mjs
import { execFileSync } from 'node:child_process';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

const shipped = /^(MIT|ISC|0BSD|Apache-2\.0|BSD-2-Clause|BSD-3-Clause)$/;
const toolsOnly = /^(MIT|ISC|0BSD|Apache-2\.0|BSD-2-Clause|BSD-3-Clause|MPL-2\.0|Unlicense|BlueOak-1\.0\.0|CC0-1\.0|CC-BY-4\.0|Python-2\.0)$/;
const copyleft = /\b(A?GPL|LGPL)/i;
let failures = 0;

function fail(message) {
  console.error('FAIL ' + message);
  failures++;
}

function licenceOf(dir) {
  const j = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'));
  return j.license ?? (j.licenses ?? []).map((l) => l.type).join(' OR ') ?? 'UNKNOWN';
}

function allowed(licence, pattern) {
  // "(MIT OR Apache-2.0)": any one option is enough.
  return licence
    .replace(/[()]/g, '')
    .split(/\s+OR\s+/)
    .some((part) => pattern.test(part.trim()));
}

function npmTree(args) {
  const out = execFileSync('npm', ['ls', '--all', '--parseable', ...args], {
    cwd: 'web',
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'ignore'],
    maxBuffer: 1 << 26,
  });
  return new Set(out.split('\n').filter((l) => l.includes('node_modules')));
}

const production = npmTree(['--omit=dev']);
const everything = npmTree([]);
const tools = new Map();
for (const dir of everything) {
  const licence = licenceOf(dir);
  const name = dir.slice(dir.lastIndexOf('node_modules/') + 13);
  if (production.has(dir)) {
    if (!allowed(licence, shipped)) fail(`npm package ${name} ships with licence ${licence}`);
  } else if (!allowed(licence, toolsOnly) || copyleft.test(licence)) {
    fail(`development tool ${name} has licence ${licence}`);
  } else if (!allowed(licence, shipped)) {
    tools.set(name, licence);
  }
}
console.log(`npm: ${production.size} shipped packages, ${everything.size - production.size} development tools`);
for (const [name, licence] of tools) console.log(`  tool only, not shipped: ${name} (${licence})`);

// The collab service ships in its own container. Its lock file names every package.
const collabLock = JSON.parse(readFileSync('collab/package-lock.json', 'utf8'));
const collabPackages = Object.entries(collabLock.packages).filter(([path]) => path.includes('node_modules/'));
for (const [path, entry] of collabPackages) {
  if (!allowed(entry.license ?? 'UNKNOWN', shipped)) fail(`collab package ${path} has licence ${entry.license}`);
}
console.log(`collab: ${collabPackages.length} shipped packages`);

// Go: every module compiled into the binary.
const list = execFileSync('go', ['list', '-deps', '-f', '{{with .Module}}{{.Path}} {{.Dir}}{{end}}', './cmd/diffluence'], {
  encoding: 'utf8',
  maxBuffer: 1 << 26,
});
const modules = new Map();
for (const line of list.split('\n')) {
  const [path, dir] = line.split(' ');
  if (path && dir && !path.startsWith('github.com/diffluence/')) modules.set(path, dir);
}
for (const [path, dir] of modules) {
  const file = readdirSync(dir).find((f) => /^(licen[cs]e|copying)/i.test(f));
  if (!file || !existsSync(join(dir, file))) {
    fail(`Go module ${path} has no licence file`);
    continue;
  }
  const text = readFileSync(join(dir, file), 'utf8');
  if (/GNU (AFFERO |LESSER )?GENERAL PUBLIC LICENSE/i.test(text)) fail(`Go module ${path} is under a GPL licence`);
  else if (!/Permission is hereby granted|Apache License|Redistribution and use|Permission to use, copy, modify/i.test(text)) {
    fail(`Go module ${path}: licence not recognised (${file})`);
  }
}
console.log(`go: ${modules.size} modules in the binary`);

if (failures) {
  console.error(`${failures} licence problem(s)`);
  process.exit(1);
}
console.log('licences OK');
