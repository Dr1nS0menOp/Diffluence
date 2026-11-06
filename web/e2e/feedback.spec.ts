// SPDX-License-Identifier: Apache-2.0
// Spec 018: animals for spaces, changes in the page, and people in pickers.
import type { Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  expect,
  grant,
  login,
  newSpace,
  publishedPage,
  test,
  unique,
} from './helpers';

const words = (text: string) => ({ type: 'paragraph', content: [{ type: 'text', text }] });
const heading = (text: string) => ({
  type: 'heading',
  attrs: { level: 2 },
  content: [{ type: 'text', text }],
});
const list = (...items: string[]) => ({
  type: 'bulletList',
  content: items.map((text) => ({ type: 'listItem', content: [words(text)] })),
});

async function publish(page: Page, id: string, title: string, version: number, content: unknown[]) {
  const res = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title, document: { type: 'doc', content }, base_version: version },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('a space has an animal by itself, and can pick another or a character', async ({ page }) => {
  const space = await newSpace(page, 'Zoo space');
  await page.goto(`/spaces/${space.key}/settings`);
  const badge = page.locator('.sidebar-head .space-badge');
  await expect(badge).toHaveAttribute('src', /\/animals\/\w+\.png$/);
  const loaded = await badge.evaluate((image) => (image as HTMLImageElement).naturalWidth);
  expect(loaded).toBeGreaterThan(0);

  const choice = page.getByRole('group', { name: 'Icon' });
  await choice.getByRole('button', { name: 'Moose' }).click();
  await expect(choice.getByRole('button', { name: 'Moose' })).toHaveAttribute('aria-pressed', 'true');
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Save', exact: true }).first().click();
  await expect(badge).toHaveAttribute('src', '/animals/moose.png');

  await choice.getByLabel('Or a character of your own').fill('Z');
  await page.getByRole('button', { name: 'Save', exact: true }).first().click();
  await expect(page.locator('.sidebar-head .space-badge')).toHaveText('Z');

  await choice.getByRole('button', { name: 'Pick one from the key' }).click();
  await page.getByRole('button', { name: 'Save', exact: true }).first().click();
  await expect(page.locator('.sidebar-head img.space-badge')).toBeVisible();
});

test('a comparison looks like the page, with green, red and words beside them', async ({ page }) => {
  const space = await newSpace(page, 'Compare space');
  const id = await publishedPage(page, space.id, 'Report', 'x');
  await publish(page, id, 'Report', 1, [
    heading('A heading that stays'),
    words('The old sentence stays here'),
    list('first item', 'second item'),
    words('This line goes away'),
  ]);
  await publish(page, id, 'Report renamed', 2, [
    heading('A heading that stays'),
    words('The new sentence stays here'),
    list('first item', 'changed item'),
    words('A brand new line'),
  ]);

  await page.goto(`/pages/${id}/compare?from=2&to=3`);
  await expect(page.getByText('The title changed from "Report" to "Report renamed".')).toBeVisible();
  await expect(page.locator('.prose h2')).toHaveText('A heading that stays');

  const changed = page.locator('.diff-block.changed').first();
  await expect(changed.locator('ins')).toHaveText('new');
  await expect(changed.locator('del')).toHaveText('old');
  await expect(page.locator('.diff-label', { hasText: 'Changed' }).first()).toBeVisible();

  // A list cannot hold marks for single words, so it shows as it was and as it is.
  await expect(page.locator('ul.diff-block.removed')).toContainText('second item');
  await expect(page.locator('ul.diff-block.added')).toContainText('changed item');
  await expect(page.locator('.diff-label', { hasText: 'Removed' }).first()).toBeVisible();
  await expect(page.locator('.diff-label', { hasText: 'Added' }).first()).toBeVisible();

  const green = await changed.locator('ins').evaluate((e) => getComputedStyle(e).backgroundColor);
  const red = await changed.locator('del').evaluate((e) => getComputedStyle(e).backgroundColor);
  expect(green).toBe('rgb(217, 242, 227)');
  expect(red).toBe('rgb(251, 220, 217)');
  await checkAccessibility(page);
});

