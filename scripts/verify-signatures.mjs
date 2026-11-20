#!/usr/bin/env node
// SPDX-License-Identifier: Apache-2.0
// Verifies the electronic signatures in an auditor's export of Kenwerk, outside Kenwerk
// (spec 064, DOC-742). It needs Node.js and nothing else. The procedure it follows is written
// down in docs/operations/regulated-use.md, so an auditor can check this script against it or
// follow the steps by hand.
//
//   node scripts/verify-signatures.mjs auditor-export.jsonl
//
// It exits 0 when every signature verifies, and 1 with the reasons when one does not.
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';

const sha256 = (data) => createHash('sha256').update(data).digest('hex');

// field writes one part of the canonical form: its name, its length in bytes, and the value.
function field(name, value) {
  const bytes = Buffer.from(value, 'utf8');
  return Buffer.concat([Buffer.from(`${name} ${bytes.length}\n`, 'utf8'), bytes, Buffer.from('\n', 'utf8')]);
}

// canonicalForm rebuilds form 1 of the signed content of a version.
function canonicalForm(content) {
  const files = [...content.files].sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  const parts = [
    Buffer.from('kenwerk-signed-content\n1\n', 'utf8'),
    field('title', content.title),
    field('schema', String(content.schema_version)),
    field('document', content.document),
    field('files', String(files.length)),
  ];
  for (const f of files) {
    parts.push(field('file', f.id), field('version', String(f.version)), field('name', f.filename), field('sha256', f.sha256));
  }
  return Buffer.concat(parts);
}

function main() {
  const path = process.argv[2];
  if (!path) {
    console.error('usage: node scripts/verify-signatures.mjs <auditor-export.jsonl>');
    process.exit(2);
  }
  const lines = readFileSync(path, 'utf8').split('\n').filter((l) => l.trim() !== '');
  const items = lines.map((l, i) => {
    try {
      return JSON.parse(l);
    } catch {
      console.error(`line ${i + 1} is not JSON`);
      process.exit(1);
    }
  });
  const problems = [];
  const head = items[0];
  const end = items[items.length - 1];
  if (!head || head.type !== 'export' || head.format !== 1) problems.push('the first line is not the header of an export of format 1');
  if (!end || end.type !== 'end' || end.lines !== items.length - 1) problems.push('the export is incomplete: its last line does not count the lines before it');

  const versions = new Map();
  const entries = new Map();
  const bySeq = new Map();
  for (const it of items) {
    if (it.type === 'version') versions.set(`${it.page_id} ${it.number}`, it);
    if (it.type === 'audit_entry') {
      entries.set(it.id, it);
      if (it.seq) bySeq.set(it.seq, it);
    }
  }

  // Every sealed entry: its hash covers the hash before it and its payload, and the next
  // entry in the export names it.
  for (const e of entries.values()) {
    if (!e.seq) continue;
    if (sha256(Buffer.concat([Buffer.from(e.prev_hash, 'hex'), Buffer.from(e.payload, 'utf8')])) !== e.hash) {
      problems.push(`audit entry ${e.seq} does not match its hash`);
    }
    const next = bySeq.get(e.seq + 1);
    if (next && next.prev_hash !== e.hash) problems.push(`audit entry ${e.seq + 1} does not follow entry ${e.seq}`);
  }

  let verified = 0;
  for (const it of items) {
    if (it.type !== 'signature') continue;
    const rec = it.record;
    const name = `signature ${rec.id} of version ${rec.version} of page ${rec.page_id}`;
    const version = versions.get(`${rec.page_id} ${rec.version}`);
    if (!version || !version.content) {
      problems.push(`${name}: the export holds no content of the version, so it cannot be checked here`);
      continue;
    }
    if (rec.content_form !== 1) {
      problems.push(`${name}: canonical form ${rec.content_form} is not known to this script`);
      continue;
    }
    const computed = sha256(canonicalForm(version.content));
    if (computed !== version.content_hash) problems.push(`${name}: the content does not give the hash the version keeps`);
    if (rec.content_hash !== version.content_hash) problems.push(`${name}: the record names another hash than the version keeps`);
    const entry = entries.get(rec.audit_event_id);
    if (!entry) {
      problems.push(`${name}: its audit entry is not in the export`);
      continue;
    }
    if (!entry.seq) {
      problems.push(`${name}: its audit entry waits to be sealed into the chain; export again in a moment`);
      continue;
    }
    let said;
    try {
      said = JSON.parse(entry.payload);
    } catch {
      problems.push(`${name}: its audit entry cannot be read`);
      continue;
    }
    const d = said.details || {};
    if (
      said.action !== 'page.signed' ||
      said.object_id !== rec.page_id ||
      said.actor_id !== rec.signer_id ||
      d.signature !== rec.id ||
      d.version !== rec.version ||
      d.content_hash !== rec.content_hash ||
      d.content_form !== rec.content_form ||
      d.meaning !== rec.meaning
    ) {
      problems.push(`${name}: its audit entry says something else`);
      continue;
    }
    if (computed === rec.content_hash) verified++;
  }

  if (problems.length > 0) {
    for (const p of problems) console.error(p);
    process.exit(1);
  }
  console.log(`${verified} signatures verified, ${entries.size} audit entries checked`);
}

main();
