// SPDX-License-Identifier: Apache-2.0
// Spec 016: the home of a space and the example space. Since spec 078 the space's header row
// leads home, and the Spaces panel does the work of the strip of spaces (frame.spec.ts).
import { ADMIN, checkAccessibility, expect, login, newSpace, publishedPage, test, unique } from './helpers';

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('the home page is marked as home, and is not a row of the tree', async ({ page }) => {
  const name = unique('Hearth');
  const space = await newSpace(page, name);
  const child = await publishedPage(page, space.id, 'Below home', 'x', space.home_page_id);
  await page.goto(`/spaces/${space.key}`);
  await expect(page).toHaveURL(new RegExp(`/pages/${space.home_page_id}$`));

  const home = page.locator('#space-head-link');
  await expect(home).toHaveText(name);
  await expect(home).toHaveAttribute('aria-current', 'page');
  // The path of the home page is the space alone, as the current place (PAG-143).
  await expect(page.locator('.crumbs [aria-current]')).toHaveText(name);

  const tree = page.getByRole('navigation', { name: 'Pages' });
  await expect(tree.getByRole('link', { name: 'Below home' })).toBeVisible();
  await expect(tree.getByRole('link', { name })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Show the current page' })).toHaveCount(0);
  await expect(tree.locator('a[data-row]').first()).toHaveAttribute('tabindex', '0');

  await page.getByLabel('Filter pages').fill(name);
  await expect(tree).toContainText('No page has that in its title.');
  await page.getByLabel('Filter pages').fill('');
  await page.getByRole('button', { name: 'Order and view' }).click();
  await page.getByRole('menuitemradio', { name: 'A to Z' }).click();
  await expect(tree.getByRole('link', { name })).toHaveCount(0);
  await page.getByRole('button', { name: 'Order and view' }).click();
  await page.getByRole('menuitemradio', { name: 'Tree' }).click();
  await checkAccessibility(page);

  await tree.getByRole('link', { name: 'Below home' }).click();
  await expect(page).toHaveURL(new RegExp(`/pages/${child}$`));
  await expect(home).not.toHaveAttribute('aria-current', /./);
  await expect(page.locator('.crumbs [aria-current]')).toHaveCount(0);
  await home.click();
  await expect(page).toHaveURL(new RegExp(`/pages/${space.home_page_id}$`));
});

test('a row can be dropped between top rows that have different parents', async ({ page }) => {
  const space = await newSpace(page, 'Mixed tops');
  const first = await publishedPage(page, space.id, 'Under home', 'x', space.home_page_id);
  await publishedPage(page, space.id, 'Also under home', 'x', space.home_page_id);
  await publishedPage(page, space.id, 'No parent', 'x');
  await page.goto(`/pages/${first}`);

  const tree = page.getByRole('navigation', { name: 'Pages' });
  const rows = tree.locator('a[data-row]');
  await expect(rows).toHaveText(['Under home', 'Also under home', 'No parent']);
  await tree.getByRole('link', { name: 'No parent' }).dragTo(page.getByTestId('gap-Under home'));
  await expect(rows).toHaveText(['No parent', 'Under home', 'Also under home']);
  await expect(page.getByRole('alert')).toHaveCount(0);
});

test('an administrator adds the example space, and every page of it works', async ({ page }) => {
  await page.goto('/admin/settings');
  await page.getByRole('button', { name: 'Add example space' }).click();
  await expect(page.locator('h1')).toContainText('Welcome to Kenwerk');
  await expect(page.locator('#space-head-link')).toHaveAttribute('aria-current', 'page');

  const tree = page.getByRole('navigation', { name: 'Pages' });
  const titles = [
    'Start here',
    'Writing a page',
    'Blocks',
    'Tables',
    'Images and files',
    'Code',
    'A wide page',
    'A long read',
    'Only the owner edits this',
  ];
  // The tree starts folded; Blocks to Code live under Writing a page.
  await tree.getByRole('button', { name: 'Writing a page', exact: true }).click();
  for (const title of titles) {
    await tree.getByRole('link', { name: title, exact: true }).click();
    await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible();
    await expect(page.locator('.prose').first()).toBeVisible();
    await checkAccessibility(page);
  }

  await tree.getByRole('link', { name: 'Images and files', exact: true }).click();
  await expect(page.locator('.page-header-image')).toBeVisible();
  await expect(page.locator('figure.doc-image img').first()).toHaveJSProperty('complete', true);
  await expect(page.locator('.file-tile')).toContainText('team-notes.txt');

  await tree.getByRole('link', { name: 'Blocks', exact: true }).click();
  await expect(page.locator('.panel')).toHaveCount(7);
  await expect(page.locator('.decision')).toBeVisible();

  await tree.getByRole('link', { name: 'A long read', exact: true }).click();
  await expect(page.locator('.byline')).toBeVisible();
  await expect(tree.locator('.node-mark').first()).toBeVisible();

  // A second one has no key left.
  await page.goto('/admin/settings');
  await page.getByRole('button', { name: 'Add example space' }).click();
  await expect(page.getByRole('alert')).toContainText('This key is already in use.');
});