test('opening a version shows what changed in it, and can show the clean page', async ({ page }) => {
  const space = await newSpace(page, 'Version space');
  const id = await publishedPage(page, space.id, 'Versions', 'the first words');
  await publish(page, id, 'Versions', 1, [words('the second words')]);

  await page.goto(`/pages/${id}/history`);
  await page.getByRole('button', { name: 'View' }).first().click();
  const dialog = page.getByRole('dialog', { name: 'Version 2' });
  await expect(dialog.locator('ins')).toHaveText('second');
  await expect(dialog.locator('del')).toHaveText('first');
  await checkAccessibility(page);

  await dialog.getByRole('button', { name: 'Show the clean version' }).click();
  await expect(dialog.locator('ins')).toHaveCount(0);
  await expect(dialog.locator('.prose')).toHaveText('the second words');
  await dialog.getByRole('button', { name: 'Show the changes' }).click();
  await expect(dialog.locator('ins')).toHaveText('second');
  await dialog.getByRole('button', { name: 'Close' }).click();

  await page.getByRole('button', { name: 'View' }).last().click();
  const first = page.getByRole('dialog', { name: 'Version 1' });
  await expect(first.locator('.prose')).toHaveText('the first words');
  await expect(first.getByRole('button', { name: 'Show the changes' })).toHaveCount(0);
});

test('a version after a removed one shows clean, with no error', async ({ page }) => {
  const space = await newSpace(page, 'Removed space');
  const id = await publishedPage(page, space.id, 'Gaps', 'the first words');
  await publish(page, id, 'Gaps', 1, [words('the leaked words')]);
  await publish(page, id, 'Gaps', 2, [words('the third words')]);
  const res = await page.request.delete(`/api/v1/admin/pages/${id}/versions/2`, {
    data: { reason: 'A leaked password' },
  });
  expect(res.ok(), await res.text()).toBeTruthy();

  await page.goto(`/pages/${id}/history`);
  await page.getByRole('button', { name: 'View' }).first().click();
  const dialog = page.getByRole('dialog', { name: 'Version 3' });
  await expect(dialog.locator('.prose')).toHaveText('the third words');
  await expect(dialog.getByRole('alert')).toHaveCount(0);
  await expect(dialog.getByRole('button', { name: 'Show the clean version' })).toHaveCount(0);
});

test('a picker shows people as soon as it has focus, and works by keyboard', async ({ page }) => {
  const space = await newSpace(page, 'Picker space');
  const id = await publishedPage(page, space.id, 'Picked', 'x');
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'Restrictions' }).click();
  const dialog = page.getByRole('dialog', { name: 'Page restrictions' });
  await dialog.getByRole('radio', { name: /Private/ }).check();
  const field = dialog.getByRole('combobox', { name: 'Add' }).first();

  await field.click();
  await expect(field).toHaveAttribute('aria-expanded', 'true');
  const admin = dialog.getByRole('option', { name: /admin@example.com/ });
  await expect(admin).toBeVisible();
  await checkAccessibility(page);

  // Escape closes the list, and the dialog stays.
  await page.keyboard.press('Escape');
  await expect(field).toHaveAttribute('aria-expanded', 'false');
  await expect(dialog).toBeVisible();

  await page.keyboard.press('ArrowDown');
  await expect(field).toHaveAttribute('aria-expanded', 'true');
  await field.fill('admin@');
  await expect(dialog.getByRole('listbox').getByRole('option')).toHaveCount(1);
  await page.keyboard.press('ArrowDown');
  await expect(field).toHaveAttribute('aria-activedescendant', /.+/);
  await expect(admin).toHaveAttribute('aria-selected', 'true');
  await page.keyboard.press('Enter');
  await expect(dialog.getByRole('listitem').filter({ hasText: 'Admin' }).first()).toBeVisible();
  await expect(field).toHaveValue('');
});

test('a person without view profiles sees no people in a picker', async ({ page, browser }) => {
  const space = await newSpace(page, 'Guest picker');
  const id = await publishedPage(page, space.id, 'Guarded', 'x');
  const email = `${unique('guest')}@example.com`;
  const guest = await createUser(browser, email, 'guest');
  await grant(page, space.id, { type: 'user', id: guest }, ['space.view', 'restriction.manage']);

  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const visitor = await context.newPage();
  await login(visitor, email);
  await visitor.goto(`/pages/${id}`);
  await visitor.getByRole('button', { name: 'Restrictions' }).click();
  const dialog = visitor.getByRole('dialog', { name: 'Page restrictions' });
  await dialog.getByRole('radio', { name: /Private/ }).check();
  const field = dialog.getByRole('combobox', { name: 'Add' }).first();
  await field.click();
  await expect(field).toHaveAttribute('aria-expanded', 'false');
  await expect(dialog.getByRole('option')).toHaveCount(0);
  await context.close();
});
