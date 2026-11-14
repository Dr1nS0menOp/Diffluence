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

// Wave DC2: reviewers answer, the author reads the panel, and readers never see the round.
test('a reviewer answers in the panel, and readers never see the review comments', async ({
  page,
  browser,
}) => {
  // Two new accounts and three logins: on a hosted runner this ran past the usual minute.
  test.slow();
  const space = await newSpace(page, 'Reviewed');
  const id = await publishedPage(
    page,
    space.id,
    'Return policy',
    'Returns within two days',
    space.home_page_id,
  );
  const reviewerName = unique('reviewer');
  const reviewer = await createUser(browser, `${reviewerName}@example.com`);
  await grant(page, space.id, { type: 'user', id: reviewer }, VIEWER);
  const readerName = unique('reader');
  const reader = await createUser(browser, `${readerName}@example.com`);
  await grant(page, space.id, { type: 'user', id: reader }, VIEWER);
  const reviewed = {
    states: [
      { id: 'draft', name: 'Draft', kind: 'draft', start: true },
      {
        id: 'review',
        name: 'In review',
        kind: 'in_review',
        review: {
          reviewers: [{ type: 'user', id: reviewer }],
          rule: 'all',
          authors_may_review: false,
          authors_may_add: false,
        },
      },
      { id: 'approved', name: 'Approved', kind: 'approved' },
      { id: 'rejected', name: 'Changes asked', kind: 'rejected' },
    ],
    transitions: [
      { id: 'submit', name: 'Send for review', from: 'draft', to: 'review' },
      { id: 'approve', name: 'Approve', from: 'review', to: 'approved', outcome: 'approved' },
      { id: 'changes', name: 'Ask for changes', from: 'review', to: 'rejected', outcome: 'changes' },
      { id: 'rework', name: 'Work on it again', from: 'rejected', to: 'draft' },
    ],
  };
  const made = await page.request.post(`/api/v1/spaces/${space.id}/workflows`, {
    data: { name: 'Reviewed', definition: reviewed },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const set = await page.request.put(`/api/v1/pages/${id}/workflow`, {
    data: { workflow_id: (await made.json()).id },
  });
  expect(set.ok(), await set.text()).toBeTruthy();

  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'Send for review' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Send for review' }).click();
  // The author has no Approve button: the reviewers' answers take that step (DOC-013).
  await expect(page.getByRole('button', { name: 'Approve' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Review', exact: true }).click();
  const panel = page.getByRole('complementary', { name: 'Review' });
  await expect(panel.getByText('0 of 1 needed so far.', { exact: false })).toBeVisible();
  await expect(panel.getByRole('columnheader', { name: 'Answer' })).toBeVisible();
  await panel.getByLabel('Review comment').fill('Name who signs off.');
  await panel.getByRole('button', { name: 'Add review comment' }).click();
  await expect(panel.getByText('Name who signs off.')).toBeVisible();
  await checkAccessibility(page);

  // The reviewer reads the never-approved page for the review only, and answers by keyboard.
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, `${reviewerName}@example.com`, PASSWORD);
  await other.goto(`/pages/${id}`);
  await expect(
    other.getByText('You can read this page because you are asked to review it.', { exact: false }),
  ).toBeVisible();
  const approve = other
    .getByRole('complementary', { name: 'Review' })
    .getByRole('button', { name: 'Approve' });
  await approve.focus();
  await other.keyboard.press('Enter');
  const dialog = other.getByRole('dialog', { name: 'Approve' });
  await expect(dialog.getByLabel('Comment')).toBeFocused();
  await other.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(approve).toBeFocused();
  await other.keyboard.press('Enter');
  await expect(dialog.getByLabel('Comment')).toBeFocused();
  await other.keyboard.type('Clear and complete.');
  await dialog.getByRole('button', { name: 'Approve' }).click();
  await context.close();

  await page.reload();
  await expect(page.getByText('Version 1 is approved.')).toBeVisible();

  const readerContext = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const readerPage = await readerContext.newPage();
  await login(readerPage, `${readerName}@example.com`, PASSWORD);
  await readerPage.goto(`/pages/${id}`);
  await expect(readerPage.getByRole('heading', { name: 'Return policy' })).toBeVisible();
  await expect(readerPage.getByRole('button', { name: 'Review', exact: true })).toHaveCount(0);
  await expect(readerPage.getByText('Name who signs off.')).toHaveCount(0);
  const asked = await readerPage.request.get(`/api/v1/pages/${id}/review`);
  expect(asked.status()).toBe(403);
  await readerContext.close();
});

test('a workflow is built by keyboard only, and the validator speaks beside its state', async ({ page }) => {
  const space = await newSpace(page, 'Keyboard');
  await page.goto(`/spaces/${space.key}/settings?section=workflows`);
  const press = async (name: string) => {
    await page.getByRole('button', { name, exact: true }).focus();
    await page.keyboard.press('Enter');
  };
  const typeInto = async (field: ReturnType<typeof page.getByLabel>, text: string) => {
    await field.focus();
    await page.keyboard.type(text);
  };
  await press('New workflow');
  await press('Start empty');
  await typeInto(page.getByLabel('Name of the workflow'), 'Keyboard flow');
  const states = page.locator('.workflow-item').filter({ has: page.getByLabel('Name of the state') });
  // Three states: a draft to start in, a review, and approved. Kinds are chosen with the arrows.
  for (const [i, name, steps] of [
    [0, 'Draft', 0],
    [1, 'In review', 1],
    [2, 'Approved', 2],
  ] as const) {
    await press('Add a state');
    await typeInto(states.nth(i).getByLabel('Name of the state'), name);
    await states.nth(i).getByLabel('Kind').focus();
    for (let n = 0; n < steps; n++) await page.keyboard.press('ArrowDown');
  }
  // With no transitions, the review cannot be reached: the message stands beside that state.
  await expect(states.nth(1).getByRole('alert')).toContainText('cannot be reached');
  await expect(page.getByRole('button', { name: 'Save workflow' })).toBeDisabled();

  const steps = page.locator('.workflow-item').filter({ has: page.getByLabel('Name on the button') });
  for (const [i, name, from, to] of [
    [0, 'Send for review', 0, 1],
    [1, 'Approve', 1, 2],
  ] as const) {
    await press('Add a transition');
    await typeInto(steps.nth(i).getByLabel('Name on the button'), name);
    // A wrapped label names a select with its chosen option too, so take the pair by place.
    await steps.nth(i).locator('.workflow-pair select').first().focus();
    await page.keyboard.press('Home');
    for (let n = 0; n < from; n++) await page.keyboard.press('ArrowDown');
    await steps.nth(i).locator('.workflow-pair select').last().focus();
    await page.keyboard.press('Home');
    for (let n = 0; n < to; n++) await page.keyboard.press('ArrowDown');
  }
  await expect(page.getByText('From In review, Approve leads to Approved.')).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);
  await press('Save workflow');
  await expect(page.getByRole('cell', { name: 'Draft, In review, Approved' })).toBeVisible();
  await checkAccessibility(page);
});
