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
  paragraph,
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

/** Says a newer version runs, and takes the reload it offers. */
async function reloadForNewVersion(page: Page, makeNewer: () => void) {
  makeNewer();
  await comeBack(page);
  await page.getByRole('status', { name: 'New version' }).getByRole('button', { name: 'Reload' }).click();
  await expect(page.getByRole('status', { name: 'New version' })).toHaveCount(0);
}

async function comment(page: Page, pageId: string, text: string) {
  const res = await page.request.post(`/api/v1/pages/${pageId}/comments`, {
    data: { body: paragraph(text) },
  });
  expect(res.status(), await res.text()).toBe(201);
}

// Spec 043: comment boxes and settings forms join the save before the reload.
test('a comment box left empty is not opened again by the reload', async ({ page }) => {
  const space = await newSpace(page, 'Empty boxes');
  const id = await publishedPage(page, space.id, 'Quiet', 'x');
  await comment(page, id, 'nobody answers this');
  let newer = false;
  await serveBuilds(page, () => newer);
  await page.goto(`/pages/${id}`);
  await page
    .getByRole('article', { name: /^Comment by / })
    .getByRole('button', { name: 'Reply' })
    .click();
  await expect(page.getByRole('textbox', { name: 'Write a reply' })).toBeVisible();
  await reloadForNewVersion(page, () => (newer = true));
  await expect(page.getByRole('article', { name: /^Comment by / })).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Write a reply' })).toHaveCount(0);
});

test('a reply and a changed comment being written come back, open and unsent, after the reload', async ({
  page,
}) => {
  const space = await newSpace(page, 'Open boxes');
  const id = await publishedPage(page, space.id, 'Discussed', 'x');
  await comment(page, id, 'answer me');
  await comment(page, id, 'original words');
  let newer = false;
  await serveBuilds(page, () => newer);
  await page.goto(`/pages/${id}`);
  const asking = page.getByRole('article', { name: /^Comment by / }).filter({ hasText: 'answer me' });
  await asking.getByRole('button', { name: 'Reply' }).click();
  await page.getByRole('textbox', { name: 'Write a reply' }).pressSequentially('kept reply');
  const changing = page.getByRole('article', { name: /^Comment by / }).filter({ hasText: 'original words' });
  await changing.getByRole('button', { name: 'Edit' }).click();
  const editing = page.getByRole('textbox', { name: 'Edit your comment' });
  await editing.press('End');
  await editing.pressSequentially(' and more');

  await reloadForNewVersion(page, () => (newer = true));
  await expect(page.getByRole('textbox', { name: 'Write a reply' })).toHaveText('kept reply');
  await expect(page.getByRole('textbox', { name: 'Edit your comment' })).toHaveText(
    'original words and more',
  );
  // Kept in the browser, not sent: the server has neither.
  const sent = await page.request.get(`/api/v1/pages/${id}/comments`);
  const text = await sent.text();
  expect(text).not.toContain('kept reply');
  expect(text).not.toContain('and more');
});

test('text typed into a settings form comes back after the reload, unsaved', async ({ page }) => {
  const space = await newSpace(page, 'Settings kept');
  let newer = false;
  await serveBuilds(page, () => newer);
  await page.goto(`/spaces/${space.key}/settings?section=details`);
  await page.getByLabel('Description').fill('typed before the reload');
  await page.getByLabel('Name', { exact: true }).fill('Settings renamed');

  await reloadForNewVersion(page, () => (newer = true));
  await expect(page.getByLabel('Description')).toHaveValue('typed before the reload');
  await expect(page.getByLabel('Name', { exact: true })).toHaveValue('Settings renamed');
  const stored = (await (await page.request.get(`/api/v1/spaces/${space.id}`)).json()) as { name: string };
  expect(stored.name).toBe('Settings kept');
});
