// SPDX-License-Identifier: Apache-2.0
// Spec 028: search in the middle of the bar, and a tree that loads what is opened. Spec 080:
// the rebuilt results screen.
import type { Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createNamedUser,
  expect,
  login,
  newSpace,
  paragraph,
  publishedPage,
  test,
  unique,
} from './helpers';

/** Five levels: the start of a tree brings three, so the last ones have to be asked for. */
async function deepTree(page: Page) {
  const space = await newSpace(page, 'Deep space');
  const one = await publishedPage(page, space.id, 'Level one', 'x');
  const two = await publishedPage(page, space.id, 'Level two', 'x', one);
  const three = await publishedPage(page, space.id, 'Level three', 'x', two);
  const four = await publishedPage(page, space.id, 'Level four', 'x', three);
  const five = await publishedPage(page, space.id, 'Level five', 'x', four);
  return { space, one, three, four, five };
}

const tree = (page: Page) => page.getByRole('navigation', { name: 'Pages' });

/** The tree starts folded from the first level (spec 033), so the first two levels are opened by hand. */
async function openToLevelThree(page: Page) {
  await tree(page).getByRole('button', { name: 'Level one', exact: true }).click();
  await tree(page).getByRole('button', { name: 'Level two', exact: true }).click();
  await expect(tree(page).getByRole('link', { name: 'Level three' })).toBeVisible();
}

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('a branch that did not come with the start loads when it opens', async ({ page }) => {
  const { space, one } = await deepTree(page);
  await page.goto(`/pages/${one}`);
  await openToLevelThree(page);
  await expect(tree(page).getByRole('link', { name: 'Level four' })).toHaveCount(0);
  const asked = page.waitForRequest((r) => r.url().includes(`/spaces/${space.id}/tree/branches?parent=`));
  await tree(page).getByRole('button', { name: 'Level three', exact: true }).click();
  await asked;
  await expect(tree(page).getByRole('link', { name: 'Level four' })).toBeVisible();
});

test('a page far down opens with the way to it in the tree', async ({ page }) => {
  const { five } = await deepTree(page);
  await page.goto(`/pages/${five}`);
  await expect(tree(page).locator('a[aria-current="page"]')).toHaveText('Level five');
  await expect(tree(page).getByRole('link', { name: 'Level four' })).toBeVisible();
});

