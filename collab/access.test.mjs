// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { CLOSE_ACCESS_ENDED, createAccess, isTheAPI, ticketFrom } from './access.mjs';

const PAGE = '0196f0a0-0000-7000-8000-000000000001';

/** An API that answers from a list, and keeps every question it was asked. */
function fakeAPI(...answers) {
  const asked = [];
  const ask = async (body) => {
    asked.push(body);
    const next = answers.shift();
    if (next instanceof Error) throw next;
    return { ok: next.status === undefined || next.status === 200, json: async () => next.body ?? next };
  };
  return { ask, asked };
}

function connection(ticket) {
  const closed = [];
  return { entry: { ticket, close: (event) => closed.push(event) }, closed };
}

test('only the token of the API is the API, compared in constant time', () => {
  const token = 'a-long-shared-token-of-the-api';
  assert.equal(isTheAPI(`Bearer ${token}`, token), true);
  assert.equal(isTheAPI(`Bearer ${token}x`, token), false);
  assert.equal(isTheAPI(`Bearer ${token.slice(0, -1)}`, token), false);
  assert.equal(isTheAPI(token, token), false);
  assert.equal(isTheAPI(undefined, token), false);
  assert.equal(isTheAPI('Bearer ', ''), false, 'an unset token lets nobody in');
});

test('the service reads the ticket and nothing else from the socket', () => {
  const headers = new Headers({ cookie: 'diffluence_session=secret', 'x-diffluence-collab-ticket': 'abc.def' });
  assert.equal(ticketFrom(headers), 'abc.def');
  assert.equal(ticketFrom(new Headers({ cookie: 'diffluence_session=secret' })), '');
});

test('the API is asked with the page and the ticket only', async () => {
  const api = fakeAPI({ allowed: true, user_id: 'u', name: 'Anna', ticket: 't2' });
  const answer = await createAccess(api.ask).authorize(PAGE, 't1');
  assert.deepEqual(api.asked, [{ page_id: PAGE, ticket: 't1' }]);
  assert.equal(answer.ticket, 't2');
});

test('a socket without a ticket is refused without asking', async () => {
  const api = fakeAPI();
  assert.equal(await createAccess(api.ask).authorize(PAGE, ''), null);
  assert.equal(api.asked.length, 0);
});

test('anything but a clear yes with a new ticket is a no', async () => {
  for (const answer of [
    { allowed: false },
    { allowed: true, user_id: 'u' },
    { status: 500, body: { allowed: true, ticket: 't' } },
  ]) {
    assert.equal(await createAccess(fakeAPI(answer).ask).authorize(PAGE, 't1'), null, JSON.stringify(answer));
  }
});

test('a recheck carries the ticket the API renewed', async () => {
  const api = fakeAPI({ allowed: true, ticket: 't2' }, { allowed: true, ticket: 't3' });
  const access = createAccess(api.ask);
  const { entry, closed } = connection('t1');
  access.remember(PAGE, entry);
  await access.recheck();
  await access.recheck();
  assert.deepEqual(api.asked.map((body) => body.ticket), ['t1', 't2']);
  assert.equal(entry.ticket, 't3');
  assert.deepEqual(closed, []);
});

test('a refused recheck closes the connection and forgets it', async () => {
  const api = fakeAPI({ allowed: false });
  const access = createAccess(api.ask);
  const { entry, closed } = connection('t1');
  access.remember(PAGE, entry);
  await access.recheck();
  assert.deepEqual(closed, [CLOSE_ACCESS_ENDED]);
  await access.recheck();
  assert.equal(api.asked.length, 1, 'a forgotten connection was asked about again');
});

test('an API that cannot answer ends access at the recheck', async () => {
  const api = fakeAPI(new Error('connection refused'));
  const access = createAccess(api.ask);
  const { entry, closed } = connection('t1');
  access.remember(PAGE, entry);
  await access.recheck();
  assert.deepEqual(closed, [CLOSE_ACCESS_ENDED]);
});

test('a connection that left is not asked about', async () => {
  const api = fakeAPI({ allowed: true, ticket: 't2' });
  const access = createAccess(api.ask);
  const stays = connection('a');
  const leaves = connection('b');
  access.remember(PAGE, stays.entry);
  access.remember(PAGE, leaves.entry);
  access.forget(PAGE, leaves.entry);
  await access.recheck();
  assert.deepEqual(api.asked.map((body) => body.ticket), ['a']);
});
