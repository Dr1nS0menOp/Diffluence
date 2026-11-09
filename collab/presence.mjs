// SPDX-License-Identifier: Apache-2.0
// Who a cursor belongs to comes from the session, never from the browser (SEC-544). Browsers
// share their cursors as awareness states, and a browser fills in its own name there. The
// service rewrites every state a connection sends, so a name in a cursor is always proven.

/**
 * Sets the person in every awareness state of one message to the person the session proved,
 * with only an id and a name. Without a proven person the states are dropped.
 */
export function bindPresence(states, user) {
  for (const [clientID, state] of states) {
    if (!user) {
      states.delete(clientID);
      continue;
    }
    states.set(clientID, { ...state, user: { id: user.id, name: user.name } });
  }
}
