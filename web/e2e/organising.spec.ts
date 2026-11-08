// SPDX-License-Identifier: Apache-2.0
// Wave D in the browser: everything that gives a space its order (spec 011).
import type { Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  createUser,
  EDITOR,
  editorSettled,
  expect,
  grant,
  login,
  newSpace,
  PASSWORD,
  publishedPage,
  test,
  unique,
} from './helpers';

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

const main = (page: Page) => page.locator('#main');

async function moreActions(page: Page, item: string) {
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: item }).click();
}

test('a status, labels and a label page', async ({ page }) => {
  const space = await newSpace(page, 'Labels');
  const id = await publishedPage(page, space.id, 'Budget', 'Money', space.home_page_id);
  await page.goto(`/pages/${id}`);

  await page.getByRole('button', { name: 'Set status' }).click();
  await page.getByRole('menuitem', { name: 'Needs review' }).click();
  await expect(page.locator('.byline .status-mark')).toHaveText('Needs review');

  const box = page.getByRole('combobox', { name: 'Add a label' });
  await box.pressSequentially('Team Plans');
  await expect(box).toHaveValue('team-plans');
  await box.press('Enter');
  await expect(page.getByRole('link', { name: 'team-plans' })).toBeVisible();
  await expect(box).toHaveValue('');
  await expect(box).toBeFocused();
  await box.pressSequentially('finance,');
  const labels = page.getByRole('region', { name: 'Labels' });
  await expect(labels.getByRole('link', { name: 'finance' })).toBeVisible();

  await page.getByRole('button', { name: 'Remove label finance' }).click();
  await expect(labels.getByRole('link', { name: 'finance' })).toHaveCount(0);

  await page.getByRole('link', { name: 'team-plans' }).click();
  await expect(page.getByRole('heading', { name: 'Label: team-plans' })).toBeVisible();
  await expect(main(page).getByRole('link', { name: 'Budget' })).toBeVisible();
  await checkAccessibility(page);
});

test('a label on a hidden page shows nowhere', async ({ page, browser }) => {
  const space = await newSpace(page, 'Hidden labels');
  const id = await publishedPage(page, space.id, 'Secret', 'x', space.home_page_id);
  await page.request.post(`/api/v1/pages/${id}/labels`, { data: { label: 'secret-label' } });
  const email = `${unique('reader')}@example.com`;
  await createUser(browser, email);
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, email, PASSWORD);
  await other.goto('/labels');
  await expect(other.getByRole('heading', { name: 'Labels' })).toBeVisible();
  await expect(other.getByText('secret-label')).toHaveCount(0);
  await other.goto('/labels/secret-label');
  await expect(other.getByText('No pages you can see have this label.')).toBeVisible();
  await context.close();
});

test('handing over a page, and a verified mark', async ({ page, browser }) => {
  const space = await newSpace(page, 'Owners');
  const id = await publishedPage(page, space.id, 'Handbook', 'x', space.home_page_id);
  const name = unique('heir');
  const heir = await createUser(browser, `${name}@example.com`);
  await grant(page, space.id, { type: 'user', id: heir }, EDITOR);
  await page.goto(`/pages/${id}`);

  await moreActions(page, 'Details');
  await page.getByRole('button', { name: 'Change owner' }).click();
  await page.getByRole('combobox', { name: 'Find the new owner' }).fill(name);
  await page.getByRole('option', { name: new RegExp(name) }).click();
  await expect(page.locator('.byline')).toContainText(name);
  await page.keyboard.press('Escape');

  await moreActions(page, 'Mark as verified');
  await page.getByLabel('Verified until').fill('2099-12-31');
  await page.getByRole('dialog').getByRole('button', { name: 'Mark as verified' }).click();
  await expect(page.locator('.verified')).toContainText('until 2099-12-31');
});

