// SPDX-License-Identifier: Apache-2.0
// Spec 012: blocks that fill themselves, charts and the files of a space.
import type { Page } from '@playwright/test';
import {
  ADMIN,
  checkAccessibility,
  editorSettled,
  expect,
  login,
  newSpace,
  paragraph,
  publishedPage,
  test,
} from './helpers';

test.beforeEach(async ({ page }) => {
  await login(page, ADMIN.email, ADMIN.password);
});

async function publish(page: Page, id: string, title: string, content: unknown[]) {
  const done = await page.request.post(`/api/v1/pages/${id}/publish`, {
    data: { title, document: { type: 'doc', content }, base_version: 1 },
  });
  expect(done.ok(), await done.text()).toBeTruthy();
}

const cell = (type: string, text: string) => ({
  type,
  content: [{ type: 'paragraph', content: [{ type: 'text', text }] }],
});
const row = (type: string, ...texts: string[]) => ({
  type: 'tableRow',
  content: texts.map((text) => cell(type, text)),
});

test('a block lists the child pages of the page it stands on', async ({ page }) => {
  const space = await newSpace(page, 'Dynamic list');
  const parent = await publishedPage(page, space.id, 'Parent page', 'x');
  await publishedPage(page, space.id, 'Child page', 'x', parent);
  await publish(page, parent, 'Parent page', [{ type: 'dynamic', attrs: { kind: 'child_pages' } }]);
  await page.goto(`/pages/${parent}`);
  const block = page.locator('.dynamic[data-kind=child_pages]');
  await expect(block.getByRole('link', { name: 'Child page' })).toBeVisible();
  await checkAccessibility(page);
});

test('a writer adds a block from the insert menu and sets it', async ({ page }) => {
  const space = await newSpace(page, 'Dynamic insert');
  const id = await publishedPage(page, space.id, 'With a block', 'start');
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await page.keyboard.press('Control+End');
  await page.keyboard.press('Enter');
  await page.keyboard.type('/Recent changes');
  await page.getByRole('listbox', { name: 'Insert' }).getByRole('option').first().click();
  await page.getByRole('button', { name: 'Settings' }).click();
  const dialog = page.getByRole('dialog', { name: 'Recent changes' });
  await dialog.getByLabel('At most').fill('3');
  await dialog.getByRole('button', { name: 'Close' }).click();
  await expect(page.locator('.dynamic[data-kind=recent_changes]')).toBeVisible();
});

test('a chart follows its table and keeps the numbers readable', async ({ page }) => {
  const space = await newSpace(page, 'Chart space');
  const id = await publishedPage(page, space.id, 'With a chart', 'x');
  await publish(page, id, 'With a chart', [
    {
      type: 'table',
      attrs: { block_id: 'numbers01' },
      content: [row('tableHeader', 'Quarter', 'Pages'), row('tableCell', 'Q1', '12')],
    },
    { type: 'chart', attrs: { table: 'numbers01', kind: 'bar', title: 'By quarter' } },
  ]);
  await page.goto(`/pages/${id}`);
  const chart = page.getByRole('group', { name: 'By quarter' });
  await expect(chart.getByRole('img')).toBeVisible();
  await chart.getByRole('button', { name: 'Show the numbers' }).click();
  await expect(chart.getByRole('cell', { name: '12' })).toBeVisible();
  await checkAccessibility(page);
});

test('a writer adds a chart from the table the cursor is in', async ({ page }) => {
  const space = await newSpace(page, 'Chart insert');
  const id = await publishedPage(page, space.id, 'Table first', 'x');
  await publish(page, id, 'Table first', [
    { type: 'table', content: [row('tableHeader', 'Quarter', 'Pages'), row('tableCell', 'Q1', '12')] },
  ]);
  await page.goto(`/pages/${id}/edit`);
  await page.getByRole('cell', { name: 'Q1' }).click();
  await editorSettled(page);
  await page.keyboard.press('End');
  await page.keyboard.type(' /Chart');
  await page.getByRole('listbox', { name: 'Insert' }).getByRole('option').first().click();
  await expect(page.getByRole('img', { name: /Chart/ })).toBeVisible();
  await page.getByLabel('Kind of chart').selectOption('pie');
  await expect(page.locator('.chart-slice').first()).toBeVisible();
});

test('an excerpt shows on another page, with its source', async ({ page }) => {
  const space = await newSpace(page, 'Excerpt space');
  const source = await publishedPage(page, space.id, 'Source page', 'x');
  await publish(page, source, 'Source page', [
    { type: 'excerpt', attrs: { name: 'intro' }, content: [paragraph('Shared words').content[0]] },
  ]);
  const shown = await publishedPage(page, space.id, 'Showing page', 'x');
  await publish(page, shown, 'Showing page', [
    { type: 'dynamic', attrs: { kind: 'show_excerpt', settings: { page: source, name: 'intro' } } },
  ]);
  await page.goto(`/pages/${shown}`);
  await expect(page.getByText('Shared words')).toBeVisible();
  await expect(page.getByRole('link', { name: 'From Source page' })).toBeVisible();
});

test('the files of a space are in one list, and versions can go back', async ({ page }) => {
  const space = await newSpace(page, 'Files space');
  const id = await publishedPage(page, space.id, 'With files', 'x');
  for (const text of ['first', 'second']) {
    const sent = await page.request.post(`/api/v1/pages/${id}/attachments?filename=notes.txt`, {
      headers: { 'Content-Type': 'text/plain' },
      data: text,
    });
    expect(sent.ok(), await sent.text()).toBeTruthy();
  }
  await page.goto(`/spaces/${space.key}/files`);
  await expect(page.getByRole('link', { name: 'notes.txt' }).first()).toBeVisible();
  await checkAccessibility(page);

  await page.goto(`/pages/${id}`);
  await page.getByRole('button', { name: 'More actions' }).click();
  await page.getByRole('menuitem', { name: 'Attachments' }).click();
  await page.getByRole('button', { name: 'Details' }).click();
  const dialog = page.getByRole('dialog', { name: 'notes.txt' });
  await expect(dialog.getByText('second')).toBeVisible();
  await dialog.getByRole('button', { name: 'Make current' }).click();
  await expect(dialog.getByText('first')).toBeVisible();
});

test('a file with a known name asks before it adds a version', async ({ page }) => {
  const space = await newSpace(page, 'Same name');
  const id = await publishedPage(page, space.id, 'Same name page', 'x');
  const add = (name: string) =>
    page.locator('input[type=file][aria-label="Add image or file"]').setInputFiles({
      name,
      mimeType: 'text/plain',
      buffer: Buffer.from('hello'),
    });
  await page.goto(`/pages/${id}/edit`);
  await page.locator('.prose.editing').click();
  await editorSettled(page);
  await add('same.txt');
  await expect(page.locator('.file-tile')).toHaveCount(1);
  await add('same.txt');
  const dialog = page.getByRole('dialog', { name: 'That name is taken' });
  await dialog.getByRole('button', { name: 'Keep both' }).click();
  await expect(page.getByText('same (2).txt')).toBeVisible();
});
