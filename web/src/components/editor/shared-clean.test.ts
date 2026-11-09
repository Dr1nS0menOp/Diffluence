// SPDX-License-Identifier: Apache-2.0
import { getSchema } from '@tiptap/core';
import { describe, expect, it } from 'vitest';
import * as Y from 'yjs';
import { documentExtensions } from './extensions';
import { cleanShared, isSafeAddress, keepSharedClean } from './shared-clean';

const schema = getSchema(documentExtensions({ shared: true }));

function paragraphWith(doc: Y.Doc, words: string, attributes?: Record<string, unknown>) {
  const paragraph = new Y.XmlElement('paragraph');
  const text = new Y.XmlText();
  paragraph.insert(0, [text]);
  doc.getXmlFragment('default').push([paragraph]);
  text.insert(0, words, attributes);
  return text;
}

describe('keeping a shared document within the schema', () => {
  it('removes a node the schema does not know, with what is in it', () => {
    const doc = new Y.Doc();
    const script = new Y.XmlElement('script');
    script.insert(0, [new Y.XmlText('alert(1)')]);
    doc.getXmlFragment('default').push([script]);
    expect(cleanShared(doc, schema)).toBe(true);
    expect(doc.getXmlFragment('default').toString()).not.toContain('script');
  });

  it('removes an attribute a known node does not have', () => {
    const doc = new Y.Doc();
    const image = new Y.XmlElement('image');
    image.setAttribute('attachment_id', 'a1');
    image.setAttribute('onerror', 'alert(1)');
    image.setAttribute('src', 'https://example.org/x.png');
    doc.getXmlFragment('default').push([image]);
    expect(cleanShared(doc, schema)).toBe(true);
    expect(image.getAttribute('onerror')).toBeUndefined();
    expect(image.getAttribute('src')).toBeUndefined();
    expect(image.getAttribute('attachment_id')).toBe('a1');
  });

  it('removes an address that leads to an unsafe scheme, also when hidden by spaces', () => {
    const doc = new Y.Doc();
    const image = new Y.XmlElement('image');
    image.setAttribute('attachment_id', 'a1');
    image.setAttribute('href', ' java\tscript:alert(1)');
    doc.getXmlFragment('default').push([image]);
    cleanShared(doc, schema);
    expect(image.getAttribute('href')).toBeUndefined();
    expect(image.getAttribute('attachment_id')).toBe('a1');
  });

  it('keeps a safe address on a known node', () => {
    const doc = new Y.Doc();
    const image = new Y.XmlElement('image');
    image.setAttribute('attachment_id', 'a1');
    image.setAttribute('href', 'https://example.org/');
    doc.getXmlFragment('default').push([image]);
    expect(cleanShared(doc, schema)).toBe(false);
    expect(image.getAttribute('href')).toBe('https://example.org/');
  });

  it('drops a link to an unsafe place and keeps its words', () => {
    const doc = new Y.Doc();
    const text = paragraphWith(doc, 'click me', { link: { href: 'javascript:alert(1)' } });
    expect(cleanShared(doc, schema)).toBe(true);
    expect(text.toDelta()).toEqual([{ insert: 'click me' }]);
  });

  it('drops a mark the schema does not know', () => {
    const doc = new Y.Doc();
    const text = paragraphWith(doc, 'x', { blink: {} });
    cleanShared(doc, schema);
    expect(text.toDelta()).toEqual([{ insert: 'x' }]);
  });

  it('keeps an extra attribute off a known mark', () => {
    const doc = new Y.Doc();
    const text = paragraphWith(doc, 'x', { link: { href: 'https://example.org/', onclick: 'alert(1)' } });
    cleanShared(doc, schema);
    const [op] = text.toDelta() as { attributes: { link: Record<string, unknown> } }[];
    expect(op?.attributes.link.onclick).toBeUndefined();
    expect(op?.attributes.link.href).toBe('https://example.org/');
  });

  it('changes nothing in a document that is already within the rules', () => {
    const doc = new Y.Doc();
    paragraphWith(doc, 'Hello', { bold: {}, link: { href: 'https://example.org/' } });
    expect(cleanShared(doc, schema)).toBe(false);
  });

  it('cleans what arrives from another client, and leaves its own typing alone', () => {
    const doc = new Y.Doc();
    const stop = keepSharedClean(doc, schema);
    const other = new Y.Doc();
    const script = new Y.XmlElement('script');
    other.getXmlFragment('default').push([script]);
    Y.applyUpdate(doc, Y.encodeStateAsUpdate(other));
    return Promise.resolve().then(() => {
      expect(doc.getXmlFragment('default').length).toBe(0);
      stop();
    });
  });
});

describe('safe addresses', () => {
  it('refuses schemes that run code or reach outside the web', () => {
    for (const bad of [
      'javascript:alert(1)',
      'JAVASCRIPT:x',
      'data:text/html,x',
      'vbscript:x',
      'java\nscript:x',
    ]) {
      expect(isSafeAddress(bad)).toBe(false);
    }
  });

  it('allows web and mail addresses, and addresses within the app', () => {
    for (const good of [
      'https://example.org/',
      'http://x',
      'mailto:a@b.c',
      '/pages/1',
      '#top',
      '?q=1',
      'page',
    ]) {
      expect(isSafeAddress(good)).toBe(true);
    }
  });
});