test('a page from a template: the gallery by keyboard, then the form', async ({ page }) => {
  const space = await newSpace(page, 'Templates');
  await page.goto(`/spaces/${space.key}/overview`);
  await page.getByRole('button', { name: 'More to create' }).click();
  await page.getByRole('menuitem', { name: 'From a template' }).click();
  const dialog = page.getByRole('dialog', { name: 'Start from a template' });
  const find = dialog.getByRole('combobox', { name: 'Find a template' });
  await find.fill('meeting');
  await expect(dialog.getByRole('option', { name: /Meeting notes/ })).toBeVisible();
  await find.press('Enter');
  const form = page.getByRole('dialog', { name: 'Meeting notes' });
  await expect(form.getByRole('button', { name: /Start the page|Next/ })).toBeVisible();
  await checkAccessibility(page);
  await form.locator('input, textarea').first().fill('Team <b>sync</b>');
  while (await form.getByRole('button', { name: 'Next' }).isVisible()) {
    await form.getByRole('button', { name: 'Next' }).click();
  }
  await form.getByRole('button', { name: 'Start the page' }).click();
  await expect(page.getByLabel('Title')).toBeVisible();
  await expect(page.locator('.prose.editing')).toContainText('Team <b>sync</b>');
  await expect(page.locator('.prose.editing b')).toHaveCount(0);
});

test('writing a template with a field and instruction text', async ({ page }) => {
  const space = await newSpace(page, 'Writing templates');
  await page.goto(`/spaces/${space.key}/templates`);
  const name = unique('Template');
  await page.getByLabel('New template name').fill(name);
  await page.getByRole('button', { name: 'New template' }).click();
  await expect(page.locator('.prose.editing')).toBeVisible();
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.type('/field');
  await page.keyboard.press('Enter');
  const field = page.getByRole('dialog', { name: 'Field' });
  await field.getByLabel('Name').fill('Topic');
  await field.getByRole('button', { name: 'Add' }).click();
  await expect(page.locator('.template-field')).toHaveText('Topic');
  await editorSettled(page);
  await page.keyboard.type(' /instruction');
  await page.keyboard.press('Enter');
  const help = page.getByRole('dialog', { name: 'Instruction text' });
  await help.getByLabel('Help for the writer').fill('Say why');
  await help.getByRole('button', { name: 'Add' }).click();
  await expect(page.locator('.instruction')).toHaveText('Say why');
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Saved' })).toBeVisible();

  await page.goto(`/spaces/${space.key}/overview`);
  await page.getByRole('button', { name: 'More to create' }).click();
  await page.getByRole('menuitem', { name: 'From a template' }).click();
  const dialog = page.getByRole('dialog', { name: 'Start from a template' });
  await dialog.getByRole('combobox', { name: 'Find a template' }).fill(name);
  await dialog.getByRole('button', { name: 'Use this template' }).click();
  await page.getByRole('dialog', { name }).getByLabel('Topic').fill('Roadmap');
  await page.getByRole('button', { name: 'Start the page' }).click();
  await expect(page.locator('.prose.editing')).toContainText('Roadmap');

  await page.getByLabel('Title').fill('From the template');
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'From the template', level: 1 })).toBeVisible();
  await expect(page.locator('.prose')).toContainText('Roadmap');
  await expect(page.locator('.prose')).not.toContainText('Say why');
});

