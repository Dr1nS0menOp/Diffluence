// SPDX-License-Identifier: Apache-2.0
// Turns a shared document into the JSON the API's gate reads. The service knows no
// schema on purpose: what is allowed is decided in one place, document.Normalize.
import * as Y from 'yjs';

/** One message. A page holds 5 MiB at most, and its shared state can be about twice that. */
export const MAX_MESSAGE_BYTES = 16 << 20;

/** The API takes 32 MiB in one body. The state grows by a third as base64, and the page comes along. */
const MAX_STATE_BYTES = 16 << 20;

/** Whether a shared state is small enough for the API to keep. */
export function fitsTheStore(state) {
  return state.length <= MAX_STATE_BYTES;
}

function textNodes(text) {
  return text.toDelta().map(({ insert, attributes }) => {
    const node = { type: 'text', text: insert };
    const marks = Object.entries(attributes ?? {}).map(([type, attrs]) =>
      attrs && typeof attrs === 'object' && Object.keys(attrs).length > 0 ? { type, attrs } : { type },
    );
    return marks.length > 0 ? { ...node, marks } : node;
  });
}

function nodesOf(item) {
  if (item instanceof Y.XmlText) return textNodes(item);
  const node = { type: item.nodeName };
  const attrs = item.getAttributes();
  if (Object.keys(attrs).length > 0) node.attrs = attrs;
  const content = item.toArray().flatMap(nodesOf);
  if (content.length > 0) node.content = content;
  return [node];
}

/** The shared document as the JSON of a page. TipTap keeps it in the fragment "default". */
export function documentJSON(document) {
  return { type: 'doc', content: document.getXmlFragment('default').toArray().flatMap(nodesOf) };
}
