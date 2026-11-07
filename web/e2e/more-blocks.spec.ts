// SPDX-License-Identifier: Apache-2.0
// Spec 026: the rest of dynamic blocks, and the small points of the UI test.
import type { Page } from '@playwright/test';
import { ADMIN, checkAccessibility, expect, login, newSpace, publishedPage, test } from './helpers';

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

async function showBlock(page: Page, id: string, title: string, kind: string, settings: object = {}) {
  const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: {
      title,
      document: { type: 'doc', content: [{ type: 'dynamic', attrs: { kind, settings } }] },
      base_version: 1,
    },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
  await page.goto(`/pages/${id}`);
  return page.locator(`.dynamic[data-kind=${kind}]`);
}

test('a page tree opens and closes, one branch or all at once', async ({ page }) => {
  const space = await newSpace(page, 'Tree block');
  const top = await publishedPage(page, space.id, 'Top page', 'x');
  const middle = await publishedPage(page, space.id, 'Middle page', 'x', top);
  await publishedPage(page, space.id, 'Bottom page', 'x', middle);
  const block = await showBlock(page, top, 'Top page', 'page_tree');
  await expect(block.getByRole('link', { name: 'Middle page' })).toBeVisible();
  await expect(block.getByRole('link', { name: 'Bottom page' })).toHaveCount(0);
  await block.getByRole('button', { name: 'Open Middle page' }).click();
  await expect(block.getByRole('link', { name: 'Bottom page' })).toBeVisible();
  await block.getByRole('button', { name: 'Collapse all' }).click();
  await expect(block.getByRole('link', { name: 'Bottom page' })).toHaveCount(0);
  await block.getByRole('button', { name: 'Expand all' }).click();
  await expect(block.getByRole('link', { name: 'Bottom page' })).toBeVisible();
  await checkAccessibility(page);
});

test('the index of a space has a letter bar', async ({ page }) => {
  const space = await newSpace(page, 'Letter block');
  const home = await publishedPage(page, space.id, 'Apple page', 'x');
  await publishedPage(page, space.id, 'Banana page', 'x');
  const block = await showBlock(page, home, 'Apple page', 'space_index');
  await expect(block.getByRole('link', { name: 'Banana page' })).toBeVisible();
  await block.getByRole('button', { name: 'A', exact: true }).click();
  await expect(block.getByRole('link', { name: 'Banana page' })).toHaveCount(0);
  await expect(block.getByRole('link', { name: 'Apple page' })).toBeVisible();
  await block.getByRole('button', { name: 'All', exact: true }).click();
  await expect(block.getByRole('link', { name: 'Banana page' })).toBeVisible();
});

test('a report table shows its columns with words', async ({ page }) => {
  const space = await newSpace(page, 'Report block');
  const home = await publishedPage(page, space.id, 'Report home', 'x');
  const block = await showBlock(page, home, 'Report home', 'report_table', {
    this_space: true,
    columns: ['labels', 'comments'],
  });
  await expect(block.getByRole('columnheader', { name: 'Labels' })).toBeVisible();
  await expect(block.getByRole('columnheader', { name: 'Comments' })).toBeVisible();
  await expect(block.getByRole('columnheader', { name: 'Created by' })).toHaveCount(0);
});

test('activity asks again for each tab', async ({ page }) => {
  const space = await newSpace(page, 'Activity block');
  const home = await publishedPage(page, space.id, 'Activity home', 'x');
  const block = await showBlock(page, home, 'Activity home', 'activity_tabs');
  await expect(block.getByRole('link', { name: 'Activity home' })).toBeVisible();
  await block.getByRole('button', { name: 'People I follow' }).click();
  await expect(block.getByRole('button', { name: 'People I follow' })).toHaveAttribute(
    'aria-pressed',
    'true',
  );
  await expect(block.getByRole('link', { name: 'Activity home' })).toHaveCount(0);
});