test('a blog post stays out of the tree, and a silent publish still makes a version', async ({ page }) => {
  const space = await newSpace(page, 'News');
  await page.goto(`/spaces/${space.key}/overview`);
  await page.getByRole('button', { name: 'More to create' }).click();
  await page.getByRole('menuitem', { name: 'Blog post' }).click();
  await page.getByRole('dialog').getByLabel('Title').fill('Launch day');
  await page.getByRole('dialog').getByRole('button', { name: 'Create' }).click();
  await page.locator('.prose.editing').click();
  await page.keyboard.type('We shipped.');
  await page.getByRole('button', { name: 'Publish options' }).click();
  const options = page.getByRole('dialog', { name: 'Publish options' });
  await options.getByLabel('Notify watchers').uncheck();
  await options.getByRole('button', { name: 'Preview' }).click();
  await expect(page.getByRole('dialog', { name: 'Preview' }).locator('.prose')).toContainText('We shipped.');
  await page.getByRole('dialog', { name: 'Preview' }).getByRole('button', { name: 'Close' }).click();
  await options.getByRole('button', { name: 'Done' }).click();
  await page.getByRole('button', { name: 'Publish', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Launch day', level: 1 })).toBeVisible();

  await expect(page.getByRole('navigation', { name: 'Pages' }).getByText('Launch day')).toHaveCount(0);
  await page.getByRole('link', { name: 'Blog', exact: true }).click();
  await expect(main(page).getByRole('link', { name: 'Launch day' })).toBeVisible();
  await page.goto('/blog');
  await expect(main(page).getByRole('link', { name: 'Launch day' })).toBeVisible();
});

test('scheduling: the page shows a mark, and the schedule can be cancelled', async ({ page }) => {
  const space = await newSpace(page, 'Schedules');
  const id = await publishedPage(page, space.id, 'Later', 'x', space.home_page_id);
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await page.keyboard.type('More. ');
  await expect(page.getByText('Saved')).toBeVisible();
  await page.getByRole('button', { name: 'Publish options' }).click();
  const options = page.getByRole('dialog', { name: 'Publish options' });
  await options.getByLabel('Publish at').fill('2099-06-01T09:00');
  await options.getByRole('button', { name: 'Schedule' }).click();
  await expect(options).toHaveCount(0);
  await page.goto(`/pages/${id}`);
  await expect(page.getByText('Publishes by itself')).toBeVisible();
});

test('a folder shows its pages, a link entry leaves the space', async ({ page }) => {
  const space = await newSpace(page, 'Kinds');
  const id = await publishedPage(page, space.id, 'Group', 'Old text', space.home_page_id);
  await publishedPage(page, space.id, 'Inside', 'x', id);
  await page.goto(`/pages/${id}`);
  await moreActions(page, 'Turn into folder');
  await page.getByRole('dialog').getByRole('button', { name: 'Turn into folder' }).click();
  await expect(main(page).getByRole('link', { name: 'Inside' })).toBeVisible();
  await expect(page.locator('.prose')).toHaveCount(0);

  await page.getByRole('button', { name: 'Add a link' }).click();
  const dialog = page.getByRole('dialog', { name: 'Add a link' });
  await dialog.getByLabel('Title').fill('Our shop');
  await dialog.getByLabel('Address').fill('https://example.com/shop');
  await dialog.getByRole('button', { name: 'Create' }).click();
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await tree.getByRole('button', { name: 'Group', exact: true }).click();
  const entry = tree.getByRole('link', { name: 'Our shop' });
  await expect(entry).toHaveAttribute('href', 'https://example.com/shop');
  await expect(entry).toHaveAttribute('target', '_blank');
});

test('moving to another space shows what changes first', async ({ page }) => {
  const from = await newSpace(page, 'From here');
  const to = await newSpace(page, 'To there');
  const id = await publishedPage(page, from.id, 'Traveller', 'x', from.home_page_id);
  await publishedPage(page, from.id, 'Luggage', 'x', id);
  await page.goto(`/pages/${id}`);
  await moreActions(page, 'Move');
  const dialog = page.getByRole('dialog', { name: 'Move' });
  await dialog.getByLabel('Space').selectOption({ label: 'To there' });
  await expect(dialog.getByRole('status')).toContainText('This moves 2 pages.');
  await expect(dialog.getByRole('status')).toContainText('Everyone who can see the target space');
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Move' }).click();
  await expect(dialog).toHaveCount(0);
  await page.reload();
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await tree.getByRole('button', { name: 'Traveller', exact: true }).click();
  await expect(tree.getByRole('link', { name: 'Luggage' })).toBeVisible();
  await expect(page.locator('.crumbs')).toContainText('To there');
  expect(to.id).not.toBe(from.id);
});

test('copying with new titles', async ({ page }) => {
  const space = await newSpace(page, 'Copies');
  const id = await publishedPage(page, space.id, 'Plan 2026', 'x', space.home_page_id);
  await page.goto(`/pages/${id}`);
  await moreActions(page, 'Copy');
  const dialog = page.getByRole('dialog', { name: 'Copy' });
  await dialog.getByLabel('Find', { exact: true }).fill('2026');
  await dialog.getByLabel('Replace with').fill('2027');
  await expect(dialog.getByRole('list', { name: 'New titles' })).toContainText('Plan 2027');
  await dialog.getByRole('button', { name: 'Copy' }).click();
  await expect(page.getByRole('heading', { name: 'Plan 2027', level: 1 })).toBeVisible();
});

test('several rows move together, with the keyboard', async ({ page }) => {
  const space = await newSpace(page, 'Selecting');
  const target = await newSpace(page, 'Selected target');
  const a = await publishedPage(page, space.id, 'Alpha', 'x', space.home_page_id);
  await publishedPage(page, space.id, 'Beta', 'x', space.home_page_id);
  await publishedPage(page, space.id, 'Gamma', 'x', space.home_page_id);
  await page.goto(`/pages/${a}`);
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await page.getByRole('button', { name: 'Select pages' }).click();
  await tree.getByRole('link', { name: 'Alpha' }).focus();
  await page.keyboard.press('Space');
  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('Space');
  await expect(page.getByRole('group', { name: 'Select pages' })).toContainText('2 selected');
  await page.getByRole('group', { name: 'Select pages' }).getByRole('button', { name: 'Move' }).click();
  const dialog = page.getByRole('dialog', { name: 'Move 2 pages' });
  await dialog.getByLabel('Space').selectOption({ label: 'Selected target' });
  await dialog.getByRole('button', { name: 'Move' }).click();
  await expect(page.getByText('Moved: 2. Failed: 0.')).toBeVisible();
  await page.goto(`/spaces/${target.key}/overview`);
  await expect(tree.getByRole('link', { name: 'Alpha' })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Beta' })).toBeVisible();
});

test('dropping a row on a space opens the Move dialog', async ({ page }) => {
  const space = await newSpace(page, 'Dragging');
  await newSpace(page, 'Drop target');
  const id = await publishedPage(page, space.id, 'Dragged', 'x', space.home_page_id);
  await page.goto(`/pages/${id}?move_to=${space.id}`);
  await expect(page.getByRole('dialog', { name: 'Move' })).toBeVisible();
  await page.getByRole('dialog', { name: 'Move' }).getByRole('button', { name: 'Close' }).click();
  await expect(page).toHaveURL(new RegExp(`/pages/${id}$`));

  const row = page.getByRole('navigation', { name: 'Pages' }).getByText('Dragged');
  const menu = page.getByRole('button', { name: 'Spaces', exact: true });
  await row.hover();
  await page.mouse.down();
  await menu.hover();
  await menu.hover();
  await page.getByRole('menu', { name: 'Spaces' }).getByRole('menuitem').first().hover();
  await page.mouse.up();
  await expect(page.getByRole('dialog', { name: 'Move' }).getByLabel('Space')).toHaveValue(/.+/);
  await expect(page).toHaveURL(/move_to=/);
});

test('an approval from asking to approved, and a new version removes the mark', async ({ page, browser }) => {
  const space = await newSpace(page, 'Approvals');
  const id = await publishedPage(page, space.id, 'Policy', 'First text', space.home_page_id);
  const name = unique('approver');
  const email = `${name}@example.com`;
  const approver = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: approver }, EDITOR);

  await page.goto(`/pages/${id}`);
  await moreActions(page, 'Approvals');
  const dialog = page.getByRole('dialog', { name: 'Approvals' });
  await dialog.getByRole('combobox', { name: 'Find a person' }).fill(name);
  await dialog.getByRole('option', { name: new RegExp(name) }).click();
  await dialog.getByRole('button', { name: 'Send request' }).click();
  await expect(dialog.getByText('Still to answer')).toBeVisible();
  await checkAccessibility(page);

  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, email, PASSWORD);
  await other.goto(`/pages/${id}`);
  await other.getByRole('button', { name: 'Give your answer' }).click();
  await other.getByRole('dialog').getByRole('button', { name: 'Approve', exact: true }).click();
  await other.reload();
  await expect(other.getByText('Approved: version 1')).toBeVisible();
  await context.close();

  const next = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title: 'Policy',
      document: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'New' }] }] },
      base_version: 1,
    },
  });
  expect(next.ok(), await next.text()).toBeTruthy();
  await page.goto(`/pages/${id}`);
  await expect(page.getByText('Version 1 was approved. This version is newer.')).toBeVisible();
});

