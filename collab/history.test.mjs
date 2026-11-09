// SPDX-License-Identifier: Apache-2.0
// What was typed and removed must really go from the shared state (SEC-548): a secret pasted by
// mistake would otherwise reach every browser that opens the page, and the stored draft.
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { Document, defaultConfiguration } from '@hocuspocus/server';
import * as Y from 'yjs';

const SECRET = 'pasted-secret-4711';

/** A browser types into a paragraph and removes part of it; the service takes each change. */
function typeThenRemove(service) {
  const browser = new Y.Doc();
  const send = (update) => Y.applyUpdate(service, update);
  browser.on('update', send);
  const paragraph = new Y.XmlElement('paragraph');
  const words = new Y.XmlText();
  paragraph.insert(0, [words]);
  browser.getXmlFragment('default').insert(0, [paragraph]);
  words.insert(0, `before ${SECRET} after`);
  words.delete('before '.length, SECRET.length + 1);
  browser.off('update', send);
  browser.destroy();
}

test('removed text is gone from the state the service keeps and sends', (t) => {
  // The service makes its documents with these options: server.mjs sets none of its own.
  const service = new Document('page', defaultConfiguration.yDocOptions);
  t.after(() => service.destroy());
  typeThenRemove(service);
  const state = Buffer.from(Y.encodeStateAsUpdate(service));
  assert.equal(state.includes(SECRET), false);
  assert.equal(state.includes('after'), true);
});

test('the service collects what was removed', () => {
  assert.equal(defaultConfiguration.yDocOptions.gc, true);
});

test('without collection the removed text would stay, so the first test can fail', (t) => {
  const service = new Y.Doc({ gc: false });
  t.after(() => service.destroy());
  typeThenRemove(service);
  assert.equal(Buffer.from(Y.encodeStateAsUpdate(service)).includes(SECRET), true);
});