test('rotating cards turn by hand and hold still when asked', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const space = await newSpace(page, 'Cards block');
  const first = await publishedPage(page, space.id, 'First card', 'x');
  const second = await publishedPage(page, space.id, 'Second card', 'x');
  const block = await showBlock(page, first, 'First card', 'rotating_cards', { pages: [first, second] });
  await expect(block.getByRole('link', { name: 'First card' })).toBeVisible();
  await block.getByRole('button', { name: 'Next card' }).click();
  await expect(block.getByRole('link', { name: 'Second card' })).toBeVisible();
  await expect(block.getByText('2 of 2')).toBeVisible();
});

test('a timeline draws its lines and tells them to a screen reader', async ({ page }) => {
  const space = await newSpace(page, 'Timeline block');
  const home = await publishedPage(page, space.id, 'Plan', 'x');
  const block = await showBlock(page, home, 'Plan', 'timeline', {
    items: ['Build | First part | 2026-01-01 | 2026-03-01', 'Build | Launch | 2026-03-15'],
  });
  await expect(block.locator('rect')).toHaveCount(1);
  await expect(block.locator('circle')).toHaveCount(1);
  await expect(block.getByRole('listitem')).toHaveCount(2);
  await checkAccessibility(page);
});

test('a button starts a page from a template', async ({ page }) => {
  const space = await newSpace(page, 'Template block');
  const home = await publishedPage(page, space.id, 'Hub', 'x');
  const made = await page.request.post(`/api/v1/spaces/${space.id}/templates`, {
    data: { name: 'Meeting note', document: { type: 'doc', content: [{ type: 'paragraph' }] } },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const template = ((await made.json()) as { id: string }).id;
  const block = await showBlock(page, home, 'Hub', 'create_from_template', {
    template,
    label: 'New meeting',
  });
  await block.getByRole('button', { name: 'New meeting' }).click();
  await expect(page).toHaveURL(/\/pages\/[0-9a-f-]+\/edit$/);

  // The template has no title pattern, and a draft without a title still needs a name.
  await page.goto('/recent');
  await page.getByRole('tab', { name: 'Drafts' }).click();
  await expect(page.getByRole('link', { name: 'Untitled' }).first()).toBeVisible();
});

test('settings stand beside the page, so the block stays in view', async ({ page }) => {
  const space = await newSpace(page, 'Side panel');
  const id = await publishedPage(page, space.id, 'With settings', 'x');
  await showBlock(page, id, 'With settings', 'child_pages');
  await page.goto(`/pages/${id}/edit`);
  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  const panel = page.getByRole('dialog', { name: 'Child pages' });
  const box = await panel.boundingBox();
  const width = page.viewportSize()?.width ?? 0;
  expect((box?.x ?? 0) + (box?.width ?? 0)).toBeCloseTo(width, 0);
  await expect(panel.getByLabel('Show the first lines')).toBeVisible();
});

test('a page with one version offers nothing to compare', async ({ page }) => {
  const space = await newSpace(page, 'One version');
  const id = await publishedPage(page, space.id, 'Only once', 'x');
  await page.goto(`/pages/${id}/history`);
  await expect(page.getByRole('heading', { name: 'History' })).toBeVisible();
  await expect(page.getByRole('columnheader', { name: 'Choose two versions' })).toHaveCount(0);
});

test('a long title stays readable while writing', async ({ page }) => {
  const space = await newSpace(page, 'Long title');
  const id = await publishedPage(page, space.id, 'Short', 'x');
  await page.goto(`/pages/${id}/edit`);
  const title = page.getByLabel('Title', { exact: true });
  const short = await title.boundingBox();
  await title.fill('A title that goes on for long enough to need a second line in the field '.repeat(3));
  await expect
    .poll(async () => (await title.boundingBox())?.height ?? 0)
    .toBeGreaterThan((short?.height ?? 0) * 1.5);
});

test('the administration says who people are in words', async ({ page }) => {
  await page.goto('/admin/users');
  await expect(page.getByRole('cell', { name: 'Local account' }).first()).toBeVisible();
  await expect(page.getByRole('cell', { name: 'local', exact: true })).toHaveCount(0);
});
