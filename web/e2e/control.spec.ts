// SPDX-License-Identifier: Apache-2.0
// Wave DC1 in the browser: the band of a controlled page, its steps, and the workflows of a
// space in its settings (spec 064).
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

const flow = {
  states: [
    { id: 'draft', name: 'Draft', kind: 'draft', start: true },
    { id: 'review', name: 'In review', kind: 'in_review' },
    { id: 'approved', name: 'Approved', kind: 'approved' },
  ],
  transitions: [
    { id: 'submit', name: 'Send for review', from: 'draft', to: 'review' },
    { id: 'approve', name: 'Approve', from: 'review', to: 'approved' },
    { id: 'change', name: 'Back to draft', from: 'approved', to: 'draft' },
  ],
};

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

test('a controlled page is hidden from readers until an author approves it', async ({ page, browser }) => {
  const space = await newSpace(page, 'Controlled');
  const id = await publishedPage(page, space.id, 'Lifting procedure', 'Lift with care', space.home_page_id);
  const made = await page.request.post(`/api/v1/spaces/${space.id}/workflows`, {
    data: { name: 'Review', definition: flow },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const workflow = (await made.json()).id as string;
  const set = await page.request.put(`/api/v1/pages/${id}/workflow`, { data: { workflow_id: workflow } });
  expect(set.ok(), await set.text()).toBeTruthy();

  const name = unique('reader');
  const reader = await createUser(browser, `${name}@example.com`);
  await grant(page, space.id, { type: 'user', id: reader }, VIEWER);
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, `${name}@example.com`, PASSWORD);
  await other.goto(`/pages/${id}`);
  await expect(other.getByText('does not exist, or you cannot see it')).toBeVisible();

  await page.goto(`/pages/${id}`);
  await expect(page.getByText('Never approved: readers do not see this page yet.')).toBeVisible();
  await expect(page.getByText('The workflow Review is set on this page.')).toBeVisible();
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Send for review' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Send for review' }).click();
  await expect(page.getByText('In review', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Approve' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Approve' }).click();
  await expect(page.getByText('Version 1 is approved.')).toBeVisible();

  await other.goto(`/pages/${id}`);
  await expect(other.getByRole('heading', { name: 'Lifting procedure' })).toBeVisible();
  await expect(other.getByText('Version 1 is approved.')).toBeVisible();
  await expect(other.getByRole('button', { name: 'Back to draft' })).toHaveCount(0);
  await context.close();

  await page.goto(`/spaces/${space.key}/settings?section=workflows`);
  await expect(page.getByRole('cell', { name: 'Review', exact: true })).toBeVisible();
  await expect(page.getByRole('cell', { name: 'Draft, In review, Approved' })).toBeVisible();
  await checkAccessibility(page);
});
