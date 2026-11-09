// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { bindPresence } from './presence.mjs';

const anna = { id: 'anna-id', name: 'Anna' };

test('a browser cannot pose as a colleague in its cursor', () => {
  const states = new Map([[7, { user: { id: 'boss-id', name: 'The boss' }, cursor: { anchor: 1, head: 2 } }]]);
  bindPresence(states, anna);
  assert.deepEqual(states.get(7), { user: anna, cursor: { anchor: 1, head: 2 } });
});

test('a state without a person gets the one the session proved', () => {
  const states = new Map([[7, { cursor: null }]]);
  bindPresence(states, anna);
  assert.deepEqual(states.get(7).user, anna);
});

test('every state of a connection is bound, not only the first', () => {
  const states = new Map([
    [7, { user: anna }],
    [8, { user: { id: 'boss-id', name: 'The boss' } }],
  ]);
  bindPresence(states, anna);
  assert.deepEqual(states.get(8).user, anna);
});

test('a person carries only an id and a name, nothing the browser added', () => {
  const states = new Map([[7, { user: { id: 'anna-id', name: 'Anna', colour: 'red', admin: true } }]]);
  bindPresence(states, anna);
  assert.deepEqual(states.get(7).user, anna);
});

test('a connection without a proven person sends no state', () => {
  const states = new Map([[7, { user: anna }]]);
  bindPresence(states, undefined);
  assert.equal(states.size, 0);
});
