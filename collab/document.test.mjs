// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { test } from 'node:test';
import * as Y from 'yjs';
import { documentJSON } from './document.mjs';

function paragraph(text, attributes) {
  const element = new Y.XmlElement('paragraph');
  const words = new Y.XmlText();
  element.insert(0, [words]);
  return { element, fill: () => words.insert(0, text, attributes) };
}

test('a shared document becomes the JSON of a page', () => {
  const doc = new Y.Doc();
  const { element, fill } = paragraph('Hello', { bold: {} });
  doc.getXmlFragment('default').insert(0, [element]);
  fill();
  assert.deepEqual(documentJSON(doc), {
    type: 'doc',
    content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Hello', marks: [{ type: 'bold' }] }] }],
  });
});

test('a mark keeps its attributes, so the gate can read them', () => {
  const doc = new Y.Doc();
  const { element, fill } = paragraph('x', { link: { href: 'javascript:alert(1)' } });
  doc.getXmlFragment('default').insert(0, [element]);
  fill();
  const [mark] = documentJSON(doc).content[0].content[0].marks;
  assert.equal(mark.attrs.href, 'javascript:alert(1)');
});

test('an unknown node is passed on for the gate to refuse', () => {
  const doc = new Y.Doc();
  doc.getXmlFragment('default').insert(0, [new Y.XmlElement('script')]);
  assert.equal(documentJSON(doc).content[0].type, 'script');
});

test('an empty document has no content', () => {
  assert.deepEqual(documentJSON(new Y.Doc()), { type: 'doc', content: [] });
});
