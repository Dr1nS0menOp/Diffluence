// SPDX-License-Identifier: Apache-2.0
// Spec 041: an open tab notices a newer version, and reloading loses nothing.
import type { Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  editorSettled,
  expect,
  login,
  newSpace,
  publishedPage,
  test,
} from './helpers';

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

/** Answers the appearance with a newer build once `newer()` says so. */
async function serveBuilds(page: Page, newer: () => boolean) {
  await page.route('**/api/v1/appearance', async (route) => {
    const res = await route.fetch();
    const look = (await res.json()) as { build: string };
    expect(look.build, 'the server names the frontend it ships').not.toBe('');
    if (newer()) look.build = `${look.build}-newer`;
    await route.fulfill({ response: res, json: look });
  });
}

/** What a browser does when a person comes back to the tab. */
async function comeBack(page: Page) {
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
}

test('nothing is said while the version stays the same', async ({ page }) => {
  await serveBuilds(page, () => false);
  await page.goto('/');
  await comeBack(page);
  await expect(page.locator('main')).toBeVisible();
  await expect(page.getByRole('status', { name: 'New version' })).toHaveCount(0);
});

test('a newer version is offered without stopping the writer, and a reload keeps the text', async ({
  page,
}) => {
  const space = await newSpace(page, 'Versions');
  const id = await publishedPage(page, space.id, 'Ongoing', 'start');
  let newer = false;
  await serveBuilds(page, () => newer);
  await page.goto(`/pages/${id}/edit`);
  const body = page.locator('.prose.editing');
  await body.click();
  await editorSettled(page);
  await page.keyboard.press('Control+End');
  await page.keyboard.type(' first');

  newer = true;
  await comeBack(page);
  const note = page.getByRole('status', { name: 'New version' });
  await expect(note).toBeVisible();
  // The notice takes no focus, so the next keys still land in the text.
  await expect(body).toBeFocused();
  await page.keyboard.type(' second');
  await expect(body).toContainText('first second');
  await checkAccessibility(page);

  await note.getByRole('button', { name: 'Reload' }).click();
  await expect(page.locator('.prose.editing')).toContainText('first second');
  // After the reload the tab runs the newer version, so there is nothing more to say.
  await expect(page.getByRole('status', { name: 'New version' })).toHaveCount(0);
});