test('archiving: read-only, out of the tree, and a restore below an archived parent', async ({ page }) => {
  const space = await newSpace(page, 'Archive');
  const parent = await publishedPage(page, space.id, 'Old parent', 'x', space.home_page_id);
  const child = await publishedPage(page, space.id, 'Old child', 'x', parent);
  await page.goto(`/pages/${parent}`);
  await moreActions(page, 'Archive');
  await page.getByRole('dialog').getByRole('button', { name: 'Archive' }).click();
  await expect(page.getByText('This page is archived.')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Edit', exact: true })).toHaveCount(0);
  const tree = page.getByRole('navigation', { name: 'Pages' });
  await expect(tree.getByText('Old parent')).toHaveCount(0);

  await page.goto(`/pages/${child}`);
  await page.getByRole('button', { name: 'Restore' }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await page.getByRole('button', { name: 'Restore both' }).click();
  await expect(page.getByText('This page is archived.')).toHaveCount(0);
  await expect(tree.getByRole('link', { name: 'Old parent' })).toBeVisible();
  await expect(tree.getByRole('link', { name: 'Old child' })).toBeVisible();
});

test('the Content screen filters, shows a summary and reports', async ({ page }) => {
  const space = await newSpace(page, 'Content');
  const id = await publishedPage(page, space.id, 'Tidy me', 'x', space.home_page_id);
  await page.request.post(`/api/v1/pages/${id}/labels`, { data: { label: 'tidy' } });
  await page.goto(`/spaces/${space.key}/content`);
  await page.getByRole('form', { name: 'Filters' }).getByLabel('Labels').fill('tidy');
  await expect(page).toHaveURL(/label=tidy/);
  await expect(page.getByText('1 page', { exact: true })).toBeVisible();
  await page.getByRole('checkbox', { name: 'Tidy me' }).check();
  await page
    .getByRole('group', { name: 'Actions on the selection' })
    .getByRole('button', { name: 'Add label' })
    .click();
  const dialog = page.getByRole('dialog', { name: 'Add label' });
  await expect(dialog).toContainText('This touches 1 of 1 pages.');
  await dialog.getByLabel('Add a label').fill('checked');
  await dialog.getByRole('button', { name: 'Add label' }).click();
  await expect(dialog.getByText('Done: 1. Failed: 0.')).toBeVisible();
  await checkAccessibility(page);
  await dialog.getByRole('button', { name: 'Done' }).click();
  await expect(page.getByRole('cell', { name: /checked/ })).toBeVisible();
});

test('the search language and its filters live in the address', async ({ page }) => {
  const space = await newSpace(page, 'Searching');
  const word = unique('needle');
  const id = await publishedPage(page, space.id, `Found ${word}`, 'x', space.home_page_id);
  await page.request.post(`/api/v1/pages/${id}/labels`, { data: { label: 'hr' } });
  await page.goto(`/search?q=${encodeURIComponent(`"found ${word}" AND label:hr`)}`);
  await expect(main(page).getByRole('link', { name: `Found ${word}` })).toBeVisible();
  await expect(page.getByText('1 result', { exact: true })).toBeVisible();

  await page.goto(`/search?q=${encodeURIComponent('((( "open AND NOT * : ?')}`);
  await expect(page.getByRole('heading', { name: 'Search' })).toBeVisible();
  await expect(page.getByRole('alert')).toHaveCount(0);

  await page.goto(`/search?q=${word}`);
  await page.getByLabel('Titles only').check();
  await expect(page).toHaveURL(/titles=1/);
  await page.getByRole('tab', { name: 'Spaces' }).click();
  await expect(page).toHaveURL(/tab=spaces/);
});

test('changing a key: the old address still leads there', async ({ page }) => {
  const space = await newSpace(page, 'Rekeyed');
  await page.goto(`/spaces/${space.key}/settings?section=key`);
  const key = unique('NEW');
  await page.getByRole('textbox', { name: 'Key', exact: true }).fill(key);
  await page.getByRole('textbox', { name: 'Key', exact: true }).press('Enter');
  await expect(page).toHaveURL(new RegExp(`/spaces/${key}/settings`));
  await page.goto(`/spaces/${space.key}/overview`);
  await expect(page.getByRole('heading', { name: 'Rekeyed', level: 1 })).toBeVisible();
});

test('deleting a space asks for its key', async ({ page }) => {
  const space = await newSpace(page, 'Doomed space');
  await page.goto(`/spaces/${space.key}/settings?section=danger`);
  await page.getByRole('button', { name: 'Delete space' }).click();
  const dialog = page.getByRole('dialog', { name: 'Delete space' });
  await expect(dialog.getByRole('button', { name: 'Delete space' })).toBeDisabled();
  await dialog.getByRole('textbox').fill(space.key);
  await dialog.getByRole('button', { name: 'Delete space' }).click();
  await expect(page).toHaveURL(/\/$/);
});

test('the trash can be searched, and restores in bulk', async ({ page }) => {
  const space = await newSpace(page, 'Bulk trash');
  const a = await publishedPage(page, space.id, 'Gone one', 'x', space.home_page_id);
  const b = await publishedPage(page, space.id, 'Gone two', 'x', space.home_page_id);
  await page.request.delete(`/api/v1/pages/${a}`);
  await page.request.delete(`/api/v1/pages/${b}`);
  await page.goto(`/spaces/${space.key}/trash`);
  await page.getByLabel('Find in the trash').fill('two');
  await expect(page.getByRole('cell', { name: 'Gone one' })).toHaveCount(0);
  await page.getByLabel('Find in the trash').fill('');
  await page.getByRole('checkbox', { name: 'Gone one' }).check();
  await page.getByRole('checkbox', { name: 'Gone two' }).check();
  await page.getByRole('group').getByRole('button', { name: 'Restore' }).click();
  await expect(page.getByText('The trash is empty.')).toBeVisible();
});

test('an administrator removes one version, and it shows as removed', async ({ page }) => {
  const space = await newSpace(page, 'Versions');
  const id = await publishedPage(page, space.id, 'Leak', 'secret', space.home_page_id);
  const fixed = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title: 'Leak',
      document: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'safe' }] }] },
      base_version: 1,
    },
  });
  expect(fixed.ok(), await fixed.text()).toBeTruthy();
  await page.goto(`/pages/${id}/history`);
  await page.getByRole('button', { name: 'Remove' }).click();
  const dialog = page.getByRole('dialog', { name: 'Remove version 1' });
  await dialog.getByLabel('Reason').fill('A leaked password');
  await dialog.getByRole('button', { name: 'Remove' }).click();
  await expect(page.locator('.lozenge', { hasText: 'Removed' })).toBeVisible();
});

