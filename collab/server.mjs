// SPDX-License-Identifier: Apache-2.0
// The service for editing together (blueprint 04, ARC-118). It moves changes between
// browsers and keeps the shared document. It has no business logic: every decision
// about who may do what is an answer from the API, and it never writes a version.
import { Server } from '@hocuspocus/server';
import * as Y from 'yjs';
import { documentJSON } from './document.mjs';

const API_URL = (process.env.DIFFLUENCE_API_URL ?? 'http://localhost:8080').replace(/\/$/, '');
const TOKEN = process.env.DIFFLUENCE_COLLAB_TOKEN ?? '';
const PORT = Number(process.env.DIFFLUENCE_COLLAB_PORT ?? 1234);
const RECHECK_MS = 60_000;
const SAVE_DEBOUNCE_MS = 2_000;
const SAVE_MAX_DEBOUNCE_MS = 10_000;
const CLOSE_ACCESS_ENDED = { code: 4403, reason: 'access-ended' };
const CLOSE_REFUSED_STATE = { code: 4422, reason: 'document-refused' };

if (!TOKEN) {
  console.error('DIFFLUENCE_COLLAB_TOKEN is not set');
  process.exit(1);
}

function callAPI(path, init = {}) {
  return fetch(`${API_URL}/collab${path}`, {
    ...init,
    headers: { ...init.headers, Authorization: `Bearer ${TOKEN}`, 'Content-Type': 'application/json' },
  });
}

/** Asks the API whether a session may edit a page. Anything but a clear yes is a no. */
async function authorize(pageId, cookie) {
  const res = await callAPI('/authorize', { method: 'POST', body: JSON.stringify({ page_id: pageId, cookie }) });
  if (!res.ok) return null;
  const answer = await res.json();
  return answer.allowed ? answer : null;
}

// Who is connected, by document, so rights can be asked for again (SEC-542).
const connections = new Map();

function remember(documentName, entry) {
  if (!connections.has(documentName)) connections.set(documentName, new Set());
  connections.get(documentName).add(entry);
}

function forget(documentName, entry) {
  connections.get(documentName)?.delete(entry);
  if (connections.get(documentName)?.size === 0) connections.delete(documentName);
}

/** Asks again for everyone. A failed answer counts as no: a right must be proven. */
async function recheck() {
  for (const [documentName, entries] of connections) {
    for (const entry of [...entries]) {
      const answer = await authorize(documentName, entry.cookie).catch(() => null);
      if (answer) continue;
      entry.close(CLOSE_ACCESS_ENDED);
      forget(documentName, entry);
    }
  }
}

const authors = new Map();

const server = new Server({
  port: PORT,
  quiet: true,
  debounce: SAVE_DEBOUNCE_MS,
  maxDebounce: SAVE_MAX_DEBOUNCE_MS,

  async onAuthenticate({ documentName, requestHeaders, connectionConfig }) {
    const cookie = requestHeaders.get('cookie') ?? '';
    const answer = await authorize(documentName, cookie);
    if (!answer) throw new Error('not allowed');
    connectionConfig.readOnly = false;
    return { user: { id: answer.user_id, name: answer.name }, cookie };
  },

  async connected({ documentName, context, connection }) {
    const entry = {
      cookie: context.cookie,
      close: (event) => {
        // A closing document says nothing to the browser, so it hears it in words first.
        connection.sendStateless(event.reason);
        connection.close(event);
      },
    };
    remember(documentName, entry);
    context.entry = entry;
  },

  async onDisconnect({ documentName, context }) {
    if (context.entry) forget(documentName, context.entry);
  },

  async onLoadDocument({ documentName, document, context }) {
    const res = await callAPI(`/documents/${documentName}`);
    if (res.status === 204) {
      // One browser fills a new shared document with the published page.
      document.getMap('meta').set('seeder', context.user.id);
      return document;
    }
    if (!res.ok) throw new Error('the document could not be loaded');
    Y.applyUpdate(document, new Uint8Array(await res.arrayBuffer()));
    return document;
  },

  async onChange({ documentName, context }) {
    if (!context.user) return;
    if (!authors.has(documentName)) authors.set(documentName, new Set());
    authors.get(documentName).add(context.user.id);
  },

  /** What the API tells the service, on the same port. Throwing ends the request here. */
  async onRequest({ request, response }) {
    if (request.url === '/livez') {
      response.writeHead(200).end('ok');
      throw null;
    }
    if (request.url === '/recheck' && request.method === 'POST' && request.headers.authorization === `Bearer ${TOKEN}`) {
      recheck().catch((error) => console.error('recheck failed', error.message));
      response.writeHead(202).end();
      throw null;
    }
  },

  /** Every stored state passes the gate of the API, and a refused one is not kept (SEC-546). */
  async onStoreDocument({ documentName, document }) {
    const res = await callAPI(`/documents/${documentName}`, {
      method: 'PUT',
      body: JSON.stringify({
        state: Buffer.from(Y.encodeStateAsUpdate(document)).toString('base64'),
        title: document.getText('title').toString(),
        document: documentJSON(document),
        authors: [...(authors.get(documentName) ?? [])],
      }),
    });
    if (res.status === 422) {
      // Everyone goes back to the last good state. The browsers load it on reconnecting.
      document.getConnections().forEach((c) => c.close(CLOSE_REFUSED_STATE));
      return;
    }
    if (!res.ok) throw new Error('the document could not be stored');
    authors.delete(documentName);
  },
});

setInterval(() => {
  recheck().catch((error) => console.error('recheck failed', error.message));
}, RECHECK_MS);

await server.listen();
console.log(`collab listening on ${PORT}`);
