// SPDX-License-Identifier: Apache-2.0
// Editing together, in two real browsers (spec 015, part 5). These need the service:
//   E2E_COLLAB=1 scripts/e2e collab.spec.ts
import type { Browser, Page } from '@playwright/test';
import { HocuspocusProvider, HocuspocusProviderWebsocket } from '@hocuspocus/provider';
import * as Y from 'yjs';
import {
  ADMIN,
  createUser,
  EDITOR,
  expect,
  grant,
  login,
  newSpace,
  PASSWORD,
  publishedPage,
  test,
  unique,
} from './helpers';

const SYNC = { timeout: 15_000 };

async function asPerson(browser: Browser, email: string): Promise<Page> {
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const page = await context.newPage();
  await login(page, email, PASSWORD);
  return page;
}

async function openEditor(page: Page, id: string) {
  await page.goto(`/pages/${id}/edit`);
  await expect(page.locator('.prose.editing')).toBeVisible(SYNC);
  await expect(page.getByText('Saved for everyone')).toBeVisible(SYNC);
}

async function typeAtEnd(page: Page, text: string) {
  await page.locator('.prose.editing').click();
  await page.keyboard.press('Control+End');
  await page.keyboard.type(text);
}

const editorOf = (page: Page) => page.locator('.prose.editing');

/** The shared document as plain JSON, to look for what the gate refuses. */
function sharedJSON(doc: Y.Doc): unknown {
  return doc.getXmlFragment('default').toJSON();
}

/**
 * A client outside the browser that holds a real session of `page` and speaks the protocol of the
 * service directly, so nothing in the editor stops what it sends (SEC-546).
 */
async function hostileClient(page: Page, pageId: string) {
  const cookies = await page.context().cookies();
  const cookie = cookies.map((c) => `${c.name}=${c.value}`).join('; ');
  const origin = 'http://localhost:8080';
  // Node's WebSocket takes extra headers, which a browser would add by itself.
  class SessionSocket extends WebSocket {
    constructor(url: string | URL, protocols?: string | string[]) {
      super(url, { protocols, headers: { cookie, origin } } as unknown as string[]);
    }
  }
  const doc = new Y.Doc();
  const socket = new HocuspocusProviderWebsocket({
    url: 'ws://localhost:8080/collab',
    WebSocketPolyfill: SessionSocket,
  });
  const provider = new HocuspocusProvider({
    websocketProvider: socket,
    name: pageId,
    document: doc,
    token: 'session',
  });
  // A socket made outside the provider is not joined by itself.
  provider.attach();
  await expect.poll(() => provider.isSynced, SYNC).toBe(true);
  return {
    doc,
    json: () => sharedJSON(doc),
    pose: (user: { id: string; name: string }) => provider.setAwarenessField('user', user),
    close: () => {
      provider.destroy();
      socket.destroy();
    },
  };
}

