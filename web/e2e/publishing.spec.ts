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

// Wave PB2 (spec 073, PUB-1212 to PUB-1214, PUB-1241): the whole space released from the release
// screen by keyboard, with a page left out from its More menu.
test('the whole space is released from the release screen, and a page left out stays in the draft', async ({
  page,
}) => {
  const space = await newSpace(page, 'Manual drafts');
  const guide = await publishedPage(page, space.id, 'Getting started', 'Open the app.', space.home_page_id);
  const notes = await publishedPage(page, space.id, 'Working notes', 'Not for readers.', space.home_page_id);
  const key = unique('MAN');
  const linked = await page.request.post(`/api/v1/spaces/${space.id}/publishing/link`, {
    data: { new_space: { key, name: 'Manual' } },
  });
  expect(linked.ok(), await linked.text()).toBeTruthy();

  await page.goto(`/pages/${notes}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Leave out of releases…' }).click();
  const leaving = page.getByRole('dialog', { name: 'Leave out of releases' });
  await leaving.getByLabel('Reason, shown on the release screen').fill('Internal');
  await leaving.getByRole('button', { name: 'Leave out' }).click();
  await expect(page.getByText('Left out of releases')).toBeVisible();

  await page.goto(`/pages/${guide}`);
  await page.getByRole('link', { name: 'Release', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('cell', { name: 'Getting started', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Release everything that changed…' }).focus();
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Release', exact: true });
  await dialog.getByText('Pages that stay as they are').click();
  await expect(dialog.getByText('Working notes')).toBeVisible();
  await dialog.getByLabel('Note for readers').fill('First edition');
  await dialog.getByRole('button', { name: /^Release \d+ pages?$/ }).click();
  await expect(page.getByText('Everything is released.', { exact: false })).toBeVisible({ timeout: 15000 });
  await checkAccessibility(page);

  const status = await page.request.get(`/api/v1/pages/${notes}/release`);
  expect(((await status.json()) as { status: string }).status).toBe('left_out');
});

// Wave PB3 (spec 073, PUB-710, PUB-711, PUB-720, PUB-1225): the mark is set in the workflow
// editor of a draft space; approving from the band releases the page, its release status sits
// in the same band, and withdrawing it takes the published page off.
test('approval releases a page, and withdrawal takes it off', async ({ page }) => {
  const space = await newSpace(page, 'Procedure drafts');
  const id = await publishedPage(page, space.id, 'Ladder safety', 'Face the ladder.', space.home_page_id);
  const linked = await page.request.post(`/api/v1/spaces/${space.id}/publishing/link`, {
    data: { new_space: { key: unique('PRC'), name: 'Procedures' } },
  });
  expect(linked.ok(), await linked.text()).toBeTruthy();
  const made = await page.request.post(`/api/v1/spaces/${space.id}/workflows`, {
    data: {
      name: 'Review',
      definition: {
        states: [
          { id: 'draft', name: 'Draft', kind: 'draft', start: true },
          { id: 'review', name: 'In review', kind: 'in_review' },
          { id: 'approved', name: 'Approved', kind: 'approved' },
          { id: 'withdrawn', name: 'Withdrawn', kind: 'obsolete' },
        ],
        transitions: [
          { id: 'submit', name: 'Send for review', from: 'draft', to: 'review' },
          { id: 'approve', name: 'Approve', from: 'review', to: 'approved' },
          { id: 'withdraw', name: 'Withdraw', from: 'approved', to: 'withdrawn' },
        ],
      },
    },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const workflow = ((await made.json()) as { id: string }).id;

  await page.goto(`/spaces/${space.key}/settings?section=workflows`);
  await page.getByRole('button', { name: 'Change Review' }).click();
  const mark = page.getByLabel('Release to the published space when a page is approved here');
  await mark.check();
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Save workflow' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Save new version' }).click();
  await expect(mark).toHaveCount(0);

  const set = await page.request.put(`/api/v1/pages/${id}/workflow`, { data: { workflow_id: workflow } });
  expect(set.ok(), await set.text()).toBeTruthy();
  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'Send for review' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Send for review' }).click();
  await expect(page.locator('.byline').getByText('In review', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Approve' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Approve' }).click();
  await expect(async () => {
    await page.reload();
    await expect(page.locator('.control-band .release-line')).toContainText('Released', { timeout: 1000 });
  }).toPass({ timeout: 20000 });
  await checkAccessibility(page);

  const status = await page.request.get(`/api/v1/pages/${id}/release`);
  const twin = ((await status.json()) as { twin_page_id: string }).twin_page_id;
  await page.goto(`/pages/${twin}`);
  await expect(page.locator('.release-line')).toContainText('Approved');

  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'Withdraw' }).click();
  await page.getByRole('dialog').getByRole('button', { name: 'Withdraw' }).click();
  await expect
    .poll(async () => (await page.request.get(`/api/v1/pages/${twin}`)).status(), { timeout: 20000 })
    .toBe(404);
});

// Wave PB4 (spec 073, PUB-1302, PUB-1224): a draft space started from a space that has content,
// its copies in step, and the tree of the draft space listing what changed since its release.
test('a draft space is started from a space with content, and its tree lists what changed', async ({
  page,
}) => {
  const space = await newSpace(page, 'Safety manuals');
  const forklift = await publishedPage(page, space.id, 'Forklift', 'Check the brakes.', space.home_page_id);
  await publishedPage(page, space.id, 'Crane', 'Never lift over people.', space.home_page_id);

  await page.goto(`/spaces/${space.key}/settings?section=published`);
  await page.getByRole('button', { name: 'Start a draft space from this space…' }).click();
  const starting = page.getByRole('dialog', { name: 'Start a draft space' });
  const key = unique('SAF');
  await starting.getByLabel('Name of the draft space').fill('Safety manuals in progress');
  await starting.getByLabel('Key of the draft space').fill(key);
  await starting.getByRole('button', { name: 'See what changes' }).click();
  await expect(starting.getByText('Readers of this space keep reading it', { exact: false })).toBeVisible();
  await starting.getByRole('button', { name: 'Start the draft space' }).click();
  await expect(starting.getByText('The draft space is ready.')).toBeVisible({ timeout: 30000 });
  await starting.getByRole('link', { name: 'Open the draft space' }).click();

  const status = (await (await page.request.get(`/api/v1/pages/${forklift}/release`)).json()) as {
    draft_page_id: string;
  };
  const copy = status.draft_page_id;
  const copied = (await (await page.request.get(`/api/v1/pages/${copy}/release`)).json()) as {
    status: string;
  };
  expect(copied.status).toBe('released');
  const edited = await page.request.post(`/api/v1/pages/${copy}/publish`, {
    data: {
      title: 'Forklift',
      document: {
        type: 'doc',
        content: [{ type: 'paragraph', content: [{ type: 'text', text: 'Check the brakes and the horn.' }] }],
      },
      base_version: 1,
    },
  });
  expect(edited.ok(), await edited.text()).toBeTruthy();

  await page.goto(`/pages/${copy}`);
  await page.getByRole('button', { name: 'Order and view' }).click();
  await page.getByRole('menuitemradio', { name: 'Changed since release' }).click();
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await expect(tree.getByRole('link', { name: 'Forklift' })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Crane' })).toHaveCount(0);
  await checkAccessibility(page);
  await page.getByRole('button', { name: 'Order and view' }).click();
  await page.getByRole('menuitemradio', { name: 'Tree' }).click();
});
