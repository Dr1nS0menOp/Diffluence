// SPDX-License-Identifier: Apache-2.0
// Keeps the shared document of editing together within what the editor's schema allows (SEC-546).
// The service passes changes on without a schema, so a client that skips the editor can send a node,
// an attribute or a link that the schema or the server's gate would refuse. The editor never renders
// those, but they would stay in the shared state, and the server then refuses to store the draft for
// everyone. So every honest browser takes them out of the shared document as soon as they arrive.
import type { MarkType, NodeType, Schema } from '@tiptap/pm/model';
import * as Y from 'yjs';

/** Where links and addresses may lead, as in the link rules of the editor and the server's gate. */
const SAFE_SCHEMES = new Set(['http:', 'https:', 'mailto:']);

/** Whether an attribute holds an address, which must lead somewhere safe, such as `href` or `button_href`. */
const isAddressAttribute = (name: string) => /(^|_)(href|src|url)$/.test(name);

/** The origin of the changes this module makes, so they are never cleaned twice. */
export const CLEAN_ORIGIN = 'diffluence-clean';

/** Whether an address leads to an allowed scheme. A relative address or an anchor is allowed. */
export function isSafeAddress(value: unknown): boolean {
  if (typeof value !== 'string') return true;
  const trimmed = value.trim();
  if (trimmed === '' || trimmed.startsWith('/') || trimmed.startsWith('#') || trimmed.startsWith('?'))
    return true;
  // Control characters and spaces can hide a scheme from a simple check, as in "java\tscript:".
  const compact = trimmed.replace(/[\u0000- \u007f]/g, '');
  const scheme = /^([a-z][a-z0-9+.-]*):/i.exec(compact);
  if (!scheme) return true;
  return SAFE_SCHEMES.has(`${(scheme[1] ?? '').toLowerCase()}:`);
}

function allowedAttributes(type: NodeType | MarkType): Set<string> {
  return new Set(Object.keys(type.spec.attrs ?? {}));
}

function cleanElement(element: Y.XmlElement, type: NodeType): boolean {
  let changed = false;
  const allowed = allowedAttributes(type);
  for (const [name, value] of Object.entries(element.getAttributes())) {
    if (!allowed.has(name) || (isAddressAttribute(name) && !isSafeAddress(value))) {
      element.removeAttribute(name);
      changed = true;
    }
  }
  return changed;
}

function cleanMark(schema: Schema, name: string, attrs: unknown): Record<string, unknown> | null | undefined {
  const type = schema.marks[name];
  if (!type) return null;
  if (!attrs || typeof attrs !== 'object') return undefined;
  const allowed = allowedAttributes(type);
  const entries = Object.entries(attrs as Record<string, unknown>);
  // A link to an unsafe place loses the whole mark, and keeps its words.
  if (entries.some(([key, value]) => isAddressAttribute(key) && !isSafeAddress(value))) return null;
  if (entries.every(([key]) => allowed.has(key))) return undefined;
  return Object.fromEntries(entries.filter(([key]) => allowed.has(key)));
}

function cleanText(text: Y.XmlText, schema: Schema): boolean {
  let changed = false;
  let index = 0;
  for (const op of text.toDelta() as { insert: unknown; attributes?: Record<string, unknown> }[]) {
    const length = typeof op.insert === 'string' ? op.insert.length : 1;
    const update: Record<string, unknown> = {};
    for (const [name, attrs] of Object.entries(op.attributes ?? {})) {
      const cleaned = cleanMark(schema, name, attrs);
      if (cleaned !== undefined) update[name] = cleaned;
    }
    if (Object.keys(update).length > 0) {
      text.format(index, length, update);
      changed = true;
    }
    index += length;
  }
  return changed;
}

function cleanChildren(parent: Y.XmlFragment | Y.XmlElement, schema: Schema): boolean {
  let changed = false;
  // Backwards, so removing a child does not move the ones still to come.
  const children = parent.toArray();
  for (let i = children.length - 1; i >= 0; i--) {
    const child = children[i];
    if (child instanceof Y.XmlText) {
      changed = cleanText(child, schema) || changed;
    } else if (child instanceof Y.XmlElement) {
      const type = schema.nodes[child.nodeName];
      if (!type) {
        parent.delete(i, 1);
        changed = true;
        continue;
      }
      changed = cleanElement(child, type) || changed;
      changed = cleanChildren(child, schema) || changed;
    }
  }
  return changed;
}

/** Takes out of a shared document what the schema does not allow. Answers whether anything changed. */
export function cleanShared(doc: Y.Doc, schema: Schema, fragmentName = 'default'): boolean {
  let changed = false;
  doc.transact(() => {
    changed = cleanChildren(doc.getXmlFragment(fragmentName), schema);
  }, CLEAN_ORIGIN);
  return changed;
}

/**
 * Cleans the shared document now, and again after each change from elsewhere. Changes of this
 * browser's own editor already passed its schema, so only those from others are looked at.
 * Answers a function that stops it.
 */
export function keepSharedClean(doc: Y.Doc, schema: Schema, fragmentName = 'default'): () => void {
  let queued = false;
  const clean = () => {
    queued = false;
    cleanShared(doc, schema, fragmentName);
  };
  const onTransaction = (transaction: Y.Transaction) => {
    if (transaction.local || transaction.origin === CLEAN_ORIGIN || queued) return;
    queued = true;
    // After the editor has drawn the change, so both see the same document.
    queueMicrotask(clean);
  };
  clean();
  doc.on('afterTransaction', onTransaction);
  return () => doc.off('afterTransaction', onTransaction);
}