test.describe('editing together', () => {
  let target = '';
  let spaceId = '';
  let colleagueId = '';
  let colleagueEmail = '';

  test.beforeEach(async ({ browser, page }) => {
    await login(page, ADMIN.email, ADMIN.password);
    const space = await newSpace(page, 'Together');
    spaceId = space.id;
    colleagueEmail = `${unique('colleague')}@example.com`;
    colleagueId = await createUser(browser, colleagueEmail);
    await grant(page, space.id, { type: 'user', id: colleagueId }, EDITOR);
    target = await publishedPage(page, space.id, 'Shared', 'Start.');
    const info = await page.request.get(`/api/v1/pages/${target}/collab`);
    const { enabled } = (await info.json()) as { enabled: boolean };
    test.skip(!enabled, 'the service is not running');
  });

  test('two people type in one line, each sees the other, and nothing is lost', async ({ browser, page }) => {
    const colleague = await asPerson(browser, colleagueEmail);
    await openEditor(page, target);
    await openEditor(colleague, target);
    await expect(page.getByText('one shared draft')).toBeVisible();
    await typeAtEnd(page, ' Mine.');
    await expect(editorOf(colleague)).toContainText('Start. Mine.', SYNC);
    await typeAtEnd(colleague, ' Theirs.');
    await expect(editorOf(page)).toContainText('Start. Mine. Theirs.', SYNC);
    await expect(page.locator('.collab-caret-name')).toBeVisible(SYNC);
    await expect(page.getByRole('button', { name: /^Go to / })).toBeVisible(SYNC);
    await colleague.context().close();
  });

  test('undo takes back only your own work', async ({ browser, page }) => {
    const colleague = await asPerson(browser, colleagueEmail);
    await openEditor(page, target);
    await openEditor(colleague, target);
    await typeAtEnd(page, ' Mine.');
    await expect(editorOf(colleague)).toContainText('Mine.', SYNC);
    await typeAtEnd(colleague, ' Theirs.');
    await expect(editorOf(page)).toContainText('Theirs.', SYNC);
    await page.locator('.prose.editing').click();
    await page.keyboard.press('Control+z');
    await expect(editorOf(page)).not.toContainText('Mine.', SYNC);
    await expect(editorOf(page)).toContainText('Theirs.');
    await colleague.context().close();
  });

  test('typing offline merges on return', async ({ browser, page }) => {
    const colleague = await asPerson(browser, colleagueEmail);
    await openEditor(page, target);
    await openEditor(colleague, target);
    await colleague.context().setOffline(true);
    await typeAtEnd(colleague, ' Offline.');
    await typeAtEnd(page, ' Online.');
    await colleague.context().setOffline(false);
    await expect(editorOf(page)).toContainText('Offline.', { timeout: 30_000 });
    await expect(editorOf(colleague)).toContainText('Online.', { timeout: 30_000 });
    await colleague.context().close();
  });

  test('someone who loses the right to edit is removed at once', async ({ browser, page }) => {
    const colleague = await asPerson(browser, colleagueEmail);
    await openEditor(colleague, target);
    await grant(page, spaceId, { type: 'user', id: colleagueId }, ['space.view']);
    await expect(colleague.getByText('You can no longer edit this page.')).toBeVisible({ timeout: 20_000 });
    await colleague.context().close();
  });

  test('a published version names everyone who took part', async ({ browser, page }) => {
    const colleague = await asPerson(browser, colleagueEmail);
    await openEditor(page, target);
    await openEditor(colleague, target);
    await typeAtEnd(colleague, ' Theirs.');
    await expect(editorOf(page)).toContainText('Theirs.', SYNC);
    // The service stores after a moment, and the list of authors comes with it.
    await page.waitForTimeout(3000);
    await page.getByRole('button', { name: 'Publish', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Shared' })).toBeVisible(SYNC);
    await expect(page.getByText('Start. Theirs.')).toBeVisible();
    const res = await page.request.get(`/api/v1/pages/${target}/version-authors`);
    const { authors } = (await res.json()) as { authors: Record<string, string[]> };
    expect(authors['2']?.length).toBe(2);
    await colleague.context().close();
  });

  test('what a hostile client sends past the gate never reaches a colleague, the draft or the page', async ({
    browser,
    page,
  }) => {
    const hostilePattern = /"(script|iframe)"|onerror|javascript:/;
    const colleague = await asPerson(browser, colleagueEmail);
    let alerted = false;
    colleague.on('dialog', async (d) => {
      alerted = true;
      await d.dismiss();
    });
    await openEditor(colleague, target);

    // A client that speaks the protocol itself, with a valid session, and skips every rule of the editor.
    const hostile = await hostileClient(page, target);
    // It poses as someone else first. Its cursor and its changes travel the same socket, in order.
    hostile.pose({ id: 'someone-else', name: 'Posing Name' });
    const words = new Y.XmlText();
    const link = new Y.XmlElement('paragraph');
    link.insert(0, [words]);
    const script = new Y.XmlElement('script');
    script.insert(0, [new Y.XmlText('alert(1)')]);
    const frame = new Y.XmlElement('iframe');
    frame.setAttribute('src', 'https://example.org/');
    const image = new Y.XmlElement('image');
    image.setAttribute('src', 'x');
    image.setAttribute('onerror', 'alert(1)');
    hostile.doc.transact(() => {
      hostile.doc.getXmlFragment('default').insert(0, [script, frame, image, link]);
      words.insert(0, 'click me', { link: { href: 'javascript:alert(1)' } });
    });

    await expect(editorOf(colleague)).toContainText('click me', SYNC);
    await expect(editorOf(colleague).locator('script, iframe, [onerror]')).toHaveCount(0);
    await expect(editorOf(colleague).locator('a[href^="javascript"]')).toHaveCount(0);
    expect(alerted).toBe(false);

    // The name a colleague sees is the one the session proved (SEC-544).
    const present = () =>
      colleague
        .locator('.presence button')
        .evaluateAll((buttons) => buttons.map((b) => b.getAttribute('aria-label')));
    await expect.poll(present, SYNC).toHaveLength(1);
    expect(await present()).not.toContain('Go to Posing Name');

    // The colleague's editor removes what its schema cannot hold, and the service keeps only what passes the gate.
    await expect.poll(() => JSON.stringify(hostile.json()), { timeout: 30_000 }).not.toMatch(hostilePattern);
    hostile.close();

    // The cleaned shared document passes the server's gate, so the draft is stored again for everyone.
    await typeAtEnd(colleague, ' After.');
    await expect(colleague.getByText('Saved for everyone')).toBeVisible(SYNC);

    // The image the hostile client left has no attachment, so the gate refuses to publish it, and says why.
    await colleague.getByRole('button', { name: 'Publish', exact: true }).click();
    await expect(colleague.getByRole('alert')).toContainText('images must point to an attachment', SYNC);
    const published = await page.request.get(`/api/v1/pages/${target}`);
    expect(JSON.stringify(await published.json())).not.toMatch(hostilePattern);
    expect(alerted).toBe(false);
    await colleague.context().close();
  });

  test('offline text leaves at logout, and unsaved work stays for its writer alone', async ({
    browser,
    page,
  }) => {
    const sharedDatabases = (p: Page) =>
      p.evaluate(async () =>
        (await indexedDB.databases()).flatMap((d) => (d.name?.includes('.shared.') ? [d.name] : [])),
      );
    const logOut = async (p: Page) => {
      await p.goto('/');
      await p.getByRole('button', { name: 'Account' }).click();
      await p.getByRole('menuitem', { name: 'Log out' }).click();
      await expect(p).toHaveURL(/login/);
    };
    await openEditor(page, target);
    await typeAtEnd(page, ' Saved.');
    await expect(page.getByText('Saved for everyone')).toBeVisible(SYNC);
    await expect.poll(() => sharedDatabases(page)).toHaveLength(1);
    await logOut(page);
    await expect.poll(() => sharedDatabases(page)).toHaveLength(0);

    const colleague = await asPerson(browser, colleagueEmail);
    await openEditor(colleague, target);
    await colleague.context().setOffline(true);
    await typeAtEnd(colleague, ' Never saved.');
    await colleague.routeWebSocket(/collab/, (socket) => socket.close());
    await colleague.context().setOffline(false);
    await logOut(colleague);
    const kept = await sharedDatabases(colleague);
    expect(kept).toHaveLength(1);
    expect(kept[0]).toContain(colleagueId);
    await colleague.context().close();
  });
});