test('text survives a failed save, and Close waits or asks', async ({ page }) => {
  const space = await newSpace(page, 'Safe text');
  const id = await publishedPage(page, space.id, 'Fragile', 'x', space.home_page_id);
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await page.route(`**/api/v1/pages/${id}/draft`, (route) => route.abort());
  await page.keyboard.type('Keep me ');
  await page.getByRole('link', { name: 'Close', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Your text is not saved' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Keep editing' }).click();
  await page.unroute(`**/api/v1/pages/${id}/draft`);

  await page.reload();
  await page.getByRole('button', { name: 'Put it back' }).click();
  await expect(page.locator('.prose.editing')).toContainText('Keep me');
});

test('a link to a page that does not exist yet starts it', async ({ page }) => {
  const space = await newSpace(page, 'New from a link');
  await page.goto(`/spaces/${space.key}/new?title=Wanted+page`);
  await expect(page.getByLabel('Title')).toHaveValue('Wanted page');
});

test('a new space stays with the person who makes it, unless they choose otherwise', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'New space' }).first().click();
  const dialog = page.getByRole('dialog', { name: 'New space' });
  await expect(dialog.getByLabel('Who gets access')).toHaveValue('private');
  await expect(dialog).toContainText('Only you can see it');
  await dialog.getByLabel('Who gets access').selectOption('team_space');
  await expect(dialog).toContainText('Every member can read and write here.');
});

