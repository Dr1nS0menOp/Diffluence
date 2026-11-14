// SPDX-License-Identifier: Apache-2.0
// Spec 045: blog posts and databases have a create permission of their own. The Create
// button offers only what a person may make, and makes that at once.
import type { Browser, Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  expect,
  grant,
  login,
  newSpace,
  test,
  unique,
} from './helpers';

async function personWith(browser: Browser, admin: Page, spaceId: string, perms: string[]): Promise<Page> {
  const email = `${unique('maker')}@example.com`;
  const id = await createUser(browser, email);
  await grant(admin, spaceId, { type: 'user', id }, perms);
  const page = await (await browser.newContext({ baseURL: 'http://localhost:8080' })).newPage();
  await login(page, email);
  return page;
}

test('someone who may only write blog posts is offered a post and nothing else', async ({
  page,
  browser,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Posts only');
  const blogger = await personWith(browser, page, space.id, ['space.view', 'post.create']);
  await blogger.goto(`/spaces/${space.key}`);

  const more = blogger.getByRole('button', { name: 'More to create' });
  await more.click();
  const menu = blogger.getByRole('menu');
  await expect(menu.getByRole('menuitem', { name: 'Blog post' })).toBeVisible();
  for (const name of ['New page', 'New database', 'From a template']) {
    await expect(menu.getByRole('menuitem', { name })).toHaveCount(0);
  }
  await blogger.keyboard.press('Escape');

  // The button itself makes what the person may make: a post.
  await blogger.getByRole('button', { name: 'Create', exact: true }).click();
  const dialog = blogger.getByRole('dialog', { name: 'Blog post' });
  await expect(dialog).toBeVisible();
  await checkAccessibility(blogger);
});

test('someone who makes pages but not databases is not offered a database', async ({ page, browser }) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Pages only');
  // The list names post.create, so the older grants route takes it as written.
  const writer = await personWith(browser, page, space.id, [
    'space.view',
    'page.create',
    'page.edit',
    'post.create',
  ]);
  await writer.goto(`/spaces/${space.key}`);
  await writer.getByRole('button', { name: 'More to create' }).click();
  const menu = writer.getByRole('menu');
  await expect(menu.getByRole('menuitem', { name: 'New page' })).toBeVisible();
  await expect(menu.getByRole('menuitem', { name: 'Blog post' })).toBeVisible();
  await expect(menu.getByRole('menuitem', { name: 'New database' })).toHaveCount(0);
});

test('databases off in the console are offered nowhere, and on again they are back (spec 069)', async ({
  page,
}) => {
  await login(page, ADMIN.email, ADMIN.password);
  const space = await newSpace(page, 'Experimental');
  await page.goto('/admin/organising');
  await expect(page.getByRole('heading', { name: 'Experimental parts' })).toBeVisible();
  const databases = page.getByRole('checkbox', { name: 'Databases on' });
  await expect(databases).toBeChecked();
  // The box follows the server, so it changes once the answer is back.
  await databases.click();
  await expect(databases).not.toBeChecked();
  // The click scrolls the page, and a row can then sit under the sticky top bar, where axe
  // measures its box against the search field. Measure the screen as it rests.
  await page.evaluate(() => window.scrollTo(0, 0));
  await checkAccessibility(page);
  try {
    await page.goto(`/spaces/${space.key}`);
    await page.getByRole('button', { name: 'More to create' }).click();
    const menu = page.getByRole('menu');
    await expect(menu.getByRole('menuitem', { name: 'New page' })).toBeVisible();
    await expect(menu.getByRole('menuitem', { name: 'New database' })).toHaveCount(0);
    await page.keyboard.press('Escape');
  } finally {
    // The other suites test databases, so scripts/e2e turns every part on.
    await page.goto('/admin/organising');
    await page.getByRole('checkbox', { name: 'Databases on' }).click();
  }
  await expect(page.getByRole('checkbox', { name: 'Databases on' })).toBeChecked();
  await page.goto(`/spaces/${space.key}`);
  await page.getByRole('button', { name: 'More to create' }).click();
  await expect(page.getByRole('menu').getByRole('menuitem', { name: 'New database' })).toBeVisible();
});

test('the role editor lists posts and databases beside pages', async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
  await page.goto('/admin/permissions');
  await page.getByRole('button', { name: 'New role' }).first().click();
  const editor = page.getByRole('dialog');
  await expect(editor.getByRole('checkbox', { name: /^Create blog posts\b/ })).toBeVisible();
  await expect(editor.getByRole('checkbox', { name: /^Create databases\b/ })).toBeVisible();
  await expect(editor).toContainText(
    'Write new blog posts in the space, and turn an unpublished page into one.',
  );
});
