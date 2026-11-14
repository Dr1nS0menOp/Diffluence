// SPDX-License-Identifier: Apache-2.0
// Wave PB1 in the browser (spec 073): linking a published space from the settings of a space,
// releasing one page from its More menu, and a reader who finds the released page and never
// the draft.
import {
  ADMIN,
  checkAccessibility,
  createUser,
  expect,
  grant,
  login,
  newSpace,
  PASSWORD,
  publishedPage,
  test,
  unique,
  VIEWER,
} from './helpers';

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('a space is linked to a new published space, and one page is released to its readers', async ({
  page,
  browser,
}) => {
  const space = await newSpace(page, 'Handbook drafts');
  const id = await publishedPage(page, space.id, 'Leave policy', 'Ask a week ahead.', space.home_page_id);

  await page.goto(`/spaces/${space.key}/settings?section=published`);
  await page.getByRole('button', { name: 'Link a published space…' }).click();
  const linking = page.getByRole('dialog', { name: 'Link a published space' });
  const key = unique('PUB');
  await linking.getByLabel('Name of the new space').fill('Handbook for everyone');
  await linking.getByLabel('Key of the new space').fill(key);
  await linking.getByRole('button', { name: 'See what changes' }).click();
  await expect(linking.getByText('Linking releases nothing.', { exact: false })).toBeVisible();
  await checkAccessibility(page);
  await linking.getByRole('button', { name: 'Link the spaces' }).click();
  await expect(page.getByText('This is the draft space of Handbook for everyone.')).toBeVisible();
  await checkAccessibility(page);

  await page.goto(`/pages/${id}`);
  await expect(page.getByText('Not released yet')).toBeVisible();
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Release this page…' }).click();
  const releasing = page.getByRole('dialog', { name: 'Release this page' });
  await expect(
    releasing.getByText('This page reaches readers for the first time', { exact: false }),
  ).toBeVisible();
  await releasing.getByLabel('Note for readers').fill('The first edition');
  await releasing.getByRole('button', { name: 'Release this page' }).click();
  await expect(page.locator('.release-line')).toContainText('Released');
  await expect(page.getByRole('link', { name: 'Open the published page' })).toBeVisible();

  const status = await page.request.get(`/api/v1/pages/${id}/release`);
  const twin = ((await status.json()) as { twin_page_id: string }).twin_page_id;
  const published = (await (await page.request.get(`/api/v1/space-keys/${key}`)).json()) as { id: string };
  const name = unique('reader');
  const reader = await createUser(browser, `${name}@example.com`);
  await grant(page, published.id, { type: 'user', id: reader }, VIEWER);
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, `${name}@example.com`, PASSWORD);
  await other.goto(`/pages/${twin}`);
  await expect(other.getByRole('heading', { name: 'Leave policy' })).toBeVisible();
  await expect(other.getByText('Ask a week ahead.')).toBeVisible();
  await expect(other.getByRole('link', { name: 'Open the draft' })).toHaveCount(0);
  await expect(other.getByRole('link', { name: 'Edit' })).toHaveCount(0);
  await checkAccessibility(other);
  await other.goto(`/pages/${id}`);
  await expect(other.getByText('does not exist, or you cannot see it')).toBeVisible();
  await context.close();
});