test('the filter finds a page the tree has not loaded', async ({ page }) => {
  const { one } = await deepTree(page);
  await page.goto(`/pages/${one}`);
  await expect(tree(page).getByRole('link', { name: 'Level four' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Filter the tree' }).click();
  await page.getByRole('searchbox', { name: 'Filter pages' }).fill('five');
  await expect(tree(page).getByRole('link', { name: 'Level five' })).toBeVisible();
  await expect(tree(page).getByRole('link', { name: 'Level four' })).toBeVisible();
  await page.getByRole('searchbox', { name: 'Filter pages' }).fill('');
  await expect(tree(page).getByRole('link', { name: 'Level five' })).toHaveCount(0);
});

test('a new page shows up in a branch that was loaded by hand', async ({ page }) => {
  const { space, one, four } = await deepTree(page);
  await page.goto(`/pages/${one}`);
  await openToLevelThree(page);
  await tree(page).getByRole('button', { name: 'Level three', exact: true }).click();
  await expect(tree(page).getByRole('link', { name: 'Level four' })).toBeVisible();
  await tree(page).getByRole('button', { name: 'Level four', exact: true }).click();
  await expect(tree(page).getByRole('link', { name: 'Level five' })).toBeVisible();
  await publishedPage(page, space.id, 'Level five too', 'x', four);
  await page.reload();
  await expect(tree(page).getByRole('link', { name: 'Level five too' })).toBeVisible();
});

test('the search box stands in the middle of the bar', async ({ page }) => {
  await page.setViewportSize({ width: 1920, height: 1000 });
  await page.goto('/');
  const box = await page.getByRole('search').boundingBox();
  const middle = (box?.x ?? 0) + (box?.width ?? 0) / 2;
  expect(Math.abs(middle - 960)).toBeLessThan(100);
  expect(box?.width ?? 0).toBeGreaterThan(400);
});

test('typing answers in the box, and Enter goes to all results', async ({ page }) => {
  const space = await newSpace(page, 'Search space');
  const word = unique('harbour');
  await publishedPage(page, space.id, `The ${word} plan`, 'x');
  await page.goto('/');
  const box = page.getByRole('combobox', { name: 'Search' });
  await box.fill(word);
  await expect(page.getByRole('option', { name: new RegExp(`The ${word} plan`) })).toBeVisible();
  await box.press('Enter');
  await expect(page).toHaveURL(new RegExp(`/search\\?q=${word}`));
  await expect(page.getByRole('link', { name: `The ${word} plan` })).toBeVisible();
  await checkAccessibility(page);
});

test('a word that only a comment holds is still found', async ({ page }) => {
  const space = await newSpace(page, 'Comment space');
  const pageId = await publishedPage(page, space.id, 'Quiet page', 'x');
  const word = unique('quartermaster');
  const res = await page.request.post(`/api/v1/pages/${pageId}/comments`, {
    data: { body: paragraph(`About ${word}`) },
  });
  expect(res.status(), await res.text()).toBe(201);
  await page.goto(`/search?q=${word}`);
  await expect(page.getByRole('heading', { name: 'In comments' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Quiet page' })).toBeVisible();
});

test('a search that finds nothing offers wider searches as links in its text', async ({ page }) => {
  const space = await newSpace(page, 'Empty search space');
  const word = unique('lighthouse');
  await publishedPage(page, space.id, `The ${word} plan`, 'x');
  await page.goto(`/search?q=${word}+label:nolabelhere`);
  await expect(page.getByRole('heading', { name: `No pages found for “${word}”` })).toBeVisible();
  await expect(page.getByRole('region', { name: /No pages found/ }).getByRole('button')).toHaveCount(0);
  await expect(page.getByRole('link', { name: /How search works/ })).toHaveAttribute('target', '_blank');
  await checkAccessibility(page);
  await page.getByRole('link', { name: 'search without the filters' }).click();
  await expect(page).toHaveURL(new RegExp(`/search\\?q=${word}$`));
  await expect(page.getByRole('link', { name: `The ${word} plan` })).toBeVisible();
});

test('each result shows the mark of its kind', async ({ page }) => {
  const space = await newSpace(page, 'Kinds space');
  const word = unique('meadow');
  await publishedPage(page, space.id, `A ${word} page`, 'x');
  await page.goto(`/search?q=${word}`);
  const row = page.getByRole('listitem').filter({ hasText: `A ${word} page` });
  await expect(row.getByRole('img', { name: 'Page' })).toBeVisible();
});

test('results narrow to the pages below a chosen page', async ({ page }) => {
  const space = await newSpace(page, 'Branch search space');
  const word = unique('orchard');
  const parent = await publishedPage(page, space.id, `Parent ${word}`, 'the parent');
  await publishedPage(page, space.id, 'Child in the branch', word, parent);
  await publishedPage(page, space.id, 'Elsewhere in the space', word);
  await page.goto(`/search?q=${word}`);
  await expect(page.getByRole('link', { name: 'Elsewhere in the space' })).toBeVisible();
  await page.getByRole('searchbox', { name: 'Below a page' }).fill(`Parent ${word}`);
  await page.getByRole('button', { name: `Parent ${word}` }).click();
  await expect(page).toHaveURL(new RegExp(`below=${parent}`));
  await expect(page.getByRole('link', { name: 'Child in the branch' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Elsewhere in the space' })).toHaveCount(0);
});

test('several people narrow the results, each with a chip of their own', async ({ page, browser }) => {
  const first = await createNamedUser(browser, `${unique('wren')}@example.com`, 'Wren Example');
  const second = await createNamedUser(browser, `${unique('lark')}@example.com`, 'Lark Example');
  await page.goto(`/search?q=anything&contributor=${first}&contributor=${second}`);
  const chips = page.getByRole('list', { name: 'Written or edited by' });
  await expect(chips.getByRole('listitem')).toHaveCount(2);
  await chips.getByRole('button', { name: 'Remove Wren Example' }).click();
  await expect(chips.getByRole('listitem')).toHaveCount(1);
  await expect(page).toHaveURL(new RegExp(`contributor=${second}`));
  await expect(page).not.toHaveURL(new RegExp(`contributor=${first}`));
});