test('the company home shows on Home, with a way to the personal screen', async ({ page }) => {
  const space = await newSpace(page, 'Company');
  const id = await publishedPage(page, space.id, 'Welcome to us', 'Our news', space.home_page_id);
  const set = await page.request.put('/api/v1/admin/organising', {
    data: { company_home: id, draft_days: 365, default_preset: '' },
  });
  expect(set.status(), await set.text()).toBeLessThan(300);
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Welcome to us' })).toBeVisible();
  await page.getByRole('link', { name: 'Your start screen' }).click();
  await expect(page.getByRole('heading', { name: 'Spaces' })).toBeVisible();
  await page.request.put('/api/v1/admin/organising', {
    data: {
      company_home: '00000000-0000-0000-0000-000000000000',
      draft_days: 365,
      default_preset: '',
    },
  });
});

test('the trash does not name a page hidden from the person, and it cannot be purged by hand', async ({
  page,
  browser,
}) => {
  const space = await newSpace(page, 'Hidden trash');
  const email = `${unique('keeper')}@example.com`;
  const keeper = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: keeper }, [...EDITOR, 'restriction.manage']);
  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, email, PASSWORD);
  const id = await publishedPage(other, space.id, 'Secret plan', 'x', space.home_page_id);
  const restricted = await other.request.put(`/api/v1/pages/${id}/restrictions`, {
    data: { kind: 'view', subjects: [{ type: 'user', id: keeper }] },
  });
  expect(restricted.ok(), await restricted.text()).toBeTruthy();
  const removed = await other.request.delete(`/api/v1/pages/${id}`);
  expect(removed.ok(), await removed.text()).toBeTruthy();
  await context.close();

  await page.goto(`/spaces/${space.key}/trash`);
  await expect(page.getByRole('cell', { name: 'A restricted page', exact: true })).toBeVisible();
  await expect(main(page)).not.toContainText('Secret plan');
  await expect(page.getByRole('button', { name: 'Delete for good' })).toHaveCount(0);
  await checkAccessibility(page);
  await main(page).getByRole('button', { name: 'Restore' }).click();
  await expect(page.getByText('The trash is empty.')).toBeVisible();
});

