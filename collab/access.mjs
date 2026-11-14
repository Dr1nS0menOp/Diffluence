// SPDX-License-Identifier: Apache-2.0
// Who is connected, and whether they still may be (SEC-542, spec 067). The service holds
// no session: the API checks the browser at the socket, and hands the service a ticket
// for one person and one page. The service asks about that ticket, and gets a new one
// with every yes.

import { createHash, timingSafeEqual } from 'node:crypto';

export const TICKET_HEADER = 'x-diffluence-collab-ticket';

/**
 * Whether an Authorization header carries the token the API shares with the service. Both
 * sides are hashed first, so the comparison takes the same time whatever the header holds,
 * also its length. An empty token lets nobody in.
 */
export function isTheAPI(authorization, token) {
  if (!token || typeof authorization !== 'string') return false;
  const digest = (value) => createHash('sha256').update(value).digest();
  return timingSafeEqual(digest(authorization), digest(`Bearer ${token}`));
}
export const CLOSE_ACCESS_ENDED = { code: 4403, reason: 'access-ended' };

/** The ticket of a socket. Nothing else from the browser's headers is read. */
export function ticketFrom(requestHeaders) {
  return requestHeaders.get(TICKET_HEADER) ?? '';
}

/**
 * Builds the checks around `ask`, which sends a question to the API and gives back its
 * response.
 */
export function createAccess(ask) {
  const connections = new Map();

  /** Anything but a clear yes with a new ticket is a no. */
  async function authorize(pageId, ticket) {
    if (!ticket) return null;
    const res = await ask({ page_id: pageId, ticket });
    if (!res.ok) return null;
    const answer = await res.json();
    return answer.allowed && answer.ticket ? answer : null;
  }

  function remember(documentName, entry) {
    if (!connections.has(documentName)) connections.set(documentName, new Set());
    connections.get(documentName).add(entry);
  }

  function forget(documentName, entry) {
    connections.get(documentName)?.delete(entry);
    if (connections.get(documentName)?.size === 0) connections.delete(documentName);
  }

  // One check runs at a time. Calls that arrive while it runs share one more run after it,
  // because a change of rights may have come after the running check passed that person;
  // a storm of changes, such as a sync of a directory, then costs two runs, not one each.
  let running = null;
  let queued = null;

  /**
   * Asks again for everyone, once the check in flight is done. A failed answer counts as
   * no: a right must be proven.
   */
  function recheck() {
    if (!running) {
      running = checkAll().finally(() => {
        running = null;
      });
      return running;
    }
    if (!queued) {
      // The failure of the run in flight reaches its own callers; it must not stop this one.
      queued = running
        .catch(() => undefined)
        .then(() => {
          queued = null;
          return recheck();
        });
    }
    return queued;
  }

  async function checkAll() {
    for (const [documentName, entries] of connections) {
      for (const entry of [...entries]) {
        const answer = await authorize(documentName, entry.ticket).catch(() => null);
        if (answer) {
          entry.ticket = answer.ticket;
          continue;
        }
        entry.close(CLOSE_ACCESS_ENDED);
        forget(documentName, entry);
      }
    }
  }

  return { authorize, remember, forget, recheck };
}
