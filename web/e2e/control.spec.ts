// SPDX-License-Identifier: Apache-2.0
// Wave DC1 in the browser: the band of a controlled page, its steps, and the workflows of a
// space in its settings (spec 064).
import {
  ADMIN,
  checkAccessibility,
  createUser,
  EDITOR,
  expect,
  grant,
  login,
  newSpace,
  paragraph,
  PASSWORD,
  publishedPage,
  SIGNED_FLOW,
  signer,
  test,
  totpCode,
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
  await expect(page.locator('.byline').getByText('In review', { exact: true })).toBeVisible();
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

// Wave DC3: what is around the page follows the version a person reads (DOC-500, DOC-560,
// DOC-607). A reader keeps the approved text while the author's change and remark wait; the
// space menu offers every approval of the space; a template names a workflow.
test('a reader keeps the approved version and asks for every approval', async ({ page, browser }) => {
  const space = await newSpace(page, 'Around');
  const id = await publishedPage(page, space.id, 'Cleaning', 'Clean with water', space.home_page_id);
  const made = await page.request.post(`/api/v1/spaces/${space.id}/workflows`, {
    data: { name: 'Review', definition: flow },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const workflow = (await made.json()).id as string;
  expect(
    (await page.request.put(`/api/v1/pages/${id}/workflow`, { data: { workflow_id: workflow } })).ok(),
  ).toBeTruthy();
  for (const transition of ['submit', 'approve']) {
    const band = await (await page.request.get(`/api/v1/pages/${id}`)).json();
    const step = await page.request.post(`/api/v1/pages/${id}/transitions`, {
      data: { transition, version: band.control.head_version },
    });
    expect(step.ok(), await step.text()).toBeTruthy();
  }
  const changed = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title: 'Cleaning', document: paragraph('Clean with a new agent'), base_version: 1 },
  });
  expect(changed.ok(), await changed.text()).toBeTruthy();
  const remark = await page.request.post(`/api/v1/pages/${id}/comments`, {
    data: { body: paragraph('The new agent needs gloves') },
  });
  expect(remark.ok(), await remark.text()).toBeTruthy();

  const name = unique('reader');
  const reader = await createUser(browser, `${name}@example.com`);
  await grant(page, space.id, { type: 'user', id: reader }, VIEWER);
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, `${name}@example.com`, PASSWORD);
  await other.goto(`/pages/${id}`);
  await expect(other.getByText('Clean with water')).toBeVisible();
  await expect(other.getByText('Clean with a new agent')).toHaveCount(0);
  await expect(other.getByText('The new agent needs gloves')).toHaveCount(0);
  await other.getByRole('button', { name: 'More for this space' }).click();
  await other.getByRole('menuitem', { name: 'Tell me about every approval in this space' }).click();
  await expect
    .poll(
      async () => (await (await other.request.get(`/api/v1/spaces/${space.id}/approvals-digest`)).json()).on,
    )
    .toBe(true);
  await other.getByRole('button', { name: 'More for this space' }).click();
  await expect(other.getByRole('menuitem', { name: 'Stop telling me about every approval' })).toBeVisible();
  await context.close();

  const template = await page.request.post(`/api/v1/spaces/${space.id}/templates`, {
    data: { name: 'Procedure' },
  });
  expect(template.ok(), await template.text()).toBeTruthy();
  const templateId = (await template.json()).id as string;
  await page.goto(`/templates/${templateId}/edit`);
  await page.getByLabel('Workflow').selectOption({ label: 'Review' });
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect
    .poll(async () => (await (await page.request.get(`/api/v1/templates/${templateId}`)).json()).workflow_id)
    .toBe(workflow);
  await checkAccessibility(page);
});

