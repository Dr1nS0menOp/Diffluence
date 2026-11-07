// SPDX-License-Identifier: Apache-2.0
// Spec 016: the home of a space, the strip of spaces, and the example space.
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

  const links = page.getByRole('navigation', { name: 'Space links' });
  const home = links.getByRole('link', { name: 'Home', exact: true });
  await expect(home).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('.crumbs [aria-current]')).toHaveText('Home');

  const tree = page.getByRole('navigation', { name: 'Pages' });
  await expect(tree.getByRole('link', { name: 'Below home' })).toBeVisible();
  await expect(tree.getByRole('link', { name })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Show the current page' })).toHaveCount(0);
  await expect(tree.locator('a[data-row]').first()).toHaveAttribute('tabindex', '0');

  await page.getByLabel('Filter pages').fill(name);
  await expect(tree).toContainText('No page has that in its title.');
  await page.getByLabel('Filter pages').fill('');
  await page.getByLabel('Order of the pages').selectOption('alphabet');
  await expect(tree.getByRole('link', { name })).toHaveCount(0);
  await page.getByLabel('Order of the pages').selectOption('tree');
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

test('the strip lists spaces, marks the current one and works from the keyboard', async ({ page }) => {
  const name = unique('Strip');
  const current = await newSpace(page, name);
  const others = [];
  for (let i = 0; i < 8; i++) others.push(await newSpace(page, unique(`Other ${i}`)));
  await page.goto(`/pages/${current.home_page_id}`);

  const strip = page.getByRole('navigation', { name: 'Your spaces' });
  const chip = strip.getByRole('link', { name, exact: true });
  await expect(chip).toHaveAttribute('aria-current', 'true');
  await expect(chip).toBeInViewport();
  await expect(strip.getByRole('link', { name: 'All spaces' })).toBeVisible();
  await checkAccessibility(page);

  const list = strip.getByRole('list');
  const scrolls = await list.evaluate((el) => el.scrollWidth > el.clientWidth);
  expect(scrolls).toBeTruthy();
  const page_scroll = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  expect(page_scroll).toBeTruthy();

  const before = await list.evaluate((el) => el.scrollLeft);
  await list.hover();
  await page.mouse.wheel(0, 200);
  await expect.poll(() => list.evaluate((el) => el.scrollLeft)).not.toBe(before);

  const last = strip.getByRole('listitem').last().getByRole('link');
  await last.focus();
  await expect(last).toBeInViewport();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/pages\//);
  await expect(strip.locator('[aria-current]')).not.toHaveText(name);
});

test('an archived space keeps its place in its own strip', async ({ page }) => {
  const name = unique('Old strip');
  const space = await newSpace(page, name);
  const res = await page.request.post(`/api/v1/spaces/${space.id}/archive`, { data: {} });
  expect(res.status()).toBe(204);
  await page.goto(`/pages/${space.home_page_id}`);
  const strip = page.getByRole('navigation', { name: 'Your spaces' });
  await expect(strip.getByRole('link', { name, exact: true })).toHaveAttribute('aria-current', 'true');
});

test('an administrator adds the example space, and every page of it works', async ({ page }) => {
  await page.goto('/admin/settings');
  await page.getByRole('button', { name: 'Add example space' }).click();
  await expect(page.locator('h1')).toContainText('Welcome to Diffluence');
  const links = page.getByRole('navigation', { name: 'Space links' });
  await expect(links.getByRole('link', { name: 'Home', exact: true })).toHaveAttribute(
    'aria-current',
    'page',
  );

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
  await expect(page.locator('.byline')).toContainText('min read');
  await expect(tree.locator('.node-mark').first()).toBeVisible();

  // A second one has no key left.
  await page.goto('/admin/settings');
  await page.getByRole('button', { name: 'Add example space' }).click();
  await expect(page.getByRole('alert')).toContainText('This key is already in use.');
});
