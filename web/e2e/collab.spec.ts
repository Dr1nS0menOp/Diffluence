// SPDX-License-Identifier: Apache-2.0
// Editing together, in two real browsers (spec 015, part 5). These need the service:
//   E2E_COLLAB=1 scripts/e2e collab.spec.ts
import type { Browser, Page } from '@playwright/test';
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