test('a copy says when it would lose a restriction from above, and stops there', async ({
  page,
  browser,
}) => {
  const space = await newSpace(page, 'Copy restrictions');
  const email = `${unique('copier')}@example.com`;
  const copier = await createUser(browser, email);
  await grant(page, space.id, { type: 'user', id: copier }, EDITOR);
  const parent = await publishedPage(page, space.id, 'Closed parent', 'x', space.home_page_id);
  const child = await publishedPage(page, space.id, 'Inner page', 'x', parent);
  const me = (await (await page.request.get('/api/v1/me')).json()) as { user: { id: string } };
  const closed = await page.request.put(`/api/v1/pages/${parent}/restrictions`, {
    data: {
      kind: 'view',
      subjects: [
        { type: 'user', id: copier },
        { type: 'user', id: me.user.id },
      ],
    },
  });
  expect(closed.ok(), await closed.text()).toBeTruthy();

  const context = await browser.newContext({ baseURL: 'http://localhost:8080' });
  const other = await context.newPage();
  await login(other, email, PASSWORD);
  await other.goto(`/pages/${child}`);
  await moreActions(other, 'Copy');
  const dialog = other.getByRole('dialog', { name: 'Copy' });
  await expect(dialog.getByRole('button', { name: 'Copy', exact: true })).toBeEnabled();
  await dialog.getByLabel('New parent').selectOption({ label: 'Top level' });
  await expect(dialog.getByRole('status')).toContainText('Closed parent');
  await expect(dialog.getByRole('status')).toContainText('manage restrictions');
  await expect(dialog.getByRole('button', { name: 'Copy', exact: true })).toBeDisabled();
  await context.close();
});