// Wave DC5: the band tells a reader when the page comes up for review, and asks them to confirm
// that they read the version, once they opened it (DOC-801, DOC-820 to DOC-824).
test('a reader sees the review date and confirms reading the approved version', async ({ page, browser }) => {
  const space = await newSpace(page, 'Reading');
  const id = await publishedPage(
    page,
    space.id,
    'Hand washing',
    'Wash for twenty seconds',
    space.home_page_id,
  );
  const name = unique('reader');
  const reader = await createUser(browser, `${name}@example.com`);
  await grant(page, space.id, { type: 'user', id: reader }, VIEWER);
  const group = await page.request.post('/api/v1/admin/groups', { data: { name: unique('Staff') } });
  expect(group.ok(), await group.text()).toBeTruthy();
  const groupId = (await group.json()).id as string;
  expect((await page.request.put(`/api/v1/admin/groups/${groupId}/members/${reader}`)).ok()).toBeTruthy();
  const definition = {
    ...flow,
    states: flow.states.map((s) =>
      s.id === 'approved'
        ? { ...s, timed: true, review_months: 12, read_and_understood: { groups: [groupId] } }
        : s,
    ),
  };
  const made = await page.request.post(`/api/v1/spaces/${space.id}/workflows`, {
    data: { name: 'Read it', definition },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const workflow = (await made.json()).id as string;
  expect(
    (await page.request.put(`/api/v1/pages/${id}/workflow`, { data: { workflow_id: workflow } })).ok(),
  ).toBeTruthy();
  for (const transition of ['submit', 'approve']) {
    const band = await (await page.request.get(`/api/v1/pages/${id}`)).json();
    const step = await page.request.post(`/api/v1/pages/${id}/transitions`, {
      data: { transition, version: band.control.head_version },
    });
    expect(step.ok(), await step.text()).toBeTruthy();
  }
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, `${name}@example.com`, PASSWORD);
  await other.goto(`/pages/${id}`);
  const band = other.locator('.control-band');
  await expect(band.getByText('Next periodic review')).toBeVisible();
  await expect(
    band.getByText('You are asked to confirm that you read and understood version 1.'),
  ).toBeVisible();
  await band.getByRole('button', { name: 'I read and understood version 1' }).click();
  await expect(band.getByText('You confirmed that you read version 1.')).toBeVisible();
  await checkAccessibility(other);
  await context.close();
});

// Wave DC4: the signing dialog names the meaning in its heading and its button, asks for the
// password and a code in the same dialog, and says the result in a live region (DOC-720,
// DOC-923). The signature then shows on the band with its mark (DOC-740).
test('an author signs an approval in the signing dialog', async ({ page, browser }) => {
  const space = await newSpace(page, 'Signed');
  const who = await signer(browser, `${unique('signer')}@example.com`, 'Mara Quist');
  await grant(page, space.id, { type: 'user', id: who.id }, EDITOR);
  const id = await publishedPage(
    page,
    space.id,
    'Cleaning procedure B',
    'Clean with water',
    space.home_page_id,
  );
  const made = await page.request.post(`/api/v1/spaces/${space.id}/workflows`, {
    data: { name: 'Signed review', definition: SIGNED_FLOW },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const set = await page.request.put(`/api/v1/pages/${id}/workflow`, {
    data: { workflow_id: (await made.json()).id },
  });
  expect(set.ok(), await set.text()).toBeTruthy();

  const author = who.page;
  await author.goto(`/pages/${id}`);
  await author.getByRole('button', { name: 'Send for review' }).click();
  await author.getByRole('dialog').getByRole('button', { name: 'Send for review' }).click();
  await author.getByRole('button', { name: 'Approve' }).click();
  await author.getByRole('dialog').getByRole('button', { name: 'Continue to sign' }).click();
  const dialog = author.getByRole('dialog', {
    name: /^Sign as approved: version \d+ of “Cleaning procedure B”$/,
  });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Sign as approved' })).toBeDisabled();
  await expect(dialog.getByRole('link', { name: 'Read the exact text of this version' })).toBeVisible();
  await checkAccessibility(author);
  await dialog.getByLabel('Your password').fill(PASSWORD);
  // The code of the next step of 30 seconds: the one of now was used to set the app up.
  await dialog.getByLabel('Code from your authenticator app').fill(totpCode(who.secret, Date.now() + 30_000));
  await dialog.getByRole('button', { name: 'Sign as approved' }).click();
  await expect(author.getByText('Version 1 is approved.')).toBeVisible();
  const signatures = author.getByRole('list', { name: 'Signatures of this version' });
  await expect(signatures.getByText('Signed as approved by Mara Quist')).toBeVisible();
  await expect(signatures.getByText('Signature valid')).toBeVisible();
  await who.close();
});
